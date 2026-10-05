// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Ipc;

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Operations;

public sealed record CertificatesParameters;

public sealed record CertificateExportPublicParameters
{
    [JsonPropertyName("certificateId")]
    public required string CertificateId { get; init; }

    [JsonPropertyName("outputPath")]
    public required string OutputPath { get; init; }

    [JsonPropertyName("format")]
    public required string Format { get; init; }
}

public sealed record CertificateExportPublicResult
{
    [JsonPropertyName("certificateDerBase64")]
    public string CertificateDerBase64 { get; init; } = string.Empty;

    [JsonPropertyName("outputPath")]
    public string OutputPath { get; init; } = string.Empty;

    [JsonPropertyName("format")]
    public string Format { get; init; } = string.Empty;

    [JsonPropertyName("encryptionSuitable")]
    public bool EncryptionSuitable { get; init; }
}

public sealed record FacturaeCreateResult
{
    [JsonPropertyName("xml")]
    public string Xml { get; init; } = string.Empty;

    [JsonPropertyName("total")]
    public string Total { get; init; } = string.Empty;
}

public sealed record SmartcardStatusParameters;

public sealed record SmartcardStatusResult
{
    [JsonPropertyName("readers")]
    public IReadOnlyList<SmartcardReaderInfo> Readers { get; init; } = [];
}

public sealed record SmartcardReaderInfo
{
    [JsonPropertyName("name")]
    public string Name { get; init; } = string.Empty;

    [JsonPropertyName("present")]
    public bool Present { get; init; }

    [JsonPropertyName("isDnie")]
    public bool IsDnie { get; init; }
}

public sealed record ValidateCertificateOnlineParameters
{
    private string _certificateId = string.Empty;

    [JsonPropertyName("certificateId")]
    public required string CertificateId
    {
        get => _certificateId;
        init => _certificateId = OperationResultText.Clean(value, 1024);
    }
}

public sealed record CertificateOnlineValidationResult
{
    private string _status = string.Empty;
    private string _userMessage = string.Empty;
    private string _reason = string.Empty;
    private string _method = string.Empty;
    private string _checkedAt = string.Empty;
    private string _revokedAt = string.Empty;

    [JsonPropertyName("status")]
    public string Status
    {
        get => _status;
        init => _status = OperationResultText.Clean(
            value,
            32,
            "unknown");
    }

    [JsonPropertyName("userMessage")]
    public string UserMessage
    {
        get => _userMessage;
        init => _userMessage = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("reason")]
    public string Reason
    {
        get => _reason;
        init => _reason = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("method")]
    public string Method
    {
        get => _method;
        init => _method = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("checkedAt")]
    public string CheckedAt
    {
        get => _checkedAt;
        init => _checkedAt = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("revokedAt")]
    public string RevokedAt
    {
        get => _revokedAt;
        init => _revokedAt = OperationResultText.Clean(value, 64);
    }
}

public sealed record OpenCertificateManagerParameters
{
    private string _managerId = string.Empty;

    [JsonPropertyName("managerId")]
    public required string ManagerId
    {
        get => _managerId;
        init => _managerId = OperationResultText.Clean(value, 256);
    }
}

public sealed record CertificateManagerInfo
{
    private string _id = string.Empty;
    private string _label = string.Empty;

    [JsonPropertyName("id")]
    public string Id
    {
        get => _id;
        init => _id = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("label")]
    public string Label
    {
        get => _label;
        init => _label = OperationResultText.Clean(
            value,
            256,
            CatalogLocalizer.Shared.TranslateVisibleText("Gestor de certificados"));
    }

    [JsonPropertyName("recommended")]
    public bool Recommended { get; init; }
}

/// <summary>
/// Vista operativa y acotada de los gestores que el backend ha descubierto.
/// </summary>
public sealed record CertificateManagerOptionsResult
{
    private string _preferredManager = string.Empty;

    [JsonPropertyName("managers")]
    [JsonConverter(typeof(CertificateManagerListConverter))]
    public IReadOnlyList<CertificateManagerInfo> Managers
    {
        get;
        init;
    } = [];

    [JsonPropertyName("preferredManager")]
    public string PreferredManager
    {
        get => _preferredManager;
        init => _preferredManager = OperationResultText.Clean(value, 256);
    }
}

public sealed class CertificateManagerListConverter
    : JsonConverter<IReadOnlyList<CertificateManagerInfo>>
{
    public override IReadOnlyList<CertificateManagerInfo> Read(
        ref Utf8JsonReader reader,
        Type typeToConvert,
        JsonSerializerOptions options)
    {
        if (reader.TokenType != JsonTokenType.StartArray)
        {
            reader.Skip();
            return [];
        }

        var entries = new List<CertificateManagerInfo>(
            OperationResultText.MaximumVisibleItems);
        while (reader.Read() &&
            reader.TokenType != JsonTokenType.EndArray)
        {
            if (entries.Count >= OperationResultText.MaximumVisibleItems)
            {
                reader.Skip();
                continue;
            }

            var entry = JsonSerializer.Deserialize<CertificateManagerInfo>(
                ref reader,
                options);
            if (entry is not null &&
                !string.IsNullOrWhiteSpace(entry.Id))
            {
                entries.Add(entry);
            }
        }
        return entries;
    }

    public override void Write(
        Utf8JsonWriter writer,
        IReadOnlyList<CertificateManagerInfo> value,
        JsonSerializerOptions options)
    {
        writer.WriteStartArray();
        foreach (var entry in (value ?? [])
            .Where(entry =>
                entry is not null &&
                !string.IsNullOrWhiteSpace(entry.Id))
            .Take(OperationResultText.MaximumVisibleItems))
        {
            JsonSerializer.Serialize(writer, entry, options);
        }
        writer.WriteEndArray();
    }
}

