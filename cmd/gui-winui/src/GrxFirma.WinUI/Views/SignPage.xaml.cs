// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Diagnostics;
using System.ComponentModel;
using System.Globalization;
using System.Security.Cryptography;
using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Input;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation.Peers;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media.Imaging;
using Windows.Foundation;
using Windows.Storage.Streams;
using Windows.System;
using Windows.UI.Core;

namespace GrxFirma.WinUI.Views;

public sealed partial class SignPage : Page
{
    private enum VisibleSealPointerMode
    {
        None,
        Move,
        Resize,
        Rotate,
    }

    private static readonly SecurePasswordPromptRequest
        CredentialPasswordPrompt = new(
            "Contraseña de la credencial",
            "&Contraseña de P12/PFX/PEM (puede quedar vacía):",
            maximumCharacters: 1024);

    private readonly DesktopOperationSession _session;
    private readonly ISecurePasswordPromptService _securePasswordPrompt;
    private CancellationTokenSource? _pageCancellation;
    private bool _isSubscribed;
    private int _previewImageRevision;
    private int _stampImageRevision;
    private int _resultRevealRevision;
    private bool _certificatePanelExpanded;
    private int _certificatePanelPreferenceRevision;
    private readonly SemaphoreSlim _certificatePanelPreferenceSaveGate = new(1, 1);
    private int _logoOpacityPreferenceRevision;
    private CancellationTokenSource? _logoOpacitySaveDelay;
    private bool _logoOpacityPending;
    private bool _applyingLogoOpacityPreference;
    private VisibleSealPointerMode _visibleSealPointerMode;
    private uint _visibleSealPointerId;
    private Point _visibleSealPointerStart;
    private double _visibleSealStartXPercent;
    private double _visibleSealStartYPercent;
    private double _visibleSealStartWidthPercent;
    private double _visibleSealStartHeightPercent;

    public SignPage()
    {
        var app = (App)Application.Current;
        _session = app.OperationSession;
        _securePasswordPrompt = app.SecurePasswordPromptService;
        ViewModel = new SignPageViewModel(
            _session,
            app.FilePickerService,
            new WindowsPdfPreviewService());
        ViewModel.VisibleSealLogoOpacityLabel = SealUiCatalog.LogoOpacityLabel(
            CultureInfo.CurrentUICulture.TwoLetterISOLanguageName);
        ViewModel.VisibleSealOpacityHelp = SealUiCatalog.SealOpacityHelp(
            CultureInfo.CurrentUICulture.TwoLetterISOLanguageName);
        ViewModel.SetSealUiLanguage(CultureInfo.CurrentUICulture.TwoLetterISOLanguageName);
        CertificatePanel = new CertificatesPageViewModel(
            _session,
            app.FilePickerService);
        InitializeComponent();
        // Button consume eventos de puntero; recibirlos también al estar marcados
        // como atendidos mantiene fiable la captura del tirador.
        VisibleSealRotateHandle.AddHandler(
            UIElement.PointerPressedEvent,
            new PointerEventHandler(OnVisibleSealRotatePointerPressed), true);
        VisibleSealRotateHandle.AddHandler(
            UIElement.PointerMovedEvent,
            new PointerEventHandler(OnVisibleSealPreviewPointerMoved), true);
        VisibleSealRotateHandle.AddHandler(
            UIElement.PointerReleasedEvent,
            new PointerEventHandler(OnVisibleSealPreviewPointerReleased), true);
    }

    public SignPageViewModel ViewModel { get; }
    public CertificatesPageViewModel CertificatePanel { get; }

    private void OnPanelCertificateSelectionChanged(
        object sender,
        SelectionChangedEventArgs args) =>
        CertificateCardSelection.Update(
            (ListView)sender,
            args,
            (Brush)Resources["CertificateSelectedBorderBrush"]);

    private void OnPanelCertificateContainerContentChanging(
        ListViewBase sender,
        ContainerContentChangingEventArgs args) =>
        CertificateCardSelection.UpdateContainer(
            (ListView)sender,
            args,
            (Brush)Resources["CertificateSelectedBorderBrush"]);

    private async void OnLoaded(object sender, RoutedEventArgs args)
    {
        if (_isSubscribed)
        {
            return;
        }

        _isSubscribed = true;
        _pageCancellation = new CancellationTokenSource();
        _session.AvailabilityChanged += OnAvailabilityChanged;
        ViewModel.PropertyChanged += OnViewModelPropertyChanged;
        CertificatePanel.PropertyChanged += OnCertificatePanelPropertyChanged;
        ViewModel.UpdateAvailability();
        CertificatePanel.UpdateAvailability();
        UpdateCertificatePanelLayout();
        await UpdateVisibleSealPreviewImageAsync();
        await RefreshCertificatesAndShowDiagnosticAsync(
            _pageCancellation.Token);
        await LoadCertificatePanelPreferenceAsync();
    }

