// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;
using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

public sealed partial class SettingsPage : Page
{
    private static readonly SecurePasswordPromptRequest
        ProxyPasswordPrompt = new(
            "Contraseña del proxy",
            "&Contraseña que se protegerá para el usuario actual:",
            maximumCharacters:
                GrxFirma.WinUI.Core.Operations
                    .DesktopOperationsClient.MaximumPasswordBytes);

    private readonly ISecurePasswordPromptService _securePasswordPrompt;
    private CancellationTokenSource? _pageCancellation;
    private bool _isLoaded;
    private bool _startupLoaded;

    public SettingsPage()
    {
        var app = (App)Application.Current;
        _securePasswordPrompt = app.SecurePasswordPromptService;
        ViewModel = new SettingsPageViewModel(
            app.OperationSession);
        InitializeComponent();
    }

    public SettingsPageViewModel ViewModel { get; }

    private async void OnLoaded(
        object sender,
        RoutedEventArgs args)
    {
        if (_isLoaded)
        {
            return;
        }

        _isLoaded = true;
        _pageCancellation = new CancellationTokenSource();
        ViewModel.DiagnosticRequested += OnDiagnosticRequested;
        ViewModel.ThemePreferenceApplied += OnThemePreferenceApplied;
        ViewModel.Activate();
        await ViewModel.LoadAsync();
        try
        {
            StartWithWindowsCheckBox.IsChecked =
                WindowsStartupRegistration.IsEnabled;
            _startupLoaded = true;
        }
        catch
        {
            StartWithWindowsCheckBox.IsEnabled = false;
        }
        if (ViewModel.CanRefreshProxySecretStatus)
        {
            await ViewModel.RefreshProxySecretStatusAsync();
        }
    }

    private void OnUnloaded(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded)
        {
            return;
        }

        _isLoaded = false;
        _startupLoaded = false;
        var cancellation = Interlocked.Exchange(
            ref _pageCancellation,
            null);
        if (cancellation is not null)
        {
            cancellation.Cancel();
            cancellation.Dispose();
        }
        ViewModel.DiagnosticRequested -= OnDiagnosticRequested;
        ViewModel.ThemePreferenceApplied -= OnThemePreferenceApplied;
        ViewModel.Deactivate();
    }

    private void OnThemePreferenceApplied(int? themeIndex)
        => ((App)Application.Current).ApplyThemePreference(themeIndex);

    private async void OnStartupChanged(object sender, RoutedEventArgs args)
    {
        if (!_startupLoaded)
        {
            return;
        }
        try
        {
            WindowsStartupRegistration.SetEnabled(
                StartWithWindowsCheckBox.IsChecked == true);
        }
        catch
        {
            _startupLoaded = false;
            try
            {
                StartWithWindowsCheckBox.IsChecked =
                    WindowsStartupRegistration.IsEnabled;
            }
            catch
            {
                StartWithWindowsCheckBox.IsChecked = false;
            }
            _startupLoaded = true;
            var dialog = new ContentDialog
            {
                Title = "No se pudo cambiar el inicio con Windows",
                Content = "Compruebe que GrxFirma esté instalada para este usuario en su ubicación habitual y vuelva a intentarlo.",
                CloseButtonText = "Aceptar",
                XamlRoot = XamlRoot,
            };
            await dialog.ShowAsync();
        }
    }

    private async void OnReloadClick(
        object sender,
        RoutedEventArgs args)
    {
        await ViewModel.LoadAsync();
        if (ViewModel.CanRefreshProxySecretStatus)
        {
            await ViewModel.RefreshProxySecretStatusAsync();
        }
    }

    private async void OnSaveClick(
        object sender,
        RoutedEventArgs args)
    {
        await ViewModel.SaveAsync();
        if (!ViewModel.IsDirty)
        {
            var app = (App)Application.Current;
            app.SetKeepInTray(ViewModel.StayResident);
            app.SetFacturaeToolsEnabled(
                ViewModel.FacturaeToolsEnabled);
        }
    }

    private void OnCancelClick(
        object sender,
        RoutedEventArgs args) =>
        ViewModel.CancelCurrentOperation();

    private async void OnRefreshProxySecretStatusClick(
        object sender,
        RoutedEventArgs args) =>
        await ViewModel.RefreshProxySecretStatusAsync();

    private async void OnStoreProxyCredentialsClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        NativePasswordBuffer? captured = null;
        byte[]? password = null;
        try
        {
            captured = await _securePasswordPrompt.CaptureAsync(
                ProxyPasswordPrompt,
                cancellation.Token);
            if (captured is null)
            {
                return;
            }

            password = NativePasswordEncoding.ToUtf8(captured);
            await ViewModel.StoreProxyCredentialsAsync(password);
        }
        catch (OperationCanceledException)
            when (cancellation.IsCancellationRequested)
        {
            // La página se cerró o la aplicación terminó. No queda material
            // pendiente y no se abre un diálogo adicional.
        }
        catch (Exception exception)
        {
            ViewModel.ReportProxyCredentialCaptureFailure(
                exception,
                cancellation.Token);
        }
        finally
        {
            if (password is not null)
            {
                CryptographicOperations.ZeroMemory(password);
            }
            captured?.Dispose();
        }
    }

    private async void OnDeleteProxyCredentialsClick(
        object sender,
        RoutedEventArgs args)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var confirmation = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = "Quitar credenciales protegidas",
            Content =
                "El proxy manual quedará sin autenticación protegida. Esta acción no cambia el host ni el puerto configurados.",
            PrimaryButtonText = "Quitar credenciales",
            CloseButtonText = "Cancelar",
            DefaultButton = ContentDialogButton.Close,
        };
        if (await confirmation.ShowAsync() ==
            ContentDialogResult.Primary)
        {
            await ViewModel.DeleteProxyCredentialsAsync();
        }
    }

    private async void OnDiagnosticRequested(
        OperationDiagnostic diagnostic)
    {
        if (!_isLoaded || XamlRoot is null)
        {
            return;
        }

        var dialog = new OperationDiagnosticDialog(diagnostic)
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
        };
        await dialog.ShowAsync();
    }
}
