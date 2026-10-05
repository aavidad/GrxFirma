// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Windowing;

namespace GrxFirma.WinUI;

public partial class App : Application
{
    private MainWindow? _window;
    private ReconnectingIpcClient? _ipcClient;
    private readonly CancellationTokenSource _lifetimeCancellation = new();
    private int _windowClosed;
    private bool _facturaeToolsEnabled;
    private bool _keepInTray = true;
    private bool _exitRequested;
    private bool _firstHideExplained;
    private WindowsTrayIcon? _tray;
    private ReleaseNotesManager? _releaseNotes;
    private bool _releaseNotesDialogOpen;
    private SingleInstanceSignal? _singleInstance;
    private PortalSealSession? _portalSealSession;

    internal DesktopOperationSession OperationSession { get; } = new();
    internal WindowsRestServer RestServer { get; } = new();
    internal IFilePickerService FilePickerService { get; private set; } = null!;
    internal ISecurePasswordPromptService SecurePasswordPromptService
    {
        get;
        private set;
    } = null!;

    internal event EventHandler<bool>? FacturaeToolsEnabledChanged;

    internal bool FacturaeToolsEnabled => _facturaeToolsEnabled;
    internal bool PortalSealActive => _portalSealSession is not null;
    internal string InstalledVersion => _releaseNotes?.InstalledVersion ?? string.Empty;
    internal void ApplyLanguagePreference(string? language) =>
        _window?.ApplyLanguagePreference(language);
    internal void RefreshTrayLanguage() => _tray?.RefreshLanguage();
    internal void ShowSupportTargetPage(string destination) =>
        _window?.ShowSupportTargetPage(destination);

    public App()
    {
        InitializeComponent();
        UnhandledException += (_, e) => RegistrarErrorNoControlado(e.Exception);
    }

    // Deja constancia local de un fallo no controlado para poder diagnosticarlo;
    // el fichero vive en el perfil del usuario y se recorta para no crecer sin fin.
    private static void RegistrarErrorNoControlado(Exception? error)
    {
        if (error is null) return;
        try
        {
            var dir = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "GrxFirma", "logs");
            Directory.CreateDirectory(dir);
            var file = Path.Combine(dir, "winui-errores.log");
            if (File.Exists(file) && new FileInfo(file).Length > 256 * 1024)
                File.Delete(file);
            File.AppendAllText(file,
                $"{DateTimeOffset.Now:O} {error.GetType().FullName} 0x{error.HResult:X8}{Environment.NewLine}{error}{Environment.NewLine}{Environment.NewLine}");
        }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        var commandLine = Environment.GetCommandLineArgs().Skip(1).ToArray();
        if (!PortalSealSession.TryLoad(commandLine, out var portalSession,
            out var portalRequested))
        {
            Exit();
            return;
        }
        if (portalRequested)
        {
            _portalSealSession = portalSession;
            _keepInTray = false;
            _window = new MainWindow();
            FilePickerService = new WindowsFilePickerService(_window);
            SecurePasswordPromptService = new WindowsSecurePasswordPromptService(_window);
            _window.Closed += OnMainWindowClosed;
            _window.AppWindow.Closing += OnMainWindowClosing;
            _window.ShowInitialPage();
            _window.OpenPortalSeal(portalSession!);
            if (_window.AppWindow.Presenter is OverlappedPresenter presenter)
                presenter.Maximize();
            _window.Activate();
            return;
        }
        try
        {
            _singleInstance = SingleInstanceSignal.Open();
            if (!_singleInstance.IsPrimary)
            {
                _singleInstance.ShowExisting();
                _singleInstance.Dispose();
                _singleInstance = null;
                Exit();
                return;
            }
        }
        catch
        {
            // Un fallo del canal local no permite abrir otra instancia.
            Exit();
            return;
        }

        _window = new MainWindow();
        FilePickerService = new WindowsFilePickerService(_window);
        SecurePasswordPromptService =
            new WindowsSecurePasswordPromptService(_window);
        _window.Closed += OnMainWindowClosed;
        _window.AppWindow.Closing += OnMainWindowClosing;
        _tray = new WindowsTrayIcon(
            _window,
            ShowWindow,
            ShowSettings,
            ShowHelpManual,
            ShowReleaseNotesHistory,
            ShowAbout,
            ShowPendingReleaseNotes,
            RequestExit);
        _releaseNotes = new ReleaseNotesManager();
        _singleInstance.Listen(() => EnqueueOnUi(ShowWindow));
        _window.ShowInitialPage();
        _ = RunUpdateLoopAsync();

