// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Services;
using GrxFirma.WinUI.Core.Operations;
using Microsoft.UI.Xaml.Controls;
using System.Text;

namespace GrxFirma.WinUI.ViewModels;

public sealed class AboutPageViewModel : ObservableObject
{
    private static string VersionUnavailable => Localizer.Text("winui.acerca.version_no_disponible");
    private const int MaxReleaseNotesBytes = 64 * 1024;
    private const int MaxCurrentSectionCharacters = 8192;
    private readonly IHelpLauncherService _launcher;
    private readonly DesktopOperationSession _session;
    private readonly OfficialUpdateChecker _officialUpdates;
    private readonly string _installedVersion;
    private CancellationTokenSource? _pageLifetime;
    private string _statusTitle = Localizer.Text("winui.acerca.informacion_del_proyecto");
    private string _statusMessage =
        Localizer.Text("winui.acerca.puede_consultar_la_licencia_o_el_codigo");
    private bool _isActive;
    private bool _isBusy;
    private bool _canLaunch;
    private bool _canCheckUpdates;
    private bool _hasStatus;
    private int _operationInProgress;
    private InfoBarSeverity _statusSeverity =
        InfoBarSeverity.Informational;

    public AboutPageViewModel(
        IHelpLauncherService launcher,
        DesktopOperationSession session,
        OfficialUpdateChecker? officialUpdates = null)
    {
        ArgumentNullException.ThrowIfNull(launcher);
        ArgumentNullException.ThrowIfNull(session);
        _launcher = launcher;
        _session = session;
        _officialUpdates = officialUpdates ?? new OfficialUpdateChecker();
        var installedVersion = ResolveInstalledVersion();
        _installedVersion = installedVersion;
        VersionText = installedVersion == VersionUnavailable
            ? Localizer.Text(installedVersion)
            : Localizer.Fill("winui.acerca.version", ("version", installedVersion));
        ReleaseNotesText = Localizer.Text(ResolveReleaseNotes(installedVersion));
    }

    public string Title { get; } = Localizer.Text("winui.comun.acerca_de");

    public string Description { get; } =
        Localizer.Text("winui.acerca.autoria_version_licencia_y_codigo_fuente");

    public string VersionText { get; }

    public string ReleaseNotesText { get; }