public sealed record CertificateImportTargetInfo
{
    private string _id = string.Empty;
    private string _label = string.Empty;
    private string _browser = string.Empty;

    [JsonPropertyName("id")]
    public string Id
    {
        get => _id;
        init => _id = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("label")]
    public string Label
    {
        get => _label;
        init => _label = OperationResultText.Clean(
            value,
            256,
            CatalogLocalizer.Shared.TranslateVisibleText("Almacén de certificados"));
    }

    [JsonPropertyName("browser")]
    public string Browser
    {
        get => _browser;
        init => _browser = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("recommended")]
    public bool Recommended { get; init; }
}

public sealed record CertificateAccessOptionsResult
{
    private string _detectedBrowser = string.Empty;
    private string _preferredManager = string.Empty;
    private string _preferredTarget = string.Empty;

    [JsonPropertyName("managers")]
    [JsonConverter(typeof(CertificateManagerListConverter))]
    public IReadOnlyList<CertificateManagerInfo> Managers
    {
        get;
        init;
    } = [];

    [JsonPropertyName("importTargets")]
    [JsonConverter(typeof(CertificateImportTargetListConverter))]
    public IReadOnlyList<CertificateImportTargetInfo> ImportTargets
    {
        get;
        init;
    } = [];

    [JsonPropertyName("detectedBrowser")]
    public string DetectedBrowser
    {
        get => _detectedBrowser;
        init => _detectedBrowser = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("preferredManager")]
    public string PreferredManager
    {
        get => _preferredManager;
        init => _preferredManager = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("preferredTarget")]
    public string PreferredTarget
    {
        get => _preferredTarget;
        init => _preferredTarget = OperationResultText.Clean(value, 256);
    }
}

public sealed class CertificateImportTargetListConverter
    : JsonConverter<IReadOnlyList<CertificateImportTargetInfo>>
{
    public override IReadOnlyList<CertificateImportTargetInfo> Read(
        ref Utf8JsonReader reader,
        Type typeToConvert,
        JsonSerializerOptions options)
    {
        if (reader.TokenType != JsonTokenType.StartArray)
        {
            reader.Skip();
            return [];
        }

        var entries = new List<CertificateImportTargetInfo>(
            OperationResultText.MaximumVisibleItems);
        while (reader.Read() &&
            reader.TokenType != JsonTokenType.EndArray)
        {
            if (entries.Count >= OperationResultText.MaximumVisibleItems)
            {
                reader.Skip();
                continue;
            }

            var entry = JsonSerializer.Deserialize<CertificateImportTargetInfo>(
                ref reader,
                options);
            if (entry is not null &&
                !string.IsNullOrWhiteSpace(entry.Id))
            {
                entries.Add(entry);
            }
        }
        return entries;
    }

    public override void Write(
        Utf8JsonWriter writer,
        IReadOnlyList<CertificateImportTargetInfo> value,
        JsonSerializerOptions options)
    {
        writer.WriteStartArray();
        foreach (var entry in (value ?? [])
            .Where(entry =>
                entry is not null &&
                !string.IsNullOrWhiteSpace(entry.Id))
            .Take(OperationResultText.MaximumVisibleItems))
        {
            JsonSerializer.Serialize(writer, entry, options);
        }
        writer.WriteEndArray();
    }
}

/// <summary>
/// Los dos buffers contienen bytes sin codificar. System.Text.Json los
/// representa como Base64 en el wire sin materializar la credencial ni la
/// contraseña como cadenas administradas.
/// </summary>
public sealed record ImportCertificateToStoreParameters
{
    [JsonPropertyName("credentialB64")]
    public required byte[] CredentialB64 { get; init; }

    [JsonPropertyName("passwordB64")]
    public required byte[] PasswordB64 { get; init; }

    [JsonPropertyName("targetId")]
    public required string TargetId { get; init; }
}

public sealed record UseTemporaryCertificateParameters
{
    [JsonPropertyName("credentialB64")]
    public required byte[] CredentialB64 { get; init; }

    [JsonPropertyName("passwordB64")]
    public required byte[] PasswordB64 { get; init; }
}

public sealed record TemporaryCertificateResult
{
    private string _id = string.Empty;
    private string _subject = string.Empty;
    private string _fingerprint = string.Empty;

