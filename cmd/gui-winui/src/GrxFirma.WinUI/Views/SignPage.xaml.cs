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
        Draw,
    }

    private static SecurePasswordPromptRequest
        CredentialPasswordPrompt => new(
            Localizer.Text("winui.comun.contrasena_de_la_credencial"),
            Localizer.Text("winui.comun.contrasena_de_p12_pfx_pem_puede_quedar"),
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
    private bool _drawingSealArea;
    private Point _sealDrawFirst;
    private Point _sealDrawSecond;
    private Microsoft.UI.Dispatching.DispatcherQueueTimer? _sealDrawPositionTimer;
    private bool _portalLayoutConfigured;
    private PortalSealSession? _portalSealSession;
    private Button? _portalSignButton;
    private TextBlock? _portalPageText;
    private readonly Dictionary<string, string> _fieldErrors = new(StringComparer.Ordinal);

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
            Localizer.Language);
        ViewModel.VisibleSealOpacityHelp = SealUiCatalog.SealOpacityHelp(
            Localizer.Language);
        ViewModel.SetSealUiLanguage(Localizer.Language);
        CertificatePanel = new CertificatesPageViewModel(
            _session,
            app.FilePickerService);
        ViewModel.RemoteSecretsPrompt = PromptRemoteSecretsAsync;
        ViewModel.SignConfirmationPrompt = ConfirmSignAsync;
        InitializeComponent();
        // La superficie recibe el inicio del dibujo desde el control transparente.
        VisibleSealPreviewSurface.AddHandler(UIElement.PointerPressedEvent,
            new PointerEventHandler(OnVisibleSealDrawPointerPressed), true);
        VisibleSealPreviewSurface.SizeChanged += (_, _) => CancelVisibleSealDrawing();
        RegisterFieldValidation(VisibleSealQrUrlTextBox, "qr");
        RegisterFieldValidation(VisibleSealCsvCodeTextBox, "csvCode");
        RegisterFieldValidation(VisibleSealCsvUrlTextBox, "csvUrl");
        RegisterFieldValidation(VisibleSealCsvTextTextBox, "csvText");
        RegisterFieldValidation(VisibleSealPagesTextBox, "pages");
        VisibleSealImageButton.LostFocus += (_, _) => RefreshFieldError("image");
        ViewModel.PropertyChanged += (_, change) =>
        {
            if (_fieldErrors.Count == 0) return;
            if (change.PropertyName is nameof(SignPageViewModel.VisibleSealEnabled) or
                nameof(SignPageViewModel.VisibleSealQrEnabled) or
                nameof(SignPageViewModel.VisibleSealCsvEnabled) or
                nameof(SignPageViewModel.IsVisibleSealCustomPages))
                foreach (var field in _fieldErrors.Keys.ToArray()) RefreshFieldError(field);
        };
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

    private void RegisterFieldValidation(TextBox field, string name)
    {
        field.LostFocus += (_, _) => RefreshFieldError(name);
        field.TextChanged += (_, _) =>
        {
            if (_fieldErrors.ContainsKey(name)) RefreshFieldError(name);
        };
    }

    private void RefreshFieldError(string name)
    {
        var issues = ViewModel.ValidateVisibleSealFields();
        if (issues.TryGetValue(name, out var key)) _fieldErrors[name] = key;
        else _fieldErrors.Remove(name);
        RenderFieldErrors();
    }

    private string? ValidateAllFields()
    {
        _fieldErrors.Clear();
        foreach (var issue in ViewModel.ValidateVisibleSealFields())
            _fieldErrors[issue.Key] = issue.Value;
        RenderFieldErrors();
        foreach (var name in new[] { "image", "qr", "csvCode", "csvUrl", "csvText", "pages" })
            if (_fieldErrors.ContainsKey(name)) return name;
        return null;
    }

    private void RenderFieldErrors()
    {
        void Show(string name, Control field, TextBlock message)
        {
            var invalid = _fieldErrors.TryGetValue(name, out var key);
            var detail = invalid ? Localizer.Text(key!) : string.Empty;
            message.Text = detail;
            message.Visibility = invalid ? Visibility.Visible : Visibility.Collapsed;
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetHelpText(field, detail);
            if (invalid) field.BorderBrush = (Brush)Application.Current.Resources["AppDiagnosticFailureBrush"];
            else field.ClearValue(Control.BorderBrushProperty);
        }
        Show("image", VisibleSealImageButton, VisibleSealImageError);
        Show("qr", VisibleSealQrUrlTextBox, VisibleSealQrError);
        Show("csvCode", VisibleSealCsvCodeTextBox, VisibleSealCsvCodeError);
        Show("csvUrl", VisibleSealCsvUrlTextBox, VisibleSealCsvUrlError);
        Show("csvText", VisibleSealCsvTextTextBox, VisibleSealCsvTextError);
        Show("pages", VisibleSealPagesTextBox, VisibleSealPagesError);
        AdvancedSignErrorCount.Visibility = _fieldErrors.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
        AdvancedSignErrorCount.Text = Localizer.Text("validacion.problemas")
            .Replace("%1", _fieldErrors.Count.ToString(CultureInfo.CurrentCulture), StringComparison.Ordinal);
    }

    private void FocusField(string name)
    {
        AdvancedSignExpander.IsExpanded = true;
        Control target = name switch
        {
            "image" => VisibleSealImageButton,
            "qr" => VisibleSealQrUrlTextBox,
            "csvCode" => VisibleSealCsvCodeTextBox,
            "csvUrl" => VisibleSealCsvUrlTextBox,
            "csvText" => VisibleSealCsvTextTextBox,
            _ => VisibleSealPagesTextBox,
        };
        target.StartBringIntoView();
        target.Focus(FocusState.Programmatic);
    }

    // Un elemento XAML solo puede tener un padre: se suelta del actual, sea del
    // tipo que sea, antes de colocarlo en la vista del portal.
    private static void DetachFromParent(FrameworkElement element)
    {
        switch (element.Parent ?? VisualTreeHelper.GetParent(element))
        {
            case Panel panel:
                panel.Children.Remove(element);
                break;
            case Border border:
                border.Child = null;
                break;
            case Viewbox viewbox:
                viewbox.Child = null;
                break;
            case ContentControl control:
                control.Content = null;
                break;
            case ContentPresenter presenter:
                presenter.Content = null;
                break;
        }
    }

    internal void ConfigurePortalSeal(PortalSealSession session)
    {
        _portalSealSession = session;
        ViewModel.ConfigurePortalSealDocument(session.DocumentPath, session.SignerName);
        if (IsLoaded) ConfigurePortalSealLayout(session);
    }

    private void ConfigurePortalSealLayout(PortalSealSession session)
    {
        if (_portalLayoutConfigured) return;
        _portalLayoutConfigured = true;
        var language = Localizer.Language;
        string Label(string key) => SealUiCatalog.Text(language, key);
        // La vista y sus controles se reubican solo después de Loaded.
        VisibleSealEditorPanel.Children.Remove(VisibleSealPreviewViewbox);
        DetachFromParent(VisibleSealPreviewViewbox);
        VisibleSealEditorPanel.Children.Remove(VisibleSealDrawControls);
        DetachFromParent(VisibleSealDrawControls);

        var pageControls = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 8 };
        var previous = new Button { Content = Label("portal.seal.previous_page"), MinHeight = 40 };
        previous.Click += async (_, _) => await NavigatePortalPageAsync(-1);
        pageControls.Children.Add(previous);
        _portalPageText = new TextBlock { VerticalAlignment = VerticalAlignment.Center };
        pageControls.Children.Add(_portalPageText);
        var next = new Button { Content = Label("portal.seal.next_page"), MinHeight = 40 };
        next.Click += async (_, _) => await NavigatePortalPageAsync(1);
        pageControls.Children.Add(next);

        var actions = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 8 };
        _portalSignButton = new Button
        {
            Content = Label("portal.seal.sign_here"), MinHeight = 40,
            IsEnabled = false,
        };
        _portalSignButton.Click += (_, _) =>
        {
            var placement = ViewModel.PortalSealPlacement();
            if (placement is null) return;
            if (session.Submit("place", [placement]))
                ((App)Application.Current).CompletePortalSeal();
        };
        actions.Children.Add(_portalSignButton);
        var without = new Button { Content = Label("portal.seal.sign_without"), MinHeight = 40 };
        without.Click += (_, _) =>
        {
            if (session.Submit("without"))
                ((App)Application.Current).CompletePortalSeal();
        };
        actions.Children.Add(without);
        var cancel = new Button { Content = Label("portal.seal.cancel"), MinHeight = 40 };
        cancel.Click += (_, _) =>
        {
            if (session.Submit("cancel"))
                ((App)Application.Current).CompletePortalSeal();
        };
        actions.Children.Add(cancel);

        var content = new StackPanel { Spacing = 16, Padding = new Thickness(20) };
        content.Children.Add(new TextBlock
        {
            Text = Label("portal.seal.title"), FontSize = 22,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
        });
        content.Children.Add(new TextBlock
        {
            Text = Label("portal.seal.instructions"), TextWrapping = TextWrapping.Wrap,
            MaxWidth = 900, HorizontalAlignment = HorizontalAlignment.Left,
        });
        content.Children.Add(actions);
        content.Children.Add(pageControls);
        var geometry = new Grid { ColumnSpacing = 8 };
        for (var index = 0; index < 5; index++)
            geometry.ColumnDefinitions.Add(new ColumnDefinition
            {
                Width = new GridLength(1, GridUnitType.Star),
            });
        void AddGeometryField(int column, string key, string property,
            double minimum, double maximum)
        {
            var box = new NumberBox
            {
                Header = Label(key), Minimum = minimum, Maximum = maximum,
                SmallChange = 1, MinWidth = 100,
            };
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(box, Label(key));
            box.SetBinding(NumberBox.ValueProperty,
                new Microsoft.UI.Xaml.Data.Binding
                {
                    Source = ViewModel,
                    Path = new PropertyPath(property),
                    Mode = Microsoft.UI.Xaml.Data.BindingMode.TwoWay,
                });
            Grid.SetColumn(box, column);
            geometry.Children.Add(box);
        }
        AddGeometryField(0, "portal.seal.x", nameof(SignPageViewModel.VisibleSealXPercent), 0, 99);
        AddGeometryField(1, "portal.seal.y", nameof(SignPageViewModel.VisibleSealYPercent), 0, 99);
        AddGeometryField(2, "portal.seal.width", nameof(SignPageViewModel.VisibleSealWidthPercent), 1, 100);
        AddGeometryField(3, "portal.seal.height", nameof(SignPageViewModel.VisibleSealHeightPercent), 1, 100);
        AddGeometryField(4, "portal.seal.rotation", nameof(SignPageViewModel.VisibleSealRotationDegrees), 0, 359);
        var geometryPanel = new ScrollViewer
        {
            Content = geometry,
            HorizontalScrollMode = ScrollMode.Auto,
            HorizontalScrollBarVisibility = ScrollBarVisibility.Auto,
            VerticalScrollMode = ScrollMode.Disabled,
            VerticalScrollBarVisibility = ScrollBarVisibility.Disabled,
        };
        // La página entera debe verse sin desplazarse: se ajusta a la altura
        // disponible bajo el título, la instrucción y los botones.
        void FitPreview()
        {
            var available = SignContentScrollViewer.ActualHeight - 260 - VisibleSealDrawControls.ActualHeight;
            VisibleSealPreviewViewbox.MaxHeight = Math.Clamp(available, 280, 900);
            VisibleSealPreviewViewbox.MaxWidth = 900;
        }
        SignContentScrollViewer.SizeChanged += (_, _) => FitPreview();
        VisibleSealDrawControls.SizeChanged += (_, _) => FitPreview();
        FitPreview();
        content.Children.Add(VisibleSealDrawControls);
        content.Children.Add(VisibleSealPreviewViewbox);
        content.Children.Add(new TextBlock
        {
            Text = Label("portal.seal.fine_tune"),
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
        });
        content.Children.Add(geometryPanel);
        SignContentScrollViewer.Content = content;
        void UpdateNavigation()
        {
            _portalPageText!.Text = Label("portal.seal.page")
                .Replace("%1", Math.Max(1, ViewModel.PortalCurrentPage).ToString(CultureInfo.CurrentCulture))
                .Replace("%2", Math.Max(1, ViewModel.PortalTotalPages).ToString(CultureInfo.CurrentCulture));
            previous.IsEnabled = ViewModel.CanGoToPreviousSealPage;
            next.IsEnabled = ViewModel.CanGoToNextSealPage;
            _portalSignButton!.IsEnabled = ViewModel.PortalSealPlacement() is not null;
        }
        _portalUpdateNavigation = UpdateNavigation;
        UpdateNavigation();
    }

    private Action? _portalUpdateNavigation;
    private int _portalRefreshInProgress;

    private async Task RefreshPortalSealAsync()
    {
        if (_portalSealSession is null ||
            Interlocked.Exchange(ref _portalRefreshInProgress, 1) != 0) return;
        try
        {
            // La primera vista puede coincidir con el renderizado inicial de la
            // página; solo se recurre al diálogo de reserva si tras varios
            // intentos no hay una página del PDF sobre la que situar el sello.
            var token = _pageCancellation?.Token ?? CancellationToken.None;
            for (var intento = 0; intento < 5; intento++)
            {
                await ViewModel.RefreshVisibleSealPreviewAsync(token);
                await UpdateVisibleSealPreviewImageAsync();
                _portalUpdateNavigation?.Invoke();
                if (token.IsCancellationRequested || ViewModel.PortalSealPlacement() is not null)
                    return;
                await Task.Delay(400, token);
            }
            ((App)Application.Current).FallbackPortalSeal();
        }
        catch (OperationCanceledException)
        {
        }
        finally { Interlocked.Exchange(ref _portalRefreshInProgress, 0); }
    }

    private async Task NavigatePortalPageAsync(int step)
    {
        CancelVisibleSealDrawing();
        await ViewModel.NavigateVisibleSealPageAsync(step,
            _pageCancellation?.Token ?? CancellationToken.None);
        await UpdateVisibleSealPreviewImageAsync();
        _portalUpdateNavigation?.Invoke();
        if (_pageCancellation?.IsCancellationRequested != true &&
            ViewModel.PortalSealPlacement() is null)
            ((App)Application.Current).FallbackPortalSeal();
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
        if (_portalSealSession is not null) ConfigurePortalSealLayout(_portalSealSession);
        _pageCancellation = new CancellationTokenSource();
        _session.AvailabilityChanged += OnAvailabilityChanged;
        ViewModel.PropertyChanged += OnViewModelPropertyChanged;
        CertificatePanel.PropertyChanged += OnCertificatePanelPropertyChanged;
        ViewModel.UpdateAvailability();
        CertificatePanel.UpdateAvailability();
        UpdateCertificatePanelLayout();
        await UpdateVisibleSealPreviewImageAsync();
        if (((App)Application.Current).PortalSealActive)
        {
            if (_portalSealSession is not null)
                await RefreshPortalSealAsync();
            return;
        }
        await RefreshCertificatesAndShowDiagnosticAsync(
            _pageCancellation.Token);
        await LoadCertificatePanelPreferenceAsync();
        await UpdateRemoteSigningButtonAsync();
    }

    // Firma remota CSC: el botón aparece si el motor la permite o si la
    // política la prohíbe (el cuadro lo explica); no, si solo está desactivada.
    private async Task UpdateRemoteSigningButtonAsync()
    {
        var cancellation = _pageCancellation;
        if (cancellation is null)
        {
            return;
        }
        try
        {
            var allowed = await RemoteSigningDialogs.ShouldShowButtonAsync(
                _session,
                cancellation.Token);
            RemoteSigningButton.Visibility = allowed
                ? Visibility.Visible
                : Visibility.Collapsed;
        }
        catch (OperationCanceledException)
        {
        }
    }

    private async void OnRemoteSigningClick(
        object sender,
        RoutedEventArgs args)
    {
        var cancellation = _pageCancellation;
        if (cancellation is null || XamlRoot is null)
        {
            return;
        }
        var dialogOpen = true;
        var changed = false;
        try
        {
            await RemoteSigningDialogs.ShowConfigurationAsync(
                XamlRoot,
                ActualTheme,
                _session,
                () =>
                {
                    if (dialogOpen)
                    {
                        changed = true;
                    }
                    else
                    {
                        _ = RefreshAfterRemoteSigningChangeAsync(cancellation.Token);
                    }
                },
                cancellation.Token);
            dialogOpen = false;
            if (changed)
            {
                await RefreshAfterRemoteSigningChangeAsync(cancellation.Token);
            }
            await UpdateRemoteSigningButtonAsync();
        }
        catch (OperationCanceledException)
        {
        }
        finally
        {
            dialogOpen = false;
        }
    }

    private async Task RefreshAfterRemoteSigningChangeAsync(
        CancellationToken cancellationToken)
    {
        try
        {
            await RefreshCertificatesAndShowDiagnosticAsync(cancellationToken);
        }
        catch (OperationCanceledException)
        {
        }
    }

    private async Task<RemoteSigningSecrets?> PromptRemoteSecretsAsync(
        CertificateInfo certificate,
        CancellationToken cancellationToken)
    {
        if (XamlRoot is null)
        {
            return null;
        }
        return await RemoteSigningDialogs.PromptSecretsAsync(
            XamlRoot,
            ActualTheme,
            _session,
            certificate,
            cancellationToken);
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
            ShowSmartcardNotice(Localizer.Text("winui.comun.el_motor_local_no_ofrece_la_consulta_de"),
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
                ShowSmartcardNotice(Localizer.Text("winui.comun.no_se_pudo_consultar_el_lector_de"),
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
            ShowSmartcardNotice(Localizer.Text("winui.comun.no_se_pudo_consultar_el_lector_de"),
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

    // «Confirmar antes de firmar»: resumen de lo que se va a firmar y con qué
    // certificado. El botón por defecto es «Cancelar» para que Intro no firme
    // sin querer.
    private async Task<bool> ConfirmSignAsync(
        SignConfirmationSummary summary,
        CancellationToken cancellationToken)
    {
        if (!_isSubscribed || XamlRoot is null || cancellationToken.IsCancellationRequested)
        {
            return false;
        }

        var rows = new Grid { ColumnSpacing = 12, RowSpacing = 8 };
        rows.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        rows.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        void AddRow(string labelKey, string value)
        {
            var row = rows.RowDefinitions.Count;
            rows.RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
            var label = new TextBlock
            {
                Text = Localizer.Text(labelKey),
                Style = (Style)Application.Current.Resources["BodyStrongTextBlockStyle"],
                TextWrapping = TextWrapping.Wrap,
            };
            var text = new TextBlock
            {
                Text = value,
                TextWrapping = TextWrapping.Wrap,
                IsTextSelectionEnabled = true,
            };
            Microsoft.UI.Xaml.Automation.AutomationProperties.SetName(text, label.Text + ": " + value);
            Grid.SetRow(label, row);
            Grid.SetRow(text, row);
            Grid.SetColumn(text, 1);
            rows.Children.Add(label);
            rows.Children.Add(text);
        }
        AddRow(summary.Batch ? "winui.firmar.confirmar.documentos" : "winui.firmar.confirmar.documento", summary.Documents);
        AddRow("winui.firmar.confirmar.certificado", summary.Certificate);
        AddRow("winui.firmar.confirmar.operacion", summary.Operation);
        AddRow("winui.firmar.confirmar.formato", summary.Format);
        if (summary.VisibleSeal)
        {
            AddRow("winui.firmar.confirmar.sello", Localizer.Text("winui.firmar.confirmar.sello_si"));
        }
        var content = new StackPanel { Spacing = 16 };
        content.Children.Add(rows);
        content.Children.Add(new TextBlock
        {
            Text = Localizer.Text("winui.firmar.confirmar.ayuda"),
            TextWrapping = TextWrapping.Wrap,
            Foreground = (Brush)Application.Current.Resources["AppMutedTextBrush"],
        });

        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            RequestedTheme = ActualTheme,
            Title = Localizer.Text(summary.Batch
                ? "winui.firmar.confirmar.titulo_lote"
                : "winui.firmar.confirmar.titulo"),
            Content = new ScrollViewer { MaxHeight = 420, Content = content },
            PrimaryButtonText = Localizer.Text("winui.firmar.confirmar.firmar"),
            CloseButtonText = Localizer.Text("winui.comun.cancelar"),
            DefaultButton = ContentDialogButton.Close,
        };
        return await Localizer.ShowAsync(dialog) == ContentDialogResult.Primary &&
            !cancellationToken.IsCancellationRequested;
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
            Title = Localizer.Text("winui.firmar.importar_en_el_almacen_de_windows"),
            Content =
                Localizer.Text("winui.firmar.la_credencial_se_instalara_de_forma"),
            PrimaryButtonText = Localizer.Text("winui.firmar.importar_en_windows"),
            CloseButtonText = Localizer.Text("winui.comun.cancelar"),
            DefaultButton = ContentDialogButton.Close,
        };
        return await Localizer.ShowAsync(dialog) ==
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
        CancelVisibleSealDrawing();
        if (_pageCancellation is null) return;
        await ShowDiagnosticIfPresentAsync(await ViewModel.NavigateVisibleSealPageAsync(-1, _pageCancellation.Token));
    }

    private async void OnNextSealPageClick(object sender, RoutedEventArgs args)
    {
        CancelVisibleSealDrawing();
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

        var firstFieldError = ValidateAllFields();
        if (firstFieldError is not null)
        {
            FocusField(firstFieldError);
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
            !IsCancelledOrDiscardedMessage(ViewModel.ValidationMessage))
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
            ? Localizer.Text("winui.firmar.resultado_guardado_no_utilice_el")
            : hasOutput ? Localizer.Text("winui.firmar.documento_firmado_correctamente")
            : Localizer.Text("winui.firmar.no_se_pudo_firmar_el_documento");
        var cause = diagnostic?.UserMessage;
        SignResultNotice.Message = unsafeOutput
            ? $"{Localizer.VisibleText(ViewModel.ResultMessage)} {Localizer.VisibleText(ViewModel.ValidationMessage)}"
            : hasOutput
            ? ViewModel.ResultMessage
            : string.IsNullOrWhiteSpace(cause)
                ? ViewModel.ValidationMessage
                : cause;
        SignResultFileName.Text = hasOutput
            ? Path.GetFileName(ViewModel.OutputPath) : string.Empty;
        SignResultVerification.Text = unsafeOutput
            ? string.IsNullOrWhiteSpace(diagnostic?.SuggestedAction)
                ? Localizer.Text("winui.firmar.cierre_el_programa_que_modifica_el_pdf")
                : diagnostic.SuggestedAction
            : verified
            ? Localizer.Text("winui.firmar.firma_verificada")
            : hasOutput && !ViewModel.ValidateAfterSigning
                ? Localizer.Text("winui.firmar.no_se_solicito_comprobar_la_firma")
            : warning ? Localizer.Text("winui.firmar.la_firma_no_se_pudo_verificar_por")
            : string.IsNullOrWhiteSpace(diagnostic?.SuggestedAction)
                ? Localizer.Text("winui.firmar.revise_los_datos_y_vuelva_a_intentarlo")
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

        var firstFieldError = ValidateAllFields();
        if (firstFieldError is not null)
        {
            FocusField(firstFieldError);
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
            SignResultNotice.Message = Localizer.Text("winui.firmar.la_carpeta_del_documento_firmado_ya_no");
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
            ViewModel.SetSealLanguagePreference(result.Data.SealLanguage);
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
        // Sin vista previa lista no se puede dibujar: el botón queda desactivado.
        VisibleSealDrawToggle.IsEnabled = ViewModel.CanDrawVisibleSealArea || VisibleSealDrawToggle.IsChecked == true;
        if (args.PropertyName is nameof(SignPageViewModel.VisibleSealPreviewImage)
            or nameof(SignPageViewModel.InputDisplayName)
            or nameof(SignPageViewModel.VisibleSealEnabled))
            CancelVisibleSealDrawing();
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
            (string.Equals(ViewModel.ResultMessage,
                Localizer.Text("winui.firmar.no_se_ha_ejecutado_ninguna_firma"), StringComparison.Ordinal) ||
             string.Equals(ViewModel.ResultMessage,
                Localizer.Text("winui.firmar.no_se_ha_ejecutado_ninguna_firma_con"), StringComparison.Ordinal)))
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
        if (_fieldErrors.ContainsKey("image")) RefreshFieldError("image");
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

    private void OnVisibleSealDrawModeChanged(object sender, RoutedEventArgs args)
    {
        CancelVisibleSealDrawing();
        var visibility = VisibleSealDrawToggle.IsChecked == true ? Visibility.Visible : Visibility.Collapsed;
        VisibleSealDrawInput.Visibility = visibility;
        VisibleSealDrawHelp.Visibility = visibility;
        if (visibility == Visibility.Visible) VisibleSealDrawInput.Focus(FocusState.Keyboard);
    }

    private bool CanDrawVisibleSealArea => VisibleSealDrawToggle.IsChecked == true &&
        VisibleSealPdfPreview.Source is not null && ViewModel.VisibleSealEnabled &&
        ViewModel.CanDrawVisibleSealArea &&
        ViewModel.PreviewCanvasWidth > 0 && ViewModel.PreviewCanvasHeight > 0;

    private void OnVisibleSealDrawPointerPressed(object sender, PointerRoutedEventArgs args)
    {
        if (!CanDrawVisibleSealArea || _visibleSealPointerMode != VisibleSealPointerMode.None) return;
        var point = args.GetCurrentPoint(VisibleSealPreviewSurface);
        if (args.Pointer.PointerDeviceType == PointerDeviceType.Mouse && !point.Properties.IsLeftButtonPressed) return;
        if (!VisibleSealPreviewSurface.CapturePointer(args.Pointer)) return;
        _visibleSealPointerMode = VisibleSealPointerMode.Draw;
        _visibleSealPointerId = args.Pointer.PointerId;
        _sealDrawFirst = NormalizeSealDrawPoint(point.Position);
        _sealDrawSecond = _sealDrawFirst;
        _drawingSealArea = true;
        VisibleSealDrawInput.Focus(FocusState.Pointer);
        UpdateSealDrawRubberBand();
        args.Handled = true;
    }

    private Point NormalizeSealDrawPoint(Point point) => new(
        Math.Clamp(point.X / ViewModel.PreviewCanvasWidth, 0, 1),
        Math.Clamp(point.Y / ViewModel.PreviewCanvasHeight, 0, 1));

    private void UpdateSealDrawRubberBand()
    {
        var rect = SealDrawGeometry.Normalize(_sealDrawFirst.X, _sealDrawFirst.Y, _sealDrawSecond.X, _sealDrawSecond.Y);
        Canvas.SetLeft(VisibleSealDrawRubberBand, rect.X * ViewModel.PreviewCanvasWidth);
        Canvas.SetTop(VisibleSealDrawRubberBand, (1 - rect.Y - rect.Height) * ViewModel.PreviewCanvasHeight);
        VisibleSealDrawRubberBand.Width = rect.Width * ViewModel.PreviewCanvasWidth;
        VisibleSealDrawRubberBand.Height = rect.Height * ViewModel.PreviewCanvasHeight;
        VisibleSealDrawRubberBand.Visibility = Visibility.Visible;
    }

    private void AnnounceSealDrawing(string key) =>
        AnnounceSealDrawingText(SealUiCatalog.Text(Localizer.Language, key));

    private void AnnounceSealDrawingText(string text)
    {
        VisibleSealDrawNotice.Text = text;
        var peer = FrameworkElementAutomationPeer.FromElement(VisibleSealDrawNotice) ??
            FrameworkElementAutomationPeer.CreatePeerForElement(VisibleSealDrawNotice);
        peer?.RaiseAutomationEvent(AutomationEvents.LiveRegionChanged);
    }

    private void CancelVisibleSealDrawing()
    {
        var wasDrawing = _drawingSealArea;
        _drawingSealArea = false;
        VisibleSealDrawRubberBand.Visibility = Visibility.Collapsed;
        if (_visibleSealPointerMode == VisibleSealPointerMode.Draw)
        {
            ResetVisibleSealPointerInteraction();
            VisibleSealPreviewSurface.ReleasePointerCaptures();
        }
        if (wasDrawing) AnnounceSealDrawing("sign.seal.draw_cancelled");
    }

    private bool ConfirmVisibleSealDrawing()
    {
        if (!_drawingSealArea || !CanDrawVisibleSealArea) return false;
        var rect = SealDrawGeometry.Normalize(_sealDrawFirst.X, _sealDrawFirst.Y, _sealDrawSecond.X, _sealDrawSecond.Y);
        _drawingSealArea = false;
        VisibleSealDrawRubberBand.Visibility = Visibility.Collapsed;
        var applied = ViewModel.ApplyDrawnSealArea(rect);
        AnnounceSealDrawing(applied ? "sign.seal.draw_applied" : "sign.seal.draw_too_small");
        return applied;
    }

    // Intro y Esc terminan el modo dibujo y devuelven el foco al botón.
    private void ExitVisibleSealDrawMode()
    {
        _sealDrawPositionTimer?.Stop();
        VisibleSealDrawToggle.IsChecked = false;
        VisibleSealDrawToggle.Focus(FocusState.Keyboard);
    }

    // Anuncia la posición con un retardo corto para no saturar el lector de pantalla.
    private void ScheduleSealDrawPositionAnnouncement()
    {
        if (_sealDrawPositionTimer is null)
        {
            _sealDrawPositionTimer = DispatcherQueue.CreateTimer();
            _sealDrawPositionTimer.Interval = TimeSpan.FromMilliseconds(350);
            _sealDrawPositionTimer.IsRepeating = false;
            _sealDrawPositionTimer.Tick += (_, _) =>
            {
                if (!_drawingSealArea) return;
                var area = SealDrawGeometry.Normalize(_sealDrawFirst.X, _sealDrawFirst.Y, _sealDrawSecond.X, _sealDrawSecond.Y);
                static string Percent(double value) => Math.Round(value * 100).ToString(CultureInfo.CurrentCulture);
                AnnounceSealDrawingText(SealUiCatalog.Text(Localizer.Language, "sign.seal.draw_position")
                    .Replace("%1", Percent(area.X))
                    .Replace("%2", Percent(1 - area.Y - area.Height))
                    .Replace("%3", Percent(area.Width))
                    .Replace("%4", Percent(area.Height)));
            };
        }
        _sealDrawPositionTimer.Stop();
        _sealDrawPositionTimer.Start();
    }

    private void OnVisibleSealDrawKeyDown(object sender, KeyRoutedEventArgs args)
    {
        if (!CanDrawVisibleSealArea) return;
        if (args.Key == VirtualKey.Escape)
        {
            CancelVisibleSealDrawing();
            ExitVisibleSealDrawMode();
            args.Handled = true;
            return;
        }
        var dx = args.Key == VirtualKey.Left ? -0.01 : args.Key == VirtualKey.Right ? 0.01 : 0;
        var dy = args.Key == VirtualKey.Up ? -0.01 : args.Key == VirtualKey.Down ? 0.01 : 0;
        if (dx == 0 && dy == 0 && args.Key != VirtualKey.Enter) return;
        if (!_drawingSealArea)
        {
            _sealDrawFirst = new(ViewModel.VisibleSealXPercent / 100,
                1 - (ViewModel.VisibleSealYPercent + ViewModel.VisibleSealHeightPercent) / 100);
            _sealDrawSecond = new(_sealDrawFirst.X + ViewModel.VisibleSealWidthPercent / 100,
                _sealDrawFirst.Y + ViewModel.VisibleSealHeightPercent / 100);
            _drawingSealArea = true;
        }
        if (args.Key == VirtualKey.Enter)
        {
            var applied = ConfirmVisibleSealDrawing();
            ResetVisibleSealPointerInteraction();
            VisibleSealPreviewSurface.ReleasePointerCaptures();
            if (applied) ExitVisibleSealDrawMode();
        }
        else
        {
            if (IsShiftPressed()) _sealDrawSecond = new(Math.Clamp(_sealDrawSecond.X + dx, 0, 1), Math.Clamp(_sealDrawSecond.Y + dy, 0, 1));
            else _sealDrawFirst = new(Math.Clamp(_sealDrawFirst.X + dx, 0, 1), Math.Clamp(_sealDrawFirst.Y + dy, 0, 1));
            UpdateSealDrawRubberBand();
            ScheduleSealDrawPositionAnnouncement();
        }
        args.Handled = true;
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
        if (VisibleSealDrawToggle.IsChecked == true ||
            _visibleSealPointerMode is not VisibleSealPointerMode.None ||
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
        if (_visibleSealPointerMode == VisibleSealPointerMode.Draw)
        {
            _sealDrawSecond = NormalizeSealDrawPoint(point.Position);
            UpdateSealDrawRubberBand();
            args.Handled = true;
            return;
        }
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
        if (_visibleSealPointerMode == VisibleSealPointerMode.Draw)
        {
            _sealDrawSecond = NormalizeSealDrawPoint(args.GetCurrentPoint(VisibleSealPreviewSurface).Position);
            ConfirmVisibleSealDrawing();
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
        CancelVisibleSealDrawing();
        EndVisibleSealPointerInteraction(args);
        args.Handled = true;
    }

    private void OnVisibleSealPreviewPointerCaptureLost(
        object sender,
        PointerRoutedEventArgs args) =>
        HandleVisibleSealCaptureLost();

    private void HandleVisibleSealCaptureLost()
    {
        CancelVisibleSealDrawing();
        ResetVisibleSealPointerInteraction();
    }

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

    // Mensajes de validación que indican que el usuario canceló o no eligió
    // destino: en esos casos no se muestra el panel de resultado. Se comparan
    // con el texto ya traducido del catálogo, no con frases escritas aquí.
    private static readonly string[] CancelledOrDiscardedMessageKeys =
    [
        "winui.firmar.no_se_eligio_un_destino_no_se_ha",
        "winui.firmar.la_consulta_de_certificados_se_cancelo",
        "winui.firmar.la_seleccion_de_la_credencial_se_cancelo",
        "winui.firmar.la_carga_de_la_credencial_se_cancelo",
        "winui.firmar.la_seleccion_del_documento_se_cancelo_no",
        "winui.firmar.la_seleccion_de_documentos_para_el_lote",
        "winui.firmar.la_seleccion_de_la_carpeta_del_lote_se",
        "winui.firmar.la_seleccion_de_la_carpeta_de_salida_se",
        "winui.firmar.el_lote_se_cancelo_solo_se_consideran",
        "winui.firmar.la_carga_de_la_previsualizacion_se",
        "winui.firmar.la_firma_se_guardo_como_pero_su",
        "winui.firmar.la_firma_se_cancelo_antes_de_completarse",
    ];

    private static bool IsCancelledOrDiscardedMessage(string message)
    {
        foreach (var key in CancelledOrDiscardedMessageKeys)
        {
            if (MatchesCatalogTemplate(message, Localizer.Text(key)))
            {
                return true;
            }
        }
        return false;
    }

    // Las plantillas con {0} coinciden si el mensaje conserva el texto fijo
    // anterior y posterior al dato insertado.
    private static bool MatchesCatalogTemplate(string message, string template)
    {
        var parts = template.Split("{0}");
        return parts.Length == 1
            ? string.Equals(message, template, StringComparison.Ordinal)
            : message.StartsWith(parts[0], StringComparison.Ordinal) &&
                message.EndsWith(parts[^1], StringComparison.Ordinal);
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
        var fieldError = ValidateAllFields();
        if (fieldError is not null)
            dialog.CorrectFieldAction = () => FocusField(fieldError);
        await Localizer.ShowAsync(dialog);
    }
}
