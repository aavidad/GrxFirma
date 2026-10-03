// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.ViewModels;

public enum CertificateCardStatus
{
    Valid,
    Warning,
    Invalid,
    Unusable,
}

public sealed record CertificateListItem
{
    public const int FnmtRenewalWindowDays = 60;

    public required string Id { get; init; }
    public required string DisplayName { get; init; }
    public override string ToString() => DisplayName;
    public required string IssuerDisplay { get; init; }
    public required string StatusDisplay { get; init; }
    public required string DefaultDisplay { get; init; }
    public bool IsDefaultCertificate { get; init; }
    public required string SearchText { get; init; }
    public required CertificateInfo SourceCertificate { get; init; }
    public required bool CanSign { get; init; }
    public bool NeedsUnlock { get; init; }
    public required bool IsTemporary { get; init; }
    public required string TemporaryDisplay { get; init; }

    // Resumen de aptitud y ficha completa, como en la aplicación Linux.
    public string SuitabilitySummary { get; init; } = string.Empty;
    public bool IsSuitable { get; init; }
    public IReadOnlyList<CertificateDetailRow> Details { get; init; } = [];
    public bool CanRenewAtFnmt { get; init; }
    public string RenewalMessage { get; init; } = string.Empty;
    public string CardStatusText { get; init; } = string.Empty;
    public string StatusReason { get; init; } = string.Empty;
    public CertificateCardStatus CardStatus { get; init; }
    public string ExpirationDisplay { get; init; } = string.Empty;
    public string ExpirationDateDisplay { get; init; } = string.Empty;
    public int DaysUntilExpiration { get; init; }
    public bool IsExpired { get; init; }
    public bool IsExpiringSoon { get; init; }
    public string ShortIssuerDisplay { get; init; } = string.Empty;
    public IReadOnlyList<CertificateDetailRow> CompactDetails { get; init; } = [];

    public static CertificateListItem From(
        CertificateInfo certificate,
        bool isTemporary = false)
    {
        ArgumentNullException.ThrowIfNull(certificate);

        var displayName = FirstVisible(
            certificate.SubjectName,
            certificate.Subject,
            "Certificado sin titular");
        var issuer = FirstVisible(
            certificate.IssuerName,
            certificate.Issuer,
            "Emisor desconocido");
        var status = certificate.IsExpired
            ? "Caducado"
            : FirstVisible(
                certificate.Status,
                certificate.CanSign ? "Disponible para firmar" : "Solo consulta");

        var engineStatus = certificate.Status.Trim();
        var expired = certificate.IsExpired ||
            engineStatus.Equals("Caducado", StringComparison.OrdinalIgnoreCase);
        var engineRejected = engineStatus.Length > 0 &&
            !engineStatus.Equals("Válido", StringComparison.OrdinalIgnoreCase) &&
            !engineStatus.Equals("Valido", StringComparison.OrdinalIgnoreCase) &&
            !engineStatus.Equals("Disponible para firmar", StringComparison.OrdinalIgnoreCase);
        var suitable = certificate.CanSign && !expired &&
            !certificate.NeedsUnlock && !engineRejected;
        var reason = expired
            ? $"Caducado el {FormatDate(FirstVisible(certificate.ValidTo, certificate.NotAfter, string.Empty), "dd/MM/yyyy")}"
            : certificate.NeedsUnlock
                ? "Requiere autorización de la tarjeta"
                : engineRejected
                    ? engineStatus
                    : !certificate.CanSign
                        ? "No dispone de clave utilizable para firmar"
                        : string.Empty;
        var summary = !suitable
            ? $"No válido para firmar. {reason}"
            : certificate.DaysUntilExpiration <= FnmtRenewalWindowDays
                ? "Válido para firmar, pero caduca pronto."
                : "Válido y apto para firmar.";

        // La FNMT permite renovar el certificado de ciudadano con el propio
        // certificado en los 60 días previos a su caducidad.
        var renewable = !expired &&
            certificate.DaysUntilExpiration is >= 0 and <= FnmtRenewalWindowDays &&
            string.Equals(certificate.Type, "fisica", StringComparison.Ordinal) &&
            issuer.Contains("FNMT", StringComparison.OrdinalIgnoreCase);

        var details = BuildDetails(certificate, displayName, issuer, status);
        var rawValidUntil = FirstVisible(
            certificate.ValidTo,
            certificate.NotAfter,
            string.Empty);
        var validUntil = FormatDate(rawValidUntil);
        var expirationDate = FormatDate(rawValidUntil, "dd/MM/yyyy");
        var cardStatus = expired
            ? CertificateCardStatus.Invalid
            : !suitable
                ? CertificateCardStatus.Unusable
                : certificate.DaysUntilExpiration is >= 0 and <= FnmtRenewalWindowDays
                        ? CertificateCardStatus.Warning
                        : CertificateCardStatus.Valid;

        return new CertificateListItem
        {
            CanRenewAtFnmt = renewable,
            RenewalMessage = renewable
                ? $"Caduca en {certificate.DaysUntilExpiration} día(s). La FNMT permite renovarlo durante los 60 días previos a la caducidad, si no está revocado: instale su Configurador, solicite la renovación identificándose con este certificado y descargue el nuevo con el código que le enviarán por correo. Solo se puede renovar así una vez; después hay que volver a acreditar la identidad."
                : string.Empty,
            SuitabilitySummary = summary,
            IsSuitable = suitable && !certificate.NeedsUnlock,
            NeedsUnlock = certificate.NeedsUnlock,
            Details = details,
            CompactDetails = BuildCompactDetails(details),
            CardStatus = cardStatus,
            StatusReason = reason,
            CardStatusText = !suitable
                ? "⚠ No válido"
                : certificate.DaysUntilExpiration is >= 0 and <= FnmtRenewalWindowDays
                    ? "Caduca pronto"
                    : "Válido",
            ExpirationDisplay = $"Vence: {expirationDate}",
            ExpirationDateDisplay = expirationDate,
            DaysUntilExpiration = certificate.DaysUntilExpiration,
            IsExpired = expired,
            IsExpiringSoon = !expired &&
                certificate.DaysUntilExpiration is >= 0 and <= FnmtRenewalWindowDays,
            ShortIssuerDisplay = ShortIssuer(issuer),
            Id = certificate.Id,
            DisplayName = displayName,
            IssuerDisplay = $"Emisor: {issuer}",
            StatusDisplay = status,
            DefaultDisplay = string.Empty,
            SearchText = string.Join(
                "\n",
                displayName,
                issuer,
                certificate.Nif,
                certificate.Organization,
                certificate.Type),
            SourceCertificate = certificate,
            CanSign = suitable,
            IsTemporary = isTemporary,
            TemporaryDisplay = isTemporary
                ? "Solo durante esta sesión"
                : "Almacén del sistema",
        };
    }

    private static IReadOnlyList<CertificateDetailRow> BuildDetails(
        CertificateInfo certificate,
        string holder,
        string issuer,
        string status)
    {
        var rows = new List<CertificateDetailRow>
        {
            new("Titular", holder),
        };
        AddIfPresent(rows, "NIF / identificador", certificate.Nif);
        AddIfPresent(rows, "Organización", certificate.Organization);
        rows.Add(new("Tipo", TypeLabel(certificate.Type)));
        rows.Add(new("Emisor", issuer));
        AddIfPresent(rows, "Número de serie", certificate.SerialNumber);
        rows.Add(new("Estado", status));
        rows.Add(new(
            "Válido hasta",
            FormatDate(FirstVisible(
                certificate.ValidTo,
                certificate.NotAfter,
                "Desconocido"))));
        rows.Add(new(
            certificate.IsExpired ? "Días caducado" : "Días restantes",
            Math.Abs(certificate.DaysUntilExpiration).ToString(
                System.Globalization.CultureInfo.CurrentCulture)));
        AddIfPresent(
            rows,
            "Huella SHA-256",
            FormatFingerprint(certificate.Fingerprint));
        return rows;
    }

