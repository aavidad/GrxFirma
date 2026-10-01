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
    private string _statusTitle = "Ayuda preparada";
    private string _statusMessage =
        "Elija una opción para consultar ayuda o soporte.";
    private bool _isActive;
    private bool _isBusy;
    private bool _canLaunch;
    private bool _hasStatus;
    private int _operationInProgress;
    private InfoBarSeverity _statusSeverity =
        InfoBarSeverity.Informational;

    public HelpPageViewModel(IHelpLauncherService launcher)
        : base(
            "Ayuda",
            "Guía básica, recursos instalados y canales oficiales de soporte.",
            "La ayuda visual está temporalmente inactiva.")
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
            "Estos recursos son locales o usan destinos oficiales fijos. Ninguna ruta ni dirección recibida durante una firma puede abrirse desde aquí.");
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
            "Buscando ayuda instalada",
            _launcher.OpenInstalledManualAsync);

    public Task OpenInstallationFolderAsync() =>
        ExecuteAsync(
            "Abriendo carpeta de instalación",
            _launcher.OpenInstallationFolderAsync);

    public Task OpenOfficialProjectAsync() =>
        ExecuteAsync(
            "Abriendo proyecto oficial",
            _launcher.OpenOfficialProjectAsync);

    public Task OpenPrivateSupportAsync() =>
        ExecuteAsync(
            "Abriendo contacto privado",
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
        StatusMessage = "Espere un momento.";
        StatusSeverity = InfoBarSeverity.Informational;

        try
        {
            var result = await operation(lifetime.Token);
            if (!IsCurrentLifetime(lifetime))
            {
                return;
            }

            StatusTitle = result.Succeeded
                ? "Acción completada"
                : "No se pudo completar";
            StatusMessage = result.Message;
            StatusSeverity = result.Succeeded
                ? InfoBarSeverity.Success
                : InfoBarSeverity.Warning;
        }
        catch (OperationCanceledException)
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "Acción cancelada";
                StatusMessage =
                    "La apertura se canceló sin modificar ningún documento.";
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "No se pudo completar";
                StatusMessage =
                    "Windows no pudo abrir el recurso solicitado. Inténtelo de nuevo o use otra opción de ayuda.";
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
