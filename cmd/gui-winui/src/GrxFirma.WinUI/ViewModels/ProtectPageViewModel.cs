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
}

public sealed record ProtectionContainerOption(
    string SourceLabel,
    string Value,
    SaveFilePickerProfile SaveProfile,
    bool RequiresTransientSecret = false)
{
    public string Label => Localizer.Text(SourceLabel);
}

public sealed record ProtectionRecipientItem
{
    public required string Id { get; init; }
    public required string Label { get; init; }
    public required string Detail { get; init; }
    public required string Profile { get; init; }
    public required string Origin { get; init; }
    public required bool AuthEnvelopedDataCompatible { get; init; }
}

public sealed record ProtectionSignerItem
{
    public required string Id { get; init; }
    public required string Label { get; init; }
}

public sealed class ProtectPageViewModel
    : WorkspacePageViewModel
{
    private const int MaximumCatalogItems = 128;
    private const int Aes256SecretBytes = 32;
    private const string CompatRecipientGuidance =
        "El perfil compatible requiere una identidad RSA con clave privada descifrable, por ejemplo un P12/PFX autorizado. " +
        "Los certificados opacos del almacén de Windows siguen disponibles para firmar, pero no se ofrecen para cifrado; " +
        "cargue un P12/PFX apto o cambie al perfil alto.";

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
    private string _protectInputDisplayName = Localizer.Text("Ningún documento seleccionado");
    private string _unprotectInputDisplayName = Localizer.Text("Ningún contenedor seleccionado");
    private string _protectValidationMessage =
        "Seleccione un documento y al menos un destinatario.";
    private string _unprotectValidationMessage =
        "Seleccione un contenedor protegido.";
    private string _protectResultMessage =
        "No se ha ejecutado ninguna protección.";
    private string _unprotectResultMessage =
        "No se ha ejecutado ninguna desprotección.";
    private int _selectedRecipientCount;
    private string _selectedRecipientSummary =
        "No hay destinatarios seleccionados.";
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
            "Proteger",
            "Protege documentos para destinatarios concretos o con una clave efímera, y recupera ambos tipos de contenedor.",
            "La protección no está disponible porque el motor local no ha publicado protection_recipients, protect y unprotect.")
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
        new("Compatible RSA-OAEP", "compat"),
        new("Alta seguridad ML-KEM + X25519", "alto"),
    ];

    public IReadOnlyList<ProtectionContainerOption> Containers { get; } =
    [
        new(
            "JSON GrxFirma (.afp)",
            "json",
            SaveFilePickerProfile.ProtectedJson),
        new(
            "CMS EnvelopedData (.enveloped)",
            "cms",
            SaveFilePickerProfile.CmsEnveloped),
        new(
            "CMS EncryptedData — requiere clave efímera segura",
            "cms-encrypted",
            SaveFilePickerProfile.CmsEncrypted,
            RequiresTransientSecret: true),
        new(
            "CMS AuthEnvelopedData (.authenveloped.p7m)",
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
        get => _protectInputDisplayName;
        private set => SetProperty(ref _protectInputDisplayName, value);
    }

    public string UnprotectInputDisplayName
    {
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
            "Motor local conectado. Protección y desprotección están disponibles.");
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
                ? "No hay destinatarios seleccionados."
                : SelectedRecipientCount == 1
                    ? "1 destinatario seleccionado."
                    : $"{SelectedRecipientCount} destinatarios seleccionados.";
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
            ProtectValidationMessage = "El motor local no permite añadir destinatarios públicos.";
            return null;
        }
        var result = await operations.ImportProtectionRecipientAsync(new() { Path = path }, cancellationToken);
        if (!IsSuccessful(result))
        {
            ProtectValidationMessage = result.SafeUserMessage;
            return OperationDiagnosticMapper.FromResult(result);
        }
        ProtectValidationMessage = "Certificado público añadido. Seleccione el destinatario para proteger.";
        return await RefreshCatalogsAsync(cancellationToken);
    }

    public async Task<OperationDiagnostic?> RemovePublicRecipientAsync(CancellationToken cancellationToken)
    {
        if (!CanRemoveRecipient) return null;
        var id = _selectedRecipientIds[0];
        if (!_session.TryGetOperations(DesktopOperationActions.ProtectionRecipientRemove, out var operations))
        {
            ProtectValidationMessage = "El motor local no permite quitar destinatarios públicos.";
            return null;
        }
        var result = await operations.RemoveProtectionRecipientAsync(new() { Id = id }, cancellationToken);
        if (!IsSuccessful(result))
        {
            ProtectValidationMessage = result.SafeUserMessage;
            return OperationDiagnosticMapper.FromResult(result);
        }
        ProtectValidationMessage = "Destinatario importado quitado.";
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
                "Espere a que termine la operación actual antes de actualizar destinatarios.";
            return null;
        }

        try
        {
            if (!_session.TryGetOperations(
                DesktopOperationActions.ProtectionRecipients,
                out var operations))
            {
                ProtectValidationMessage =
                    "El motor local no ofrece el catálogo de destinatarios.";
                return null;
            }

            ProtectValidationMessage = "Consultando destinatarios de protección…";
            var recipientsResult =
                await operations.GetProtectionRecipientsAsync(
                    operationCancellation.Token);
            if (!IsSuccessful(recipientsResult))
            {
                ProtectValidationMessage =
                    "El motor local no pudo cargar los destinatarios.";
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
                        Detail = Localizer.Format("Origen: {0} · Perfil: {1}",
                            Localizer.Text(OriginLabel(recipient.Origin)),
                            Localizer.Text(recipient.Profile)) +
                            (string.IsNullOrWhiteSpace(recipient.Algorithm) ? "" :
                                Localizer.Format(" · Algoritmo: {0}", recipient.Algorithm)),
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
                        "No se pudieron cargar los certificados para proteger y firmar.";
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
                                ? "Certificado sin titular"
                                : label,
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
                "La actualización de destinatarios se canceló.";
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation.Token);
        }
        catch (IpcClientException exception)
        {
            ProtectValidationMessage =
                "Falló la comunicación segura al consultar destinatarios.";
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            ProtectValidationMessage =
                "La aplicación no pudo cargar los destinatarios.";
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

    public async Task<OperationDiagnostic?> ProtectAsync(
        byte[]? transientSecret,
        byte[]? transientSecretConfirmation,
        CancellationToken cancellationToken = default)
    {
        CancellationTokenSource? operationCancellation = null;
        Dictionary<string, string>? options = null;
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
                    "Ya hay una operación en curso. Espere o cancélela.";
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
                    "El motor local no ofrece la operación de protección seleccionada.";
                return null;
            }

            var container = SelectedContainer!;
            var encryptedData = container.RequiresTransientSecret;
            var saveProfile = signToo
                ? SaveFilePickerProfile.CmsSignedEnveloped
                : container.SaveProfile;
            ProtectValidationMessage =
                "Elija dónde guardar el documento protegido. La operación aún no ha comenzado.";
            var outputPath = await _filePicker.PickSaveFileAsync(
                saveProfile,
                SuggestedOutputName(_protectInputPath!),
                operationCancellation.Token);
            if (string.IsNullOrWhiteSpace(outputPath))
            {
                ProtectValidationMessage =
                    "No se eligió un destino. No se ha protegido ningún documento.";
                return null;
            }
            // La extensión se propone antes del diálogo. Conservar exactamente
            // el destino confirmado, incluida su autorización de sobrescritura.

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
                CertificateId = signToo
                    ? SelectedSigningCertificate!.Id
                    : null,
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
            };
            ProtectValidationMessage = signToo
                ? "Protegiendo y firmando el documento…"
                : "Protegiendo el documento…";
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
                    "La protección no se completó. Abra el diagnóstico para conocer el fallo.";
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null ||
                string.IsNullOrWhiteSpace(result.Data.OutputPath) ||
                !encryptedData && result.Data.RecipientCount <= 0)
            {
                ProtectValidationMessage =
                    "El motor confirmó la protección, pero no devolvió un resultado completo.";
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_PROTECTION_OUTPUT");
            }
            if (!PathsEqual(result.Data.OutputPath, outputPath) ||
                !HasNonEmptyOutput(outputPath))
            {
                ProtectValidationMessage =
                    "El motor no confirmó el fichero protegido solicitado. No se mostrará un éxito falso.";
                return InvalidResultDiagnostic(
                    result,
                    "PROTECTION_OUTPUT_NOT_FOUND");
            }

            _protectOutputPath = outputPath;
            ProtectResultMessage = encryptedData
                ? Localizer.Format("Protección CMS EncryptedData completada: {0}.",
                    SafeFileName(outputPath))
                : Localizer.Format(signToo
                        ? "Protección firmada completada para {0} destinatario(s): {1}."
                        : "Protección completada para {0} destinatario(s): {1}.",
                    result.Data.RecipientCount, SafeFileName(outputPath));
            ProtectValidationMessage =
                encryptedData
                    ? "El documento protegido está listo. Conserve la clave efímera fuera de la aplicación para poder recuperarlo."
                    : "El documento protegido está listo para abrir.";
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
                "La protección se canceló antes de completarse.";
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation?.Token ?? cancellationToken);
        }
        catch (IpcClientException exception)
        {
            ProtectValidationMessage =
                "Falló la comunicación segura durante la protección.";
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            ProtectValidationMessage =
                "La aplicación no pudo completar la protección.";
            return OperationDiagnosticMapper.FromException(exception);
        }
        finally
        {
            options?.Clear();
            ZeroTransientSecret(transientSecret);
            ZeroTransientSecret(transientSecretConfirmation);
            if (operationCancellation is not null)
            {
                EndOperation(operationCancellation);
            }
        }
    }

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
                    "Ya hay una operación en curso. Espere o cancélela.";
                return null;
            }

            ClearUnprotectedOutput();
            if (!_session.TryGetOperations(
                DesktopOperationActions.Unprotect,
                out var operations))
            {
                UnprotectValidationMessage =
                    "El motor local no ofrece la operación unprotect.";
                return null;
            }

            UnprotectValidationMessage = IsEncryptedDataUnprotectSelected
                ? "Desprotegiendo CMS EncryptedData con la clave efímera…"
                : "Desprotegiendo con la clave disponible en el almacén…";
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
                    "La desprotección no se completó. Abra el diagnóstico para conocer el fallo.";
                return OperationDiagnosticMapper.FromResult(result);
            }
            if (result.Data is null ||
                string.IsNullOrWhiteSpace(result.Data.OutputPath))
            {
                UnprotectValidationMessage =
                    "El motor confirmó la desprotección, pero no devolvió una salida válida.";
                return InvalidResultDiagnostic(
                    result,
                    "MISSING_UNPROTECTED_OUTPUT");
            }
            if (!HasNonEmptyOutput(result.Data.OutputPath))
            {
                UnprotectValidationMessage =
                    "El fichero desprotegido no está disponible. No se mostrará un éxito falso.";
                return InvalidResultDiagnostic(
                    result,
                    "UNPROTECTED_OUTPUT_NOT_FOUND");
            }

            _unprotectOutputPath = result.Data.OutputPath;
            UnprotectResultMessage =
                $"Documento recuperado como {SafeFileName(result.Data.OutputPath)}.";
            UnprotectValidationMessage =
                "La desprotección terminó correctamente. Puede abrir el resultado.";
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
                "La desprotección se canceló antes de completarse.";
            return OperationDiagnosticMapper.FromException(
                exception,
                operationCancellation?.Token ?? cancellationToken);
        }
        catch (IpcClientException exception)
        {
            UnprotectValidationMessage =
                "Falló la comunicación segura durante la desprotección.";
            return OperationDiagnosticMapper.FromException(exception);
        }
        catch (Exception exception)
        {
            UnprotectValidationMessage =
                "La aplicación no pudo completar la desprotección.";
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
                        "No se seleccionó ningún contenedor protegido.";
                }
                else
                {
                    ProtectValidationMessage =
                        "No se seleccionó ningún documento para proteger.";
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
                    "No se pudo abrir el selector de contenedores.";
            }
            else
            {
                ProtectValidationMessage =
                    "No se pudo abrir el selector de documentos.";
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
    }

    private string? ValidateBeforeProtect(
        byte[]? transientSecret,
        byte[]? transientSecretConfirmation)
    {
        if (!IsOperationConnected)
        {
            return "Conecte el motor local antes de proteger un documento.";
        }
        if (string.IsNullOrWhiteSpace(_protectInputPath))
        {
            return "Seleccione el documento que desea proteger.";
        }
        if (!File.Exists(_protectInputPath))
        {
            _protectInputPath = null;
            ProtectInputDisplayName = Localizer.Text("Ningún documento seleccionado");
            ClearProtectedOutput();
            UpdateCommandStates();
            return "El documento seleccionado ya no está disponible.";
        }
        if (SelectedProfile is null || SelectedContainer is null)
        {
            return "Seleccione un perfil y un contenedor de protección.";
        }
        var encryptedData = SelectedContainer.RequiresTransientSecret;
        if (encryptedData &&
            !IsAes256Secret(transientSecret))
        {
            return "Introduzca una clave efímera Base64 canónica de 32 bytes (44 caracteres).";
        }
        if (encryptedData &&
            !TransientSecretsMatch(
                transientSecret,
                transientSecretConfirmation))
        {
            return "La confirmación no coincide con la clave efímera.";
        }
        if (!encryptedData && _selectedRecipientIds.Count == 0)
        {
            return "Seleccione al menos un destinatario compatible.";
        }
        if (IsProtectAndSign &&
            SelectedSigningCertificate is null)
        {
            return "Seleccione el certificado que firmará el contenedor protegido.";
        }
        return null;
    }

    private string? ValidateBeforeUnprotect(byte[]? transientSecret)
    {
        if (!IsOperationConnected)
        {
            return "Conecte el motor local antes de desproteger.";
        }
        if (string.IsNullOrWhiteSpace(_unprotectInputPath))
        {
            return "Seleccione el contenedor que desea desproteger.";
        }
        if (!File.Exists(_unprotectInputPath))
        {
            _unprotectInputPath = null;
            IsEncryptedDataUnprotectSelected = false;
            UnprotectInputDisplayName =
                Localizer.Text("Ningún contenedor seleccionado");
            ClearUnprotectedOutput();
            UpdateCommandStates();
            return "El contenedor seleccionado ya no está disponible.";
        }
        if (IsEncryptedDataUnprotectSelected &&
            !IsAes256Secret(transientSecret))
        {
            return "Este contenedor EncryptedData requiere una clave efímera Base64 canónica de 32 bytes (44 caracteres).";
        }
        return null;
    }

    private void UpdateValidationMessages()
    {
        ProtectValidationMessage =
            !IsOperationConnected
                ? "Conecte el motor local para habilitar la protección."
                : string.IsNullOrWhiteSpace(_protectInputPath)
                    ? "Seleccione el documento que desea proteger."
                    : SelectedContainer?.RequiresTransientSecret == true
                        ? "Introduzca y confirme la clave efímera Base64. No se guardará en la aplicación."
                        : _selectedRecipientIds.Count == 0
                            ? VisibleRecipients.Count == 0
                                ? SelectedProfile?.Value == "compat"
                                    ? CompatRecipientGuidance
                                    : "No hay destinatarios compatibles con el perfil y contenedor elegidos."
                                : "Seleccione al menos un destinatario compatible."
                            : IsProtectAndSign &&
                              SelectedSigningCertificate is null
                                ? "Seleccione el certificado de firma."
                                : "Documento y destinatarios preparados para proteger.";
        UnprotectValidationMessage =
            !IsOperationConnected
                ? "Conecte el motor local para habilitar la desprotección."
                : string.IsNullOrWhiteSpace(_unprotectInputPath)
                    ? "Seleccione el contenedor que desea desproteger."
                    : IsEncryptedDataUnprotectSelected
                        ? "Introduzca la clave efímera Base64 usada al crear este EncryptedData."
                        : "Contenedor preparado para desproteger con el almacén local.";
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
            "No se ha ejecutado ninguna protección con esta selección.";
        UpdateCommandStates();
    }

    private void ClearUnprotectedOutput()
    {
        _unprotectOutputPath = null;
        UnprotectResultMessage =
            "No se ha ejecutado ninguna desprotección con esta selección.";
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
                    "Todavía no hay un documento protegido que abrir.";
            }
            else
            {
                UnprotectValidationMessage =
                    "Todavía no hay un documento recuperado que abrir.";
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
        "importado" => "Importado",
        "otras_personas" => "Otras personas (Windows)",
        _ => "Propio",
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
            ? Localizer.Text("documento-protegido")
            : baseName + Localizer.Text("-protegido");
    }

    private static string SafeFileName(string path)
    {
        try
        {
            var name = Path.GetFileName(path);
            return string.IsNullOrWhiteSpace(name)
                ? Localizer.Text("documento")
                : name;
        }
        catch
        {
            return Localizer.Text("documento");
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
