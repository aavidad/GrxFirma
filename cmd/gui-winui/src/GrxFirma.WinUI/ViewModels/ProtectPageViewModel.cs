// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Security.Cryptography;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;

namespace GrxFirma.WinUI.ViewModels;

public sealed record ProtectionProfileOption(string SourceLabel, string Value)
{
    public string Label => Localizer.Text(SourceLabel);
    // El lector de pantalla anuncia ToString(): nunca el volcado del record.
    public override string ToString() => Label;
}

public sealed record ProtectionContainerOption(
    string SourceLabel,
    string Value,
    SaveFilePickerProfile SaveProfile,
    bool RequiresTransientSecret = false)
{
    public string Label => Localizer.Text(SourceLabel);
    // El lector de pantalla anuncia ToString(): nunca el volcado del record.
    public override string ToString() => Label;
}

public sealed record ProtectionRecipientItem
{
    public required string Id { get; init; }
    public required string Label { get; init; }
    public required string Detail { get; init; }
    public required string Profile { get; init; }
    public required string Origin { get; init; }
    public required bool AuthEnvelopedDataCompatible { get; init; }

    // Nombre que anuncia el lector de pantalla en la lista de destinatarios.
    public override string ToString() => Label + ". " + Detail;
}

public sealed record ProtectionSignerItem
{
    public required string Id { get; init; }
    public required string Label { get; init; }

    // Datos del certificado para saber si es remoto y pide PIN u OTP.
    public CertificateInfo? Certificate { get; init; }

    public override string ToString() => Label;
}

