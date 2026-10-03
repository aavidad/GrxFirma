// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace GrxFirma.WinUI.Core.Operations;

public sealed record GetSettingsParameters;

/// <summary>
/// Subconjunto de preferencias compartidas que consume la interfaz WinUI.
/// Las propiedades adicionales se conservan únicamente para que guardar una
/// edición parcial no borre preferencias válidas creadas por la GUI Qt.
/// Nunca se muestran ni se registran.
/// </summary>
public sealed record DesktopSettingsDocument
{
    public const int MaximumProxyExcludedUrlItems = 512;
    public const int MaximumProxyExcludedUrlCharacters = 2048;

    private const int MaximumPreservedArrayItems = 512;
    private const int MaximumPreservedStringCharacters = 8192;
    private const int MaximumPreservedArrayStringCharacters = 2048;
    private const int MaximumPreservedNumberCharacters = 128;
    private const int MaximumPreservedDocumentBytes = 512 * 1024;

    // Lista cerrada de preferencias ya publicadas por el contrato Go/Qt.
    // Las propiedades tipadas de este DTO no aparecen en JsonExtensionData.
    // Las referencias a secretos, credenciales y estados efímeros se omiten
    // deliberadamente aunque existan en el documento recibido.
    private static readonly IReadOnlySet<string> SafeAdditionalSettingNames =
        new HashSet<string>(
        [
            "tema",
            "expertMode",
            "autoClose",
            "omitAskOnClose",
            "hideDnieStartScreen",
            "secureConnections",
            "secureDomainsList",
            "signAction",
            "signProfile",
            "signOverwrite",
            "signAllowInvalidPDF",
            "autoFormatPdf",
            "autoFormatOoxml",
            "autoFormatFacturae",
            "autoFormatOdf",
            "autoFormatXml",
            "autoFormatBinary",
            "signReason",
            "signLocation",
            "signContactInfo",
            "facturaePolicyVersion",
            "policyIdentifier",
            "policyIdentifierHash",
            "policyQualifier",
            "signerClaimedRole",
            "signatureProductionCity",
            "signatureProductionProvince",
            "signatureProductionPostalCode",
            "signatureProductionCountry",
            "padesSubFilter",
            "padesPolicyIdentifier",
            "padesPolicyIdentifierHash",
            "padesPolicyIdentifierHashAlgorithm",
            "padesPolicyQualifier",
            "padesObfuscateCertInfo",
            "padesVisibleStamp",
            "allowShadowAttack",
            "allowCertifiedPDF",
            "padesCertificationLevel",
            "cadesPolicyIdentifier",
            "cadesPolicyIdentifierHash",
            "cadesPolicyIdentifierHashAlgorithm",
            "cadesPolicyQualifier",
            "cadesImplicitMode",
            "cadesMultisign",
            "xadesPolicyIdentifier",
            "xadesPolicyIdentifierHash",
            "xadesPolicyIdentifierHashAlgorithm",
            "xadesPolicyQualifier",
            "xadesSignFormat",
            "xadesMultisign",
            "xadesSignerClaimedRole",
            "xadesSignatureProductionCity",
            "xadesSignatureProductionProvince",
            "xadesSignatureProductionPostalCode",
            "xadesSignatureProductionCountry",
            "signSealPages",
            "signSealAllPages",
            "signSealX",
            "signSealY",
            "signSealW",
            "signSealH",
            "signSealKeepText",
            "signSealRotation",
            "signSealImagePath",
            "signQRContent",
            "multiCosignEnabled",
            "multiCosignPrimaryCertificateId",
            "multiCosignCertificateIds",
            "defaultHashAlgorithm",
            "defaultHashCopyToClipboard",
            "defaultHashFormatFile",
            "defaultHashFormatDirectory",
            "defaultHashRecursive",
            "defaultHashSaveReport",
            "stickySigner",
            "autoSelectSingleCertificate",
            "preferredCertificateId",
            "defaultKeystore",
            "defaultLocalKeystorePath",
            "useDefaultStoreInBrowserCalls",
            "useOnlySignatureCertificates",
            "useOnlyAliasCertificates",
            "skipAuthCertDnie",
            "showDefaultCertificateFirst",
            "showUsableCertificatesFirst",
            "showValidCertificatesFirst",
            "rememberCertificateFilter",
            "certificateFilterText",
            "certsExpiredShow",
            "certsInvalidShow",
            "certificateTypeFilter",
            "certificateRequireNIF",
            "certificateRequireOrganization",
            "legacyWebUiSize",
            "legacyWebTrayResident",
            "webCompatibilityDurationMinutes",
        ],
        StringComparer.Ordinal);