    private void OnUnloaded(object sender, RoutedEventArgs args)
    {
        ++_resultRevealRevision;
        if (_isSubscribed)
        {
            _session.AvailabilityChanged -= OnAvailabilityChanged;
            ViewModel.PropertyChanged -= OnViewModelPropertyChanged;
            CertificatePanel.PropertyChanged -= OnCertificatePanelPropertyChanged;
            _isSubscribed = false;
        }
        Interlocked.Increment(ref _previewImageRevision);
        Interlocked.Increment(ref _stampImageRevision);

        var cancellation = Interlocked.Exchange(
            ref _pageCancellation,
            null);
        if (cancellation is not null)
        {
            cancellation.Cancel();
            cancellation.Dispose();
        }
        _logoOpacitySaveDelay?.Cancel();
        _logoOpacitySaveDelay?.Dispose();
        _logoOpacitySaveDelay = null;
        if (_logoOpacityPending)
        {
            _ = SaveLogoOpacityPreferenceAsync();
        }
        ViewModel.CancelCurrentOperation();
        CertificatePanel.CancelCurrentOperation();
        ViewModel.DiscardPreparedCredential();
        ViewModel.DiscardVisibleSealPreview();
        VisibleSealPdfPreview.Source = null;
        VisibleSealStampPreview.Source = null;
        VisibleSealPreviewSurface.ReleasePointerCaptures();
        ResetVisibleSealPointerInteraction();
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
            CertificatePanel.UpdateAvailability();
            var cancellation = _pageCancellation;
            if (ViewModel.IsOperationConnected && cancellation is not null)
            {
                await RefreshCertificatesAndShowDiagnosticAsync(
                    cancellation.Token);
                await LoadCertificatePanelPreferenceAsync();
            }
        });
    }

    private async void OnSelectDocumentClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.SelectDocumentAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnRefreshCertificatesClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        await RefreshCertificatesAndShowDiagnosticAsync(
            cancellation.Token);
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
            CertificatePanel.FilterText = string.Empty;
            await RefreshCertificatesAndShowDiagnosticAsync(cancellation.Token);
            if (cancellation.IsCancellationRequested)
            {
                return;
            }
            var first = CertificatePanel.VisibleCertificates.FirstOrDefault(
                certificate => certificate.NeedsUnlock);
            var guidance = SmartcardGuidance.Describe(result.Data, first is not null);
            if (result.Data.Readers.Any(reader => reader.Present) && first is not null)
            {
                CertificatePanel.SelectedCertificate = first;
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

    private async void OnShowPanelCertificateValidationClick(
        object sender,
        RoutedEventArgs args)
    {
        if (CertificatePanel.SelectedCertificate is { } certificate &&
            XamlRoot is not null)
        {
            await CertificateValidationDialog.ShowAsync(
                certificate, XamlRoot, ActualTheme);
        }
    }

    private async void OnValidatePanelCertificateOnlineClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        var diagnostic = await CertificatePanel.ValidateSelectedCertificateAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnSetPanelDefaultCertificateClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        var diagnostic = await CertificatePanel.SetSelectedAsDefaultAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnClearPanelDefaultCertificateClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        var diagnostic = await CertificatePanel.ClearDefaultCertificateAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnOpenPanelValideClick(
        object sender,
        RoutedEventArgs args)
    {
        var result = await new WindowsHelpLauncherService()
            .OpenValideAsync(_pageCancellation?.Token ?? default);
        CertificatePanel.ReportExternalValidationLaunch(
            result.Succeeded, result.Message);
    }

    private async void OnRenewPanelFnmtClick(
        object sender,
        RoutedEventArgs args)
    {
        var result = await new WindowsHelpLauncherService()
            .OpenFnmtRenewalAsync(_pageCancellation?.Token ?? default);
        CertificatePanel.ReportRenewalLaunch(result.Succeeded, result.Message);
    }

    private async void OnUseTemporaryCredentialClick(
        object sender,
        RoutedEventArgs args) =>
        await SelectAndUseCredentialAsync(
            importIntoWindows: false);

    private async void OnImportCredentialToWindowsClick(
        object sender,
        RoutedEventArgs args)
    {
        if (!await ConfirmWindowsCredentialImportAsync())
        {
            return;
        }

        await SelectAndUseCredentialAsync(
            importIntoWindows: true);
    }

    private async Task SelectAndUseCredentialAsync(
        bool importIntoWindows)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        byte[]? password = null;
        try
        {
            var diagnostic =
                await ViewModel.SelectCredentialFileAsync(
                    cancellation.Token);
            await ShowDiagnosticIfPresentAsync(diagnostic);
            if (!ViewModel.HasPreparedCredential ||
                cancellation.IsCancellationRequested)
            {
                return;
            }

            using var captured =
                await _securePasswordPrompt.CaptureAsync(
                    CredentialPasswordPrompt,
                    cancellation.Token);
            if (captured is null)
            {
                ViewModel.DiscardPreparedCredential();
                return;
            }

            password = NativePasswordEncoding.ToUtf8(captured);
            diagnostic =
                await ViewModel.UsePreparedCredentialAsync(
                    password,
                    importIntoWindows,
                    cancellation.Token);
            await ShowDiagnosticIfPresentAsync(diagnostic);
            if (!cancellation.IsCancellationRequested)
            {
                await RefreshCertificatePanelAsync(cancellation.Token);
            }
        }
        catch (OperationCanceledException)
            when (cancellation.IsCancellationRequested)
        {
            // La página se cerró o el usuario canceló la operación.
        }
        catch (Exception exception)
        {
            var diagnostic =
                ViewModel.ReportCredentialCaptureFailure(exception);
            await ShowDiagnosticIfPresentAsync(diagnostic);
        }
        finally
        {
            if (password is not null)
            {
                CryptographicOperations.ZeroMemory(password);
            }
        }
    }

    private async Task<bool> ConfirmWindowsCredentialImportAsync()
    {
        if (!_isSubscribed || XamlRoot is null)
        {
            return false;
        }

        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = "Importar en el almacén de Windows",
            Content =
                "La credencial se instalará de forma persistente en el almacén personal del usuario actual de Windows (Cert:\\CurrentUser\\My). Podrá usarla esta aplicación y otros programas con acceso a ese almacén. Continúe solo si quiere conservarla allí.",
            PrimaryButtonText = "Importar en Windows",
            CloseButtonText = "Cancelar",
            DefaultButton = ContentDialogButton.Close,
        };
        return await dialog.ShowAsync() ==
            ContentDialogResult.Primary;
    }

    private async void OnSelectBatchFilesClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.SelectBatchFilesAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnSelectBatchFolderClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.SelectBatchFolderAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnSelectBatchOutputClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic =
            await ViewModel.SelectBatchOutputDirectoryAsync(
                cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private void OnClearBatchSelectionClick(
        object sender,
        RoutedEventArgs args)
    {
        ViewModel.ClearBatchSelection();
    }

    private void OnAdditionalSignersSelectionChanged(
        object sender,
        SelectionChangedEventArgs args)
    {
        ViewModel.SetSelectedAdditionalSigners(
            AdditionalSignersList.SelectedItems
                .OfType<CertificateListItem>());
    }

    private async void OnRefreshVisibleSealPreviewClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.RefreshVisibleSealPreviewAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async void OnPreviousSealPageClick(object sender, RoutedEventArgs args)
    {
        if (_pageCancellation is null) return;
        await ShowDiagnosticIfPresentAsync(await ViewModel.NavigateVisibleSealPageAsync(-1, _pageCancellation.Token));
    }

    private async void OnNextSealPageClick(object sender, RoutedEventArgs args)
    {
        if (_pageCancellation is null) return;
        await ShowDiagnosticIfPresentAsync(await ViewModel.NavigateVisibleSealPageAsync(1, _pageCancellation.Token));
    }

    private void OnApplySealToAllPagesClick(object sender, RoutedEventArgs args) => ViewModel.ApplySealToAllPages();

    private void OnToggleSealOnPageClick(object sender, RoutedEventArgs args) => ViewModel.ToggleSealOnCurrentPage();

    private async void OnSignClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        SignResultPanel.Visibility = Visibility.Collapsed;
        var revealRevision = ++_resultRevealRevision;
        var previousCompletion = ViewModel.CompletedSignPresentationId;
        var signAttemptStarted = false;
        void ObserveSignProgress(object? source, PropertyChangedEventArgs change)
        {
            if (change.PropertyName == nameof(SignPageViewModel.IsBusy) &&
                ViewModel.IsBusy)
            {
                signAttemptStarted = true;
            }
        }
        ViewModel.PropertyChanged += ObserveSignProgress;
        OperationDiagnostic? diagnostic;
        try
        {
            diagnostic = await ViewModel.SignAsync(cancellation.Token);
        }
        finally
        {
            ViewModel.PropertyChanged -= ObserveSignProgress;
        }
        var completion = ViewModel.CompletedSignPresentationId;
        if (diagnostic is null && completion != Guid.Empty && completion != previousCompletion)
        {
            _ = DispatcherQueue.TryEnqueue(Microsoft.UI.Dispatching.DispatcherQueuePriority.Low, () =>
            {
                if (!_isSubscribed || !ReferenceEquals(_pageCancellation, cancellation) ||
                    cancellation.IsCancellationRequested || revealRevision != _resultRevealRevision ||
                    ViewModel.IsBusy || completion != ViewModel.CompletedSignPresentationId ||
                    !ViewModel.CanOpenOutput || XamlRoot is null)
                {
                    return;
                }
                ShowSignResult(null, true);
                SignResultHeading.UpdateLayout();
                SignResultHeading.StartBringIntoView(new BringIntoViewOptions
                {
                    AnimationDesired = false,
                    VerticalAlignmentRatio = 0,
                });
                ResultButton.Focus(FocusState.Programmatic);
            });
        }
        await ShowDiagnosticIfPresentAsync(diagnostic);
        if (diagnostic is not null && _isSubscribed &&
            ReferenceEquals(_pageCancellation, cancellation) &&
            !cancellation.IsCancellationRequested && XamlRoot is not null)
        {
            ShowSignResult(diagnostic,
                signAttemptStarted && ViewModel.CanOpenOutput);
            SignResultHeading.StartBringIntoView(new BringIntoViewOptions
            {
                AnimationDesired = false,
                VerticalAlignmentRatio = 0,
            });
            (ResultButton.Visibility == Visibility.Visible
                ? ResultButton : ReviewSignButton)
                .Focus(FocusState.Programmatic);
        }
        else if (diagnostic is null &&
            (completion == Guid.Empty || completion == previousCompletion) &&
            _isSubscribed && ReferenceEquals(_pageCancellation, cancellation) &&
            !cancellation.IsCancellationRequested &&
            !ViewModel.ValidationMessage.Contains("No se eligió un destino", StringComparison.Ordinal) &&
            !ViewModel.ValidationMessage.Contains("canceló", StringComparison.OrdinalIgnoreCase))
        {
            ShowSignResult(null, false);
            ReviewSignButton.Focus(FocusState.Programmatic);
        }
    }

    private void ShowSignResult(
        OperationDiagnostic? diagnostic,
        bool outputFromThisAttempt)
    {
        var hasOutput = outputFromThisAttempt && ViewModel.CanOpenOutput &&
            !string.IsNullOrWhiteSpace(ViewModel.OutputPath);
        var unsafeOutput = hasOutput && string.Equals(
            diagnostic?.FailureCode,
            "PDF_CHANGED_DURING_SIGN", StringComparison.Ordinal);
        var verified = hasOutput && !unsafeOutput &&
            ViewModel.ValidateAfterSigning &&
            ViewModel.ValidationMessage.Contains(
                "integridad y confianza", StringComparison.OrdinalIgnoreCase);
        var warning = hasOutput && !unsafeOutput &&
            ViewModel.ValidateAfterSigning && !verified;
        SignResultNotice.Severity = hasOutput && !unsafeOutput
            ? InfoBarSeverity.Success : InfoBarSeverity.Error;
        SignResultNotice.Title = unsafeOutput
            ? "Resultado guardado: no utilice el documento"
            : hasOutput ? "Documento firmado correctamente"
            : "No se pudo firmar el documento";
        var cause = diagnostic?.UserMessage;
        SignResultNotice.Message = unsafeOutput
            ? $"{ViewModel.ResultMessage} {ViewModel.ValidationMessage}"
            : hasOutput
            ? ViewModel.ResultMessage
            : string.IsNullOrWhiteSpace(cause)
                ? ViewModel.ValidationMessage
                : cause;
        SignResultFileName.Text = hasOutput
            ? Path.GetFileName(ViewModel.OutputPath) : string.Empty;
        SignResultVerification.Text = unsafeOutput
            ? string.IsNullOrWhiteSpace(diagnostic?.SuggestedAction)
                ? "Cierre el programa que modifica el PDF, actualice la previsualización y repita la firma."
                : diagnostic.SuggestedAction
            : verified
            ? "Firma verificada"
            : hasOutput && !ViewModel.ValidateAfterSigning
                ? "No se solicitó comprobar la firma."
            : warning ? "La firma no se pudo verificar por completo. Revise el detalle antes de usarla."
            : string.IsNullOrWhiteSpace(diagnostic?.SuggestedAction)
                ? "Revise los datos y vuelva a intentarlo."
                : diagnostic.SuggestedAction;
        SignResultVerification.Foreground = (Brush)Application.Current.Resources[
            unsafeOutput ? "AppDiagnosticFailureBrush"
                : warning ? "AppDiagnosticUnknownBrush"
                : hasOutput ? "AppDiagnosticSuccessBrush"
                : "AppDiagnosticFailureBrush"];
        ResultButton.Visibility = hasOutput ? Visibility.Visible : Visibility.Collapsed;
        OpenResultFolderButton.Visibility = hasOutput ? Visibility.Visible : Visibility.Collapsed;
        ReviewSignButton.Visibility = hasOutput ? Visibility.Collapsed : Visibility.Visible;
        SignResultPanel.Visibility = Visibility.Visible;
        var peer = FrameworkElementAutomationPeer.FromElement(SignResultNotice) ??
            FrameworkElementAutomationPeer.CreatePeerForElement(SignResultNotice);
        peer?.RaiseAutomationEvent(AutomationEvents.LiveRegionChanged);
    }

    private void OnReviewSignClick(object sender, RoutedEventArgs args)
    {
        DocumentBrowseButton.StartBringIntoView();
        DocumentBrowseButton.Focus(FocusState.Programmatic);
    }

    private async void OnSignBatchClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }

        var diagnostic = await ViewModel.SignBatchAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private void OnCancelClick(object sender, RoutedEventArgs args)
    {
        ViewModel.CancelCurrentOperation();
    }

    private async void OnOpenResultClick(
        object sender,
        RoutedEventArgs args)
    {
        var validationDiagnostic = ViewModel.ValidateOutputForOpening();
        if (validationDiagnostic is not null)
        {
            await ShowDiagnosticAsync(validationDiagnostic);
            return;
        }

        var outputPath = ViewModel.OutputPath;
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

    private async void OnOpenResultFolderClick(object sender, RoutedEventArgs args)
    {
        var validationDiagnostic = ViewModel.ValidateOutputForOpening();
        if (validationDiagnostic is not null)
        {
            await ShowDiagnosticAsync(validationDiagnostic);
            return;
        }
        if (string.IsNullOrWhiteSpace(ViewModel.OutputPath))
        {
            return;
        }
        var directory = Path.GetDirectoryName(ViewModel.OutputPath);
        if (string.IsNullOrWhiteSpace(directory) || !Directory.Exists(directory))
        {
            SignResultNotice.Severity = InfoBarSeverity.Error;
            SignResultNotice.Message = "La carpeta del documento firmado ya no está disponible. Compruebe el destino de guardado.";
            return;
        }
        try
        {
            Process.Start(new ProcessStartInfo
            {
                FileName = directory,
                UseShellExecute = true,
                Verb = "open",
            });
        }
        catch (Exception exception)
        {
            await ShowDiagnosticAsync(OperationDiagnosticMapper.FromException(exception));
        }
    }

    private async void OnOpenBatchResultsClick(
        object sender,
        RoutedEventArgs args)
    {
        var validationDiagnostic =
            ViewModel.ValidateBatchOutputForOpening();
        if (validationDiagnostic is not null)
        {
            await ShowDiagnosticAsync(validationDiagnostic);
            return;
        }

        var outputDirectory = ViewModel.BatchOutputDirectory;
        if (string.IsNullOrWhiteSpace(outputDirectory))
        {
            return;
        }

        try
        {
            Process.Start(new ProcessStartInfo
            {
                FileName = outputDirectory,
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

    private async Task RefreshCertificatesAndShowDiagnosticAsync(
        CancellationToken cancellationToken)
    {
        var diagnostic = await ViewModel.RefreshCertificatesAsync(
            cancellationToken);
        if (!cancellationToken.IsCancellationRequested)
        {
            await ShowDiagnosticIfPresentAsync(diagnostic);
            await RefreshCertificatePanelAsync(cancellationToken);
        }
    }

    private async Task RefreshCertificatePanelAsync(
        CancellationToken cancellationToken)
    {
        var diagnostic = await CertificatePanel.RefreshAsync(cancellationToken);
        if (!cancellationToken.IsCancellationRequested)
        {
            await ShowDiagnosticIfPresentAsync(diagnostic);
            if (CertificatePanel.SelectedCertificate?.IsDefaultCertificate != true)
            {
                SelectPanelCertificateById(
                    ViewModel.SelectedCertificate?.Id ??
                    ViewModel.Certificates.FirstOrDefault()?.Id);
            }
        }
    }

    private void SelectPanelCertificateById(string? certificateId)
    {
        if (string.IsNullOrWhiteSpace(certificateId))
        {
            return;
        }
        var matching = CertificatePanel.VisibleCertificates.FirstOrDefault(item =>
            string.Equals(item.Id, certificateId, StringComparison.Ordinal));
        if (matching is not null &&
            !string.Equals(CertificatePanel.SelectedCertificate?.Id,
                certificateId, StringComparison.Ordinal))
        {
            CertificatePanel.SelectedCertificate = matching;
        }
    }

    private async void OnChangeCertificateClick(object sender, RoutedEventArgs args)
    {
        _certificatePanelExpanded = true;
        ++_certificatePanelPreferenceRevision;
        UpdateCertificatePanelLayout();
        await SaveCertificatePanelPreferenceAsync();
        _ = DispatcherQueue.TryEnqueue(
            Microsoft.UI.Dispatching.DispatcherQueuePriority.Low, () =>
            {
                if (!_isSubscribed || CertificateSidebar.Visibility != Visibility.Visible)
                {
                    return;
                }
                PanelCertificateList.StartBringIntoView();
                PanelCertificateList.Focus(FocusState.Programmatic);
            });
    }

    private async void OnHideCertificatePanelClick(object sender, RoutedEventArgs args)
    {
        _certificatePanelExpanded = false;
        ++_certificatePanelPreferenceRevision;
        UpdateCertificatePanelLayout();
        await SaveCertificatePanelPreferenceAsync();
    }

    private async Task LoadCertificatePanelPreferenceAsync()
    {
        if (!_session.TryGetOperations(DesktopOperationActions.GetSettings,
                out var operations)) return;
        try
        {
            var result = await operations.GetSettingsAsync();
            if (!_isSubscribed || !result.IsSuccess || result.Data is null) return;
            ViewModel.VisibleSealLogoOpacityLabel = SealUiCatalog.LogoOpacityLabel(
                result.Data.Language);
            ViewModel.VisibleSealOpacityHelp = SealUiCatalog.SealOpacityHelp(
                result.Data.Language);
            ViewModel.SetSealUiLanguage(result.Data.Language);
            if (_certificatePanelPreferenceRevision == 0)
            {
                _certificatePanelExpanded = result.Data.SignCertificatePanelExpanded == true;
                UpdateCertificatePanelLayout();
            }
            if (_logoOpacityPreferenceRevision == 0 &&
                result.Data.SealLogoOpacityPercent is >= 0 and <= 100)
            {
                _applyingLogoOpacityPreference = true;
                try
                {
                    ViewModel.VisibleSealLogoOpacityPercent =
                        result.Data.SealLogoOpacityPercent.Value;
                }
                finally
                {
                    _applyingLogoOpacityPreference = false;
                }
            }
        }
        catch (Exception) { /* La preferencia no impide firmar. */ }
    }

    private async Task SaveCertificatePanelPreferenceAsync()
    {
        await _certificatePanelPreferenceSaveGate.WaitAsync();
        try
        {
            if (!_session.TryGetOperations(DesktopOperationActions.GetSettings,
                    out var operations) ||
                !_session.Supports(DesktopOperationActions.SaveSettings)) return;
            var result = await operations.GetSettingsAsync();
            if (!result.IsSuccess || result.Data is null) return;
            await operations.SaveSettingsAsync(result.Data with
            {
                SignCertificatePanelExpanded = _certificatePanelExpanded,
            });
        }
        catch (Exception) { /* Se mantiene la preferencia en esta página. */ }
        finally { _certificatePanelPreferenceSaveGate.Release(); }
    }

    private void ScheduleLogoOpacityPreferenceSave()
    {
        _logoOpacitySaveDelay?.Cancel();
        _logoOpacitySaveDelay?.Dispose();
        var delay = new CancellationTokenSource();
        _logoOpacitySaveDelay = delay;
        _logoOpacityPending = true;
        _ = SaveLogoOpacityAfterDelayAsync(delay.Token);
    }

    private async Task SaveLogoOpacityAfterDelayAsync(
        CancellationToken cancellationToken)
    {
        try
        {
            await Task.Delay(350, cancellationToken);
            await SaveLogoOpacityPreferenceAsync();
        }
        catch (OperationCanceledException)
        {
        }
    }

    private async Task SaveLogoOpacityPreferenceAsync()
    {
        await _certificatePanelPreferenceSaveGate.WaitAsync();
        try
        {
            if (!_logoOpacityPending ||
                !_session.TryGetOperations(DesktopOperationActions.GetSettings,
                    out var operations) ||
                !_session.Supports(DesktopOperationActions.SaveSettings)) return;
            var result = await operations.GetSettingsAsync();
            if (!result.IsSuccess || result.Data is null) return;
            var revision = _logoOpacityPreferenceRevision;
            var value = (int)ViewModel.VisibleSealLogoOpacityPercent;
            var saved = await operations.SaveSettingsAsync(result.Data with
            {
                SealLogoOpacityPercent = value,
            });
            if (saved.IsSuccess && revision == _logoOpacityPreferenceRevision)
            {
                _logoOpacityPending = false;
            }
        }
        catch (Exception) { /* La preferencia no impide firmar. */ }
        finally { _certificatePanelPreferenceSaveGate.Release(); }
    }

    private void OnSignLayoutSizeChanged(object sender, SizeChangedEventArgs args) =>
        UpdateCertificatePanelLayout();

    private void UpdateCertificatePanelLayout()
    {
        CentralCertificatePickerPanel.Visibility = Visibility.Visible;
        CertificateExpandedPanel.Visibility = Visibility.Visible;
        CertificateSidebar.Visibility = !_certificatePanelExpanded
            ? Visibility.Collapsed : Visibility.Visible;
        var sidebarWidth = _certificatePanelExpanded && XamlRoot is not null &&
            XamlRoot.Size.Width >= 760 ? 330 : 0;
        SignLayoutGrid.ColumnSpacing = sidebarWidth > 0 ? 20 : 0;
        if (CertificatePanelColumn.Width.Value != sidebarWidth)
        {
            CertificatePanelColumn.Width = new GridLength(sidebarWidth);
        }
    }

    private void OnCertificatePanelPropertyChanged(
        object? sender,
        PropertyChangedEventArgs args)
    {
        if (args.PropertyName != nameof(CertificatesPageViewModel.SelectedCertificate))
        {
            return;
        }
        if (CertificatePanel.SelectedCertificate is not { } selected)
        {
            ViewModel.SelectedCertificate = null;
            UpdateCertificatePanelLayout();
            return;
        }
        var signingItem = ViewModel.Certificates.FirstOrDefault(item =>
            string.Equals(item.Id, selected.Id, StringComparison.Ordinal));
        if (signingItem is not null &&
            !string.Equals(ViewModel.SelectedCertificate?.Id,
                signingItem.Id, StringComparison.Ordinal))
        {
            ViewModel.SelectedCertificate = signingItem;
        }
        UpdateCertificatePanelLayout();
    }

    private void OnViewModelPropertyChanged(
        object? sender,
        PropertyChangedEventArgs args)
    {
        if (args.PropertyName == nameof(
            SignPageViewModel.VisibleSealLogoOpacityPercent) &&
            !_applyingLogoOpacityPreference)
        {
            ++_logoOpacityPreferenceRevision;
            ScheduleLogoOpacityPreferenceSave();
        }
        if (args.PropertyName == nameof(SignPageViewModel.SelectedCertificate))
        {
            SelectPanelCertificateById(ViewModel.SelectedCertificate?.Id);
            UpdateCertificatePanelLayout();
            return;
        }
        if (args.PropertyName == nameof(SignPageViewModel.ResultMessage) &&
            ViewModel.ResultMessage.StartsWith(
                "No se ha ejecutado ninguna firma", StringComparison.Ordinal))
        {
            SignResultPanel.Visibility = Visibility.Collapsed;
            return;
        }
        if (string.Equals(
            args.PropertyName,
            nameof(SignPageViewModel.VisibleSealStampImage),
            StringComparison.Ordinal))
        {
            _ = DispatcherQueue.TryEnqueue(
                async () => await UpdateVisibleSealStampImageAsync());
            return;
        }
        if (args.PropertyName is nameof(SignPageViewModel.VisibleSealEnabled)
            or nameof(SignPageViewModel.InputDisplayName)
            or nameof(SignPageViewModel.VisibleSealPages)
            or nameof(SignPageViewModel.CanRefreshVisibleSealPreview))
        {
            ScheduleAutomaticVisibleSealPreview();
            return;
        }
        if (!string.Equals(
            args.PropertyName,
            nameof(SignPageViewModel.VisibleSealPreviewImage),
            StringComparison.Ordinal))
        {
            return;
        }

        _ = DispatcherQueue.TryEnqueue(
            async () => await UpdateVisibleSealPreviewImageAsync());
    }

    // El sello no contiene datos sensibles más allá del nombre del
    // certificado, pero se decodifica igual que la página: la última
    // petición gana y un fallo solo deja la muestra orientativa.
    private async Task UpdateVisibleSealStampImageAsync()
    {
        var revision = Interlocked.Increment(ref _stampImageRevision);
        var stamp = ViewModel.VisibleSealStampImage;
        if (stamp.IsEmpty)
        {
            VisibleSealStampPreview.Source = null;
            return;
        }
        try
        {
            using var stream = new InMemoryRandomAccessStream();
            using (var writer = new DataWriter(stream.GetOutputStreamAt(0)))
            {
                writer.WriteBytes(stamp.ToArray());
                await writer.StoreAsync();
                await writer.FlushAsync();
                writer.DetachStream();
            }
            stream.Seek(0);
            var bitmap = new BitmapImage();
            await bitmap.SetSourceAsync(stream);
            if (_isSubscribed &&
                revision == Volatile.Read(ref _stampImageRevision))
            {
                VisibleSealStampPreview.Source = bitmap;
            }
        }
        catch
        {
            if (revision == Volatile.Read(ref _stampImageRevision))
            {
                VisibleSealStampPreview.Source = null;
            }
        }
    }

    private bool _automaticPreviewQueued;

    // Carga la página del PDF en cuanto hay documento y sello activado, como
    // hace la aplicación Linux; el botón sigue disponible para recargarla.
    private void ScheduleAutomaticVisibleSealPreview()
    {
        if (_automaticPreviewQueued || !ViewModel.NeedsAutomaticVisibleSealPreview)
        {
            return;
        }
        _automaticPreviewQueued = true;
        _ = DispatcherQueue.TryEnqueue(async () =>
        {
            _automaticPreviewQueued = false;
            var cancellation = _pageCancellation;
            if (!_isSubscribed || cancellation is null ||
                !ViewModel.NeedsAutomaticVisibleSealPreview)
            {
                return;
            }
            var diagnostic = await ViewModel.RefreshVisibleSealPreviewAsync(
                cancellation.Token);
            await ShowDiagnosticIfPresentAsync(diagnostic);
        });
    }

    private async void OnPickVisibleSealImageClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        var diagnostic = await ViewModel.PickVisibleSealImageAsync(
            cancellation.Token);
        await ShowDiagnosticIfPresentAsync(diagnostic);
    }

    private async Task UpdateVisibleSealPreviewImageAsync()
    {
        var revision = Interlocked.Increment(ref _previewImageRevision);
        var preview = ViewModel.VisibleSealPreviewImage;
        if (preview.IsEmpty)
        {
            VisibleSealPdfPreview.Source = null;
            return;
        }

        try
        {
            var previewBytes = preview.ToArray();
            using var stream = new InMemoryRandomAccessStream();
            try
            {
                using (var writer =
                    new DataWriter(stream.GetOutputStreamAt(0)))
                {
                    writer.WriteBytes(previewBytes);
                    await writer.StoreAsync();
                    await writer.FlushAsync();
                    writer.DetachStream();
                }
                stream.Seek(0);
                var bitmap = new BitmapImage();
                await bitmap.SetSourceAsync(stream);
                if (!_isSubscribed ||
                    revision != Volatile.Read(ref _previewImageRevision))
                {
                    return;
                }
                VisibleSealPdfPreview.Source = bitmap;
                ViewModel.ConfirmPreviewImageRendered();
            }
            finally
            {
                CryptographicOperations.ZeroMemory(previewBytes);
            }
        }
        catch
        {
            if (!_isSubscribed ||
                revision != Volatile.Read(ref _previewImageRevision))
            {
                return;
            }
            VisibleSealPdfPreview.Source = null;
            await ShowDiagnosticAsync(
                ViewModel.PreviewImageRenderingFailed());
        }
    }

    private void OnVisibleSealMovePointerPressed(
        object sender,
        PointerRoutedEventArgs args) =>
        BeginVisibleSealPointerInteraction(
            VisibleSealPointerMode.Move,
            args);

    private void OnVisibleSealResizePointerPressed(
        object sender,
        PointerRoutedEventArgs args) =>
        BeginVisibleSealPointerInteraction(
            VisibleSealPointerMode.Resize,
            args);

    private void OnVisibleSealRotatePointerPressed(
        object sender,
        PointerRoutedEventArgs args) =>
        BeginVisibleSealPointerInteraction(
            VisibleSealPointerMode.Rotate,
            args);

    private void BeginVisibleSealPointerInteraction(
        VisibleSealPointerMode mode,
        PointerRoutedEventArgs args)
    {
        if (_visibleSealPointerMode is not VisibleSealPointerMode.None ||
            VisibleSealPdfPreview.Source is null ||
            !HasValidVisibleSealPreviewGeometry())
        {
            return;
        }

        var point = args.GetCurrentPoint(VisibleSealPreviewSurface);
        if (args.Pointer.PointerDeviceType is PointerDeviceType.Mouse &&
            !point.Properties.IsLeftButtonPressed)
        {
            return;
        }
        var captureTarget = mode is VisibleSealPointerMode.Rotate
            ? (UIElement)VisibleSealRotateHandle
            : VisibleSealPreviewSurface;
        if (!captureTarget.CapturePointer(args.Pointer))
        {
            return;
        }

        _visibleSealPointerMode = mode;
        _visibleSealPointerId = args.Pointer.PointerId;
        _visibleSealPointerStart = point.Position;
        _visibleSealStartXPercent = ViewModel.VisibleSealXPercent;
        _visibleSealStartYPercent = ViewModel.VisibleSealYPercent;
        _visibleSealStartWidthPercent =
            ViewModel.VisibleSealWidthPercent;
        _visibleSealStartHeightPercent =
            ViewModel.VisibleSealHeightPercent;
        if (mode is VisibleSealPointerMode.Rotate)
        {
            ViewModel.BeginVisibleSealRotation();
            VisibleSealRotateHandle.Focus(FocusState.Pointer);
        }
        args.Handled = true;
    }

    private void OnVisibleSealPreviewPointerMoved(
        object sender,
        PointerRoutedEventArgs args)
    {
        if (_visibleSealPointerMode is VisibleSealPointerMode.None ||
            args.Pointer.PointerId != _visibleSealPointerId)
        {
            return;
        }

        var point = args.GetCurrentPoint(VisibleSealPreviewSurface);
        var canvasWidth = ViewModel.PreviewCanvasWidth;
        var canvasHeight = ViewModel.PreviewCanvasHeight;
        if (!double.IsFinite(canvasWidth) ||
            !double.IsFinite(canvasHeight) ||
            canvasWidth <= 0 ||
            canvasHeight <= 0)
        {
            EndVisibleSealPointerInteraction(args);
            return;
        }

        var deltaXPercent =
            ((point.Position.X - _visibleSealPointerStart.X) /
                canvasWidth) * 100;
        var deltaYPercent =
            ((point.Position.Y - _visibleSealPointerStart.Y) /
                canvasHeight) * 100;

        if (_visibleSealPointerMode is VisibleSealPointerMode.Rotate)
        {
            var centerX = ViewModel.VisibleSealPreviewX +
                ViewModel.VisibleSealPreviewWidth / 2;
            var centerY = ViewModel.VisibleSealPreviewTop +
                ViewModel.VisibleSealPreviewHeight / 2;
            var baseAngle = Math.Atan2(
                -ViewModel.VisibleSealPreviewHeight / 2,
                ViewModel.VisibleSealPreviewWidth / 2);
            var degrees = (Math.Atan2(
                point.Position.Y - centerY,
                point.Position.X - centerX) - baseAngle) * 180 / Math.PI;
            degrees = (degrees % 360 + 360) % 360;
            if (IsShiftPressed())
            {
                degrees = Math.Round(degrees / 15) * 15;
            }
            else
            {
                var nearest = Math.Round(degrees / 90) * 90;
                if (Math.Abs(degrees - nearest) <= 4) degrees = nearest;
            }
            ViewModel.VisibleSealRotationDegrees = Math.Round(degrees) % 360;
        }
        else if (_visibleSealPointerMode is VisibleSealPointerMode.Move)
        {
            ViewModel.VisibleSealXPercent = RoundPreviewPercent(
                Math.Clamp(
                    _visibleSealStartXPercent + deltaXPercent,
                    0,
                    100 - _visibleSealStartWidthPercent));
            ViewModel.VisibleSealYPercent = RoundPreviewPercent(
                Math.Clamp(
                    _visibleSealStartYPercent - deltaYPercent,
                    0,
                    100 - _visibleSealStartHeightPercent));
        }
        else
        {
            const double minimumSizePercent = 2;
            // El tirador gira con la tarjeta: proyectar el movimiento del
            // puntero sobre sus ejes originales antes de cambiar ancho/alto.
            var radians = ViewModel.VisibleSealPreviewRotation * Math.PI / 180;
            var dx = point.Position.X - _visibleSealPointerStart.X;
            var dy = point.Position.Y - _visibleSealPointerStart.Y;
            var localXPercent = (dx * Math.Cos(radians) + dy * Math.Sin(radians)) /
                canvasWidth * 100;
            var localYPercent = (-dx * Math.Sin(radians) + dy * Math.Cos(radians)) /
                canvasHeight * 100;
            var newWidth = Math.Clamp(
                _visibleSealStartWidthPercent + localXPercent,
                minimumSizePercent,
                100 - _visibleSealStartXPercent);
            var newHeight = Math.Clamp(
                _visibleSealStartHeightPercent + localYPercent,
                minimumSizePercent,
                _visibleSealStartHeightPercent +
                    _visibleSealStartYPercent);
            ViewModel.VisibleSealWidthPercent =
                RoundPreviewPercent(newWidth);
            ViewModel.VisibleSealHeightPercent =
                RoundPreviewPercent(newHeight);
            ViewModel.VisibleSealYPercent = RoundPreviewPercent(
                _visibleSealStartYPercent -
                    (newHeight - _visibleSealStartHeightPercent));
        }
        args.Handled = true;
    }

    private void OnVisibleSealPreviewPointerReleased(
        object sender,
        PointerRoutedEventArgs args)
    {
        if (args.Pointer.PointerId != _visibleSealPointerId)
        {
            return;
        }
        EndVisibleSealPointerInteraction(args);
        args.Handled = true;
    }

    private void OnVisibleSealPreviewPointerCanceled(
        object sender,
        PointerRoutedEventArgs args)
    {
        if (args.Pointer.PointerId != _visibleSealPointerId)
        {
            return;
        }
        EndVisibleSealPointerInteraction(args);
        args.Handled = true;
    }

    private void OnVisibleSealPreviewPointerCaptureLost(
        object sender,
        PointerRoutedEventArgs args) =>
        ResetVisibleSealPointerInteraction();

    private void EndVisibleSealPointerInteraction(
        PointerRoutedEventArgs args)
    {
        var captureTarget = _visibleSealPointerMode is VisibleSealPointerMode.Rotate
            ? (UIElement)VisibleSealRotateHandle
            : VisibleSealPreviewSurface;
        captureTarget.ReleasePointerCapture(args.Pointer);
        ResetVisibleSealPointerInteraction();
    }

    private void OnVisibleSealRotateKeyDown(object sender, KeyRoutedEventArgs args)
    {
        var direction = args.Key switch
        {
            VirtualKey.Left or VirtualKey.Down => -1,
            VirtualKey.Right or VirtualKey.Up => 1,
            _ => 0,
        };
        if (direction == 0) return;
        ViewModel.VisibleSealRotationDegrees += direction * (IsShiftPressed() ? 15 : 1);
        args.Handled = true;
    }

    private static bool IsShiftPressed() =>
        (InputKeyboardSource.GetKeyStateForCurrentThread(VirtualKey.Shift) &
            CoreVirtualKeyStates.Down) != 0;

    private bool HasValidVisibleSealPreviewGeometry()
    {
        var values = new[]
        {
            ViewModel.VisibleSealXPercent,
            ViewModel.VisibleSealYPercent,
            ViewModel.VisibleSealWidthPercent,
            ViewModel.VisibleSealHeightPercent,
        };
        return values.All(double.IsFinite) &&
            ViewModel.VisibleSealWidthPercent > 0 &&
            ViewModel.VisibleSealHeightPercent > 0 &&
            ViewModel.VisibleSealXPercent >= 0 &&
            ViewModel.VisibleSealYPercent >= 0 &&
            ViewModel.VisibleSealXPercent +
                ViewModel.VisibleSealWidthPercent <= 100 &&
            ViewModel.VisibleSealYPercent +
                ViewModel.VisibleSealHeightPercent <= 100;
    }

    private void ResetVisibleSealPointerInteraction()
    {
        if (_visibleSealPointerMode is VisibleSealPointerMode.Rotate)
        {
            ViewModel.EndVisibleSealRotation();
        }
        _visibleSealPointerMode = VisibleSealPointerMode.None;
        _visibleSealPointerId = 0;
    }

    private static double RoundPreviewPercent(double value) =>
        Math.Round(value, 2, MidpointRounding.AwayFromZero);

    private Task ShowDiagnosticIfPresentAsync(
        OperationDiagnostic? diagnostic) =>
        diagnostic is null
            ? Task.CompletedTask
            : ShowDiagnosticAsync(diagnostic);

    private async Task ShowDiagnosticAsync(OperationDiagnostic diagnostic)
    {
        ++_resultRevealRevision;
        if (!_isSubscribed || XamlRoot is null)
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
