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
    private bool _restBusy;
    private bool _restHealthy;
    private bool _restPortBusy;

    public SettingsPage()
    {
        var app = (App)Application.Current;
        _securePasswordPrompt = app.SecurePasswordPromptService;
        ViewModel = new SettingsPageViewModel(
            app.OperationSession);
        InitializeComponent();
        ApplyParityLabels();
    }

    public SettingsPageViewModel ViewModel { get; }

    private void ApplyParityLabels()
    {
        string Label(string key) => SealUiCatalog.Text(
            System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName,
            key);
        TsaEnabledCheckBox.Content = Label("winui.parity.tsa.enabled");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(TsaEnabledCheckBox, (string)TsaEnabledCheckBox.Content);
        TsaUrlTextBox.Header = Label("winui.parity.tsa.url");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(TsaUrlTextBox, (string)TsaUrlTextBox.Header);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetHelpText(
            TsaUrlTextBox, Label("winui.parity.tsa.invalid"));
        RestSectionTitle.Text = Label("winui.parity.rest.title");
        RestTokenText.Header = Label("winui.parity.rest.token");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(RestTokenText, (string)RestTokenText.Header);
        SetRestButtonLabel(RestRefreshButton, Label("winui.parity.rest.refresh"));
        SetRestButtonLabel(RestStartButton, Label("winui.parity.rest.start"));
        SetRestButtonLabel(RestStopButton, Label("winui.parity.rest.stop"));
        SetRestButtonLabel(RestOpenButton, Label("winui.parity.rest.open"));
        RestStatusText.Text = Label("winui.parity.rest.unknown");
    }

    private static void SetRestButtonLabel(Button button, string label)
    {
        button.Content = label;
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(button, label);
    }

    private string ParityText(string key) => SealUiCatalog.Text(
        System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName, key);

    private void UpdateRestControls()
    {
        var server = ((App)Application.Current).RestServer;
        RestStartButton.IsEnabled = !_restBusy && !_restPortBusy;
        RestStopButton.IsEnabled = !_restBusy && server.IsOwnedRunning;
        RestOpenButton.IsEnabled = !_restBusy && _restHealthy;
        RestRefreshButton.IsEnabled = !_restBusy;
        RestTokenText.Text = server.Token ?? string.Empty;
        RestTokenText.Visibility = server.IsOwnedRunning ? Visibility.Visible : Visibility.Collapsed;
    }

    private async Task RefreshRestAsync()
    {
        if (_pageCancellation is null) return;
        _restBusy = true;
        RestStatusText.Text = ParityText("winui.parity.rest.checking");
        UpdateRestControls();
        try
        {
            var server = ((App)Application.Current).RestServer;
            _restPortBusy = await server.IsHealthyAsync(_pageCancellation.Token);
            _restHealthy = _restPortBusy && server.IsOwnedRunning;
            RestStatusText.Text = ParityText(_restHealthy
                ? "winui.parity.rest.running"
                : _restPortBusy ? "winui.parity.rest.other" : "winui.parity.rest.stopped");
        }
        catch (OperationCanceledException) { }
        finally { _restBusy = false; UpdateRestControls(); }
    }

    private async void OnRefreshRestClick(object sender, RoutedEventArgs args) =>
        await RefreshRestAsync();

    private async void OnStartRestClick(object sender, RoutedEventArgs args)
    {
        if (_pageCancellation is null || _restBusy) return;
        _restBusy = true;
        RestStatusText.Text = ParityText("winui.parity.rest.starting");
        UpdateRestControls();
        try
        {
            await ((App)Application.Current).RestServer.StartAsync(_pageCancellation.Token);
            _restHealthy = true;
            _restPortBusy = true;
            RestStatusText.Text = ParityText("winui.parity.rest.running");
        }
        catch (OperationCanceledException) { }
        catch (Exception) { RestStatusText.Text = ParityText("winui.parity.rest.start_error"); }
        finally { _restBusy = false; UpdateRestControls(); }
    }

    private async void OnStopRestClick(object sender, RoutedEventArgs args)
    {
        if (_restBusy) return;
        ((App)Application.Current).RestServer.Stop();
        await RefreshRestAsync();
    }

    private async void OnOpenRestClick(object sender, RoutedEventArgs args)
    {
        if (!_restHealthy) return;
        try
        {
            if (!await Windows.System.Launcher.LaunchUriAsync(
                new Uri(GrxFirma.WinUI.Core.Operations.RestServerLaunch.ConsoleUrl)))
                RestStatusText.Text = ParityText("winui.parity.rest.open_error");
        }
        catch (Exception)
        {
            RestStatusText.Text = ParityText("winui.parity.rest.open_error");
        }
    }

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
        await RefreshRestAsync();
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
            await Localizer.ShowAsync(dialog);
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
            app.ApplyLanguagePreference(ViewModel.SelectedLanguage.Value);
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
        if (await Localizer.ShowAsync(confirmation) ==
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
        await Localizer.ShowAsync(dialog);
    }
}
