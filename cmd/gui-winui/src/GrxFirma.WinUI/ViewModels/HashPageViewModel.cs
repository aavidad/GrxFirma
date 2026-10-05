// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public sealed class HashPageViewModel
    : WorkspacePageViewModel
{
    private const int MaximumVisibleResults = 50;

    private static readonly IReadOnlyList<HashFormatOption>
        FileFormatOptions =
        [
            new("winui.huella.hexadecimal_grxfirma_hexhash", "hex"),
            new("winui.huella.base64_grxfirma_hashb64", "base64"),
            new("winui.huella.binario_hash", "bin"),
        ];

    private static readonly IReadOnlyList<HashFormatOption>
        DirectoryFormatOptions =
        [
            new("winui.huella.xml_grxfirma_hashfiles", "xml"),
            new("winui.huella.texto_grxfirma_txthashfiles", "txt"),
            new("winui.huella.csv_csv", "csv"),
        ];

    private static readonly IReadOnlySet<string> FileManifestExtensions =
        new HashSet<string>(
            [".hexhash", ".hashb64", ".hash"],
            StringComparer.OrdinalIgnoreCase);

    private static readonly IReadOnlySet<string> DirectoryManifestExtensions =
        new HashSet<string>(
            [".hashfiles", ".txthashfiles", ".csv", ".xml", ".txt"],
            StringComparer.OrdinalIgnoreCase);

    private readonly DesktopOperationSession _session;
    private readonly IFilePickerService _filePicker;
    private CancellationTokenSource? _pageLifetime;
    private CancellationTokenSource? _operationCancellation;
    private string? _inputPath;
    private string? _manifestPath;
    private string _inputName = string.Empty;
    private string _manifestName = string.Empty;
    private string _selectedAlgorithm = "SHA-256";
    private IReadOnlyList<HashFormatOption> _availableFormats =
        FileFormatOptions;
    private HashFormatOption _selectedFormat = FileFormatOptions[0];
    private string _actionLabel = Localizer.Text("winui.huella.crear_huella");
    private Visibility _manifestVisibility = Visibility.Collapsed;
    private string _resultTitle = Localizer.Text("winui.comun.sin_resultado");
    private string _resultMessage =
        Localizer.Text("winui.huella.seleccione_una_operacion_y_un_origen");
    private string _resultSummary = Localizer.Text("winui.huella.sin_evidencias_del_motor_local");
    private IReadOnlyList<string> _resultItems = [];
    private bool _isActive;
    private bool _isCreateMode = true;
    private bool _isDirectoryMode;
    private bool _isRecursive;
    private bool _isBusy;
    private bool _canSelect;
    private bool _canSetRecursive;
    private bool _canExecute;
    private bool _canCancel;
    private bool _hasResult;
    private InfoBarSeverity _resultSeverity =
        InfoBarSeverity.Informational;

    public HashPageViewModel(
        DesktopOperationSession session,
        IFilePickerService filePicker)
        : base(
            Localizer.Text("winui.ventana.huellas"),
            Localizer.Text("winui.huella.crea_o_comprueba_huellas_de_ficheros_y"),
            Localizer.Text("winui.huella.las_huellas_no_estan_disponibles_porque"))
    {
        ArgumentNullException.ThrowIfNull(session);
        ArgumentNullException.ThrowIfNull(filePicker);
        _session = session;
        _filePicker = filePicker;
    }

    public event Action<OperationDiagnostic>? DiagnosticRequested;

    public IReadOnlyList<string> Algorithms { get; } =
        ["SHA-256", "SHA-384", "SHA-512"];

    public string InputName
    {
        // Vacío sin selección: el PlaceholderText traducido muestra el aviso
        // y sigue el idioma aunque cambie en caliente.
        get => _inputName;
        private set => SetProperty(ref _inputName, value);
    }

    public string ManifestName
    {
        // Vacío sin selección: el PlaceholderText traducido muestra el aviso
        // y sigue el idioma aunque cambie en caliente.
        get => _manifestName;
        private set => SetProperty(ref _manifestName, value);
    }

    public string SelectedAlgorithm
    {
        get => _selectedAlgorithm;
        private set => SetProperty(ref _selectedAlgorithm, value);
    }

    public IReadOnlyList<HashFormatOption> AvailableFormats
    {
        get => _availableFormats;
        private set => SetProperty(ref _availableFormats, value);
    }

    public HashFormatOption SelectedFormat
    {
        get => _selectedFormat;
        private set => SetProperty(ref _selectedFormat, value);
    }

    public string ActionLabel
    {
        get => _actionLabel;
        private set => SetProperty(ref _actionLabel, value);
    }

    public string ResultTitle
    {
        get => _resultTitle;
        private set => SetProperty(ref _resultTitle, value);
    }

    public string ResultMessage
    {
        get => _resultMessage;
        private set => SetProperty(ref _resultMessage, value);
    }

    public string ResultSummary
    {
        get => _resultSummary;
        private set => SetProperty(ref _resultSummary, value);
    }

    public IReadOnlyList<string> ResultItems
    {
        get => _resultItems;
        private set => SetProperty(ref _resultItems, value);
    }

    public bool IsCreateMode
    {
        get => _isCreateMode;
        private set => SetProperty(ref _isCreateMode, value);
    }

    public bool IsDirectoryMode
    {
        get => _isDirectoryMode;
        private set => SetProperty(ref _isDirectoryMode, value);
    }

    public bool IsRecursive
    {
        get => _isRecursive;
        private set => SetProperty(ref _isRecursive, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanSelect
    {
        get => _canSelect;
        private set => SetProperty(ref _canSelect, value);
    }

    public bool CanExecute
    {
        get => _canExecute;
        private set => SetProperty(ref _canExecute, value);
    }

    public bool CanSetRecursive
    {
        get => _canSetRecursive;
        private set => SetProperty(ref _canSetRecursive, value);
    }

    public bool CanCancel
    {
        get => _canCancel;
        private set => SetProperty(ref _canCancel, value);
    }

    public bool HasResult
    {
        get => _hasResult;
        private set => SetProperty(ref _hasResult, value);
    }

    public InfoBarSeverity ResultSeverity
    {
        get => _resultSeverity;
        private set => SetProperty(ref _resultSeverity, value);
    }

    public Visibility ManifestVisibility
    {
        get => _manifestVisibility;
        private set => SetProperty(ref _manifestVisibility, value);
    }

    public void Activate()
    {
        if (_isActive)
        {
            return;
        }

        _isActive = true;
        _pageLifetime = new CancellationTokenSource();
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
        _operationCancellation?.Cancel();
        _pageLifetime?.Cancel();
        _pageLifetime?.Dispose();
        _pageLifetime = null;
        RefreshAvailability();
    }

    public void SetCreateMode(bool create)
    {
        if (IsBusy || IsCreateMode == create)
        {
            return;
        }

        IsCreateMode = create;
        ActionLabel = create ? Localizer.Text("winui.huella.crear_huella") : Localizer.Text("winui.huella.comprobar_huella");
        ManifestVisibility = create
            ? Visibility.Collapsed
            : Visibility.Visible;
        _manifestPath = null;
        ManifestName = string.Empty;
        ResetResult();
        RefreshCommandState();
    }

    public void SetDirectoryMode(bool directory)
    {
        if (IsBusy || IsDirectoryMode == directory)
        {
            return;
        }

        IsDirectoryMode = directory;
        AvailableFormats = directory
            ? DirectoryFormatOptions
            : FileFormatOptions;
        SelectedFormat = AvailableFormats[0];
        _inputPath = null;
        _manifestPath = null;
        InputName = string.Empty;
        ManifestName = string.Empty;
        ResetResult();
        RefreshCommandState();
    }

    public void SetAlgorithm(string? algorithm)
    {
        if (IsBusy || algorithm is null ||
            !Algorithms.Contains(algorithm, StringComparer.Ordinal))
        {
            return;
        }

        SelectedAlgorithm = algorithm;
        ResetResult();
    }

    public void SetFormat(HashFormatOption? format)
    {
        if (IsBusy || format is null ||
            !AvailableFormats.Contains(format))
        {
            return;
        }

        SelectedFormat = format;
        ResetResult();
    }

    public void SetRecursive(bool recursive)
    {
        if (IsBusy)
        {
            return;
        }

        IsRecursive = recursive;
        ResetResult();
    }

    public async Task SelectInputAsync()
    {
        if (!CanSelect || _pageLifetime is null)
        {
            return;
        }

        try
        {
            var path = IsDirectoryMode
                ? await _filePicker.PickFolderAsync(_pageLifetime.Token)
                : await _filePicker.PickOpenFileAsync(
                    OpenFilePickerProfile.SignedOrOriginalDocument,
                    _pageLifetime.Token);
            if (path is null)
            {
                return;
            }

            _inputPath = path;
            InputName = DisplayPathName(path, IsDirectoryMode);
            ResetResult();
            RefreshCommandState();
        }
        catch (OperationCanceledException)
            when (_pageLifetime?.IsCancellationRequested == true)
        {
        }
        catch (Exception exception)
        {
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception));
        }
    }

    public async Task SelectManifestAsync()
    {
        if (!CanSelect || IsCreateMode || _pageLifetime is null)
        {
            return;
        }

        try
        {
            var path = await _filePicker.PickOpenFileAsync(
                OpenFilePickerProfile.HashManifest,
                _pageLifetime.Token);
            if (path is null)
            {
                return;
            }
            if (!IsAllowedManifest(path))
            {
                _manifestPath = null;
                ManifestName = Localizer.Text("winui.huella.formato_de_manifiesto_no_admitido");
                ShowLocalValidation(
                    Localizer.Text("winui.huella.manifiesto_no_valido"),
                    IsDirectoryMode
                        ? Localizer.Text("winui.huella.seleccione_un_manifiesto_hashfiles")
                        : Localizer.Text("winui.huella.seleccione_una_huella_hexhash_hashb64_o"));
                RefreshCommandState();
                return;
            }

            _manifestPath = path;
            ManifestName = DisplayPathName(path, directory: false);
            ResetResult();
            RefreshCommandState();
        }
        catch (OperationCanceledException)
            when (_pageLifetime?.IsCancellationRequested == true)
        {
        }
        catch (Exception exception)
        {
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception));
        }
    }

    public async Task ExecuteAsync()
    {
        if (!CanExecute || _inputPath is null ||
            _pageLifetime is null)
        {
            RefreshAvailability();
            return;
        }

        var requiredAction = IsCreateMode
            ? DesktopOperationActions.HashCreate
            : DesktopOperationActions.HashCheck;
        if (!_session.TryGetOperations(requiredAction, out var operations))
        {
            RefreshAvailability();
            return;
        }

        using var operationCancellation =
            CancellationTokenSource.CreateLinkedTokenSource(
                _pageLifetime.Token);
        _operationCancellation = operationCancellation;
        SetBusy(true);
        HasResult = false;
        ResultTitle = IsCreateMode
            ? Localizer.Text("winui.huella.creando_huella")
            : Localizer.Text("winui.huella.comprobando_huella");
        ResultMessage =
            Localizer.Text("winui.huella.el_motor_local_esta_procesando_el");

        try
        {
            if (IsCreateMode)
            {
                await CreateHashAsync(
                    operations,
                    operationCancellation.Token);
            }
            else
            {
                await CheckHashAsync(
                    operations,
                    operationCancellation.Token);
            }
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            ShowLocalValidation(
                Localizer.Text("winui.comun.operacion_cancelada"),
                Localizer.Text("winui.comun.la_operacion_se_detuvo_antes_de_obtener"),
                InfoBarSeverity.Warning);
        }
        catch (Exception exception)
        {
            ShowLocalValidation(
                Localizer.Text("winui.comun.no_se_pudo_completar"),
                Localizer.Text("winui.huella.la_operacion_termino_sin_un_resultado_de"),
                InfoBarSeverity.Error);
            RequestDiagnostic(OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token));
        }
        finally
        {
            if (ReferenceEquals(
                _operationCancellation,
                operationCancellation))
            {
                _operationCancellation = null;
            }
            SetBusy(false);
        }
    }

    public void CancelCurrentOperation() =>
        _operationCancellation?.Cancel();

    private async Task CreateHashAsync(
        DesktopOperationsClient operations,
        CancellationToken cancellationToken)
    {
        var result = await operations.CreateHashAsync(
            new HashCreateParameters
            {
                InputPath = _inputPath!,
                OutputPath = string.Empty,
                Algorithm = SelectedAlgorithm,
                Format = SelectedFormat.Value,
                Recursive = IsDirectoryMode && IsRecursive,
            },
            cancellationToken);
        if (!result.IsSuccess || result.Outcome != "success")
        {
            ShowBackendFailure(result.SafeUserMessage);
            RequestDiagnostic(OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (!IsCoherentCreateResult(result.Data))
        {
            ShowIncoherentResult();
            return;
        }

        PresentCreateResult(result.Data!);
    }

    private async Task CheckHashAsync(
        DesktopOperationsClient operations,
        CancellationToken cancellationToken)
    {
        if (_manifestPath is null || !IsAllowedManifest(_manifestPath))
        {
            ShowLocalValidation(
                Localizer.Text("winui.huella.falta_el_manifiesto"),
                Localizer.Text("winui.huella.seleccione_el_fichero_de_huella_o"),
                InfoBarSeverity.Warning);
            return;
        }

        var result = await operations.CheckHashAsync(
            new HashCheckParameters
            {
                InputPath = _inputPath!,
                HashPath = _manifestPath,
                OutputPath = null,
                Algorithm = SelectedAlgorithm,
                Recursive = IsDirectoryMode ? IsRecursive : null,
                SaveReportToDisk = IsDirectoryMode,
            },
            cancellationToken);
        if (!result.IsSuccess || result.Outcome != "success")
        {
            ShowBackendFailure(result.SafeUserMessage);
            RequestDiagnostic(OperationDiagnosticMapper.FromResult(result));
            return;
        }
        if (!IsCoherentCheckResult(result.Data))
        {
            ShowIncoherentResult();
            return;
        }

        PresentCheckResult(result.Data!);
    }

    private void PresentCreateResult(HashCreateResult data)
    {
        HasResult = true;
        ResultSeverity = InfoBarSeverity.Success;
        ResultTitle = Localizer.Text("winui.huella.huella_creada");
        ResultMessage =
            Localizer.Text("winui.huella.la_huella_se_ha_creado_y_guardado");
        var outputName = DisplayPathName(
            data.DisplayOutputPath,
            directory: false);
        ResultSummary = Localizer.Format(
            "winui.huella.algoritmo_formato_salida",
            data.Algorithm, Localizer.Text(FormatLabel(data.Format)), outputName);

        if (IsDirectoryMode)
        {
            ResultItems =
            [
                Localizer.Format("winui.huella.entradas_incluidas",
                    data.Entries.GetValueOrDefault()),
                data.Recursive == true
                    ? Localizer.Text("winui.huella.se_incluyeron_subdirectorios")
                    : Localizer.Text("winui.huella.no_se_incluyeron_subdirectorios"),
            ];
        }
        else
        {
            ResultItems =
            [
                Localizer.Format("winui.huella.huella",
                    SafeIpcText.Clean(data.Hash, 300,
                        Localizer.Text("winui.huella.no_disponible"))),
            ];
        }
    }

    private void PresentCheckResult(HashCheckResult data)
    {
        HasResult = true;
        ResultSeverity = data.IsValid
            ? InfoBarSeverity.Success
            : InfoBarSeverity.Error;
        ResultTitle = data.IsValid
            ? Localizer.Text("winui.huella.la_huella_coincide")
            : Localizer.Text("winui.huella.la_huella_no_coincide");
        ResultMessage = data.IsValid
            ? Localizer.Text("winui.huella.el_contenido_comprobado_coincide_con_la")
            : Localizer.Text("winui.huella.el_contenido_ha_cambiado_falta");
        ResultSummary = Localizer.Format(
            "winui.huella.algoritmo_detectado_formato",
            data.Algorithm, Localizer.Text(FormatLabel(data.Format)));

        var items = new List<string>();
        if (!IsDirectoryMode)
        {
            items.Add(
                Localizer.Format("winui.huella.esperada",
                    SafeIpcText.Clean(data.ExpectedHash, 300,
                        Localizer.Text("winui.huella.no_disponible"))));
            items.Add(
                Localizer.Format("winui.huella.calculada",
                    SafeIpcText.Clean(data.ActualHash, 300,
                        Localizer.Text("winui.huella.no_disponible"))));
        }
        else
        {
            AddResultItems(items, Localizer.Text("winui.huella.coincide"), data.VisibleMatchingHash);
            AddResultItems(
                items,
                Localizer.Text("winui.huella.no_coincide"),
                data.VisibleNotMatchingHash);
            AddResultItems(
                items,
                Localizer.Text("winui.huella.sin_fichero"),
                data.VisibleHashWithoutFile);
            AddResultItems(
                items,
                Localizer.Text("winui.huella.sin_huella"),
                data.VisibleFileWithoutHash);
            if (!string.IsNullOrWhiteSpace(data.DisplayReportOutputPath))
            {
                items.Add(
                    Localizer.Format("winui.huella.informe_de_comprobacion", DisplayPathName(data.DisplayReportOutputPath, directory: false)));
            }
        }

        ResultItems = items
            .Take(MaximumVisibleResults)
            .ToArray();
    }

    private void OnAvailabilityChanged(object? sender, EventArgs args) =>
        RefreshAvailability();

    private void RefreshAvailability()
    {
        var available =
            _isActive &&
            _session.Supports(DesktopOperationActions.HashCreate) &&
            _session.Supports(DesktopOperationActions.HashCheck);
        SetOperationAvailability(
            available,
            Localizer.Text("winui.huella.motor_local_listo_para_crear_y_comprobar"));
        if (!available)
        {
            _operationCancellation?.Cancel();
        }
        RefreshCommandState();
    }

    private void SetBusy(bool value)
    {
        IsBusy = value;
        RefreshCommandState();
    }

    private void RefreshCommandState()
    {
        CanSelect =
            _isActive &&
            IsOperationConnected &&
            !IsBusy;
        CanExecute =
            CanSelect &&
            !string.IsNullOrWhiteSpace(_inputPath) &&
            (IsCreateMode ||
                (!string.IsNullOrWhiteSpace(_manifestPath) &&
                    IsAllowedManifest(_manifestPath)));
        CanSetRecursive = CanSelect && IsDirectoryMode;
        CanCancel = _isActive && IsBusy;
    }

    private void ResetResult()
    {
        HasResult = false;
        ResultSeverity = InfoBarSeverity.Informational;
        ResultTitle = Localizer.Text("winui.comun.sin_resultado");
        ResultMessage =
            Localizer.Text("winui.huella.seleccione_una_operacion_y_un_origen");
        ResultSummary = Localizer.Text("winui.huella.sin_evidencias_del_motor_local");
        ResultItems = [];
    }

    private void ShowBackendFailure(string message) =>
        ShowLocalValidation(
            Localizer.Text("winui.comun.no_se_pudo_completar"),
            message,
            InfoBarSeverity.Error);

    private void ShowIncoherentResult()
    {
        ShowLocalValidation(
            Localizer.Text("winui.comun.resultado_no_utilizable"),
            Localizer.Text("winui.huella.el_motor_local_no_devolvio_datos_de"),
            InfoBarSeverity.Error);
        RequestDiagnostic(OperationDiagnosticMapper.FromException(
            new InvalidOperationException()));
    }

    private void ShowLocalValidation(
        string title,
        string message,
        InfoBarSeverity severity = InfoBarSeverity.Error)
    {
        HasResult = true;
        ResultTitle = title;
        ResultMessage = message;
        ResultSummary = Localizer.Text("winui.huella.revise_los_datos_seleccionados_antes_de");
        ResultItems = [];
        ResultSeverity = severity;
    }

    private void RequestDiagnostic(OperationDiagnostic diagnostic) =>
        DiagnosticRequested?.Invoke(diagnostic);

    private bool IsAllowedManifest(string path)
    {
        var extension = Path.GetExtension(path);
        return (IsDirectoryMode
                ? DirectoryManifestExtensions
                : FileManifestExtensions)
            .Contains(extension);
    }

    private bool IsCoherentCreateResult(HashCreateResult? data) =>
        data is not null &&
        Algorithms.Contains(data.Algorithm, StringComparer.Ordinal) &&
        AvailableFormats.Any(
            format => string.Equals(
                format.Value,
                data.Format,
                StringComparison.OrdinalIgnoreCase)) &&
        !string.IsNullOrWhiteSpace(data.DisplayOutputPath) &&
        HasNonEmptyOutput(data.OutputPath) &&
        (IsDirectoryMode
            ? data.Entries is >= 0
            : !string.IsNullOrWhiteSpace(data.Hash));

    private static bool HasNonEmptyOutput(string? path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return false;
        }

        try
        {
            return new FileInfo(path).Length > 0;
        }
        catch
        {
            return false;
        }
    }

    private bool IsCoherentCheckResult(HashCheckResult? data) =>
        data is not null &&
        Algorithms.Contains(data.Algorithm, StringComparer.Ordinal) &&
        (IsDirectoryMode ||
            (!string.IsNullOrWhiteSpace(data.ExpectedHash) &&
                !string.IsNullOrWhiteSpace(data.ActualHash)));

    private static void AddResultItems(
        ICollection<string> destination,
        string label,
        IEnumerable<string> source)
    {
        if (destination.Count >= MaximumVisibleResults)
        {
            return;
        }

        foreach (var item in source.Take(MaximumVisibleResults))
        {
            destination.Add(
                Localizer.Format("{0}: {1}", Localizer.Text(label),
                    SafeIpcText.Clean(item, 300,
                        Localizer.Text("winui.huella.elemento_sin_nombre"))));
            if (destination.Count >= MaximumVisibleResults)
            {
                return;
            }
        }
    }

    private static string FormatLabel(string? format) =>
        format?.Trim().ToLowerInvariant() switch
        {
            "hex" => Localizer.Text("winui.huella.hexadecimal"),
            "base64" => "Base64",
            "bin" => Localizer.Text("winui.huella.binario"),
            "xml" or "hashfiles" => Localizer.Text("winui.huella.xml_grxfirma"),
            "txt" or "txthashfiles" => Localizer.Text("winui.huella.texto_grxfirma"),
            "csv" => "CSV",
            _ => Localizer.Text("winui.comun.no_determinado"),
        };

    private static string DisplayPathName(
        string path,
        bool directory)
    {
        var trimmed = path.TrimEnd(
            Path.DirectorySeparatorChar,
            Path.AltDirectorySeparatorChar);
        return SafeIpcText.Clean(
            Path.GetFileName(trimmed),
            256,
            Localizer.Text(directory
                ? "winui.huella.directorio_seleccionado" : "winui.comun.fichero_seleccionado"));
    }

    public sealed record HashFormatOption(
        string SourceLabel,
        string Value)
    {
        public string Label => Localizer.Text(SourceLabel);
        // El lector de pantalla anuncia ToString(): nunca el volcado del record.
        public override string ToString() => Label;
    }
}
