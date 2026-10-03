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
    private NdjsonIpcClient? _ipcClient;
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
    internal IFilePickerService FilePickerService { get; private set; } = null!;
    internal ISecurePasswordPromptService SecurePasswordPromptService
    {
        get;
        private set;
    } = null!;

    internal event EventHandler<bool>? FacturaeToolsEnabledChanged;

    internal bool FacturaeToolsEnabled => _facturaeToolsEnabled;
    internal bool PortalSealActive => _portalSealSession is not null;

    public App()
    {
        InitializeComponent();
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
                var language = System.Globalization.CultureInfo.CurrentUICulture.TwoLetterISOLanguageName;
                var message = SealUiCatalog.Text(language,
                    "Actualizado a %1: ver novedades").Replace("%1", _releaseNotes.InstalledVersion);
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
            var client = await NdjsonIpcClient.ConnectAsync(
                new NamedPipeIpcConnector(),
                launchOptions.ToEndpoint(),
                cancellationToken: _lifetimeCancellation.Token);
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
                _ = _window?.RefreshUpdateAvailabilityAsync();
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
                "No se pudo iniciar la conexión segura con el motor local."));
        }
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
        NdjsonIpcClient client)
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