    public string StatusTitle
    {
        get => _statusTitle;
        private set => SetProperty(ref _statusTitle, Localizer.Text(value));
    }

    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, Localizer.Text(value));
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

    public bool CanCheckUpdates
    {
        get => _canCheckUpdates;
        private set => SetProperty(ref _canCheckUpdates, value);
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
        _session.AvailabilityChanged += OnAvailabilityChanged;
        RefreshAvailability();
    }

    public void Deactivate()
    {
        if (!_isActive)
        {
            return;
        }

        _isActive = false;
        _session.AvailabilityChanged -= OnAvailabilityChanged;
        CanLaunch = false;
        CanCheckUpdates = false;
        _pageLifetime?.Cancel();
        _pageLifetime?.Dispose();
        _pageLifetime = null;
    }

    public Task OpenOfficialLicenseAsync() =>
        ExecuteAsync(
            Localizer.Text("winui.acerca.abriendo_licencia_oficial"),
            _launcher.OpenOfficialLicenseAsync);

    public Task OpenOfficialProjectAsync() =>
        ExecuteAsync(
            Localizer.Text("winui.acerca.abriendo_repositorio_oficial"),
            _launcher.OpenOfficialProjectAsync);

    public Task OpenOfficialReleasesAsync() =>
        ExecuteAsync(
            Localizer.Text("winui.acerca.abriendo_versiones_oficiales"),
            _launcher.OpenOfficialReleasesAsync);

    public async Task CheckUpdatesAsync()
    {
        var lifetime = _pageLifetime;
        if (!_isActive ||
            lifetime is null ||
            !_session.TryGetOperations(
                DesktopOperationActions.CheckUpdates,
                out var operations) ||
            Interlocked.CompareExchange(
                ref _operationInProgress,
                1,
                0) != 0)
        {
            HasStatus = true;
            StatusTitle = Localizer.Text("winui.comun.comprobacion_no_disponible");
            StatusMessage =
                Localizer.Text("winui.acerca.abra_grxfirma_desde_el_lanzador");
            StatusSeverity = InfoBarSeverity.Warning;
            RefreshAvailability();
            return;
        }

        IsBusy = true;
        CanLaunch = false;
        CanCheckUpdates = false;
        HasStatus = true;
        StatusTitle = Localizer.Text("winui.acerca.consultando_github");
        StatusMessage =
            Localizer.Text("winui.acerca.se_esta_consultando_la_ultima_version");
        StatusSeverity = InfoBarSeverity.Informational;

        try
        {
            // Sin token: cancelar la petición ya enviada invalidaría la conexión
            // con el motor; si la página se cierra, el resultado se descarta.
            var result = await operations.CheckUpdatesAsync();
            if (!IsCurrentLifetime(lifetime))
            {
                return;
            }
            if (!result.IsSuccess ||
                result.Outcome != "success" ||
                result.Data is null)
            {
                // El motor no lee el proxy del sistema de Windows; la consulta
                // directa sí lo usa, con las credenciales de Windows solo para
                // el proxy.
                await CheckDirectlyAsync(lifetime);
                return;
            }

            var latest = CleanVersion(result.Data.LatestVersion);
            var current = CleanVersion(result.Data.CurrentVersion);
            if (result.Data.State == "sin_publicaciones")
            {
                StatusTitle = result.Data.Title;
                StatusMessage = result.Data.Message;
                StatusSeverity = InfoBarSeverity.Informational;
            }
            else if (result.Data.HasNewVersion)
            {
                StatusTitle = Localizer.Text("winui.comun.nueva_version_disponible");
                StatusMessage = Localizer.Fill(
                    "winui.acerca.esta_disponible_esta_instalacion_usa",
                    ("latest", latest), ("current", current));
                StatusSeverity = InfoBarSeverity.Warning;
            }
            else if (!result.Data.IsComparable)
            {
                StatusTitle = Localizer.Text("winui.acerca.build_no_comparable");
                StatusMessage = Localizer.Fill(
                    "winui.acerca.la_ultima_version_publicada_es_pero_este",
                    ("latest", latest));
                StatusSeverity = InfoBarSeverity.Informational;
            }
            else
            {
                StatusTitle = Localizer.Text("winui.acerca.grxfirma_esta_actualizado");
                StatusMessage = Localizer.Fill(
                    "winui.acerca.la_version_instalada_es_la_ultima",
                    ("current", current));
                StatusSeverity = InfoBarSeverity.Success;
            }
        }
        catch (OperationCanceledException)
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = Localizer.Text("winui.comun.comprobacion_cancelada");
                StatusMessage =
                    Localizer.Text("winui.acerca.no_se_descargo_ni_instalo_ningun_archivo");
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = Localizer.Text("winui.acerca.no_se_pudo_comprobar");
                StatusMessage =
                    Localizer.Text("winui.acerca.no_se_pudo_consultar_github_de_forma");
                StatusSeverity = InfoBarSeverity.Warning;
            }
        }
        finally
        {
            Interlocked.Exchange(ref _operationInProgress, 0);
            if (IsCurrentLifetime(lifetime))
            {
                IsBusy = false;
                RefreshAvailability();
            }
        }
    }

    private async Task CheckDirectlyAsync(CancellationTokenSource lifetime)
    {
        try
        {
            var release = await _officialUpdates.CheckAsync(lifetime.Token);
            if (!IsCurrentLifetime(lifetime))
            {
                return;
            }
            var current = CleanVersion(_installedVersion);
            if (release is null)
            {
                StatusTitle = Localizer.Text("winui.actualizaciones.sin_version_estable_titulo");
                StatusMessage = Localizer.Text("winui.actualizaciones.sin_version_estable");
                StatusSeverity = InfoBarSeverity.Informational;
            }
            else if (OfficialUpdateChecker.IsNewer(_installedVersion, release.Version))
            {
                StatusTitle = Localizer.Text("winui.comun.nueva_version_disponible");
                StatusMessage = Localizer.Fill(
                    "winui.acerca.esta_disponible_esta_instalacion_usa",
                    ("latest", CleanVersion(release.Version)), ("current", current));
                StatusSeverity = InfoBarSeverity.Warning;
            }
            else if (!OfficialUpdateChecker.IsComparable(_installedVersion))
            {
                StatusTitle = Localizer.Text("winui.acerca.build_no_comparable");
                StatusMessage = Localizer.Fill(
                    "winui.acerca.la_ultima_version_publicada_es_pero_este",
                    ("latest", CleanVersion(release.Version)));
                StatusSeverity = InfoBarSeverity.Informational;
            }
            else
            {
                StatusTitle = Localizer.Text("winui.acerca.grxfirma_esta_actualizado");
                StatusMessage = Localizer.Fill(
                    "winui.acerca.la_version_instalada_es_la_ultima",
                    ("current", current));
                StatusSeverity = InfoBarSeverity.Success;
            }
        }
        catch (OfficialUpdateCheckException error)
        {
            (Microsoft.UI.Xaml.Application.Current as App)?.LogAutomaticUpdateFailure(error);
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = Localizer.Text("winui.acerca.no_se_pudo_comprobar");
                StatusMessage = Localizer.Text(error.Failure.MessageKey);
                StatusSeverity = InfoBarSeverity.Warning;
            }
        }
    }

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
                StatusMessage = Localizer.Text("winui.acerca.no_se_ha_abierto_ningun_recurso_externo");
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = Localizer.Text("winui.comun.no_se_pudo_completar");
                StatusMessage =
                    Localizer.Text("winui.comun.windows_no_pudo_abrir_el_recurso_oficial");
                StatusSeverity = InfoBarSeverity.Error;
            }
        }
        finally
        {
            Interlocked.Exchange(ref _operationInProgress, 0);
            if (IsCurrentLifetime(lifetime))
            {
                IsBusy = false;
                RefreshAvailability();
            }
        }
    }

    private void OnAvailabilityChanged(
        object? sender,
        EventArgs args) =>
        RefreshAvailability();

    private void RefreshAvailability()
    {
        var idle = Volatile.Read(ref _operationInProgress) == 0;
        CanLaunch = _isActive && idle;
        CanCheckUpdates =
            _isActive &&
            idle &&
            _session.Supports(DesktopOperationActions.CheckUpdates);
    }

    private static string CleanVersion(string? value)
    {
        var cleaned = new string(
            (value ?? string.Empty)
                .Trim()
                .Take(64)
                .Where(character =>
                    char.IsAsciiLetterOrDigit(character) ||
                    character is '.' or '-' or '+' or '_')
                .ToArray());
        return cleaned.Length == 0
            ? Localizer.Text("winui.acerca.desconocida")
            : cleaned;
    }

    private bool IsCurrentLifetime(
        CancellationTokenSource lifetime) =>
        _isActive &&
        ReferenceEquals(_pageLifetime, lifetime);

    private static string ResolveInstalledVersion()
    {
        try
        {
            var baseDirectory = Path.TrimEndingDirectorySeparator(
                Path.GetFullPath(AppContext.BaseDirectory));
            var versionFile = Path.GetFullPath(
                Path.Combine(baseDirectory, "VERSION.txt"));
            var relativePath = Path.GetRelativePath(
                baseDirectory,
                versionFile);
            if (Path.IsPathRooted(relativePath) ||
                relativePath.StartsWith(
                    $"..{Path.DirectorySeparatorChar}",
                    StringComparison.Ordinal) ||
                !File.Exists(versionFile) ||
                (File.GetAttributes(versionFile) &
                 FileAttributes.ReparsePoint) != 0)
            {
                return VersionUnavailable;
            }

            var value = File.ReadAllText(versionFile).Trim();
            if (value.Length is < 1 or > 64 ||
                value.Any(character =>
                    !char.IsAsciiLetterOrDigit(character) &&
                    character is not '.' and not '-' and not '+' and not '_'))
            {
                return VersionUnavailable;
            }

            return value;
        }
        catch
        {
            return VersionUnavailable;
        }
    }

    private static string ResolveReleaseNotes(string versionText)
    {
        if (versionText == VersionUnavailable)
        {
            return Localizer.Text("winui.acerca.no_se_pueden_mostrar_las_novedades");
        }

        try
        {
            var baseDirectory = Path.TrimEndingDirectorySeparator(
                Path.GetFullPath(AppContext.BaseDirectory));
            var helpDirectory = Path.GetFullPath(
                Path.Combine(baseDirectory, "help"));
            var notesFile = Path.GetFullPath(
                Path.Combine(helpDirectory, "NOVEDADES.md"));
            var relativePath = Path.GetRelativePath(baseDirectory, notesFile);
            if (Path.IsPathRooted(relativePath) ||
                relativePath.StartsWith(
                    $"..{Path.DirectorySeparatorChar}",
                    StringComparison.Ordinal) ||
                !Directory.Exists(helpDirectory) ||
                !File.Exists(notesFile) ||
                (File.GetAttributes(helpDirectory) & FileAttributes.ReparsePoint) != 0 ||
                (File.GetAttributes(notesFile) & FileAttributes.ReparsePoint) != 0)
            {
                return Localizer.Text("winui.acerca.no_se_encontro_el_archivo_de_novedades");
            }

            using var stream = new FileStream(
                notesFile,
                FileMode.Open,
                FileAccess.Read,
                FileShare.Read,
                4096,
                FileOptions.SequentialScan);
            var bytes = new byte[MaxReleaseNotesBytes + 1];
            var total = 0;
            while (total < bytes.Length)
            {
                var count = stream.Read(bytes, total, bytes.Length - total);
                if (count == 0)
                {
                    break;
                }
                total += count;
            }
            if (total > MaxReleaseNotesBytes)
            {
                return Localizer.Text("winui.acerca.el_archivo_de_novedades_instalado_es");
            }

            var content = new UTF8Encoding(false, true).GetString(bytes, 0, total);
            using var reader = new StringReader(content);
            var heading = "## " + versionText + " — ";
            var section = new StringBuilder();
            var inCurrentSection = false;
            string? line;
            while ((line = reader.ReadLine()) is not null)
            {
                if (line.StartsWith("## ", StringComparison.Ordinal))
                {
                    if (inCurrentSection)
                    {
                        break;
                    }
                    inCurrentSection = line.StartsWith(heading, StringComparison.Ordinal);
                    continue;
                }
                if (inCurrentSection)
                {
                    section.AppendLine(line);
                    if (section.Length > MaxCurrentSectionCharacters)
                    {
                        return Localizer.Text("winui.acerca.las_novedades_de_esta_version_son");
                    }
                }
            }
            var currentNotes = section.ToString().Trim();
            return inCurrentSection && currentNotes.Length > 0
                ? currentNotes
                : Localizer.Text("winui.acerca.no_se_encontro_la_seccion_de_novedades");
        }
        catch
        {
            return Localizer.Text("winui.acerca.no_se_pudieron_leer_las_novedades");
        }
    }
}
