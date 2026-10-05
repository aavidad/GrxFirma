// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Runtime.InteropServices;
using System.Security.Cryptography;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;

namespace GrxFirma.WinUI.ViewModels;

public sealed record SignActionOption(string SourceLabel, string Value)
{
    public string Label => Localizer.Text(SourceLabel);
}

public sealed record SignatureFormatOption(
    string SourceLabel,
    string Value,
    SaveFilePickerProfile SaveProfile)
{
    public string Label => Localizer.Text(SourceLabel);
}

public sealed record SignatureProfileOption(string SourceLabel, string Value)
{
    public string Label => Localizer.Text(SourceLabel);
}

public sealed record VisibleSealRotationOption(string SourceLabel, int Value)
{
    public string Label => Localizer.Text(SourceLabel);
}

public sealed record VisibleSealStyleOption(
    string SourceLabel,
    string Value,
    string SourceDescription)
{
    public string Label => Localizer.Text(SourceLabel);
    public string Description => Localizer.Text(SourceDescription);
}

public sealed record VisibleSealPageModeOption(string SourceLabel, string Value)
{
    public string Label => Localizer.Text(SourceLabel);
}

public sealed record BatchSignDisplayItem(
    string DocumentName,
    string Status,
    string Detail,
    bool IsSuccessful,
    string? OutputPath);