    [JsonPropertyName("id")]
    public string Id
    {
        get => _id;
        init => _id = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("subject")]
    public string Subject
    {
        get => _subject;
        init => _subject = OperationResultText.Clean(
            value,
            512,
            CatalogLocalizer.Shared.TranslateVisibleText("Certificado temporal"));
    }

    [JsonPropertyName("fingerprint")]
    public string Fingerprint
    {
        get => _fingerprint;
        init => _fingerprint = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("temporary")]
    public bool Temporary { get; init; }
}

public sealed record RemoveTemporaryCertificateParameters
{
    [JsonPropertyName("certificateId")]
    public required string CertificateId { get; init; }
}

public sealed record ClearTemporaryCertificatesParameters;

public sealed record SignParameters
{
    [JsonPropertyName("inputPath")]
    public required string InputPath { get; init; }

    [JsonPropertyName("outputPath")]
    public string OutputPath { get; init; } = string.Empty;

    [JsonPropertyName("certificateId")]
    public string? CertificateId { get; init; }

    [JsonPropertyName("certificateIndex")]
    public int CertificateIndex { get; init; }

    [JsonPropertyName("additionalCertificateIds")]
    public IReadOnlyList<string>? AdditionalCertificateIds { get; init; }

    [JsonPropertyName("format")]
    public string Format { get; init; } = string.Empty;

    [JsonPropertyName("action")]
    public string Action { get; init; } = DesktopOperationActions.Sign;

    [JsonPropertyName("overwrite")]
    public string Overwrite { get; init; } = string.Empty;

    [JsonPropertyName("saveToDisk")]
    public bool SaveToDisk { get; init; } = true;

    [JsonPropertyName("returnSignatureB64")]
    public bool ReturnSignatureBase64 { get; init; }

    [JsonPropertyName("visibleSeal")]
    public VisibleSealParameters? VisibleSeal { get; init; }

    [JsonPropertyName("allowInvalidPDF")]
    public bool AllowInvalidPdf { get; init; }

    [JsonPropertyName("strictCompat")]
    public bool StrictCompatibility { get; init; }

    [JsonPropertyName("qrContent")]
    public string? QrContent { get; init; }

    [JsonPropertyName("reason")]
    public string? Reason { get; init; }

    [JsonPropertyName("location")]
    public string? Location { get; init; }

    [JsonPropertyName("contactInfo")]
    public string? ContactInfo { get; init; }

    [JsonPropertyName("extraOptions")]
    public IReadOnlyDictionary<string, string>? ExtraOptions { get; init; }

    /// <summary>
    /// PIN del certificado remoto (firma CSC). Binario para que
    /// System.Text.Json lo envíe en Base64 sin crear un string; el cliente lo
    /// borra al terminar la petición.
    /// </summary>
    [JsonPropertyName("remotePin")]
    public byte[]? RemotePin { get; init; }

    [JsonPropertyName("remoteOtp")]
    public byte[]? RemoteOtp { get; init; }
}

public sealed record BatchSignParameters
{
    [JsonPropertyName("inputPaths")]
    public IReadOnlyList<string> InputPaths { get; init; } = [];

    [JsonPropertyName("directoryPath")]
    public string DirectoryPath { get; init; } = string.Empty;

    [JsonPropertyName("outputDir")]
    public required string OutputDirectory { get; init; }

    [JsonPropertyName("certificateId")]
    public string? CertificateId { get; init; }

    [JsonPropertyName("certificateIndex")]
    public int CertificateIndex { get; init; }

    [JsonPropertyName("additionalCertificateIds")]
    public IReadOnlyList<string>? AdditionalCertificateIds { get; init; }

    [JsonPropertyName("format")]
    public string Format { get; init; } = string.Empty;

    [JsonPropertyName("action")]
    public string Action { get; init; } = DesktopOperationActions.Sign;

    [JsonPropertyName("overwrite")]
    public string Overwrite { get; init; } = "rename";

    [JsonPropertyName("visibleSeal")]
    public VisibleSealParameters? VisibleSeal { get; init; }

    [JsonPropertyName("allowInvalidPDF")]
    public bool AllowInvalidPdf { get; init; }

    [JsonPropertyName("strictCompat")]
    public bool StrictCompatibility { get; init; }

    [JsonPropertyName("qrContent")]
    public string? QrContent { get; init; }

    [JsonPropertyName("reason")]
    public string? Reason { get; init; }

    [JsonPropertyName("location")]
    public string? Location { get; init; }

    [JsonPropertyName("contactInfo")]
    public string? ContactInfo { get; init; }

    [JsonPropertyName("extraOptions")]
    public IReadOnlyDictionary<string, string>? ExtraOptions { get; init; }

    [JsonPropertyName("documentOverrides")]
    public IReadOnlyList<BatchSignDocumentOverride>? DocumentOverrides
    {
        get;
        init;
    }

    /// <summary>
    /// PIN del certificado remoto (firma CSC). Binario para que
    /// System.Text.Json lo envíe en Base64 sin crear un string; el cliente lo
    /// borra al terminar la petición.
    /// </summary>
    [JsonPropertyName("remotePin")]
    public byte[]? RemotePin { get; init; }

