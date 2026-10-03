// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.ViewModels;
using GrxFirma.WinUI.Views;
using Microsoft.UI.Xaml;
using Windows.System;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Navigation;
using Microsoft.UI.Windowing;
using Windows.Graphics;

namespace GrxFirma.WinUI;

public sealed partial class MainWindow : Window
{
    private readonly App _app;
    private readonly OfficialUpdateChecker _officialUpdates = new();
    private readonly UpdateNoticeSchedule _updateSchedule = new(TimeProvider.System);
    private string _updateVersion = string.Empty;
    private string _updateReleaseUrl = string.Empty;
    private readonly IHelpLauncherService _helpLauncher =
        new WindowsHelpLauncherService();

    public MainWindow()
    {
        _app = (App)Application.Current;
        ViewModel = new MainWindowViewModel();
        InitializeComponent();
        var language = System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName;
        UpdateNotice.Title = SealUiCatalog.Text(language, "Nueva versión disponible");
        DownloadUpdateButton.Content = SealUiCatalog.Text(language, "Descargar e instalar");
        ReleaseNotesButton.Content = SealUiCatalog.Text(language, "Ver novedades");
        DismissUpdateButton.Content = SealUiCatalog.Text(language, "Ahora no");
        ConfigureInitialWindow();
        SetFacturaeNavigationVisibility(
            _app.FacturaeToolsEnabled);
        _app.FacturaeToolsEnabledChanged +=
            OnFacturaeToolsEnabledChanged;
        Closed += OnClosed;
    }

    internal void ShowInitialPage()
    {
        RootNavigation.SelectedItem = RootNavigation.MenuItems[0];
        ContentFrame.Navigate(typeof(SignPage));
    }

    internal void OpenPortalSeal(PortalSealSession session)
    {
        // En el editor del portal no hay motor local: su aviso solo confundiría.
        ConnectionNotices.Visibility = Visibility.Collapsed;
        RootNavigation.IsPaneVisible = false;
        RootNavigation.IsBackButtonVisible = NavigationViewBackButtonVisible.Collapsed;
        RootNavigation.MenuItems.Clear();
        RootNavigation.FooterMenuItems.Clear();
        Title = SealUiCatalog.Text(
            System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName,
            "portal.seal.title");
        if (ContentFrame.Content is not SignPage page)
            return;
        // El árbol visual se reorganiza cuando la página ya está cargada: antes,
        // WinUI no garantiza padres ni enlaces y la reubicación puede fallar.
        if (page.IsLoaded)
        {
            page.ConfigurePortalSeal(session);
            return;
        }
        void OnLoaded(object sender, RoutedEventArgs args)
        {
            page.Loaded -= OnLoaded;
            page.ConfigurePortalSeal(session);
        }
        page.Loaded += OnLoaded;
    }

    internal void ShowSettingsPage()
    {
        RootNavigation.SelectedItem = SettingsNavigationItem;
        ViewModel.ActivePageTitle = "Configuración";
        if (ContentFrame.CurrentSourcePageType != typeof(SettingsPage))
        {
            ContentFrame.Navigate(typeof(SettingsPage));
        }
    }

    internal async Task OpenHelpManualAsync()
    {
        RootNavigation.SelectedItem = HelpNavigationItem;
        ViewModel.ActivePageTitle = SealUiCatalog.Text(
            System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName,
            "Ayuda");
        if (ContentFrame.CurrentSourcePageType != typeof(HelpPage))
        {
            ContentFrame.Navigate(typeof(HelpPage));
        }
        if (ContentFrame.Content is HelpPage page)
        {
            page.ViewModel.Activate();
            await page.ViewModel.OpenInstalledManualAsync();
        }
    }

    internal void ShowAboutPage()
    {
        RootNavigation.SelectedItem = AboutNavigationItem;
        ViewModel.ActivePageTitle = SealUiCatalog.Text(
            System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName,
            "Acerca de GrxFirma");
        if (ContentFrame.CurrentSourcePageType != typeof(AboutPage))
        {
            ContentFrame.Navigate(typeof(AboutPage));
        }
    }

