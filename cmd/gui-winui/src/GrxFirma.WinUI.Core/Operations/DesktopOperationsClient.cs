// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Facturae;
using System.Buffers;
using System.Globalization;
using System.Security.Cryptography;
using System.Text;

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Operations;

public static class DesktopOperationActions
{
    public const string Certificates = "certificates";
    public const string CertificateExportPublic = "certificate_export_public";
    public const string SmartcardStatus = "smartcard_status";
    public const string FacturaeCreate = "facturae_create";
    public const string ValidateInvoice = "validate_invoice";
    public const string ValidateVeriFactu = "validate_verifactu";
    public const string ReadVeriFactuQr = "read_verifactu_qr";
    public const string QueryVeriFactuQr = "query_verifactu_qr";
    public const string DetectVeriFactu = "detect_verifactu";
    public const string GenerateEniDocument = "generate_eni_document";
    public const string GenerateEniFile = "generate_eni_file";
    public const string ValidateCertificateOnline =
        "validate_certificate_online";
    public const string OpenCertificateManager =
        "open_certificate_manager";
    public const string Sign = "sign";
    public const string SignMultiCosign = "sign_multicosign";
    public const string SignBatch = "sign_batch";
    public const string Verify = "verify";
    public const string PdfPreview = "pdf_preview";
    public const string SealPreview = "seal_preview";
    public const string HashCreate = "hash_create";
    public const string HashCheck = "hash_check";
    public const string GetSettings = "get_settings";
    public const string SaveSettings = "save_settings";
    public const string ProtectionRecipients = "protection_recipients";
    public const string ProtectionRecipientImport = "protection_recipient_import";
    public const string ProtectionRecipientRemove = "protection_recipient_remove";
    public const string Protect = "protect";
    public const string ProtectAndSign = "protect_sign";
    public const string Unprotect = "unprotect";
    public const string Ping = "ping";
    public const string CheckCertificates = "check_certificates";
    public const string CheckUpdates = "check_updates";
    public const string CertificateAccessOptions =
        "certificate_access_options";
    public const string ImportCertificateToStore =
        "import_certificate_to_store";
    public const string UseTemporaryCertificate =
        "use_temporary_certificate";
    public const string RemoveTemporaryCertificate =
        "remove_temporary_certificate";
    public const string ClearTemporaryCertificates =
        "clear_temporary_certificates";
    public const string ProxySecretStoreStatus =
        "proxy_secret_store_status";
    public const string ProxySecretStore =
        "proxy_secret_store";
    public const string ProxySecretDelete =
        "proxy_secret_delete";
    public const string TlsDiagnostics =
        "tls_diagnostics";
    public const string ClockDiagnostics =
        "clock_diagnostics";
    public const string ExportDiagnostic =
        "export_diagnostic";
    public const string InstallPublicRoots =
        "install_public_roots";
    public const string ClearTlsTrust =
        "clear_tls_trust";
    public const string CscStatus = "csc_status";
    public const string CscConfigure = "csc_configure";
    public const string CscConnect = "csc_connect";
    public const string CscDisconnect = "csc_disconnect";
    public const string CscSendOtp = "csc_send_otp";
}

/// <summary>
/// Fachada tipada de las operaciones reales expuestas por el motor de escritorio.
/// El transporte conserva la correlación, los diagnósticos y los errores estables
/// proporcionados por <see cref="IIpcClient"/>.
/// </summary>
public sealed class DesktopOperationsClient
{
    public const int MaximumCredentialBytes = 2 * 1024 * 1024;
    public const int MaximumPasswordBytes = 4 * 1024;
    public const int MaximumProxyCredentialTextBytes = 256;

    private readonly IIpcClient _ipcClient;

    public DesktopOperationsClient(IIpcClient ipcClient)
    {
        ArgumentNullException.ThrowIfNull(ipcClient);
        _ipcClient = ipcClient;
    }

    public Task<IpcCallResult<IReadOnlyList<CertificateInfo>>>
        GetCertificatesAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            CertificatesParameters,
            IReadOnlyList<CertificateInfo>>(
                DesktopOperationActions.Certificates,
                new CertificatesParameters(),
                cancellationToken);

    public Task<IpcCallResult<CertificateExportPublicResult>> ExportPublicCertificateAsync(
        CertificateExportPublicParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<CertificateExportPublicParameters, CertificateExportPublicResult>(
            DesktopOperationActions.CertificateExportPublic, parameters, cancellationToken);
    }