        if (!WinUiLaunchOptions.TryParse(commandLine, out var options, out var safeError))
        {
            _window.Activate();
            _window.ViewModel.SetConnectionFailure(
                safeError,
                "invalid_configuration",
                "admission",
                "app_local");
            return;
        }

        if (!options.StartHidden || _tray?.Installed != true ||
            !options.HasBackendEndpoint)
        {
            _window.Activate();
        }
        if (!string.IsNullOrEmpty(_releaseNotes.Pending))
        {
            if (options.StartHidden && _tray?.Installed == true &&
                options.HasBackendEndpoint)
            {
                var language = Localizer.Language;
                var message = SealUiCatalog.Text(language,
                    "winui.ventana.actualizado_a_1_ver_novedades").Replace("%1", _releaseNotes.InstalledVersion);
                _tray.ShowReleaseNotesNotification(message);
            }
            else
            {
                _ = _window.DispatcherQueue.TryEnqueue(ShowPendingReleaseNotes);
            }
        }

        if (!options.HasBackendEndpoint)
        {
            _window.ViewModel.SetStandalone();
            return;
        }

        _window.ViewModel.SetConnecting();
        _ = ConnectBackendAsync(options);
    }

    private async Task ConnectBackendAsync(WinUiLaunchOptions launchOptions)
    {
        try
        {
            var initial = await NdjsonIpcClient.ConnectAsync(
                new NamedPipeIpcConnector(),
                launchOptions.ToEndpoint(),
                cancellationToken: _lifetimeCancellation.Token);
            // Si una cancelación o un error invalidan el canal, la siguiente
            // petición abre otro: la aplicación no se queda sin motor.
            var client = new ReconnectingIpcClient(
                initial,
                cancellationToken => NdjsonIpcClient.ConnectAsync(
                    new NamedPipeIpcConnector(),
                    launchOptions.ToEndpoint(),
                    cancellationToken: cancellationToken));
            var previous = Interlocked.CompareExchange(
                ref _ipcClient,
                client,
                null);
            if (previous is not null)
            {
                await DisposeClientQuietlyAsync(client);
                return;
            }

            // El cierre puede ganar la carrera justo después de terminar el
            // hello. En ese caso se retira y cierra el cliente recién creado.
            if (Volatile.Read(ref _windowClosed) != 0)
            {
                var owned = Interlocked.CompareExchange(
                    ref _ipcClient,
                    null,
                    client);
                if (ReferenceEquals(owned, client))
                {
                    await DisposeClientQuietlyAsync(client);
                }
                return;
            }

            EnqueueOnUi(() =>
            {
                OperationSession.Attach(client);
                _window?.ViewModel.SetConnected(client.ServerHello);
                _ = _window?.RefreshFacturaeToolsAvailabilityAsync();

            });
        }
        catch (OperationCanceledException)
            when (_lifetimeCancellation.IsCancellationRequested)
        {
            // La ventana ya no existe; no se publica un falso error de red.
        }
        catch (IpcClientException exception)
        {
            if (Volatile.Read(ref _windowClosed) != 0)
            {
                return;
            }
            EnqueueOnUi(() => _window?.ViewModel.SetConnectionFailure(
                exception.UserMessage,
                exception.Code,
                exception.Phase,
                exception.LikelyOwner));
        }
        catch
        {
            if (Volatile.Read(ref _windowClosed) != 0)
            {
                return;
            }
            EnqueueOnUi(() => _window?.ViewModel.SetConnectionFailure(
                Localizer.Text("winui.ventana.no_se_pudo_iniciar_la_conexion_segura")));
        }
    }


    internal void NotifyUpdateInTray(string message)
    {
        if (_window is not null && !_window.AppWindow.IsVisible)
            _tray?.ShowUpdateNotification(message);
    }

    internal void LogAutomaticUpdateFailure(Exception error)
    {
        try
        {
            var directory = Path.Combine(Environment.GetFolderPath(
                Environment.SpecialFolder.LocalApplicationData), "GrxFirma", "logs");
            Directory.CreateDirectory(directory);
            var path = Path.Combine(directory, "winui-updates.log");
            if (File.Exists(path) && new FileInfo(path).Length > 256 * 1024)
                File.Delete(path);
            File.AppendAllText(path,
                $"{DateTimeOffset.Now:O} {error.GetType().Name}{Environment.NewLine}");
        }
        catch (Exception ignored) when (ignored is IOException or UnauthorizedAccessException) { }
    }

    private async Task RunUpdateLoopAsync()
    {
        using var timer = new PeriodicTimer(UpdateNoticeSchedule.Interval);
        await Task.Yield();
        try
        {
            if (_window is not null)
            {
                await _window.WaitForEngineSettledAsync(TimeSpan.FromSeconds(20), _lifetimeCancellation.Token);
                await _window.RefreshUpdateAvailabilityAsync(_lifetimeCancellation.Token);
            }
            while (await timer.WaitForNextTickAsync(_lifetimeCancellation.Token))
                if (_window is not null)
                    await _window.RefreshUpdateAvailabilityAsync(_lifetimeCancellation.Token);
        }
        catch (OperationCanceledException) when (_lifetimeCancellation.IsCancellationRequested) { }
    }

    internal void SetFacturaeToolsEnabled(bool enabled)
    {
        if (_facturaeToolsEnabled == enabled)
        {
            return;
        }

        _facturaeToolsEnabled = enabled;
        FacturaeToolsEnabledChanged?.Invoke(this, enabled);
    }

    internal void ApplyThemePreference(int? themeIndex)
        => _window?.ApplyThemePreference(themeIndex);

    internal void SetKeepInTray(bool enabled) => _keepInTray = enabled;

    private void ShowWindow()
    {
        _window?.AppWindow.Show();
        _window?.Activate();
    }

    private void ShowSettings()
    {
        ShowWindow();
        _window?.ShowSettingsPage();
    }

    private void ShowHelpManual()
    {
        ShowWindow();
        if (_window is not null)
        {
            _ = _window.OpenHelpManualAsync();
        }
    }

    private void ShowAbout()
    {
        ShowWindow();
        _window?.ShowAboutPage();
    }

    internal async Task ShowReleaseNotesHistoryAsync()
    {
        if (_releaseNotesDialogOpen) return;
        ShowWindow();
        if (_window is not null && _releaseNotes is not null)
        {
            _releaseNotesDialogOpen = true;
            try
            {
                await _window.ShowReleaseNotesAsync(_releaseNotes.History,
                    _releaseNotes.InstalledVersion, false);
            }
            finally { _releaseNotesDialogOpen = false; }
        }
    }

    private void ShowReleaseNotesHistory() => _ = ShowReleaseNotesHistoryAsync();

    private void ShowPendingReleaseNotes() => _ = ShowPendingReleaseNotesAsync();

    private async Task ShowPendingReleaseNotesAsync()
    {
        if (_releaseNotesDialogOpen || _window is null || _releaseNotes is null ||
            string.IsNullOrEmpty(_releaseNotes.Pending)) return;
        ShowWindow();
        _releaseNotesDialogOpen = true;
        try
        {
            if (await _window.ShowReleaseNotesAsync(_releaseNotes.Pending,
                _releaseNotes.InstalledVersion, true))
                _releaseNotes.Acknowledge();
        }
        finally { _releaseNotesDialogOpen = false; }
    }

    private void RequestExit()
    {
        _exitRequested = true;
        _window?.Close();
    }

    internal void CompletePortalSeal()
    {
        _exitRequested = true;
        _window?.Close();
    }

    internal void FallbackPortalSeal()
    {
        _portalSealSession = null;
        _exitRequested = true;
        _window?.Close();
    }

    private void OnMainWindowClosing(AppWindow sender, AppWindowClosingEventArgs args)
    {
        if (_portalSealSession is not null)
        {
            _portalSealSession.CancelOnClose();
            return;
        }
        if (_exitRequested || !_keepInTray || _tray?.Installed != true)
        {
            return;
        }
        args.Cancel = true;
        sender.Hide();
        if (!_firstHideExplained)
        {
            _firstHideExplained = true;
            _tray.ExplainFirstHide();
        }
    }

    private void EnqueueOnUi(Action action)
    {
        if (Volatile.Read(ref _windowClosed) != 0)
        {
            return;
        }

        var dispatcher = _window?.DispatcherQueue;
        if (dispatcher is null ||
            !dispatcher.TryEnqueue(() =>
            {
                if (Volatile.Read(ref _windowClosed) == 0)
                {
                    action();
                }
            }))
        {
            return;
        }
    }

    private async void OnMainWindowClosed(object sender, WindowEventArgs args)
    {
        if (Interlocked.Exchange(ref _windowClosed, 1) != 0)
        {
            return;
        }

        _lifetimeCancellation.Cancel();
        RestServer.Dispose();
        _tray?.Dispose();
        _singleInstance?.Dispose();
        var client = Interlocked.Exchange(ref _ipcClient, null);
        if (client is not null)
        {
            OperationSession.Detach(client);
            await DisposeClientQuietlyAsync(client);
        }
    }

    private static async Task DisposeClientQuietlyAsync(
        IIpcClient client)
    {
        try
        {
            await client.DisposeAsync();
        }
        catch
        {
            // El cierre no presenta ni registra detalles de transporte.
        }
    }
}