    [JsonPropertyName("idioma")]
    public string? Language { get; init; }

    [JsonPropertyName("themeIndex")]
    public int? ThemeIndex { get; init; }

    [JsonPropertyName("confirmToSign")]
    public bool? ConfirmBeforeSigning { get; init; }

    [JsonPropertyName("closeBehavior")]
    public string? CloseBehavior { get; init; }

    [JsonPropertyName("signFormat")]
    public string? DefaultSignatureFormat { get; init; }

    [JsonPropertyName("tsaEnabled")]
    public bool? TsaEnabled { get; init; }

    [JsonPropertyName("tsaUrl")]
    public string? TsaUrl { get; init; }

    [JsonPropertyName("signStrictCompat")]
    public bool? StrictSignatureCompatibility { get; init; }

    [JsonPropertyName("signVisibleSeal")]
    public bool? VisiblePdfSeal { get; init; }

    [JsonPropertyName("signSealLogoOpacityPercent")]
    public int? SealLogoOpacityPercent { get; init; }

    [JsonPropertyName("signSealPerPage")]
    public bool? SealPerPage { get; init; }

    [JsonPropertyName("signQREnabled")]
    public bool? SealQrEnabled { get; init; }

    [JsonPropertyName("signSealPlacements")]
    public IReadOnlyDictionary<string, VisibleSealPlacementParameters>? SealPlacements { get; init; }

    [JsonPropertyName("facturaeToolsEnabled")]
    public bool? FacturaeToolsEnabled { get; init; }

    [JsonPropertyName("checkForUpdates")]
    public bool? CheckForUpdates { get; init; }

    [JsonPropertyName("preferDefaultCertificate")]
    public bool? PreferDefaultCertificate { get; init; }

    [JsonPropertyName("signCertificatePanelExpanded")]
    public bool? SignCertificatePanelExpanded { get; init; }

    [JsonPropertyName("defaultCertificateId")]
    public string? DefaultCertificateId { get; init; }

    [JsonPropertyName("proxyEnabled")]
    public bool? ProxyEnabled { get; init; }

    [JsonPropertyName("proxyType")]
    public string? ProxyType { get; init; }

    [JsonPropertyName("proxyHost")]
    public string? ProxyHost { get; init; }

    [JsonPropertyName("proxyPort")]
    public int? ProxyPort { get; init; }

    [JsonPropertyName("proxyExcludedUrls")]
    public IReadOnlyList<string>? ProxyExcludedUrls { get; init; }

    // Metadatos de solo lectura del almacén seguro. Permiten representar si
    // existe una credencial, pero CreateSafeSaveSnapshot los retira siempre:
    // proxy_secret_store/proxy_secret_delete son las únicas acciones que
    // pueden mutar esta referencia en el backend.
    [JsonPropertyName("proxySecretId")]
    public string? ProxySecretId { get; init; }

    [JsonPropertyName("proxyRealm")]
    public string? ProxyRealm { get; init; }

    [JsonExtensionData]
    public Dictionary<string, JsonElement> AdditionalSettings { get; init; } =
        new(StringComparer.Ordinal);

    /// <summary>
    /// Genera el documento que se puede devolver a save_settings. Conserva
    /// claves planas de preferencias para no destruir opciones de otras GUI,
    /// pero elimina credenciales, referencias protegidas, estado efímero y
    /// estructuras arbitrarias.
    /// </summary>
    public DesktopSettingsDocument CreateSafeSaveSnapshot()
    {
        var preserved = new Dictionary<string, JsonElement>(
            StringComparer.Ordinal);
        var preservedBytes = 0;
        foreach (var (name, value) in AdditionalSettings)
        {
            if (!IsSafeAdditionalName(name) ||
                !IsSafeAdditionalValue(value))
            {
                continue;
            }

            var entryBytes =
                Encoding.UTF8.GetByteCount(name) +
                Encoding.UTF8.GetByteCount(value.GetRawText()) +
                8;
            if (entryBytes >
                MaximumPreservedDocumentBytes - preservedBytes)
            {
                continue;
            }

            preserved[name] = value.Clone();
            preservedBytes += entryBytes;
        }

        return this with
        {
            Language = NormalizeOptional(Language, 16),
            CloseBehavior = NormalizeOptional(CloseBehavior, 16),
            DefaultSignatureFormat = NormalizeOptional(
                DefaultSignatureFormat,
                32),
            TsaUrl = NormalizeOptional(TsaUrl, 2048),
            DefaultCertificateId = DefaultCertificateId is null
                ? null
                : OperationResultText.Clean(
                    DefaultCertificateId,
                    1024),
            ProxyType = NormalizeOptional(ProxyType, 16),
            ProxyHost = NormalizeOptional(ProxyHost, 512),
            ProxyExcludedUrls = NormalizeProxyExcludedUrls(
                ProxyExcludedUrls),
            SealPlacements = NormalizeSealPlacements(SealPlacements),
            ProxySecretId = null,
            ProxyRealm = null,
            AdditionalSettings = preserved,
        };
    }