    [JsonPropertyName("remoteOtp")]
    public byte[]? RemoteOtp { get; init; }
}

public sealed record BatchSignDocumentOverride
{
    [JsonPropertyName("inputPath")]
    public required string InputPath { get; init; }

    [JsonPropertyName("visibleSeal")]
    public required VisibleSealParameters VisibleSeal { get; init; }
}

public sealed record VisibleSealParameters
{
    [JsonPropertyName("page")]
    public required string Page { get; init; }

    [JsonPropertyName("x")]
    public double X { get; init; }

    [JsonPropertyName("y")]
    public double Y { get; init; }

    [JsonPropertyName("w")]
    public double Width { get; init; }

    [JsonPropertyName("h")]
    public double Height { get; init; }

    [JsonPropertyName("pageWidth")]
    public required double PageWidth { get; init; }

    [JsonPropertyName("pageHeight")]
    public required double PageHeight { get; init; }

    [JsonPropertyName("rotation")]
    public int Rotation { get; init; }

    [JsonPropertyName("keepText")]
    public bool KeepText { get; init; } = true;

    [JsonPropertyName("imagePath")]
    public string? ImagePath { get; init; }

    [JsonPropertyName("logoOpacityPercent")]
    public int LogoOpacityPercent { get; init; } = 100;

    [JsonPropertyName("placements")]
    public IReadOnlyList<VisibleSealPlacementParameters>? Placements { get; init; }
}

public sealed record VisibleSealPlacementParameters
{
    [JsonPropertyName("page")]
    public required int Page { get; init; }

    [JsonPropertyName("rect")]
    public required VisibleSealRectParameters Rect { get; init; }

    [JsonPropertyName("rotation")]
    public int Rotation { get; init; }
}

public sealed record VisibleSealRectParameters
{
    [JsonPropertyName("x")]
    public double X { get; init; }

    [JsonPropertyName("y")]
    public double Y { get; init; }

    [JsonPropertyName("w")]
    public double Width { get; init; }

    [JsonPropertyName("h")]
    public double Height { get; init; }
}

public sealed record VerifyParameters
{
    [JsonPropertyName("inputPath")]
    public required string InputPath { get; init; }

    [JsonPropertyName("originalPath")]
    public string? OriginalPath { get; init; }
}

public sealed record PdfPreviewParameters
{
    [JsonPropertyName("path")]
    public required string Path { get; init; }

    [JsonPropertyName("page")]
    public int Page { get; init; } = 1;
}

public sealed record PdfPreviewResult
{
    [JsonPropertyName("data")]
    public byte[] Data { get; init; } = [];

    [JsonPropertyName("width")]
    public double Width { get; init; }

    [JsonPropertyName("height")]
    public double Height { get; init; }

    [JsonPropertyName("currentPage")]
    public int CurrentPage { get; init; }

    [JsonPropertyName("totalPages")]
    public int TotalPages { get; init; }
}

// Mismos campos de sello que la firma: el motor devuelve el PNG exacto que
// incrustará, con el nombre y el emisor del certificado elegido.
public sealed record SealPreviewParameters
{
    [JsonPropertyName("certificateId")]
    public string? CertificateId { get; init; }

    [JsonPropertyName("visibleSeal")]
    public required VisibleSealParameters VisibleSeal { get; init; }

    [JsonPropertyName("qrContent")]
    public string? QrContent { get; init; }

    [JsonPropertyName("reason")]
    public string? Reason { get; init; }

    [JsonPropertyName("location")]
    public string? Location { get; init; }

    [JsonPropertyName("contactInfo")]
    public string? ContactInfo { get; init; }

    [JsonPropertyName("extraOptions")]
    public IReadOnlyDictionary<string, string>? ExtraOptions { get; init; }
}

public sealed record SealPreviewResult
{
    [JsonPropertyName("image")]
    public byte[] Image { get; init; } = [];
}

public sealed record HashCreateParameters
{
    [JsonPropertyName("inputPath")]
    public required string InputPath { get; init; }

    [JsonPropertyName("outputPath")]
    public string OutputPath { get; init; } = string.Empty;

    [JsonPropertyName("algorithm")]
    public string Algorithm { get; init; } = string.Empty;

    [JsonPropertyName("format")]
    public string Format { get; init; } = string.Empty;

    [JsonPropertyName("recursive")]
    public bool Recursive { get; init; }
}

public sealed record HashCheckParameters
{
    [JsonPropertyName("inputPath")]
    public required string InputPath { get; init; }

    [JsonPropertyName("hashPath")]
    public required string HashPath { get; init; }

    [JsonPropertyName("outputPath")]
    public string? OutputPath { get; init; }

    [JsonPropertyName("algorithm")]
    public string? Algorithm { get; init; }

    [JsonPropertyName("recursive")]
    public bool? Recursive { get; init; }

    [JsonPropertyName("saveReportToDisk")]
    public bool? SaveReportToDisk { get; init; }
}