    internal async Task<bool> ShowReleaseNotesAsync(string notes, string version,
        bool acknowledge)
    {
        var language = System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName;
        string Label(string key) => SealUiCatalog.Text(language, key);
        var content = new StackPanel { Spacing = 10, Padding = new Thickness(8) };
        foreach (var rawLine in notes.Split('\n'))
        {
            var line = rawLine.Trim();
            if (line.Length == 0) continue;
            var heading = line.StartsWith("## ", StringComparison.Ordinal);
            if (heading) line = line[3..];
            if (line.StartsWith("- ", StringComparison.Ordinal))
                line = "• " + line[2..];
            // Solo texto: no se crean controles HTML ni Hipervínculos.
            line = line.Replace("**", string.Empty).Replace("`", string.Empty);
            content.Children.Add(new TextBlock
            {
                Text = line,
                TextWrapping = TextWrapping.Wrap,
                FontSize = heading ? 18 : 14,
                FontWeight = heading
                    ? Microsoft.UI.Text.FontWeights.SemiBold
                    : Microsoft.UI.Text.FontWeights.Normal,
            });
        }
        if (content.Children.Count == 0)
        {
            content.Children.Add(new TextBlock
            {
                Text = Label("Novedades no disponibles en esta instalación."),
                TextWrapping = TextWrapping.Wrap,
            });
        }
        var dialog = new ContentDialog
        {
            XamlRoot = RootNavigation.XamlRoot,
            Title = Label("Novedades de GrxFirma %1").Replace("%1", version),
            Content = new ScrollViewer
            {
                Content = content,
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                MaxHeight = 520,
                MinWidth = 320,
            },
            PrimaryButtonText = Label("Entendido"),
            DefaultButton = ContentDialogButton.Primary,
        };
        var result = await dialog.ShowAsync();
        return acknowledge && result == ContentDialogResult.Primary;
    }

    public MainWindowViewModel ViewModel { get; }

    private void ConfigureInitialWindow()
    {
        // Respetar el área útil, también en una VM o con escala de texto alta.
        var area = DisplayArea.GetFromWindowId(AppWindow.Id, DisplayAreaFallback.Primary).WorkArea;
        var width = Math.Min(1120, Math.Max(1, area.Width - 48));
        var height = Math.Min(820, Math.Max(1, area.Height - 48));
        AppWindow.Resize(new SizeInt32(width, height));
        AppWindow.Move(new PointInt32(area.X + (area.Width - width) / 2,
            area.Y + (area.Height - height) / 2));
    }

    internal void ApplyThemePreference(int? themeIndex)
    {
        AppRoot.RequestedTheme = themeIndex switch
        {
            1 => ElementTheme.Light,
            0 or 2 => ElementTheme.Dark,
            // Las preferencias ausentes se muestran como índice 0 en Configuración.
            // Usar el mismo valor evita anunciar «oscuro» mientras se hereda el claro del SO.
            _ => ElementTheme.Dark,
        };
    }

    private void OnNavigationItemInvoked(
        NavigationView sender,
        NavigationViewItemInvokedEventArgs args)
    {
        if (args.InvokedItemContainer?.Tag is not string tag)
        {
            return;
        }

        ViewModel.ActivePageTitle = tag switch
        {
            "sign" => "Firmar",
            "verify" => "Verificar",
            "hash" => "Huellas",
            "protect" => "Proteger",
            "certificates" => "Certificados",
            "facturae" => "Facturae y FACe",
            "settings" => "Configuración",
            "diagnostics" => "Diagnóstico",
            "help" => "Ayuda",
            "about" => "Acerca de",
            _ => "GrxFirma",
        };

        var pageType = tag switch
        {
            "sign" => typeof(SignPage),
            "verify" => typeof(VerifyPage),
            "hash" => typeof(HashPage),
            "protect" => typeof(ProtectPage),
            "certificates" => typeof(CertificatesPage),
            "facturae" => typeof(FacturaePage),
            "settings" => typeof(SettingsPage),
            "diagnostics" => typeof(DiagnosticsPage),
            "help" => typeof(HelpPage),
            "about" => typeof(AboutPage),
            _ => null,
        };
        if (pageType is not null && ContentFrame.CurrentSourcePageType != pageType)
        {
            ContentFrame.Navigate(pageType);
        }
    }