    public static bool IsSupportedLanguage(string? value) =>
        value is "es" or "ca" or "va" or "eu" or "gl" or "en" or
            "de" or "fr" or "pt" or "it" or "zh";

    public static bool IsSupportedTheme(int? value) =>
        value is >= 0 and <= 2;

    public static bool IsSupportedCloseBehavior(string? value) =>
        value is "exit" or "resident";

    public static bool IsSupportedSignatureFormat(string? value) =>
        value is "" or "pades" or "cades" or "xades" or "xmldsig" or
            "odf" or "ooxml" or "facturae" or "asic-xades";

    public static bool IsSupportedProxyType(string? value) =>
        value is "none" or "manual";

    private static string? NormalizeOptional(
        string? value,
        int maximumCharacters)
    {
        if (value is null)
        {
            return null;
        }

        var trimmed = value.Trim();
        return trimmed.Length <= maximumCharacters
            ? trimmed
            : trimmed[..maximumCharacters];
    }

    private static IReadOnlyList<string> NormalizeProxyExcludedUrls(
        IReadOnlyList<string>? values)
    {
        if (values is null || values.Count == 0)
        {
            return [];
        }

        var normalized = new List<string>(
            Math.Min(values.Count, MaximumProxyExcludedUrlItems));
        var seen = new HashSet<string>(StringComparer.Ordinal);
        foreach (var value in values.Take(MaximumProxyExcludedUrlItems))
        {
            var candidate = value?.Trim() ?? string.Empty;
            if (candidate.Length == 0 ||
                candidate.Length >
                    MaximumProxyExcludedUrlCharacters ||
                candidate.Any(char.IsControl) ||
                !seen.Add(candidate))
            {
                continue;
            }
            normalized.Add(candidate);
        }
        return normalized;
    }

    private static IReadOnlyDictionary<string, VisibleSealPlacementParameters>? NormalizeSealPlacements(
        IReadOnlyDictionary<string, VisibleSealPlacementParameters>? placements)
    {
        if (placements is null || placements.Count > 128) return null;
        var safe = new Dictionary<string, VisibleSealPlacementParameters>(StringComparer.Ordinal);
        foreach (var (key, placement) in placements)
        {
            if (placement is null || placement.Rect is null) return null;
            var rect = placement.Rect;
            if (!int.TryParse(key, System.Globalization.NumberStyles.None,
                    System.Globalization.CultureInfo.InvariantCulture, out var page) ||
                page is < 1 or > 10000 ||
                placement.Page != page || placement.Rotation is < 0 or > 359 ||
                !double.IsFinite(rect.X) || !double.IsFinite(rect.Y) ||
                !double.IsFinite(rect.Width) || !double.IsFinite(rect.Height) ||
                rect.X < 0 || rect.Y < 0 || rect.Width <= 0 || rect.Height <= 0 ||
                rect.X + rect.Width > 1 || rect.Y + rect.Height > 1)
                return null;
            safe[key] = placement;
        }
        return safe;
    }

    private static bool IsSafeAdditionalName(string name)
        => SafeAdditionalSettingNames.Contains(name);

    private static bool IsSafeAdditionalValue(JsonElement value) =>
        value.ValueKind switch
        {
            JsonValueKind.String =>
                (value.GetString()?.Length ?? 0) <=
                    MaximumPreservedStringCharacters,
            JsonValueKind.Number =>
                value.GetRawText().Length <=
                    MaximumPreservedNumberCharacters,
            JsonValueKind.True or
            JsonValueKind.False => true,
            JsonValueKind.Array =>
                value.GetArrayLength() <= MaximumPreservedArrayItems &&
                value.EnumerateArray().All(IsSafeAdditionalArrayItem),
            _ => false,
        };

    private static bool IsSafeAdditionalArrayItem(JsonElement value) =>
        value.ValueKind switch
        {
            JsonValueKind.String =>
                (value.GetString()?.Length ?? 0) <=
                    MaximumPreservedArrayStringCharacters,
            JsonValueKind.Number =>
                value.GetRawText().Length <=
                    MaximumPreservedNumberCharacters,
            JsonValueKind.True or
            JsonValueKind.False => true,
            _ => false,
        };
}
