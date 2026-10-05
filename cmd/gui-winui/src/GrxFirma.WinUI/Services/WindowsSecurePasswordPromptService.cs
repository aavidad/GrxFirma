// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.ComponentModel;
using System.Runtime.ExceptionServices;
using System.Runtime.InteropServices;
using System.Text;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using WinRT.Interop;

namespace GrxFirma.WinUI.Services;

internal sealed class SecurePasswordPromptException : Exception
{
    internal SecurePasswordPromptException(
        string supportCode,
        Exception innerException)
        : base(
            Localizer.Text("winui.contrasena.windows_no_pudo_completar_el_dialogo"),
            innerException)
    {
        if (string.IsNullOrWhiteSpace(supportCode))
        {
            throw new ArgumentException(
                Localizer.Text("winui.contrasena.el_codigo_de_soporte_es_obligatorio"),
                nameof(supportCode));
        }

        SupportCode = supportCode;
    }

    internal string SupportCode { get; }
}

/// <summary>
/// Diálogo Win32 modal que evita los campos de contraseña WinUI: el secreto
/// pasa directamente del control EDIT nativo a
/// <see cref="NativePasswordBuffer"/>.
/// Debe invocarse desde el hilo propietario de la ventana WinUI.
/// </summary>
public sealed class WindowsSecurePasswordPromptService
    : ISecurePasswordPromptService
{
    private const int IdOk = 1;
    private const int IdCancel = 2;
    private const int DialogFailure = -2;
    private const int GwlpUserData = -21;
    private const int DefaultGuiFont = 17;
    private const int CredUiUsernameCapacity = 514;
    private const int CredUiMaximumPasswordCharacters = 256;
    private const int CredUiMaximumCaptionCharacters = 128;

    private const uint WmClose = 0x0010;
    private const uint WmNcDestroy = 0x0082;
    private const uint WmCommand = 0x0111;
    private const uint WmInitDialog = 0x0110;
    private const uint WmSetFont = 0x0030;
    private const uint WmGetText = 0x000D;
    private const uint WmGetTextLength = 0x000E;
    private const uint EmSetSel = 0x00B1;
    private const uint EmReplaceSel = 0x00C2;
    private const uint EmSetLimitText = 0x00C5;
    private const uint EmSetPasswordChar = 0x00CC;
    private const uint CancellationMessage = 0x8041;
    private const uint ErrorCancelled = 1223;
    private const uint GwOwner = 4;

    private const uint CredUiFlagsDoNotPersist = 0x00002;
    private const uint CredUiFlagsAlwaysShowUi = 0x00080;
    private const uint CredUiFlagsPasswordOnlyOk = 0x00200;
    private const uint CredUiFlagsGenericCredentials = 0x40000;

    private const uint WsPopup = 0x80000000;
    private const uint WsChild = 0x40000000;
    private const uint WsVisible = 0x10000000;
    private const uint WsCaption = 0x00C00000;
    private const uint WsBorder = 0x00800000;
    private const uint WsSysMenu = 0x00080000;
    private const uint WsTabStop = 0x00010000;
    private const uint DsModalFrame = 0x00000080;
    private const uint DsCenter = 0x00000800;
    private const uint WsExClientEdge = 0x00000200;
    private const uint EsPassword = 0x00000020;
    private const uint EsAutoHScroll = 0x00000080;
    private const uint BsDefaultPushButton = 0x00000001;

    private static readonly DialogProcedure DialogProcedureReference =
        ProcessDialogMessage;

    private readonly Func<nint> _windowHandleProvider;
    private readonly DispatcherQueue? _ownerDispatcher;
    private int _activePrompt;

    public WindowsSecurePasswordPromptService(Window ownerWindow)
        : this(
            ownerWindow is null
                ? throw new ArgumentNullException(nameof(ownerWindow))
                : () => WindowNative.GetWindowHandle(ownerWindow),
            ownerWindow.DispatcherQueue)
    {
    }

    public WindowsSecurePasswordPromptService(
        Func<nint> windowHandleProvider)
        : this(windowHandleProvider, null)
    {
    }

    internal WindowsSecurePasswordPromptService(
        Func<nint> windowHandleProvider,
        DispatcherQueue? ownerDispatcher)
    {
        ArgumentNullException.ThrowIfNull(windowHandleProvider);
        _windowHandleProvider = windowHandleProvider;
        _ownerDispatcher = ownerDispatcher;
    }

    public Task<NativePasswordBuffer?> CaptureAsync(
        SecurePasswordPromptRequest request,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        cancellationToken.ThrowIfCancellationRequested();
        request = new SecurePasswordPromptRequest(
            Localizer.Text(request.Title),
            Localizer.Text(request.Label),
            request.MaximumCharacters);
        if (Interlocked.CompareExchange(ref _activePrompt, 1, 0) != 0)
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.contrasena.ya_hay_una_solicitud_segura_de"));
        }

        if (_ownerDispatcher is not null &&
            !_ownerDispatcher.HasThreadAccess)
        {
            var completion =
                new TaskCompletionSource<NativePasswordBuffer?>(
                    TaskCreationOptions.RunContinuationsAsynchronously);
            if (!_ownerDispatcher.TryEnqueue(
                DispatcherQueuePriority.Normal,
                () => CompleteDispatchedCapture(
                    request,
                    cancellationToken,
                    completion)))
            {
                Volatile.Write(ref _activePrompt, 0);
                throw new SecurePasswordPromptException(
                    "PASSWORD_DISPATCH_UNAVAILABLE",
                    new InvalidOperationException(
                        Localizer.Text("winui.contrasena.no_se_pudo_programar_el_dialogo_seguro")));
            }
            return completion.Task;
        }

        return Task.FromResult(
            CaptureOnOwnerThread(request, cancellationToken));
    }

    private NativePasswordBuffer? CaptureOnOwnerThread(
        SecurePasswordPromptRequest request,
        CancellationToken cancellationToken)
    {
        try
        {
            nint owner;
            try
            {
                owner = RequireOwnerWindowOnCurrentThread();
            }
            catch (Exception exception)
                when (exception is not OperationCanceledException)
            {
                throw WrapPromptFailure(
                    "PASSWORD_OWNER",
                    exception);
            }

            try
            {
                return ShowDialog(
                    owner,
                    request,
                    cancellationToken);
            }
            catch (Exception exception)
                when (exception is not OperationCanceledException)
            {
                try
                {
                    return ShowCredentialUiFallback(
                        owner,
                        request,
                        cancellationToken);
                }
                catch (Exception fallbackException)
                    when (fallbackException is not
                        OperationCanceledException)
                {
                    throw WrapPromptFailure(
                        "PASSWORD_DIALOG_FALLBACK",
                        fallbackException);
                }
            }
        }
        finally
        {
            Volatile.Write(ref _activePrompt, 0);
        }
    }

    /// <summary>
    /// Respaldo del sistema para equipos donde la plantilla Win32 no se pueda
    /// crear. CredUI ofrece un diálogo modal de solo contraseña y, con
    /// DO_NOT_PERSIST, no consulta ni escribe el Administrador de
    /// credenciales. El secreto entra directamente en el buffer nativo
    /// borrable.
    /// </summary>
    private static NativePasswordBuffer? ShowCredentialUiFallback(
        nint owner,
        SecurePasswordPromptRequest request,
        CancellationToken cancellationToken)
    {
        cancellationToken.ThrowIfCancellationRequested();
        var maximumCharacters = Math.Min(
            request.MaximumCharacters,
            CredUiMaximumPasswordCharacters);
        var password =
            NativePasswordBuffer.Allocate(maximumCharacters);
        nint userName = 0;
        var userNameBytes =
            checked(CredUiUsernameCapacity * sizeof(char));
        try
        {
            userName = Marshal.AllocHGlobal(userNameBytes);
            NativeMemoryProtection.Zero(userName, userNameBytes);
            var info = new CredUiInfo
            {
                Size = checked((uint)Marshal.SizeOf<CredUiInfo>()),
                Parent = owner,
                MessageText = request.Label.Replace(
                    "&",
                    string.Empty,
                    StringComparison.Ordinal),
                CaptionText = request.Title.Length <=
                    CredUiMaximumCaptionCharacters
                        ? request.Title
                        : request.Title[
                            ..CredUiMaximumCaptionCharacters],
                Banner = 0,
            };
            var cancellation = new CredUiCancellationState(
                owner,
                GetCurrentThreadId());
            using var registration = cancellationToken.Register(
                static state =>
                    ((CredUiCancellationState)state!).RequestCancellation(),
                cancellation);
            cancellationToken.ThrowIfCancellationRequested();

            var save = false;
            var result = CredUIPromptForCredentialsW(
                ref info,
                "GrxFirma/contraseña-transitoria",
                0,
                0,
                userName,
                checked((uint)CredUiUsernameCapacity),
                password.DangerousBuffer,
                checked((uint)(maximumCharacters + 1)),
                ref save,
                CredUiFlagsGenericCredentials |
                    CredUiFlagsAlwaysShowUi |
                    CredUiFlagsDoNotPersist |
                    CredUiFlagsPasswordOnlyOk);

            if (cancellationToken.IsCancellationRequested)
            {
                throw new OperationCanceledException(cancellationToken);
            }
            if (result == ErrorCancelled)
            {
                password.Dispose();
                return null;
            }
            if (result != 0)
            {
                throw new Win32Exception(
                    checked((int)result),
                    Localizer.Text("winui.contrasena.windows_no_pudo_mostrar_el_dialogo_de"));
            }

            password.SetCharacterCount(
                ReadNullTerminatedLength(
                    password.DangerousBuffer,
                    maximumCharacters));
            return password;
        }
        catch
        {
            password.Dispose();
            throw;
        }
        finally
        {
            if (userName != 0)
            {
                NativeMemoryProtection.Zero(
                    userName,
                    userNameBytes);
                Marshal.FreeHGlobal(userName);
            }
        }
    }

    private static int ReadNullTerminatedLength(
        nint buffer,
        int capacityCharacters)
    {
        for (var index = 0; index <= capacityCharacters; index++)
        {
            if (Marshal.ReadInt16(
                    buffer,
                    checked(index * sizeof(char))) == 0)
            {
                return index;
            }
        }
        throw new InvalidOperationException(
            Localizer.Text("winui.contrasena.windows_devolvio_un_secreto_sin"));
    }

    private void CompleteDispatchedCapture(
        SecurePasswordPromptRequest request,
        CancellationToken cancellationToken,
        TaskCompletionSource<NativePasswordBuffer?> completion)
    {
        try
        {
            completion.SetResult(
                CaptureOnOwnerThread(request, cancellationToken));
        }
        catch (OperationCanceledException exception)
        {
            completion.SetCanceled(exception.CancellationToken);
        }
        catch (Exception exception)
        {
            completion.SetException(exception);
        }
    }

    private static SecurePasswordPromptException WrapPromptFailure(
        string stage,
        Exception exception)
    {
        if (exception is SecurePasswordPromptException promptException)
        {
            return promptException;
        }

        var suffix = exception switch
        {
            Win32Exception win32Exception =>
                $"WIN32_{win32Exception.NativeErrorCode}",
            InvalidOperationException => "STATE",
            OverflowException => "RANGE",
            ArgumentException => "ARGUMENT",
            _ =>
                "UNEXPECTED_" +
                exception.GetType().Name.ToUpperInvariant() +
                "_" +
                unchecked((uint)exception.HResult).ToString("X8"),
        };
        return new SecurePasswordPromptException(
            $"{stage}_{suffix}",
            exception);
    }

    private NativePasswordBuffer? ShowDialog(
        nint owner,
        SecurePasswordPromptRequest request,
        CancellationToken cancellationToken)
    {
        var state = new PromptState(request);
        var stateHandle = GCHandle.Alloc(state);
        var template = AllocateDialogTemplate();
        nint result;
        int dialogError;

        try
        {
            using var registration = cancellationToken.Register(
                static value =>
                    ((PromptState)value!).RequestCancellation(),
                state);
            result = DialogBoxIndirectParamW(
                GetModuleHandleW(null),
                template,
                owner,
                DialogProcedureReference,
                GCHandle.ToIntPtr(stateHandle));
            dialogError = result == -1
                ? Marshal.GetLastWin32Error()
                : 0;
        }
        finally
        {
            Marshal.FreeHGlobal(template);
            stateHandle.Free();
        }

        var password = state.TakePassword();
        try
        {
            state.ThrowIfFailed();
            if (result == -1)
            {
                throw new Win32Exception(
                    dialogError,
                    Localizer.Text("winui.contrasena.windows_no_pudo_crear_el_dialogo_seguro"));
            }

            if (result == DialogFailure)
            {
                throw new InvalidOperationException(
                    Localizer.Text("winui.contrasena.el_dialogo_seguro_de_contrasena_termino"));
            }

            if (cancellationToken.IsCancellationRequested)
            {
                throw new OperationCanceledException(cancellationToken);
            }

            if (result == IdOk)
            {
                return password ??
                    throw new InvalidOperationException(
                        Localizer.Text("winui.contrasena.el_dialogo_termino_sin_entregar_el"));
            }

            password?.Dispose();
            return null;
        }
        catch
        {
            password?.Dispose();
            throw;
        }
    }

    private nint RequireOwnerWindowOnCurrentThread()
    {
        var owner = _windowHandleProvider();
        if (owner == 0)
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.comun.la_ventana_propietaria_todavia_no_esta"));
        }

        var ownerThread = GetWindowThreadProcessId(owner, out _);
        if (ownerThread == 0)
        {
            throw new Win32Exception(
                Marshal.GetLastWin32Error(),
                Localizer.Text("winui.contrasena.no_se_pudo_determinar_el_hilo_de_la"));
        }

        if (ownerThread != GetCurrentThreadId())
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.contrasena.el_dialogo_de_contrasena_debe_abrirse"));
        }

        return owner;
    }

    private static nint ProcessDialogMessage(
        nint dialog,
        uint message,
        nuint wParam,
        nint lParam)
    {
        PromptState? state = null;
        try
        {
            if (message == WmInitDialog)
            {
                state = GCHandle.FromIntPtr(lParam).Target as PromptState ??
                    throw new InvalidOperationException(
                        Localizer.Text("winui.contrasena.no_se_recibio_el_estado_del_dialogo"));
                _ = SetWindowLongPtrW(dialog, GwlpUserData, lParam);
                state.AttachDialog(dialog);
                InitializeDialog(dialog, state);
                return 0;
            }

            var statePointer = GetWindowLongPtrW(dialog, GwlpUserData);
            if (statePointer == 0)
            {
                return 0;
            }

            state = GCHandle.FromIntPtr(statePointer).Target as PromptState;
            if (state is null)
            {
                return 0;
            }

            switch (message)
            {
                case WmCommand:
                {
                    var command = unchecked((int)(wParam & 0xFFFF));
                    if (command == IdOk)
                    {
                        CaptureAndClose(dialog, state);
                        return 1;
                    }

                    if (command == IdCancel)
                    {
                        CancelAndClose(dialog, state);
                        return 1;
                    }

                    break;
                }

                case WmClose:
                    CancelAndClose(dialog, state);
                    return 1;

                case CancellationMessage:
                    CancelAndClose(dialog, state);
                    return 1;

                case WmNcDestroy:
                    WipeEditContents(
                        state.DetachEdit(),
                        state.Request.MaximumCharacters);
                    state.DetachDialog();
                    _ = SetWindowLongPtrW(dialog, GwlpUserData, 0);
                    break;
            }

            return 0;
        }
        catch (Exception exception)
        {
            state?.SetFailure(exception);
            if (state is not null)
            {
                WipeEditContents(
                    state.DetachEdit(),
                    state.Request.MaximumCharacters);
            }

            _ = EndDialog(dialog, DialogFailure);
            return 1;
        }
    }

    private static void InitializeDialog(
        nint dialog,
        PromptState state)
    {
        _ = SetWindowTextW(
            dialog,
            state.Request.Title);
        if (!GetClientRect(dialog, out var client))
        {
            throw new Win32Exception(Marshal.GetLastWin32Error());
        }

        var width = Math.Max(client.Right - client.Left, 360);
        var height = Math.Max(client.Bottom - client.Top, 135);
        var module = GetModuleHandleW(null);
        var font = GetStockObject(DefaultGuiFont);

        var label = CreateRequiredControl(
            0,
            "STATIC",
            state.Request.Label,
            WsChild | WsVisible,
            16,
            14,
            width - 32,
            20,
            dialog,
            100,
            module);
        SetControlFont(label, font);

        var edit = CreateRequiredControl(
            WsExClientEdge,
            "EDIT",
            string.Empty,
            WsChild |
                WsVisible |
                WsTabStop |
                WsBorder |
                EsPassword |
                EsAutoHScroll,
            16,
            38,
            width - 32,
            27,
            dialog,
            101,
            module);
        state.AttachEdit(edit);
        _ = SendMessageW(
            edit,
            EmSetLimitText,
            checked((nuint)state.Request.MaximumCharacters),
            0);
        _ = SendMessageW(edit, EmSetPasswordChar, 0x25CF, 0);
        SetControlFont(edit, font);

        var cancel = CreateRequiredControl(
            0,
            "BUTTON",
            Localizer.Text("winui.comun.cancelar"),
            WsChild | WsVisible | WsTabStop,
            width - 112,
            height - 42,
            96,
            28,
            dialog,
            IdCancel,
            module);
        SetControlFont(cancel, font);

        var accept = CreateRequiredControl(
            0,
            "BUTTON",
            Localizer.Text("winui.comun.aceptar"),
            WsChild | WsVisible | WsTabStop | BsDefaultPushButton,
            width - 216,
            height - 42,
            96,
            28,
            dialog,
            IdOk,
            module);
        SetControlFont(accept, font);
        _ = SetFocus(edit);

        if (state.IsCancellationRequested)
        {
            _ = PostMessageW(dialog, CancellationMessage, 0, 0);
        }
    }

    private static void CaptureAndClose(
        nint dialog,
        PromptState state)
    {
        var edit = state.EditHandle;
        var currentLength = checked(
            (int)SendMessageW(edit, WmGetTextLength, 0, 0));
        if (currentLength < 0 ||
            currentLength > state.Request.MaximumCharacters)
        {
            throw new InvalidOperationException(
                Localizer.Text("winui.contrasena.la_longitud_del_secreto_introducido_no"));
        }

        var password = NativePasswordBuffer.Allocate(currentLength);
        try
        {
            var copied = checked(
                (int)SendMessageW(
                    edit,
                    WmGetText,
                    checked((nuint)(currentLength + 1)),
                    password.DangerousBuffer));
            password.SetCharacterCount(copied);
            WipeEditContents(
                state.DetachEdit(),
                state.Request.MaximumCharacters);
            state.SetPassword(password);
        }
        catch
        {
            password.Dispose();
            throw;
        }

        _ = EndDialog(dialog, IdOk);
    }

    private static void CancelAndClose(
        nint dialog,
        PromptState state)
    {
        WipeEditContents(
            state.DetachEdit(),
            state.Request.MaximumCharacters);
        _ = EndDialog(dialog, IdCancel);
    }

    private static void WipeEditContents(
        nint edit,
        int maximumCharacters)
    {
        if (edit == 0)
        {
            return;
        }

        nint filler = 0;
        var fillerBytes = 0;
        try
        {
            var length = checked(
                (int)SendMessageW(edit, WmGetTextLength, 0, 0));
            if (length > 0 && length <= maximumCharacters)
            {
                fillerBytes = checked((length + 1) * sizeof(char));
                filler = Marshal.AllocHGlobal(fillerBytes);
                NativeMemoryProtection.Zero(filler, fillerBytes);
                for (var index = 0; index < length; index++)
                {
                    Marshal.WriteInt16(
                        filler,
                        index * sizeof(char),
                        checked((short)' '));
                }

                _ = SendMessageW(edit, EmSetSel, 0, -1);
                _ = SendMessageW(edit, EmReplaceSel, 0, filler);
            }

            _ = SetWindowTextW(edit, string.Empty);
        }
        catch
        {
            // El control será destruido inmediatamente. El buffer propio se
            // sigue borrando de forma determinista aunque Windows rechace una
            // operación de limpieza defensiva sobre el EDIT.
        }
        finally
        {
            if (filler != 0)
            {
                NativeMemoryProtection.Zero(filler, fillerBytes);
                Marshal.FreeHGlobal(filler);
            }
        }
    }

    private static nint AllocateDialogTemplate()
    {
        const int templateBytes = 24;
        var template = Marshal.AllocHGlobal(templateBytes);
        NativeMemoryProtection.Zero(template, templateBytes);
        Marshal.WriteInt32(
            template,
            0,
            unchecked((int)(
                DsModalFrame |
                DsCenter |
                WsPopup |
                WsCaption |
                WsSysMenu)));
        Marshal.WriteInt32(template, 4, 0);
        Marshal.WriteInt16(template, 8, 0);
        Marshal.WriteInt16(template, 10, 0);
        Marshal.WriteInt16(template, 12, 0);
        Marshal.WriteInt16(template, 14, 260);
        Marshal.WriteInt16(template, 16, 100);
        Marshal.WriteInt16(template, 18, 0);
        Marshal.WriteInt16(template, 20, 0);
        Marshal.WriteInt16(template, 22, 0);
        return template;
    }

    private static nint CreateRequiredControl(
        uint extendedStyle,
        string className,
        string text,
        uint style,
        int x,
        int y,
        int width,
        int height,
        nint parent,
        int identifier,
        nint module)
    {
        var control = CreateWindowExW(
            extendedStyle,
            className,
            text,
            style,
            x,
            y,
            width,
            height,
            parent,
            identifier,
            module,
            0);
        return control != 0
            ? control
            : throw new Win32Exception(Marshal.GetLastWin32Error());
    }

    private static void SetControlFont(nint control, nint font) =>
        _ = SendMessageW(
            control,
            WmSetFont,
            unchecked((nuint)font),
            1);

    private sealed class PromptState
    {
        private nint _dialogHandle;
        private nint _editHandle;
        private int _cancellationRequested;
        private NativePasswordBuffer? _password;
        private ExceptionDispatchInfo? _failure;

        internal PromptState(SecurePasswordPromptRequest request)
        {
            Request = request;
        }

        internal SecurePasswordPromptRequest Request { get; }

        internal bool IsCancellationRequested =>
            Volatile.Read(ref _cancellationRequested) != 0;

        internal nint EditHandle =>
            Volatile.Read(ref _editHandle) is var handle && handle != 0
                ? handle
                : throw new InvalidOperationException(
                    Localizer.Text("winui.contrasena.el_control_de_contrasena_no_esta"));

        internal void AttachDialog(nint dialog)
        {
            Volatile.Write(ref _dialogHandle, dialog);
            if (IsCancellationRequested)
            {
                _ = PostMessageW(dialog, CancellationMessage, 0, 0);
            }
        }

        internal void DetachDialog() =>
            Volatile.Write(ref _dialogHandle, 0);

        internal void AttachEdit(nint edit) =>
            Volatile.Write(ref _editHandle, edit);

        internal nint DetachEdit() =>
            Interlocked.Exchange(ref _editHandle, 0);

        internal void RequestCancellation()
        {
            Volatile.Write(ref _cancellationRequested, 1);
            var dialog = Volatile.Read(ref _dialogHandle);
            if (dialog != 0)
            {
                _ = PostMessageW(
                    dialog,
                    CancellationMessage,
                    0,
                    0);
            }
        }

        internal void SetPassword(NativePasswordBuffer password)
        {
            if (Interlocked.CompareExchange(
                    ref _password,
                    password,
                    null) is not null)
            {
                password.Dispose();
                throw new InvalidOperationException(
                    Localizer.Text("winui.contrasena.el_dialogo_ya_entrego_una_contrasena"));
            }
        }

        internal NativePasswordBuffer? TakePassword() =>
            Interlocked.Exchange(ref _password, null);

        internal void SetFailure(Exception exception) =>
            Interlocked.CompareExchange(
                ref _failure,
                ExceptionDispatchInfo.Capture(exception),
                null);

        internal void ThrowIfFailed() => _failure?.Throw();
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct NativeRectangle
    {
        internal int Left;
        internal int Top;
        internal int Right;
        internal int Bottom;
    }

    [StructLayout(
        LayoutKind.Sequential,
        CharSet = CharSet.Unicode)]
    private struct CredUiInfo
    {
        internal uint Size;
        internal nint Parent;

        [MarshalAs(UnmanagedType.LPWStr)]
        internal string MessageText;

        [MarshalAs(UnmanagedType.LPWStr)]
        internal string CaptionText;

        internal nint Banner;
    }

    private sealed class CredUiCancellationState
    {
        private readonly nint _owner;
        private readonly uint _threadId;
        private int _cancellationRequested;
        private int _dialogClosed;

        internal CredUiCancellationState(
            nint owner,
            uint threadId)
        {
            _owner = owner;
            _threadId = threadId;
        }

        internal void RequestCancellation()
        {
            if (Interlocked.Exchange(
                    ref _cancellationRequested,
                    1) != 0)
            {
                return;
            }
            _ = ThreadPool.UnsafeQueueUserWorkItem(
                static state =>
                    state.CloseDialogWhenAvailable(),
                this,
                preferLocal: false);
        }

        private void CloseDialogWhenAvailable()
        {
            for (
                var attempt = 0;
                attempt < 200 &&
                    Volatile.Read(ref _dialogClosed) == 0;
                attempt++)
            {
                if (TryCloseDialog())
                {
                    Volatile.Write(ref _dialogClosed, 1);
                    return;
                }
                Thread.Sleep(25);
            }
        }

        private bool TryCloseDialog()
        {
            var closed = false;
            _ = EnumThreadWindows(
                _threadId,
                (window, parameter) =>
                {
                    if (
                        GetWindow(window, GwOwner) == _owner &&
                        IsStandardDialogClass(window)
                    )
                    {
                        _ = PostMessageW(
                            window,
                            WmClose,
                            0,
                            0);
                        closed = true;
                        return false;
                    }
                    return true;
                },
                0);
            return closed;
        }

        private static bool IsStandardDialogClass(nint window)
        {
            const string standardDialogClass = "#32770";
            var className = new StringBuilder(32);
            var length = GetClassNameW(
                window,
                className,
                className.Capacity);
            // GetClassNameW devuelve la longitud sin el terminador NUL.
            return length == standardDialogClass.Length &&
                className.ToString().Equals(
                    standardDialogClass,
                    StringComparison.Ordinal);
        }
    }

    private delegate nint DialogProcedure(
        nint dialog,
        uint message,
        nuint wParam,
        nint lParam);

    private delegate bool EnumThreadWindowsProcedure(
        nint window,
        nint parameter);

    [DllImport(
        "credui.dll",
        EntryPoint = "CredUIPromptForCredentialsW",
        ExactSpelling = true,
        CharSet = CharSet.Unicode)]
    private static extern uint CredUIPromptForCredentialsW(
        ref CredUiInfo information,
        [MarshalAs(UnmanagedType.LPWStr)] string targetName,
        nint reserved,
        uint authenticationError,
        nint userName,
        uint userNameCapacity,
        nint password,
        uint passwordCapacity,
        [MarshalAs(UnmanagedType.Bool)] ref bool save,
        uint flags);

    [DllImport(
        "user32.dll",
        EntryPoint = "EnumThreadWindows",
        ExactSpelling = true,
        SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool EnumThreadWindows(
        uint threadIdentifier,
        EnumThreadWindowsProcedure callback,
        nint parameter);

    [DllImport(
        "user32.dll",
        EntryPoint = "GetWindow",
        ExactSpelling = true)]
    private static extern nint GetWindow(
        nint window,
        uint command);

    [DllImport(
        "user32.dll",
        EntryPoint = "GetClassNameW",
        ExactSpelling = true,
        CharSet = CharSet.Unicode,
        SetLastError = true)]
    private static extern int GetClassNameW(
        nint window,
        StringBuilder className,
        int maximumCount);

    [DllImport(
        "user32.dll",
        EntryPoint = "DialogBoxIndirectParamW",
        ExactSpelling = true,
        SetLastError = true)]
    private static extern nint DialogBoxIndirectParamW(
        nint instance,
        nint dialogTemplate,
        nint parent,
        DialogProcedure dialogProcedure,
        nint initializationParameter);

    [DllImport(
        "user32.dll",
        EntryPoint = "CreateWindowExW",
        ExactSpelling = true,
        SetLastError = true,
        CharSet = CharSet.Unicode)]
    private static extern nint CreateWindowExW(
        uint extendedStyle,
        string className,
        string windowName,
        uint style,
        int x,
        int y,
        int width,
        int height,
        nint parent,
        nint menu,
        nint instance,
        nint parameter);

    [DllImport(
        "user32.dll",
        EntryPoint = "SetWindowLongPtrW",
        ExactSpelling = true,
        SetLastError = true)]
    private static extern nint SetWindowLongPtrW(
        nint window,
        int index,
        nint value);

    [DllImport(
        "user32.dll",
        EntryPoint = "GetWindowLongPtrW",
        ExactSpelling = true)]
    private static extern nint GetWindowLongPtrW(
        nint window,
        int index);

    [DllImport(
        "user32.dll",
        EntryPoint = "SendMessageW",
        ExactSpelling = true)]
    private static extern nint SendMessageW(
        nint window,
        uint message,
        nuint wParam,
        nint lParam);

    [DllImport(
        "user32.dll",
        EntryPoint = "PostMessageW",
        ExactSpelling = true,
        SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool PostMessageW(
        nint window,
        uint message,
        nuint wParam,
        nint lParam);

    [DllImport(
        "user32.dll",
        EntryPoint = "EndDialog",
        ExactSpelling = true,
        SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool EndDialog(
        nint dialog,
        nint result);

    [DllImport(
        "user32.dll",
        EntryPoint = "SetWindowTextW",
        ExactSpelling = true,
        SetLastError = true,
        CharSet = CharSet.Unicode)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetWindowTextW(
        nint window,
        string text);

    [DllImport(
        "user32.dll",
        EntryPoint = "GetClientRect",
        ExactSpelling = true,
        SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetClientRect(
        nint window,
        out NativeRectangle rectangle);

    [DllImport(
        "user32.dll",
        EntryPoint = "SetFocus",
        ExactSpelling = true)]
    private static extern nint SetFocus(nint window);

    [DllImport(
        "user32.dll",
        EntryPoint = "GetWindowThreadProcessId",
        ExactSpelling = true,
        SetLastError = true)]
    private static extern uint GetWindowThreadProcessId(
        nint window,
        out uint processIdentifier);

    [DllImport(
        "kernel32.dll",
        EntryPoint = "GetCurrentThreadId",
        ExactSpelling = true)]
    private static extern uint GetCurrentThreadId();

    [DllImport(
        "kernel32.dll",
        EntryPoint = "GetModuleHandleW",
        ExactSpelling = true,
        SetLastError = true,
        CharSet = CharSet.Unicode)]
    private static extern nint GetModuleHandleW(string? moduleName);

    [DllImport(
        "gdi32.dll",
        EntryPoint = "GetStockObject",
        ExactSpelling = true)]
    private static extern nint GetStockObject(int objectIdentifier);
}
