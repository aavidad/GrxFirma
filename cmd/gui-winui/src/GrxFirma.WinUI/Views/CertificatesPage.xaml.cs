// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;
using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace GrxFirma.WinUI.Views;

public sealed partial class CertificatesPage : Page
{
    private static readonly SecurePasswordPromptRequest
        CredentialPasswordPrompt = new(
            "Contraseña de la credencial",
            "&Contraseña de P12/PFX/PEM (puede quedar vacía):",
            maximumCharacters: 1024);

    private readonly DesktopOperationSession _session;
    private readonly ISecurePasswordPromptService _securePasswordPrompt;
    private CancellationTokenSource? _pageCancellation;
    private bool _isSubscribed;

    public CertificatesPage()
    {
        var app = (App)Application.Current;
        _session = app.OperationSession;
        _securePasswordPrompt = app.SecurePasswordPromptService;
        ViewModel = new CertificatesPageViewModel(
            _session,
            app.FilePickerService);
        InitializeComponent();
        string Label(string key) => SealUiCatalog.Text(
            Localizer.Language, key);
        CertificateTypeFilterTitle.Text = Label("winui.parity.certs.type");
        FilterPersonalCheck.Content = Label("winui.parity.certs.person");
        FilterRepresentativeCheck.Content = Label("winui.parity.certs.representative");
        FilterSealCheck.Content = Label("winui.parity.certs.seal");
        FilterPublicEmployeeCheck.Content = Label("winui.parity.certs.employee");
        RequireNifFilter.Content = Label("winui.parity.certs.nif");
        RequireOrganizationFilter.Content = Label("winui.parity.certs.organization");
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            FilterPersonalCheck, (string)FilterPersonalCheck.Content);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            FilterRepresentativeCheck, (string)FilterRepresentativeCheck.Content);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            FilterSealCheck, (string)FilterSealCheck.Content);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            FilterPublicEmployeeCheck, (string)FilterPublicEmployeeCheck.Content);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            RequireNifFilter, (string)RequireNifFilter.Content);
        Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(
            RequireOrganizationFilter, (string)RequireOrganizationFilter.Content);
    }

    public CertificatesPageViewModel ViewModel { get; }

    private async void OnExportPublicCertificateClick(object sender, RoutedEventArgs args)
    {
        if (_pageCancellation is null || XamlRoot is null) return;
        try
        {
            await PublicCertificateExport.ExportAsync(_session,
                ((App)Application.Current).FilePickerService, XamlRoot,
                ViewModel.SelectedCertificate?.Id, _pageCancellation.Token);
        }
        catch (OperationCanceledException) { }
    }

    private void OnCertificateSelectionChanged(
        object sender,
        SelectionChangedEventArgs args) =>
        CertificateCardSelection.Update(
            (ListView)sender,
            args,
            (Brush)Resources["CertificateSelectedBorderBrush"]);

    private void OnCertificateContainerContentChanging(
        ListViewBase sender,
        ContainerContentChangingEventArgs args) =>
        CertificateCardSelection.UpdateContainer(
            (ListView)sender,
            args,
            (Brush)Resources["CertificateSelectedBorderBrush"]);

    private async void OnRenewFnmtClick(object sender, RoutedEventArgs args)
    {
        var result = await new WindowsHelpLauncherService()
            .OpenFnmtRenewalAsync(_pageCancellation?.Token ?? default);
        ViewModel.ReportRenewalLaunch(result.Succeeded, result.Message);
    }

    private async void OnOpenValideClick(object sender, RoutedEventArgs args)
    {
        var result = await new WindowsHelpLauncherService()
            .OpenValideAsync(_pageCancellation?.Token ?? default);
        ViewModel.ReportExternalValidationLaunch(result.Succeeded, result.Message);
    }

    private async void OnShowCertificateValidationClick(
        object sender,
        RoutedEventArgs args)
    {
        if (ViewModel.SelectedCertificate is { } certificate && XamlRoot is not null)
        {
            await CertificateValidationDialog.ShowAsync(
                certificate, XamlRoot, ActualTheme);
        }
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
        await RefreshAndShowDiagnosticAsync(_pageCancellation.Token);
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
                await RefreshAndShowDiagnosticAsync(cancellation.Token);
            }
        });
    }

    private async void OnRefreshClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        await RefreshAndShowDiagnosticAsync(cancellation.Token);
    }

    private async void OnUseSmartcardClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        if (!_session.TryGetOperations(
            DesktopOperationActions.SmartcardStatus,
            out var operations))
        {
            ShowSmartcardNotice("El motor local no ofrece la consulta de tarjetas.",
                InfoBarSeverity.Warning, false);
            return;
        }
        try
        {
            var result = await operations.GetSmartcardStatusAsync(
                cancellation.Token);
            if (!result.IsSuccess || result.Data is null ||
                !string.Equals(result.Outcome, "success", StringComparison.Ordinal))
            {
                ShowSmartcardNotice("No se pudo consultar el lector de tarjetas.",
                    InfoBarSeverity.Error, false);
                return;
            }
            ViewModel.FilterText = string.Empty;
            await RefreshAndShowDiagnosticAsync(cancellation.Token);
            if (cancellation.IsCancellationRequested)
            {
                return;
            }
            var first = ViewModel.VisibleCertificates.FirstOrDefault(
                certificate => certificate.NeedsUnlock);
            var guidance = SmartcardGuidance.Describe(result.Data, first is not null);
            if (result.Data.Readers.Any(reader => reader.Present) && first is not null)
            {
                ViewModel.SelectedCertificate = first;
            }
            ShowSmartcardNotice(guidance.Message, guidance.Severity,
                guidance.ShowOfficialLink);
        }
        catch (OperationCanceledException) when (cancellation.IsCancellationRequested)
        {
            // La página se cerró durante la consulta.
        }
        catch (Exception)
        {
            ShowSmartcardNotice("No se pudo consultar el lector de tarjetas.",
                InfoBarSeverity.Error, false);
        }
    }

    private async void OnOpenOfficialDNIeClick(
        object sender,
        RoutedEventArgs args)
    {
        try
        {
            var result = await new WindowsHelpLauncherService()
                .OpenOfficialDNIeAsync(_pageCancellation?.Token ?? default);
            if (!result.Succeeded)
            {
                ShowSmartcardNotice(result.Message, InfoBarSeverity.Warning, true);
            }
        }
        catch (OperationCanceledException)
        {
            // La página se cerró antes de abrir el navegador.
        }
    }

    private void ShowSmartcardNotice(
        string message,
        InfoBarSeverity severity,
        bool showOfficialLink)
    {
        if (!_isSubscribed)
        {
            return;
        }
        SmartcardInfoBar.Message = message;
        SmartcardInfoBar.Severity = severity;
        OfficialDNIeButton.Visibility = showOfficialLink
            ? Visibility.Visible : Visibility.Collapsed;
        SmartcardInfoBar.IsOpen = true;
    }

    private async void OnSelectImportCredentialClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        var diagnostic =
            await ViewModel.SelectImportCredentialAsync(
                cancellation.Token);
        await ShowDiagnosticIfPresentAsync(
            diagnostic,
            cancellation.Token);
    }

    private async void OnImportCredentialClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        byte[]? password = null;
        try
        {
            using var captured =
                await _securePasswordPrompt.CaptureAsync(
                    CredentialPasswordPrompt,
                    cancellation.Token);
            if (captured is null)
            {
                return;
            }

            password = NativePasswordEncoding.ToUtf8(captured);
            var diagnostic =
                await ViewModel.ImportSelectedCredentialAsync(
                    password,
                    cancellation.Token);
            await ShowDiagnosticIfPresentAsync(
                diagnostic,
                cancellation.Token);
        }
        catch (OperationCanceledException)
            when (cancellation.IsCancellationRequested)
        {
            // La página se cerró o el usuario canceló la operación.
        }
        catch (Exception exception)
        {
            var diagnostic =
                ViewModel.ReportImportCaptureFailure(exception);
            await ShowDiagnosticIfPresentAsync(
                diagnostic,
                cancellation.Token);
        }
        finally
        {
            if (password is not null)
            {
                CryptographicOperations.ZeroMemory(password);
            }
        }
    }

    private async void OnRemoveTemporaryCertificateClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        if (!await ConfirmTemporaryRemovalAsync(
            "Retirar credencial temporal",
            "La clave privada seleccionada dejará de estar disponible durante esta sesión.",
            "Retirar"))
        {
            return;
        }
        var diagnostic =
            await ViewModel.RemoveSelectedTemporaryCertificateAsync(
                cancellation.Token);
        await ShowDiagnosticIfPresentAsync(
            diagnostic,
            cancellation.Token);
    }

    private async void OnClearTemporaryCertificatesClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        if (!await ConfirmTemporaryRemovalAsync(
            "Retirar todas las credenciales temporales",
            "Todas las claves privadas cargadas solo para esta sesión dejarán de estar disponibles.",
            "Retirar todas"))
        {
            return;
        }
        var diagnostic =
            await ViewModel.ClearTemporaryCertificatesAsync(
                cancellation.Token);
        await ShowDiagnosticIfPresentAsync(
            diagnostic,
            cancellation.Token);
    }

    private async void OnValidateCertificateClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic =
            await ViewModel.ValidateSelectedCertificateAsync(
                cancellation.Token);
        if (diagnostic is not null &&
            !cancellation.IsCancellationRequested)
        {
            await ShowDiagnosticAsync(diagnostic);
        }
    }

    private async void OnSetDefaultCertificateClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic =
            await ViewModel.SetSelectedAsDefaultAsync(
                cancellation.Token);
        if (diagnostic is not null &&
            !cancellation.IsCancellationRequested)
        {
            await ShowDiagnosticAsync(diagnostic);
        }
    }

    private async void OnClearDefaultCertificateClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic =
            await ViewModel.ClearDefaultCertificateAsync(
                cancellation.Token);
        if (diagnostic is not null &&
            !cancellation.IsCancellationRequested)
        {
            await ShowDiagnosticAsync(diagnostic);
        }
    }

    private async void OnOpenCertificateManagerClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.OpenCertificateManagerAsync(
            cancellation.Token);
        if (diagnostic is not null &&
            !cancellation.IsCancellationRequested)
        {
            await ShowDiagnosticAsync(diagnostic);
        }
    }

    private async Task RefreshAndShowDiagnosticAsync(
        CancellationToken cancellationToken)
    {
        var diagnostic = await ViewModel.RefreshAsync(cancellationToken);
        if (diagnostic is not null &&
            !cancellationToken.IsCancellationRequested)
        {
            await ShowDiagnosticAsync(diagnostic);
        }
    }

    private async Task ShowDiagnosticAsync(OperationDiagnostic diagnostic)
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

    private async Task ShowDiagnosticIfPresentAsync(
        OperationDiagnostic? diagnostic,
        CancellationToken cancellationToken)
    {
        if (diagnostic is not null &&
            !cancellationToken.IsCancellationRequested)
        {
            await ShowDiagnosticAsync(diagnostic);
        }
    }

    private async Task<bool> ConfirmTemporaryRemovalAsync(
        string title,
        string message,
        string primaryButtonText)
    {
        if (!_isSubscribed || XamlRoot is null)
        {
            return false;
        }

        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = title,
            Content = message,
            PrimaryButtonText = primaryButtonText,
            CloseButtonText = "Cancelar",
            DefaultButton = ContentDialogButton.Close,
        };
        return await Localizer.ShowAsync(dialog) ==
            ContentDialogResult.Primary;
    }
}