public sealed class SignPageViewModel
    : WorkspacePageViewModel
{
    private const int MaximumCatalogItems = 512;

    // A fresh success identity, never inferred from an existing output or text.
    public Guid CompletedSignPresentationId { get; private set; }
    private const int MaximumBatchDocuments = 128;
    private const long MaximumBatchDocumentBytes =
        100L * 1024 * 1024;
    private const long MaximumBatchPayloadBytes =
        256L * 1024 * 1024;
    private const int MaximumPageSelectionLength = 128;
    private const int MaximumQrUrlLength = 1024;
    private static string OrientativeSealMessage =>
        Localizer.Text("winui.firmar.muestra_orientativa_de_posicion_y_giro");
    private const long MaximumSealImageBytes = 10 * 1024 * 1024;
    private const int MaximumSealPreviewBytes = 16 * 1024 * 1024;
    private const int MaximumMetadataLength = 256;
    private const int MaximumPreviewImageBytes = 2_900_000;
    private const long MaximumPreviewPdfBytes = 100L * 1024 * 1024;
    private const double DefaultPreviewPageWidth = 595.28;
    private const double DefaultPreviewPageHeight = 841.89;
    private const string WindowsPersonalStoreTargetId = "windows-my";

    private readonly DesktopOperationSession _session;
    private readonly IFilePickerService _filePicker;
    private readonly IPdfPreviewService? _pdfPreview;
    private IReadOnlyList<CertificateListItem> _certificates = [];
    private CertificateListItem? _selectedCertificate;
    private IReadOnlyList<CertificateListItem> _additionalSignerCandidates = [];
    private IReadOnlyList<string> _selectedAdditionalCertificateIds = [];
    private SignActionOption? _selectedAction;
    private SignatureFormatOption? _selectedFormat;
    private SignatureProfileOption? _selectedProfile;
    private CancellationTokenSource? _activeCancellation;
    private string? _inputPath;
    private string? _outputPath;
    private string? _selectedCredentialPath;
    private IReadOnlyList<string> _batchExplicitInputPaths = [];
    private IReadOnlyList<string> _batchDirectoryInputPaths = [];
    private IReadOnlyList<string> _batchInputPaths = [];
    private string? _batchSourceDirectory;
    private string? _batchOutputDirectory;
    private string _inputDisplayName = string.Empty;
    private string _batchInputSummary =
        Localizer.Text("winui.firmar.no_se_han_seleccionado_documentos_para");
    private string _batchOutputSummary =
        Localizer.Text("winui.firmar.no_se_ha_seleccionado_una_carpeta_de");
    private string _batchProgressText =
        Localizer.Text("winui.firmar.el_lote_aun_no_se_ha_ejecutado");
    private IReadOnlyList<BatchSignDisplayItem> _batchItems = [];
    private string _resultMessage =
        Localizer.Text("winui.firmar.no_se_ha_ejecutado_ninguna_firma");
    private string _validationMessage =
        Localizer.Text("winui.firmar.seleccione_un_documento_y_un_certificado");
    private bool _isBusy;
    private bool _canSelectDocument;
    private bool _canRefreshCertificates;
    private bool _canUseTemporaryCredential;
    private bool _canImportCredentialToWindows;
    private bool _canSign;
    private bool _canCancel;
    private bool _canOpenOutput;
    private bool _canValidateAfterSigning;
    private bool _validateAfterSigning = true;
    private bool _batchModeEnabled;
    private bool _canConfigureBatch;
    private bool _canSelectBatchSources;
    private bool _canSelectBatchOutput;
    private bool _canRunBatch;
    private bool _canOpenBatchOutput;
    private bool _hasBatchResults;
    private bool _isBatchProgressIndeterminate;
    private double _batchProgressValue;
    private double _batchProgressMaximum = 1;
    private bool _guidedMultiCosignEnabled;
    private bool _canConfigureGuidedMultiCosign;
    private bool _canSelectAdditionalSigners;
    private string _guidedMultiCosignSummary =
        Localizer.Text("winui.firmar.seleccione_un_documento_compatible_y_un");
    private bool _visibleSealEnabled;
    private bool _canConfigureVisibleSeal;
    private bool _canRefreshVisibleSealPreview;
    private string _visibleSealPages = "1";
    private double _visibleSealXPercent = 62;
    private double _visibleSealYPercent = 4;
    private double _visibleSealWidthPercent = 34;
    private double _visibleSealHeightPercent = 12;
    private bool _visibleSealKeepText = true;
    private double _visibleSealLogoOpacityPercent = 100;
    private string? _sealLanguagePreference;
    private string _visibleSealLogoOpacityLabel = string.Empty;
    private string _visibleSealOpacityHelp = string.Empty;
    private int _visibleSealRotationDegrees;
    private bool _rotatingVisibleSeal;
    private bool _perPageSealEnabled;
    private bool _loadingSealPage;
    private int _requestedPreviewPage;
    private string _sealUiLanguage = "es";
    private readonly Dictionary<int, VisibleSealPlacementParameters> _sealPlacements = [];
    private VisibleSealStyleOption? _selectedVisibleSealStyle;
    private string? _visibleSealImagePath;
    private bool _visibleSealCsvEnabled;
    private string _visibleSealCsvCode = string.Empty;
    private string _visibleSealCsvUrl = string.Empty;
    private string _visibleSealCsvText = string.Empty;
    private bool _visibleSealCsvQr;
    private bool _visibleSealQrEnabled;
    private string _visibleSealQrUrl = string.Empty;
    private VisibleSealPageModeOption? _selectedVisibleSealPageMode;
    private ReadOnlyMemory<byte> _visibleSealStampImage =
        ReadOnlyMemory<byte>.Empty;
    private string _visibleSealStampMessage = OrientativeSealMessage;
    private CancellationTokenSource? _stampPreviewCancellation;
    private string _signatureReason = string.Empty;
    private string _signatureLocation = string.Empty;
    private string _signatureContact = string.Empty;
    private string _visibleSealPreviewMessage =
        Localizer.Text("winui.firmar.active_el_sello_y_cargue_la");
    private ReadOnlyMemory<byte> _visibleSealPreviewImage =
        ReadOnlyMemory<byte>.Empty;
    private double _previewPageWidth;
    private double _previewPageHeight;
    private int _previewCurrentPage;
    private int _previewTotalPages;
    private string? _previewInputPath;
    private bool _portalSealMode;
    private string _portalSignerName = string.Empty;
    private long _previewFileLength;
    private long _previewFileLastWriteUtcTicks;
    private byte[]? _previewFileDigest;
    private bool _isPreviewImageRendered;
    private int _operationInProgress;

    public SignPageViewModel(
        DesktopOperationSession session,
        IFilePickerService filePicker,
        IPdfPreviewService? pdfPreview = null)
        : base(
            Localizer.Text("winui.comun.firmar"),
            Localizer.Text("winui.firmar.firma_cofirma_o_contrafirma_documentos"),
            Localizer.Text("winui.firmar.la_firma_no_esta_disponible_porque_el"))
    {
        ArgumentNullException.ThrowIfNull(session);
        ArgumentNullException.ThrowIfNull(filePicker);
        _session = session;
        _filePicker = filePicker;
        _pdfPreview = pdfPreview;
        _selectedAction = Actions[0];
        _selectedFormat = Formats[0];
        _selectedProfile = Profiles[0];
        _selectedVisibleSealStyle = VisibleSealStyles[0];
        _selectedVisibleSealPageMode = VisibleSealPageModes[0];
        UpdateAvailability();
    }

    public IReadOnlyList<SignActionOption> Actions { get; } =
    [
        new("winui.firmar.firma", "sign"),
        new("winui.firmar.cofirma", "cosign"),
        new("winui.firmar.contrafirma", "countersign"),
    ];

    private bool _verifactuInput;
    private static readonly SignatureFormatOption VeriFactuFormat = new("verifactu.profile_label", "verifactu", SaveFilePickerProfile.FacturaeXml);
    public IReadOnlyList<SignatureFormatOption> Formats => _verifactuInput
        ? StandardFormats.Concat(new[] { VeriFactuFormat }).ToArray()
        : StandardFormats;
    private IReadOnlyList<SignatureFormatOption> StandardFormats { get; } =
    [
        new("winui.comun.automatico", "", SaveFilePickerProfile.CadesSignature),
        new("PAdES", "pades", SaveFilePickerProfile.SignedPdf),
        new("CAdES", "cades", SaveFilePickerProfile.CadesSignature),
        new("XAdES", "xades", SaveFilePickerProfile.XadesSignature),
        new("winui.firmar.xmldsig", "xmldsig", SaveFilePickerProfile.XmlDsigSignature),
        new("winui.comun.odf", "odf", SaveFilePickerProfile.CadesSignature),
        new("winui.comun.ooxml", "ooxml", SaveFilePickerProfile.CadesSignature),
        new("winui.comun.facturae", "facturae", SaveFilePickerProfile.FacturaeXml),
        // El backend Go y la GUI Qt generan actualmente .p7s para este
        // formato en el flujo sign, incluso cuando el contenido es ASiC.
        new("winui.comun.asic_xades", "asic-xades", SaveFilePickerProfile.CadesSignature),
    ];

    public IReadOnlyList<SignatureProfileOption> Profiles { get; } =
    [
        new("Baseline", "baseline"),
        new("T", "t"),
        new("LT", "lt"),
        new("LTA", "lta"),
    ];

    public IReadOnlyList<VisibleSealRotationOption> VisibleSealRotations
    {
        get;
    } =
    [
        new("winui.firmar.sin_rotacion", 0),
        new("90°", 90),
        new("180°", 180),
        new("270°", 270),
    ];

    public IReadOnlyList<VisibleSealStyleOption> VisibleSealStyles
    {
        get;
    } =
    [
        new(
            "winui.firmar.institucional_con_emblema",
            "institucional",
            "winui.firmar.emblema_de_firma_de_la_diputacion_a_la"),
        new(
            "winui.firmar.solo_texto",
            "texto",
            "winui.firmar.sello_sobrio_con_los_datos_de_la_firma"),
        new(
            "winui.firmar.imagen_propia",
            "imagen",
            "winui.firmar.una_imagen_png_o_jpeg_suya_logotipo"),
    ];

    public IReadOnlyList<VisibleSealPageModeOption> VisibleSealPageModes
    {
        get;
    } =
    [
        new("winui.firmar.primera_pagina", "first"),
        new("winui.firmar.ultima_pagina", "last"),
        new("winui.firmar.todas_las_paginas", "all"),
        new("winui.firmar.paginas_concretas", "custom"),
    ];

    public IReadOnlyList<CertificateListItem> Certificates
    {
        get => _certificates;
        private set => SetProperty(ref _certificates, value);
    }

    /// <summary>
    /// Pide el PIN y el OTP de un certificado remoto (firma CSC) en un
    /// diálogo con campos de contraseña. Devuelve null si la persona cancela.
    /// </summary>
    public Func<CertificateInfo, CancellationToken, Task<RemoteSigningSecrets?>>? RemoteSecretsPrompt { get; set; }

    private async Task<(bool Proceed, RemoteSigningSecrets? Secrets)> PrepareRemoteSigningAsync(
        CertificateListItem certificate,
        bool batch,
        CancellationToken cancellationToken,
        int documentCount = 1,
        bool multiCosign = false)
    {
        var source = certificate.SourceCertificate;
        if (!source.Remote)
        {
            return (true, null);
        }
        // Un lote con OTP solo se firma si el prestador autoriza varias firmas
        // con un código (multisign) y el lote cabe en una autorización.
        if (batch && source.RemoteOtp)
        {
            if (multiCosign || source.RemoteMultiSign < 2)
            {
                ValidationMessage = Localizer.Text("csc.error.otp_lote");
                return (false, null);
            }
            if (documentCount > source.RemoteMultiSign)
            {
                ValidationMessage = Localizer.Text("csc.error.otp_lote_excede");
                return (false, null);
            }
        }
        if (!source.RemotePin && !source.RemoteOtp)
        {
            return (true, null);
        }
        var prompt = RemoteSecretsPrompt;
        if (prompt is null)
        {
            ValidationMessage = Localizer.Text("csc.error.secreto_no_pedido");
            return (false, null);
        }
        var secrets = await prompt(source, cancellationToken);
        if (secrets is null)
        {
            ValidationMessage = Localizer.Text("winui.comun.operacion_cancelada");
            return (false, null);
        }
        return (true, secrets);
    }

    private static string? RemoteSigningFailureMessage(string? errorCode) =>
        RemoteSigningInput.MessageKey(errorCode) is { } key
            ? Localizer.Text(key)
            : null;

    public CertificateListItem? SelectedCertificate
    {
        get => _selectedCertificate;
        set
        {
            if (SetProperty(ref _selectedCertificate, value))
            {
                InvalidateBatchResults();
                RefreshGuidedMultiCosignState();
                ScheduleSealStampPreview();
                UpdateValidationMessage();
                UpdateCommandStates();
            }
        }
    }

    public IReadOnlyList<CertificateListItem> AdditionalSignerCandidates
    {
        get => _additionalSignerCandidates;
        private set => SetProperty(ref _additionalSignerCandidates, value);
    }

    public bool GuidedMultiCosignEnabled
    {
        get => _guidedMultiCosignEnabled;
        set
        {
            var normalized =
                value && IsGuidedMultiCosignEligible();
            if (SetProperty(ref _guidedMultiCosignEnabled, normalized))
            {
                ClearOutput();
                InvalidateBatchResults();
                UpdateGuidedMultiCosignSummary();
                UpdateValidationMessage();
                UpdateCommandStates();
            }
        }
    }

    public bool CanConfigureGuidedMultiCosign
    {
        get => _canConfigureGuidedMultiCosign;
        private set => SetProperty(
            ref _canConfigureGuidedMultiCosign,
            value);
    }

    public bool CanSelectAdditionalSigners
    {
        get => _canSelectAdditionalSigners;
        private set => SetProperty(
            ref _canSelectAdditionalSigners,
            value);
    }

    public string GuidedMultiCosignSummary
    {
        get => _guidedMultiCosignSummary;
        private set => SetProperty(
            ref _guidedMultiCosignSummary,
            value);
    }

    public SignActionOption? SelectedAction
    {
        get => _selectedAction;
        set
        {
            if (SetProperty(ref _selectedAction, value))
            {
                ClearOutput();
                InvalidateBatchResults();
                RefreshGuidedMultiCosignState();
                UpdateValidationMessage();
                UpdateCommandStates();
            }
        }
    }

    private async Task UpdateVeriFactuFormatAsync(string path)
    {
        _verifactuInput = false;
        if (SelectedFormat?.Value == "verifactu") SelectedFormat = StandardFormats[0];
        RaisePropertyChanged(nameof(Formats));
        if (!path.EndsWith(".xml", StringComparison.OrdinalIgnoreCase) ||
            !_session.TryGetOperations(DesktopOperationActions.DetectVeriFactu, out var operations)) return;
        try
        {
            var result = await operations.DetectVeriFactuAsync(path);
            if (!PathsEqual(_inputPath ?? string.Empty, path)) return;
            _verifactuInput = result.IsSuccess && result.Data?.IsVerifactu == true;
            RaisePropertyChanged(nameof(Formats));
        }
        catch (Exception) { /* La firma conserva el formato automático si falla la detección. */ }
    }

    public SignatureFormatOption? SelectedFormat
    {
        get => _selectedFormat;
        set
        {
            if (SetProperty(ref _selectedFormat, value))
            {
                ClearOutput();
                InvalidateBatchResults();
                ClearVisibleSealPreview();
                RefreshGuidedMultiCosignState();
                UpdateValidationMessage();
                UpdateCommandStates();
            }
        }
    }

    public SignatureProfileOption? SelectedProfile
    {
        get => _selectedProfile;
        set
        {
            if (SetProperty(ref _selectedProfile, value))
            {
                ClearOutput();
                InvalidateBatchResults();
                UpdateCommandStates();
            }
        }
    }

    public string InputDisplayName
    {
        // Vacío sin selección: el PlaceholderText traducido muestra el aviso
        // y sigue el idioma aunque cambie en caliente.
        get => _inputDisplayName;
        private set => SetProperty(ref _inputDisplayName, value);
    }

    public bool BatchModeEnabled
    {
        get => _batchModeEnabled;
        set
        {
            var normalized =
                value &&
                _session.Supports(DesktopOperationActions.SignBatch);
            if (!SetProperty(ref _batchModeEnabled, normalized))
            {
                return;
            }

            if (normalized && VisibleSealEnabled)
            {
                VisibleSealEnabled = false;
            }
            ClearOutput();
            ResetBatchResults();
            RefreshGuidedMultiCosignState();
            UpdateValidationMessage();
            UpdateCommandStates();
        }
    }

    public string BatchInputSummary
    {
        get => _batchInputSummary;
        private set => SetProperty(ref _batchInputSummary, value);
    }

    public string BatchOutputSummary
    {
        get => _batchOutputSummary;
        private set => SetProperty(ref _batchOutputSummary, value);
    }

    public string BatchProgressText
    {
        get => _batchProgressText;
        private set => SetProperty(ref _batchProgressText, value);
    }

    public IReadOnlyList<BatchSignDisplayItem> BatchItems
    {
        get => _batchItems;
        private set => SetProperty(ref _batchItems, value);
    }

    public bool CanConfigureBatch
    {
        get => _canConfigureBatch;
        private set => SetProperty(ref _canConfigureBatch, value);
    }

    public bool CanSelectBatchSources
    {
        get => _canSelectBatchSources;
        private set => SetProperty(ref _canSelectBatchSources, value);
    }

    public bool CanSelectBatchOutput
    {
        get => _canSelectBatchOutput;
        private set => SetProperty(ref _canSelectBatchOutput, value);
    }

    public bool CanRunBatch
    {
        get => _canRunBatch;
        private set => SetProperty(ref _canRunBatch, value);
    }

    public bool CanOpenBatchOutput
    {
        get => _canOpenBatchOutput;
        private set => SetProperty(ref _canOpenBatchOutput, value);
    }

    public bool HasBatchResults
    {
        get => _hasBatchResults;
        private set => SetProperty(ref _hasBatchResults, value);
    }

    public bool IsBatchProgressIndeterminate
    {
        get => _isBatchProgressIndeterminate;
        private set => SetProperty(
            ref _isBatchProgressIndeterminate,
            value);
    }

    public double BatchProgressValue
    {
        get => _batchProgressValue;
        private set => SetProperty(ref _batchProgressValue, value);
    }

    public double BatchProgressMaximum
    {
        get => _batchProgressMaximum;
        private set => SetProperty(
            ref _batchProgressMaximum,
            Math.Max(1, value));
    }

    public string? BatchOutputDirectory => _batchOutputDirectory;

    public string ResultMessage
    {
        get => _resultMessage;
        private set => SetProperty(ref _resultMessage, value);
    }

    public string ValidationMessage
    {
        get => _validationMessage;
        private set => SetProperty(ref _validationMessage, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanSelectDocument
    {
        get => _canSelectDocument;
        private set => SetProperty(ref _canSelectDocument, value);
    }

    public bool CanRefreshCertificates
    {
        get => _canRefreshCertificates;
        private set => SetProperty(
            ref _canRefreshCertificates,
            value);
    }

    public bool CanUseTemporaryCredential
    {
        get => _canUseTemporaryCredential;
        private set => SetProperty(
            ref _canUseTemporaryCredential,
            value);
    }

    public bool CanImportCredentialToWindows
    {
        get => _canImportCredentialToWindows;
        private set => SetProperty(
            ref _canImportCredentialToWindows,
            value);
    }

    public bool HasPreparedCredential =>
        !string.IsNullOrWhiteSpace(_selectedCredentialPath);

    public bool CanSign
    {
        get => _canSign;
        private set => SetProperty(ref _canSign, value);
    }

    public bool CanCancel
    {
        get => _canCancel;
        private set => SetProperty(ref _canCancel, value);
    }

    public bool CanOpenOutput
    {
        get => _canOpenOutput;
        private set => SetProperty(ref _canOpenOutput, value);
    }

    public bool CanValidateAfterSigning
    {
        get => _canValidateAfterSigning;
        private set => SetProperty(ref _canValidateAfterSigning, value);
    }

    public bool ValidateAfterSigning
    {
        get => _validateAfterSigning;
        set => SetProperty(ref _validateAfterSigning, value);
    }

    public bool VisibleSealEnabled
    {
        get => _visibleSealEnabled;
        set
        {
            if (SetProperty(ref _visibleSealEnabled, value))
            {
                ClearOutput();
                if (!value)
                {
                    ClearVisibleSealPreview();
                }
                ScheduleSealStampPreview();
                UpdateValidationMessage();
                UpdateCommandStates();
            }
        }
    }

    public bool CanConfigureVisibleSeal
    {
        get => _canConfigureVisibleSeal;
        private set => SetProperty(ref _canConfigureVisibleSeal, value);
    }

    public bool CanRefreshVisibleSealPreview
    {
        get => _canRefreshVisibleSealPreview;
        private set => SetProperty(
            ref _canRefreshVisibleSealPreview,
            value);
    }

    public string VisibleSealPages
    {
        get => _visibleSealPages;
        set
        {
            if (SetProperty(ref _visibleSealPages, value ?? string.Empty))
            {
                ClearOutput();
                ClearVisibleSealPreview();
                UpdateValidationMessage();
            }
        }
    }

    public double VisibleSealXPercent
    {
        get => _visibleSealXPercent;
        set
        {
            if (SetProperty(ref _visibleSealXPercent, value))
            {
                OnVisibleSealGeometryChanged();
            }
        }
    }

    public double VisibleSealYPercent
    {
        get => _visibleSealYPercent;
        set
        {
            if (SetProperty(ref _visibleSealYPercent, value))
            {
                OnVisibleSealGeometryChanged();
            }
        }
    }

    public double VisibleSealWidthPercent
    {
        get => _visibleSealWidthPercent;
        set
        {
            if (SetProperty(ref _visibleSealWidthPercent, value))
            {
                OnVisibleSealGeometryChanged();
            }
        }
    }

    public double VisibleSealHeightPercent
    {
        get => _visibleSealHeightPercent;
        set
        {
            if (SetProperty(ref _visibleSealHeightPercent, value))
            {
                OnVisibleSealGeometryChanged();
            }
        }
    }

    public bool VisibleSealKeepText
    {
        get => _visibleSealKeepText;
        set
        {
            if (SetProperty(ref _visibleSealKeepText, value))
            {
                ClearOutput();
                RaisePropertyChanged(nameof(VisibleSealPreviewText));
                ScheduleSealStampPreview();
                UpdateValidationMessage();
            }
        }
    }

    public double VisibleSealLogoOpacityPercent
    {
        get => _visibleSealLogoOpacityPercent;
        set
        {
            if (!double.IsFinite(value) || value is < 0 or > 100)
            {
                return;
            }
            var rounded = Math.Round(value);
            if (SetProperty(ref _visibleSealLogoOpacityPercent, rounded))
            {
                RaisePropertyChanged(nameof(VisibleSealLogoOpacityDisplay));
                ClearOutput();
                ScheduleSealStampPreview();
                UpdateValidationMessage();
            }
        }
    }

    public string VisibleSealLogoOpacityDisplay =>
        $"{VisibleSealLogoOpacityPercent:0} %";

    public string VisibleSealLogoOpacityLabel
    {
        get => _visibleSealLogoOpacityLabel;
        set => SetProperty(ref _visibleSealLogoOpacityLabel, value ?? string.Empty);
    }

    public string VisibleSealOpacityHelp
    {
        get => _visibleSealOpacityHelp;
        set => SetProperty(ref _visibleSealOpacityHelp, value ?? string.Empty);
    }

    // Los giros rápidos (0, 90, 180 y 270) y el giro libre comparten un
    // único valor en grados; si no coincide con uno rápido, la lista queda
    // sin selección y el control muestra «Giro libre».
    public VisibleSealRotationOption? SelectedVisibleSealRotation
    {
        get => VisibleSealRotations.FirstOrDefault(
            option => option.Value == _visibleSealRotationDegrees);
        set
        {
            if (value is not null)
            {
                VisibleSealRotationDegrees = value.Value;
            }
        }
    }

    public double VisibleSealRotationDegrees
    {
        get => _visibleSealRotationDegrees;
        set
        {
            var normalized = NormalizeRotation(value);
            if (normalized == _visibleSealRotationDegrees)
            {
                return;
            }
            _visibleSealRotationDegrees = normalized;
            ConstrainVisibleSealCardToPage();
            SaveCurrentSealPlacement();
            RaisePropertyChanged();
            RaisePropertyChanged(nameof(SelectedVisibleSealRotation));
            ClearOutput();
            RaiseVisibleSealPreviewGeometryChanged();
            if (!_rotatingVisibleSeal)
            {
                ScheduleSealStampPreview();
            }
            UpdateValidationMessage();
        }
    }

    public void BeginVisibleSealRotation()
    {
        _rotatingVisibleSeal = true;
        CancelSealStampPreview();
    }

    public void EndVisibleSealRotation()
    {
        if (!_rotatingVisibleSeal) return;
        _rotatingVisibleSeal = false;
        ScheduleSealStampPreview();
    }

    public bool PerPageSealEnabled
    {
        get => _perPageSealEnabled;
        set
        {
            if (value && !_perPageSealEnabled)
            {
                var pages = SelectedPlacementPages();
                if (pages.Count == 0 || pages.Count > 128)
                {
                    ValidationMessage = GrxFirma.WinUI.Services.SealUiCatalog.Text(_sealUiLanguage, "sign.seal.page_limit");
                    RaisePropertyChanged();
                    return;
                }
                var geometry = CurrentSealPlacement();
                _sealPlacements.Clear();
                foreach (var page in pages)
                {
                    _sealPlacements[page] = geometry with { Page = page };
                }
                _requestedPreviewPage = _previewCurrentPage;
            }
            if (SetProperty(ref _perPageSealEnabled, value))
            {
                RaisePropertyChanged(nameof(HasSealOnPreviewPage));
                RaisePropertyChanged(nameof(SealPageToggleLabel));
                UpdateValidationMessage();
            }
        }
    }

    public bool HasSealOnPreviewPage =>
        !_perPageSealEnabled || _sealPlacements.ContainsKey(_previewCurrentPage);

    public string SealPageToggleLabel => SealText(HasSealOnPreviewPage ? "sign.seal.remove_this_page" : "sign.seal.add_this_page");

    public string SealOneByOneLabel => SealText("sign.seal.one_by_one");
    public string SealApplyAllLabel => SealText("sign.seal.apply_all_pages");
    public string SealPreviousPageLabel => SealText("sign.seal.previous_page");
    public string SealNextPageLabel => SealText("sign.seal.next_page");
    public bool CanDrawVisibleSealArea => _isPreviewImageRendered &&
        !VisibleSealPreviewImage.IsEmpty && _previewCurrentPage > 0 &&
        (_requestedPreviewPage <= 0 || _previewCurrentPage == _requestedPreviewPage) &&
        PathsEqual(_previewInputPath ?? string.Empty, _inputPath ?? string.Empty);

    public string VisibleSealDrawAreaLabel => SealText("sign.seal.draw_area");
    public string VisibleSealDrawAreaHelp => SealText("sign.seal.draw_help");

    public bool ApplyDrawnSealArea(SealDrawRect rect)
    {
        if (!SealDrawGeometry.IsLargeEnough(rect) ||
            !double.IsFinite(rect.X) || !double.IsFinite(rect.Y) ||
            !double.IsFinite(rect.Width) || !double.IsFinite(rect.Height) ||
            rect.X < 0 || rect.Y < 0 || rect.X + rect.Width > 1 + 1e-12 ||
            rect.Y + rect.Height > 1 + 1e-12) return false;
        // La geometría se cambia de una vez: los setters individuales limitan
        // la tarjeta girada y podrían desplazarla con un tamaño intermedio.
        _visibleSealXPercent = rect.X * 100;
        _visibleSealYPercent = rect.Y * 100;
        _visibleSealWidthPercent = rect.Width * 100;
        _visibleSealHeightPercent = rect.Height * 100;
        if (_perPageSealEnabled && _previewCurrentPage > 0)
        {
            _sealPlacements[_previewCurrentPage] = CurrentSealPlacement();
            RaisePropertyChanged(nameof(HasSealOnPreviewPage));
            RaisePropertyChanged(nameof(SealPageToggleLabel));
        }
        OnVisibleSealGeometryChanged();
        foreach (var name in new[] { nameof(VisibleSealXPercent), nameof(VisibleSealYPercent),
            nameof(VisibleSealWidthPercent), nameof(VisibleSealHeightPercent) })
            RaisePropertyChanged(name);
        return true;
    }

    public string VisibleSealRotateHandleLabel => SealText("sign.seal.rotate_handle");
    public string VisibleSealRotateHandleHelp => SealText("sign.seal.rotate_help");

    private string SealText(string key) => GrxFirma.WinUI.Services.SealUiCatalog.Text(_sealUiLanguage, key);

    // Preferencia «Idioma del sello» de la configuración (signSealLanguage).
    public void SetSealLanguagePreference(string? preference)
    {
        if (string.Equals(
            _sealLanguagePreference,
            preference,
            StringComparison.Ordinal))
        {
            return;
        }
        _sealLanguagePreference = preference;
        if (VisibleSealEnabled)
        {
            ScheduleSealStampPreview();
        }
    }

    public void SetSealUiLanguage(string? language)
    {
        _sealUiLanguage = language ?? "es";
        foreach (var name in new[] { nameof(VisibleSealDrawAreaLabel), nameof(VisibleSealDrawAreaHelp), nameof(SealOneByOneLabel), nameof(SealApplyAllLabel), nameof(SealPreviousPageLabel), nameof(SealNextPageLabel), nameof(SealPageToggleLabel), nameof(VisibleSealPageSummary), nameof(VisibleSealRotateHandleLabel), nameof(VisibleSealRotateHandleHelp), nameof(VisibleSealCsvLabel), nameof(VisibleSealCsvNotice), nameof(VisibleSealCsvCodeLabel), nameof(VisibleSealCsvUrlLabel), nameof(VisibleSealCsvTextLabel), nameof(VisibleSealCsvQrLabel) })
            RaisePropertyChanged(name);
    }

    public string VisibleSealPageSummary =>
        _previewTotalPages > 0
            ? SealText("sign.seal.page_counter").Replace("%1", _previewCurrentPage.ToString(System.Globalization.CultureInfo.CurrentCulture)).Replace("%2", _previewTotalPages.ToString(System.Globalization.CultureInfo.CurrentCulture))
            : string.Empty;

    public bool CanGoToPreviousSealPage => _previewCurrentPage > 1;
    public bool CanGoToNextSealPage => _previewCurrentPage > 0 && _previewCurrentPage < _previewTotalPages;

    public async Task<OperationDiagnostic?> NavigateVisibleSealPageAsync(int step, CancellationToken cancellationToken = default)
    {
        if (_previewTotalPages <= 0) return null;
        var page = Math.Clamp(_previewCurrentPage + step, 1, _previewTotalPages);
        if (page == _previewCurrentPage) return null;
        SaveCurrentSealPlacement();
        _requestedPreviewPage = page;
        LoadSealPlacement(page);
        return await RefreshVisibleSealPreviewPageAsync(cancellationToken);
    }

    public void ApplySealToAllPages()
    {
        if (!PerPageSealEnabled) PerPageSealEnabled = true;
        if (!PerPageSealEnabled) return;
        if (_previewTotalPages is < 1 or > 128)
        {
            ValidationMessage = SealText("sign.seal.page_limit");
            return;
        }
        var geometry = CurrentSealPlacement();
        _sealPlacements.Clear();
        for (var page = 1; page <= _previewTotalPages; page++) _sealPlacements[page] = geometry with { Page = page };
        _selectedVisibleSealPageMode = VisibleSealPageModes.First(option => option.Value == "all");
        _visibleSealPages = "all";
        RaisePropertyChanged(nameof(SelectedVisibleSealPageMode));
        RaisePropertyChanged(nameof(IsVisibleSealCustomPages));
        RaisePropertyChanged(nameof(VisibleSealPages));
        RaisePropertyChanged(nameof(HasSealOnPreviewPage));
        RaisePropertyChanged(nameof(SealPageToggleLabel));
        UpdateValidationMessage();
    }

    public void ToggleSealOnCurrentPage()
    {
        if (!PerPageSealEnabled || _previewCurrentPage < 1) return;
        if (!_sealPlacements.Remove(_previewCurrentPage)) _sealPlacements[_previewCurrentPage] = CurrentSealPlacement();
        RaisePropertyChanged(nameof(HasSealOnPreviewPage));
        RaisePropertyChanged(nameof(SealPageToggleLabel));
        UpdateValidationMessage();
    }

    private VisibleSealPlacementParameters CurrentSealPlacement() => new()
    {
        Page = _previewCurrentPage,
        Rect = new VisibleSealRectParameters
        {
            X = VisibleSealXPercent / 100, Y = VisibleSealYPercent / 100,
            Width = VisibleSealWidthPercent / 100, Height = VisibleSealHeightPercent / 100,
        },
        Rotation = _visibleSealRotationDegrees,
    };

    private void SaveCurrentSealPlacement()
    {
        if (_perPageSealEnabled && !_loadingSealPage && _sealPlacements.ContainsKey(_previewCurrentPage))
            _sealPlacements[_previewCurrentPage] = CurrentSealPlacement();
    }

    private void LoadSealPlacement(int page)
    {
        if (!_perPageSealEnabled || !_sealPlacements.TryGetValue(page, out var placement)) return;
        _loadingSealPage = true;
        VisibleSealXPercent = placement.Rect.X * 100;
        VisibleSealYPercent = placement.Rect.Y * 100;
        VisibleSealWidthPercent = placement.Rect.Width * 100;
        VisibleSealHeightPercent = placement.Rect.Height * 100;
        VisibleSealRotationDegrees = placement.Rotation;
        _loadingSealPage = false;
    }

    private List<int> SelectedPlacementPages()
    {
        var pages = new List<int>();
        if (_previewTotalPages is < 1 or > 128 ||
            !TryParsePageSelection(VisibleSealPages, _previewTotalPages, out var selection, out _, out _)) return pages;
        if (selection == "all") return Enumerable.Range(1, _previewTotalPages).ToList();
        foreach (var part in selection.Split(','))
        {
            var bounds = part.Split('-');
            var first = int.Parse(bounds[0], System.Globalization.CultureInfo.InvariantCulture);
            var last = bounds.Length > 1 ? int.Parse(bounds[1], System.Globalization.CultureInfo.InvariantCulture) : first;
            for (var page = first; page <= last && pages.Count <= 128; page++)
                if (!pages.Contains(page)) pages.Add(page);
        }
        return pages;
    }

    public VisibleSealStyleOption? SelectedVisibleSealStyle
    {
        get => _selectedVisibleSealStyle;
        set
        {
            if (value is null ||
                !SetProperty(ref _selectedVisibleSealStyle, value))
            {
                return;
            }
            RaisePropertyChanged(nameof(IsVisibleSealCustomImageStyle));
            RaisePropertyChanged(nameof(HasVisibleSealLogo));
            RaisePropertyChanged(nameof(VisibleSealStyleDescription));
            RaisePropertyChanged(nameof(VisibleSealImageSummary));
            ClearOutput();
            ScheduleSealStampPreview();
            UpdateValidationMessage();
        }
    }

    public bool IsVisibleSealCustomImageStyle =>
        string.Equals(
            SelectedVisibleSealStyle?.Value,
            "imagen",
            StringComparison.Ordinal);

    public bool HasVisibleSealLogo =>
        SelectedVisibleSealStyle?.Value is "institucional" or "imagen";

    public string VisibleSealStyleDescription =>
        SelectedVisibleSealStyle?.Description ?? string.Empty;

    public string VisibleSealImageSummary =>
        string.IsNullOrWhiteSpace(_visibleSealImagePath)
            ? Localizer.Text("winui.firmar.ninguna_imagen_elegida")
            : Localizer.Format("winui.firmar.imagen",
                Path.GetFileName(_visibleSealImagePath));

    public string VisibleSealCsvLabel => SealText("paridad.lote3.csv.enable");
    public string VisibleSealCsvNotice => SealText("paridad.lote3.csv.notice");
    public string VisibleSealCsvCodeLabel => SealText("paridad.lote3.csv.code");
    public string VisibleSealCsvUrlLabel => SealText("paridad.lote3.csv.url");
    public string VisibleSealCsvTextLabel => SealText("paridad.lote3.csv.text_optional");
    public string VisibleSealCsvQrLabel => SealText("paridad.lote3.csv.qr");
    public bool VisibleSealCsvEnabled { get => _visibleSealCsvEnabled; set { if (SetProperty(ref _visibleSealCsvEnabled, value)) { ClearOutput(); UpdateValidationMessage(); } } }
    public string VisibleSealCsvCode { get => _visibleSealCsvCode; set { if (SetProperty(ref _visibleSealCsvCode, value ?? string.Empty)) ClearOutput(); } }
    public string VisibleSealCsvUrl { get => _visibleSealCsvUrl; set { if (SetProperty(ref _visibleSealCsvUrl, value ?? string.Empty)) ClearOutput(); } }
    public string VisibleSealCsvText { get => _visibleSealCsvText; set { if (SetProperty(ref _visibleSealCsvText, value ?? string.Empty)) ClearOutput(); } }
    public bool VisibleSealCsvQr { get => _visibleSealCsvQr; set { if (SetProperty(ref _visibleSealCsvQr, value)) ClearOutput(); } }

    public bool VisibleSealQrEnabled
    {
        get => _visibleSealQrEnabled;
        set
        {
            if (SetProperty(ref _visibleSealQrEnabled, value))
            {
                ClearOutput();
                ScheduleSealStampPreview();
                UpdateValidationMessage();
            }
        }
    }

    public string VisibleSealQrUrl
    {
        get => _visibleSealQrUrl;
        set
        {
            if (SetProperty(ref _visibleSealQrUrl, value ?? string.Empty))
            {
                ClearOutput();
                ScheduleSealStampPreview();
                UpdateValidationMessage();
            }
        }
    }

    public VisibleSealPageModeOption? SelectedVisibleSealPageMode
    {
        get => _selectedVisibleSealPageMode;
        set
        {
            if (value is null ||
                !SetProperty(ref _selectedVisibleSealPageMode, value))
            {
                return;
            }
            RaisePropertyChanged(nameof(IsVisibleSealCustomPages));
            switch (value.Value)
            {
                case "first":
                case "last":
                    // La última página se concreta al cargar la
                    // previsualización, cuando se conoce el total.
                    VisibleSealPages = "1";
                    break;
                case "all":
                    VisibleSealPages = "all";
                    break;
            }
            ClearOutput();
            UpdateValidationMessage();
        }
    }

    public bool IsVisibleSealCustomPages =>
        string.Equals(
            SelectedVisibleSealPageMode?.Value,
            "custom",
            StringComparison.Ordinal);

    public ReadOnlyMemory<byte> VisibleSealStampImage
    {
        get => _visibleSealStampImage;
        private set
        {
            if (_visibleSealStampImage.Equals(value))
            {
                return;
            }
            _visibleSealStampImage = value;
            RaisePropertyChanged();
            RaisePropertyChanged(nameof(HasVisibleSealStamp));
            RaisePropertyChanged(nameof(ShowVisibleSealMock));
        }
    }

    public bool HasVisibleSealStamp => !_visibleSealStampImage.IsEmpty;

    public bool ShowVisibleSealMock => _visibleSealStampImage.IsEmpty;

    public string VisibleSealStampMessage
    {
        get => _visibleSealStampMessage;
        private set => SetProperty(ref _visibleSealStampMessage, value);
    }

    public string SignatureReason
    {
        get => _signatureReason;
        set
        {
            if (SetProperty(ref _signatureReason, value ?? string.Empty))
            {
                ClearOutput();
                InvalidateBatchResults();
                ScheduleSealStampPreview();
                UpdateValidationMessage();
            }
        }
    }

    public string SignatureLocation
    {
        get => _signatureLocation;
        set
        {
            if (SetProperty(ref _signatureLocation, value ?? string.Empty))
            {
                ClearOutput();
                InvalidateBatchResults();
                ScheduleSealStampPreview();
                UpdateValidationMessage();
            }
        }
    }

    public string SignatureContact
    {
        get => _signatureContact;
        set
        {
            if (SetProperty(ref _signatureContact, value ?? string.Empty))
            {
                ClearOutput();
                InvalidateBatchResults();
                ScheduleSealStampPreview();
                UpdateValidationMessage();
            }
        }
    }

    public string VisibleSealPreviewMessage
    {
        get => _visibleSealPreviewMessage;
        private set => SetProperty(ref _visibleSealPreviewMessage, value);
    }

    public ReadOnlyMemory<byte> VisibleSealPreviewImage
    {
        get => _visibleSealPreviewImage;
        private set
        {
            if (_visibleSealPreviewImage.Equals(value))
            {
                return;
            }
            var previous = _visibleSealPreviewImage;
            _visibleSealPreviewImage = value;
            RaisePropertyChanged();
            ClearMemory(previous);
        }
    }

    public double PreviewCanvasWidth =>
        IsPositiveFinite(_previewPageWidth)
            ? _previewPageWidth
            : DefaultPreviewPageWidth;

    public double PreviewCanvasHeight =>
        IsPositiveFinite(_previewPageHeight)
            ? _previewPageHeight
            : DefaultPreviewPageHeight;

    public double VisibleSealPreviewX =>
        PercentToPoints(VisibleSealXPercent, PreviewCanvasWidth);

    public double VisibleSealPreviewTop =>
        Math.Max(
            0,
            PreviewCanvasHeight -
                PercentToPoints(
                    VisibleSealYPercent + VisibleSealHeightPercent,
                    PreviewCanvasHeight));

    public double VisibleSealPreviewWidth =>
        PercentToPoints(VisibleSealWidthPercent, PreviewCanvasWidth);

    public double VisibleSealPreviewHeight =>
        PercentToPoints(VisibleSealHeightPercent, PreviewCanvasHeight);

    public double VisibleSealPreviewRotation => _visibleSealRotationDegrees;

    public double VisibleSealPreviewContentWidth =>
        Math.Max(1, VisibleSealPreviewWidth - 16);

    public double VisibleSealPreviewContentHeight =>
        Math.Max(1, VisibleSealPreviewHeight - 16);

    public string VisibleSealPreviewText => VisibleSealKeepText
        ? (_portalSealMode && !string.IsNullOrWhiteSpace(_portalSignerName)
            ? _portalSignerName : Localizer.Text("winui.firmar.identidad_ubicacion_fecha"))
        : Localizer.Text("winui.firmar.rotulo_sin_datos_personales");

    public void ConfigurePortalSealDocument(string path, string signerName)
    {
        _portalSealMode = true;
        _portalSignerName = signerName;
        _inputPath = path;
        _ = UpdateVeriFactuFormatAsync(path);
        _visibleSealPages = "1";
        _requestedPreviewPage = 1;
        InputDisplayName = SafeFileName(path);
        VisibleSealEnabled = true;
        RaisePropertyChanged(nameof(VisibleSealPreviewText));
    }

    public VisibleSealPlacementParameters? PortalSealPlacement() =>
        _portalSealMode && _isPreviewImageRendered &&
        _previewCurrentPage is > 0 &&
        _previewCurrentPage <= _previewTotalPages &&
        !VisibleSealPreviewImage.IsEmpty &&
        !string.IsNullOrWhiteSpace(_previewInputPath) &&
        PathsEqual(_previewInputPath, _inputPath ?? string.Empty)
            ? CurrentSealPlacement() : null;

    public int PortalCurrentPage => _previewCurrentPage;
    public int PortalTotalPages => _previewTotalPages;

    public string VisibleSealGeometrySummary =>
        Localizer.Format("winui.firmar.tamano_mm_giro",
            (VisibleSealPreviewWidth * 25.4 / 72).ToString("0.#",
                System.Globalization.CultureInfo.CurrentCulture),
            (VisibleSealPreviewHeight * 25.4 / 72).ToString("0.#",
                System.Globalization.CultureInfo.CurrentCulture),
            VisibleSealPreviewRotation.ToString("0",
                System.Globalization.CultureInfo.CurrentCulture));

    public string VisibleSealReadabilityHint =>
        _visibleSealRotationDegrees % 180 != 0 &&
        VisibleSealPreviewWidth > VisibleSealPreviewHeight
            ? Localizer.Text("winui.firmar.para_leer_mejor_el_texto_girado_use_una")
            : Math.Min(VisibleSealPreviewWidth, VisibleSealPreviewHeight) < 28
                ? Localizer.Text("winui.firmar.la_zona_es_pequena_ampliela_si_el_texto")
                : Localizer.Text("winui.firmar.deje_libre_el_texto_del_documento_y");

    public string? OutputPath => _outputPath;

    public void UpdateAvailability()
    {
        var available =
            _session.Supports(DesktopOperationActions.Certificates) &&
            _session.Supports(DesktopOperationActions.Sign);
        SetOperationAvailability(
            available,
            Localizer.Text("winui.firmar.motor_local_conectado_la_firma_y_el"));
        if (!available)
        {
            CancelCurrentOperation();
            Certificates = [];
            SelectedCertificate = null;
            DiscardPreparedCredential();
            ClearVisibleSealPreview();
        }
        if (!_session.Supports(DesktopOperationActions.Verify))
        {
            ValidateAfterSigning = false;
        }
        if (BatchModeEnabled &&
            !_session.Supports(DesktopOperationActions.SignBatch))
        {
            _batchModeEnabled = false;
            RaisePropertyChanged(nameof(BatchModeEnabled));
        }
        RefreshGuidedMultiCosignState();
        ScheduleSealStampPreview();
        UpdateValidationMessage();
        UpdateCommandStates();
    }

    public void SetSelectedAdditionalSigners(
        IEnumerable<CertificateListItem>? certificates)
    {
        var selectedIds = new HashSet<string>(
            (certificates ?? [])
                .Where(certificate => certificate is not null)
                .Select(certificate => certificate.Id),
            StringComparer.Ordinal);
        _selectedAdditionalCertificateIds = AdditionalSignerCandidates
            .Where(certificate => selectedIds.Contains(certificate.Id))
            .Select(certificate => certificate.Id)
            .Distinct(StringComparer.Ordinal)
            .Take(MaximumCatalogItems)
            .ToArray();
        ClearOutput();
        InvalidateBatchResults();
        UpdateGuidedMultiCosignSummary();
        UpdateValidationMessage();
        UpdateCommandStates();
    }

    public void CancelCurrentOperation()
    {
        CompletedSignPresentationId = Guid.Empty;
        try
        {
            Volatile.Read(ref _activeCancellation)?.Cancel();
        }
        catch (ObjectDisposedException)
        {
            // La operación ya terminó y liberó su token.
        }
    }

    public async Task<OperationDiagnostic?> RefreshCertificatesAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.espere_a_que_termine_la_operacion_actual");
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.Certificates,
                out var operations))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_pueden_cargar_certificados_el");
                return null;
            }

            ValidationMessage = Localizer.Text("winui.firmar.consultando_certificados_aptos_para");
            var result = await operations.GetCertificatesAsync(
                operationCancellation.Token);
            if (!result.IsSuccess ||
                !string.Equals(
                    result.Outcome,
                    "success",
                    StringComparison.Ordinal))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_motor_local_no_pudo_cargar_los");
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null)
            {
                ValidationMessage =
                    Localizer.Text("winui.comun.el_motor_confirmo_la_consulta_pero_no");
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_CERTIFICATE_CATALOG");
            }

            ReplaceCertificateCatalog(
                result.Data,
                SelectedCertificate?.Id);
            UpdateValidationMessage();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_consulta_de_certificados_se_cancelo");
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.fallo_la_comunicacion_segura_al_cargar");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_aplicacion_no_pudo_cargar_los");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?> SelectCredentialFileAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.espere_a_que_termine_la_operacion_actual_2");
            return null;
        }

        try
        {
            var selectedPath = await _filePicker.PickOpenFileAsync(
                OpenFilePickerProfile.Certificate,
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(selectedPath))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_selecciono_ninguna_credencial");
                return null;
            }

            DesktopCertificateCredentialFile.ValidateSelection(
                selectedPath);
            _selectedCredentialPath = selectedPath;
            ValidationMessage =
                Localizer.Format("winui.firmar.preparada_introduzca_su_contrasena_para", DesktopCertificateCredentialFile.SafeDisplayName(selectedPath));
            RaisePropertyChanged(nameof(HasPreparedCredential));
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_seleccion_de_la_credencial_se_cancelo");
            return null;
        }
        catch (InvalidDataException exception)
        {
            DiscardPreparedCredential();
            ValidationMessage = exception.Message;
            return null;
        }
        catch (Exception exception)
        {
            DiscardPreparedCredential();
            ValidationMessage =
                Localizer.Text("winui.firmar.no_se_pudo_preparar_la_credencial");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    /// <summary>
    /// Toma propiedad del buffer de contraseña y lo borra siempre.
    /// </summary>
    public async Task<OperationDiagnostic?> UsePreparedCredentialAsync(
        byte[] password,
        bool importIntoWindows,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(password);
        byte[]? credential = null;
        CancellationTokenSource? operationCancellation = null;
        try
        {
            var path = _selectedCredentialPath;
            if (string.IsNullOrWhiteSpace(path))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.seleccione_primero_una_credencial_p12");
                return null;
            }
            if (password.Length >
                DesktopOperationsClient.MaximumPasswordBytes)
            {
                ValidationMessage =
                    Localizer.Text("winui.comun.la_contrasena_supera_el_limite_de");
                return null;
            }
            if (!TryBeginOperation(
                cancellationToken,
                out operationCancellation))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.espere_a_que_termine_la_operacion_actual_3");
                return null;
            }

            var action = importIntoWindows
                ? DesktopOperationActions.ImportCertificateToStore
                : DesktopOperationActions.UseTemporaryCertificate;
            if (!_session.TryGetOperations(action, out var operations))
            {
                ValidationMessage = importIntoWindows
                    ? Localizer.Text("winui.firmar.el_motor_local_no_permite_importar_en_el")
                    : Localizer.Text("winui.firmar.el_motor_local_no_permite_usar");
                return null;
            }

            credential =
                await DesktopCertificateCredentialFile.ReadAsync(
                    path,
                    operationCancellation.Token);
            string? preferredCertificateId = null;
            if (importIntoWindows)
            {
                if (!_session.Supports(
                    DesktopOperationActions.CertificateAccessOptions))
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.el_motor_no_permite_comprobar_el_almacen");
                    return null;
                }

                var options =
                    await operations.GetCertificateImportOptionsAsync(
                        operationCancellation.Token);
                if (!IsSuccessful(options))
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.no_se_pudo_comprobar_el_almacen_oficial");
                    return OperationDiagnosticMapper.FromResult(options);
                }
                var windowsTarget = options.Data?.ImportTargets
                    .FirstOrDefault(target => string.Equals(
                        target.Id,
                        WindowsPersonalStoreTargetId,
                        StringComparison.Ordinal));
                if (windowsTarget is null)
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.el_almacen_personal_oficial_de_windows");
                    return null;
                }

                ValidationMessage =
                    Localizer.Text("winui.firmar.importando_la_credencial_en_el_almacen");
                var importResult =
                    await operations.ImportCertificateToStoreAsync(
                        new ImportCertificateToStoreParameters
                        {
                            CredentialB64 = credential,
                            PasswordB64 = password,
                            TargetId = windowsTarget.Id,
                        },
                        operationCancellation.Token);
                if (!IsSuccessful(importResult) ||
                    string.IsNullOrWhiteSpace(importResult.Data))
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.windows_no_confirmo_la_importacion_de_la");
                    return OperationDiagnosticMapper.FromResult(
                        importResult);
                }
            }
            else
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.cargando_la_credencial_solo_para_esta");
                var temporaryResult =
                    await operations.UseTemporaryCertificateAsync(
                        new UseTemporaryCertificateParameters
                        {
                            CredentialB64 = credential,
                            PasswordB64 = password,
                        },
                        operationCancellation.Token);
                if (!IsSuccessful(temporaryResult))
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.no_se_pudo_cargar_la_credencial_temporal");
                    return OperationDiagnosticMapper.FromResult(
                        temporaryResult);
                }
                if (temporaryResult.Data is null ||
                    !temporaryResult.Data.Temporary ||
                    string.IsNullOrWhiteSpace(temporaryResult.Data.Id))
                {
                    ValidationMessage =
                        Localizer.Text("winui.comun.el_motor_no_confirmo_una_credencial");
                    return InvalidResultDiagnostic(
                        temporaryResult,
                        "MISSING_TEMPORARY_CERTIFICATE");
                }

                preferredCertificateId = temporaryResult.Data.Id;
                _session.TrackTemporaryCertificate(
                    preferredCertificateId);
            }

            var catalogResult = await operations.GetCertificatesAsync(
                operationCancellation.Token);
            if (!IsSuccessful(catalogResult) ||
                catalogResult.Data is null)
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_credencial_se_anadio_pero_no_se_pudo");
                return OperationDiagnosticMapper.FromResult(catalogResult);
            }

            var previousId = preferredCertificateId ??
                SelectedCertificate?.Id;
            ReplaceCertificateCatalog(
                catalogResult.Data,
                previousId);
            if (preferredCertificateId is not null &&
                !string.Equals(
                    SelectedCertificate?.Id,
                    preferredCertificateId,
                    StringComparison.Ordinal))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_credencial_temporal_se_cargo_pero_no");
                return InvalidResultDiagnostic(
                    catalogResult,
                    "TEMPORARY_CERTIFICATE_NOT_IN_CATALOG");
            }

            DiscardPreparedCredential();
            ValidationMessage = importIntoWindows
                ? Localizer.Text("winui.firmar.credencial_importada_en_el_almacen")
                : Localizer.Text("winui.firmar.credencial_temporal_cargada_y");
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_carga_de_la_credencial_se_cancelo");
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation?.Token ?? cancellationToken);
        }
        catch (IpcClientException exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.fallo_la_comunicacion_segura_al_cargar_2");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.no_se_pudo_leer_o_cargar_la_credencial");
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

    public void DiscardPreparedCredential()
    {
        if (_selectedCredentialPath is null)
        {
            return;
        }

        _selectedCredentialPath = null;
        RaisePropertyChanged(nameof(HasPreparedCredential));
        UpdateCommandStates();
    }

    public OperationDiagnostic ReportCredentialCaptureFailure(
        Exception exception)
    {
        ArgumentNullException.ThrowIfNull(exception);
        ValidationMessage = exception is
            SecurePasswordPromptException promptException
            ? Localizer.Format(
                "winui.firmar.no_se_pudo_capturar_la_contrasena_de_la",
                promptException.SupportCode)
            : Localizer.Text("winui.firmar.no_se_pudo_capturar_la_contrasena_de_la_2");
        return OperationDiagnosticMapper.FromException(exception);
    }

    public async Task<OperationDiagnostic?> SelectDocumentAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.espere_a_que_termine_la_operacion_actual_4");
            return null;
        }

        try
        {
            var selectedPath = await _filePicker.PickOpenFileAsync(
                OpenFilePickerProfile.SignedOrOriginalDocument,
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(selectedPath))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_selecciono_ningun_documento_la");
                return null;
            }

            if (!PathsEqual(_inputPath ?? string.Empty, selectedPath))
            {
                _sealPlacements.Clear();
                _perPageSealEnabled = false;
                _requestedPreviewPage = 0;
                RaisePropertyChanged(nameof(PerPageSealEnabled));
                RaisePropertyChanged(nameof(HasSealOnPreviewPage));
            }
            _inputPath = selectedPath;
            _ = UpdateVeriFactuFormatAsync(selectedPath);
            InputDisplayName = SafeFileName(selectedPath);
            ClearOutput();
            ClearVisibleSealPreview();
            RefreshGuidedMultiCosignState();
            UpdateValidationMessage();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_seleccion_del_documento_se_cancelo_no");
            return null;
        }
        catch (Exception exception)
        {
            ValidationMessage =
                Localizer.Text("winui.comun.no_se_pudo_abrir_el_selector_de");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?> SelectBatchFilesAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.espere_a_que_termine_la_operacion_actual_5");
            return null;
        }

        try
        {
            var selectedPaths = await _filePicker.PickOpenFilesAsync(
                OpenFilePickerProfile.SignedOrOriginalDocument,
                operationCancellation.Token);
            if (selectedPaths.Count == 0)
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_anadieron_documentos_al_lote");
                return null;
            }

            if (!TryNormalizeBatchPaths(
                _batchExplicitInputPaths.Concat(selectedPaths),
                out var explicitPaths,
                out var selectionError) ||
                !TryCombineBatchInputs(
                    explicitPaths,
                    _batchDirectoryInputPaths,
                    out var combinedPaths,
                    out selectionError))
            {
                ValidationMessage = selectionError;
                return null;
            }

            _batchExplicitInputPaths = explicitPaths;
            ApplyBatchSelection(combinedPaths);
            ValidationMessage =
                Localizer.Format("winui.firmar.documento_s_preparados_seleccione_la", combinedPaths.Count);
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_seleccion_de_documentos_para_el_lote");
            return null;
        }
        catch (Exception exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.no_se_pudo_completar_la_seleccion_de");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?> SelectBatchFolderAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.espere_a_que_termine_la_operacion_actual_5");
            return null;
        }

        try
        {
            var selectedDirectory = await _filePicker.PickFolderAsync(
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(selectedDirectory))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_anadio_ninguna_carpeta_al_lote");
                return null;
            }
            if (!TryEnumerateBatchDirectory(
                selectedDirectory,
                out var directoryPath,
                out var directoryInputs,
                out var directoryError) ||
                !TryCombineBatchInputs(
                    _batchExplicitInputPaths,
                    directoryInputs,
                    out var combinedPaths,
                    out directoryError))
            {
                ValidationMessage = directoryError;
                return null;
            }

            _batchSourceDirectory = directoryPath;
            _batchDirectoryInputPaths = directoryInputs;
            ApplyBatchSelection(combinedPaths);
            ValidationMessage =
                Localizer.Format("winui.firmar.documento_s_preparados_incluidos_los", combinedPaths.Count);
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_seleccion_de_la_carpeta_del_lote_se");
            return null;
        }
        catch (Exception exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.no_se_pudo_leer_la_carpeta_seleccionada");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?> SelectBatchOutputDirectoryAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.espere_a_que_termine_la_operacion_actual_6");
            return null;
        }

        try
        {
            var selectedDirectory = await _filePicker.PickFolderAsync(
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(selectedDirectory))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_cambio_la_carpeta_de_salida_del");
                return null;
            }

            var fullPath = Path.GetFullPath(selectedDirectory);
            if (!Directory.Exists(fullPath))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_carpeta_de_salida_seleccionada_ya_no");
                return null;
            }

            _batchOutputDirectory = fullPath;
            BatchOutputSummary = Localizer.Format("winui.firmar.salida",
                SafeDirectoryName(fullPath));
            ResetBatchResults();
            UpdateValidationMessage();
            UpdateCommandStates();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_seleccion_de_la_carpeta_de_salida_se");
            return null;
        }
        catch (Exception exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.no_se_pudo_seleccionar_la_carpeta_de");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public void ClearBatchSelection()
    {
        if (IsBusy)
        {
            return;
        }

        _batchExplicitInputPaths = [];
        _batchDirectoryInputPaths = [];
        _batchInputPaths = [];
        _batchSourceDirectory = null;
        BatchInputSummary =
            Localizer.Text("winui.firmar.no_se_han_seleccionado_documentos_para");
        ResetBatchResults();
        UpdateValidationMessage();
        UpdateCommandStates();
    }

    public async Task<OperationDiagnostic?> SignBatchAsync(
        CancellationToken cancellationToken = default)
    {
        var validation = ValidateBeforeBatch(
            out var signatureReason,
            out var signatureLocation,
            out var signatureContact);
        if (validation is not null)
        {
            ValidationMessage = validation;
            return null;
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.ya_hay_una_operacion_en_curso_espere_o");
            return null;
        }

        var batchStarted = false;
        RemoteSigningSecrets? remoteSecrets = null;
        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.SignBatch,
                out var operations))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_motor_local_no_ofrece_la_operacion");
                return null;
            }

            var inputPaths = _batchInputPaths.ToArray();
            var outputDirectory = _batchOutputDirectory!;
            if (!TryNormalizeBatchPaths(
                inputPaths,
                out var currentPaths,
                out var inputError) ||
                currentPaths.Count != inputPaths.Length)
            {
                ValidationMessage = string.IsNullOrWhiteSpace(inputError)
                    ? Localizer.Text("winui.firmar.la_seleccion_del_lote_cambio_seleccione")
                    : inputError;
                return null;
            }
            if (!Directory.Exists(outputDirectory))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_carpeta_de_salida_del_lote_ya_no_esta");
                return null;
            }
            if (HasBatchOutputNameCollision(
                currentPaths,
                SelectedFormat!.Value,
                out var collisionName))
            {
                ValidationMessage =
                    Localizer.Format("winui.firmar.dos_documentos_generarian_la_misma", collisionName);
                return null;
            }

            var certificate = SelectedCertificate!;
            var action = SelectedAction!;
            var format = SelectedFormat!;
            var profile = SelectedProfile!;
            var useGuidedMultiCosign = GuidedMultiCosignEnabled;
            string? batchTsaUrl = null;
            if (_session.Supports(DesktopOperationActions.GetSettings))
            {
                var settings = await operations.GetSettingsAsync(operationCancellation.Token);
                if (!settings.IsSuccess || settings.Data is null)
                {
                    ValidationMessage = SealText("winui.parity.tsa.unavailable");
                    return null;
                }
                if (settings.Data.TsaEnabled == true)
                {
                    if (!TsaConfiguration.TryNormalize(true, settings.Data.TsaUrl, out var normalized))
                    {
                        ValidationMessage = SealText("winui.parity.tsa.invalid");
                        return null;
                    }
                    batchTsaUrl = normalized;
                }
            }
            var remote = await PrepareRemoteSigningAsync(
                certificate,
                batch: true,
                operationCancellation.Token,
                documentCount: currentPaths.Count,
                multiCosign: useGuidedMultiCosign);
            if (!remote.Proceed)
            {
                return null;
            }
            remoteSecrets = remote.Secrets;
            BatchItems = currentPaths
                .Select(path => new BatchSignDisplayItem(
                    SafeFileName(path),
                    Localizer.Text("winui.firmar.en_espera"),
                    Localizer.Text("winui.firmar.pendiente_de_respuesta_del_motor_local"),
                    false,
                    null))
                .ToArray();
            HasBatchResults = true;
            BatchProgressMaximum = currentPaths.Count;
            BatchProgressValue = 0;
            IsBatchProgressIndeterminate = true;
            BatchProgressText =
                Localizer.Format("winui.firmar.firmando_lote_de_documento_s", currentPaths.Count);
            ValidationMessage =
                Localizer.Text("winui.firmar.el_lote_esta_en_curso_puede_cancelarlo");
            batchStarted = true;

            var result = await operations.SignBatchAsync(
                new BatchSignParameters
                {
                    RemotePin = remoteSecrets?.Pin,
                    RemoteOtp = remoteSecrets?.Otp,
                    InputPaths = currentPaths,
                    // La carpeta se enumera antes de enviar para que la lista
                    // visible y la respuesta esperada sean exactamente iguales.
                    DirectoryPath = string.Empty,
                    OutputDirectory = outputDirectory,
                    CertificateId = certificate.Id,
                    CertificateIndex = 0,
                    AdditionalCertificateIds = useGuidedMultiCosign
                        ? _selectedAdditionalCertificateIds.ToArray()
                        : null,
                    Format = format.Value,
                    Action = action.Value,
                    Overwrite = "rename",
                    AllowInvalidPdf = false,
                    StrictCompatibility = false,
                    Reason = signatureReason,
                    Location = signatureLocation,
                    ContactInfo = signatureContact,
                    ExtraOptions = batchTsaUrl is null
                        ? new Dictionary<string, string>(StringComparer.Ordinal)
                            { ["profile"] = profile.Value }
                        : new Dictionary<string, string>(StringComparer.Ordinal)
                            { ["profile"] = profile.Value, ["tsaURL"] = batchTsaUrl },
                },
                operationCancellation.Token);

            IsBatchProgressIndeterminate = false;
            if (!result.IsSuccess ||
                result.Outcome is not ("success" or "partial"))
            {
                MarkBatchNotProcessed(
                    currentPaths,
                    Localizer.Text("winui.firmar.el_motor_no_confirmo_la_ejecucion_del"));
                ValidationMessage =
                    RemoteSigningFailureMessage(result.ErrorCode) ??
                    Localizer.Text("winui.firmar.el_lote_no_se_completo_abra_el");
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null)
            {
                MarkBatchNotProcessed(
                    currentPaths,
                    Localizer.Text("winui.firmar.el_motor_no_devolvio_resultados_por"));
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_motor_confirmo_la_peticion_pero_no");
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_BATCH_RESULT");
            }

            var protocolCoherent = TryApplyBatchResult(
                result.Data,
                currentPaths,
                outputDirectory,
                out var successCount,
                out var failureCount);
            BatchProgressValue = currentPaths.Count;
            BatchProgressText = Localizer.Format(
                "winui.firmar.lote_finalizado_correcto_s_fallido_s",
                successCount, failureCount);
            ValidationMessage = failureCount == 0 && protocolCoherent
                ? Localizer.Text("winui.firmar.todos_los_documentos_tienen_una_salida")
                : Localizer.Text("winui.firmar.el_lote_termino_con_incidencias_revise");

            if (!protocolCoherent)
            {
                return InvalidResultDiagnostic(
                    result,
                    "INCOHERENT_BATCH_RESULT");
            }
            if (failureCount > 0 ||
                string.Equals(
                    result.Outcome,
                    "partial",
                    StringComparison.Ordinal))
            {
                return OperationDiagnosticMapper.FromResult(result);
            }
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException exception)
        {
            if (batchStarted)
            {
                MarkPendingBatchAsCancelled();
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_lote_se_cancelo_solo_se_consideran");
            }
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            if (batchStarted)
            {
                MarkPendingBatchAsUnconfirmed();
            }
            ValidationMessage =
                Localizer.Text("winui.firmar.fallo_la_comunicacion_segura_durante_la");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            if (batchStarted)
            {
                MarkPendingBatchAsUnconfirmed();
            }
            ValidationMessage =
                Localizer.Text("winui.firmar.la_aplicacion_no_pudo_completar_la_firma");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            IsBatchProgressIndeterminate = false;
            remoteSecrets?.Dispose();
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?> RefreshVisibleSealPreviewAsync(
        CancellationToken cancellationToken = default)
    {
        if (!string.Equals(
            SelectedVisibleSealPageMode?.Value,
            "last",
            StringComparison.Ordinal))
        {
            return await RefreshVisibleSealPreviewPageAsync(
                cancellationToken);
        }

        // «Última página»: primero se abre la primera para conocer el total
        // y después se previsualiza la última, que puede tener otro tamaño.
        _visibleSealPages = "1";
        RaisePropertyChanged(nameof(VisibleSealPages));
        var diagnostic = await RefreshVisibleSealPreviewPageAsync(
            cancellationToken);
        if (diagnostic is not null || _previewTotalPages <= 0)
        {
            return diagnostic;
        }
        var lastPage = _previewTotalPages;
        if (lastPage > 1)
        {
            _visibleSealPages = lastPage.ToString(
                System.Globalization.CultureInfo.InvariantCulture);
            RaisePropertyChanged(nameof(VisibleSealPages));
            diagnostic = await RefreshVisibleSealPreviewPageAsync(
                cancellationToken);
            if (diagnostic is not null)
            {
                return diagnostic;
            }
        }
        VisibleSealPreviewMessage +=
            Localizer.Format("winui.firmar.ha_elegido_ultima_pagina_el_documento", lastPage, lastPage);
        return null;
    }

    private async Task<OperationDiagnostic?> RefreshVisibleSealPreviewPageAsync(
        CancellationToken cancellationToken)
    {
        var localValidation = ValidateVisibleSealPreviewRequest(
            out var inputPath,
            out var previewPage);
        if (localValidation is not null)
        {
            ValidationMessage = localValidation;
            return VisibleSealValidationDiagnostic(
                "VISIBLE_SEAL_PREVIEW_INPUT_INVALID",
                localValidation,
                Localizer.Text("winui.firmar.corrija_la_seleccion_del_pdf_o_de"),
                DiagnosticStepStatus.Failure);
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.ya_hay_otra_operacion_en_curso_espere_o");
            return null;
        }

        byte[]? previewDigestBefore = null;
        byte[]? previewDigestAfter = null;
        ReadOnlyMemory<byte> pendingPreviewImage =
            ReadOnlyMemory<byte>.Empty;
        try
        {
            DesktopOperationsClient? operations = null;
            if (_pdfPreview is null &&
                !_session.TryGetOperations(
                    DesktopOperationActions.PdfPreview,
                    out operations))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_motor_local_no_ofrece_la");
                return VisibleSealValidationDiagnostic(
                    "PDF_PREVIEW_NOT_AVAILABLE",
                    ValidationMessage,
                    Localizer.Text("winui.firmar.actualice_o_reinicie_el_motor_local_y"),
                    DiagnosticStepStatus.Skipped);
            }

            ClearVisibleSealPreview();
            ValidationMessage =
                Localizer.Format("winui.firmar.cargando_la_geometria_real_de_la_pagina", previewPage);
            if (!TryReadFileStamp(
                inputPath!,
                out var fileLengthBefore,
                out var fileWriteBefore))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_pudo_comprobar_el_pdf_antes_de");
                return VisibleSealValidationDiagnostic(
                    "PDF_PREVIEW_INPUT_UNAVAILABLE",
                    ValidationMessage,
                    Localizer.Text("winui.firmar.seleccione_de_nuevo_el_pdf_y_repita_la"),
                    DiagnosticStepStatus.Failure);
            }
            using var previewGuard = OpenPdfReadGuard(inputPath!);
            if (previewGuard is null)
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_pudo_bloquear_el_pdf_contra");
                return VisibleSealValidationDiagnostic(
                    "PDF_PREVIEW_INPUT_UNAVAILABLE",
                    ValidationMessage,
                    Localizer.Text("winui.firmar.cierre_el_programa_que_esta_modificando"),
                    DiagnosticStepStatus.Failure);
            }
            previewDigestBefore = await ComputeSha256Async(
                previewGuard,
                operationCancellation.Token);
            PdfPreviewResult? previewResult;
            IpcCallResult<PdfPreviewResult>? ipcResult = null;
            if (_pdfPreview is not null)
            {
                try
                {
                    previewResult =
                        await _pdfPreview.RenderPageAsync(
                            inputPath!,
                            previewPage,
                            operationCancellation.Token);
                }
                catch (OperationCanceledException)
                    when (operationCancellation.IsCancellationRequested)
                {
                    throw;
                }
                catch
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.windows_no_pudo_generar_la");
                    return VisibleSealValidationDiagnostic(
                        "WINDOWS_PDF_PREVIEW_FAILED",
                        ValidationMessage,
                        Localizer.Text("winui.firmar.compruebe_que_el_pdf_se_abre"),
                        DiagnosticStepStatus.Failure);
                }
            }
            else
            {
                ipcResult = await operations!.GetPdfPreviewAsync(
                    new PdfPreviewParameters
                    {
                        Path = inputPath!,
                        Page = previewPage,
                    },
                    operationCancellation.Token);
                if (!ipcResult.IsSuccess ||
                    !string.Equals(
                        ipcResult.Outcome,
                        "success",
                        StringComparison.Ordinal))
                {
                    ClearBytes(ipcResult.Data?.Data);
                    ValidationMessage =
                        Localizer.Text("winui.firmar.no_se_pudo_previsualizar_el_pdf_abra_el");
                    return OperationDiagnosticMapper.FromResult(ipcResult);
                }
                previewResult = ipcResult.Data;
            }
            if (!TryValidatePreviewResult(
                previewResult,
                previewPage,
                out pendingPreviewImage,
                out var previewError))
            {
                ValidationMessage = previewError;
                return ipcResult is null
                    ? VisibleSealValidationDiagnostic(
                        "INVALID_WINDOWS_PDF_PREVIEW_RESULT",
                        previewError,
                        Localizer.Text("winui.firmar.seleccione_de_nuevo_el_pdf_y_repita_la"),
                        DiagnosticStepStatus.Failure)
                    : InvalidResultDiagnostic(
                        ipcResult,
                        "INVALID_PDF_PREVIEW_RESULT");
            }
            previewGuard.Position = 0;
            previewDigestAfter = await ComputeSha256Async(
                previewGuard,
                operationCancellation.Token);
            if (!TryReadFileStamp(
                inputPath!,
                out var fileLengthAfter,
                out var fileWriteAfter) ||
                fileLengthAfter != fileLengthBefore ||
                fileWriteAfter != fileWriteBefore ||
                !CryptographicOperations.FixedTimeEquals(
                    previewDigestBefore,
                    previewDigestAfter))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_pdf_cambio_mientras_se_generaba_la");
                return VisibleSealValidationDiagnostic(
                    "PDF_CHANGED_DURING_PREVIEW",
                    ValidationMessage,
                    Localizer.Text("winui.firmar.cierre_el_programa_que_esta_modificando_2"),
                    DiagnosticStepStatus.Failure);
            }

            _previewInputPath = inputPath;
            _previewFileLength = fileLengthAfter;
            _previewFileLastWriteUtcTicks = fileWriteAfter;
            ClearBytes(_previewFileDigest);
            _previewFileDigest = previewDigestAfter;
            previewDigestAfter = null;
            _previewPageWidth = previewResult!.Width;
            _previewPageHeight = previewResult.Height;
            _previewCurrentPage = previewResult.CurrentPage;
            _previewTotalPages = previewResult.TotalPages;
            RaisePropertyChanged(nameof(VisibleSealPageSummary));
            RaisePropertyChanged(nameof(CanGoToPreviousSealPage));
            RaisePropertyChanged(nameof(CanGoToNextSealPage));
            RaisePropertyChanged(nameof(HasSealOnPreviewPage));
            RaisePropertyChanged(nameof(SealPageToggleLabel));
            VisibleSealPreviewImage = pendingPreviewImage;
            pendingPreviewImage = ReadOnlyMemory<byte>.Empty;
            RaiseVisibleSealPreviewGeometryChanged();
            ScheduleSealStampPreview();

            if (!TryParsePageSelection(
                VisibleSealPages,
                _previewTotalPages,
                out var normalizedPages,
                out _,
                out var pageError))
            {
                ClearVisibleSealPreview();
                ValidationMessage = pageError;
                return VisibleSealValidationDiagnostic(
                    "VISIBLE_SEAL_PAGE_SELECTION_INVALID",
                    pageError,
                    Localizer.Text("winui.firmar.indique_paginas_existentes_por_ejemplo_1"),
                    DiagnosticStepStatus.Failure);
            }

            _visibleSealPages = normalizedPages;
            RaisePropertyChanged(nameof(VisibleSealPages));
            VisibleSealPreviewMessage = Localizer.Format(
                "winui.firmar.pdf_real_pagina_de_puntos_el_sello_se",
                _previewCurrentPage, _previewTotalPages,
                _previewPageWidth.ToString("0.##",
                    System.Globalization.CultureInfo.CurrentCulture),
                _previewPageHeight.ToString("0.##",
                    System.Globalization.CultureInfo.CurrentCulture));
            ValidationMessage =
                Localizer.Text("winui.firmar.previsualizacion_preparada_revise");
            UpdateCommandStates();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_carga_de_la_previsualizacion_se");
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.fallo_la_comunicacion_segura_al");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_aplicacion_no_pudo_preparar_la");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            ClearBytes(previewDigestBefore);
            ClearBytes(previewDigestAfter);
            ClearMemory(pendingPreviewImage);
            EndOperation(operationCancellation);
        }
    }

    public async Task<OperationDiagnostic?> SignAsync(
        CancellationToken cancellationToken = default)
    {
        var validation = ValidateBeforeSign();
        if (validation is not null)
        {
            ValidationMessage = validation;
            return null;
        }
        if (!TryBuildVisibleSeal(
            out var visibleSeal,
            out var signatureReason,
            out var signatureLocation,
            out var signatureContact,
            out var sealQrContent,
            out var sealExtraOptions,
            out var visibleSealErrorCode,
            out var visibleSealError))
        {
            ValidationMessage = visibleSealError;
            return VisibleSealValidationDiagnostic(
                visibleSealErrorCode,
                visibleSealError,
                Localizer.Text("winui.firmar.corrija_la_configuracion_indicada_y"),
                DiagnosticStepStatus.Failure);
        }
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.ya_hay_una_firma_en_curso_espere_o");
            return null;
        }

        ClearOutput();
        var postValidationStarted = false;
        FileStream? signInputGuard = null;
        byte[]? signInputDigest = null;
        byte[]? expectedSignInputDigest = null;
        RemoteSigningSecrets? remoteSecrets = null;
        try
        {
            var useGuidedMultiCosign = GuidedMultiCosignEnabled;
            var requiredAction = useGuidedMultiCosign
                ? DesktopOperationActions.SignMultiCosign
                : DesktopOperationActions.Sign;
            if (!_session.TryGetOperations(
                requiredAction,
                out var operations))
            {
                ValidationMessage = useGuidedMultiCosign
                    ? Localizer.Text("winui.firmar.no_se_puede_cofirmar_con_varios")
                    : Localizer.Text("winui.firmar.no_se_puede_firmar_el_motor_local_no");
                return null;
            }

            var inputPath = _inputPath!;
            var certificate = SelectedCertificate!;
            var action = SelectedAction!;
            var format = SelectedFormat!;
            var profile = SelectedProfile!;
            string? tsaUrl = null;
            if (_session.Supports(DesktopOperationActions.GetSettings))
            {
                var settings = await operations.GetSettingsAsync(operationCancellation.Token);
                if (!settings.IsSuccess || settings.Data is null)
                {
                    ValidationMessage = SealText("winui.parity.tsa.unavailable");
                    return null;
                }
                if (settings.Data.TsaEnabled == true)
                {
                    if (!TsaConfiguration.TryNormalize(true, settings.Data.TsaUrl, out var normalized))
                    {
                        ValidationMessage = SealText("winui.parity.tsa.invalid");
                        return null;
                    }
                    tsaUrl = normalized;
                }
            }
            var saveProfile = ResolveSaveProfile(format, inputPath);
            ValidationMessage =
                Localizer.Text("winui.firmar.elija_donde_guardar_el_resultado_la");
            var outputPath = await _filePicker.PickSaveFileAsync(
                saveProfile,
                SuggestedOutputName(inputPath),
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(outputPath))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.no_se_eligio_un_destino_no_se_ha");
                return null;
            }

            if (visibleSeal is not null)
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.comprobando_que_el_pdf_no_ha_cambiado");
                signInputGuard = OpenPdfReadGuard(inputPath);
                if (signInputGuard is null ||
                    _previewFileDigest is null)
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.no_se_pudo_inmovilizar_el_pdf_para");
                    return VisibleSealValidationDiagnostic(
                        "VISIBLE_SEAL_INPUT_GUARD_FAILED",
                        ValidationMessage,
                        Localizer.Text("winui.firmar.cierre_el_programa_que_esta_modificando_3"),
                        DiagnosticStepStatus.Failure);
                }
                expectedSignInputDigest =
                    _previewFileDigest.ToArray();
                signInputDigest = await ComputeSha256Async(
                    signInputGuard,
                    operationCancellation.Token);
                if (!CryptographicOperations.FixedTimeEquals(
                    signInputDigest,
                    expectedSignInputDigest))
                {
                    ValidationMessage =
                        Localizer.Text("winui.firmar.el_contenido_del_pdf_cambio_despues_de");
                    return VisibleSealValidationDiagnostic(
                        "PDF_CHANGED_AFTER_PREVIEW",
                        ValidationMessage,
                        Localizer.Text("winui.firmar.cargue_de_nuevo_la_previsualizacion_del"),
                        DiagnosticStepStatus.Failure);
                }
                ClearBytes(signInputDigest);
                signInputDigest = null;
            }

            var remote = await PrepareRemoteSigningAsync(
                certificate,
                batch: false,
                operationCancellation.Token);
            if (!remote.Proceed)
            {
                return null;
            }
            remoteSecrets = remote.Secrets;
            ValidationMessage = useGuidedMultiCosign
                ? Localizer.Text("winui.firmar.aplicando_el_firmante_principal_y_las")
                : Localizer.Text("winui.firmar.firmando_el_documento");
            var signParameters = new SignParameters
            {
                RemotePin = remoteSecrets?.Pin,
                RemoteOtp = remoteSecrets?.Otp,
                InputPath = inputPath,
                OutputPath = outputPath,
                CertificateId = certificate.Id,
                CertificateIndex = 0,
                AdditionalCertificateIds = useGuidedMultiCosign
                    ? _selectedAdditionalCertificateIds.ToArray()
                    : null,
                Format = visibleSeal is null
                    ? format.Value
                    : "pades",
                Action = action.Value,
                Overwrite = "force",
                SaveToDisk = true,
                ReturnSignatureBase64 = false,
                VisibleSeal = visibleSeal,
                QrContent = sealQrContent,
                Reason = signatureReason,
                Location = signatureLocation,
                ContactInfo = signatureContact,
                ExtraOptions = MergeExtraOptions(
                    tsaUrl is null ? sealExtraOptions : MergeExtraOptions(sealExtraOptions, "tsaURL", tsaUrl),
                    "profile",
                    profile.Value),
            };
            var result = useGuidedMultiCosign
                ? await operations.SignMultiCosignAsync(
                    signParameters,
                    operationCancellation.Token)
                : await operations.SignAsync(
                    signParameters,
                    operationCancellation.Token);
            if (!result.IsSuccess ||
                !string.Equals(
                    result.Outcome,
                    "success",
                    StringComparison.Ordinal))
            {
                ValidationMessage =
                    RemoteSigningFailureMessage(result.ErrorCode) ??
                    Localizer.Text("winui.firmar.la_firma_no_se_completo_abra_el");
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null ||
                string.IsNullOrWhiteSpace(result.Data.OutputPath))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_motor_confirmo_la_operacion_pero_no");
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_SIGNATURE_OUTPUT");
            }
            if (!PathsEqual(result.Data.OutputPath, outputPath) ||
                !HasNonEmptyOutput(outputPath))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_motor_no_confirmo_el_fichero_de");
                return InvalidResultDiagnostic(
                    result,
                    "SIGNATURE_OUTPUT_NOT_FOUND");
            }

            _outputPath = outputPath;
            if (signInputGuard is not null &&
                expectedSignInputDigest is not null)
            {
                signInputGuard.Position = 0;
                signInputDigest = await ComputeSha256Async(
                    signInputGuard,
                    operationCancellation.Token);
                if (!CryptographicOperations.FixedTimeEquals(
                    signInputDigest,
                    expectedSignInputDigest))
                {
                    ResultMessage =
                        Localizer.Format("winui.firmar.la_firma_se_guardo_como_pero_el_pdf_de", SafeFileName(outputPath));
                    ValidationMessage =
                        Localizer.Text("winui.firmar.no_use_el_resultado_hasta_revisar_el");
                    UpdateCommandStates();
                    return VisibleSealValidationDiagnostic(
                        "PDF_CHANGED_DURING_SIGN",
                        ValidationMessage,
                        Localizer.Text("winui.firmar.cierre_el_programa_que_modifica_el_pdf_y"),
                        DiagnosticStepStatus.Failure);
                }
                ClearBytes(signInputDigest);
                signInputDigest = null;
            }
            if (ValidateAfterSigning)
            {
                postValidationStarted = true;
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_firma_se_guardo_comprobando_ahora_su");
                var verification = await operations.VerifyAsync(
                    PostSignVerification.Create(
                        outputPath,
                        inputPath,
                        result.Data.Format,
                        signParameters.Action),
                    operationCancellation.Token);
                if (!verification.IsSuccess ||
                    !string.Equals(
                        verification.Outcome,
                        "success",
                        StringComparison.Ordinal))
                {
                    ResultMessage =
                        Localizer.Format("winui.firmar.la_firma_se_guardo_como_pero_no_pudo", SafeFileName(outputPath));
                    ValidationMessage =
                        Localizer.Text("winui.firmar.el_fichero_firmado_existe_pero_su");
                    UpdateCommandStates();
                    return OperationDiagnosticMapper.FromResult(verification);
                }
                if (!IsCoherentVerification(verification.Data))
                {
                    ResultMessage =
                        Localizer.Format("winui.firmar.la_firma_se_guardo_como_pero_el", SafeFileName(outputPath));
                    ValidationMessage =
                        Localizer.Text("winui.firmar.el_motor_no_devolvio_evidencias");
                    UpdateCommandStates();
                    return InvalidResultDiagnostic(
                        verification,
                        "INVALID_POST_SIGN_VERIFICATION_RESULT");
                }
                if (!VerificationAssessment.HasValidSignatureEvidence(
                    verification.Data!))
                {
                    ResultMessage =
                        Localizer.Format("winui.firmar.la_firma_se_guardo_como_pero_no_supero", SafeFileName(outputPath));
                    ValidationMessage =
                        Localizer.Text("winui.firmar.la_firma_generada_no_es_valida_no_la");
                    UpdateCommandStates();
                    return InvalidResultDiagnostic(
                        verification,
                        "POST_SIGN_VERIFICATION_FAILED");
                }

                ResultMessage = verification.Data!.IsValid
                    ? useGuidedMultiCosign
                        ? Localizer.Format("winui.firmar.cofirma_multiple_completada_guardada_y", SafeFileName(outputPath))
                        : Localizer.Format("winui.firmar.firma_completada_guardada_y_validada", SafeFileName(outputPath))
                    : useGuidedMultiCosign
                        ? Localizer.Format("winui.firmar.cofirma_multiple_completada_y_guardada", SafeFileName(outputPath))
                        : Localizer.Format("winui.firmar.firma_completada_y_guardada_como_la", SafeFileName(outputPath));
                ValidationMessage = PostValidationSuccessMessage(
                    verification.Data);
            }
            else
            {
                ResultMessage = useGuidedMultiCosign
                    ? Localizer.Format("winui.firmar.cofirma_multiple_completada_y_guardada_2", SafeFileName(outputPath))
                    : Localizer.Format("winui.firmar.firma_completada_y_guardada_como", SafeFileName(outputPath));
                ValidationMessage = useGuidedMultiCosign
                    ? Localizer.Text("winui.firmar.la_cofirma_multiple_termino")
                    : Localizer.Text("winui.firmar.la_firma_termino_correctamente_pero_no");
            }
            operationCancellation.Token.ThrowIfCancellationRequested();
            CompletedSignPresentationId = Guid.NewGuid();
            UpdateCommandStates();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException exception)
        {
            if (postValidationStarted &&
                !string.IsNullOrWhiteSpace(_outputPath))
            {
                ResultMessage =
                    Localizer.Format("winui.firmar.la_firma_se_guardo_como_pero_su", SafeFileName(_outputPath));
                ValidationMessage =
                    Localizer.Text("winui.firmar.el_fichero_firmado_existe_pero_no");
                UpdateCommandStates();
            }
            else
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_firma_se_cancelo_antes_de_completarse");
            }
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            if (postValidationStarted &&
                !string.IsNullOrWhiteSpace(_outputPath))
            {
                ResultMessage =
                    Localizer.Format("winui.firmar.la_firma_se_guardo_como_pero_no_pudo_2", SafeFileName(_outputPath));
                ValidationMessage =
                    Localizer.Text("winui.firmar.fallo_la_comunicacion_segura_durante_la_2");
                UpdateCommandStates();
            }
            else
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.fallo_la_comunicacion_segura_durante_la_3");
            }
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            if (postValidationStarted &&
                !string.IsNullOrWhiteSpace(_outputPath))
            {
                ResultMessage =
                    Localizer.Format("winui.firmar.la_firma_se_guardo_como_pero_no_pudo_2", SafeFileName(_outputPath));
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_aplicacion_no_pudo_completar_la");
                UpdateCommandStates();
            }
            else
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.la_aplicacion_no_pudo_completar_la_firma_2");
            }
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            ClearBytes(signInputDigest);
            ClearBytes(expectedSignInputDigest);
            signInputGuard?.Dispose();
            remoteSecrets?.Dispose();
            EndOperation(operationCancellation);
        }
    }

    public OperationDiagnostic? ValidateOutputForOpening()
    {
        if (string.IsNullOrWhiteSpace(_outputPath))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.todavia_no_hay_un_resultado_de_firma_que");
            UpdateCommandStates();
            return null;
        }
        if (!File.Exists(_outputPath))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.el_fichero_firmado_ya_no_esta_disponible");
            ClearOutput();
            return OperationDiagnosticMapper.FromResult(
                new IpcCallResult<object>
                {
                    Protocol = DesktopIpcProtocol.Name,
                    RequestId = "local-output-check",
                    TraceId = "local-output-check",
                    Action = DesktopOperationActions.Sign,
                    IsSuccess = false,
                    Outcome = "failure",
                    ErrorCode = "SIGNATURE_OUTPUT_NOT_FOUND",
                    Phase = "operation",
                    Retryable = false,
                    Data = null,
                    Diagnostic = null,
                });
        }

        return null;
    }

    public OperationDiagnostic? ValidateBatchOutputForOpening()
    {
        if (string.IsNullOrWhiteSpace(_batchOutputDirectory) ||
            !Directory.Exists(_batchOutputDirectory))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.la_carpeta_de_salida_del_lote_ya_no_esta");
            UpdateCommandStates();
            return OperationDiagnosticMapper.FromResult(
                new IpcCallResult<object>
                {
                    Protocol = DesktopIpcProtocol.Name,
                    RequestId = "local-batch-output-check",
                    TraceId = "local-batch-output-check",
                    Action = DesktopOperationActions.SignBatch,
                    IsSuccess = false,
                    Outcome = "failure",
                    ErrorCode = "BATCH_OUTPUT_DIRECTORY_NOT_FOUND",
                    Phase = "operation",
                    Retryable = false,
                    Data = null,
                    Diagnostic = null,
                });
        }
        return null;
    }

    public OperationDiagnostic PreviewImageRenderingFailed()
    {
        ClearVisibleSealPreview();
        ValidationMessage =
            Localizer.Text("winui.firmar.windows_no_pudo_representar_de_forma");
        return VisibleSealValidationDiagnostic(
            "PDF_PREVIEW_IMAGE_INVALID",
            ValidationMessage,
            Localizer.Text("winui.firmar.vuelva_a_cargar_la_previsualizacion_si"),
            DiagnosticStepStatus.Failure);
    }

    public void ConfirmPreviewImageRendered()
    {
        if (VisibleSealPreviewImage.IsEmpty)
        {
            return;
        }
        _isPreviewImageRendered = true;
        UpdateValidationMessage();
        UpdateCommandStates();
        // La página ya se ve: dibujar el área y firmar en el portal dependen de ello.
        RaisePropertyChanged(nameof(CanDrawVisibleSealArea));
    }

    // La página del PDF se carga sola al activar el sello o cambiar de
    // documento o páginas: sin ella no se puede arrastrar ni redimensionar el
    // sello sobre el folio.
    public bool NeedsAutomaticVisibleSealPreview =>
        VisibleSealEnabled &&
        CanRefreshVisibleSealPreview &&
        Volatile.Read(ref _operationInProgress) == 0 &&
        !string.IsNullOrWhiteSpace(_inputPath) &&
        VisibleSealPreviewImage.IsEmpty;

    public void DiscardVisibleSealPreview()
    {
        CancelSealStampPreview();
        ClearVisibleSealPreview();
    }

    public async Task<OperationDiagnostic?> PickVisibleSealImageAsync(
        CancellationToken cancellationToken = default)
    {
        try
        {
            var selectedPath = await _filePicker.PickOpenFileAsync(
                OpenFilePickerProfile.SealImage,
                cancellationToken);
            if (string.IsNullOrWhiteSpace(selectedPath))
            {
                return null;
            }
            var file = new FileInfo(selectedPath);
            if (!file.Exists ||
                file.Length is <= 0 or > MaximumSealImageBytes)
            {
                VisibleSealStampMessage =
                    Localizer.Text("winui.firmar.la_imagen_del_sello_debe_ser_un_png_o");
                return null;
            }
            _visibleSealImagePath = selectedPath;
            RaisePropertyChanged(nameof(VisibleSealImageSummary));
            ClearOutput();
            ScheduleSealStampPreview();
            UpdateValidationMessage();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (Exception exception)
        {
            return OperationDiagnosticMapper.FromException(exception);
        }
    }

    // Aspecto del sello común a la firma y a la vista previa: estilo, imagen
    // propia y QR de verificación.
    private bool TryBuildSealAppearance(
        out string? imagePath,
        out string? qrContent,
        out IReadOnlyDictionary<string, string>? extraOptions,
        out string error)
    {
        imagePath = null;
        qrContent = null;
        extraOptions = null;
        error = string.Empty;
        switch (SelectedVisibleSealStyle?.Value)
        {
            case "institucional":
                extraOptions = new Dictionary<string, string>(
                    StringComparer.Ordinal)
                {
                    ["visibleSealLogo"] = "institucional",
                };
                break;
            case "imagen":
                if (string.IsNullOrWhiteSpace(_visibleSealImagePath))
                {
                    error =
                        Localizer.Text("winui.firmar.ha_elegido_imagen_propia_seleccione_la");
                    return false;
                }
                imagePath = _visibleSealImagePath;
                break;
        }
        extraOptions = MergeExtraOptions(
            extraOptions,
            "visibleSealLogoOpacityPercent",
            ((int)VisibleSealLogoOpacityPercent).ToString(
                System.Globalization.CultureInfo.InvariantCulture));
        // El sello se escribe en el idioma fijado en la configuración
        // (signSealLanguage) o, si sigue a la interfaz, en el de la interfaz.
        // El motor impone igualmente el idioma fijo.
        var sealLanguage = DesktopSettingsDocument.EffectiveSealLanguage(
            _sealLanguagePreference,
            Localizer.Language);
        if (sealLanguage is not null)
        {
            extraOptions = MergeExtraOptions(
                extraOptions,
                "sealLanguage",
                sealLanguage);
        }
        if (VisibleSealQrEnabled)
        {
            if (!TryNormalizeVerificationUrl(
                VisibleSealQrUrl,
                out qrContent,
                out error))
            {
                return false;
            }
        }
        return true;
    }

    // Como el QR: sin esquema se entiende https://; cualquier otro esquema, un
    // usuario o una contraseña en la dirección se rechazan.
    private static bool TryNormalizeCsvUrl(string raw, string code, out string url)
    {
        url = string.Empty;
        var probe = raw.Replace("{csv}", Uri.EscapeDataString(code), StringComparison.Ordinal);
        return VerificationUrlNormalizer.TryNormalize(probe, out _) &&
            VerificationUrlNormalizer.TryNormalize(raw, out url);
    }

    private bool TryNormalizeVerificationUrl(
        string? raw,
        out string? url,
        out string error)
    {
        url = null;
        error = SealText("sign.seal.qr_https_error");
        if (!VerificationUrlNormalizer.TryNormalize(raw, out var normalized) ||
            normalized.Length > MaximumQrUrlLength)
            return false;
        url = normalized;
        error = string.Empty;
        return true;
    }

    private (string Field, string Key)? CsvLegendError(string? field = null)
    {
        var code = VisibleSealCsvCode.Trim();
        var url = VisibleSealCsvUrl.Trim();
        var text = VisibleSealCsvText.Trim();
        if (field is null or "csvCode")
        {
            if (code.Length == 0) return ("csvCode", "csv.error.code_missing");
            if (code.Length > 128 || code.Any(char.IsControl) || VerificationUrlNormalizer.ContainsFormatCharacter(code)) return ("csvCode", "csv.error.code_invalid");
        }
        if (field is null or "csvUrl")
        {
            if (url.Length == 0) return ("csvUrl", "csv.error.url_missing");
            if (!TryNormalizeCsvUrl(url, code, out _)) return ("csvUrl", "csv.error.url_invalid");
        }
        if ((field is null || field == "csvText") &&
            (text.Length > 512 || text.Any(char.IsControl) || VerificationUrlNormalizer.ContainsFormatCharacter(text)))
            return ("csvText", "csv.error.text_invalid");
        return null;
    }

    public IReadOnlyDictionary<string, string> ValidateVisibleSealFields()
    {
        var errors = new Dictionary<string, string>(StringComparer.Ordinal);
        if (!VisibleSealEnabled) return errors;
        if (SelectedVisibleSealStyle?.Value == "imagen" && string.IsNullOrWhiteSpace(_visibleSealImagePath))
            errors["image"] = "validacion.sello.imagen";
        if (VisibleSealQrEnabled && !TryNormalizeVerificationUrl(VisibleSealQrUrl, out _, out _))
            errors["qr"] = "sign.seal.qr_https_error";
        if (VisibleSealCsvEnabled)
        {
            foreach (var field in new[] { "csvCode", "csvUrl", "csvText" })
            {
                var csvError = CsvLegendError(field);
                if (csvError is { } issue) errors[issue.Field] = issue.Key;
            }
        }
        if (IsVisibleSealCustomPages &&
            !TryParsePageSelection(VisibleSealPages, _previewTotalPages, out _, out _, out _))
            errors["pages"] = "validacion.sello.paginas";
        return errors;
    }

    private static IReadOnlyDictionary<string, string> MergeExtraOptions(
        IReadOnlyDictionary<string, string>? baseOptions,
        string key,
        string value)
    {
        var merged = baseOptions is null
            ? new Dictionary<string, string>(StringComparer.Ordinal)
            : new Dictionary<string, string>(
                baseOptions,
                StringComparer.Ordinal);
        merged[key] = value;
        return merged;
    }

    private static int NormalizeRotation(double degrees)
    {
        if (double.IsNaN(degrees) || double.IsInfinity(degrees))
        {
            return 0;
        }
        var rounded = (int)Math.Round(degrees) % 360;
        return rounded < 0 ? rounded + 360 : rounded;
    }

    private void CancelSealStampPreview()
    {
        var previous = Interlocked.Exchange(
            ref _stampPreviewCancellation,
            null);
        if (previous is not null)
        {
            previous.Cancel();
            previous.Dispose();
        }
    }

    // Pide al motor el sello real con un pequeño retardo para no generar una
    // imagen por cada pulsación mientras el usuario arrastra o escribe. El
    // token solo cancela la espera; la respuesta obsoleta se descarta.
    private void ScheduleSealStampPreview()
    {
        CancelSealStampPreview();
        if (!VisibleSealEnabled ||
            !_session.TryGetOperations(
                DesktopOperationActions.SealPreview,
                out var operations))
        {
            VisibleSealStampImage = ReadOnlyMemory<byte>.Empty;
            VisibleSealStampMessage = OrientativeSealMessage;
            return;
        }
        var cancellation = new CancellationTokenSource();
        _stampPreviewCancellation = cancellation;
        _ = RefreshSealStampAsync(operations, cancellation.Token);
    }

    private async Task RefreshSealStampAsync(
        DesktopOperationsClient operations,
        CancellationToken cancellationToken)
    {
        try
        {
            await Task.Delay(250, cancellationToken);
            if (!TryBuildSealAppearance(
                out var imagePath,
                out var qrContent,
                out var extraOptions,
                out var appearanceError))
            {
                VisibleSealStampImage = ReadOnlyMemory<byte>.Empty;
                VisibleSealStampMessage = appearanceError;
                return;
            }
            TryNormalizeMetadata(SignatureReason, "winui.firmar.motivo", out var reason, out _);
            TryNormalizeMetadata(SignatureLocation, "winui.firmar.ubicacion", out var location, out _);
            TryNormalizeMetadata(SignatureContact, "winui.firmar.contacto", out var contact, out _);
            var result = await operations.GetSealPreviewAsync(
                new SealPreviewParameters
                {
                    CertificateId = SelectedCertificate?.Id,
                    VisibleSeal = new VisibleSealParameters
                    {
                        Page = "1",
                        X = VisibleSealXPercent / 100,
                        Y = VisibleSealYPercent / 100,
                        Width = VisibleSealWidthPercent / 100,
                        Height = VisibleSealHeightPercent / 100,
                        PageWidth = PreviewCanvasWidth,
                        PageHeight = PreviewCanvasHeight,
                        Rotation = 0, // El editor gira la tarjeta completa.
                        KeepText = VisibleSealKeepText,
                        ImagePath = imagePath,
                        LogoOpacityPercent = (int)VisibleSealLogoOpacityPercent,
                    },
                    QrContent = qrContent,
                    Reason = reason,
                    Location = location,
                    ContactInfo = contact,
                    ExtraOptions = extraOptions,
                },
                // Cancelar una petición ya enviada invalida la conexión con el
                // motor: se deja terminar y se descarta si quedó obsoleta.
                CancellationToken.None);
            if (cancellationToken.IsCancellationRequested)
            {
                return;
            }
            if (!result.IsSuccess || result.Data is null ||
                result.Data.Image.Length is 0 or > MaximumSealPreviewBytes)
            {
                VisibleSealStampImage = ReadOnlyMemory<byte>.Empty;
                VisibleSealStampMessage =
                    Localizer.Text("winui.firmar.no_se_pudo_generar_la_vista_previa_del");
                return;
            }
            VisibleSealStampImage = result.Data.Image;
            VisibleSealStampMessage = SelectedCertificate is null
                ? Localizer.Text("winui.firmar.vista_previa_real_del_sello_elija_un")
                : Localizer.Text("winui.firmar.vista_previa_real_del_sello_con_el");
        }
        catch (OperationCanceledException)
        {
        }
        catch (Exception)
        {
            if (!cancellationToken.IsCancellationRequested)
            {
                VisibleSealStampImage = ReadOnlyMemory<byte>.Empty;
                VisibleSealStampMessage =
                    Localizer.Text("winui.firmar.el_motor_local_no_pudo_generar_la_vista");
            }
        }
    }

    private bool TryBeginOperation(
        CancellationToken cancellationToken,
        out CancellationTokenSource operationCancellation)
    {
        if (Interlocked.CompareExchange(
            ref _operationInProgress,
            1,
            0) != 0)
        {
            operationCancellation = null!;
            return false;
        }

        operationCancellation =
            CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        Volatile.Write(ref _activeCancellation, operationCancellation);
        CompletedSignPresentationId = Guid.Empty;
        SetBusy(true);
        return true;
    }

    private void EndOperation(CancellationTokenSource operationCancellation)
    {
        Interlocked.CompareExchange(
            ref _activeCancellation,
            null,
            operationCancellation);
        operationCancellation.Dispose();
        Interlocked.Exchange(ref _operationInProgress, 0);
        SetBusy(false);
    }

    private string? ValidateBeforeSign()
    {
        if (!IsOperationConnected)
        {
            return Localizer.Text("winui.firmar.no_se_puede_firmar_porque_el_motor_local");
        }
        if (string.IsNullOrWhiteSpace(_inputPath))
        {
            return Localizer.Text("winui.firmar.seleccione_el_documento_que_desea_firmar");
        }
        if (!File.Exists(_inputPath))
        {
            _inputPath = null;
            _ = UpdateVeriFactuFormatAsync(string.Empty);
            InputDisplayName = string.Empty;
            ClearOutput();
            UpdateCommandStates();
            return Localizer.Text("winui.firmar.el_documento_seleccionado_ya_no_esta");
        }
        if (SelectedCertificate is null)
        {
            return Localizer.Text("winui.firmar.seleccione_un_certificado_apto_para");
        }
        if (!SelectedCertificate.CanSign)
        {
            return Localizer.Format("winui.firmar.el_certificado_seleccionado_no_es_valido", SelectedCertificate.StatusReason);
        }
        if (SelectedAction is null)
        {
            return Localizer.Text("winui.firmar.seleccione_si_desea_firmar_cofirmar_o");
        }
        if (SelectedFormat is null)
        {
            return Localizer.Text("winui.firmar.seleccione_un_formato_de_firma");
        }
        if (SelectedProfile is null)
        {
            return Localizer.Text("winui.firmar.seleccione_un_perfil_de_firma");
        }
        if (GuidedMultiCosignEnabled)
        {
            if (!_session.Supports(
                DesktopOperationActions.SignMultiCosign))
            {
                return Localizer.Text("winui.firmar.el_motor_local_no_ofrece_la_cofirma");
            }
            if (string.Equals(
                SelectedAction.Value,
                "countersign",
                StringComparison.Ordinal))
            {
                return Localizer.Text("winui.firmar.la_cofirma_multiple_guiada_no_admite");
            }
            if (!IsGuidedMultiCosignFormatSupported())
            {
                return Localizer.Text("winui.firmar.la_cofirma_multiple_guiada_solo_esta");
            }
            if (_selectedAdditionalCertificateIds.Count == 0)
            {
                return Localizer.Text("winui.firmar.seleccione_al_menos_un_certificado");
            }
        }
        return null;
    }

    private string? ValidateBeforeBatch(
        out string? reason,
        out string? location,
        out string? contact)
    {
        reason = null;
        location = null;
        contact = null;
        if (!IsOperationConnected)
        {
            return Localizer.Text("winui.firmar.no_se_puede_firmar_el_lote_porque_el");
        }
        if (!BatchModeEnabled ||
            !_session.Supports(DesktopOperationActions.SignBatch))
        {
            return Localizer.Text("winui.firmar.active_la_firma_por_lotes_y_compruebe");
        }
        if (_batchInputPaths.Count == 0)
        {
            return Localizer.Text("winui.firmar.seleccione_varios_documentos_o_una");
        }
        if (_batchInputPaths.Count > MaximumBatchDocuments)
        {
            return Localizer.Format("winui.firmar.el_lote_admite_como_maximo_documentos", MaximumBatchDocuments);
        }
        if (string.IsNullOrWhiteSpace(_batchOutputDirectory) ||
            !Directory.Exists(_batchOutputDirectory))
        {
            return Localizer.Text("winui.firmar.seleccione_una_carpeta_de_salida_valida");
        }
        if (SelectedCertificate?.CanSign != true)
        {
            return Localizer.Text("winui.firmar.seleccione_un_certificado_apto_para_2");
        }
        if (SelectedAction is null)
        {
            return Localizer.Text("winui.firmar.seleccione_la_operacion_de_firma_del");
        }
        if (SelectedFormat is null)
        {
            return Localizer.Text("winui.firmar.seleccione_el_formato_de_firma_del_lote");
        }
        if (SelectedProfile is null)
        {
            return Localizer.Text("winui.firmar.seleccione_el_perfil_de_firma_del_lote");
        }
        if (VisibleSealEnabled)
        {
            return Localizer.Text("winui.firmar.el_sello_visible_requiere");
        }
        if (GuidedMultiCosignEnabled)
        {
            if (!_session.Supports(
                DesktopOperationActions.SignMultiCosign))
            {
                return Localizer.Text("winui.firmar.el_motor_local_no_ofrece_la_cofirma_2");
            }
            if (_selectedAdditionalCertificateIds.Count == 0)
            {
                return Localizer.Text("winui.firmar.seleccione_al_menos_un_certificado_2");
            }
            if (!IsGuidedMultiCosignFormatSupported())
            {
                return Localizer.Text("winui.firmar.la_cofirma_multiple_del_lote_solo_admite");
            }
        }
        if (!TryNormalizeMetadata(
            SignatureReason,
            "winui.firmar.motivo",
            out reason,
            out var metadataError) ||
            !TryNormalizeMetadata(
                SignatureLocation,
                "winui.firmar.ubicacion",
                out location,
                out metadataError) ||
            !TryNormalizeMetadata(
                SignatureContact,
                "winui.firmar.contacto",
                out contact,
                out metadataError))
        {
            return metadataError;
        }
        return null;
    }

    private string? ValidateVisibleSealPreviewRequest(
        out string? inputPath,
        out int previewPage)
    {
        inputPath = _inputPath;
        previewPage = 1;
        if (!VisibleSealEnabled)
        {
            return Localizer.Text("winui.firmar.active_primero_el_sello_visible");
        }
        if (!IsVisibleSealSupportedByCurrentSelection())
        {
            return Localizer.Text("winui.firmar.el_sello_visible_requiere_un_pdf_formato");
        }
        if (string.IsNullOrWhiteSpace(inputPath) ||
            !File.Exists(inputPath))
        {
            return Localizer.Text("winui.firmar.el_pdf_seleccionado_ya_no_esta");
        }
        if (!TryParsePageSelection(
            VisibleSealPages,
            0,
            out _,
            out previewPage,
            out var pageError))
        {
            return pageError;
        }
        if ((_perPageSealEnabled || _portalSealMode) && _requestedPreviewPage > 0) previewPage = _requestedPreviewPage;
        return null;
    }

    private bool TryBuildVisibleSeal(
        out VisibleSealParameters? visibleSeal,
        out string? reason,
        out string? location,
        out string? contact,
        out string? sealQrContent,
        out IReadOnlyDictionary<string, string>? sealExtraOptions,
        out string errorCode,
        out string error)
    {
        visibleSeal = null;
        sealQrContent = null;
        sealExtraOptions = null;
        reason = null;
        location = null;
        contact = null;
        errorCode = string.Empty;
        error = string.Empty;
        if (!VisibleSealEnabled)
        {
            return true;
        }
        if (!IsVisibleSealSupportedByCurrentSelection())
        {
            errorCode = "VISIBLE_SEAL_NOT_SUPPORTED";
            error =
                Localizer.Text("winui.firmar.el_sello_visible_solo_puede_aplicarse_a");
            return false;
        }
        if (!TryParsePageSelection(
            VisibleSealPages,
            _previewTotalPages,
            out var normalizedPages,
            out var previewPage,
            out error))
        {
            errorCode = "VISIBLE_SEAL_PAGE_SELECTION_INVALID";
            return false;
        }
        if (VisibleSealPreviewImage.IsEmpty ||
            !_isPreviewImageRendered ||
            _previewFileDigest is null ||
            !IsPositiveFinite(_previewPageWidth) ||
            !IsPositiveFinite(_previewPageHeight) ||
            _previewCurrentPage != (_perPageSealEnabled ? _requestedPreviewPage : previewPage) ||
            string.IsNullOrWhiteSpace(_previewInputPath) ||
            string.IsNullOrWhiteSpace(_inputPath) ||
            !PathsEqual(_previewInputPath, _inputPath) ||
            !TryReadFileStamp(
                _inputPath,
                out var currentFileLength,
                out var currentFileWrite) ||
            currentFileLength != _previewFileLength ||
            currentFileWrite != _previewFileLastWriteUtcTicks)
        {
            errorCode = "VISIBLE_SEAL_PREVIEW_REQUIRED";
            error =
                Localizer.Text("winui.firmar.la_previsualizacion_no_corresponde_al");
            return false;
        }
        if (!TryValidateVisibleSealGeometry(out error))
        {
            errorCode = "VISIBLE_SEAL_GEOMETRY_INVALID";
            return false;
        }
        if (_visibleSealRotationDegrees is < 0 or > 359)
        {
            errorCode = "VISIBLE_SEAL_ROTATION_INVALID";
            error = Localizer.Text("winui.firmar.indique_un_giro_entre_0_y_359_grados");
            return false;
        }
        if (!TryBuildSealAppearance(
            out var imagePath,
            out var qrContent,
            out var extraOptions,
            out error))
        {
            errorCode = "VISIBLE_SEAL_APPEARANCE_INVALID";
            return false;
        }
        if (!TryNormalizeMetadata(
            SignatureReason,
            "winui.firmar.motivo",
            out reason,
            out error) ||
            !TryNormalizeMetadata(
                SignatureLocation,
                "winui.firmar.ubicacion",
                out location,
                out error) ||
            !TryNormalizeMetadata(
                SignatureContact,
                "winui.firmar.contacto",
                out contact,
                out error))
        {
            errorCode = "VISIBLE_SEAL_METADATA_INVALID";
            return false;
        }

        visibleSeal = new VisibleSealParameters
        {
            Page = normalizedPages,
            X = VisibleSealXPercent / 100,
            Y = VisibleSealYPercent / 100,
            Width = VisibleSealWidthPercent / 100,
            Height = VisibleSealHeightPercent / 100,
            PageWidth = _previewPageWidth,
            PageHeight = _previewPageHeight,
            Rotation = _visibleSealRotationDegrees,
            KeepText = VisibleSealKeepText,
            ImagePath = imagePath,
            LogoOpacityPercent = (int)VisibleSealLogoOpacityPercent,
            Placements = _perPageSealEnabled
                ? _sealPlacements.Values.OrderBy(item => item.Page).ToArray()
                : null,
        };
        if (_perPageSealEnabled && visibleSeal.Placements!.Count == 0)
        {
            errorCode = "VISIBLE_SEAL_PLACEMENTS_EMPTY";
            error = SealText("sign.seal.no_pages_error");
            return false;
        }
        if (VisibleSealCsvEnabled)
        {
            var code = VisibleSealCsvCode.Trim();
            var url = VisibleSealCsvUrl.Trim();
            var text = VisibleSealCsvText.Trim();
            // Cada problema tiene su propio mensaje: uno genérico hacía creer
            // que fallaba la URL cuando faltaba, por ejemplo, el código.
            var csvError = CsvLegendError();
            if (csvError is not null)
            {
                errorCode = "VISIBLE_SEAL_CSV_INVALID";
                error = SealText(csvError.Value.Key);
                return false;
            }
            TryNormalizeCsvUrl(url, code, out url);
            extraOptions = MergeExtraOptions(extraOptions, "csv", code);
            extraOptions = MergeExtraOptions(extraOptions, "csvUrl", url);
            extraOptions = MergeExtraOptions(extraOptions, "csvText", text);
            extraOptions = MergeExtraOptions(extraOptions, "csvQR", VisibleSealCsvQr ? "true" : "false");
        }
        sealQrContent = qrContent;
        sealExtraOptions = extraOptions;
        return true;
    }

    private void UpdateValidationMessage()
    {
        if (!IsOperationConnected)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.conecte_el_motor_local_para_habilitar_la");
            return;
        }
        if (BatchModeEnabled)
        {
            if (_batchInputPaths.Count == 0)
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.anada_varios_documentos_o_una_carpeta_al");
                return;
            }
            if (string.IsNullOrWhiteSpace(_batchOutputDirectory))
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.seleccione_la_carpeta_donde_se_guardaran");
                return;
            }
            if (SelectedCertificate is null)
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.seleccione_el_certificado_que_se");
                return;
            }
            if (!SelectedCertificate.CanSign)
            {
                ValidationMessage = Localizer.Format("winui.firmar.no_valido_para_firmar", SelectedCertificate.StatusReason);
                return;
            }
            if (GuidedMultiCosignEnabled &&
                _selectedAdditionalCertificateIds.Count == 0)
            {
                ValidationMessage =
                    Localizer.Text("winui.firmar.seleccione_al_menos_un_certificado_2");
                return;
            }
            ValidationMessage =
                Localizer.Format("winui.firmar.documento_s_certificado_y_salida", _batchInputPaths.Count);
            return;
        }
        if (string.IsNullOrWhiteSpace(_inputPath))
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.seleccione_el_documento_que_desea_firmar");
            return;
        }
        if (SelectedCertificate is null)
        {
            ValidationMessage = Certificates.Count == 0
                ? Localizer.Text("winui.firmar.no_hay_certificados_aptos_para_firma_en")
                : Localizer.Text("winui.firmar.seleccione_un_certificado_apto_para");
            return;
        }
        if (!SelectedCertificate.CanSign)
        {
            ValidationMessage = Localizer.Format("winui.firmar.no_valido_para_firmar", SelectedCertificate.StatusReason);
            return;
        }
        if (GuidedMultiCosignEnabled &&
            _selectedAdditionalCertificateIds.Count == 0)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.seleccione_al_menos_un_certificado_3");
            return;
        }
        if (!VisibleSealEnabled)
        {
            ValidationMessage = GuidedMultiCosignEnabled
                ? Localizer.Text("winui.firmar.documento_y_firmantes_preparados_para_la")
                : Localizer.Text("winui.firmar.documento_y_certificado_preparados_para");
            return;
        }
        if (!IsVisibleSealSupportedByCurrentSelection())
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.el_sello_visible_requiere_un_pdf_formato_2");
            return;
        }
        if (VisibleSealPreviewImage.IsEmpty)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.cargue_la_previsualizacion_real_del_pdf");
            return;
        }
        if (!_isPreviewImageRendered)
        {
            ValidationMessage =
                Localizer.Text("winui.firmar.windows_esta_comprobando_la_imagen_de");
            return;
        }
        ValidationMessage = TryValidateVisibleSealGeometry(out var error)
            ? GuidedMultiCosignEnabled
                ? Localizer.Text("winui.firmar.documento_firmantes_y_sello_visible")
                : Localizer.Text("winui.firmar.documento_certificado_y_sello_visible")
            : error;
    }

    private void ClearOutput()
    {
        CompletedSignPresentationId = Guid.Empty;
        _outputPath = null;
        ResultMessage = Localizer.Text("winui.firmar.no_se_ha_ejecutado_ninguna_firma_con");
        UpdateCommandStates();
    }

    private void SetBusy(bool value)
    {
        IsBusy = value;
        UpdateCommandStates();
    }

    private void UpdateCommandStates()
    {
        CanRefreshCertificates =
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(DesktopOperationActions.Certificates);
        CanUseTemporaryCredential =
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.UseTemporaryCertificate);
        CanImportCredentialToWindows =
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(
                DesktopOperationActions.CertificateAccessOptions) &&
            _session.Supports(
                DesktopOperationActions.ImportCertificateToStore);
        CanConfigureBatch =
            IsOperationConnected &&
            !IsBusy &&
            _session.Supports(DesktopOperationActions.SignBatch);
        CanSelectBatchSources =
            CanConfigureBatch &&
            BatchModeEnabled;
        CanSelectBatchOutput =
            CanConfigureBatch &&
            BatchModeEnabled &&
            _batchInputPaths.Count > 0;
        CanRunBatch =
            CanConfigureBatch &&
            BatchModeEnabled &&
            _batchInputPaths.Count > 0 &&
            !string.IsNullOrWhiteSpace(_batchOutputDirectory) &&
            SelectedCertificate?.CanSign == true &&
            SelectedAction is not null &&
            SelectedFormat is not null &&
            SelectedProfile is not null &&
            (!GuidedMultiCosignEnabled ||
                _selectedAdditionalCertificateIds.Count > 0);
        CanConfigureGuidedMultiCosign =
            IsOperationConnected &&
            !IsBusy &&
            IsGuidedMultiCosignEligible();
        CanSelectAdditionalSigners =
            CanConfigureGuidedMultiCosign &&
            GuidedMultiCosignEnabled &&
            SelectedCertificate is not null;
        CanConfigureVisibleSeal =
            IsOperationConnected &&
            !IsBusy &&
            !BatchModeEnabled &&
            IsVisibleSealSupportedByCurrentSelection();
        CanRefreshVisibleSealPreview =
            CanConfigureVisibleSeal &&
            VisibleSealEnabled;
        CanSelectDocument =
            IsOperationConnected &&
            !IsBusy &&
            !BatchModeEnabled;
        CanSign =
            IsOperationConnected &&
            !IsBusy &&
            !BatchModeEnabled &&
            !string.IsNullOrWhiteSpace(_inputPath) &&
            SelectedCertificate?.CanSign == true &&
            SelectedAction is not null &&
            SelectedFormat is not null &&
            SelectedProfile is not null &&
            (!GuidedMultiCosignEnabled ||
                _selectedAdditionalCertificateIds.Count > 0);
        CanCancel = IsBusy;
        CanOpenOutput =
            !IsBusy &&
            !string.IsNullOrWhiteSpace(_outputPath);
        CanOpenBatchOutput =
            !IsBusy &&
            HasBatchResults &&
            BatchItems.Any(item => item.IsSuccessful) &&
            !string.IsNullOrWhiteSpace(_batchOutputDirectory);
        CanValidateAfterSigning =
            IsOperationConnected &&
            !IsBusy &&
            !BatchModeEnabled &&
            _session.Supports(DesktopOperationActions.Verify);
    }

    private void ReplaceCertificateCatalog(
        IReadOnlyList<CertificateInfo> catalog,
        string? preferredCertificateId)
    {
        ArgumentNullException.ThrowIfNull(catalog);

        Certificates = catalog
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
        SelectedCertificate =
            string.IsNullOrWhiteSpace(preferredCertificateId)
                ? Certificates.FirstOrDefault(item => item.CanSign)
                : Certificates.FirstOrDefault(item =>
                    string.Equals(
                        item.Id,
                        preferredCertificateId,
                        StringComparison.Ordinal) && item.CanSign) ??
                    Certificates.FirstOrDefault(item => item.CanSign);
        RefreshGuidedMultiCosignState();
    }

    private void RefreshGuidedMultiCosignState()
    {
        var primaryId = SelectedCertificate?.Id;
        var candidates = Certificates
            .Where(certificate =>
                certificate.CanSign && !string.Equals(
                    certificate.Id,
                    primaryId,
                    StringComparison.Ordinal))
            .Take(MaximumCatalogItems)
            .ToArray();
        if (!AdditionalSignerCandidates.SequenceEqual(candidates))
        {
            AdditionalSignerCandidates = candidates;
        }

        var selectedIds = new HashSet<string>(
            _selectedAdditionalCertificateIds,
            StringComparer.Ordinal);
        _selectedAdditionalCertificateIds =
            candidates
                .Where(certificate =>
                    selectedIds.Contains(certificate.Id))
                .Select(certificate => certificate.Id)
                .ToArray();

        if (GuidedMultiCosignEnabled &&
            !IsGuidedMultiCosignEligible() &&
            SetProperty(
                ref _guidedMultiCosignEnabled,
                false,
                nameof(GuidedMultiCosignEnabled)))
        {
            ClearOutput();
        }
        UpdateGuidedMultiCosignSummary();
    }

    private void UpdateGuidedMultiCosignSummary()
    {
        if (!_session.Supports(
            DesktopOperationActions.SignMultiCosign))
        {
            GuidedMultiCosignSummary =
                Localizer.Text("winui.firmar.el_motor_local_no_ha_publicado_sign");
            return;
        }
        if (string.Equals(
            SelectedAction?.Value,
            "countersign",
            StringComparison.Ordinal))
        {
            GuidedMultiCosignSummary =
                Localizer.Text("winui.firmar.la_cofirma_multiple_guiada_admite_firma");
            return;
        }
        if (!IsGuidedMultiCosignFormatSupported())
        {
            GuidedMultiCosignSummary =
                Localizer.Text("winui.firmar.disponible_unicamente_para_pades_odf_u");
            return;
        }
        if (AdditionalSignerCandidates.Count == 0)
        {
            GuidedMultiCosignSummary =
                Localizer.Text("winui.firmar.se_necesitan_al_menos_dos_certificados");
            return;
        }
        if (!GuidedMultiCosignEnabled)
        {
            GuidedMultiCosignSummary =
                Localizer.Text("winui.firmar.activela_para_aplicar_primero_el");
            return;
        }
        if (_selectedAdditionalCertificateIds.Count == 0)
        {
            GuidedMultiCosignSummary =
                Localizer.Text("winui.firmar.seleccione_al_menos_un_certificado_4");
            return;
        }

        var selectedIds = new HashSet<string>(
            _selectedAdditionalCertificateIds,
            StringComparer.Ordinal);
        var names = AdditionalSignerCandidates
            .Where(certificate => selectedIds.Contains(certificate.Id))
            .Select(certificate => certificate.DisplayName)
            .Take(3)
            .ToArray();
        var remaining =
            _selectedAdditionalCertificateIds.Count - names.Length;
        var suffix = remaining > 0
            ? Localizer.Format("winui.firmar.y_mas", remaining)
            : string.Empty;
        GuidedMultiCosignSummary = Localizer.Format(
            "winui.firmar.principal_despues",
            SelectedCertificate?.DisplayName ?? Localizer.Text("winui.firmar.no_definido"),
            string.Join(", ", names), suffix);
    }

    private bool IsGuidedMultiCosignEligible() =>
        _session.Supports(DesktopOperationActions.SignMultiCosign) &&
        !string.Equals(
            SelectedAction?.Value,
            "countersign",
            StringComparison.Ordinal) &&
        IsGuidedMultiCosignFormatSupported();

    private bool IsGuidedMultiCosignFormatSupported()
    {
        var format = SelectedFormat?.Value;
        if (BatchModeEnabled &&
            string.IsNullOrWhiteSpace(format))
        {
            return false;
        }
        if (string.IsNullOrWhiteSpace(format))
        {
            format = InferFormatFromPath(_inputPath);
        }
        return string.Equals(
                format,
                "pades",
                StringComparison.OrdinalIgnoreCase) ||
            string.Equals(
                format,
                "odf",
                StringComparison.OrdinalIgnoreCase) ||
            string.Equals(
                format,
                "ooxml",
                StringComparison.OrdinalIgnoreCase);
    }

    private static string InferFormatFromPath(string? path) =>
        Path.GetExtension(path ?? string.Empty).ToLowerInvariant() switch
        {
            ".pdf" => "pades",
            ".odt" or ".ods" or ".odp" or ".odg" or ".odf" => "odf",
            ".docx" or ".xlsx" or ".pptx" or ".ppsx" => "ooxml",
            ".dsig" or ".xmlsig" => "xmldsig",
            ".xml" or ".xsig" => "xades",
            ".asics" => "asic-xades",
            _ => "cades",
        };

    private bool IsVisibleSealSupportedByCurrentSelection()
    {
        if ((!_portalSealMode && !_session.Supports(DesktopOperationActions.PdfPreview)) ||
            string.IsNullOrWhiteSpace(_inputPath) ||
            !string.Equals(
                Path.GetExtension(_inputPath),
                ".pdf",
                StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        var format = SelectedFormat?.Value;
        return string.IsNullOrEmpty(format) ||
            string.Equals(format, "pades", StringComparison.Ordinal);
    }

    private void OnVisibleSealGeometryChanged()
    {
        ConstrainVisibleSealCardToPage();
        SaveCurrentSealPlacement();
        ClearOutput();
        RaiseVisibleSealPreviewGeometryChanged();
        ScheduleSealStampPreview();
        UpdateValidationMessage();
    }

    // Las coordenadas guardadas son las de la tarjeta sin girar, con Y desde
    // abajo. Solo la caja girada limita el desplazamiento dentro de la página.
    private void ConstrainVisibleSealCardToPage()
    {
        var pageW = PreviewCanvasWidth;
        var pageH = PreviewCanvasHeight;
        var w = _visibleSealWidthPercent * pageW / 100;
        var h = _visibleSealHeightPercent * pageH / 100;
        var radians = _visibleSealRotationDegrees * Math.PI / 180;
        var bw = Math.Abs(w * Math.Cos(radians)) + Math.Abs(h * Math.Sin(radians));
        var bh = Math.Abs(w * Math.Sin(radians)) + Math.Abs(h * Math.Cos(radians));
        if (w <= 0 || h <= 0 || w > pageW || h > pageH || bw > pageW || bh > pageH)
        {
            return;
        }
        var cxMargin = Math.Max(bw, w) / 2;
        var cyMargin = Math.Max(bh, h) / 2;
        var cx = Math.Clamp((_visibleSealXPercent + _visibleSealWidthPercent / 2) * pageW / 100,
            cxMargin, pageW - cxMargin);
        var cy = Math.Clamp((_visibleSealYPercent + _visibleSealHeightPercent / 2) * pageH / 100,
            cyMargin, pageH - cyMargin);
        var x = (cx - w / 2) / pageW * 100;
        var y = (cy - h / 2) / pageH * 100;
        if (Math.Abs(x - _visibleSealXPercent) > 0.0001)
        {
            _visibleSealXPercent = x;
            RaisePropertyChanged(nameof(VisibleSealXPercent));
        }
        if (Math.Abs(y - _visibleSealYPercent) > 0.0001)
        {
            _visibleSealYPercent = y;
            RaisePropertyChanged(nameof(VisibleSealYPercent));
        }
    }

    private void RaiseVisibleSealPreviewGeometryChanged()
    {
        RaisePropertyChanged(nameof(PreviewCanvasWidth));
        RaisePropertyChanged(nameof(PreviewCanvasHeight));
        RaisePropertyChanged(nameof(VisibleSealPreviewX));
        RaisePropertyChanged(nameof(VisibleSealPreviewTop));
        RaisePropertyChanged(nameof(VisibleSealPreviewWidth));
        RaisePropertyChanged(nameof(VisibleSealPreviewHeight));
        RaisePropertyChanged(nameof(VisibleSealPreviewRotation));
        RaisePropertyChanged(nameof(VisibleSealPreviewContentWidth));
        RaisePropertyChanged(nameof(VisibleSealPreviewContentHeight));
        RaisePropertyChanged(nameof(VisibleSealGeometrySummary));
        RaisePropertyChanged(nameof(VisibleSealReadabilityHint));
    }

    private void ClearVisibleSealPreview()
    {
        _previewInputPath = null;
        _previewPageWidth = 0;
        _previewPageHeight = 0;
        _previewCurrentPage = 0;
        _previewTotalPages = 0;
        RaisePropertyChanged(nameof(VisibleSealPageSummary));
        RaisePropertyChanged(nameof(CanGoToPreviousSealPage));
        RaisePropertyChanged(nameof(CanGoToNextSealPage));
        RaisePropertyChanged(nameof(HasSealOnPreviewPage));
        _previewFileLength = 0;
        _previewFileLastWriteUtcTicks = 0;
        ClearBytes(_previewFileDigest);
        _previewFileDigest = null;
        _isPreviewImageRendered = false;
        VisibleSealPreviewImage = ReadOnlyMemory<byte>.Empty;
        VisibleSealPreviewMessage = VisibleSealEnabled
            ? Localizer.Text("winui.firmar.cargue_la_previsualizacion_para_obtener")
            : Localizer.Text("winui.firmar.active_el_sello_y_cargue_la");
        RaiseVisibleSealPreviewGeometryChanged();
        UpdateCommandStates();
    }

    private void ApplyBatchSelection(
        IReadOnlyList<string> combinedPaths)
    {
        _batchInputPaths = combinedPaths;
        var folderSuffix = string.IsNullOrWhiteSpace(
            _batchSourceDirectory)
            ? string.Empty
            : Localizer.Format("winui.firmar.incluye_la_carpeta",
                SafeDirectoryName(_batchSourceDirectory));
        BatchInputSummary = Localizer.Format(
            "winui.firmar.documento_s_seleccionado_s",
            combinedPaths.Count, folderSuffix);
        ResetBatchResults();
        UpdateValidationMessage();
        UpdateCommandStates();
    }

    private void ResetBatchResults()
    {
        BatchItems = _batchInputPaths
            .Select(path => new BatchSignDisplayItem(
                SafeFileName(path),
                Localizer.Text("winui.firmar.preparado"),
                Localizer.Text("winui.firmar.pendiente_de_ejecutar"),
                false,
                null))
            .ToArray();
        HasBatchResults = false;
        BatchProgressMaximum = Math.Max(1, _batchInputPaths.Count);
        BatchProgressValue = 0;
        IsBatchProgressIndeterminate = false;
        BatchProgressText = _batchInputPaths.Count == 0
            ? Localizer.Text("winui.firmar.el_lote_aun_no_se_ha_configurado")
            : Localizer.Text("winui.firmar.el_lote_esta_preparado_y_aun_no_se_ha");
        UpdateCommandStates();
    }

    private void InvalidateBatchResults()
    {
        if (HasBatchResults)
        {
            ResetBatchResults();
        }
    }

    private void MarkBatchNotProcessed(
        IReadOnlyList<string> paths,
        string detail)
    {
        BatchItems = paths
            .Select(path => new BatchSignDisplayItem(
                SafeFileName(path),
                Localizer.Text("winui.firmar.no_procesado"),
                detail,
                false,
                null))
            .ToArray();
        HasBatchResults = true;
        BatchProgressValue = 0;
        IsBatchProgressIndeterminate = false;
        BatchProgressText =
            Localizer.Text("winui.firmar.el_motor_no_confirmo_resultados_del_lote");
        UpdateCommandStates();
    }

    private void MarkPendingBatchAsCancelled()
    {
        BatchItems = BatchItems
            .Select(item =>
                string.Equals(
                    item.Status,
                    Localizer.Text("winui.firmar.en_espera"),
                    StringComparison.Ordinal)
                    ? item with
                    {
                        Status = Localizer.Text("winui.firmar.cancelado"),
                        Detail =
                            Localizer.Text("winui.firmar.no_existe_confirmacion_de_salida_para"),
                    }
                    : item)
            .ToArray();
        HasBatchResults = true;
        IsBatchProgressIndeterminate = false;
        BatchProgressText =
            Localizer.Text("winui.firmar.lote_cancelado_antes_de_recibir");
        UpdateCommandStates();
    }

    private void MarkPendingBatchAsUnconfirmed()
    {
        BatchItems = BatchItems
            .Select(item =>
                string.Equals(
                    item.Status,
                    Localizer.Text("winui.firmar.en_espera"),
                    StringComparison.Ordinal)
                    ? item with
                    {
                        Status = Localizer.Text("winui.firmar.sin_confirmar"),
                        Detail =
                            Localizer.Text("winui.firmar.la_comunicacion_termino_sin_confirmar"),
                    }
                    : item)
            .ToArray();
        HasBatchResults = true;
        IsBatchProgressIndeterminate = false;
        BatchProgressText =
            Localizer.Text("winui.firmar.no_se_recibieron_resultados_confirmados");
        UpdateCommandStates();
    }

    private bool TryApplyBatchResult(
        BatchSignResult result,
        IReadOnlyList<string> expectedPaths,
        string outputDirectory,
        out int successCount,
        out int failureCount)
    {
        successCount = 0;
        failureCount = 0;
        var coherent =
            result.SuccessCount >= 0 &&
            result.FailureCount >= 0 &&
            result.Results.Count == expectedPaths.Count &&
            result.SuccessCount + result.FailureCount ==
                expectedPaths.Count;
        var expectedByPath = new Dictionary<string, string>(
            StringComparer.OrdinalIgnoreCase);
        foreach (var path in expectedPaths)
        {
            if (!TryCanonicalPath(path, out var canonical) ||
                !expectedByPath.TryAdd(canonical, path))
            {
                coherent = false;
            }
        }

        var wireByPath = new Dictionary<string, BatchSignItemResult>(
            StringComparer.OrdinalIgnoreCase);
        foreach (var item in result.Results)
        {
            if (!TryCanonicalPath(item.InputPath, out var canonical) ||
                !expectedByPath.ContainsKey(canonical) ||
                !wireByPath.TryAdd(canonical, item))
            {
                coherent = false;
            }
        }

        var visibleItems =
            new List<BatchSignDisplayItem>(expectedPaths.Count);
        foreach (var expectedPath in expectedPaths)
        {
            if (!TryCanonicalPath(expectedPath, out var canonical) ||
                !wireByPath.TryGetValue(canonical, out var item))
            {
                failureCount++;
                visibleItems.Add(new BatchSignDisplayItem(
                    SafeFileName(expectedPath),
                    Localizer.Text("winui.comun.sin_resultado"),
                    Localizer.Text("winui.firmar.el_motor_omitio_el_resultado_de_este"),
                    false,
                    null));
                coherent = false;
                continue;
            }
            if (!item.IsSuccess)
            {
                failureCount++;
                visibleItems.Add(new BatchSignDisplayItem(
                    SafeFileName(expectedPath),
                    Localizer.Text("winui.firmar.fallo"),
                    string.IsNullOrWhiteSpace(item.Error)
                        ? Localizer.Text("winui.firmar.el_motor_indico_un_fallo_sin_detalle")
                        : item.Error,
                    false,
                    null));
                continue;
            }
            if (string.IsNullOrWhiteSpace(item.OutputPath) ||
                !IsDirectChildPath(
                    item.OutputPath,
                    outputDirectory) ||
                !HasNonEmptyOutput(item.OutputPath))
            {
                failureCount++;
                visibleItems.Add(new BatchSignDisplayItem(
                    SafeFileName(expectedPath),
                    Localizer.Text("winui.firmar.salida_no_confirmada"),
                    Localizer.Text("winui.firmar.la_respuesta_no_corresponde_a_un_fichero"),
                    false,
                    null));
                coherent = false;
                continue;
            }

            successCount++;
            visibleItems.Add(new BatchSignDisplayItem(
                SafeFileName(expectedPath),
                Localizer.Text("winui.firmar.estado_firmado"),
                Localizer.Format("winui.firmar.salida_confirmada",
                    SafeFileName(item.OutputPath)),
                true,
                item.OutputPath));
        }

        if (successCount != result.SuccessCount ||
            failureCount != result.FailureCount)
        {
            coherent = false;
        }
        BatchItems = visibleItems;
        HasBatchResults = true;
        UpdateCommandStates();
        return coherent;
    }

    private static bool TryNormalizeBatchPaths(
        IEnumerable<string> paths,
        out IReadOnlyList<string> normalizedPaths,
        out string error)
    {
        var normalized = new List<string>();
        var seen = new HashSet<string>(
            StringComparer.OrdinalIgnoreCase);
        long totalBytes = 0;
        foreach (var path in paths)
        {
            if (string.IsNullOrWhiteSpace(path))
            {
                continue;
            }

            string fullPath;
            FileInfo file;
            try
            {
                fullPath = Path.GetFullPath(path);
                file = new FileInfo(fullPath);
            }
            catch
            {
                normalizedPaths = [];
                error =
                    Localizer.Text("winui.firmar.uno_de_los_documentos_seleccionados");
                return false;
            }
            if (!file.Exists ||
                file.Length <= 0 ||
                file.Length > MaximumBatchDocumentBytes)
            {
                normalizedPaths = [];
                error =
                    Localizer.Format("winui.firmar.el_documento_no_existe_esta_vacio_o", SafeFileName(fullPath));
                return false;
            }
            if (!seen.Add(fullPath))
            {
                continue;
            }
            if (normalized.Count >= MaximumBatchDocuments)
            {
                normalizedPaths = [];
                error =
                    Localizer.Format("winui.firmar.el_lote_admite_como_maximo_documentos", MaximumBatchDocuments);
                return false;
            }
            totalBytes += file.Length;
            if (totalBytes > MaximumBatchPayloadBytes)
            {
                normalizedPaths = [];
                error =
                    Localizer.Text("winui.firmar.el_lote_supera_256_mb_en_conjunto");
                return false;
            }
            normalized.Add(fullPath);
        }

        normalizedPaths = normalized;
        error = string.Empty;
        return true;
    }

    private static bool TryCombineBatchInputs(
        IEnumerable<string> explicitPaths,
        IEnumerable<string> directoryPaths,
        out IReadOnlyList<string> combinedPaths,
        out string error) =>
        TryNormalizeBatchPaths(
            explicitPaths.Concat(directoryPaths),
            out combinedPaths,
            out error);

    private static bool TryEnumerateBatchDirectory(
        string path,
        out string directoryPath,
        out IReadOnlyList<string> inputs,
        out string error)
    {
        directoryPath = string.Empty;
        inputs = [];
        error = string.Empty;
        try
        {
            directoryPath = Path.GetFullPath(path);
            if (!Directory.Exists(directoryPath))
            {
                error =
                    Localizer.Text("winui.firmar.la_carpeta_seleccionada_ya_no_esta");
                return false;
            }

            var candidates = Directory
                .EnumerateFiles(
                    directoryPath,
                    "*",
                    SearchOption.TopDirectoryOnly)
                .Where(file =>
                {
                    try
                    {
                        return (
                            File.GetAttributes(file) &
                            FileAttributes.ReparsePoint) == 0;
                    }
                    catch
                    {
                        return false;
                    }
                })
                .OrderBy(
                    file => file,
                    StringComparer.OrdinalIgnoreCase)
                .Take(MaximumBatchDocuments + 1)
                .ToArray();
            if (candidates.Length == 0)
            {
                error =
                    Localizer.Text("winui.firmar.la_carpeta_seleccionada_no_contiene");
                return false;
            }
            return TryNormalizeBatchPaths(
                candidates,
                out inputs,
                out error);
        }
        catch
        {
            error =
                Localizer.Text("winui.firmar.no_se_pudo_enumerar_de_forma_segura_la");
            return false;
        }
    }

    private static bool HasBatchOutputNameCollision(
        IEnumerable<string> inputPaths,
        string selectedFormat,
        out string collisionName)
    {
        var names = new HashSet<string>(
            StringComparer.OrdinalIgnoreCase);
        foreach (var inputPath in inputPaths)
        {
            var format = string.IsNullOrWhiteSpace(selectedFormat)
                ? InferFormatFromPath(inputPath)
                : selectedFormat;
            var outputName =
                BatchOutputName(inputPath, format);
            if (!names.Add(outputName))
            {
                collisionName = outputName;
                return true;
            }
        }
        collisionName = string.Empty;
        return false;
    }

    private static string BatchOutputName(
        string inputPath,
        string format)
    {
        var baseName = Path.GetFileNameWithoutExtension(inputPath);
        var suffix = format.ToLowerInvariant() switch
        {
            "pades" => "_firmado.pdf",
            "xmldsig" => "_firmado.dsig",
            "xades" => "_firmado.xsig",
            _ => "_firmado.p7s",
        };
        return $"{baseName}{suffix}";
    }

    private static bool TryCanonicalPath(
        string? path,
        out string canonical)
    {
        canonical = string.Empty;
        if (string.IsNullOrWhiteSpace(path))
        {
            return false;
        }
        try
        {
            canonical = Path.GetFullPath(path);
            return true;
        }
        catch
        {
            return false;
        }
    }

    private static bool IsDirectChildPath(
        string path,
        string directory)
    {
        try
        {
            var fullDirectory = Path.TrimEndingDirectorySeparator(
                Path.GetFullPath(directory));
            var parent = Path.GetDirectoryName(
                Path.GetFullPath(path));
            return !string.IsNullOrWhiteSpace(parent) &&
                string.Equals(
                    Path.TrimEndingDirectorySeparator(parent),
                    fullDirectory,
                    StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            return false;
        }
    }

    private static string SafeDirectoryName(string path)
    {
        try
        {
            var normalized =
                Path.TrimEndingDirectorySeparator(path);
            var name = Path.GetFileName(normalized);
            return string.IsNullOrWhiteSpace(name)
                ? Localizer.Text("winui.firmar.carpeta_seleccionada")
                : name;
        }
        catch
        {
            return Localizer.Text("winui.firmar.carpeta_seleccionada");
        }
    }

    private bool TryValidateVisibleSealGeometry(out string error)
    {
        error = string.Empty;
        var values = new[]
        {
            VisibleSealXPercent,
            VisibleSealYPercent,
            VisibleSealWidthPercent,
            VisibleSealHeightPercent,
        };
        if (values.Any(value =>
            !double.IsFinite(value) ||
            value < 0 ||
            value > 100))
        {
            error =
                Localizer.Text("winui.firmar.x_y_ancho_y_alto_deben_ser_porcentajes");
            return false;
        }
        if (VisibleSealWidthPercent <= 0 ||
            VisibleSealHeightPercent <= 0)
        {
            error =
                Localizer.Text("winui.firmar.el_ancho_y_el_alto_del_sello_deben_ser");
            return false;
        }
        if (VisibleSealXPercent + VisibleSealWidthPercent > 100 ||
            VisibleSealYPercent + VisibleSealHeightPercent > 100)
        {
            error =
                Localizer.Text("winui.firmar.la_zona_del_sello_debe_quedar");
            return false;
        }
        return true;
    }

    private static bool TryParsePageSelection(
        string? raw,
        int totalPages,
        out string normalized,
        out int previewPage,
        out string error)
    {
        normalized = string.Empty;
        previewPage = 1;
        error = string.Empty;
        if (string.IsNullOrWhiteSpace(raw))
        {
            error =
                Localizer.Text("winui.firmar.indique_las_paginas_del_sello_1_1_3_5_o");
            return false;
        }
        if (raw.Length > MaximumPageSelectionLength ||
            raw.Any(character =>
                char.IsControl(character) &&
                !char.IsWhiteSpace(character)))
        {
            error =
                Localizer.Text("winui.firmar.la_seleccion_de_paginas_es_demasiado");
            return false;
        }

        normalized = string.Concat(raw.Where(
            character => !char.IsWhiteSpace(character)));
        if (string.Equals(
            normalized,
            "all",
            StringComparison.OrdinalIgnoreCase) ||
            normalized == "*")
        {
            normalized = "all";
            return true;
        }

        var parts = normalized.Split(
            ',',
            StringSplitOptions.None);
        if (parts.Length is 0 or > 32 ||
            parts.Any(string.IsNullOrWhiteSpace))
        {
            error =
                Localizer.Text("winui.firmar.la_seleccion_de_paginas_no_es_valida_use");
            return false;
        }

        var normalizedParts = new List<string>(parts.Length);
        var firstPageAssigned = false;
        foreach (var part in parts)
        {
            var bounds = part.Split('-', StringSplitOptions.None);
            if (bounds.Length is < 1 or > 2 ||
                !int.TryParse(
                    bounds[0],
                    System.Globalization.NumberStyles.None,
                    System.Globalization.CultureInfo.InvariantCulture,
                    out var start) ||
                start < 1 ||
                start > 1_000_000)
            {
                error =
                    Localizer.Text("winui.firmar.la_seleccion_de_paginas_no_es_valida_use_2");
                return false;
            }

            var end = start;
            if (bounds.Length == 2 &&
                (!int.TryParse(
                    bounds[1],
                    System.Globalization.NumberStyles.None,
                    System.Globalization.CultureInfo.InvariantCulture,
                    out end) ||
                 end < start ||
                 end > 1_000_000))
            {
                error =
                    Localizer.Text("winui.firmar.cada_rango_de_paginas_debe_ir_de_menor_a");
                return false;
            }
            if (totalPages > 0 && end > totalPages)
            {
                error =
                    Localizer.Format("winui.firmar.la_seleccion_incluye_la_pagina_pero_el", end, totalPages);
                return false;
            }
            if (!firstPageAssigned)
            {
                previewPage = start;
                firstPageAssigned = true;
            }
            normalizedParts.Add(
                bounds.Length == 1
                    ? start.ToString(
                        System.Globalization.CultureInfo.InvariantCulture)
                    : string.Create(
                        System.Globalization.CultureInfo.InvariantCulture,
                        $"{start}-{end}"));
        }

        normalized = string.Join(",", normalizedParts);
        return true;
    }

    private static bool TryValidatePreviewResult(
        PdfPreviewResult? result,
        int requestedPage,
        out ReadOnlyMemory<byte> previewImage,
        out string error)
    {
        previewImage = ReadOnlyMemory<byte>.Empty;
        error =
            Localizer.Text("winui.firmar.el_motor_no_devolvio_una");
        if (result is null)
        {
            return false;
        }
        if (!IsPositiveFinite(result.Width) ||
            !IsPositiveFinite(result.Height) ||
            result.Width > 100_000 ||
            result.Height > 100_000 ||
            result.CurrentPage != requestedPage ||
            result.TotalPages < result.CurrentPage ||
            result.TotalPages > 1_000_000 ||
            result.Data is null ||
            result.Data.Length is < 8 or > MaximumPreviewImageBytes)
        {
            ClearBytes(result.Data);
            return false;
        }

        if (result.Data[0] != 0x89 ||
            result.Data[1] != 0x50 ||
            result.Data[2] != 0x4E ||
            result.Data[3] != 0x47 ||
            result.Data[4] != 0x0D ||
            result.Data[5] != 0x0A ||
            result.Data[6] != 0x1A ||
            result.Data[7] != 0x0A)
        {
            ClearBytes(result.Data);
            return false;
        }

        previewImage = result.Data;
        error = string.Empty;
        return true;
    }

    private static bool TryNormalizeMetadata(
        string? value,
        string label,
        out string? normalized,
        out string error)
    {
        normalized = null;
        error = string.Empty;
        if (string.IsNullOrWhiteSpace(value))
        {
            return true;
        }
        if (value.Length > MaximumMetadataLength ||
            value.Any(char.IsControl))
        {
            error = Localizer.Format(
                "winui.firmar.el_campo_admite_hasta_caracteres_de",
                Localizer.Text(label), MaximumMetadataLength);
            return false;
        }
        normalized = value.Trim();
        return true;
    }

    private static bool TryReadFileStamp(
        string path,
        out long length,
        out long lastWriteUtcTicks)
    {
        length = 0;
        lastWriteUtcTicks = 0;
        try
        {
            var file = new FileInfo(path);
            if (!file.Exists || file.Length <= 0)
            {
                return false;
            }
            length = file.Length;
            lastWriteUtcTicks = file.LastWriteTimeUtc.Ticks;
            return true;
        }
        catch
        {
            return false;
        }
    }

    private static FileStream? OpenPdfReadGuard(string path)
    {
        try
        {
            var stream = new FileStream(
                path,
                new FileStreamOptions
                {
                    Mode = FileMode.Open,
                    Access = FileAccess.Read,
                    Share = FileShare.Read,
                    BufferSize = 81920,
                    Options =
                        FileOptions.Asynchronous |
                        FileOptions.SequentialScan,
                });
            if (stream.Length is <= 0 or > MaximumPreviewPdfBytes)
            {
                stream.Dispose();
                return null;
            }
            return stream;
        }
        catch
        {
            return null;
        }
    }

    private static async Task<byte[]> ComputeSha256Async(
        FileStream stream,
        CancellationToken cancellationToken)
    {
        stream.Position = 0;
        using var sha256 = SHA256.Create();
        return await sha256.ComputeHashAsync(
            stream,
            cancellationToken);
    }

    private static void ClearBytes(byte[]? buffer)
    {
        if (buffer is not null)
        {
            CryptographicOperations.ZeroMemory(buffer);
        }
    }

    private static void ClearMemory(ReadOnlyMemory<byte> buffer)
    {
        if (!buffer.IsEmpty &&
            MemoryMarshal.TryGetArray(
                buffer,
                out ArraySegment<byte> segment))
        {
            CryptographicOperations.ZeroMemory(segment.AsSpan());
        }
    }

    private static OperationDiagnostic VisibleSealValidationDiagnostic(
        string failureCode,
        string userMessage,
        string suggestedAction,
        DiagnosticStepStatus status) =>
        new()
        {
            Category = "configuration",
            FailureCode = failureCode,
            UserMessage = userMessage,
            ExpertMessage =
                Localizer.Text("winui.firmar.la_validacion_local_detuvo_la_operacion"),
            LikelyOwner = "local",
            ResponsibilityMessage =
                Localizer.Text("winui.firmar.el_fallo_se_ha_detectado_en_la"),
            SuggestedAction = suggestedAction,
            UserCanResolveDirectly = true,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = "document_selected",
                    Label = Localizer.Text("winui.firmar.documento_pdf_seleccionado"),
                    Status = DiagnosticStepStatus.Success,
                    Owner = "local",
                    UserMessage =
                        Localizer.Text("winui.firmar.la_aplicacion_dispone_de_una_seleccion"),
                    EvidenceRef = "phase:input",
                },
                new OperationDiagnosticStep
                {
                    Code = failureCode,
                    Label = Localizer.Text("winui.firmar.configuracion_del_sello_visible"),
                    Status = status,
                    Owner = "local",
                    UserMessage = userMessage,
                    SuggestedAction = suggestedAction,
                    EvidenceRef = "phase:configuration",
                },
                new OperationDiagnosticStep
                {
                    Code = "sign_not_started",
                    Label = Localizer.Text("winui.firmar.firma_criptografica"),
                    Status = DiagnosticStepStatus.Skipped,
                    Owner = "local",
                    UserMessage =
                        Localizer.Text("winui.firmar.la_firma_no_se_inicio_porque_la"),
                    EvidenceRef = "phase:operation",
                },
            ],
        };

    private static bool IsPositiveFinite(double value) =>
        double.IsFinite(value) && value > 0;

    private static double PercentToPoints(
        double percentage,
        double dimension) =>
        double.IsFinite(percentage) &&
        percentage >= 0 &&
        IsPositiveFinite(dimension)
            ? percentage / 100 * dimension
            : 0;

    private static SaveFilePickerProfile ResolveSaveProfile(
        SignatureFormatOption format,
        string inputPath)
    {
        if (!string.IsNullOrEmpty(format.Value))
        {
            return format.SaveProfile;
        }

        var extension = Path.GetExtension(inputPath);
        return string.Equals(
            extension,
            ".pdf",
            StringComparison.OrdinalIgnoreCase)
            ? SaveFilePickerProfile.SignedPdf
            : string.Equals(
                extension,
                ".xml",
                StringComparison.OrdinalIgnoreCase) ||
              string.Equals(
                extension,
                ".xsig",
                StringComparison.OrdinalIgnoreCase)
                ? SaveFilePickerProfile.XadesSignature
                : SaveFilePickerProfile.CadesSignature;
    }

    private static string SuggestedOutputName(string inputPath)
    {
        var baseName = Path.GetFileNameWithoutExtension(inputPath);
        return string.IsNullOrWhiteSpace(baseName)
            ? Localizer.Text("winui.firmar.documento_firmado")
            : baseName + Localizer.Text("winui.firmar.firmado");
    }

    private static string SafeFileName(string path)
    {
        try
        {
            var fileName = Path.GetFileName(path);
            return string.IsNullOrWhiteSpace(fileName)
                ? Localizer.Text("winui.firmar.documento_seleccionado")
                : fileName;
        }
        catch
        {
            return Localizer.Text("winui.firmar.documento_seleccionado");
        }
    }

    private static bool PathsEqual(string first, string second)
    {
        try
        {
            return string.Equals(
                Path.GetFullPath(first),
                Path.GetFullPath(second),
                StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            return false;
        }
    }

    private static bool HasNonEmptyOutput(string path)
    {
        try
        {
            return new FileInfo(path).Length > 0;
        }
        catch
        {
            return false;
        }
    }

    private static bool IsCoherentVerification(VerifyResult? data) =>
        VerificationAssessment.IsCoherent(data);

    private static string PostValidationSuccessMessage(VerifyResult data)
    {
        var trust = data.Trust.Status?.Trim().ToLowerInvariant();
        return trust switch
        {
            "valid" =>
                Localizer.Text("winui.firmar.la_firma_termino_y_la_validacion"),
            "invalid" =>
                Localizer.Text("winui.firmar.la_firma_es_criptograficamente_valida"),
            "warning" =>
                Localizer.Text("winui.firmar.la_firma_es_criptograficamente_valida_y"),
            _ =>
                Localizer.Text("winui.firmar.la_firma_es_criptograficamente_valida_2"),
        };
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
