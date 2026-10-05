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
            Localizer.Text("winui.comun.certificado_sin_titular"));
        var issuer = FirstVisible(
            certificate.IssuerName,
            certificate.Issuer,
            Localizer.Text("winui.certificados.emisor_desconocido"));
        var status = certificate.IsExpired
            ? Localizer.Text("winui.certificados.caducado")
            : FirstVisible(
                certificate.Status,
                certificate.CanSign ? Localizer.Text("winui.certificados.disponible_para_firmar") : Localizer.Text("winui.certificados.solo_consulta"));

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
            ? Localizer.Fill("winui.certificados.caducado_el",
                ("date", FormatDate(FirstVisible(certificate.ValidTo, certificate.NotAfter, string.Empty), "dd/MM/yyyy")))
            : certificate.NeedsUnlock
                ? Localizer.Text("winui.certificados.requiere_autorizacion_de_la_tarjeta")
                : engineRejected
                    ? engineStatus
                    : !certificate.CanSign
                        ? Localizer.Text("winui.certificados.no_dispone_de_clave_utilizable_para")
                        : string.Empty;
        var summary = !suitable
            ? Localizer.Fill("winui.certificados.no_valido_para_firmar", ("reason", Localizer.Text(reason)))
            : certificate.DaysUntilExpiration <= FnmtRenewalWindowDays
                ? Localizer.Text("winui.certificados.valido_para_firmar_pero_caduca_pronto")
                : Localizer.Text("winui.certificados.valido_y_apto_para_firmar");

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
                ? Localizer.Fill("winui.certificados.caduca_en_dia_s_la_fnmt_permite",
                    ("days", certificate.DaysUntilExpiration.ToString(System.Globalization.CultureInfo.CurrentCulture)))
                : string.Empty,
            SuitabilitySummary = summary,
            IsSuitable = suitable && !certificate.NeedsUnlock,
            NeedsUnlock = certificate.NeedsUnlock,
            Details = details,
            CompactDetails = BuildCompactDetails(details),
            CardStatus = cardStatus,
            StatusReason = reason,
            CardStatusText = !suitable
                ? Localizer.Text("winui.certificados.no_valido")
                : certificate.DaysUntilExpiration is >= 0 and <= FnmtRenewalWindowDays
                    ? Localizer.Text("winui.certificados.caduca_pronto")
                    : Localizer.Text("winui.certificados.valido"),
            ExpirationDisplay = Localizer.Fill("winui.certificados.vence", ("date", expirationDate)),
            ExpirationDateDisplay = expirationDate,
            DaysUntilExpiration = certificate.DaysUntilExpiration,
            IsExpired = expired,
            IsExpiringSoon = !expired &&
                certificate.DaysUntilExpiration is >= 0 and <= FnmtRenewalWindowDays,
            ShortIssuerDisplay = ShortIssuer(issuer),
            Id = certificate.Id,
            DisplayName = displayName,
            IssuerDisplay = Localizer.Fill("winui.certificados.emisor", ("issuer", issuer)),
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
            // Firma remota CSC: lo custodia un prestador, no está en el equipo.
            TemporaryDisplay = certificate.Remote
                ? Localizer.Text("csc.gui.remoto")
                : isTemporary
                    ? Localizer.Text("winui.certificados.solo_durante_esta_sesion")
                    : Localizer.Text("winui.certificados.almacen_del_sistema"),
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
            new(Localizer.Text("winui.certificados.titular"), holder),
        };
        AddIfPresent(rows, Localizer.Text("winui.certificados.nif_identificador"), certificate.Nif);
        AddIfPresent(rows, Localizer.Text("winui.certificados.organizacion"), certificate.Organization);
        rows.Add(new(Localizer.Text("winui.certificados.tipo"), TypeLabel(certificate.Type)));
        rows.Add(new(Localizer.Text("winui.certificados.emisor_2"), issuer));
        AddIfPresent(rows, Localizer.Text("winui.certificados.numero_de_serie"), certificate.SerialNumber);
        rows.Add(new(Localizer.Text("winui.certificados.estado"), status));
        rows.Add(new(
            Localizer.Text("winui.certificados.valido_hasta"),
            FormatDate(FirstVisible(
                certificate.ValidTo,
                certificate.NotAfter,
                Localizer.Text("winui.certificados.desconocido")))));
        rows.Add(new(
            certificate.IsExpired ? Localizer.Text("winui.certificados.dias_caducado") : Localizer.Text("winui.certificados.dias_restantes"),
            Math.Abs(certificate.DaysUntilExpiration).ToString(
                System.Globalization.CultureInfo.CurrentCulture)));
        AddIfPresent(
            rows,
            Localizer.Text("winui.certificados.huella_sha_256"),
            FormatFingerprint(certificate.Fingerprint));
        return rows;
    }

    private static IReadOnlyList<CertificateDetailRow> BuildCompactDetails(
        IReadOnlyList<CertificateDetailRow> details)
    {
        var rows = new List<CertificateDetailRow>();
        foreach (var label in new[]
        {
            Localizer.Text("winui.certificados.titular"), Localizer.Text("winui.certificados.emisor_2"), Localizer.Text("winui.certificados.valido_hasta"), Localizer.Text("winui.certificados.huella_sha_256"),
        })
        {
            var row = details.FirstOrDefault(item => item.Label == label);
            if (row is not null)
            {
                rows.Add(row);
            }
        }
        var identifier = details.FirstOrDefault(item =>
            item.Label == Localizer.Text("winui.certificados.numero_de_serie")) ?? details.FirstOrDefault(item =>
                item.Label == Localizer.Text("winui.certificados.nif_identificador"));
        if (identifier is not null)
        {
            rows.Insert(Math.Min(2, rows.Count),
                new CertificateDetailRow(Localizer.Text("winui.certificados.no_serie_nif"), identifier.Value));
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
            "fisica" => Localizer.Text("winui.certificados.persona_fisica"),
            "representacion" => Localizer.Text("winui.certificados.representante_de_entidad"),
            "sello" => Localizer.Text("winui.certificados.sello_de_entidad"),
            "empleado_publico" => Localizer.Text("winui.certificados.empleado_publico"),
            _ => Localizer.Text("winui.certificados.no_identificado"),
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
            : Localizer.Text("winui.certificados.fecha_desconocida");

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

public sealed record CertificateDetailRow(string Label, string Value)
{
    public override string ToString() => Localizer.LabelValue(Label, Value);
}

public sealed record CertificateImportOption(
    string Label,
    string Detail,
    bool IsTemporary,
    string TargetId)
{
    public override string ToString() => Label + ". " + Detail;
}

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
        Localizer.Text("winui.certificados.ninguna_credencial_seleccionada");
    private string _catalogMessage =
        Localizer.Text("winui.certificados.conecte_el_motor_local_y_actualice_para");
    private string _actionStatusTitle = Localizer.Text("winui.comun.sin_comprobaciones");
    private string _actionStatusMessage =
        Localizer.Text("winui.certificados.seleccione_un_certificado_para_comprobar");
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
            Localizer.Text("winui.comun.certificados"),
            Localizer.Text("winui.certificados.consulta_filtra_y_comprueba_los"),
            Localizer.Text("winui.certificados.el_almacen_no_esta_disponible_porque_el"))
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

    public string VisibleCountText => Localizer.Fill(
        "winui.certificados.mostrando_de",
        ("shown", VisibleCertificates.Count.ToString(System.Globalization.CultureInfo.CurrentCulture)),
        ("total", _allCertificates.Count.ToString(System.Globalization.CultureInfo.CurrentCulture)));

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
            succeeded ? Localizer.Text("winui.certificados.renovacion_en_la_fnmt") : Localizer.Text("winui.certificados.no_se_pudo_abrir_la_fnmt"),
            message,
            succeeded ? InfoBarSeverity.Informational : InfoBarSeverity.Warning);

    public void ReportExternalValidationLaunch(bool succeeded, string message) =>
        SetActionStatus(
            succeeded ? Localizer.Text("winui.certificados.valide_abierto") : Localizer.Text("winui.certificados.no_se_pudo_abrir_valide"),
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
                    ? Localizer.Text("winui.certificados.este_certificado")
                    : Localizer.Text("winui.certificados.su_certificado_predeterminado");
                return certificate.ExpirationDateDisplay == Localizer.Text("winui.certificados.fecha_desconocida")
                    ? Localizer.Fill("winui.certificados.ha_caducado_y_no_puede_usarse_para",
                        ("subject", Localizer.Text(subject)))
                    : Localizer.Fill("winui.certificados.caduco_el_y_no_puede_usarse_para_firmar",
                        ("subject", Localizer.Text(subject)),
                        ("date", certificate.ExpirationDateDisplay));
            }
            if (!certificate.IsExpiringSoon)
            {
                return string.Empty;
            }
            var remaining = certificate.DaysUntilExpiration.ToString(
                System.Globalization.CultureInfo.CurrentCulture);
            var renewal = certificate.CanRenewAtFnmt
                ? Localizer.Text("winui.certificados.puede_renovarlo_ahora")
                : Localizer.Text("winui.certificados.contacte_con_su_emisor_para_renovarlo");
            var owner = isSelected
                ? Localizer.Text("winui.certificados.su_certificado")
                : Localizer.Text("winui.certificados.su_certificado_predeterminado");
            return Localizer.Fill("winui.certificados.caduca_el_quedan_dias",
                ("owner", Localizer.Text(owner)),
                ("date", certificate.ExpirationDateDisplay),
                ("remaining", remaining),
                ("renewal", Localizer.Text(renewal)));
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
            Localizer.Text(value));
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
        private set => SetProperty(ref _catalogMessage, Localizer.Text(value));
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
        private set => SetProperty(ref _actionStatusTitle, Localizer.Text(value));
    }

    public string ActionStatusMessage
    {
        get => _actionStatusMessage;
        private set => SetProperty(ref _actionStatusMessage, Localizer.Text(value));
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
            Localizer.Text("winui.certificados.motor_local_conectado_puede_consultar_el"));
        if (!available)
        {
            CancelCurrentOperation();
            ReplaceCatalog([]);
            _windowsCertificateManager = null;
            ImportOptions = [];
            SelectedImportOption = null;
            ClearSelectedCredential();
            CatalogMessage =
                Localizer.Text("winui.certificados.el_catalogo_no_esta_disponible_mientras");
            SetActionStatus(
                Localizer.Text("winui.certificados.motor_local_desconectado"),
                Localizer.Text("winui.certificados.no_se_puede_comprobar_certificados_ni"),
                InfoBarSeverity.Warning);
        }
        else if (!_session.Supports(
            DesktopOperationActions.ValidateCertificateOnline))
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.comprobacion_online_no_disponible"),
                Localizer.Text("winui.certificados.el_motor_conectado_no_publica_la"),
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
                Localizer.Text("winui.certificados.ya_se_esta_actualizando_el_catalogo_de");
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.Certificates,
                out var operations))
            {
                CatalogMessage =
                    Localizer.Text("winui.certificados.no_se_puede_consultar_el_almacen_el");
                return null;
            }

            CatalogMessage = Localizer.Text("winui.certificados.consultando_el_almacen_de_windows");
            var result = await operations.GetCertificatesAsync(
                operationCancellation.Token);
            if (!result.IsSuccess ||
                !string.Equals(
                    result.Outcome,
                    "success",
                    StringComparison.Ordinal))
            {
                CatalogMessage =
                    Localizer.Text("winui.certificados.el_motor_local_no_pudo_completar_la");
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null)
            {
                CatalogMessage =
                    Localizer.Text("winui.comun.el_motor_confirmo_la_consulta_pero_no");
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
                ? Localizer.Text("winui.certificados.el_almacen_se_consulto_correctamente")
                : catalog.Length == 1
                    ? Localizer.Text("winui.certificados.se_ha_cargado_1_certificado_del_almacen")
                    : Localizer.Fill("winui.certificados.se_han_cargado_certificados_del_almacen",
                        ("count", catalog.Length.ToString(System.Globalization.CultureInfo.CurrentCulture)));

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
            CatalogMessage = Localizer.Text("winui.certificados.la_actualizacion_del_catalogo_se_cancelo");
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            CatalogMessage =
                Localizer.Text("winui.certificados.fallo_la_comunicacion_segura_al");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            CatalogMessage =
                Localizer.Text("winui.certificados.la_aplicacion_no_pudo_completar_la");
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
                Localizer.Text("winui.certificados.operacion_en_curso"),
                Localizer.Text("winui.certificados.espere_a_que_termine_la_operacion_actual"),
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
                    Localizer.Text("winui.certificados.seleccion_cancelada"),
                    Localizer.Text("winui.certificados.no_se_ha_cambiado_la_credencial"),
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
                    Localizer.Text("winui.certificados.credencial_no_valida"),
                    exception.Message,
                    InfoBarSeverity.Warning);
                return null;
            }

            _selectedCredentialPath = path;
            SelectedCredentialDisplayName =
                DesktopCertificateCredentialFile.SafeDisplayName(path);
            SetActionStatus(
                Localizer.Text("winui.certificados.credencial_preparada"),
                Localizer.Text("winui.certificados.elija_si_quiere_usarla_solo_durante_esta"),
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
                Localizer.Text("winui.certificados.no_se_pudo_leer_la_seleccion"),
                Localizer.Text("winui.certificados.la_aplicacion_no_pudo_preparar_la"),
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
                    Localizer.Text("winui.certificados.faltan_datos_para_importar"),
                    Localizer.Text("winui.certificados.seleccione_una_credencial_y_un_destino"),
                    InfoBarSeverity.Warning);
                return null;
            }
            if (password.Length >
                DesktopOperationsClient.MaximumPasswordBytes)
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.contrasena_demasiado_larga"),
                    Localizer.Text("winui.comun.la_contrasena_supera_el_limite_de"),
                    InfoBarSeverity.Warning);
                return null;
            }
            if (!TryBeginOperation(
                cancellationToken,
                out operationCancellation))
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.operacion_en_curso"),
                    Localizer.Text("winui.certificados.espere_a_que_termine_la_operacion_actual"),
                    InfoBarSeverity.Informational);
                return null;
            }

            var action = option.IsTemporary
                ? DesktopOperationActions.UseTemporaryCertificate
                : DesktopOperationActions.ImportCertificateToStore;
            if (!_session.TryGetOperations(action, out var operations))
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.importacion_no_disponible"),
                    Localizer.Text("winui.certificados.el_motor_local_no_publica_la_accion"),
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
                    Localizer.Text("winui.certificados.cargando_credencial_temporal"),
                    Localizer.Text("winui.certificados.la_clave_privada_se_conservara"),
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
                        Localizer.Text("winui.certificados.no_se_pudo_cargar_la_credencial"),
                        Localizer.Text("winui.certificados.no_se_ha_anadido_ninguna_credencial"),
                        InfoBarSeverity.Error);
                    return OperationDiagnosticMapper.FromResult(result);
                }
                if (result.Data is null ||
                    !result.Data.Temporary ||
                    string.IsNullOrWhiteSpace(result.Data.Id))
                {
                    SetActionStatus(
                        Localizer.Text("winui.certificados.confirmacion_incompleta"),
                        Localizer.Text("winui.comun.el_motor_no_confirmo_una_credencial"),
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
                    Localizer.Text("winui.certificados.credencial_temporal_disponible"),
                    Localizer.Text("winui.certificados.puede_firmar_con_ella_durante_esta"),
                    InfoBarSeverity.Success);
            }
            else
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.importando_credencial"),
                    Localizer.Fill("winui.certificados.instalando_la_credencial_en",
                        ("store", Localizer.Text(option.Label))),
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
                        Localizer.Text("winui.certificados.no_se_pudo_importar_la_credencial"),
                        Localizer.Text("winui.certificados.el_almacen_no_confirmo_la_importacion"),
                        InfoBarSeverity.Error);
                    return OperationDiagnosticMapper.FromResult(result);
                }
                if (string.IsNullOrWhiteSpace(result.Data))
                {
                    SetActionStatus(
                        Localizer.Text("winui.certificados.confirmacion_incompleta"),
                        Localizer.Text("winui.certificados.el_motor_no_confirmo_que_la_credencial"),
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
                    Localizer.Text("winui.certificados.credencial_importada"),
                    Localizer.Fill("winui.certificados.el_almacen_confirmo_la_importacion",
                        ("store", Localizer.Text(option.Label))),
                    InfoBarSeverity.Success);
            }

            ClearSelectedCredential();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.importacion_cancelada"),
                Localizer.Text("winui.certificados.no_se_recibio_confirmacion_de_que_la"),
                InfoBarSeverity.Warning);
            return null;
        }
        catch (OperationCanceledException exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.importacion_cancelada"),
                Localizer.Text("winui.certificados.la_operacion_termino_antes_de_recibir"),
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation?.Token ?? cancellationToken);
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.fallo_la_comunicacion_segura"),
                Localizer.Text("winui.certificados.no_se_pudo_completar_la_importacion_de"),
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.no_se_pudo_importar_la_credencial"),
                Localizer.Text("winui.certificados.la_aplicacion_no_pudo_leer_o_procesar_la"),
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
                Localizer.Text("winui.certificados.seleccione_una_credencial_temporal"),
                Localizer.Text("winui.certificados.solo_se_pueden_retirar_desde_aqui_las"),
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
                    Localizer.Text("winui.certificados.retirada_no_disponible"),
                    Localizer.Text("winui.certificados.el_motor_local_no_ofrece_esta_accion"),
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
                    Localizer.Text("winui.certificados.no_se_pudo_retirar_la_credencial"),
                    Localizer.Text("winui.certificados.no_se_recibio_confirmacion_de_la"),
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
                Localizer.Text("winui.certificados.credencial_temporal_retirada"),
                Localizer.Text("winui.certificados.la_clave_privada_ya_no_esta_disponible"),
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
                Localizer.Text("winui.certificados.no_se_pudo_retirar_la_credencial"),
                Localizer.Text("winui.certificados.la_operacion_no_termino_correctamente"),
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
                    Localizer.Text("winui.certificados.limpieza_no_disponible"),
                    Localizer.Text("winui.certificados.el_motor_local_no_ofrece_esta_accion"),
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
                    Localizer.Text("winui.certificados.no_se_pudieron_retirar_las_credenciales"),
                    Localizer.Text("winui.certificados.no_se_recibio_confirmacion_de_la_2"),
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
                Localizer.Text("winui.certificados.credenciales_temporales_retiradas"),
                Localizer.Text("winui.certificados.la_sesion_ya_no_conserva_claves_privadas"),
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
                Localizer.Text("winui.certificados.no_se_pudieron_retirar_las_credenciales"),
                Localizer.Text("winui.certificados.la_operacion_no_termino_correctamente"),
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
            Localizer.Text("winui.certificados.no_se_pudo_leer_la_contrasena"),
            Localizer.Text("winui.certificados.windows_no_pudo_capturar_o_codificar_la"),
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
                Localizer.Text("winui.certificados.seleccione_un_certificado"),
                Localizer.Text("winui.certificados.elija_un_certificado_del_catalogo_antes"),
                InfoBarSeverity.Warning);
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.operacion_en_curso"),
                Localizer.Text("winui.certificados.espere_a_que_termine_la_operacion_actual"),
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
                    Localizer.Text("winui.certificados.preferencia_no_disponible"),
                    Localizer.Text("winui.certificados.el_motor_local_no_permite_leer_y_guardar"),
                    InfoBarSeverity.Warning);
                return null;
            }

            SetActionStatus(
                Localizer.Text("winui.certificados.guardando_certificado_predeterminado"),
                Localizer.Text("winui.certificados.leyendo_primero_las_preferencias"),
                InfoBarSeverity.Informational);
            var settingsResult = await operations.GetSettingsAsync(
                operationCancellation.Token);
            if (!IsSuccessful(settingsResult))
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.no_se_pudieron_leer_las_preferencias"),
                    Localizer.Text("winui.certificados.no_se_ha_modificado_el_certificado"),
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(
                    settingsResult);
            }
            if (settingsResult.Data is null)
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.preferencias_incompletas"),
                    Localizer.Text("winui.certificados.el_motor_no_devolvio_un_documento"),
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
                    Localizer.Text("winui.certificados.no_se_pudo_guardar_la_preferencia"),
                    Localizer.Text("winui.certificados.no_se_recibio_confirmacion_del_cambio"),
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(saveResult);
            }
            if (string.IsNullOrWhiteSpace(saveResult.Data))
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.confirmacion_incompleta"),
                    Localizer.Text("winui.certificados.el_motor_no_confirmo_el_guardado_y_la"),
                    InfoBarSeverity.Error);
                return InvalidResultDiagnostic(
                    saveResult,
                    "MISSING_SETTINGS_SAVE_CONFIRMATION");
            }

            ApplyDefaultCertificate(certificate.Id);
            SetActionStatus(
                Localizer.Text("winui.certificados.certificado_predeterminado_guardado"),
                Localizer.Fill("winui.certificados.se_usara_primero_cuando_la_aplicacion",
                    ("certificate", certificate.DisplayName)),
                InfoBarSeverity.Success);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            SetActionStatus(
                Localizer.Text("winui.comun.guardado_cancelado"),
                Localizer.Text("winui.certificados.no_se_recibio_confirmacion_de_que_el"),
                InfoBarSeverity.Warning);
            return null;
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.fallo_la_comunicacion_segura"),
                Localizer.Text("winui.certificados.no_se_pudo_guardar_el_certificado"),
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.no_se_pudo_guardar_la_preferencia"),
                Localizer.Text("winui.certificados.la_aplicacion_no_pudo_completar_el"),
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
                Localizer.Text("winui.certificados.operacion_en_curso"),
                Localizer.Text("winui.certificados.espere_a_que_termine_la_operacion_actual"),
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
                    Localizer.Text("winui.certificados.preferencia_no_disponible"),
                    Localizer.Text("winui.certificados.el_motor_local_no_permite_quitar_de"),
                    InfoBarSeverity.Warning);
                return null;
            }

            SetActionStatus(
                Localizer.Text("winui.certificados.quitando_certificado_predeterminado"),
                Localizer.Text("winui.certificados.leyendo_primero_las_preferencias"),
                InfoBarSeverity.Informational);
            var settingsResult = await operations.GetSettingsAsync(
                operationCancellation.Token);
            if (!IsSuccessful(settingsResult))
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.no_se_pudieron_leer_las_preferencias"),
                    Localizer.Text("winui.certificados.no_se_ha_modificado_el_certificado"),
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(
                    settingsResult);
            }
            if (settingsResult.Data is null)
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.preferencias_incompletas"),
                    Localizer.Text("winui.certificados.el_motor_no_devolvio_un_documento"),
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
                    Localizer.Text("winui.certificados.no_se_pudo_quitar_la_preferencia"),
                    Localizer.Text("winui.certificados.no_se_recibio_confirmacion_del_cambio"),
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(saveResult);
            }
            if (string.IsNullOrWhiteSpace(saveResult.Data))
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.confirmacion_incompleta"),
                    Localizer.Text("winui.certificados.el_motor_no_confirmo_el_guardado_y_la"),
                    InfoBarSeverity.Error);
                return InvalidResultDiagnostic(
                    saveResult,
                    "MISSING_SETTINGS_SAVE_CONFIRMATION");
            }

            ApplyDefaultCertificate(null);
            SetActionStatus(
                Localizer.Text("winui.certificados.certificado_predeterminado_eliminado"),
                Localizer.Text("winui.certificados.la_aplicacion_volvera_a_pedir_el"),
                InfoBarSeverity.Success);
            return null;
        }
        catch (OperationCanceledException)
            when (operationCancellation.IsCancellationRequested)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.cambio_cancelado"),
                Localizer.Text("winui.certificados.no_se_recibio_confirmacion_de_que_la_2"),
                InfoBarSeverity.Warning);
            return null;
        }
        catch (IpcClientException exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.fallo_la_comunicacion_segura"),
                Localizer.Text("winui.certificados.no_se_pudo_quitar_el_certificado"),
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.no_se_pudo_quitar_la_preferencia"),
                Localizer.Text("winui.certificados.la_aplicacion_no_pudo_completar_el"),
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
                Localizer.Text("winui.certificados.seleccione_un_certificado"),
                Localizer.Text("winui.certificados.elija_un_certificado_del_catalogo_antes_2"),
                InfoBarSeverity.Warning);
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.operacion_en_curso"),
                Localizer.Text("winui.certificados.espere_a_que_termine_la_operacion_actual"),
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
                    Localizer.Text("winui.certificados.comprobacion_online_no_disponible"),
                    Localizer.Text("winui.certificados.el_motor_local_no_ofrece_la_comprobacion"),
                    InfoBarSeverity.Warning);
                return null;
            }

            SetActionStatus(
                Localizer.Text("winui.certificados.comprobando_certificado"),
                Localizer.Text("winui.certificados.consultando_de_forma_online_el_estado_de"),
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
                    Localizer.Text("winui.certificados.no_se_pudo_comprobar_el_certificado"),
                    Localizer.Text("winui.certificados.abra_el_diagnostico_para_conocer_la_fase"),
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null)
            {
                SetActionStatus(
                    Localizer.Text("winui.certificados.respuesta_incompleta"),
                    Localizer.Text("winui.certificados.el_motor_confirmo_la_operacion_pero_no"),
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
                Localizer.Text("winui.certificados.fallo_la_comunicacion_segura"),
                Localizer.Text("winui.certificados.no_se_pudo_completar_la_comprobacion"),
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.no_se_pudo_comprobar_el_certificado"),
                Localizer.Text("winui.certificados.la_aplicacion_no_pudo_completar_la_2"),
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
                Localizer.Text("winui.certificados.gestor_de_windows_no_disponible"),
                Localizer.Text("winui.certificados.actualice_el_catalogo_para_volver_a"),
                InfoBarSeverity.Warning);
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.operacion_en_curso"),
                Localizer.Text("winui.certificados.espere_a_que_termine_la_operacion_actual"),
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
                    Localizer.Text("winui.certificados.gestor_de_windows_no_disponible"),
                    Localizer.Text("winui.certificados.el_motor_local_no_ofrece_la_accion"),
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
                    Localizer.Text("winui.certificados.no_se_pudo_abrir_el_gestor"),
                    Localizer.Text("winui.certificados.abra_el_diagnostico_para_conocer_la"),
                    InfoBarSeverity.Error);
                return OperationDiagnosticMapper.FromResult(result);
            }

            SetActionStatus(
                Localizer.Text("winui.certificados.gestor_de_windows_abierto"),
                Localizer.Text("winui.certificados.el_motor_local_ha_abierto_el_gestor_de"),
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
                Localizer.Text("winui.certificados.fallo_la_comunicacion_segura"),
                Localizer.Text("winui.certificados.no_se_pudo_solicitar_la_apertura_del"),
                InfoBarSeverity.Error);
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.no_se_pudo_abrir_el_gestor"),
                Localizer.Text("winui.certificados.la_aplicacion_no_pudo_completar_la_2"),
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
                Localizer.Text("winui.certificados.no_se_pudieron_consultar_los_destinos"),
                Localizer.Text("winui.certificados.el_catalogo_esta_disponible_pero_no_se"),
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromResult(result);
        }
        if (result.Data is null)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.inventario_de_certificados_incompleto"),
                Localizer.Text("winui.certificados.el_motor_no_devolvio_una_lista_valida_de"),
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
                Localizer.Text("winui.certificados.solo_esta_sesion_recomendado"),
                Localizer.Text("winui.certificados.la_clave_privada_permanece_en_memoria_y"),
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
                        ? Localizer.Fill("winui.certificados.recomendado",
                            ("store", Localizer.Text(target.Label)))
                        : target.Label,
                    string.IsNullOrWhiteSpace(target.Browser)
                        ? Localizer.Text("winui.certificados.importacion_persistente_en_el_almacen")
                        : Localizer.Fill("winui.certificados.importacion_persistente_para",
                            ("browser", target.Browser)),
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
                ? Localizer.Text("winui.certificados.importacion_no_disponible")
                : Localizer.Text("winui.certificados.certificados_preparados"),
            importOptions.Count == 0
                ? Localizer.Text("winui.certificados.el_motor_no_ha_publicado_destinos")
                : Localizer.Text("winui.certificados.puede_cargar_una_credencial_solo_para"),
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
                Localizer.Text("winui.certificados.no_se_pudo_leer_el_certificado"),
                Localizer.Text("winui.certificados.el_catalogo_esta_disponible_pero_no_se_2"),
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromResult(result);
        }
        if (result.Data is null)
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.preferencias_incompletas"),
                Localizer.Text("winui.certificados.el_motor_no_devolvio_el_certificado"),
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
                Localizer.Text("winui.certificados.certificados_preparados"),
                Localizer.Text("winui.certificados.seleccione_un_certificado_para"),
                InfoBarSeverity.Informational);
        }
        else if (_allCertificates.Any(item => string.Equals(
            item.Id,
            defaultId,
            StringComparison.Ordinal)))
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.certificado_predeterminado_cargado"),
                Localizer.Text("winui.certificados.el_catalogo_identifica_visualmente_la"),
                InfoBarSeverity.Informational);
        }
        else
        {
            SetActionStatus(
                Localizer.Text("winui.certificados.certificado_predeterminado_no_disponible"),
                Localizer.Text("winui.certificados.la_preferencia_guardada_no_aparece_en_el"),
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
                    ? Localizer.Text("winui.certificados.predeterminado")
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
        HasVisibleCertificates = visible.Length > 0;
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
                Localizer.Text("winui.certificados.la_operacion_se_confirmo_pero_no_se_pudo");
            SetActionStatus(
                Localizer.Text("winui.certificados.catalogo_pendiente_de_actualizar"),
                Localizer.Text("winui.certificados.la_credencial_cambio_pero_la_lista"),
                InfoBarSeverity.Warning);
            return OperationDiagnosticMapper.FromResult(result);
        }
        if (result.Data is null)
        {
            CatalogMessage =
                Localizer.Text("winui.certificados.la_operacion_se_confirmo_pero_el");
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
            ? Localizer.Text("winui.certificados.se_ha_cargado_1_certificado_del_almacen")
            : Localizer.Fill("winui.certificados.se_han_cargado_certificados_del_almacen",
                ("count", catalog.Length.ToString(System.Globalization.CultureInfo.CurrentCulture)));
        return null;
    }

    private void ClearSelectedCredential()
    {
        _selectedCredentialPath = null;
        SelectedCredentialDisplayName =
            Localizer.Text("winui.certificados.ninguna_credencial_seleccionada");
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
                : Localizer.Text("winui.certificados.la_comprobacion_online_ha_finalizado");
        switch (result.Status)
        {
            case "valid":
                SetActionStatus(
                    Localizer.Text("winui.certificados.certificado_no_revocado"),
                    message,
                    InfoBarSeverity.Success);
                break;
            case "revoked":
                SetActionStatus(
                    Localizer.Text("winui.certificados.certificado_revocado"),
                    message,
                    InfoBarSeverity.Error);
                break;
            case "inconclusive":
                SetActionStatus(
                    Localizer.Text("winui.certificados.resultado_no_concluyente"),
                    message,
                    InfoBarSeverity.Warning);
                break;
            case "unavailable":
                SetActionStatus(
                    Localizer.Text("winui.comun.comprobacion_no_disponible"),
                    message,
                    InfoBarSeverity.Warning);
                break;
            default:
                SetActionStatus(
                    Localizer.Text("winui.certificados.estado_no_reconocido"),
                    Localizer.Text("winui.certificados.el_motor_devolvio_un_estado_nuevo_que"),
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