public sealed record CertificateInfo
{
    private string _id = string.Empty;
    private string _subject = string.Empty;
    private string _subjectName = string.Empty;
    private string _issuer = string.Empty;
    private string _issuerName = string.Empty;
    private string _notAfter = string.Empty;
    private string _validTo = string.Empty;
    private string _fingerprint = string.Empty;
    private string _serialNumber = string.Empty;
    private string _status = string.Empty;
    private string _type = string.Empty;
    private string _organization = string.Empty;
    private string _nif = string.Empty;

    [JsonPropertyName("id")]
    public string Id
    {
        get => _id;
        init => _id = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("subject")]
    public string Subject
    {
        get => _subject;
        init => _subject = OperationResultText.Clean(
            value,
            512,
            "Certificado sin titular");
    }

    [JsonPropertyName("subjectName")]
    public string SubjectName
    {
        get => _subjectName;
        init => _subjectName = OperationResultText.Clean(
            value,
            512,
            "Certificado sin titular");
    }

    [JsonPropertyName("issuer")]
    public string Issuer
    {
        get => _issuer;
        init => _issuer = OperationResultText.Clean(
            value,
            512,
            "Emisor desconocido");
    }

    [JsonPropertyName("issuerName")]
    public string IssuerName
    {
        get => _issuerName;
        init => _issuerName = OperationResultText.Clean(
            value,
            512,
            "Emisor desconocido");
    }

    [JsonPropertyName("notAfter")]
    public string NotAfter
    {
        get => _notAfter;
        init => _notAfter = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("validTo")]
    public string ValidTo
    {
        get => _validTo;
        init => _validTo = OperationResultText.Clean(value, 64);
    }

    [JsonPropertyName("fingerprint")]
    public string Fingerprint
    {
        get => _fingerprint;
        init => _fingerprint = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("serialNumber")]
    public string SerialNumber
    {
        get => _serialNumber;
        init => _serialNumber = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("status")]
    public string Status
    {
        get => _status;
        init => _status = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("canSign")]
    public bool CanSign { get; init; }

    // La tarjeta o el token pedirá PIN o confirmación al firmar; no
    // garantiza que la clave esté disponible.
    [JsonPropertyName("needsUnlock")]
    public bool NeedsUnlock { get; init; }

    [JsonPropertyName("tipo")]
    public string Type
    {
        get => _type;
        init => _type = OperationResultText.Clean(value, 64, "desconocido");
    }

    [JsonPropertyName("organizacion")]
    public string Organization
    {
        get => _organization;
        init => _organization = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("nif")]
    public string Nif
    {
        get => _nif;
        init => _nif = OperationResultText.Clean(value, 128);
    }

    [JsonPropertyName("caducado")]
    public bool IsExpired { get; init; }

    [JsonPropertyName("diasCaducidad")]
    public int DaysUntilExpiration { get; init; }

    // Firma remota CSC: el certificado lo custodia un prestador.
    [JsonPropertyName("remote")]
    public bool Remote { get; init; }

    [JsonPropertyName("remotePin")]
    public bool RemotePin { get; init; }

    [JsonPropertyName("remoteOtp")]
    public bool RemoteOtp { get; init; }

    [JsonPropertyName("remoteOtpOnline")]
    public bool RemoteOtpOnline { get; init; }
}

public sealed record ProtectionRecipientsParameters;

public sealed record ProtectionRecipientImportParameters
{
    [JsonPropertyName("path")]
    public required string Path { get; init; }
}

public sealed record ProtectionRecipientRemoveParameters
{
    [JsonPropertyName("id")]
    public required string Id { get; init; }
}

public sealed record ProtectionRecipientChangeResult
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;
}

public sealed record ProtectionRecipientsResult
{
    [JsonPropertyName("recipients")]
    public IReadOnlyList<ProtectionRecipientInfo> Recipients { get; init; } = [];

    [JsonIgnore]
    public IReadOnlyList<ProtectionRecipientInfo> VisibleRecipients =>
        (Recipients ?? [])
            .Where(recipient =>
                !string.IsNullOrWhiteSpace(recipient.Id))
            .Take(OperationResultText.MaximumVisibleItems)
            .ToArray();
}

public sealed record ProtectionRecipientInfo
{
    private string _id = string.Empty;
    private string _label = string.Empty;
    private string _profile = string.Empty;
    private string _algorithm = string.Empty;
    private string _origin = string.Empty;

    [JsonPropertyName("origin")]
    public string Origin
    {
        get => _origin;
        init => _origin = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("id")]
    public string Id
    {
        get => _id;
        init => _id = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("label")]
    public string Label
    {
        get => _label;
        init => _label = OperationResultText.Clean(
            value,
            512,
            CatalogLocalizer.Shared.TranslateVisibleText("Destinatario sin nombre"));
    }

    [JsonPropertyName("profile")]
    public string Profile
    {
        get => _profile;
        init => _profile = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("algorithm")]
    public string Algorithm
    {
        get => _algorithm;
        init => _algorithm = OperationResultText.Clean(value, 160);
    }

    [JsonPropertyName("authEnvelopedDataCompatible")]
    public bool AuthEnvelopedDataCompatible { get; init; }
}

