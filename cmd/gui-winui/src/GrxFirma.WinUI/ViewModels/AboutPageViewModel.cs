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
    private const string VersionUnavailable = "Versión no disponible";
    private const int MaxReleaseNotesBytes = 64 * 1024;
    private const int MaxCurrentSectionCharacters = 8192;
    private readonly IHelpLauncherService _launcher;
    private readonly DesktopOperationSession _session;
    private CancellationTokenSource? _pageLifetime;
    private string _statusTitle = "Información del proyecto";
    private string _statusMessage =
        "Puede consultar la licencia o el código fuente mediante destinos oficiales fijos.";
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
        DesktopOperationSession session)
    {
        ArgumentNullException.ThrowIfNull(launcher);
        ArgumentNullException.ThrowIfNull(session);
        _launcher = launcher;
        _session = session;
        VersionText = ResolveInstalledVersion();
        ReleaseNotesText = ResolveReleaseNotes(VersionText);
    }

    public string Title { get; } = "Acerca de";

    public string Description { get; } =
        "Autoría, versión, licencia y código fuente de GrxFirma.";

    public string VersionText { get; }

    public string ReleaseNotesText { get; }

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
            "Abriendo licencia oficial",
            _launcher.OpenOfficialLicenseAsync);

    public Task OpenOfficialProjectAsync() =>
        ExecuteAsync(
            "Abriendo repositorio oficial",
            _launcher.OpenOfficialProjectAsync);

    public Task OpenOfficialReleasesAsync() =>
        ExecuteAsync(
            "Abriendo versiones oficiales",
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
            StatusTitle = "Comprobación no disponible";
            StatusMessage =
                "Abra GrxFirma desde el lanzador instalado para conectar el motor local y vuelva a intentarlo; la firma local no queda bloqueada.";
            StatusSeverity = InfoBarSeverity.Warning;
            RefreshAvailability();
            return;
        }

        IsBusy = true;
        CanLaunch = false;
        CanCheckUpdates = false;
        HasStatus = true;
        StatusTitle = "Consultando GitHub";
        StatusMessage =
            "Se está consultando la última versión publicada; no se descargará ni instalará ningún archivo.";
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
                StatusTitle = "No se pudo comprobar";
                StatusMessage = result.SafeUserMessage;
                StatusSeverity = InfoBarSeverity.Warning;
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
                StatusTitle = "Nueva versión disponible";
                StatusMessage =
                    $"Está disponible {latest}; esta instalación usa {current}. Revise las notas en GitHub. GrxFirma no descargará ni ejecutará nada automáticamente.";
                StatusSeverity = InfoBarSeverity.Warning;
            }
            else if (!result.Data.IsComparable)
            {
                StatusTitle = "Build no comparable";
                StatusMessage =
                    $"La última versión publicada es {latest}, pero este build de desarrollo no se puede comparar automáticamente.";
                StatusSeverity = InfoBarSeverity.Informational;
            }
            else
            {
                StatusTitle = "GrxFirma está actualizado";
                StatusMessage = $"La versión instalada ({current}) es la última publicada.";
                StatusSeverity = InfoBarSeverity.Success;
            }
        }
        catch (OperationCanceledException)
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "Comprobación cancelada";
                StatusMessage =
                    "No se descargó ni instaló ningún archivo.";
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "No se pudo comprobar";
                StatusMessage =
                    "No se pudo consultar GitHub de forma segura. Compruebe la conexión a Internet o el proxy y vuelva a intentarlo; la firma local sigue disponible.";
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
                StatusMessage = "No se ha abierto ningún recurso externo.";
                StatusSeverity = InfoBarSeverity.Informational;
            }
        }
        catch
        {
            if (IsCurrentLifetime(lifetime))
            {
                StatusTitle = "No se pudo completar";
                StatusMessage =
                    "Windows no pudo abrir el recurso oficial solicitado.";
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
            ? "desconocida"
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

            return $"Versión {value}";
        }
        catch
        {
            return VersionUnavailable;
        }
    }

    private static string ResolveReleaseNotes(string versionText)
    {
        if (!versionText.StartsWith("Versión ", StringComparison.Ordinal))
        {
            return "No se pueden mostrar las novedades porque no se encontró la versión instalada.";
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
                return "No se encontró el archivo de novedades instalado. Compruebe que la instalación esté completa.";
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
                return "El archivo de novedades instalado es demasiado grande para mostrarlo.";
            }

            var content = new UTF8Encoding(false, true).GetString(bytes, 0, total);
            using var reader = new StringReader(content);
            var heading = "## " + versionText["Versión ".Length..] + " — ";
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
                        return "Las novedades de esta versión son demasiado extensas para mostrarlas aquí.";
                    }
                }
            }
            var currentNotes = section.ToString().Trim();
            return inCurrentSection && currentNotes.Length > 0
                ? currentNotes
                : "No se encontró la sección de novedades de esta versión en la instalación.";
        }
        catch
        {
            return "No se pudieron leer las novedades instaladas. Compruebe que la instalación esté completa.";
        }
    }
}
