// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Controls;
using GrxFirma.WinUI.Core.Localization;
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
    private readonly DeferredTreePass _localization;

    public MainWindow()
    {
        _app = (App)Application.Current;
        ViewModel = new MainWindowViewModel();
        InitializeComponent();
        ApplyControlLanguage();
        RefreshProgrammaticLanguage();
        _localization = Localizer.Attach(AppRoot);
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
            Localizer.Language,
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
        ViewModel.ActivePageTitle = Localizer.Text("winui.comun.configuracion");
        if (ContentFrame.CurrentSourcePageType != typeof(SettingsPage))
        {
            ContentFrame.Navigate(typeof(SettingsPage));
        }
    }

    internal void ShowSupportTargetPage(string destination)
    {
        foreach (var item in RootNavigation.MenuItems.OfType<NavigationViewItem>())
        {
            if (string.Equals(item.Tag as string, destination, StringComparison.Ordinal))
            {
                RootNavigation.SelectedItem = item;
                var page = destination switch
                {
                    "verify" => typeof(VerifyPage),
                    "certificates" => typeof(CertificatesPage),
                    _ => typeof(SignPage),
                };
                ViewModel.ActivePageTitle = item.Content?.ToString() ?? string.Empty;
                ContentFrame.Navigate(page);
                return;
            }
        }
    }

    internal async Task OpenHelpManualAsync()
    {
        RootNavigation.SelectedItem = HelpNavigationItem;
        ViewModel.ActivePageTitle = SealUiCatalog.Text(
            Localizer.Language,
            "winui.comun.ayuda");
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
            Localizer.Language,
            "winui.comun.acerca_de_grxfirma");
        if (ContentFrame.CurrentSourcePageType != typeof(AboutPage))
        {
            ContentFrame.Navigate(typeof(AboutPage));
        }
    }

    internal async Task<bool> ShowReleaseNotesAsync(string notes, string version,
        bool acknowledge)
    {
        var language = Localizer.Language;
        string Label(string key) => SealUiCatalog.Text(language, key);
        var content = new StackPanel { Spacing = 10, Padding = new Thickness(8) };
        foreach (var rawLine in notes.Split('\n'))
        {
            var line = rawLine.Trim();
            if (line.Length == 0) continue;
            var heading = line.StartsWith("## ", StringComparison.Ordinal);
            if (heading) line = line[3..];
            var bullet = line.StartsWith("- ", StringComparison.Ordinal);
            if (bullet) line = line[2..];
            // Solo texto: no se crean controles HTML ni Hipervínculos.
            line = line.Replace("**", string.Empty).Replace("`", string.Empty);
            line = Localizer.Text(line);
            if (bullet) line = "• " + line;
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
                Text = Label("winui.ventana.novedades_no_disponibles_en_esta"),
                TextWrapping = TextWrapping.Wrap,
            });
        }
        var dialog = new ContentDialog
        {
            XamlRoot = RootNavigation.XamlRoot,
            Title = Label("winui.ventana.novedades_de_grxfirma_1").Replace("%1", version),
            Content = new ScrollViewer
            {
                Content = content,
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                MaxHeight = 520,
                MinWidth = 320,
            },
            PrimaryButtonText = Label("winui.ventana.entendido"),
            DefaultButton = ContentDialogButton.Primary,
        };
        var result = await Localizer.ShowAsync(dialog);
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

    internal void ApplyLanguagePreference(string? language)
    {
        var changed = Localizer.SetLanguage(language);
        ApplyControlLanguage();
        if (!changed) return;
        RefreshProgrammaticLanguage();
        // NavigationView almacena sus entradas aparte del árbol visual.
        foreach (var item in RootNavigation.MenuItems.OfType<DependencyObject>())
            Localizer.Apply(item);
        foreach (var item in RootNavigation.FooterMenuItems.OfType<DependencyObject>())
            Localizer.Apply(item);
        Localizer.Apply(AppRoot);
        _app.RefreshTrayLanguage();
        if (RootNavigation.SelectedItem is NavigationViewItem selected)
            ViewModel.ActivePageTitle = selected.Content?.ToString() ?? string.Empty;
        var pageType = ContentFrame.CurrentSourcePageType;
        if (pageType is not null) ContentFrame.Navigate(pageType);
    }

    // Los textos propios de WinUI (selector de fecha y hora, «Cerrar
    // navegación», iconos) siguen el idioma de la aplicación y no el de
    // Windows: Language en la raíz para formatos y nombres de los controles,
    // y PrimaryLanguageOverride para los recursos que se cargan después.
    private void ApplyControlLanguage()
    {
        AppRoot.Language = GrxFirma.WinUI.Core.Localization.AppCulture.Tag(Localizer.Language);
        StartupLanguage.ApplyOverride(Localizer.Language);
    }

    private void RefreshProgrammaticLanguage()
    {
        var language = Localizer.Language;
        EniNavigationItem.Content = SealUiCatalog.Text(language, "paridad.lote3.eni.nav");
        UpdateNotice.Title = SealUiCatalog.Text(language, "winui.comun.nueva_version_disponible");
        DownloadUpdateButton.Content = SealUiCatalog.Text(language, "winui.ventana.descargar_e_instalar");
        ReleaseNotesButton.Content = SealUiCatalog.Text(language, "winui.ventana.ver_novedades");
        DismissUpdateButton.Content = SealUiCatalog.Text(language, "winui.ventana.ahora_no");
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
            "sign" => Localizer.Text("winui.comun.firmar"),
            "verify" => Localizer.Text("winui.comun.verificar"),
            "hash" => Localizer.Text("winui.ventana.huellas"),
            "protect" => Localizer.Text("winui.comun.proteger"),
            "certificates" => Localizer.Text("winui.comun.certificados"),
            "facturae" => Localizer.Text("winui.comun.facturae_y_face"),
            "eni" => SealUiCatalog.Text(Localizer.Language, "paridad.lote3.eni.nav"),
            "settings" => Localizer.Text("winui.comun.configuracion"),
            "diagnostics" => Localizer.Text("winui.comun.diagnostico"),
            "help" => Localizer.Text("winui.comun.ayuda"),
            "about" => Localizer.Text("winui.comun.acerca_de"),
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
            "eni" => typeof(EniPage),
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
                ApplyLanguagePreference(result.Data.Language);
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

    // Espera una petición al motor como mucho el plazo indicado sin cancelarla:
    // si no llega a tiempo devuelve null y la petición termina por su cuenta.
    private static async Task<T?> WaitWithinAsync<T>(
        Task<T> request, TimeSpan budget, CancellationToken cancellationToken)
        where T : class
    {
        var finished = await Task.WhenAny(
            request, Task.Delay(budget, cancellationToken)).ConfigureAwait(true);
        if (!ReferenceEquals(finished, request))
        {
            _ = request.ContinueWith(
                static task => _ = task.Exception,
                TaskContinuationOptions.OnlyOnFaulted);
            return null;
        }
        try
        {
            return await request.ConfigureAwait(true);
        }
        catch (Exception error) when (error is not OperationCanceledException)
        {
            return null;
        }
    }

    internal async Task RefreshUpdateAvailabilityAsync(CancellationToken cancellationToken = default)
    {
        if (!UpdatePreferenceStore.Read()) return;
        // El plazo del motor solo limita la espera: cancelar una petición IPC ya
        // enviada invalidaría la conexión con el motor.
        var engineBudget = TimeSpan.FromSeconds(4);
        try
        {
            var engineAvailable = _app.OperationSession.Supports(DesktopOperationActions.CheckUpdates);
            if (engineAvailable && _app.OperationSession.TryGetOperations(
                    DesktopOperationActions.GetSettings, out var settingsOperations))
            {
                var settings = await WaitWithinAsync(
                    settingsOperations.GetSettingsAsync(), engineBudget, cancellationToken);
                if (settings is not null && settings.IsSuccess &&
                    settings.Outcome == "success" && settings.Data is not null)
                {
                    UpdatePreferenceStore.Write(settings.Data.CheckForUpdates != false);
                    if (settings.Data.CheckForUpdates == false) return;
                }
                else engineAvailable = false;
            }
            else if (engineAvailable) engineAvailable = false;
            OfficialRelease? release = null;
            var current = _app.InstalledVersion;
            if (engineAvailable && _app.OperationSession.TryGetOperations(
                    DesktopOperationActions.CheckUpdates, out var updateOperations))
            {
                var result = await WaitWithinAsync(
                    updateOperations.CheckUpdatesAsync(), engineBudget, cancellationToken);
                if (result is not null && result.IsSuccess && result.Outcome == "success" &&
                    result.Data?.HasNewVersion == true &&
                    OfficialUpdateChecker.IsReleaseForVersion(result.Data.ReleaseUrl, result.Data.LatestVersion) &&
                    OfficialUpdateChecker.IsNewer(current, result.Data.LatestVersion))
                    release = new(result.Data.LatestVersion, result.Data.ReleaseUrl);
                if (result is null || !result.IsSuccess) engineAvailable = false;
            }
            else if (engineAvailable) engineAvailable = false;
            if (!engineAvailable && !cancellationToken.IsCancellationRequested)
                release = await _officialUpdates.CheckAsync(cancellationToken);
            if (release is null || !OfficialUpdateChecker.IsNewer(current, release.Version) ||
                !_updateSchedule.ShouldShow(release.Version) || cancellationToken.IsCancellationRequested)
                return;
            _updateVersion = release.Version;
            _updateReleaseUrl = release.Url;
            var language = Localizer.Language;
            var message = SealUiCatalog.Text(language,
                "winui.ventana.hay_una_version_nueva_de_grxfirma_1")
                .Replace("%1", release.Version).Replace("%2", current);
            // Solo se avisa del motor si su conexión ha fallado de verdad, no si
            // la comprobación se adelantó a que terminara de conectar.
            if (ViewModel.HasConnectionError)
                message += "\n" + SealUiCatalog.Text(language,
                    "winui.ventana.ademas_el_motor_local_no_responde");
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
        _localization.Close();
        _app.FacturaeToolsEnabledChanged -=
            OnFacturaeToolsEnabledChanged;
        Closed -= OnClosed;
    }

    private bool _unexpectedErrorShown;

    // Un único aviso a la vez: si el propio aviso fallara, no se encadenan
    // diálogos sin fin.
    internal void ShowUnexpectedError(Exception exception)
    {
        if (_unexpectedErrorShown || RootNavigation.XamlRoot is null) return;
        _unexpectedErrorShown = true;
        DispatcherQueue.TryEnqueue(async () =>
        {
            try
            {
                var dialog = new OperationDiagnosticDialog(
                    GrxFirma.WinUI.Core.Diagnostics.OperationDiagnosticMapper.FromException(exception))
                {
                    XamlRoot = RootNavigation.XamlRoot,
                };
                await Localizer.ShowAsync(dialog);
            }
            catch (Exception)
            {
                // El fallo ya consta en el registro local.
            }
            finally
            {
                _unexpectedErrorShown = false;
            }
        });
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
        await Localizer.ShowAsync(dialog);
    }

    private void OnContentFrameNavigated(
        object sender,
        NavigationEventArgs args)
    {
        if (args.Content is not FrameworkElement page)
        {
            return;
        }

        // Traducir antes de que la página se cargue: los controles de WinUI
        // (selector de fecha, por ejemplo) componen su nombre accesible al
        // cargarse y conservaban el texto en castellano.
        Localizer.Apply(page);

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