public sealed record ProtectionParameters
{
    [JsonPropertyName("inputPath")]
    public required string InputPath { get; init; }

    [JsonPropertyName("outputPath")]
    public string OutputPath { get; init; } = string.Empty;

    [JsonPropertyName("certificateId")]
    public string? CertificateId { get; init; }

    [JsonPropertyName("certificateIndex")]
    public int CertificateIndex { get; init; }

    [JsonPropertyName("profile")]
    public required string Profile { get; init; }

    [JsonPropertyName("recipientIds")]
    public IReadOnlyList<string>? RecipientIds { get; init; }

    [JsonPropertyName("overwrite")]
    public string Overwrite { get; init; } = string.Empty;

    [JsonPropertyName("saveToDisk")]
    public bool SaveToDisk { get; init; } = true;

    [JsonPropertyName("returnProtectedB64")]
    public bool ReturnProtectedBase64 { get; init; }

    [JsonPropertyName("returnUnprotectedB64")]
    public bool ReturnUnprotectedBase64 { get; init; }

    [JsonPropertyName("options")]
    public IReadOnlyDictionary<string, string>? Options { get; init; }

    // System.Text.Json codifica byte[] como Base64 y permite borrar el buffer
    // tras SendAsync. Nunca se representa esta clave como string administrada.
    [JsonPropertyName("secretB64")]
    public byte[]? SymmetricKey { get; init; }
}

public sealed record ProtectionResult
{
    private string _profile = string.Empty;
    private string _documentName = string.Empty;
    private string _mimeType = string.Empty;
    private string _certificateId = string.Empty;

    [JsonPropertyName("outputPath")]
    public string? OutputPath { get; init; }

    [JsonPropertyName("protectedContentBase64")]
    public string? ProtectedContentBase64 { get; init; }

    [JsonPropertyName("profile")]
    public string Profile
    {
        get => _profile;
        init => _profile = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("recipientCount")]
    public int RecipientCount { get; init; }

    [JsonPropertyName("documentName")]
    public string DocumentName
    {
        get => _documentName;
        init => _documentName = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("mimeType")]
    public string MimeType
    {
        get => _mimeType;
        init => _mimeType = OperationResultText.Clean(value, 160);
    }

    [JsonPropertyName("certificateId")]
    public string CertificateId
    {
        get => _certificateId;
        init => _certificateId = OperationResultText.Clean(value, 1024);
    }
}

public sealed record UnprotectResult
{
    private string _profile = string.Empty;
    private string _recipientId = string.Empty;
    private string _documentName = string.Empty;
    private string _mimeType = string.Empty;

    [JsonPropertyName("outputPath")]
    public string? OutputPath { get; init; }

    [JsonPropertyName("unprotectedContentBase64")]
    public string? UnprotectedContentBase64 { get; init; }

    [JsonPropertyName("profile")]
    public string Profile
    {
        get => _profile;
        init => _profile = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("recipientId")]
    public string RecipientId
    {
        get => _recipientId;
        init => _recipientId = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("documentName")]
    public string DocumentName
    {
        get => _documentName;
        init => _documentName = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("mimeType")]
    public string MimeType
    {
        get => _mimeType;
        init => _mimeType = OperationResultText.Clean(value, 160);
    }
}

public sealed record SignResult
{
    // El nombre con O y P mayúsculas forma parte del contrato Go existente.
    [JsonPropertyName("OutputPath")]
    public string? OutputPath { get; init; }

    [JsonPropertyName("signature_b64")]
    public string? SignatureBase64 { get; init; }

    // Metadato opcional: el motor confirma el formato realmente generado,
    // también cuando la solicitud eligió Automático.
    [JsonPropertyName("format")]
    public string? Format { get; init; }

    [JsonIgnore]
    public string DisplayOutputPath =>
        OperationResultText.Clean(OutputPath, 1024);
}

public sealed record BatchSignItemResult
{
    private string _inputPath = string.Empty;
    private string _outputPath = string.Empty;
    private string _format = string.Empty;
    private string _error = string.Empty;

    [JsonPropertyName("inputPath")]
    public string InputPath
    {
        get => _inputPath;
        init => _inputPath = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("outputPath")]
    public string OutputPath
    {
        get => _outputPath;
        init => _outputPath = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("format")]
    public string Format
    {
        get => _format;
        init => _format = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("ok")]
    public bool IsSuccess { get; init; }

    [JsonPropertyName("error")]
    public string Error
    {
        get => _error;
        init => _error = OperationResultText.Clean(value, 1024);
    }
}

public sealed record BatchSignResult
{
    [JsonPropertyName("results")]
    [JsonConverter(typeof(BatchSignItemResultListConverter))]
    public IReadOnlyList<BatchSignItemResult> Results { get; init; } = [];

    [JsonPropertyName("okCount")]
    public int SuccessCount { get; init; }

    [JsonPropertyName("failCount")]
    public int FailureCount { get; init; }
}