    private static IReadOnlyList<CertificateDetailRow> BuildCompactDetails(
        IReadOnlyList<CertificateDetailRow> details)
    {
        var rows = new List<CertificateDetailRow>();
        foreach (var label in new[]
        {
            "Titular", "Emisor", "Válido hasta", "Huella SHA-256",
        })
        {
            var row = details.FirstOrDefault(item => item.Label == label);
            if (row is not null)
            {
                rows.Add(row);
            }
        }
        var identifier = details.FirstOrDefault(item =>
            item.Label == "Número de serie") ?? details.FirstOrDefault(item =>
                item.Label == "NIF / identificador");
        if (identifier is not null)
        {
            rows.Insert(Math.Min(2, rows.Count),
                new CertificateDetailRow("Nº Serie / NIF", identifier.Value));
        }
        return rows;
    }

    private static void AddIfPresent(
        List<CertificateDetailRow> rows,
        string label,
        string? value)
    {
        if (!string.IsNullOrWhiteSpace(value))
        {
            rows.Add(new(label, value.Trim()));
        }
    }

    private static string TypeLabel(string? type) =>
        type switch
        {
            "fisica" => "Persona física",
            "representacion" => "Representante de entidad",
            "sello" => "Sello de entidad",
            "empleado_publico" => "Empleado público",
            _ => "No identificado",
        };

    private static string FormatDate(
        string raw,
        string format = "dd/MM/yyyy HH:mm") =>
        DateTimeOffset.TryParse(
            raw,
            System.Globalization.CultureInfo.InvariantCulture,
            System.Globalization.DateTimeStyles.AssumeUniversal,
            out var date)
            ? date.ToLocalTime().ToString(
                format,
                System.Globalization.CultureInfo.CurrentCulture)
            : "Fecha desconocida";

    private static string ShortIssuer(string issuer)
    {
        var commonName = issuer.Split(',')
            .Select(part => part.Trim())
            .FirstOrDefault(part => part.StartsWith("CN=", StringComparison.OrdinalIgnoreCase));
        return commonName is null ? issuer : commonName[3..].Trim();
    }

    // Agrupa la huella en bloques de 4 para leerla y compararla a simple vista.
    private static string FormatFingerprint(string? raw)
    {
        var hex = new string((raw ?? string.Empty)
            .Where(Uri.IsHexDigit)
            .Select(char.ToUpperInvariant)
            .ToArray());
        if (hex.Length < 16)
        {
            return raw?.Trim() ?? string.Empty;
        }
        return string.Join(
            " ",
            Enumerable.Range(0, (hex.Length + 3) / 4)
                .Select(i => hex.Substring(i * 4, Math.Min(4, hex.Length - i * 4))));
    }

    private static string FirstVisible(
        string? first,
        string? second,
        string fallback = "") =>
        !string.IsNullOrWhiteSpace(first)
            ? first
            : !string.IsNullOrWhiteSpace(second)
                ? second
                : fallback;
}

public sealed record CertificateDetailRow(string Label, string Value);

public sealed record CertificateImportOption(
    string Label,
    string Detail,
    bool IsTemporary,
    string TargetId);

