// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public sealed class HelpPageViewModel
    : WorkspacePageViewModel
{
    private readonly IHelpLauncherService _launcher;
    private CancellationTokenSource? _pageLifetime;
    private string _statusTitle = Localizer.Text("winui.ayuda.ayuda_preparada");
    private string _statusMessage =
        Localizer.Text("winui.ayuda.elija_una_opcion_para_consultar_ayuda_o");
    private bool _isActive;
    private bool _isBusy;
    private bool _canLaunch;
    private bool _hasStatus;
    private int _operationInProgress;
    private InfoBarSeverity _statusSeverity =
        InfoBarSeverity.Informational;

    public HelpPageViewModel(IHelpLauncherService launcher)
        : base(
            Localizer.Text("winui.comun.ayuda"),
            Localizer.Text("winui.ayuda.guia_basica_recursos_instalados_y"),
            Localizer.Text("winui.ayuda.la_ayuda_visual_esta_temporalmente"))
    {
        ArgumentNullException.ThrowIfNull(launcher);
        _launcher = launcher;
    }

    public string StatusTitle
    {
        get => _statusTitle;
        private set => SetProperty(ref _statusTitle, value);
    }

    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanLaunch
    {
        get => _canLaunch;
        private set => SetProperty(ref _canLaunch, value);
    }

    public bool HasStatus
    {
        get => _hasStatus;
        private set => SetProperty(ref _hasStatus, value);
    }

    public InfoBarSeverity StatusSeverity
    {
        get => _statusSeverity;
        private set => SetProperty(ref _statusSeverity, value);
    }

    public void Activate()
    {
        if (_isActive)
        {
            return;
        }

        _pageLifetime = new CancellationTokenSource();
        _isActive = true;
        SetOperationAvailability(
            true,
            Localizer.Text("winui.ayuda.estos_recursos_son_locales_o_usan"));
        CanLaunch = Volatile.Read(ref _operationInProgress) == 0;
    }

    public void Deactivate()
    {
        if (!_isActive)
        {
            return;
        }

        _isActive = false;
        CanLaunch = false;
        _pageLifetime?.Cancel();
        _pageLifetime?.Dispose();
        _pageLifetime = null;
    }

    public Task OpenInstalledManualAsync() =>
        ExecuteAsync(
            Localizer.Text("winui.ayuda.buscando_ayuda_instalada"),
            _launcher.OpenInstalledManualAsync);

    public Task OpenInstallationFolderAsync() =>
        ExecuteAsync(
            Localizer.Text("winui.ayuda.abriendo_carpeta_de_instalacion"),
            _launcher.OpenInstallationFolderAsync);

    public Task OpenOfficialProjectAsync() =>
        ExecuteAsync(
            Localizer.Text("winui.ayuda.abriendo_proyecto_oficial"),
            _launcher.OpenOfficialProjectAsync);

    public Task OpenPrivateSupportAsync() =>
        ExecuteAsync(
            Localizer.Text("winui.ayuda.abriendo_contacto_privado"),
            _launcher.OpenPrivateSupportAsync);

    private async Task ExecuteAsync(
        string progressTitle,
        Func<CancellationToken, Task<HelpLaunchResult>> operation)
    {
        var lifetime = _pageLifetime;
        if (!_isActive ||
            lifetime is null ||
            Interlocked.CompareExchange(
                ref _operationInProgress,
                1,
                0) != 0)
        {
            return;
        }

        IsBusy = true;
        CanLaunch = false;
        HasStatus = true;
        StatusTitle = progressTitle;
        StatusMessage = Localizer.Text("winui.comun.espere_un_momento");
        StatusSeverity = InfoBarSeverity.Informational;

        try
        {
            var result = await operation(lifetime.Token);
            if (!IsCurrentLifetime(lifetime))
            {
                return;
            }

            StatusTitle = result.Succeeded
                ? Localizer.Text("winui.comun.accion_completada")
                : Localizer.Text("winui.comun.no_se_pudo_completar");
            StatusMessage = result.Message;
            StatusSeverity = result.Succeeded
                ? InfoBarSeverity.Success
                : InfoBarSeverity.Warning;
        }
        catch (OperationCanceledException)
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = Localizer.Text("winui.comun.accion_cancelada");
                StatusMessage =
                    Localizer.Text("winui.ayuda.la_apertura_se_cancelo_sin_modificar");
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = Localizer.Text("winui.comun.no_se_pudo_completar");
                StatusMessage =
                    Localizer.Text("winui.ayuda.windows_no_pudo_abrir_el_recurso");
                StatusSeverity = InfoBarSeverity.Error;
            }
        }
        finally
        {
            Interlocked.Exchange(ref _operationInProgress, 0);
            if (IsCurrentLifetime(lifetime))
            {
                IsBusy = false;
                CanLaunch = true;
            }
        }
    }

    private bool IsCurrentLifetime(
        CancellationTokenSource lifetime) =>
        _isActive &&
        ReferenceEquals(_pageLifetime, lifetime);
}