public sealed class BatchSignItemResultListConverter
    : JsonConverter<IReadOnlyList<BatchSignItemResult>>
{
    public override IReadOnlyList<BatchSignItemResult> Read(
        ref Utf8JsonReader reader,
        Type typeToConvert,
        JsonSerializerOptions options)
    {
        if (reader.TokenType != JsonTokenType.StartArray)
        {
            reader.Skip();
            return [];
        }

        var entries = new List<BatchSignItemResult>(
            OperationResultText.MaximumVisibleItems + 1);
        while (reader.Read() &&
            reader.TokenType != JsonTokenType.EndArray)
        {
            // Se conserva una entrada centinela adicional para que el
            // consumidor detecte una respuesta que exceda el máximo local.
            if (entries.Count >
                OperationResultText.MaximumVisibleItems)
            {
                reader.Skip();
                continue;
            }

            var entry = JsonSerializer.Deserialize<BatchSignItemResult>(
                ref reader,
                options);
            if (entry is not null)
            {
                entries.Add(entry);
            }
        }
        return entries;
    }

    public override void Write(
        Utf8JsonWriter writer,
        IReadOnlyList<BatchSignItemResult> value,
        JsonSerializerOptions options)
    {
        writer.WriteStartArray();
        foreach (var entry in (value ?? [])
            .Where(entry => entry is not null)
            .Take(OperationResultText.MaximumVisibleItems + 1))
        {
            JsonSerializer.Serialize(writer, entry, options);
        }
        writer.WriteEndArray();
    }
}

public sealed record VerifyResult
{
    private string _reason = string.Empty;
    private string _format = string.Empty;
    private string _coverage = string.Empty;

    [JsonPropertyName("valid")]
    public bool IsValid { get; init; }

    [JsonPropertyName("reason")]
    public string Reason
    {
        get => _reason;
        init => _reason = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("details")]
    public IReadOnlyList<string> Details { get; init; } = [];

    [JsonPropertyName("signers")]
    public IReadOnlyList<string> Signers { get; init; } = [];

    [JsonPropertyName("format")]
    public string Format
    {
        get => _format;
        init => _format = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("coverage")]
    public string Coverage
    {
        get => _coverage;
        init => _coverage = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("integrity")]
    public VerifyAspect Integrity { get; init; } = new();

    [JsonPropertyName("certificate")]
    public VerifyAspect Certificate { get; init; } = new();

    [JsonPropertyName("trust")]
    public VerifyAspect Trust { get; init; } = new();

    [JsonPropertyName("signerSummaries")]
    public IReadOnlyList<VerifySignerSummary> SignerSummaries { get; init; } = [];

    [JsonPropertyName("warnings")]
    public IReadOnlyList<string> Warnings { get; init; } = [];

    [JsonPropertyName("errors")]
    public IReadOnlyList<string> Errors { get; init; } = [];

    [JsonPropertyName("evidence")]
    public IReadOnlyList<VerifyEvidence> Evidence { get; init; } = [];

    [JsonIgnore]
    public IReadOnlyList<string> VisibleDetails =>
        OperationResultText.CleanList(Details);

    [JsonIgnore]
    public IReadOnlyList<string> VisibleSigners =>
        OperationResultText.CleanList(
            Signers,
            maximumItems: OperationResultText.MaximumVisibleItems,
            maximumCharacters: 512);

    [JsonIgnore]
    public IReadOnlyList<VerifySignerSummary> VisibleSignerSummaries =>
        (SignerSummaries ?? [])
            .Where(static item => item is not null)
            .Take(OperationResultText.MaximumVisibleItems)
            .ToArray();

    [JsonIgnore]
    public IReadOnlyList<string> VisibleWarnings =>
        OperationResultText.CleanList(Warnings);

    [JsonIgnore]
    public IReadOnlyList<string> VisibleErrors =>
        OperationResultText.CleanList(Errors);

    [JsonIgnore]
    public IReadOnlyList<VerifyEvidence> VisibleEvidence =>
        (Evidence ?? [])
            .Where(static item => item is not null)
            .Take(OperationResultText.MaximumVisibleItems)
            .ToArray();
}

public sealed record VerifyAspect
{
    private string _status = string.Empty;
    private string _reason = string.Empty;

    [JsonPropertyName("status")]
    public string Status
    {
        get => _status;
        init => _status = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("reason")]
    public string Reason
    {
        get => _reason;
        init => _reason = OperationResultText.Clean(value, 1024);
    }

    [JsonPropertyName("details")]
    public IReadOnlyList<string> Details { get; init; } = [];

    [JsonIgnore]
    public IReadOnlyList<string> VisibleDetails =>
        OperationResultText.CleanList(Details);
}

public sealed record VerifySignerSummary
{
    private string _id = string.Empty;
    private string _subject = string.Empty;
    private string _issuer = string.Empty;
    private string _fingerprint = string.Empty;

    [JsonPropertyName("id")]
    public string Id
    {
        get => _id;
        init => _id = OperationResultText.Clean(value, 256);
    }

    [JsonPropertyName("subject")]
    public string Subject
    {
        get => _subject;
        init => _subject = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("issuer")]
    public string Issuer
    {
        get => _issuer;
        init => _issuer = OperationResultText.Clean(value, 512);
    }