    internal async Task RefreshFacturaeToolsAvailabilityAsync()
    {
        if (!_app.OperationSession.TryGetOperations(
                GrxFirma.WinUI.Core.Operations
                    .DesktopOperationActions.GetSettings,
                out var operations))
        {
            SetFacturaeNavigationVisibility(false);
            return;
        }

        try
        {
            var result = await operations.GetSettingsAsync();
            var enabled =
                result.IsSuccess &&
                result.Outcome == "success" &&
                result.Data?.FacturaeToolsEnabled == true;
            _app.SetFacturaeToolsEnabled(enabled);
            SetFacturaeNavigationVisibility(enabled);
            if (result.IsSuccess && result.Outcome == "success" && result.Data is not null)
            {
                ApplyThemePreference(result.Data.ThemeIndex);
                _app.SetKeepInTray(result.Data.CloseBehavior != "exit");
            }
        }
        catch
        {
            _app.SetFacturaeToolsEnabled(false);
            SetFacturaeNavigationVisibility(false);
        }
    }

    // Espera a que el motor termine de conectar o falle, como mucho el plazo
    // indicado, para que la primera comprobación use el motor si está listo.
    internal async Task WaitForEngineSettledAsync(TimeSpan maximum, CancellationToken cancellationToken)
    {
        var deadline = DateTimeOffset.UtcNow + maximum;
        while (!ViewModel.IsBackendReady && !ViewModel.HasConnectionError &&
               DateTimeOffset.UtcNow < deadline)
            await Task.Delay(500, cancellationToken);
    }

    internal async Task RefreshUpdateAvailabilityAsync(CancellationToken cancellationToken = default)
    {
        if (!UpdatePreferenceStore.Read()) return;
        using var engineBudget = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        engineBudget.CancelAfter(TimeSpan.FromSeconds(4));
        try
        {
            var engineAvailable = _app.OperationSession.Supports(DesktopOperationActions.CheckUpdates);
            if (engineAvailable && _app.OperationSession.TryGetOperations(
                    DesktopOperationActions.GetSettings, out var settingsOperations))
            {
                try
                {
                    var settings = await settingsOperations.GetSettingsAsync(engineBudget.Token);
                    if (settings.IsSuccess && settings.Outcome == "success" && settings.Data is not null)
                    {
                        UpdatePreferenceStore.Write(settings.Data.CheckForUpdates != false);
                        if (settings.Data.CheckForUpdates == false) return;
                    }
                    else engineAvailable = false;
                }
                catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
                {
                    engineAvailable = false;
                }
            }
            else if (engineAvailable) engineAvailable = false;
            OfficialRelease? release = null;
            var current = _app.InstalledVersion;
            if (engineAvailable && _app.OperationSession.TryGetOperations(
                    DesktopOperationActions.CheckUpdates, out var updateOperations))
            {
                try
                {
                    var result = await updateOperations.CheckUpdatesAsync(engineBudget.Token);
                    if (result.IsSuccess && result.Outcome == "success" &&
                        result.Data?.HasNewVersion == true &&
                        OfficialUpdateChecker.IsReleaseForVersion(result.Data.ReleaseUrl, result.Data.LatestVersion) &&
                        OfficialUpdateChecker.IsNewer(current, result.Data.LatestVersion))
                        release = new(result.Data.LatestVersion, result.Data.ReleaseUrl);
                    if (!result.IsSuccess) engineAvailable = false;
                }
                catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
                {
                    engineAvailable = false;
                }
            }
            else if (engineAvailable) engineAvailable = false;
            if (!engineAvailable && !cancellationToken.IsCancellationRequested)
                release = await _officialUpdates.CheckAsync(cancellationToken);
            if (release is null || !OfficialUpdateChecker.IsNewer(current, release.Version) ||
                !_updateSchedule.ShouldShow(release.Version) || cancellationToken.IsCancellationRequested)
                return;
            _updateVersion = release.Version;
            _updateReleaseUrl = release.Url;
            var language = System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName;
            var message = SealUiCatalog.Text(language,
                "Hay una versión nueva de GrxFirma (%1). Tienes la %2.")
                .Replace("%1", release.Version).Replace("%2", current);
            // Solo se avisa del motor si su conexión ha fallado de verdad, no si
            // la comprobación se adelantó a que terminara de conectar.
            if (ViewModel.HasConnectionError)
                message += "\n" + SealUiCatalog.Text(language,
                    "Además, el motor local no responde; instalar la versión nueva puede resolverlo.");
            ViewModel.SetUpdateAvailable(message);
            _app.NotifyUpdateInTray(message);
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested) { }
        catch (Exception error)
        {
            _app.LogAutomaticUpdateFailure(error);
        }
    }