public sealed class ProtectPageViewModel
    : WorkspacePageViewModel
{
    private const int MaximumCatalogItems = 128;
    private const int Aes256SecretBytes = 32;
    private static string CompatRecipientGuidance =>
        Localizer.Text("winui.proteger.el_perfil_compatible_requiere_una");

    private readonly DesktopOperationSession _session;
    private readonly IFilePickerService _filePicker;
    private IReadOnlyList<ProtectionRecipientItem> _allRecipients = [];
    private IReadOnlyList<ProtectionRecipientItem> _visibleRecipients = [];
    private IReadOnlyList<ProtectionContainerOption> _visibleContainers = [];
    private IReadOnlyList<ProtectionSignerItem> _signingCertificates = [];
    private IReadOnlyList<string> _selectedRecipientIds = [];
    private ProtectionProfileOption? _selectedProfile;
    private ProtectionContainerOption? _selectedContainer;
    private ProtectionSignerItem? _selectedSigningCertificate;
    private CancellationTokenSource? _activeCancellation;
    private string? _protectInputPath;
    private string? _protectOutputPath;
    private string? _unprotectInputPath;
    private string? _unprotectOutputPath;
    private string _protectInputDisplayName = string.Empty;
    private string _unprotectInputDisplayName = string.Empty;
    private string _protectValidationMessage =
        Localizer.Text("winui.proteger.seleccione_un_documento_y_al_menos_un");
    private string _unprotectValidationMessage =
        Localizer.Text("winui.proteger.seleccione_un_contenedor_protegido");
    private string _protectResultMessage =
        Localizer.Text("winui.proteger.no_se_ha_ejecutado_ninguna_proteccion");
    private string _unprotectResultMessage =
        Localizer.Text("winui.proteger.no_se_ha_ejecutado_ninguna_desproteccion");
    private int _selectedRecipientCount;
    private string _selectedRecipientSummary =
        Localizer.Text("winui.proteger.no_hay_destinatarios_seleccionados");
    private bool _isProtectAndSign;
    private bool _supportsProtectAndSign;
    private bool _isBusy;
    private bool _canSelectFiles;
    private bool _canRemoveRecipient;
    private bool _canProtect;
    private bool _canUnprotect;
    private bool _canCancel;
    private bool _canOpenProtectedOutput;
    private bool _canOpenUnprotectedOutput;
    private bool _canUseProtectAndSign;
    private bool _canSelectSigningCertificate;
    private bool _isEncryptedDataSelected;
    private bool _isEncryptedDataUnprotectSelected;
    private int _operationInProgress;

    public ProtectPageViewModel(
        DesktopOperationSession session,
        IFilePickerService filePicker)
        : base(
            Localizer.Text("winui.comun.proteger"),
            Localizer.Text("winui.proteger.protege_documentos_para_destinatarios"),
            Localizer.Text("winui.proteger.la_proteccion_no_esta_disponible_porque"))
    {
        ArgumentNullException.ThrowIfNull(session);
        ArgumentNullException.ThrowIfNull(filePicker);
        _session = session;
        _filePicker = filePicker;
        _selectedProfile = Profiles[0];
        RebuildContainers();
        UpdateAvailability();
    }

    public IReadOnlyList<ProtectionProfileOption> Profiles { get; } =
    [
        new("winui.proteger.compatible_rsa_oaep", "compat"),
        new("winui.proteger.alta_seguridad_ml_kem_x25519", "alto"),
    ];

    public IReadOnlyList<ProtectionContainerOption> Containers { get; } =
    [
        new(
            "winui.proteger.json_grxfirma_afp",
            "json",
            SaveFilePickerProfile.ProtectedJson),
        new(
            "winui.proteger.cms_envelopeddata_enveloped",
            "cms",
            SaveFilePickerProfile.CmsEnveloped),
        new(
            "winui.proteger.cms_encrypteddata_requiere_clave_efimera",
            "cms-encrypted",
            SaveFilePickerProfile.CmsEncrypted,
            RequiresTransientSecret: true),
        new(
            "winui.proteger.cms_authenvelopeddata_authenveloped_p7m",
            "authenvelopeddata",
            SaveFilePickerProfile.CmsAuthEnveloped),
    ];

    public IReadOnlyList<ProtectionContainerOption> VisibleContainers
    {
        get => _visibleContainers;
        private set => SetProperty(ref _visibleContainers, value);
    }

    public ProtectionProfileOption? SelectedProfile
    {
        get => _selectedProfile;
        set
        {
            if (SetProperty(ref _selectedProfile, value))
            {
                if (value?.Value == "alto")
                {
                    IsProtectAndSign = false;
                }
                RebuildContainers();
                RebuildVisibleRecipients();
                ClearProtectedOutput();
                UpdateValidationMessages();
                UpdateCommandStates();
            }
        }
    }

    public ProtectionContainerOption? SelectedContainer
    {
        get => _selectedContainer;
        set
        {
            if (SetProperty(ref _selectedContainer, value))
            {
                if (value?.RequiresTransientSecret == true)
                {
                    IsProtectAndSign = false;
                }
                RebuildVisibleRecipients();
                ClearProtectedOutput();
                UpdateValidationMessages();
                UpdateCommandStates();
            }
        }
    }

    public IReadOnlyList<ProtectionRecipientItem> VisibleRecipients
    {
        get => _visibleRecipients;
        private set => SetProperty(ref _visibleRecipients, value);
    }

    public IReadOnlyList<ProtectionSignerItem> SigningCertificates
    {
        get => _signingCertificates;
        private set => SetProperty(ref _signingCertificates, value);
    }

    public ProtectionSignerItem? SelectedSigningCertificate
    {
        get => _selectedSigningCertificate;
        set
        {
            if (SetProperty(ref _selectedSigningCertificate, value))
            {
                UpdateValidationMessages();
                UpdateCommandStates();
            }
        }
    }

    public bool IsProtectAndSign
    {
        get => _isProtectAndSign;
        set
        {
            var normalized = value && _supportsProtectAndSign;
            if (SetProperty(ref _isProtectAndSign, normalized))
            {
                if (normalized)
                {
                    SelectedProfile = Profiles.First(profile =>
                        profile.Value == "compat");
                    RebuildContainers();
                    SelectedContainer = VisibleContainers.First(container =>
                        container.Value == "cms");
                }
                ClearProtectedOutput();
                UpdateValidationMessages();
                UpdateCommandStates();
            }
        }
    }

    public bool CanUseProtectAndSign
    {
        get => _canUseProtectAndSign;
        private set => SetProperty(ref _canUseProtectAndSign, value);
    }

    public bool CanSelectSigningCertificate
    {
        get => _canSelectSigningCertificate;
        private set => SetProperty(
            ref _canSelectSigningCertificate,
            value);
    }

    public bool IsEncryptedDataSelected
    {
        get => _isEncryptedDataSelected;
        private set => SetProperty(ref _isEncryptedDataSelected, value);
    }

    public bool IsEncryptedDataUnprotectSelected
    {
        get => _isEncryptedDataUnprotectSelected;
        private set => SetProperty(
            ref _isEncryptedDataUnprotectSelected,
            value);
    }

    public string ProtectInputDisplayName
    {
        // Vacío sin selección: el PlaceholderText traducido muestra el aviso
        // y sigue el idioma aunque cambie en caliente.
        get => _protectInputDisplayName;
        private set => SetProperty(ref _protectInputDisplayName, value);
    }

    public string UnprotectInputDisplayName
    {
        // Vacío sin selección: el PlaceholderText traducido muestra el aviso
        // y sigue el idioma aunque cambie en caliente.
        get => _unprotectInputDisplayName;
        private set => SetProperty(ref _unprotectInputDisplayName, value);
    }

    public string ProtectValidationMessage
    {
        get => _protectValidationMessage;
        private set => SetProperty(ref _protectValidationMessage, value);
    }

    public string UnprotectValidationMessage
    {
        get => _unprotectValidationMessage;
        private set => SetProperty(ref _unprotectValidationMessage, value);
    }

    public string ProtectResultMessage
    {
        get => _protectResultMessage;
        private set => SetProperty(ref _protectResultMessage, value);
    }

    public string UnprotectResultMessage
    {
        get => _unprotectResultMessage;
        private set => SetProperty(ref _unprotectResultMessage, value);
    }

    public int SelectedRecipientCount
    {
        get => _selectedRecipientCount;
        private set => SetProperty(ref _selectedRecipientCount, value);
    }

    public string SelectedRecipientSummary
    {
        get => _selectedRecipientSummary;
        private set => SetProperty(ref _selectedRecipientSummary, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    public bool CanSelectFiles
    {
        get => _canSelectFiles;
        private set => SetProperty(ref _canSelectFiles, value);
    }

    public bool CanRemoveRecipient
    {
        get => _canRemoveRecipient;
        private set => SetProperty(ref _canRemoveRecipient, value);
    }

    public bool CanProtect
    {
        get => _canProtect;
        private set => SetProperty(ref _canProtect, value);
    }

    public bool CanUnprotect
    {
        get => _canUnprotect;
        private set => SetProperty(ref _canUnprotect, value);
    }

    public bool CanCancel
    {
        get => _canCancel;
        private set => SetProperty(ref _canCancel, value);
    }

    public bool CanOpenProtectedOutput
    {
        get => _canOpenProtectedOutput;
        private set => SetProperty(ref _canOpenProtectedOutput, value);
    }

    public bool CanOpenUnprotectedOutput
    {
        get => _canOpenUnprotectedOutput;
        private set => SetProperty(ref _canOpenUnprotectedOutput, value);
    }

    public string? ProtectedOutputPath => _protectOutputPath;
    public string? UnprotectedOutputPath => _unprotectOutputPath;

    public void UpdateAvailability()
    {
        var available =
            _session.Supports(DesktopOperationActions.ProtectionRecipients) &&
            _session.Supports(DesktopOperationActions.Protect) &&
            _session.Supports(DesktopOperationActions.Unprotect);
        _supportsProtectAndSign =
            _session.Supports(DesktopOperationActions.ProtectAndSign) &&
            _session.Supports(DesktopOperationActions.Certificates);
        SetOperationAvailability(
            available,
            Localizer.Text("winui.proteger.motor_local_conectado_proteccion_y"));
        if (!available)
        {
            CancelCurrentOperation();
            _allRecipients = [];
            VisibleRecipients = [];
            SetSelectedRecipients([]);
        }
        if (!_supportsProtectAndSign)
        {
            IsProtectAndSign = false;
            SigningCertificates = [];
            SelectedSigningCertificate = null;
        }
        UpdateValidationMessages();
        UpdateCommandStates();
    }

    public void SetSelectedRecipients(
        IEnumerable<ProtectionRecipientItem>? recipients)
    {
        var allowed = new HashSet<string>(
            VisibleRecipients.Select(item => item.Id),
            StringComparer.Ordinal);
        _selectedRecipientIds = (recipients ?? [])
            .Select(item => item.Id)
            .Where(id =>
                !string.IsNullOrWhiteSpace(id) &&
                allowed.Contains(id))
            .Distinct(StringComparer.Ordinal)
            .Take(MaximumCatalogItems)
            .ToArray();
        SelectedRecipientCount = _selectedRecipientIds.Count;
        SelectedRecipientSummary =
            SelectedRecipientCount == 0
                ? Localizer.Text("winui.proteger.no_hay_destinatarios_seleccionados")
                : SelectedRecipientCount == 1
                    ? Localizer.Text("winui.proteger.1_destinatario_seleccionado")
                    : Localizer.Format("winui.proteger.destinatarios_seleccionados", SelectedRecipientCount);
        UpdateValidationMessages();
        UpdateCommandStates();
    }

    public async Task<OperationDiagnostic?> ImportPublicRecipientAsync(CancellationToken cancellationToken)
    {
        var path = await _filePicker.PickOpenFileAsync(
            OpenFilePickerProfile.PublicRecipientCertificate, cancellationToken);
        if (string.IsNullOrWhiteSpace(path)) return null;
        if (!_session.TryGetOperations(DesktopOperationActions.ProtectionRecipientImport, out var operations))
        {
            ProtectValidationMessage = Localizer.Text("winui.proteger.el_motor_local_no_permite_anadir");
            return null;
        }
        var result = await operations.ImportProtectionRecipientAsync(new() { Path = path }, cancellationToken);
        if (!IsSuccessful(result))
        {
            ProtectValidationMessage = result.SafeUserMessage;
            return OperationDiagnosticMapper.FromResult(result);
        }
        ProtectValidationMessage = Localizer.Text("winui.proteger.certificado_publico_anadido_seleccione");
        return await RefreshCatalogsAsync(cancellationToken);
    }

    public async Task<OperationDiagnostic?> RemovePublicRecipientAsync(CancellationToken cancellationToken)
    {
        if (!CanRemoveRecipient) return null;
        var id = _selectedRecipientIds[0];
        if (!_session.TryGetOperations(DesktopOperationActions.ProtectionRecipientRemove, out var operations))
        {
            ProtectValidationMessage = Localizer.Text("winui.proteger.el_motor_local_no_permite_quitar");
            return null;
        }
        var result = await operations.RemoveProtectionRecipientAsync(new() { Id = id }, cancellationToken);
        if (!IsSuccessful(result))
        {
            ProtectValidationMessage = result.SafeUserMessage;
            return OperationDiagnosticMapper.FromResult(result);
        }
        ProtectValidationMessage = Localizer.Text("winui.proteger.destinatario_importado_quitado");
        return await RefreshCatalogsAsync(cancellationToken);
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

    public async Task<OperationDiagnostic?> RefreshCatalogsAsync(
        CancellationToken cancellationToken = default)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.espere_a_que_termine_la_operacion_actual");
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.ProtectionRecipients,
                out var operations))
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.el_motor_local_no_ofrece_el_catalogo_de");
                return null;
            }

            ProtectValidationMessage = Localizer.Text("winui.proteger.consultando_destinatarios_de_proteccion");
            var recipientsResult =
                await operations.GetProtectionRecipientsAsync(
                    operationCancellation.Token);
            if (!IsSuccessful(recipientsResult))
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.el_motor_local_no_pudo_cargar_los");
                return OperationDiagnosticMapper.FromResult(recipientsResult);
            }
            if (recipientsResult.Data is null)
            {
                return InvalidResultDiagnostic(
                    recipientsResult,
                    "MISSING_PROTECTION_RECIPIENTS");
            }

            _allRecipients = recipientsResult.Data.VisibleRecipients
                .GroupBy(recipient =>
                    recipient.Id,
                    StringComparer.Ordinal)
                .Select(group =>
                {
                    var recipient = group.First();
                    return new ProtectionRecipientItem
                    {
                        Id = recipient.Id,
                        Label = recipient.Label,
                        Detail = Localizer.Format("winui.proteger.origen_perfil",
                            Localizer.Text(OriginLabel(recipient.Origin)),
                            Localizer.Text(recipient.Profile)) +
                            (string.IsNullOrWhiteSpace(recipient.Algorithm) ? "" :
                                Localizer.Format("winui.proteger.algoritmo", recipient.Algorithm)),
                        Profile = recipient.Profile,
                        Origin = recipient.Origin,
                        AuthEnvelopedDataCompatible =
                            recipient.AuthEnvelopedDataCompatible,
                    };
                })
                .Take(MaximumCatalogItems)
                .ToArray();
            SetSelectedRecipients([]);
            RebuildVisibleRecipients();

            if (_supportsProtectAndSign &&
                _session.TryGetOperations(
                    DesktopOperationActions.Certificates,
                    out operations))
            {
                var certificateResult = await operations.GetCertificatesAsync(
                    operationCancellation.Token);
                if (!IsSuccessful(certificateResult))
                {
                    ProtectValidationMessage =
                        Localizer.Text("winui.proteger.no_se_pudieron_cargar_los_certificados");
                    return OperationDiagnosticMapper.FromResult(
                        certificateResult);
                }
                if (certificateResult.Data is null)
                {
                    return InvalidResultDiagnostic(
                        certificateResult,
                        "MISSING_SIGNING_CERTIFICATES");
                }

                SigningCertificates = certificateResult.Data
                    .Where(certificate =>
                        !string.IsNullOrWhiteSpace(certificate.Id) &&
                        certificate.CanSign &&
                        !certificate.IsExpired)
                    .GroupBy(
                        certificate => certificate.Id,
                        StringComparer.Ordinal)
                    .Select(group =>
                    {
                        var certificate = group.First();
                        var label = !string.IsNullOrWhiteSpace(
                            certificate.SubjectName)
                            ? certificate.SubjectName
                            : certificate.Subject;
                        return new ProtectionSignerItem
                        {
                            Id = certificate.Id,
                            Label = string.IsNullOrWhiteSpace(label)
                                ? Localizer.Text("winui.comun.certificado_sin_titular")
                                : label,
                            Certificate = certificate,
                        };
                    })
                    .Take(MaximumCatalogItems)
                    .ToArray();
                SelectedSigningCertificate =
                    SigningCertificates.FirstOrDefault();
            }

            UpdateValidationMessages();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException exception)
        {
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.la_actualizacion_de_destinatarios_se");
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.fallo_la_comunicacion_segura_al");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.la_aplicacion_no_pudo_cargar_los");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    public Task<OperationDiagnostic?> SelectProtectInputAsync(
        CancellationToken cancellationToken = default) =>
        SelectInputAsync(
            OpenFilePickerProfile.SignedOrOriginalDocument,
            isProtectedInput: false,
            cancellationToken);

    public Task<OperationDiagnostic?> SelectUnprotectInputAsync(
        CancellationToken cancellationToken = default) =>
        SelectInputAsync(
            OpenFilePickerProfile.ProtectedContainer,
            isProtectedInput: true,
            cancellationToken);

    /// <summary>
    /// Pide el PIN y el OTP del certificado remoto que firma en «Proteger y
    /// firmar», con el mismo diálogo que Firmar. Devuelve null si la persona
    /// cancela.
    /// </summary>
    public Func<CertificateInfo, CancellationToken, Task<RemoteSigningSecrets?>>? RemoteSecretsPrompt { get; set; }

    public async Task<OperationDiagnostic?> ProtectAsync(
        byte[]? transientSecret,
        byte[]? transientSecretConfirmation,
        CancellationToken cancellationToken = default)
    {
        CancellationTokenSource? operationCancellation = null;
        Dictionary<string, string>? options = null;
        RemoteSigningSecrets? remoteSecrets = null;
        try
        {
            var validation = ValidateBeforeProtect(
                transientSecret,
                transientSecretConfirmation);
            if (validation is not null)
            {
                ProtectValidationMessage = validation;
                return null;
            }
            if (!TryBeginOperation(
                cancellationToken,
                out operationCancellation))
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.ya_hay_una_operacion_en_curso_espere_o");
                return null;
            }

            ClearProtectedOutput();
            var signToo = IsProtectAndSign;
            var action = signToo
                ? DesktopOperationActions.ProtectAndSign
                : DesktopOperationActions.Protect;
            if (!_session.TryGetOperations(action, out var operations))
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.el_motor_local_no_ofrece_la_operacion_de");
                return null;
            }

            var container = SelectedContainer!;
            var encryptedData = container.RequiresTransientSecret;
            var saveProfile = signToo
                ? SaveFilePickerProfile.CmsSignedEnveloped
                : container.SaveProfile;
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.elija_donde_guardar_el_documento");
            var outputPath = await _filePicker.PickSaveFileAsync(
                saveProfile,
                SuggestedOutputName(_protectInputPath!),
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(outputPath))
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.no_se_eligio_un_destino_no_se_ha");
                return null;
            }
            // La extensión se propone antes del diálogo. Conservar exactamente
            // el destino confirmado, incluida su autorización de sobrescritura.

            // Un certificado remoto que pide PIN u OTP los recibe aquí, para
            // esta única operación; nunca se guardan ni se reenvían solos.
            var signer = signToo ? SelectedSigningCertificate! : null;
            if (signer is not null &&
                RemoteSigningInput.NeedsSecrets(signer.Certificate))
            {
                var prompt = RemoteSecretsPrompt;
                if (prompt is null)
                {
                    ProtectValidationMessage =
                        Localizer.Text("csc.error.secreto_no_pedido");
                    return null;
                }
                remoteSecrets = await prompt(
                    signer.Certificate!,
                    operationCancellation.Token);
                if (remoteSecrets is null)
                {
                    ProtectValidationMessage = Localizer.Text("winui.comun.operacion_cancelada");
                    return null;
                }
            }

            options = new Dictionary<string, string>(
                StringComparer.Ordinal)
            {
                ["container"] = signToo
                    ? "signedandenvelopeddata"
                    : container.Value,
            };
            var parameters = new ProtectionParameters
            {
                InputPath = _protectInputPath!,
                OutputPath = outputPath,
                CertificateId = signer?.Id,
                CertificateIndex = 0,
                Profile = SelectedProfile!.Value,
                RecipientIds = encryptedData
                    ? []
                    : _selectedRecipientIds.ToArray(),
                Overwrite = "force",
                SaveToDisk = true,
                ReturnProtectedBase64 = false,
                ReturnUnprotectedBase64 = false,
                Options = options,
                SymmetricKey = encryptedData
                    ? transientSecret
                    : null,
                RemotePin = remoteSecrets?.Pin,
                RemoteOtp = remoteSecrets?.Otp,
            };
            ProtectValidationMessage = signToo
                ? Localizer.Text("winui.proteger.protegiendo_y_firmando_el_documento")
                : Localizer.Text("winui.proteger.protegiendo_el_documento");
            var result = signToo
                ? await operations.ProtectAndSignAsync(
                    parameters,
                    operationCancellation.Token)
                : await operations.ProtectAsync(
                    parameters,
                    operationCancellation.Token);
            if (!IsSuccessful(result))
            {
                ProtectValidationMessage =
                    RemoteSigningFailureMessage(result.ErrorCode) ??
                    Localizer.Text("winui.proteger.la_proteccion_no_se_completo_abra_el");
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null ||
                string.IsNullOrWhiteSpace(result.Data.OutputPath) ||
                !encryptedData && result.Data.RecipientCount <= 0)
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.el_motor_confirmo_la_proteccion_pero_no");
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_PROTECTION_OUTPUT");
            }
            if (!PathsEqual(result.Data.OutputPath, outputPath) ||
                !HasNonEmptyOutput(outputPath))
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.el_motor_no_confirmo_el_fichero");
                return InvalidResultDiagnostic(
                    result,
                    "PROTECTION_OUTPUT_NOT_FOUND");
            }

            _protectOutputPath = outputPath;
            ProtectResultMessage = encryptedData
                ? Localizer.Format("winui.proteger.proteccion_cms_encrypteddata_completada",
                    SafeFileName(outputPath))
                : result.Data.RecipientCount == 1
                    // Primero para quién y después el fichero: que el nombre
                    // del fichero no se lea como un destinatario.
                    ? Localizer.Format(signToo
                            ? "winui.proteger.proteccion_firmada_un_destinatario"
                            : "winui.proteger.proteccion_completada_un_destinatario",
                        SafeFileName(outputPath))
                    : Localizer.Format(signToo
                            ? "winui.proteger.proteccion_firmada_completada_para"
                            : "winui.proteger.proteccion_completada_para_destinatario",
                        result.Data.RecipientCount, SafeFileName(outputPath));
            ProtectValidationMessage =
                encryptedData
                    ? Localizer.Text("winui.proteger.el_documento_protegido_esta_listo")
                    : Localizer.Text("winui.proteger.el_documento_protegido_esta_listo_para");
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
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.la_proteccion_se_cancelo_antes_de");
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation?.Token ?? cancellationToken);
        }
        catch (IpcClientException exception)
        {
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.fallo_la_comunicacion_segura_durante_la");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            ProtectValidationMessage =
                Localizer.Text("winui.proteger.la_aplicacion_no_pudo_completar_la");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            options?.Clear();
            remoteSecrets?.Dispose();
            ZeroTransientSecret(transientSecret);
            ZeroTransientSecret(transientSecretConfirmation);
            if (operationCancellation is not null)
            {
                EndOperation(operationCancellation);
            }
        }
    }

    private static string? RemoteSigningFailureMessage(string? errorCode) =>
        RemoteSigningInput.MessageKey(errorCode) is { } key
            ? Localizer.Text(key)
            : null;

    public async Task<OperationDiagnostic?> UnprotectAsync(
        byte[]? transientSecret,
        CancellationToken cancellationToken = default)
    {
        CancellationTokenSource? operationCancellation = null;
        try
        {
            var validation = ValidateBeforeUnprotect(transientSecret);
            if (validation is not null)
            {
                UnprotectValidationMessage = validation;
                return null;
            }
            if (!TryBeginOperation(
                cancellationToken,
                out operationCancellation))
            {
                UnprotectValidationMessage =
                    Localizer.Text("winui.proteger.ya_hay_una_operacion_en_curso_espere_o");
                return null;
            }

            ClearUnprotectedOutput();
            if (!_session.TryGetOperations(
                DesktopOperationActions.Unprotect,
                out var operations))
            {
                UnprotectValidationMessage =
                    Localizer.Text("winui.proteger.el_motor_local_no_ofrece_la_operacion");
                return null;
            }

            UnprotectValidationMessage = IsEncryptedDataUnprotectSelected
                ? Localizer.Text("winui.proteger.desprotegiendo_cms_encrypteddata_con_la")
                : Localizer.Text("winui.proteger.desprotegiendo_con_la_clave_disponible");
            var result = await operations.UnprotectAsync(
                new ProtectionParameters
                {
                    InputPath = _unprotectInputPath!,
                    OutputPath = string.Empty,
                    Profile = string.Empty,
                    Overwrite = "rename",
                    SaveToDisk = true,
                    ReturnProtectedBase64 = false,
                    ReturnUnprotectedBase64 = false,
                    SymmetricKey = IsEncryptedDataUnprotectSelected
                        ? transientSecret
                        : null,
                },
                operationCancellation.Token);
            if (!IsSuccessful(result))
            {
                UnprotectValidationMessage =
                    Localizer.Text("winui.proteger.la_desproteccion_no_se_completo_abra_el");
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null ||
                string.IsNullOrWhiteSpace(result.Data.OutputPath))
            {
                UnprotectValidationMessage =
                    Localizer.Text("winui.proteger.el_motor_confirmo_la_desproteccion_pero");
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_UNPROTECTED_OUTPUT");
            }
            if (!HasNonEmptyOutput(result.Data.OutputPath))
            {
                UnprotectValidationMessage =
                    Localizer.Text("winui.proteger.el_fichero_desprotegido_no_esta");
                return InvalidResultDiagnostic(
                    result,
                    "UNPROTECTED_OUTPUT_NOT_FOUND");
            }

            // Como en Proteger, se pregunta dónde guardar. El motor ya lo ha
            // escrito junto al contenedor sin sobrescribir nada; si se elige
            // otro destino se mueve allí, y si no, queda donde está.
            _unprotectOutputPath = await ChooseUnprotectedDestinationAsync(
                result.Data.OutputPath,
                string.IsNullOrWhiteSpace(result.Data.DocumentName)
                    ? SafeFileName(result.Data.OutputPath)
                    : result.Data.DocumentName,
                operationCancellation.Token);
            UnprotectResultMessage =
                Localizer.Format("winui.proteger.documento_recuperado_en", _unprotectOutputPath);
            UnprotectValidationMessage =
                Localizer.Text("winui.proteger.la_desproteccion_termino_correctamente");
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
            UnprotectValidationMessage =
                Localizer.Text("winui.proteger.la_desproteccion_se_cancelo_antes_de");
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation?.Token ?? cancellationToken);
        }
        catch (IpcClientException exception)
        {
            UnprotectValidationMessage =
                Localizer.Text("winui.proteger.fallo_la_comunicacion_segura_durante_la_2");
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            UnprotectValidationMessage =
                Localizer.Text("winui.proteger.la_aplicacion_no_pudo_completar_la_2");
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            ZeroTransientSecret(transientSecret);
            if (operationCancellation is not null)
            {
                EndOperation(operationCancellation);
            }
        }
    }

    private async Task<string> ChooseUnprotectedDestinationAsync(
        string writtenPath,
        string suggestedName,
        CancellationToken cancellationToken)
    {
        string? chosen;
        try
        {
            chosen = await _filePicker.PickSaveFileAsync(
                SaveFilePickerProfile.UnprotectedDocument,
                suggestedName,
                cancellationToken);
        }
        catch (Exception exception) when (exception is not OperationCanceledException)
        {
            return writtenPath;
        }
        if (string.IsNullOrWhiteSpace(chosen) || PathsEqual(chosen, writtenPath))
        {
            return writtenPath;
        }
        try
        {
            // El diálogo ya ha pedido confirmación si el destino existía.
            File.Move(writtenPath, chosen, overwrite: true);
            return chosen;
        }
        catch (Exception exception) when (exception is IOException or UnauthorizedAccessException)
        {
            return writtenPath;
        }
    }

    public OperationDiagnostic? ValidateProtectedOutputForOpening() =>
        ValidateOutputForOpening(
            _protectOutputPath,
            isProtectedOutput: true);

    public OperationDiagnostic? ValidateUnprotectedOutputForOpening() =>
        ValidateOutputForOpening(
            _unprotectOutputPath,
            isProtectedOutput: false);

    private async Task<OperationDiagnostic?> SelectInputAsync(
        OpenFilePickerProfile pickerProfile,
        bool isProtectedInput,
        CancellationToken cancellationToken)
    {
        if (!TryBeginOperation(
            cancellationToken,
            out var operationCancellation))
        {
            return null;
        }

        try
        {
            var path = await _filePicker.PickOpenFileAsync(
                pickerProfile,
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(path))
            {
                if (isProtectedInput)
                {
                    UnprotectValidationMessage =
                        Localizer.Text("winui.proteger.no_se_selecciono_ningun_contenedor");
                }
                else
                {
                    ProtectValidationMessage =
                        Localizer.Text("winui.proteger.no_se_selecciono_ningun_documento_para");
                }
                return null;
            }

            if (isProtectedInput)
            {
                _unprotectInputPath = path;
                UnprotectInputDisplayName = SafeFileName(path);
                IsEncryptedDataUnprotectSelected =
                    IsKnownEncryptedDataPath(path);
                ClearUnprotectedOutput();
            }
            else
            {
                _protectInputPath = path;
                ProtectInputDisplayName = SafeFileName(path);
                ClearProtectedOutput();
            }
            UpdateValidationMessages();
            return null;
        }
        catch (OperationCanceledException)
            when (cancellationToken.IsCancellationRequested)
        {
            return null;
        }
        catch (OperationCanceledException)
        {
            return null;
        }
        catch (Exception exception)
        {
            if (isProtectedInput)
            {
                UnprotectValidationMessage =
                    Localizer.Text("winui.proteger.no_se_pudo_abrir_el_selector_de");
            }
            else
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.comun.no_se_pudo_abrir_el_selector_de");
            }
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            EndOperation(operationCancellation);
        }
    }

    private void RebuildContainers()
    {
        var strong = SelectedProfile?.Value == "alto";
        VisibleContainers = IsProtectAndSign
            ? Containers.Where(container =>
                container.Value == "cms").ToArray()
            : strong
                ? Containers.Where(container =>
                    container.Value == "json").ToArray()
                : Containers.ToArray();
        if (SelectedContainer is null ||
            !VisibleContainers.Any(container =>
                container.Value == SelectedContainer.Value))
        {
            SelectedContainer = VisibleContainers.FirstOrDefault();
        }
        IsEncryptedDataSelected =
            SelectedContainer?.RequiresTransientSecret == true;
    }

    private void RebuildVisibleRecipients()
    {
        var profile = SelectedProfile?.Value ?? string.Empty;
        var authEnveloped =
            SelectedContainer?.Value == "authenvelopeddata";
        VisibleRecipients =
            SelectedContainer?.RequiresTransientSecret == true
                ? []
                : _allRecipients
                    .Where(recipient =>
                        string.Equals(
                            recipient.Profile,
                            profile,
                            StringComparison.OrdinalIgnoreCase) &&
                        (!authEnveloped ||
                            recipient.AuthEnvelopedDataCompatible))
                    .ToArray();
        SetSelectedRecipients([]);
        IsEncryptedDataSelected =
            SelectedContainer?.RequiresTransientSecret == true;
        ShowsNoRecipientsHint =
            !IsEncryptedDataSelected && VisibleRecipients.Count == 0;
    }

    // Estado vacío de la lista: sin él, la lista salía en blanco sin explicar
    // qué hacer (recorrido Windows 0.0.117, M6).
    public bool ShowsNoRecipientsHint
    {
        get => _showsNoRecipientsHint;
        private set
        {
            if (SetProperty(ref _showsNoRecipientsHint, value))
                RaisePropertyChanged(nameof(ShowsRecipientsList));
        }
    }
    private bool _showsNoRecipientsHint;

    // Con el estado vacío la lista se oculta: antes quedaba una caja vacía
    // de unos 130 px encima del aviso (recorrido Windows 0.0.118, B7).
    public bool ShowsRecipientsList => !_showsNoRecipientsHint;

    private string? ValidateBeforeProtect(
        byte[]? transientSecret,
        byte[]? transientSecretConfirmation)
    {
        if (!IsOperationConnected)
        {
            return Localizer.Text("winui.proteger.conecte_el_motor_local_antes_de_proteger");
        }
        if (string.IsNullOrWhiteSpace(_protectInputPath))
        {
            return Localizer.Text("winui.proteger.seleccione_el_documento_que_desea");
        }
        if (!File.Exists(_protectInputPath))
        {
            _protectInputPath = null;
            ProtectInputDisplayName = string.Empty;
            ClearProtectedOutput();
            UpdateCommandStates();
            return Localizer.Text("winui.proteger.el_documento_seleccionado_ya_no_esta");
        }
        if (SelectedProfile is null || SelectedContainer is null)
        {
            return Localizer.Text("winui.proteger.seleccione_un_perfil_y_un_contenedor_de");
        }
        var encryptedData = SelectedContainer.RequiresTransientSecret;
        if (encryptedData &&
            !IsAes256Secret(transientSecret))
        {
            return Localizer.Text("winui.proteger.introduzca_una_clave_efimera_base64");
        }
        if (encryptedData &&
            !TransientSecretsMatch(
                transientSecret,
                transientSecretConfirmation))
        {
            return Localizer.Text("winui.proteger.la_confirmacion_no_coincide_con_la_clave");
        }
        if (!encryptedData && _selectedRecipientIds.Count == 0)
        {
            return Localizer.Text("winui.proteger.seleccione_al_menos_un_destinatario");
        }
        if (IsProtectAndSign &&
            SelectedSigningCertificate is null)
        {
            return Localizer.Text("winui.proteger.seleccione_el_certificado_que_firmara_el");
        }
        return null;
    }

    private string? ValidateBeforeUnprotect(byte[]? transientSecret)
    {
        if (!IsOperationConnected)
        {
            return Localizer.Text("winui.proteger.conecte_el_motor_local_antes_de");
        }
        if (string.IsNullOrWhiteSpace(_unprotectInputPath))
        {
            return Localizer.Text("winui.proteger.seleccione_el_contenedor_que_desea");
        }
        if (!File.Exists(_unprotectInputPath))
        {
            _unprotectInputPath = null;
            IsEncryptedDataUnprotectSelected = false;
            UnprotectInputDisplayName =
                Localizer.Text("winui.proteger.ningun_contenedor_seleccionado");
            ClearUnprotectedOutput();
            UpdateCommandStates();
            return Localizer.Text("winui.proteger.el_contenedor_seleccionado_ya_no_esta");
        }
        if (IsEncryptedDataUnprotectSelected &&
            !IsAes256Secret(transientSecret))
        {
            return Localizer.Text("winui.proteger.este_contenedor_encrypteddata_requiere");
        }
        return null;
    }

    private void UpdateValidationMessages()
    {
        ProtectValidationMessage =
            !IsOperationConnected
                ? Localizer.Text("winui.proteger.conecte_el_motor_local_para_habilitar_la")
                : string.IsNullOrWhiteSpace(_protectInputPath)
                    ? Localizer.Text("winui.proteger.seleccione_el_documento_que_desea")
                    : SelectedContainer?.RequiresTransientSecret == true
                        ? Localizer.Text("winui.proteger.introduzca_y_confirme_la_clave_efimera")
                        : _selectedRecipientIds.Count == 0
                            ? VisibleRecipients.Count == 0
                                ? SelectedProfile?.Value == "compat"
                                    ? CompatRecipientGuidance
                                    : Localizer.Text("winui.proteger.no_hay_destinatarios_compatibles_con_el")
                                : Localizer.Text("winui.proteger.seleccione_al_menos_un_destinatario")
                            : IsProtectAndSign &&
                              SelectedSigningCertificate is null
                                ? Localizer.Text("winui.proteger.seleccione_el_certificado_de_firma")
                                : Localizer.Text("winui.proteger.documento_y_destinatarios_preparados");
        UnprotectValidationMessage =
            !IsOperationConnected
                ? Localizer.Text("winui.proteger.conecte_el_motor_local_para_habilitar_la_2")
                : string.IsNullOrWhiteSpace(_unprotectInputPath)
                    ? Localizer.Text("winui.proteger.seleccione_el_contenedor_que_desea")
                    : IsEncryptedDataUnprotectSelected
                        ? Localizer.Text("winui.proteger.introduzca_la_clave_efimera_base64_usada")
                        : Localizer.Text("winui.proteger.contenedor_preparado_para_desproteger");
    }

    private void UpdateCommandStates()
    {
        CanSelectFiles = IsOperationConnected && !IsBusy;
        CanRemoveRecipient = CanSelectFiles && _selectedRecipientIds.Count == 1 &&
            VisibleRecipients.Any(item => item.Id == _selectedRecipientIds[0] && item.Origin == "importado" && item.Profile == "compat");
        CanUseProtectAndSign =
            IsOperationConnected &&
            !IsBusy &&
            _supportsProtectAndSign &&
            SigningCertificates.Count > 0;
        CanSelectSigningCertificate =
            CanUseProtectAndSign &&
            IsProtectAndSign;
        CanProtect =
            IsOperationConnected &&
            !IsBusy &&
            !string.IsNullOrWhiteSpace(_protectInputPath) &&
            SelectedProfile is not null &&
            SelectedContainer is not null &&
            (SelectedContainer.RequiresTransientSecret ||
                _selectedRecipientIds.Count > 0) &&
            (!IsProtectAndSign ||
                SelectedSigningCertificate is not null);
        CanUnprotect =
            IsOperationConnected &&
            !IsBusy &&
            !string.IsNullOrWhiteSpace(_unprotectInputPath);
        CanCancel = IsBusy;
        CanOpenProtectedOutput =
            !IsBusy &&
            !string.IsNullOrWhiteSpace(_protectOutputPath);
        CanOpenUnprotectedOutput =
            !IsBusy &&
            !string.IsNullOrWhiteSpace(_unprotectOutputPath);
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
        operationCancellation.Dispose();
        Interlocked.Exchange(ref _operationInProgress, 0);
        SetBusy(false);
    }

    private void SetBusy(bool value)
    {
        IsBusy = value;
        UpdateCommandStates();
    }

    private void ClearProtectedOutput()
    {
        _protectOutputPath = null;
        ProtectResultMessage =
            Localizer.Text("winui.proteger.no_se_ha_ejecutado_ninguna_proteccion_2");
        UpdateCommandStates();
    }

    private void ClearUnprotectedOutput()
    {
        _unprotectOutputPath = null;
        UnprotectResultMessage =
            Localizer.Text("winui.proteger.no_se_ha_ejecutado_ninguna_desproteccion_2");
        UpdateCommandStates();
    }

    private OperationDiagnostic? ValidateOutputForOpening(
        string? path,
        bool isProtectedOutput)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            if (isProtectedOutput)
            {
                ProtectValidationMessage =
                    Localizer.Text("winui.proteger.todavia_no_hay_un_documento_protegido");
            }
            else
            {
                UnprotectValidationMessage =
                    Localizer.Text("winui.proteger.todavia_no_hay_un_documento_recuperado");
            }
            return null;
        }
        if (HasNonEmptyOutput(path))
        {
            return null;
        }

        if (isProtectedOutput)
        {
            ClearProtectedOutput();
        }
        else
        {
            ClearUnprotectedOutput();
        }
        return OperationDiagnosticMapper.FromResult(
            new IpcCallResult<object>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = "local-output-check",
                TraceId = "local-output-check",
                Action = isProtectedOutput
                    ? DesktopOperationActions.Protect
                    : DesktopOperationActions.Unprotect,
                IsSuccess = false,
                Outcome = "failure",
                ErrorCode = "PROTECTION_OUTPUT_NOT_FOUND",
                Phase = "operation",
                Retryable = false,
                Data = null,
                Diagnostic = null,
            });
    }

    private static bool IsSuccessful<TData>(
        IpcCallResult<TData> result) =>
        result.IsSuccess &&
        string.Equals(
            result.Outcome,
            "success",
            StringComparison.Ordinal);

    private static string OriginLabel(string origin) => origin switch
    {
        "importado" => Localizer.Text("winui.proteger.importado"),
        "otras_personas" => Localizer.Text("winui.proteger.otras_personas_windows"),
        _ => Localizer.Text("winui.proteger.propio"),
    };

    private static bool IsKnownEncryptedDataPath(string? path) =>
        !string.IsNullOrWhiteSpace(path) &&
        path.EndsWith(
            ".encrypted.p7m",
            StringComparison.OrdinalIgnoreCase);

    private static bool IsAes256Secret(byte[]? secret)
    {
        return secret is { Length: Aes256SecretBytes };
    }

    private static bool TransientSecretsMatch(
        byte[]? secret,
        byte[]? confirmation)
    {
        if (secret is null ||
            confirmation is null ||
            secret.Length != confirmation.Length)
        {
            return false;
        }
        return CryptographicOperations.FixedTimeEquals(
            secret,
            confirmation);
    }

    private static void ZeroTransientSecret(byte[]? secret)
    {
        if (secret is null || secret.Length == 0)
        {
            return;
        }
        CryptographicOperations.ZeroMemory(secret);
    }

    private static string SuggestedOutputName(string inputPath)
    {
        var baseName = Path.GetFileNameWithoutExtension(inputPath);
        return string.IsNullOrWhiteSpace(baseName)
            ? Localizer.Text("winui.proteger.documento_protegido")
            : baseName + Localizer.Text("winui.proteger.protegido");
    }

    private static string SafeFileName(string path)
    {
        try
        {
            var name = Path.GetFileName(path);
            return string.IsNullOrWhiteSpace(name)
                ? Localizer.Text("winui.proteger.documento")
                : name;
        }
        catch
        {
            return Localizer.Text("winui.proteger.documento");
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
