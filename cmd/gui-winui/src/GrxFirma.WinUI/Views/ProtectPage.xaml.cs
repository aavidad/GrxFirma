// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Buffers;
using System.Buffers.Text;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class ProtectPage : Page
{
    private const int Aes256SecretBytes = 32;
    private const int Aes256SecretBase64Characters = 44;

    private static readonly SecurePasswordPromptRequest
        ProtectSecretPrompt = new(
            "Clave efímera de protección",
            "&Clave AES-256 en Base64 (44 caracteres):",
            Aes256SecretBase64Characters);

    private static readonly SecurePasswordPromptRequest
        ProtectSecretConfirmationPrompt = new(
            "Confirmar clave efímera",
            "&Repita la misma clave Base64:",
            Aes256SecretBase64Characters);

    private static readonly SecurePasswordPromptRequest
        UnprotectSecretPrompt = new(
            "Clave efímera de desprotección",
            "&Clave usada al crear el EncryptedData:",
            Aes256SecretBase64Characters);

    private readonly DesktopOperationSession _session;
    private readonly ISecurePasswordPromptService _securePasswordPrompt;
    private CancellationTokenSource? _pageCancellation;
    private bool _isSubscribed;

    public ProtectPage()
    {
        var app = (App)Application.Current;
        _session = app.OperationSession;
        _securePasswordPrompt = app.SecurePasswordPromptService;
        ViewModel = new ProtectPageViewModel(
            _session,
            app.FilePickerService);
        InitializeComponent();
    }

    public ProtectPageViewModel ViewModel { get; }

    private async void OnShareMyCertificateClick(object sender, RoutedEventArgs args)
    {
        if (_pageCancellation is null || XamlRoot is null) return;
        try
        {
            await PublicCertificateExport.ExportAsync(_session,
                ((App)Application.Current).FilePickerService, XamlRoot,
                null, _pageCancellation.Token);
        }
        catch (OperationCanceledException) { }
    }

    private async void OnLoaded(object sender, RoutedEventArgs args)
    {
        if (_isSubscribed)
        {
            return;
        }

        _isSubscribed = true;
        _pageCancellation = new CancellationTokenSource();
        _session.AvailabilityChanged += OnAvailabilityChanged;
        ViewModel.UpdateAvailability();
        if (ViewModel.IsOperationConnected)
        {
            await RefreshCatalogsAndShowDiagnosticAsync(
                _pageCancellation.Token);
        }
    }

    private void OnUnloaded(object sender, RoutedEventArgs args)
    {
        if (_isSubscribed)
        {
            _session.AvailabilityChanged -= OnAvailabilityChanged;
            _isSubscribed = false;
        }

        var cancellation = Interlocked.Exchange(
            ref _pageCancellation,
            null);
        if (cancellation is not null)
        {
            cancellation.Cancel();
            cancellation.Dispose();
        }
        ViewModel.CancelCurrentOperation();
    }

    private void OnAvailabilityChanged(object? sender, EventArgs args)
    {
        _ = DispatcherQueue.TryEnqueue(async () =>
        {
            if (!_isSubscribed)
            {
                return;
            }

            ViewModel.UpdateAvailability();
            var cancellation = _pageCancellation;
            if (ViewModel.IsOperationConnected && cancellation is not null)
            {
                await RefreshCatalogsAndShowDiagnosticAsync(
                    cancellation.Token);
            }
        });
    }

    private void OnRecipientsSelectionChanged(
        object sender,
        SelectionChangedEventArgs args)
    {
        ViewModel.SetSelectedRecipients(
            RecipientsList.SelectedItems
                .OfType<ProtectionRecipientItem>());
    }

    private async void OnAddRecipientClick(object sender, RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null) return;
        try { await ShowDiagnosticIfPresentAsync(await ViewModel.ImportPublicRecipientAsync(cancellation.Token)); }
        catch (OperationCanceledException) when (cancellation.IsCancellationRequested) { }
        catch (Exception exception) { await ShowDiagnosticAsync(OperationDiagnosticMapper.FromException(exception)); }
    }

    private async void OnRemoveRecipientClick(object sender, RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null) return;
        try { await ShowDiagnosticIfPresentAsync(await ViewModel.RemovePublicRecipientAsync(cancellation.Token)); }
        catch (OperationCanceledException) when (cancellation.IsCancellationRequested) { }
        catch (Exception exception) { await ShowDiagnosticAsync(OperationDiagnosticMapper.FromException(exception)); }
    }

    private async void OnSelectProtectInputClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.SelectProtectInputAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnSelectUnprotectInputClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.SelectUnprotectInputAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnProtectClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        byte[]? transientSecret = null;
        byte[]? transientSecretConfirmation = null;
        try
        {
            if (ViewModel.IsEncryptedDataSelected)
            {
                using var capturedSecret =
                    await _securePasswordPrompt.CaptureAsync(
                        ProtectSecretPrompt,
                        cancellation.Token);
                if (capturedSecret is null)
                {
                    return;
                }
                transientSecret =
                    DecodeCanonicalAes256Secret(capturedSecret);
                if (transientSecret is null)
                {
                    await ViewModel.ProtectAsync(
                        null,
                        null,
                        cancellation.Token);
                    return;
                }

                using var capturedConfirmation =
                    await _securePasswordPrompt.CaptureAsync(
                        ProtectSecretConfirmationPrompt,
                        cancellation.Token);
                if (capturedConfirmation is null)
                {
                    return;
                }
                transientSecretConfirmation =
                    DecodeCanonicalAes256Secret(capturedConfirmation);
            }
            var diagnostic = await ViewModel.ProtectAsync(
                transientSecret,
                transientSecretConfirmation,
                cancellation.Token);
            await ShowDiagnosticIfPresentAsync(diagnostic);
        }
        catch (OperationCanceledException)
            when (cancellation.IsCancellationRequested)
        {
        }
        catch (Exception exception)
        {
            await ShowDiagnosticAsync(
                OperationDiagnosticMapper.FromException(exception));
        }
        finally
        {
            ZeroTransientSecret(transientSecret);
            ZeroTransientSecret(transientSecretConfirmation);
        }
    }

    private async void OnUnprotectClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        byte[]? transientSecret = null;
        try
        {
            if (ViewModel.IsEncryptedDataUnprotectSelected)
            {
                using var capturedSecret =
                    await _securePasswordPrompt.CaptureAsync(
                        UnprotectSecretPrompt,
                        cancellation.Token);
                if (capturedSecret is null)
                {
                    return;
                }
                transientSecret =
                    DecodeCanonicalAes256Secret(capturedSecret);
            }
            var diagnostic = await ViewModel.UnprotectAsync(
                transientSecret,
                cancellation.Token);
            await ShowDiagnosticIfPresentAsync(diagnostic);
        }
        catch (OperationCanceledException)
            when (cancellation.IsCancellationRequested)
        {
        }
        catch (Exception exception)
        {
            await ShowDiagnosticAsync(
                OperationDiagnosticMapper.FromException(exception));
        }
        finally
        {
            ZeroTransientSecret(transientSecret);
        }
    }

    private void OnCancelClick(object sender, RoutedEventArgs args)
    {
        ViewModel.CancelCurrentOperation();
    }

    private async void OnOpenProtectedResultClick(
        object sender,
        RoutedEventArgs args)
    {
        var diagnostic =
            ViewModel.ValidateProtectedOutputForOpening();
        if (diagnostic is not null)
        {
            await ShowDiagnosticAsync(diagnostic);
            return;
        }

        await OpenOutputAsync(ViewModel.ProtectedOutputPath);
    }

    private async void OnOpenUnprotectedResultClick(
        object sender,
        RoutedEventArgs args)
    {
        var diagnostic =
            ViewModel.ValidateUnprotectedOutputForOpening();
        if (diagnostic is not null)
        {
            await ShowDiagnosticAsync(diagnostic);
            return;
        }

        await OpenOutputAsync(ViewModel.UnprotectedOutputPath);
    }

    private async Task OpenOutputAsync(string? outputPath)
    {
        if (string.IsNullOrWhiteSpace(outputPath))
        {
            return;
        }

        try
        {
            Process.Start(new ProcessStartInfo
            {
                FileName = outputPath,
                UseShellExecute = true,
                Verb = "open",
            });
        }
        catch (Exception exception)
        {
            await ShowDiagnosticAsync(
                OperationDiagnosticMapper.FromException(exception));
        }
    }

    private async Task RefreshCatalogsAndShowDiagnosticAsync(
        CancellationToken cancellationToken)
    {
        var diagnostic = await ViewModel.RefreshCatalogsAsync(
            cancellationToken);
        if (!cancellationToken.IsCancellationRequested)
        {
            await ShowDiagnosticIfPresentAsync(diagnostic);
        }
    }

    private Task ShowDiagnosticIfPresentAsync(
        OperationDiagnostic? diagnostic) =>
        diagnostic is null
            ? Task.CompletedTask
            : ShowDiagnosticAsync(diagnostic);

    private async Task ShowDiagnosticAsync(
        OperationDiagnostic diagnostic)
    {
        if (!_isSubscribed || XamlRoot is null)
        {
            return;
        }

        var dialog = new OperationDiagnosticDialog(diagnostic)
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
        };
        await Localizer.ShowAsync(dialog);
    }

    private static byte[]? DecodeCanonicalAes256Secret(
        NativePasswordBuffer captured)
    {
        ArgumentNullException.ThrowIfNull(captured);
        return captured.Use<byte[]?>(
            static (pointer, characterCount) =>
        {
            if (characterCount != Aes256SecretBase64Characters)
            {
                return null;
            }

            var encoded = GC.AllocateUninitializedArray<byte>(
                Aes256SecretBase64Characters);
            var canonical = GC.AllocateUninitializedArray<byte>(
                Aes256SecretBase64Characters);
            var secret = GC.AllocateUninitializedArray<byte>(
                Aes256SecretBytes);
            try
            {
                for (var index = 0; index < characterCount; index++)
                {
                    var value = Marshal.ReadInt16(
                        pointer,
                        checked(index * sizeof(char)));
                    if (value is < 0 or > 0x7f)
                    {
                        CryptographicOperations.ZeroMemory(secret);
                        return null;
                    }
                    encoded[index] = checked((byte)value);
                }

                var decodeStatus = Base64.DecodeFromUtf8(
                    encoded,
                    secret,
                    out var consumed,
                    out var written,
                    isFinalBlock: true);
                if (decodeStatus != OperationStatus.Done ||
                    consumed != encoded.Length ||
                    written != secret.Length)
                {
                    CryptographicOperations.ZeroMemory(secret);
                    return null;
                }

                var encodeStatus = Base64.EncodeToUtf8(
                    secret,
                    canonical,
                    out var encodedInput,
                    out var encodedOutput,
                    isFinalBlock: true);
                if (encodeStatus != OperationStatus.Done ||
                    encodedInput != secret.Length ||
                    encodedOutput != canonical.Length ||
                    !CryptographicOperations.FixedTimeEquals(
                        encoded,
                        canonical))
                {
                    CryptographicOperations.ZeroMemory(secret);
                    return null;
                }
                return secret;
            }
            finally
            {
                CryptographicOperations.ZeroMemory(encoded);
                CryptographicOperations.ZeroMemory(canonical);
            }
        });
    }

    private static void ZeroTransientSecret(byte[]? secret)
    {
        if (secret is null || secret.Length == 0)
        {
            return;
        }
        CryptographicOperations.ZeroMemory(secret);
    }
}