public sealed class CertificatesPageViewModel
    : WorkspacePageViewModel
{
    private const int MaximumCatalogItems = 512;
    private const string WindowsCertificateManagerId =
        "windows-certmgr";

    private readonly DesktopOperationSession _session;
    private readonly IFilePickerService _filePicker;
    private IReadOnlyList<CertificateListItem> _allCertificates = [];
    private IReadOnlyList<CertificateListItem> _visibleCertificates = [];
    private IReadOnlyList<CertificateImportOption> _importOptions = [];
    private CertificateListItem? _selectedCertificate;
    private CertificateImportOption? _selectedImportOption;
    private CertificateManagerInfo? _windowsCertificateManager;
    private CancellationTokenSource? _activeCancellation;
    private string? _selectedCredentialPath;
    private string? _defaultCertificateId;
    private string _filterText = string.Empty;
    private bool _requireNif;
    private bool _requireOrganization;
    private bool _filterPersonal;
    private bool _filterRepresentative;
    private bool _filterSeal;
    private bool _filterPublicEmployee;
    private string _selectedCredentialDisplayName =
        "Ninguna credencial seleccionada";
    private string _catalogMessage =
        "Conecte el motor local y actualice para consultar el almacén.";
    private string _actionStatusTitle = "Sin comprobaciones";
    private string _actionStatusMessage =
        "Seleccione un certificado para comprobar online su estado.";
    private bool _isBusy;
    private bool _canRefresh;
    private bool _canSetSelectedAsDefault;
    private bool _canClearDefaultCertificate;
    private bool _canValidateSelectedCertificate;
    private bool _canOpenCertificateManager;
    private bool _canSelectCredential;
    private bool _canImportCredential;
    private bool _canRemoveSelectedTemporary;
    private bool _canClearTemporaryCertificates;
    private bool _hasVisibleCertificates;
    private bool _hasDefaultCertificate;
    private InfoBarSeverity _actionStatusSeverity =
        InfoBarSeverity.Informational;
    private int _operationInProgress;

    public CertificatesPageViewModel(
        DesktopOperationSession session,
        IFilePickerService filePicker)
        : base(
            "Certificados",
            "Consulta, filtra y comprueba los certificados publicados por el almacén de Windows.",
            "El almacén no está disponible porque el motor local no ha publicado la operación certificates.")
    {
        ArgumentNullException.ThrowIfNull(session);
        ArgumentNullException.ThrowIfNull(filePicker);
        _session = session;
        _filePicker = filePicker;
        UpdateAvailability();
    }

    public bool FilterPersonal
    {
        get => _filterPersonal;
        set { if (SetProperty(ref _filterPersonal, value)) ApplyFilter(); }
    }

    public bool FilterRepresentative
    {
        get => _filterRepresentative;
        set { if (SetProperty(ref _filterRepresentative, value)) ApplyFilter(); }
    }

    public bool FilterSeal
    {
        get => _filterSeal;
        set { if (SetProperty(ref _filterSeal, value)) ApplyFilter(); }
    }

    public bool FilterPublicEmployee
    {
        get => _filterPublicEmployee;
        set { if (SetProperty(ref _filterPublicEmployee, value)) ApplyFilter(); }
    }

    public bool RequireNif
    {
        get => _requireNif;
        set { if (SetProperty(ref _requireNif, value)) ApplyFilter(); }
    }

    public bool RequireOrganization
    {
        get => _requireOrganization;
        set { if (SetProperty(ref _requireOrganization, value)) ApplyFilter(); }
    }

    public IReadOnlyList<CertificateListItem> VisibleCertificates
    {
        get => _visibleCertificates;
        private set => SetProperty(ref _visibleCertificates, value);
    }

    public string VisibleCountText =>
        $"Mostrando {VisibleCertificates.Count} de {_allCertificates.Count}";

    public CertificateListItem? SelectedCertificate
    {
        get => _selectedCertificate;
        set
        {
            if (SetProperty(ref _selectedCertificate, value))
            {
                RaisePropertyChanged(nameof(HasSelectedCertificate));
                RaisePropertyChanged(nameof(ShowCertificateDetailHint));
                RaisePropertyChanged(nameof(SelectedCertificateTitle));
                RaisePropertyChanged(nameof(SelectedCertificateSummary));
                RaisePropertyChanged(nameof(SelectedCertificateDetails));
                RaisePropertyChanged(nameof(SelectedCertificateCompactDetails));
                RaisePropertyChanged(nameof(CanRenewSelectedCertificate));
                RaisePropertyChanged(nameof(SelectedCertificateRenewalMessage));
                RaisePropertyChanged(nameof(IsExpiryNoticeOpen));
                RaisePropertyChanged(nameof(ExpiryNoticeMessage));
                RaisePropertyChanged(nameof(ExpiryNoticeSeverity));
                RaisePropertyChanged(nameof(CanRenewExpiryNoticeCertificate));
                UpdateCommandStates();
            }
        }
    }

    public bool HasSelectedCertificate => _selectedCertificate is not null;

    public bool ShowCertificateDetailHint =>
        _selectedCertificate is null && HasVisibleCertificates;

    public string SelectedCertificateTitle =>
        _selectedCertificate?.DisplayName ?? string.Empty;

    public string SelectedCertificateSummary =>
        _selectedCertificate?.SuitabilitySummary ?? string.Empty;

    public IReadOnlyList<CertificateDetailRow> SelectedCertificateDetails =>
        _selectedCertificate?.Details ?? [];

    public IReadOnlyList<CertificateDetailRow> SelectedCertificateCompactDetails =>
        _selectedCertificate?.CompactDetails ?? [];

    public void ReportRenewalLaunch(bool succeeded, string message) =>
        SetActionStatus(
            succeeded ? "Renovación en la FNMT" : "No se pudo abrir la FNMT",
            message,
            succeeded ? InfoBarSeverity.Informational : InfoBarSeverity.Warning);

    public void ReportExternalValidationLaunch(bool succeeded, string message) =>
        SetActionStatus(
            succeeded ? "VALIDe abierto" : "No se pudo abrir VALIDe",
            message,
            succeeded ? InfoBarSeverity.Informational : InfoBarSeverity.Warning);

    public bool CanRenewSelectedCertificate =>
        _selectedCertificate?.CanRenewAtFnmt == true;

    public string SelectedCertificateRenewalMessage =>
        _selectedCertificate?.RenewalMessage ?? string.Empty;

    private CertificateListItem? ExpiryNoticeCertificate
    {
        get
        {
            var fallback = _allCertificates.FirstOrDefault(item =>
                string.Equals(item.Id, _defaultCertificateId, StringComparison.Ordinal));
            if (_selectedCertificate?.IsExpired == true)
            {
                return _selectedCertificate;
            }
            if (fallback?.IsExpired == true)
            {
                return fallback;
            }
            return _selectedCertificate?.IsExpiringSoon == true
                ? _selectedCertificate
                : fallback?.IsExpiringSoon == true ? fallback : null;
        }
    }

    public bool IsExpiryNoticeOpen => ExpiryNoticeCertificate is not null;

    public bool CanRenewExpiryNoticeCertificate =>
        ExpiryNoticeCertificate?.CanRenewAtFnmt == true;

    public InfoBarSeverity ExpiryNoticeSeverity =>
        ExpiryNoticeCertificate?.IsExpired == true
            ? InfoBarSeverity.Error
            : InfoBarSeverity.Warning;

    public string ExpiryNoticeMessage
    {
        get
        {
            var certificate = ExpiryNoticeCertificate;
            if (certificate is null)
            {
                return string.Empty;
            }
            var isSelected = string.Equals(
                certificate.Id,
                _selectedCertificate?.Id,
                StringComparison.Ordinal);
            if (certificate.IsExpired)
            {
                var subject = isSelected
                    ? "Este certificado"
                    : "Su certificado predeterminado";
                return certificate.ExpirationDateDisplay == "Fecha desconocida"
                    ? $"{subject} ha caducado y no puede usarse para firmar."
                    : $"{subject} caducó el {certificate.ExpirationDateDisplay} y no puede usarse para firmar.";
            }
            if (!certificate.IsExpiringSoon)
            {
                return string.Empty;
            }
            var remaining = certificate.DaysUntilExpiration.ToString(
                System.Globalization.CultureInfo.CurrentCulture);
            var renewal = certificate.CanRenewAtFnmt
                ? "Puede renovarlo ahora."
                : "Contacte con su emisor para renovarlo.";
            var owner = isSelected
                ? "Su certificado"
                : "Su certificado predeterminado";
            return $"{owner} caduca el {certificate.ExpirationDateDisplay} (quedan {remaining} días). {renewal}";
        }
    }

    public IReadOnlyList<CertificateImportOption> ImportOptions
    {
        get => _importOptions;
        private set => SetProperty(ref _importOptions, value);
    }

    public CertificateImportOption? SelectedImportOption
    {
        get => _selectedImportOption;
        set
        {
            if (SetProperty(ref _selectedImportOption, value))
            {
                UpdateCommandStates();
            }
        }
    }

    public string SelectedCredentialDisplayName
    {
        get => _selectedCredentialDisplayName;
        private set => SetProperty(
            ref _selectedCredentialDisplayName,
            value);
    }

    public string FilterText
    {
        get => _filterText;
        set
        {
            if (SetProperty(ref _filterText, value ?? string.Empty))
            {
                ApplyFilter();
            }
        }
    }

    public string CatalogMessage
    {
        get => _catalogMessage;
        private set => SetProperty(ref _catalogMessage, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanRefresh
    {
        get => _canRefresh;
        private set => SetProperty(ref _canRefresh, value);
    }

    public bool CanValidateSelectedCertificate
    {
        get => _canValidateSelectedCertificate;
        private set => SetProperty(
            ref _canValidateSelectedCertificate,
            value);
    }

    public bool CanSetSelectedAsDefault
    {
        get => _canSetSelectedAsDefault;
        private set => SetProperty(
            ref _canSetSelectedAsDefault,
            value);
    }

    public bool CanClearDefaultCertificate
    {
        get => _canClearDefaultCertificate;
        private set => SetProperty(
            ref _canClearDefaultCertificate,
            value);
    }

    public bool CanOpenCertificateManager
    {
        get => _canOpenCertificateManager;
        private set => SetProperty(
            ref _canOpenCertificateManager,
            value);
    }

    public bool CanSelectCredential
    {
        get => _canSelectCredential;
        private set => SetProperty(ref _canSelectCredential, value);
    }

    public bool CanImportCredential
    {
        get => _canImportCredential;
        private set => SetProperty(ref _canImportCredential, value);
    }

    public bool CanRemoveSelectedTemporary
    {
        get => _canRemoveSelectedTemporary;
        private set => SetProperty(
            ref _canRemoveSelectedTemporary,
            value);
    }

    public bool CanClearTemporaryCertificates
    {
        get => _canClearTemporaryCertificates;
        private set => SetProperty(
            ref _canClearTemporaryCertificates,
            value);
    }

    public string ActionStatusTitle
    {
        get => _actionStatusTitle;
        private set => SetProperty(ref _actionStatusTitle, value);
    }

    public string ActionStatusMessage
    {
        get => _actionStatusMessage;
        private set => SetProperty(ref _actionStatusMessage, value);
    }

    public InfoBarSeverity ActionStatusSeverity
    {
        get => _actionStatusSeverity;
        private set => SetProperty(ref _actionStatusSeverity, value);
    }

    public bool HasVisibleCertificates
    {
        get => _hasVisibleCertificates;
        private set
        {
            if (SetProperty(ref _hasVisibleCertificates, value))
            {
                RaisePropertyChanged(nameof(ShowCertificateDetailHint));
            }
        }
    }

    public bool IsCatalogEmpty => !HasVisibleCertificates;

    public void UpdateAvailability()
    {
        var available = _session.Supports(DesktopOperationActions.Certificates);
        SetOperationAvailability(
            available,
            "Motor local conectado. Puede consultar el almacén real de certificados.");
        if (!available)
        {
            CancelCurrentOperation();
            ReplaceCatalog([]);
            _windowsCertificateManager = null;
            ImportOptions = [];
            SelectedImportOption = null;
            ClearSelectedCredential();
            CatalogMessage =
                "El catálogo no está disponible mientras el motor local está desconectado.";
            SetActionStatus(
                "Motor local desconectado",
                "No se puede comprobar certificados ni abrir su gestor hasta recuperar la conexión.",
                InfoBarSeverity.Warning);
        }
        else if (!_session.Supports(
            DesktopOperationActions.ValidateCertificateOnline))
        {
            SetActionStatus(
                "Comprobación online no disponible",
                "El motor conectado no publica la comprobación online individual. Puede seguir consultando el almacén.",
                InfoBarSeverity.Warning);
        }
        if (!_session.Supports(
                DesktopOperationActions.CertificateAccessOptions) ||
            !_session.Supports(
                DesktopOperationActions.OpenCertificateManager))
        {
            _windowsCertificateManager = null;
        }
        if (!_session.Supports(
                DesktopOperationActions.CertificateAccessOptions))
        {
            ImportOptions = [];
            SelectedImportOption = null;
        }
        UpdateCommandStates();
    }

    public void CancelCurrentOperation()
    {
        try
        {
            Volatile.Read(ref _activeCancellation)?.Cancel();
        }
        catch (ObjectDisposedException)
        {
            // La operación ya terminó y liberó su token.
        }
    }

    public async Task<OperationDiagnostic?> RefreshAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            CatalogMessage =
                "Ya se está actualizando el catálogo de certificados.";
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.Certificates,
                out var operations))
            {
                CatalogMessage =
                    "No se puede consultar el almacén: el motor local no ofrece la operación certificates.";
                return null;
            }

            CatalogMessage = "Consultando el almacén de Windows…";
            var result = await operations.GetCertificatesAsync(
                operationCancellation.Token);
            if (!result.IsSuccess ||
                !string.Equals(
                    result.Outcome,
                    "success",
                    StringComparison.Ordinal))
            {
                CatalogMessage =
                    "El motor local no pudo completar la consulta del almacén.";
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null)
            {
                CatalogMessage =
                    "El motor confirmó la consulta, pero no devolvió un catálogo válido.";
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_CERTIFICATE_CATALOG");
            }

            var previousId = SelectedCertificate?.Id;
            var catalog = result.Data
                .Where(certificate =>
                    certificate is not null &&
                    !string.IsNullOrWhiteSpace(certificate.Id))
                .GroupBy(certificate =>
                    certificate.Id,
                    StringComparer.Ordinal)
                .Select(group => CertificateListItem.From(
                    group.First(),
                    _session.IsTemporaryCertificateTracked(group.Key)))
                .OrderBy(item => item.CanSign ? 0 : 1)
                .Take(MaximumCatalogItems)
                .ToArray();
            ReplaceCatalog(catalog, previousId);
            CatalogMessage = catalog.Length == 0
                ? "El almacén se consultó correctamente, pero no contiene certificados disponibles."
                : catalog.Length == 1
                    ? "Se ha cargado 1 certificado del almacén real."
                    : $"Se han cargado {catalog.Length} certificados del almacén real.";

            var managerDiagnostic =
                await RefreshCertificateManagerAsync(
                    operations,
                    operationCancellation.Token);
            var defaultDiagnostic =
                await RefreshDefaultCertificateAsync(
                    operations,
                    operationCancellation.Token);
            return managerDiagnostic ?? defaultDiagnostic;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException exception)
        {
            CatalogMessage = "La actualización del catálogo se canceló.";
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            CatalogMessage =
                "Falló la comunicación segura al consultar el almacén.";
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            CatalogMessage =
                "La aplicación no pudo completar la consulta del almacén.";
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?> SelectImportCredentialAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                "Operación en curso",
                "Espere a que termine la operación actual.",
                InfoBarSeverity.Informational);
            return null;
        }

        try
        {
            var path = await _filePicker.PickOpenFileAsync(
                OpenFilePickerProfile.Certificate,
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(path))
            {
                SetActionStatus(
                    "Selección cancelada",
                    "No se ha cambiado la credencial preparada.",
                    InfoBarSeverity.Informational);
                return null;
            }
            try
            {
                DesktopCertificateCredentialFile.ValidateSelection(path);
            }
            catch (InvalidDataException exception)
            {
                ClearSelectedCredential();
                SetActionStatus(
                    "Credencial no válida",
                    exception.Message,
                    InfoBarSeverity.Warning);
                return null;
            }

            _selectedCredentialPath = path;
            SelectedCredentialDisplayName =
                DesktopCertificateCredentialFile.SafeDisplayName(path);
            SetActionStatus(
                "Credencial preparada",
                "Elija si quiere usarla solo durante esta sesión o importarla en un almacén persistente.",
                InfoBarSeverity.Informational);
            UpdateCommandStates();
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            return null;
        }
        catch (Exception exception)
        {
            ClearSelectedCredential();
            SetActionStatus(
                "No se pudo leer la selección",
                "La aplicación no pudo preparar la credencial elegida.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    /// <summary>
    /// Toma propiedad del buffer de contraseña y lo borra siempre. La
    /// credencial se lee en un buffer acotado y también se borra al terminar.
    /// </summary>
    public async Task<OperationDiagnostic?> ImportSelectedCredentialAsync(
        byte[] password,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(password);
        byte[]? credential = null;
        CancellationTokenSource? operationCancellation = null;
        try
        {
            var option = SelectedImportOption;
            var path = _selectedCredentialPath;
            if (option is null || string.IsNullOrWhiteSpace(path))
            {
                SetActionStatus(
                    "Faltan datos para importar",
                    "Seleccione una credencial y un destino antes de continuar.",
                    InfoBarSeverity.Warning);
                return null;
            }
            if (password.Length >
                DesktopOperationsClient.MaximumPasswordBytes)
            {
                SetActionStatus(
                    "Contraseña demasiado larga",
                    "La contraseña supera el límite de seguridad permitido.",
                    InfoBarSeverity.Warning);
                return null;
            }
            if (!TryBeginOperation(
                cancellationToken,
                out operationCancellation))
            {
                SetActionStatus(
                    "Operación en curso",
                    "Espere a que termine la operación actual.",
                    InfoBarSeverity.Informational);
                return null;
            }

            var action = option.IsTemporary
                ? DesktopOperationActions.UseTemporaryCertificate
                : DesktopOperationActions.ImportCertificateToStore;
            if (!_session.TryGetOperations(action, out var operations))
            {
                SetActionStatus(
                    "Importación no disponible",
                    "El motor local no publica la acción segura seleccionada.",
                    InfoBarSeverity.Warning);
                return null;
            }

            credential =
                await DesktopCertificateCredentialFile.ReadAsync(
                path,
                operationCancellation.Token);
            if (option.IsTemporary)
            {
                SetActionStatus(
                    "Cargando credencial temporal",
                    "La clave privada se conservará únicamente en memoria durante esta sesión.",
                    InfoBarSeverity.Informational);
                var result =
                    await operations.UseTemporaryCertificateAsync(
                        new UseTemporaryCertificateParameters
                        {
                            CredentialB64 = credential,
                            PasswordB64 = password,
                        },
                        operationCancellation.Token);
                if (!IsSuccessful(result))
                {
                    SetActionStatus(
                        "No se pudo cargar la credencial",
                        "No se ha añadido ninguna credencial temporal. Abra el diagnóstico para conocer la causa.",
                        InfoBarSeverity.Error);
                    return OperationDiagnosticMapper.FromResult(result);
                }
                if (result.Data is null ||
                    !result.Data.Temporary ||
                    string.IsNullOrWhiteSpace(result.Data.Id))
                {
                    SetActionStatus(
                        "Confirmación incompleta",
                        "El motor no confirmó una credencial temporal utilizable.",
                        InfoBarSeverity.Error);
                    return InvalidResultDiagnostic(
                        result,
                        "MISSING_TEMPORARY_CERTIFICATE");
                }

                _session.TrackTemporaryCertificate(result.Data.Id);
                var refreshDiagnostic =
                    await ReloadCatalogAfterMutationAsync(
                        operations,
                        result.Data.Id,
                        operationCancellation.Token);
                if (refreshDiagnostic is not null)
                {
                    return refreshDiagnostic;
                }
                SetActionStatus(
                    "Credencial temporal disponible",
                    "Puede firmar con ella durante esta sesión. No se ha instalado de forma persistente.",
                    InfoBarSeverity.Success);
            }
            else
            {
                SetActionStatus(
                    "Importando credencial",
                    $"Instalando la credencial en {option.Label}.",
                    InfoBarSeverity.Informational);
                var result =
                    await operations.ImportCertificateToStoreAsync(
                        new ImportCertificateToStoreParameters
                        {
                            CredentialB64 = credential,
                            PasswordB64 = password,
                            TargetId = option.TargetId,
                        },
                        operationCancellation.Token);
                if (!IsSuccessful(result))
                {
                    SetActionStatus(
                        "No se pudo importar la credencial",
                        "El almacén no confirmó la importación. Abra el diagnóstico para conocer la causa.",
                        InfoBarSeverity.Error);
                    return OperationDiagnosticMapper.FromResult(result);
                }
                if (string.IsNullOrWhiteSpace(result.Data))
                {
                    SetActionStatus(
                        "Confirmación incompleta",
                        "El motor no confirmó que la credencial se haya instalado.",
                        InfoBarSeverity.Error);
                    return InvalidResultDiagnostic(
                        result,
                        "MISSING_CERTIFICATE_IMPORT_CONFIRMATION");
                }

                var refreshDiagnostic =
                    await ReloadCatalogAfterMutationAsync(
                        operations,
                        null,
                        operationCancellation.Token);
                if (refreshDiagnostic is not null)
                {
                    return refreshDiagnostic;
                }
                SetActionStatus(
                    "Credencial importada",
                    $"El almacén {option.Label} confirmó la importación.",
                    InfoBarSeverity.Success);
            }

            ClearSelectedCredential();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            SetActionStatus(
                "Importación cancelada",
                "No se recibió confirmación de que la credencial se haya añadido.",
                InfoBarSeverity.Warning);
            return null;
        }
        catch (OperationCanceledException exception)
        {
            SetActionStatus(
                "Importación cancelada",
                "La operación terminó antes de recibir una confirmación.",
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation?.Token ?? cancellationToken);
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                "Falló la comunicación segura",
                "No se pudo completar la importación de la credencial.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                "No se pudo importar la credencial",
                "La aplicación no pudo leer o procesar la credencial seleccionada.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            if (credential is not null)
            {
                CryptographicOperations.ZeroMemory(credential);
            }
            CryptographicOperations.ZeroMemory(password);
            if (operationCancellation is not null)
            {
                EndOperation(operationCancellation);
            }
        }
    }

    public async Task<OperationDiagnostic?>
        RemoveSelectedTemporaryCertificateAsync(
            CancellationToken cancellationToken = default)
    {
        var certificate = SelectedCertificate;
        if (certificate is null ||
            !_session.IsTemporaryCertificateTracked(certificate.Id))
        {
            SetActionStatus(
                "Seleccione una credencial temporal",
                "Solo se pueden retirar desde aquí las credenciales cargadas para esta sesión.",
                InfoBarSeverity.Warning);
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.RemoveTemporaryCertificate,
                out var operations))
            {
                SetActionStatus(
                    "Retirada no disponible",
                    "El motor local no ofrece esta acción.",
                    InfoBarSeverity.Warning);
                return null;
            }
            var result =
                await operations.RemoveTemporaryCertificateAsync(
                    new RemoveTemporaryCertificateParameters
                    {
                        CertificateId = certificate.Id,
                    },
                    operationCancellation.Token);
            if (!IsSuccessful(result) ||
                string.IsNullOrWhiteSpace(result.Data))
            {
                SetActionStatus(
                    "No se pudo retirar la credencial",
                    "No se recibió confirmación de la retirada.",
                    InfoBarSeverity.Error);
                return !IsSuccessful(result)
                    ? OperationDiagnosticMapper.FromResult(result)
                    : InvalidResultDiagnostic(
                        result,
                        "MISSING_TEMPORARY_REMOVE_CONFIRMATION");
            }

            _session.UntrackTemporaryCertificate(certificate.Id);
            RemoveCertificateFromCatalog(certificate.Id);
            var refreshDiagnostic =
                await ReloadCatalogAfterMutationAsync(
                    operations,
                    null,
                    operationCancellation.Token);
            if (refreshDiagnostic is not null)
            {
                return refreshDiagnostic;
            }
            SetActionStatus(
                "Credencial temporal retirada",
                "La clave privada ya no está disponible en esta sesión.",
                InfoBarSeverity.Success);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            return null;
        }
        catch (Exception exception)
        {
            SetActionStatus(
                "No se pudo retirar la credencial",
                "La operación no terminó correctamente.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?>
        ClearTemporaryCertificatesAsync(
            CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.ClearTemporaryCertificates,
                out var operations))
            {
                SetActionStatus(
                    "Limpieza no disponible",
                    "El motor local no ofrece esta acción.",
                    InfoBarSeverity.Warning);
                return null;
            }
            var result =
                await operations.ClearTemporaryCertificatesAsync(
                    operationCancellation.Token);
            if (!IsSuccessful(result) ||
                string.IsNullOrWhiteSpace(result.Data))
            {
                SetActionStatus(
                    "No se pudieron retirar las credenciales",
                    "No se recibió confirmación de la limpieza.",
                    InfoBarSeverity.Error);
                return !IsSuccessful(result)
                    ? OperationDiagnosticMapper.FromResult(result)
                    : InvalidResultDiagnostic(
                        result,
                        "MISSING_TEMPORARY_CLEAR_CONFIRMATION");
            }

            RemoveTrackedTemporaryCertificatesFromCatalog();
            _session.ClearTrackedTemporaryCertificates();
            var refreshDiagnostic =
                await ReloadCatalogAfterMutationAsync(
                    operations,
                    null,
                    operationCancellation.Token);
            if (refreshDiagnostic is not null)
            {
                return refreshDiagnostic;
            }
            SetActionStatus(
                "Credenciales temporales retiradas",
                "La sesión ya no conserva claves privadas cargadas temporalmente.",
                InfoBarSeverity.Success);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            return null;
        }
        catch (Exception exception)
        {
            SetActionStatus(
                "No se pudieron retirar las credenciales",
                "La operación no terminó correctamente.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public OperationDiagnostic ReportImportCaptureFailure(
        Exception exception)
    {
        ArgumentNullException.ThrowIfNull(exception);
        SetActionStatus(
            "No se pudo leer la contraseña",
            "Windows no pudo capturar o codificar la contraseña de forma segura.",
            InfoBarSeverity.Error);
        return OperationDiagnosticMapper.FromException(exception);
    }

    public async Task<OperationDiagnostic?>
        SetSelectedAsDefaultAsync(
            CancellationToken cancellationToken = default)
    {
        var certificate = SelectedCertificate;
        if (certificate is null ||
            string.IsNullOrWhiteSpace(certificate.Id))
        {
            SetActionStatus(
                "Seleccione un certificado",
                "Elija un certificado del catálogo antes de establecerlo como predeterminado.",
                InfoBarSeverity.Warning);
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                "Operación en curso",
                "Espere a que termine la operación actual.",
                InfoBarSeverity.Informational);
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                    DesktopOperationActions.GetSettings,
                    out var operations) ||
                !_session.Supports(
                    DesktopOperationActions.SaveSettings))
            {
                SetActionStatus(
                    "Preferencia no disponible",
                    "El motor local no permite leer y guardar de forma segura el certificado predeterminado.",
                    InfoBarSeverity.Warning);
                return null;
            }

            SetActionStatus(
                "Guardando certificado predeterminado",
                "Leyendo primero las preferencias actuales para conservar el resto de opciones.",
                InfoBarSeverity.Informational);
            var settingsResult = await operations.GetSettingsAsync(
                operationCancellation.Token);
            if (!IsSuccessful(settingsResult))
            {
                SetActionStatus(
                    "No se pudieron leer las preferencias",
                    "No se ha modificado el certificado predeterminado. Abra el diagnóstico para conocer la causa.",
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(
                    settingsResult);
            }
            if (settingsResult.Data is null)
            {
                SetActionStatus(
                    "Preferencias incompletas",
                    "El motor no devolvió un documento utilizable y no se ha guardado ningún cambio.",
                    InfoBarSeverity.Error);
                return InvalidResultDiagnostic(
                    settingsResult,
                    "MISSING_SETTINGS_DOCUMENT");
            }

            var updatedSettings =
                settingsResult.Data.CreateSafeSaveSnapshot() with
                {
                    PreferDefaultCertificate = true,
                    DefaultCertificateId = certificate.Id,
                };
            var saveResult = await operations.SaveSettingsAsync(
                updatedSettings,
                operationCancellation.Token);
            if (!IsSuccessful(saveResult))
            {
                SetActionStatus(
                    "No se pudo guardar la preferencia",
                    "No se recibió confirmación del cambio. Abra el diagnóstico para conocer la causa.",
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(saveResult);
            }
            if (string.IsNullOrWhiteSpace(saveResult.Data))
            {
                SetActionStatus(
                    "Confirmación incompleta",
                    "El motor no confirmó el guardado y la interfaz no dará el cambio por aplicado.",
                    InfoBarSeverity.Error);
                return InvalidResultDiagnostic(
                    saveResult,
                    "MISSING_SETTINGS_SAVE_CONFIRMATION");
            }

            ApplyDefaultCertificate(certificate.Id);
            SetActionStatus(
                "Certificado predeterminado guardado",
                $"{certificate.DisplayName} se usará primero cuando la aplicación solicite un certificado.",
                InfoBarSeverity.Success);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            SetActionStatus(
                "Guardado cancelado",
                "No se recibió confirmación de que el certificado predeterminado haya cambiado.",
                InfoBarSeverity.Warning);
            return null;
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                "Falló la comunicación segura",
                "No se pudo guardar el certificado predeterminado.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                "No se pudo guardar la preferencia",
                "La aplicación no pudo completar el cambio.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?>
        ClearDefaultCertificateAsync(
            CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                "Operación en curso",
                "Espere a que termine la operación actual.",
                InfoBarSeverity.Informational);
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                    DesktopOperationActions.GetSettings,
                    out var operations) ||
                !_session.Supports(
                    DesktopOperationActions.SaveSettings))
            {
                SetActionStatus(
                    "Preferencia no disponible",
                    "El motor local no permite quitar de forma segura el certificado predeterminado.",
                    InfoBarSeverity.Warning);
                return null;
            }

            SetActionStatus(
                "Quitando certificado predeterminado",
                "Leyendo primero las preferencias actuales para conservar el resto de opciones.",
                InfoBarSeverity.Informational);
            var settingsResult = await operations.GetSettingsAsync(
                operationCancellation.Token);
            if (!IsSuccessful(settingsResult))
            {
                SetActionStatus(
                    "No se pudieron leer las preferencias",
                    "No se ha modificado el certificado predeterminado. Abra el diagnóstico para conocer la causa.",
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(
                    settingsResult);
            }
            if (settingsResult.Data is null)
            {
                SetActionStatus(
                    "Preferencias incompletas",
                    "El motor no devolvió un documento utilizable y no se ha guardado ningún cambio.",
                    InfoBarSeverity.Error);
                return InvalidResultDiagnostic(
                    settingsResult,
                    "MISSING_SETTINGS_DOCUMENT");
            }

            var updatedSettings =
                settingsResult.Data.CreateSafeSaveSnapshot() with
                {
                    PreferDefaultCertificate = false,
                    DefaultCertificateId = string.Empty,
                };
            var saveResult = await operations.SaveSettingsAsync(
                updatedSettings,
                operationCancellation.Token);
            if (!IsSuccessful(saveResult))
            {
                SetActionStatus(
                    "No se pudo quitar la preferencia",
                    "No se recibió confirmación del cambio. Abra el diagnóstico para conocer la causa.",
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(saveResult);
            }
            if (string.IsNullOrWhiteSpace(saveResult.Data))
            {
                SetActionStatus(
                    "Confirmación incompleta",
                    "El motor no confirmó el guardado y la interfaz no dará el cambio por aplicado.",
                    InfoBarSeverity.Error);
                return InvalidResultDiagnostic(
                    saveResult,
                    "MISSING_SETTINGS_SAVE_CONFIRMATION");
            }

            ApplyDefaultCertificate(null);
            SetActionStatus(
                "Certificado predeterminado eliminado",
                "La aplicación volverá a pedir el certificado cuando sea necesario.",
                InfoBarSeverity.Success);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            SetActionStatus(
                "Cambio cancelado",
                "No se recibió confirmación de que la preferencia haya cambiado.",
                InfoBarSeverity.Warning);
            return null;
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                "Falló la comunicación segura",
                "No se pudo quitar el certificado predeterminado.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                "No se pudo quitar la preferencia",
                "La aplicación no pudo completar el cambio.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?>
        ValidateSelectedCertificateAsync(
            CancellationToken cancellationToken = default)
    {
        var certificate = SelectedCertificate;
        if (certificate is null ||
            string.IsNullOrWhiteSpace(certificate.Id))
        {
            SetActionStatus(
                "Seleccione un certificado",
                "Elija un certificado del catálogo antes de iniciar la comprobación online.",
                InfoBarSeverity.Warning);
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                "Operación en curso",
                "Espere a que termine la operación actual.",
                InfoBarSeverity.Informational);
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.ValidateCertificateOnline,
                out var operations))
            {
                SetActionStatus(
                    "Comprobación online no disponible",
                    "El motor local no ofrece la comprobación online individual.",
                    InfoBarSeverity.Warning);
                return null;
            }

            SetActionStatus(
                "Comprobando certificado",
                "Consultando de forma online el estado de revocación del certificado seleccionado…",
                InfoBarSeverity.Informational);
            var result = await operations.ValidateCertificateOnlineAsync(
                new ValidateCertificateOnlineParameters
                {
                    CertificateId = certificate.Id,
                },
                operationCancellation.Token);
            if (!IsSuccessful(result))
            {
                SetActionStatus(
                    "No se pudo comprobar el certificado",
                    "Abra el diagnóstico para conocer la fase y el responsable del fallo.",
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null)
            {
                SetActionStatus(
                    "Respuesta incompleta",
                    "El motor confirmó la operación, pero no devolvió el estado del certificado.",
                    InfoBarSeverity.Error);
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_CERTIFICATE_ONLINE_STATUS");
            }

            PublishOnlineValidation(result.Data);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            return null;
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                "Falló la comunicación segura",
                "No se pudo completar la comprobación online.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                "No se pudo comprobar el certificado",
                "La aplicación no pudo completar la operación.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?>
        OpenCertificateManagerAsync(
            CancellationToken cancellationToken = default)
    {
        var manager = _windowsCertificateManager;
        if (manager is null)
        {
            SetActionStatus(
                "Gestor de Windows no disponible",
                "Actualice el catálogo para volver a consultar los gestores permitidos por el motor.",
                InfoBarSeverity.Warning);
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                "Operación en curso",
                "Espere a que termine la operación actual.",
                InfoBarSeverity.Informational);
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.OpenCertificateManager,
                out var operations))
            {
                _windowsCertificateManager = null;
                SetActionStatus(
                    "Gestor de Windows no disponible",
                    "El motor local no ofrece la acción segura para abrirlo.",
                    InfoBarSeverity.Warning);
                return null;
            }

            var result = await operations.OpenCertificateManagerAsync(
                new OpenCertificateManagerParameters
                {
                    ManagerId = manager.Id,
                },
                operationCancellation.Token);
            if (!IsSuccessful(result))
            {
                SetActionStatus(
                    "No se pudo abrir el gestor",
                    "Abra el diagnóstico para conocer la causa y la acción recomendada.",
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(result);
            }

            SetActionStatus(
                "Gestor de Windows abierto",
                "El motor local ha abierto el gestor de certificados permitido.",
                InfoBarSeverity.Success);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            return null;
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                "Falló la comunicación segura",
                "No se pudo solicitar la apertura del gestor.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                "No se pudo abrir el gestor",
                "La aplicación no pudo completar la operación.",
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    private async Task<OperationDiagnostic?>
        RefreshCertificateManagerAsync(
            DesktopOperationsClient operations,
            CancellationToken cancellationToken)
    {
        _windowsCertificateManager = null;
        ImportOptions = [];
        SelectedImportOption = null;
        if (!_session.Supports(
                DesktopOperationActions.CertificateAccessOptions))
        {
            UpdateCommandStates();
            return null;
        }

        var result = await operations.GetCertificateImportOptionsAsync(
            cancellationToken);
        if (!IsSuccessful(result))
        {
            SetActionStatus(
                "No se pudieron consultar los destinos",
                "El catálogo está disponible, pero no se pudo preparar la importación ni la apertura de gestores.",
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromResult(result);
        }
        if (result.Data is null)
        {
            SetActionStatus(
                "Inventario de certificados incompleto",
                "El motor no devolvió una lista válida de gestores y destinos permitidos.",
                InfoBarSeverity.Warning);
            return InvalidResultDiagnostic(
                result,
                "MISSING_CERTIFICATE_ACCESS_OPTIONS");
        }

        if (_session.Supports(
            DesktopOperationActions.OpenCertificateManager))
        {
            _windowsCertificateManager = result.Data.Managers
                .FirstOrDefault(manager => string.Equals(
                    manager.Id,
                    WindowsCertificateManagerId,
                    StringComparison.Ordinal));
        }

        var importOptions = new List<CertificateImportOption>();
        if (_session.Supports(
            DesktopOperationActions.UseTemporaryCertificate))
        {
            importOptions.Add(new CertificateImportOption(
                "Solo esta sesión (recomendado)",
                "La clave privada permanece en memoria y se elimina al cerrar.",
                IsTemporary: true,
                TargetId: string.Empty));
        }
        if (_session.Supports(
            DesktopOperationActions.ImportCertificateToStore))
        {
            importOptions.AddRange(result.Data.ImportTargets
                .Where(target =>
                    !string.IsNullOrWhiteSpace(target.Id))
                .Select(target => new CertificateImportOption(
                    target.Recommended
                        ? $"{target.Label} (recomendado)"
                        : target.Label,
                    string.IsNullOrWhiteSpace(target.Browser)
                        ? "Importación persistente en el almacén seleccionado."
                        : $"Importación persistente para {target.Browser}.",
                    IsTemporary: false,
                    TargetId: target.Id)));
        }
        ImportOptions = importOptions;
        SelectedImportOption = importOptions.FirstOrDefault(
            option => option.IsTemporary) ??
            importOptions.FirstOrDefault(option => string.Equals(
                option.TargetId,
                result.Data.PreferredTarget,
                StringComparison.Ordinal)) ??
            importOptions.FirstOrDefault();

        SetActionStatus(
            importOptions.Count == 0
                ? "Importación no disponible"
                : "Certificados preparados",
            importOptions.Count == 0
                ? "El motor no ha publicado destinos seguros para cargar credenciales."
                : "Puede cargar una credencial solo para esta sesión o elegir un almacén persistente.",
            importOptions.Count == 0
                ? InfoBarSeverity.Warning
                : InfoBarSeverity.Informational);
        UpdateCommandStates();
        return null;
    }

    private async Task<OperationDiagnostic?>
        RefreshDefaultCertificateAsync(
            DesktopOperationsClient operations,
            CancellationToken cancellationToken)
    {
        ApplyDefaultCertificate(null);
        if (!_session.Supports(DesktopOperationActions.GetSettings))
        {
            UpdateCommandStates();
            return null;
        }

        var result = await operations.GetSettingsAsync(cancellationToken);
        if (!IsSuccessful(result))
        {
            SetActionStatus(
                "No se pudo leer el certificado predeterminado",
                "El catálogo está disponible, pero no se pudo cargar esta preferencia.",
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromResult(result);
        }
        if (result.Data is null)
        {
            SetActionStatus(
                "Preferencias incompletas",
                "El motor no devolvió el certificado predeterminado guardado.",
                InfoBarSeverity.Warning);
            return InvalidResultDiagnostic(
                result,
                "MISSING_SETTINGS_DOCUMENT");
        }

        var defaultId = result.Data.CreateSafeSaveSnapshot()
            .DefaultCertificateId;
        ApplyDefaultCertificate(defaultId);
        if (string.IsNullOrWhiteSpace(defaultId))
        {
            SetActionStatus(
                "Certificados preparados",
                "Seleccione un certificado para comprobarlo o establecerlo como predeterminado.",
                InfoBarSeverity.Informational);
        }
        else if (_allCertificates.Any(item => string.Equals(
            item.Id,
            defaultId,
            StringComparison.Ordinal)))
        {
            SetActionStatus(
                "Certificado predeterminado cargado",
                "El catálogo identifica visualmente la preferencia guardada.",
                InfoBarSeverity.Informational);
        }
        else
        {
            SetActionStatus(
                "Certificado predeterminado no disponible",
                "La preferencia guardada no aparece en el almacén actual. Puede seleccionar otro certificado.",
                InfoBarSeverity.Warning);
        }
        return null;
    }

    private void ApplyDefaultCertificate(string? certificateId)
    {
        var normalizedId = string.IsNullOrWhiteSpace(certificateId)
            ? null
            : certificateId;
        _defaultCertificateId = normalizedId;
        _hasDefaultCertificate = normalizedId is not null;
        var selectedId = normalizedId ?? SelectedCertificate?.Id;
        _allCertificates = _allCertificates
            .Select(item => item with
            {
                IsDefaultCertificate = string.Equals(
                    item.Id,
                    normalizedId,
                    StringComparison.Ordinal),
                DefaultDisplay = string.Equals(
                    item.Id,
                    normalizedId,
                    StringComparison.Ordinal)
                    ? "Predeterminado"
                    : string.Empty,
            })
            .ToArray();
        ApplyFilter(selectedId);
        RaisePropertyChanged(nameof(IsExpiryNoticeOpen));
        RaisePropertyChanged(nameof(ExpiryNoticeMessage));
        RaisePropertyChanged(nameof(ExpiryNoticeSeverity));
        RaisePropertyChanged(nameof(CanRenewExpiryNoticeCertificate));
        UpdateCommandStates();
    }

    private void ReplaceCatalog(
        IReadOnlyList<CertificateListItem> catalog,
        string? selectedId = null)
    {
        _allCertificates = catalog;
        ApplyFilter(selectedId);
    }

    private void RemoveCertificateFromCatalog(string certificateId)
    {
        _allCertificates = _allCertificates
            .Where(item => !string.Equals(
                item.Id,
                certificateId,
                StringComparison.Ordinal))
            .ToArray();
        ApplyFilter();
    }

    private void RemoveTrackedTemporaryCertificatesFromCatalog()
    {
        _allCertificates = _allCertificates
            .Where(item =>
                !_session.IsTemporaryCertificateTracked(item.Id))
            .ToArray();
        ApplyFilter();
    }

    private void ApplyFilter(string? selectedId = null)
    {
        selectedId ??= SelectedCertificate?.Id;
        var filter = FilterText.Trim();
        var types = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        if (FilterPersonal) types.Add("fisica");
        if (FilterRepresentative) types.Add("representacion");
        if (FilterSeal) types.Add("sello");
        if (FilterPublicEmployee) types.Add("empleado_publico");
        var visible = _allCertificates
            .Where(item => CertificateStructuredFilter.Matches(
                item.SourceCertificate, RequireNif, RequireOrganization,
                types) &&
                (string.IsNullOrEmpty(filter) || item.SearchText.Contains(
                    filter, StringComparison.CurrentCultureIgnoreCase)))
            .ToArray();

        VisibleCertificates = visible.OrderBy(item => item.CanSign ? 0 : 1).ToArray();
        RaisePropertyChanged(nameof(VisibleCountText));
        HasVisibleCertificates = visible.Count > 0;
        SelectedCertificate = (selectedId is null
            ? null
            : visible.FirstOrDefault(item =>
                string.Equals(
                    item.Id,
                    selectedId,
                    StringComparison.Ordinal))) ??
            VisibleCertificates.FirstOrDefault(item => item.CanSign &&
                string.Equals(item.Id, _defaultCertificateId, StringComparison.Ordinal)) ??
            VisibleCertificates.FirstOrDefault(item => item.CanSign) ??
            VisibleCertificates.FirstOrDefault();
    }

    private async Task<OperationDiagnostic?>
        ReloadCatalogAfterMutationAsync(
            DesktopOperationsClient operations,
            string? selectedId,
            CancellationToken cancellationToken)
    {
        var result = await operations.GetCertificatesAsync(
            cancellationToken);
        if (!IsSuccessful(result))
        {
            CatalogMessage =
                "La operación se confirmó, pero no se pudo actualizar el catálogo.";
            SetActionStatus(
                "Catálogo pendiente de actualizar",
                "La credencial cambió, pero la lista visual no pudo recargarse. Pulse Actualizar.",
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromResult(result);
        }
        if (result.Data is null)
        {
            CatalogMessage =
                "La operación se confirmó, pero el catálogo recibido no es válido.";
            return InvalidResultDiagnostic(
                result,
                "MISSING_CERTIFICATE_CATALOG");
        }

        var catalog = result.Data
            .Where(certificate =>
                certificate is not null &&
                !string.IsNullOrWhiteSpace(certificate.Id))
            .GroupBy(
                certificate => certificate.Id,
                StringComparer.Ordinal)
            .Select(group => CertificateListItem.From(
                group.First(),
                _session.IsTemporaryCertificateTracked(group.Key)))
            .OrderBy(item => item.CanSign ? 0 : 1)
            .Take(MaximumCatalogItems)
            .ToArray();
        ReplaceCatalog(catalog, selectedId);
        ApplyDefaultCertificate(_defaultCertificateId);
        CatalogMessage = catalog.Length == 1
            ? "Se ha cargado 1 certificado del almacén real."
            : $"Se han cargado {catalog.Length} certificados del almacén real.";
        return null;
    }

    private void ClearSelectedCredential()
    {
        _selectedCredentialPath = null;
        SelectedCredentialDisplayName =
            "Ninguna credencial seleccionada";
        UpdateCommandStates();
    }

    private void SetBusy(bool value)
    {
        IsBusy = value;
        UpdateCommandStates();
    }

    private void UpdateCommandStates()
    {
        CanRefresh = IsOperationConnected && !IsBusy;
        CanSetSelectedAsDefault =
            IsOperationConnected &&
            !IsBusy &&
            SelectedCertificate is not null &&
            _session.Supports(DesktopOperationActions.GetSettings) &&
            _session.Supports(DesktopOperationActions.SaveSettings);
        CanClearDefaultCertificate =
            IsOperationConnected &&
            !IsBusy &&
            _hasDefaultCertificate &&
            _session.Supports(DesktopOperationActions.GetSettings) &&
            _session.Supports(DesktopOperationActions.SaveSettings);
        CanValidateSelectedCertificate =
            IsOperationConnected &&
            !IsBusy &&
            SelectedCertificate is not null &&
            _session.Supports(
                DesktopOperationActions.ValidateCertificateOnline);
        CanOpenCertificateManager =
            IsOperationConnected &&
            !IsBusy &&
            _windowsCertificateManager is not null &&
            _session.Supports(
                DesktopOperationActions.OpenCertificateManager);
        CanSelectCredential =
            IsOperationConnected &&
            !IsBusy &&
            ImportOptions.Count > 0;
        CanImportCredential =
            CanSelectCredential &&
            !string.IsNullOrWhiteSpace(_selectedCredentialPath) &&
            SelectedImportOption is not null &&
            (SelectedImportOption.IsTemporary
                ? _session.Supports(
                    DesktopOperationActions.UseTemporaryCertificate)
                : _session.Supports(
                    DesktopOperationActions.ImportCertificateToStore));
        CanRemoveSelectedTemporary =
            IsOperationConnected &&
            !IsBusy &&
            SelectedCertificate is not null &&
            _session.IsTemporaryCertificateTracked(
                SelectedCertificate.Id) &&
            _session.Supports(
                DesktopOperationActions.RemoveTemporaryCertificate);
        CanClearTemporaryCertificates =
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.ClearTemporaryCertificates);
    }

    private bool TryBeginOperation(
        CancellationToken cancellationToken,
        out CancellationTokenSource operationCancellation)
    {
        operationCancellation =
            CancellationTokenSource.CreateLinkedTokenSource(
                cancellationToken);
        if (Interlocked.CompareExchange(
            ref _operationInProgress,
            1,
            0) != 0)
        {
            operationCancellation.Dispose();
            return false;
        }

        Volatile.Write(ref _activeCancellation, operationCancellation);
        SetBusy(true);
        return true;
    }

    private void EndOperation(
        CancellationTokenSource operationCancellation)
    {
        Interlocked.CompareExchange(
            ref _activeCancellation,
            null,
            operationCancellation);
        Interlocked.Exchange(ref _operationInProgress, 0);
        SetBusy(false);
        operationCancellation.Dispose();
    }

    private void PublishOnlineValidation(
        CertificateOnlineValidationResult result)
    {
        var message = !string.IsNullOrWhiteSpace(result.UserMessage)
            ? result.UserMessage
            : !string.IsNullOrWhiteSpace(result.Reason)
                ? result.Reason
                : "La comprobación online ha finalizado.";
        switch (result.Status)
        {
            case "valid":
                SetActionStatus(
                    "Certificado no revocado",
                    message,
                    InfoBarSeverity.Success);
                break;
            case "revoked":
                SetActionStatus(
                    "Certificado revocado",
                    message,
                    InfoBarSeverity.Error);
                break;
            case "inconclusive":
                SetActionStatus(
                    "Resultado no concluyente",
                    message,
                    InfoBarSeverity.Warning);
                break;
            case "unavailable":
                SetActionStatus(
                    "Comprobación no disponible",
                    message,
                    InfoBarSeverity.Warning);
                break;
            default:
                SetActionStatus(
                    "Estado no reconocido",
                    "El motor devolvió un estado nuevo que esta interfaz todavía no reconoce.",
                    InfoBarSeverity.Warning);
                break;
        }
    }

    private void SetActionStatus(
        string title,
        string message,
        InfoBarSeverity severity)
    {
        ActionStatusTitle = title;
        ActionStatusMessage = message;
        ActionStatusSeverity = severity;
    }

    private static bool IsSuccessful<TData>(
        IpcCallResult<TData> result) =>
        result.IsSuccess &&
        string.Equals(
            result.Outcome,
            "success",
            StringComparison.Ordinal);

    private static OperationDiagnostic InvalidResultDiagnostic<TData>(
        IpcCallResult<TData> result,
        string errorCode) =>
        OperationDiagnosticMapper.FromResult(new IpcCallResult<object>
        {
            Protocol = result.Protocol,
            RequestId = result.RequestId,
            TraceId = result.TraceId,
            Action = result.Action,
            IsSuccess = false,
            Outcome = "failure",
            ErrorCode = errorCode,
            Phase = result.Phase,
            Retryable = false,
            Data = null,
            Diagnostic = null,
        });
}