    public Task<IpcCallResult<SmartcardStatusResult>>
        GetSmartcardStatusAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            SmartcardStatusParameters,
            SmartcardStatusResult>(
                DesktopOperationActions.SmartcardStatus,
                new SmartcardStatusParameters(),
                cancellationToken);

    public Task<IpcCallResult<InvoiceValidationResult>> ValidateVeriFactuAsync(string inputPath, CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<object, InvoiceValidationResult>(DesktopOperationActions.ValidateVeriFactu,
            new { inputPath }, cancellationToken);

    public Task<IpcCallResult<VeriFactuDetectionResult>> DetectVeriFactuAsync(string inputPath, CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<object, VeriFactuDetectionResult>(DesktopOperationActions.DetectVeriFactu,
            new { inputPath }, cancellationToken);

    public Task<IpcCallResult<VeriFactuQrResult>> ReadVeriFactuQrAsync(string url, CancellationToken cancellationToken = default)
    {
        VeriFactuQrInput.EnsureAllowedAuthority(url);
        return _ipcClient.SendAsync<object, VeriFactuQrResult>(DesktopOperationActions.ReadVeriFactuQr,
            new { url }, cancellationToken);
    }

    public Task<IpcCallResult<VeriFactuQrQueryResult>> QueryVeriFactuQrAsync(string url, CancellationToken cancellationToken = default)
    {
        VeriFactuQrInput.EnsureAllowedAuthority(url);
        return _ipcClient.SendAsync<object, VeriFactuQrQueryResult>(DesktopOperationActions.QueryVeriFactuQr,
            new { url }, cancellationToken);
    }

    public Task<IpcCallResult<InvoiceValidationResult>> ValidateInvoiceAsync(string inputPath, CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<object, InvoiceValidationResult>(DesktopOperationActions.ValidateInvoice,
            new { inputPath }, cancellationToken);

    public Task<IpcCallResult<EniGenerationResult>> GenerateEniDocumentAsync(
        string inputPath, string? originalPath, string outputPath,
        IReadOnlyDictionary<string, string> options, CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<object, EniGenerationResult>(DesktopOperationActions.GenerateEniDocument,
            new { inputPath, originalPath, outputPath, options }, cancellationToken);

    public Task<IpcCallResult<EniGenerationResult>> GenerateEniFileAsync(
        string directoryPath, string outputPath, string certificateId,
        IReadOnlyDictionary<string, string> options, CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<object, EniGenerationResult>(DesktopOperationActions.GenerateEniFile,
            new { directoryPath, outputPath, certificateId, options }, cancellationToken);

    public Task<IpcCallResult<CertificateOnlineValidationResult>>
        ValidateCertificateOnlineAsync(
            ValidateCertificateOnlineParameters parameters,
            CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<
            ValidateCertificateOnlineParameters,
            CertificateOnlineValidationResult>(
                DesktopOperationActions.ValidateCertificateOnline,
                parameters,
                cancellationToken);
    }

    /// <summary>
    /// Si la firma lleva el PIN o el OTP de un certificado remoto, sus buffers
    /// quedan sobrescritos al terminar, con éxito o con error.
    /// </summary>
    public Task<IpcCallResult<SignResult>> SignAsync(
        SignParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return SendWithRemoteSecretsAsync<SignParameters, SignResult>(
            DesktopOperationActions.Sign,
            parameters,
            parameters.RemotePin,
            parameters.RemoteOtp,
            cancellationToken);
    }

    public Task<IpcCallResult<SignResult>> SignMultiCosignAsync(
        SignParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return SendWithRemoteSecretsAsync<SignParameters, SignResult>(
            DesktopOperationActions.SignMultiCosign,
            parameters,
            parameters.RemotePin,
            parameters.RemoteOtp,
            cancellationToken);
    }

    private async Task<IpcCallResult<TData>> SendWithRemoteSecretsAsync<TParameters, TData>(
        string action,
        TParameters parameters,
        byte[]? remotePin,
        byte[]? remoteOtp,
        CancellationToken cancellationToken)
    {
        try
        {
            RemoteSigningInput.ValidateSecret(remotePin, nameof(remotePin));
            RemoteSigningInput.ValidateSecret(remoteOtp, nameof(remoteOtp));
            return await _ipcClient.SendAsync<TParameters, TData>(
                action,
                parameters,
                cancellationToken).ConfigureAwait(false);
        }
        finally
        {
            if (remotePin is not null) CryptographicOperations.ZeroMemory(remotePin);
            if (remoteOtp is not null) CryptographicOperations.ZeroMemory(remoteOtp);
        }
    }

    public Task<IpcCallResult<RemoteSigningStatus>> GetRemoteSigningStatusAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<RemoteSigningStatusParameters, RemoteSigningStatus>(
            DesktopOperationActions.CscStatus,
            new RemoteSigningStatusParameters(),
            cancellationToken);

    public Task<IpcCallResult<RemoteSigningDiscovery>> ConfigureRemoteSigningAsync(
        string serviceUrl,
        string clientId,
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<RemoteSigningConfigureParameters, RemoteSigningDiscovery>(
            DesktopOperationActions.CscConfigure,
            RemoteSigningInput.Normalize(serviceUrl, clientId),
            cancellationToken);

    public Task<IpcCallResult<RemoteSigningConnection>> ConnectRemoteSigningAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<RemoteSigningConnectParameters, RemoteSigningConnection>(
            DesktopOperationActions.CscConnect,
            new RemoteSigningConnectParameters(),
            cancellationToken);

    public Task<IpcCallResult<RemoteSigningAck>> DisconnectRemoteSigningAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<RemoteSigningDisconnectParameters, RemoteSigningAck>(
            DesktopOperationActions.CscDisconnect,
            new RemoteSigningDisconnectParameters(),
            cancellationToken);

    public Task<IpcCallResult<RemoteSigningAck>> SendRemoteSigningOtpAsync(
        string certificateId,
        CancellationToken cancellationToken = default)
    {
        if (!RemoteSigningInput.IsValidCertificateId(certificateId))
        {
            throw new ArgumentException("csc.error.parametro_invalido", nameof(certificateId));
        }
        return _ipcClient.SendAsync<RemoteSigningSendOtpParameters, RemoteSigningAck>(
            DesktopOperationActions.CscSendOtp,
            new RemoteSigningSendOtpParameters { CertificateId = certificateId },
            cancellationToken);
    }

    public Task<IpcCallResult<VerifyResult>> VerifyAsync(
        VerifyParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<VerifyParameters, VerifyResult>(
            DesktopOperationActions.Verify,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<PdfPreviewResult>> GetPdfPreviewAsync(
        PdfPreviewParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<PdfPreviewParameters, PdfPreviewResult>(
            DesktopOperationActions.PdfPreview,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<SealPreviewResult>> GetSealPreviewAsync(
        SealPreviewParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<SealPreviewParameters, SealPreviewResult>(
            DesktopOperationActions.SealPreview,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<HashCreateResult>> CreateHashAsync(
        HashCreateParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<HashCreateParameters, HashCreateResult>(
            DesktopOperationActions.HashCreate,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<HashCheckResult>> CheckHashAsync(
        HashCheckParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<HashCheckParameters, HashCheckResult>(
            DesktopOperationActions.HashCheck,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<ProtectionRecipientsResult>>
        GetProtectionRecipientsAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            ProtectionRecipientsParameters,
            ProtectionRecipientsResult>(
                DesktopOperationActions.ProtectionRecipients,
                new ProtectionRecipientsParameters(),
                cancellationToken);

    public Task<IpcCallResult<ProtectionRecipientChangeResult>> ImportProtectionRecipientAsync(
        ProtectionRecipientImportParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<ProtectionRecipientImportParameters, ProtectionRecipientChangeResult>(
            DesktopOperationActions.ProtectionRecipientImport, parameters, cancellationToken);
    }

    public Task<IpcCallResult<ProtectionRecipientChangeResult>> RemoveProtectionRecipientAsync(
        ProtectionRecipientRemoveParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<ProtectionRecipientRemoveParameters, ProtectionRecipientChangeResult>(
            DesktopOperationActions.ProtectionRecipientRemove, parameters, cancellationToken);
    }

    public Task<IpcCallResult<ProtectionResult>> ProtectAsync(
        ProtectionParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<ProtectionParameters, ProtectionResult>(
            DesktopOperationActions.Protect,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<ProtectionResult>> ProtectAndSignAsync(
        ProtectionParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<ProtectionParameters, ProtectionResult>(
            DesktopOperationActions.ProtectAndSign,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<UnprotectResult>> UnprotectAsync(
        ProtectionParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<ProtectionParameters, UnprotectResult>(
            DesktopOperationActions.Unprotect,
            parameters,
            cancellationToken);
    }

    public Task<IpcCallResult<FacturaeCreateResult>> CreateFacturaeAsync(
        FacturaeInvoiceDraft draft,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(draft);
        object Party(FacturaePartyDraft party) => new
        {
            personTypeCode = party.PersonTypeCode,
            taxIdentificationNumber = party.TaxIdentificationNumber,
            name = party.Name,
            firstSurname = party.FirstSurname,
            secondSurname = party.SecondSurname,
            address = new
            {
                address = party.Address.Address,
                postCode = party.Address.PostCode,
                town = party.Address.Town,
                province = party.Address.Province,
            },
            electronicMail = party.ElectronicMail,
        };
        string Number(decimal value) =>
            value.ToString("0.########", CultureInfo.InvariantCulture);
        var parameters = new
        {
            draft = new
            {
                invoiceNumber = draft.InvoiceNumber,
                invoiceSeriesCode = draft.InvoiceSeriesCode,
                issueDate = draft.IssueDate.ToString("yyyy-MM-dd", CultureInfo.InvariantCulture),
                seller = Party(draft.Seller),
                buyer = Party(draft.Buyer),
                accountingOfficeDir3 = draft.AccountingOfficeDir3,
                managingBodyDir3 = draft.ManagingBodyDir3,
                processingUnitDir3 = draft.ProcessingUnitDir3,
                lines = draft.Lines.Select(line => new
                {
                    description = line.Description,
                    quantity = Number(line.Quantity),
                    unitPriceWithoutTax = Number(line.UnitPriceWithoutTax),
                    vatRate = Number(line.VatRate),
                }).ToArray(),
                installmentDueDate = draft.InstallmentDueDate?.ToString("yyyy-MM-dd", CultureInfo.InvariantCulture) ?? string.Empty,
                iban = draft.Iban,
                invoiceDescription = draft.InvoiceDescription,
                fileReference = draft.FileReference,
                receiverContractReference = draft.ReceiverContractReference,
            },
            outputPath = string.Empty,
        };
        return _ipcClient.SendAsync<object, FacturaeCreateResult>(
            DesktopOperationActions.FacturaeCreate, parameters, cancellationToken);
    }

    public Task<IpcCallResult<DesktopSettingsDocument>> GetSettingsAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            GetSettingsParameters,
            DesktopSettingsDocument>(
                DesktopOperationActions.GetSettings,
                new GetSettingsParameters(),
                cancellationToken);

    public Task<IpcCallResult<string>> SaveSettingsAsync(
        DesktopSettingsDocument settings,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(settings);
        return _ipcClient.SendAsync<DesktopSettingsDocument, string>(
            DesktopOperationActions.SaveSettings,
            settings.CreateSafeSaveSnapshot(),
            cancellationToken);
    }

    public Task<IpcCallResult<PingResult>> PingAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<PingParameters, PingResult>(
                DesktopOperationActions.Ping,
                new PingParameters(),
                cancellationToken);

    public Task<IpcCallResult<CheckCertificatesSummaryResult>>
        CheckCertificatesAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            CheckCertificatesParameters,
            CheckCertificatesSummaryResult>(
                DesktopOperationActions.CheckCertificates,
                new CheckCertificatesParameters(),
                cancellationToken);

    public Task<IpcCallResult<UpdateCheckResult>> CheckUpdatesAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<UpdateCheckParameters, UpdateCheckResult>(
            DesktopOperationActions.CheckUpdates,
            new UpdateCheckParameters(),
            cancellationToken);

    public Task<IpcCallResult<CertificateAccessInventoryResult>>
        GetCertificateAccessOptionsAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            CertificateAccessOptionsParameters,
            CertificateAccessInventoryResult>(
                DesktopOperationActions.CertificateAccessOptions,
                new CertificateAccessOptionsParameters(),
                cancellationToken);

    public Task<IpcCallResult<CertificateManagerOptionsResult>>
        GetCertificateManagersAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            CertificateAccessOptionsParameters,
            CertificateManagerOptionsResult>(
                DesktopOperationActions.CertificateAccessOptions,
                new CertificateAccessOptionsParameters(),
                cancellationToken);

    public Task<IpcCallResult<CertificateAccessOptionsResult>>
        GetCertificateImportOptionsAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            CertificateAccessOptionsParameters,
            CertificateAccessOptionsResult>(
                DesktopOperationActions.CertificateAccessOptions,
                new CertificateAccessOptionsParameters(),
                cancellationToken);

    public Task<IpcCallResult<string>> OpenCertificateManagerAsync(
        OpenCertificateManagerParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return _ipcClient.SendAsync<
            OpenCertificateManagerParameters,
            string>(
                DesktopOperationActions.OpenCertificateManager,
                parameters,
                cancellationToken);
    }

    public Task<IpcCallResult<string>> ImportCertificateToStoreAsync(
        ImportCertificateToStoreParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        ValidateCredentialBuffers(
            parameters.CredentialB64,
            parameters.PasswordB64);
        if (string.IsNullOrWhiteSpace(parameters.TargetId))
        {
            throw new ArgumentException(
                CatalogLocalizer.Shared.TranslateVisibleText("Debe indicar el almacén de destino."),
                nameof(parameters));
        }
        return _ipcClient.SendAsync<
            ImportCertificateToStoreParameters,
            string>(
                DesktopOperationActions.ImportCertificateToStore,
                parameters,
                cancellationToken);
    }

    public Task<IpcCallResult<TemporaryCertificateResult>>
        UseTemporaryCertificateAsync(
            UseTemporaryCertificateParameters parameters,
            CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        ValidateCredentialBuffers(
            parameters.CredentialB64,
            parameters.PasswordB64);
        return _ipcClient.SendAsync<
            UseTemporaryCertificateParameters,
            TemporaryCertificateResult>(
                DesktopOperationActions.UseTemporaryCertificate,
                parameters,
                cancellationToken);
    }

    public Task<IpcCallResult<string>> RemoveTemporaryCertificateAsync(
        RemoveTemporaryCertificateParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (string.IsNullOrWhiteSpace(parameters.CertificateId))
        {
            throw new ArgumentException(
                CatalogLocalizer.Shared.TranslateVisibleText("Debe indicar la credencial temporal."),
                nameof(parameters));
        }
        return _ipcClient.SendAsync<
            RemoveTemporaryCertificateParameters,
            string>(
                DesktopOperationActions.RemoveTemporaryCertificate,
                parameters,
                cancellationToken);
    }

    public Task<IpcCallResult<string>> ClearTemporaryCertificatesAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            ClearTemporaryCertificatesParameters,
            string>(
                DesktopOperationActions.ClearTemporaryCertificates,
                new ClearTemporaryCertificatesParameters(),
                cancellationToken);

    public Task<IpcCallResult<BatchSignResult>> SignBatchAsync(
        BatchSignParameters parameters,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return SendWithRemoteSecretsAsync<BatchSignParameters, BatchSignResult>(
            DesktopOperationActions.SignBatch,
            parameters,
            parameters.RemotePin,
            parameters.RemoteOtp,
            cancellationToken);
    }

    public Task<IpcCallResult<ProxySecretStoreStatusResult>>
        GetProxySecretStoreStatusAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            ProxySecretStoreStatusParameters,
            ProxySecretStoreStatusResult>(
                DesktopOperationActions.ProxySecretStoreStatus,
                new ProxySecretStoreStatusParameters(),
                cancellationToken);

    /// <summary>
    /// Consume el buffer de contraseña: queda sobrescrito tanto si la
    /// validación falla como si el transporte termina con error o éxito.
    /// </summary>
    public async Task<IpcCallResult<ProxySecretMutationResult>>
        StoreProxySecretAsync(
            ProxySecretStoreParameters parameters,
            CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        ArgumentNullException.ThrowIfNull(parameters.Password);
        try
        {
            var realm = NormalizeProxyCredentialText(
                parameters.Realm,
                nameof(parameters.Realm));
            var username = NormalizeProxyCredentialText(
                parameters.Username,
                nameof(parameters.Username));
            ValidateProxyPassword(parameters.Password);
            return await _ipcClient.SendAsync<
                ProxySecretStoreParameters,
                ProxySecretMutationResult>(
                    DesktopOperationActions.ProxySecretStore,
                    parameters with
                    {
                        Realm = realm,
                        Username = username,
                    },
                    cancellationToken).ConfigureAwait(false);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(parameters.Password);
        }
    }

    public Task<IpcCallResult<ProxySecretMutationResult>>
        DeleteProxySecretAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            ProxySecretDeleteParameters,
            ProxySecretMutationResult>(
                DesktopOperationActions.ProxySecretDelete,
                new ProxySecretDeleteParameters(),
                cancellationToken);

    public Task<IpcCallResult<TlsStoreDiagnosticResult>>
        GetTlsDiagnosticsAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            TlsDiagnosticsParameters,
            TlsStoreDiagnosticResult>(
                DesktopOperationActions.TlsDiagnostics,
                new TlsDiagnosticsParameters(),
                cancellationToken);

    public Task<IpcCallResult<ClockDiagnosticResult>>
        GetClockDiagnosticsAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            ClockDiagnosticsParameters,
            ClockDiagnosticResult>(
                DesktopOperationActions.ClockDiagnostics,
                new ClockDiagnosticsParameters(),
                cancellationToken);

    public Task<IpcCallResult<DesktopDiagnosticSummaryResult>>
        ExportDiagnosticSummaryAsync(
            CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            ExportDiagnosticParameters,
            DesktopDiagnosticSummaryResult>(
                DesktopOperationActions.ExportDiagnostic,
                new ExportDiagnosticParameters(),
                cancellationToken);

    public Task<IpcCallResult<string>> InstallPublicRootsAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            InstallPublicRootsParameters,
            string>(
                DesktopOperationActions.InstallPublicRoots,
                new InstallPublicRootsParameters(),
                cancellationToken);

    public Task<IpcCallResult<int>> ClearTlsTrustAsync(
        CancellationToken cancellationToken = default) =>
        _ipcClient.SendAsync<
            ClearTlsTrustParameters,
            int>(
                DesktopOperationActions.ClearTlsTrust,
                new ClearTlsTrustParameters(),
                cancellationToken);

    private static void ValidateCredentialBuffers(
        byte[]? credential,
        byte[]? password)
    {
        ArgumentNullException.ThrowIfNull(credential);
        ArgumentNullException.ThrowIfNull(password);
        if (credential.Length is 0 or > MaximumCredentialBytes)
        {
            throw new ArgumentException(
                CatalogLocalizer.Shared.TranslateVisibleText("La credencial está vacía o supera el límite permitido."),
                nameof(credential));
        }
        if (password.Length > MaximumPasswordBytes)
        {
            throw new ArgumentException(
                CatalogLocalizer.Shared.TranslateVisibleText("La contraseña supera el límite permitido."),
                nameof(password));
        }
    }

    private static string NormalizeProxyCredentialText(
        string? value,
        string parameterName)
    {
        ArgumentNullException.ThrowIfNull(value, parameterName);
        var normalized = value.Trim();
        if (normalized.Length == 0 ||
            Encoding.UTF8.GetByteCount(normalized) >
                MaximumProxyCredentialTextBytes ||
            normalized.Any(char.IsControl))
        {
            throw new ArgumentException(
                CatalogLocalizer.Shared.TranslateVisibleText("El dominio o usuario del proxy no es válido."),
                parameterName);
        }
        return normalized;
    }

    private static void ValidateProxyPassword(byte[] password)
    {
        if (password.Length is 0 or > MaximumPasswordBytes)
        {
            throw new ArgumentException(
                CatalogLocalizer.Shared.TranslateVisibleText("La contraseña del proxy está vacía o supera 4096 bytes."),
                nameof(password));
        }

        var remaining = password.AsSpan();
        while (!remaining.IsEmpty)
        {
            var status = Rune.DecodeFromUtf8(
                remaining,
                out var rune,
                out var consumed);
            if (status != OperationStatus.Done ||
                consumed <= 0 ||
                Rune.GetUnicodeCategory(rune) ==
                    UnicodeCategory.Control)
            {
                throw new ArgumentException(
                    CatalogLocalizer.Shared.TranslateVisibleText("La contraseña del proxy no contiene texto UTF-8 válido."),
                    nameof(password));
            }
            remaining = remaining[consumed..];
        }
    }
}
