// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;

namespace GrxFirma.WinUI.Services;

// El evento no transporta datos: su única orden posible es mostrar la ventana.
// Local limita el nombre a la sesión y la DACL concede acceso al usuario actual.
internal sealed class SingleInstanceSignal : IDisposable
{
    private const int ErrorAlreadyExists = 183;
    private const uint WaitObject0 = 0;
    private readonly nint _handle;
    private readonly CancellationTokenSource _stop = new();
    private Task? _listener;

    private SingleInstanceSignal(nint handle, bool isPrimary)
    {
        _handle = handle;
        IsPrimary = isPrimary;
    }

    public bool IsPrimary { get; }

    public static SingleInstanceSignal Open()
    {
        var sid = WindowsIdentity.GetCurrent().User?.Value
            ?? throw new InvalidOperationException("No se pudo identificar al usuario.");
        var name = $@"Local\GrxFirma.WinUI.Show.{sid}";
        var sddl = $"D:P(A;;0x001F0003;;;{sid})(A;;0x001F0003;;;SY)";
        if (!ConvertStringSecurityDescriptorToSecurityDescriptor(
                sddl, 1, out var descriptor, out _))
        {
            throw new Win32Exception(Marshal.GetLastWin32Error());
        }

        try
        {
            var attributes = new SecurityAttributes
            {
                Length = Marshal.SizeOf<SecurityAttributes>(),
                SecurityDescriptor = descriptor,
            };
            var handle = CreateEvent(ref attributes, false, false, name);
            if (handle == 0)
            {
                throw new Win32Exception(Marshal.GetLastWin32Error());
            }
            return new SingleInstanceSignal(
                handle, Marshal.GetLastWin32Error() != ErrorAlreadyExists);
        }
        finally
        {
            _ = LocalFree(descriptor);
        }
    }

    public void ShowExisting()
    {
        if (!IsPrimary && !SetEvent(_handle))
        {
            throw new Win32Exception(Marshal.GetLastWin32Error());
        }
    }

    public void Listen(Action show)
    {
        if (!IsPrimary)
        {
            return;
        }
        _listener = Task.Run(() =>
        {
            while (!_stop.IsCancellationRequested)
            {
                if (WaitForSingleObject(_handle, 0xFFFFFFFF) != WaitObject0)
                {
                    break;
                }
                if (!_stop.IsCancellationRequested)
                {
                    show();
                }
            }
        });
    }

    public void Dispose()
    {
        _stop.Cancel();
        if (IsPrimary)
        {
            _ = SetEvent(_handle);
            try
            {
                if (_listener is not null)
                {
                    _listener.GetAwaiter().GetResult();
                }
            }
            catch
            {
                // El cierre no propaga detalles del canal local.
            }
        }
        _ = CloseHandle(_handle);
        _stop.Dispose();
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct SecurityAttributes
    {
        public int Length;
        public nint SecurityDescriptor;
        public int InheritHandle;
    }

    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true,
        EntryPoint = "ConvertStringSecurityDescriptorToSecurityDescriptorW")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool ConvertStringSecurityDescriptorToSecurityDescriptor(
        string sddl, uint revision, out nint descriptor, out uint size);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern nint LocalFree(nint memory);

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true,
        EntryPoint = "CreateEventW")]
    private static extern nint CreateEvent(
        ref SecurityAttributes attributes,
        [MarshalAs(UnmanagedType.Bool)] bool manualReset,
        [MarshalAs(UnmanagedType.Bool)] bool initialState,
        string name);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetEvent(nint handle);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern uint WaitForSingleObject(nint handle, uint milliseconds);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CloseHandle(nint handle);
}