    [JsonPropertyName("fingerprint")]
    public string Fingerprint
    {
        get => _fingerprint;
        init => _fingerprint = OperationResultText.Clean(value, 256);
    }
}

public sealed record VerifyEvidence
{
    private string _type = string.Empty;
    private string _summary = string.Empty;

    [JsonPropertyName("type")]
    public string Type
    {
        get => _type;
        init => _type = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("summary")]
    public string Summary
    {
        get => _summary;
        init => _summary = OperationResultText.Clean(value, 1024);
    }
}

/// <summary>
/// Unión por campos del resultado de hash de fichero y de directorio.
/// El motor devuelve uno u otro objeto bajo la misma acción.
/// </summary>
public sealed record HashCreateResult
{
    private string _algorithm = string.Empty;
    private string _format = string.Empty;
    private string _hash = string.Empty;

    [JsonPropertyName("algorithm")]
    public string Algorithm
    {
        get => _algorithm;
        init => _algorithm = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("format")]
    public string Format
    {
        get => _format;
        init => _format = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("hash")]
    public string Hash
    {
        get => _hash;
        init => _hash = OperationResultText.Clean(value, 4096);
    }

    [JsonPropertyName("outputPath")]
    public string? OutputPath { get; init; }

    [JsonPropertyName("entries")]
    public int? Entries { get; init; }

    [JsonPropertyName("recursive")]
    public bool? Recursive { get; init; }

    [JsonPropertyName("manifestBase64")]
    public string? ManifestBase64 { get; init; }

    [JsonIgnore]
    public string DisplayOutputPath =>
        OperationResultText.Clean(OutputPath, 1024);
}

/// <summary>
/// Unión por campos del resultado de comprobación de fichero y de directorio.
/// </summary>
public sealed record HashCheckResult
{
    private string _algorithm = string.Empty;
    private string _format = string.Empty;
    private string _expectedHash = string.Empty;
    private string _actualHash = string.Empty;

    [JsonPropertyName("valid")]
    public bool IsValid { get; init; }

    [JsonPropertyName("algorithm")]
    public string Algorithm
    {
        get => _algorithm;
        init => _algorithm = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("format")]
    public string Format
    {
        get => _format;
        init => _format = OperationResultText.Clean(value, 80);
    }

    [JsonPropertyName("expectedHash")]
    public string ExpectedHash
    {
        get => _expectedHash;
        init => _expectedHash = OperationResultText.Clean(value, 4096);
    }

    [JsonPropertyName("actualHash")]
    public string ActualHash
    {
        get => _actualHash;
        init => _actualHash = OperationResultText.Clean(value, 4096);
    }

    [JsonPropertyName("recursive")]
    public bool? Recursive { get; init; }

    [JsonPropertyName("matching_hash")]
    public IReadOnlyList<string> MatchingHash { get; init; } = [];

    [JsonPropertyName("not_matching_hash")]
    public IReadOnlyList<string> NotMatchingHash { get; init; } = [];

    [JsonPropertyName("hash_without_file")]
    public IReadOnlyList<string> HashWithoutFile { get; init; } = [];

    [JsonPropertyName("file_without_hash")]
    public IReadOnlyList<string> FileWithoutHash { get; init; } = [];

    [JsonPropertyName("reportBase64")]
    public string? ReportBase64 { get; init; }

    [JsonPropertyName("reportOutputPath")]
    public string? ReportOutputPath { get; init; }

    [JsonIgnore]
    public string DisplayReportOutputPath =>
        OperationResultText.Clean(ReportOutputPath, 1024);

    [JsonIgnore]
    public IReadOnlyList<string> VisibleMatchingHash =>
        OperationResultText.CleanPathList(MatchingHash);

    [JsonIgnore]
    public IReadOnlyList<string> VisibleNotMatchingHash =>
        OperationResultText.CleanPathList(NotMatchingHash);

    [JsonIgnore]
    public IReadOnlyList<string> VisibleHashWithoutFile =>
        OperationResultText.CleanPathList(HashWithoutFile);

    [JsonIgnore]
    public IReadOnlyList<string> VisibleFileWithoutHash =>
        OperationResultText.CleanPathList(FileWithoutHash);
}

internal static class OperationResultText
{
    internal const int MaximumVisibleItems = 128;
    private const int MaximumVisiblePaths = 2048;

    internal static string Clean(
        string? value,
        int maximumCharacters,
        string fallback = "") =>
        SafeIpcText.Clean(value, maximumCharacters, fallback);

    internal static IReadOnlyList<string> CleanList(
        IEnumerable<string>? values,
        int maximumItems = MaximumVisibleItems,
        int maximumCharacters = 1024) =>
        (values ?? [])
            .Take(maximumItems)
            .Select(value => Clean(value, maximumCharacters))
            .ToArray();

    internal static IReadOnlyList<string> CleanPathList(
        IEnumerable<string>? values) =>
        CleanList(
            values,
            maximumItems: MaximumVisiblePaths,
            maximumCharacters: 1024);
}
