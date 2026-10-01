// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DesktopSettingsClientTests
{
    private static readonly JsonSerializerOptions WireOptions = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    [TestMethod]
    public void SealPlacements_ArePreservedOnlyWhenBounded()
    {
        var valid = JsonSerializer.Deserialize<DesktopSettingsDocument>(
            """
            {"signSealPerPage":true,"signQREnabled":false,"signSealPlacements":{"2":{"page":2,"rect":{"x":0.1,"y":0.2,"w":0.3,"h":0.1},"rotation":90}}}
            """)!;
        var snapshot = valid.CreateSafeSaveSnapshot();
        Assert.AreEqual(true, snapshot.SealPerPage);
        Assert.AreEqual(false, snapshot.SealQrEnabled);
        Assert.AreEqual(90, snapshot.SealPlacements!["2"].Rotation);

        var invalid = JsonSerializer.Deserialize<DesktopSettingsDocument>(
            """
            {"signSealPlacements":{"2":{"page":2,"rect":{"x":0.9,"y":0.2,"w":0.3,"h":0.1},"rotation":360}}}
            """)!;
        Assert.IsNull(invalid.CreateSafeSaveSnapshot().SealPlacements);
    }

    [TestMethod]
    public async Task Methods_SendExactActionsTypesAndCancellation()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        using var cancellation = new CancellationTokenSource();
        var settings = NewSettings();

        await client.GetSettingsAsync(cancellation.Token);
        await client.SaveSettingsAsync(settings, cancellation.Token);

        CollectionAssert.AreEqual(
            new[] { "get_settings", "save_settings" },
            transport.Calls.Select(call => call.Action).ToArray());
        CollectionAssert.AreEqual(
            new[]
            {
                typeof(GetSettingsParameters),
                typeof(DesktopSettingsDocument),
            },
            transport.Calls
                .Select(call => call.Parameters.GetType())
                .ToArray());
        CollectionAssert.AreEqual(
            new[]
            {
                typeof(DesktopSettingsDocument),
                typeof(string),
            },
            transport.Calls.Select(call => call.DataType).ToArray());
        Assert.IsTrue(transport.Calls.All(
            call => call.CancellationToken == cancellation.Token));
    }

    [TestMethod]
    public void Document_UsesExactGoAndQtJsonNames()
    {
        using var document = JsonDocument.Parse(
            JsonSerializer.Serialize(NewSettings(), WireOptions));
        var names = document.RootElement
            .EnumerateObject()
            .Select(property => property.Name)
            .Order(StringComparer.Ordinal)
            .ToArray();

        CollectionAssert.AreEqual(
            new[]
            {
                "closeBehavior",
                "confirmToSign",
                "idioma",
                "defaultCertificateId",
                "facturaeToolsEnabled",
                "preferDefaultCertificate",
                "proxyEnabled",
                "proxyExcludedUrls",
                "proxyHost",
                "proxyPort",
                "proxyType",
                "signFormat",
                "signSealLogoOpacityPercent",
                "signStrictCompat",
                "signVisibleSeal",
                "tema",
                "themeIndex",
            }.Order(StringComparer.Ordinal).ToArray(),
            names);
    }

    [TestMethod]
    public void Document_DeserializesTypedFieldsAndPreservesLegacyExtras()
    {
        var settings = JsonSerializer.Deserialize<DesktopSettingsDocument>(
            """
            {
              "idioma":"ca",
              "themeIndex":2,
              "confirmToSign":true,
              "closeBehavior":"resident",
              "signFormat":"pades",
              "signStrictCompat":true,
              "signVisibleSeal":false,
              "facturaeToolsEnabled":true,
              "preferDefaultCertificate":true,
              "defaultCertificateId":"cert-default",
              "proxyEnabled":true,
              "proxyType":"manual",
              "proxyHost":"proxy.example",
              "proxyPort":8443,
              "proxyExcludedUrls":["localhost","*.example"],
              "proxySecretId":"secret-ref",
              "proxyRealm":"corp-proxy",
              "tema":"oscuro"
            }
            """);

        Assert.IsNotNull(settings);
        Assert.AreEqual("ca", settings!.Language);
        Assert.AreEqual(2, settings.ThemeIndex);
        Assert.AreEqual(true, settings.ConfirmBeforeSigning);
        Assert.AreEqual("resident", settings.CloseBehavior);
        Assert.AreEqual("pades", settings.DefaultSignatureFormat);
        Assert.AreEqual(true, settings.FacturaeToolsEnabled);
        Assert.AreEqual(true, settings.PreferDefaultCertificate);
        Assert.AreEqual("cert-default", settings.DefaultCertificateId);
        Assert.AreEqual(8443, settings.ProxyPort);
        CollectionAssert.AreEqual(
            new[] { "localhost", "*.example" },
            settings.ProxyExcludedUrls!.ToArray());
        Assert.AreEqual("secret-ref", settings.ProxySecretId);
        Assert.AreEqual("corp-proxy", settings.ProxyRealm);
        Assert.AreEqual(
            "oscuro",
            settings.AdditionalSettings["tema"].GetString());
    }

    [TestMethod]
    public void SafeSnapshot_DropsSecretsEphemeralStateAndNestedPayloads()
    {
        var settings = JsonSerializer.Deserialize<DesktopSettingsDocument>(
            """
            {
              "idioma":" es ",
              "themeIndex":0,
              "tema":"oscuro",
              "preferDefaultCertificate":true,
              "defaultCertificateId":"cert-default",
              "certificateTypeFilter":["fisica","representante"],
              "proxySecretId":"secret-ref",
              "proxyRealm":"realm",
              "proxyUsername":"legacy-user",
              "proxyPassword":"clear",
              "securityAccessPassword":"clear",
              "webCompatibilityActive":true,
              "webCompatibilityExpiresAt":"2030-01-01T00:00:00Z",
              "inventedScalar":"not-preserved",
              "inventedNested":{"password":"clear"}
            }
            """);

        var safe = settings!.CreateSafeSaveSnapshot();

        Assert.AreEqual("es", safe.Language);
        Assert.AreEqual(true, safe.PreferDefaultCertificate);
        Assert.AreEqual("cert-default", safe.DefaultCertificateId);
        Assert.IsNull(safe.ProxySecretId);
        Assert.IsNull(safe.ProxyRealm);
        Assert.IsTrue(safe.AdditionalSettings.ContainsKey("tema"));
        Assert.IsTrue(
            safe.AdditionalSettings.ContainsKey("certificateTypeFilter"));
        foreach (var forbidden in new[]
        {
            "proxyUsername",
            "proxyPassword",
            "securityAccessPassword",
            "webCompatibilityActive",
            "webCompatibilityExpiresAt",
            "inventedScalar",
            "inventedNested",
        })
        {
            Assert.IsFalse(
                safe.AdditionalSettings.ContainsKey(forbidden),
                forbidden);
        }
        using var serialized = JsonDocument.Parse(
            JsonSerializer.Serialize(safe, WireOptions));
        Assert.IsFalse(
            serialized.RootElement.TryGetProperty(
                "proxySecretId",
                out _));
        Assert.IsFalse(
            serialized.RootElement.TryGetProperty(
                "proxyRealm",
                out _));
    }

    [TestMethod]
    public void SafeSnapshot_TypesAndBoundsDefaultCertificatePreference()
    {
        var oversizedId = new string('I', 1_100);
        using var extra = JsonDocument.Parse(
            """
            {
              "preferDefaultCertificate":false,
              "defaultCertificateId":"extension-id",
              "preferredCertificateId":"cert-last"
            }
            """);
        var settings = new DesktopSettingsDocument
        {
            PreferDefaultCertificate = true,
            DefaultCertificateId = $"  {oversizedId}\u0000  ",
            AdditionalSettings = extra.RootElement
                .EnumerateObject()
                .ToDictionary(
                    property => property.Name,
                    property => property.Value.Clone(),
                    StringComparer.Ordinal),
        };

        var safe = settings.CreateSafeSaveSnapshot();
        using var serialized = JsonDocument.Parse(
            JsonSerializer.Serialize(safe, WireOptions));

        Assert.AreEqual(true, safe.PreferDefaultCertificate);
        Assert.AreEqual(1024, safe.DefaultCertificateId!.Length);
        Assert.IsFalse(safe.DefaultCertificateId.Any(char.IsControl));
        Assert.IsFalse(
            safe.AdditionalSettings.ContainsKey(
                "preferDefaultCertificate"));
        Assert.IsFalse(
            safe.AdditionalSettings.ContainsKey(
                "defaultCertificateId"));
        Assert.IsTrue(
            safe.AdditionalSettings.ContainsKey(
                "preferredCertificateId"));
        Assert.AreEqual(
            1,
            serialized.RootElement
                .EnumerateObject()
                .Count(property =>
                    property.NameEquals("defaultCertificateId")));
    }

    [TestMethod]
    public void SafeSnapshot_BoundsKnownLegacyValues()
    {
        var oversizedString = new string('x', 8_193);
        var oversizedArrayItem = new string('y', 2_049);
        var settings = JsonSerializer.Deserialize<DesktopSettingsDocument>(
            $$"""
            {
              "signReason":{{JsonSerializer.Serialize(oversizedString)}},
              "certificateTypeFilter":[
                {{JsonSerializer.Serialize(oversizedArrayItem)}}
              ],
              "tsaEnabled":true
            }
            """);

        var safe = settings!.CreateSafeSaveSnapshot();

        Assert.IsFalse(safe.AdditionalSettings.ContainsKey("signReason"));
        Assert.IsFalse(
            safe.AdditionalSettings.ContainsKey("certificateTypeFilter"));
        Assert.IsTrue(safe.AdditionalSettings.ContainsKey("tsaEnabled"));
    }

    [TestMethod]
    public void SafeSnapshot_NormalizesProxyExclusionsWithoutSecretMetadata()
    {
        var settings = new DesktopSettingsDocument
        {
            ProxyExcludedUrls =
            [
                " localhost ",
                "localhost",
                "*.organizacion.es",
                "invalid\nvalue",
                new string(
                    'x',
                    DesktopSettingsDocument
                        .MaximumProxyExcludedUrlCharacters + 1),
            ],
            ProxySecretId = "secret-ref",
            ProxyRealm = "corp-proxy",
        };

        var safe = settings.CreateSafeSaveSnapshot();

        CollectionAssert.AreEqual(
            new[] { "localhost", "*.organizacion.es" },
            safe.ProxyExcludedUrls!.ToArray());
        Assert.IsNull(safe.ProxySecretId);
        Assert.IsNull(safe.ProxyRealm);
    }

    [TestMethod]
    public void SafeSnapshot_BoundsTheAggregatePreservedDocument()
    {
        var largeValue = JsonSerializer.SerializeToElement(
            new string('\u0001', 8_192));
        var safeNames = new[]
        {
            "signReason",
            "signLocation",
            "signContactInfo",
            "facturaePolicyVersion",
            "policyIdentifierHash",
            "signerClaimedRole",
            "signatureProductionCity",
            "signatureProductionProvince",
            "signatureProductionPostalCode",
            "signatureProductionCountry",
            "padesPolicyIdentifierHash",
            "padesPolicyQualifier",
        };
        var settings = new DesktopSettingsDocument
        {
            AdditionalSettings = safeNames.ToDictionary(
                name => name,
                _ => largeValue.Clone(),
                StringComparer.Ordinal),
        };

        var safe = settings.CreateSafeSaveSnapshot();

        Assert.IsTrue(
            safe.AdditionalSettings.Count < safeNames.Length);
        Assert.IsTrue(
            JsonSerializer.SerializeToUtf8Bytes(safe).Length <
                600 * 1024);
    }

    [TestMethod]
    public void Enumerations_MatchCurrentQtAndGoContract()
    {
        foreach (var language in new[]
        {
            "es", "ca", "va", "eu", "gl", "en",
            "de", "fr", "pt", "it", "zh",
        })
        {
            Assert.IsTrue(
                DesktopSettingsDocument.IsSupportedLanguage(language),
                language);
        }
        Assert.IsFalse(
            DesktopSettingsDocument.IsSupportedLanguage("invented"));

        foreach (var format in new[]
        {
            "", "pades", "cades", "xades", "xmldsig",
            "odf", "ooxml", "facturae", "asic-xades",
        })
        {
            Assert.IsTrue(
                DesktopSettingsDocument.IsSupportedSignatureFormat(format),
                format);
        }
        Assert.IsFalse(
            DesktopSettingsDocument.IsSupportedSignatureFormat("pdf"));
        Assert.IsTrue(
            DesktopSettingsDocument.IsSupportedCloseBehavior("exit"));
        Assert.IsTrue(
            DesktopSettingsDocument.IsSupportedCloseBehavior("resident"));
        Assert.IsTrue(
            DesktopSettingsDocument.IsSupportedProxyType("none"));
        Assert.IsTrue(
            DesktopSettingsDocument.IsSupportedProxyType("manual"));
        Assert.IsFalse(
            DesktopSettingsDocument.IsSupportedTheme(3));
    }

    private static DesktopSettingsDocument NewSettings()
    {
        using var extra = JsonDocument.Parse(
            """{"tema":"oscuro"}""");
        return new DesktopSettingsDocument
        {
            Language = "es",
            ThemeIndex = 1,
            ConfirmBeforeSigning = true,
            CloseBehavior = "exit",
            DefaultSignatureFormat = "pades",
            StrictSignatureCompatibility = true,
            VisiblePdfSeal = false,
            SealLogoOpacityPercent = 30,
            FacturaeToolsEnabled = false,
            PreferDefaultCertificate = true,
            DefaultCertificateId = "cert-default",
            ProxyEnabled = true,
            ProxyType = "manual",
            ProxyHost = "proxy.example",
            ProxyPort = 8080,
            ProxyExcludedUrls =
                ["localhost", "127.0.0.1", "*.organizacion.es"],
            AdditionalSettings = new Dictionary<string, JsonElement>(
                StringComparer.Ordinal)
            {
                ["tema"] = extra.RootElement
                    .GetProperty("tema")
                    .Clone(),
            },
        };
    }

    private sealed class RecordingIpcClient : IIpcClient
    {
        public IpcHello? ServerHello => null;
        public List<RecordedCall> Calls { get; } = [];

        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
            string action,
            TParameters parameters,
            CancellationToken cancellationToken = default)
        {
            Calls.Add(new(
                action,
                parameters!,
                typeof(TData),
                cancellationToken));
            return Task.FromResult(new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = "request-settings",
                TraceId = "trace-settings",
                Action = action,
                IsSuccess = true,
                Outcome = "success",
                Phase = "operation",
                Retryable = false,
            });
        }

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }

    private sealed record RecordedCall(
        string Action,
        object Parameters,
        Type DataType,
        CancellationToken CancellationToken);
}