    private void OnDismissUpdate(object sender, RoutedEventArgs args)
    {
        _updateSchedule.Dismiss(_updateVersion);
        ViewModel.HideUpdateNotice();
    }

    private async void OnOpenUpdateRelease(object sender, RoutedEventArgs args)
    {
        if (!OfficialUpdateChecker.IsReleaseForVersion(_updateReleaseUrl, _updateVersion)) return;
        // GitHub publica SHA256SUMS; con firma de código se podrá automatizar la instalación.
        await Launcher.LaunchUriAsync(new Uri(_updateReleaseUrl));
    }

    private void OnFacturaeToolsEnabledChanged(
        object? sender,
        bool enabled) =>
        SetFacturaeNavigationVisibility(enabled);

    private void SetFacturaeNavigationVisibility(bool enabled)
    {
        var visibility = enabled
            ? Visibility.Visible
            : Visibility.Collapsed;
        ComplementsHeader.Visibility = visibility;
        FacturaeNavigationItem.Visibility = visibility;
    }

    private void OnClosed(
        object sender,
        WindowEventArgs args)
    {
        _app.FacturaeToolsEnabledChanged -=
            OnFacturaeToolsEnabledChanged;
        Closed -= OnClosed;
    }

    private async void OnOpenCurrentDiagnostic(
        object sender,
        RoutedEventArgs args)
    {
        var diagnostic = ViewModel.CurrentDiagnostic;
        if (diagnostic is null)
        {
            return;
        }

        var dialog = new OperationDiagnosticDialog(diagnostic)
        {
            XamlRoot = RootNavigation.XamlRoot,
        };
        await dialog.ShowAsync();
    }

    private void OnContentFrameNavigated(
        object sender,
        NavigationEventArgs args)
    {
        if (args.Content is not FrameworkElement page)
        {
            return;
        }

        if (page.IsLoaded)
        {
            _ = FocusFirstPageActionAsync(page);
            return;
        }

        RoutedEventHandler? loadedHandler = null;
        loadedHandler = (_, _) =>
        {
            page.Loaded -= loadedHandler;
            _ = FocusFirstPageActionAsync(page);
        };
        page.Loaded += loadedHandler;
    }

    private static async Task FocusFirstPageActionAsync(
        FrameworkElement page)
    {
        var firstAction =
            FocusManager.FindFirstFocusableElement(page);
        if (firstAction is null)
        {
            return;
        }

        await FocusManager.TryFocusAsync(
            firstAction,
            FocusState.Programmatic);
    }
}
