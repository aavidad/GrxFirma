// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Reflection;
using System.Text.Json;
using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DesktopOperationsClientTests
{
    private static readonly JsonSerializerOptions WireOptions = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    [TestMethod]
    public void PublicCertificateExport_UsesExactJsonNames()
    {
        var parameters = new CertificateExportPublicParameters
        {
            CertificateId = "mio",
            OutputPath = @"C:\docs\mio.cer",
            Format = "der",
        };
        using var request = JsonDocument.Parse(JsonSerializer.Serialize(parameters));
        Assert.AreEqual("mio", request.RootElement.GetProperty("certificateId").GetString());
        Assert.AreEqual(@"C:\docs\mio.cer", request.RootElement.GetProperty("outputPath").GetString());
        Assert.AreEqual("der", request.RootElement.GetProperty("format").GetString());
        var response = JsonSerializer.Deserialize<CertificateExportPublicResult>(
            """{"certificateDerBase64":"AQID","outputPath":"mio.cer","format":"der","encryptionSuitable":true}""");
        Assert.IsNotNull(response);
        Assert.AreEqual("AQID", response.CertificateDerBase64);
        Assert.IsTrue(response.EncryptionSuitable);
    }

    [TestMethod]
    public async Task Methods_SendExactActionsAndParameterTypes()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);

        await client.GetCertificatesAsync();
        await client.ExportPublicCertificateAsync(new()
        {
            CertificateId = "cert-1",
            OutputPath = @"C:\docs\mi-certificado.cer",
            Format = "der",
        });
        await client.GetSmartcardStatusAsync();
        await client.ValidateCertificateOnlineAsync(
            new() { CertificateId = "cert-1" });
        await client.SignAsync(NewSignParameters());
        await client.SignMultiCosignAsync(NewSignParameters());
        await client.VerifyAsync(new() { InputPath = @"C:\docs\signed.pdf" });
        await client.GetPdfPreviewAsync(new()
        {
            Path = @"C:\docs\document.pdf",
            Page = 2,
        });
        await client.GetSealPreviewAsync(new()
        {
            CertificateId = "cert-1",
            QrContent = "https://sede.example/verificar",
            VisibleSeal = new()
            {
                Page = "all",
                PageWidth = 595,
                PageHeight = 842,
            },
        });
        await client.CreateHashAsync(new()
        {
            InputPath = @"C:\docs\document.pdf",
            Algorithm = "SHA-256",
            Format = "hex",
        });
        await client.CheckHashAsync(new()
        {
            InputPath = @"C:\docs\document.pdf",
            HashPath = @"C:\docs\document.sha256",
        });
        await client.GetProtectionRecipientsAsync();
        await client.ImportProtectionRecipientAsync(new() { Path = @"C:\docs\persona.cer" });
        await client.RemoveProtectionRecipientAsync(new() { Id = "x509-abc" });
        await client.ProtectAsync(NewProtectionParameters());
        await client.ProtectAndSignAsync(NewProtectionParameters());
        await client.UnprotectAsync(NewProtectionParameters());
        await client.PingAsync();
        await client.CheckCertificatesAsync();
        await client.GetCertificateAccessOptionsAsync();
        await client.GetCertificateManagersAsync();
        await client.OpenCertificateManagerAsync(
            new() { ManagerId = "windows-certmgr" });
        await client.GetProxySecretStoreStatusAsync();
        await client.StoreProxySecretAsync(new()
        {
            Realm = "corp-proxy",
            Username = "alberto",
            Password = "secreta"u8.ToArray(),
        });
        await client.DeleteProxySecretAsync();
        await client.GetTlsDiagnosticsAsync();
        await client.GetClockDiagnosticsAsync();
        await client.ExportDiagnosticSummaryAsync();
        await client.InstallPublicRootsAsync();
        await client.ClearTlsTrustAsync();

        CollectionAssert.AreEqual(
            new[]
            {
                "certificates",
                "certificate_export_public",
                "smartcard_status",
                "validate_certificate_online",
                "sign",
                "sign_multicosign",
                "verify",
                "pdf_preview",
                "seal_preview",
                "hash_create",
                "hash_check",
                "protection_recipients",
                "protection_recipient_import",
                "protection_recipient_remove",
                "protect",
                "protect_sign",
                "unprotect",
                "ping",
                "check_certificates",
                "certificate_access_options",
                "certificate_access_options",
                "open_certificate_manager",
                "proxy_secret_store_status",
                "proxy_secret_store",
                "proxy_secret_delete",
                "tls_diagnostics",
                "clock_diagnostics",
                "export_diagnostic",
                "install_public_roots",
                "clear_tls_trust",
            },
            transport.Calls.Select(call => call.Action).ToArray());
        CollectionAssert.AreEqual(
            new[]
            {
                typeof(CertificatesParameters),
                typeof(CertificateExportPublicParameters),
                typeof(SmartcardStatusParameters),
                typeof(ValidateCertificateOnlineParameters),
                typeof(SignParameters),
                typeof(SignParameters),
                typeof(VerifyParameters),
                typeof(PdfPreviewParameters),
                typeof(SealPreviewParameters),
                typeof(HashCreateParameters),
                typeof(HashCheckParameters),
                typeof(ProtectionRecipientsParameters),
                typeof(ProtectionRecipientImportParameters),
                typeof(ProtectionRecipientRemoveParameters),
                typeof(ProtectionParameters),
                typeof(ProtectionParameters),
                typeof(ProtectionParameters),
                typeof(PingParameters),
                typeof(CheckCertificatesParameters),
                typeof(CertificateAccessOptionsParameters),
                typeof(CertificateAccessOptionsParameters),
                typeof(OpenCertificateManagerParameters),
                typeof(ProxySecretStoreStatusParameters),
                typeof(ProxySecretStoreParameters),
                typeof(ProxySecretDeleteParameters),
                typeof(TlsDiagnosticsParameters),
                typeof(ClockDiagnosticsParameters),
                typeof(ExportDiagnosticParameters),
                typeof(InstallPublicRootsParameters),
                typeof(ClearTlsTrustParameters),
            },
            transport.Calls.Select(call => call.ParametersType).ToArray());
        CollectionAssert.AreEqual(
            new[]
            {
                typeof(IReadOnlyList<CertificateInfo>),
                typeof(CertificateExportPublicResult),
                typeof(SmartcardStatusResult),
                typeof(CertificateOnlineValidationResult),
                typeof(SignResult),
                typeof(SignResult),
                typeof(VerifyResult),
                typeof(PdfPreviewResult),
                typeof(SealPreviewResult),
                typeof(HashCreateResult),
                typeof(HashCheckResult),
                typeof(ProtectionRecipientsResult),
                typeof(ProtectionRecipientChangeResult),
                typeof(ProtectionRecipientChangeResult),
                typeof(ProtectionResult),
                typeof(ProtectionResult),
                typeof(UnprotectResult),
                typeof(PingResult),
                typeof(CheckCertificatesSummaryResult),
                typeof(CertificateAccessInventoryResult),
                typeof(CertificateManagerOptionsResult),
                typeof(string),
                typeof(ProxySecretStoreStatusResult),
                typeof(ProxySecretMutationResult),
                typeof(ProxySecretMutationResult),
                typeof(TlsStoreDiagnosticResult),
                typeof(ClockDiagnosticResult),
                typeof(DesktopDiagnosticSummaryResult),
                typeof(string),
                typeof(int),
            },
            transport.Calls.Select(call => call.DataType).ToArray());
    }

    [TestMethod]
    public async Task Result_PreservesTransportCorrelationAndCancellation()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        using var cancellation = new CancellationTokenSource();

        var result = await client.SignAsync(
            NewSignParameters(),
            cancellation.Token);

        Assert.AreSame(transport.LastResult, result);
        Assert.AreEqual("request-real-42", result.RequestId);
        Assert.AreEqual("trace-real-42", result.TraceId);
        Assert.AreEqual("sign", result.Action);
        Assert.AreEqual(cancellation.Token, transport.Calls.Single().CancellationToken);
    }

    [TestMethod]
    public async Task SignMultiCosign_SendsExactActionAndPreservesParameters()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        var parameters = NewSignParameters();

        await client.SignMultiCosignAsync(parameters);

        var call = transport.Calls.Single();
        Assert.AreEqual("sign_multicosign", call.Action);
        Assert.AreSame(parameters, call.Parameters);
        Assert.AreEqual(typeof(SignParameters), call.ParametersType);
        Assert.AreEqual(typeof(SignResult), call.DataType);
    }

    [TestMethod]
    public void RequestDtos_UseExactGoJsonNames()
    {
        Assert.AreEqual(
            "{}",
            JsonSerializer.Serialize(
                new CertificatesParameters(),
                WireOptions));

        using var validateCertificate = Serialize(
            new ValidateCertificateOnlineParameters
            {
                CertificateId = "cert-1",
            });
        AssertPropertyNames(
            validateCertificate.RootElement,
            "certificateId");

        using var sign = Serialize(NewSignParameters());
        AssertPropertyNames(
            sign.RootElement,
            "inputPath",
            "outputPath",
            "certificateId",
            "certificateIndex",
            "additionalCertificateIds",
            "format",
            "action",
            "overwrite",
            "saveToDisk",
            "returnSignatureB64",
            "visibleSeal",
            "allowInvalidPDF",
            "strictCompat",
            "qrContent",
            "reason",
            "location",
            "contactInfo",
            "extraOptions");
        AssertPropertyNames(
            sign.RootElement.GetProperty("visibleSeal"),
            "page",
            "x",
            "y",
            "w",
            "h",
            "pageWidth",
            "pageHeight",
            "rotation",
            "keepText",
            "imagePath",
            "logoOpacityPercent");

        using var verify = Serialize(new VerifyParameters
        {
            InputPath = "signed.pdf",
            OriginalPath = "original.pdf",
        });
        AssertPropertyNames(
            verify.RootElement,
            "inputPath",
            "originalPath");

        using var pdfPreview = Serialize(new PdfPreviewParameters
        {
            Path = "document.pdf",
            Page = 2,
        });
        AssertPropertyNames(
            pdfPreview.RootElement,
            "path",
            "page");

        using var createHash = Serialize(new HashCreateParameters
        {
            InputPath = "document.pdf",
            OutputPath = "document.sha256",
            Algorithm = "SHA-256",
            Format = "hex",
            Recursive = true,
        });
        AssertPropertyNames(
            createHash.RootElement,
            "inputPath",
            "outputPath",
            "algorithm",
            "format",
            "recursive");

        using var checkHash = Serialize(new HashCheckParameters
        {
            InputPath = "document.pdf",
            HashPath = "document.sha256",
            OutputPath = "report.json",
            Algorithm = "SHA-256",
            Recursive = true,
            SaveReportToDisk = true,
        });
        AssertPropertyNames(
            checkHash.RootElement,
            "inputPath",
            "hashPath",
            "outputPath",
            "algorithm",
            "recursive",
            "saveReportToDisk");

        Assert.AreEqual(
            "{}",
            JsonSerializer.Serialize(
                new ProtectionRecipientsParameters(),
                WireOptions));
        foreach (var parameters in new object[]
        {
            new PingParameters(),
            new CheckCertificatesParameters(),
            new CertificateAccessOptionsParameters(),
            new ProxySecretStoreStatusParameters(),
            new ProxySecretDeleteParameters(),
        })
        {
            Assert.AreEqual(
                "{}",
                JsonSerializer.Serialize(
                    parameters,
                    parameters.GetType(),
                    WireOptions));
        }

        using var proxySecret = Serialize(
            new ProxySecretStoreParameters
            {
                Realm = "corp-proxy",
                Username = "alberto",
                Password = "secreta"u8.ToArray(),
            });
        AssertPropertyNames(
            proxySecret.RootElement,
            "realm",
            "username",
            "password");
        Assert.AreEqual(
            Convert.ToBase64String("secreta"u8),
            proxySecret.RootElement
                .GetProperty("password")
                .GetString());

        using var openCertificateManager = Serialize(
            new OpenCertificateManagerParameters
            {
                ManagerId = "windows-certmgr",
            });
        AssertPropertyNames(
            openCertificateManager.RootElement,
            "managerId");

        using var protection = Serialize(NewProtectionParameters());
        AssertPropertyNames(
            protection.RootElement,
            "inputPath",
            "outputPath",
            "certificateId",
            "certificateIndex",
            "profile",
            "recipientIds",
            "overwrite",
            "saveToDisk",
            "returnProtectedB64",
            "returnUnprotectedB64",
            "options",
            "secretB64");
        Assert.AreEqual(
            "authenvelopeddata",
            protection.RootElement
                .GetProperty("options")
                .GetProperty("container")
                .GetString());

        using var minimalSign = Serialize(new SignParameters
        {
            InputPath = "document.pdf",
        });
        AssertPropertyNames(
            minimalSign.RootElement,
            "inputPath",
            "outputPath",
            "certificateIndex",
            "format",
            "action",
            "overwrite",
            "saveToDisk",
            "returnSignatureB64",
            "allowInvalidPDF",
            "strictCompat");

        using var minimalHashCheck = Serialize(new HashCheckParameters
        {
            InputPath = "document.pdf",
            HashPath = "document.sha256",
        });
        AssertPropertyNames(
            minimalHashCheck.RootElement,
            "inputPath",
            "hashPath");
    }

    [TestMethod]
    public void SealPreview_UsesExactGoJsonNames()
    {
        AssertJsonPropertyNames<SealPreviewResult>("image");
        using var parameters = Serialize(new SealPreviewParameters
        {
            CertificateId = "cert-1",
            QrContent = "https://sede.example/verificar",
            ExtraOptions = new Dictionary<string, string>
            {
                ["visibleSealLogo"] = "institucional",
            },
            VisibleSeal = new()
            {
                Page = "1",
                PageWidth = 595,
                PageHeight = 842,
                Rotation = 15,
                LogoOpacityPercent = 30,
            },
        });
        AssertPropertyNames(
            parameters.RootElement,
            "certificateId",
            "visibleSeal",
            "qrContent",
            "extraOptions");
        Assert.AreEqual(
            30,
            parameters.RootElement.GetProperty("visibleSeal")
                .GetProperty("logoOpacityPercent").GetInt32());

        var result = JsonSerializer.Deserialize<SealPreviewResult>(
            """{"image":"iVBORw0KGgo="}""");
        CollectionAssert.AreEqual(
            new byte[] { 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A },
            result!.Image);
    }

    [TestMethod]
    public void PdfPreviewResult_UsesExactGoJsonNames()
    {
        AssertJsonPropertyNames<PdfPreviewResult>(
            "data",
            "width",
            "height",
            "currentPage",
            "totalPages");

        var result = JsonSerializer.Deserialize<PdfPreviewResult>(
            """
            {
              "data":"iVBORw0KGgo=",
              "width":612.0,
              "height":792.0,
              "currentPage":2,
              "totalPages":5
            }
            """);

        Assert.IsNotNull(result);
        CollectionAssert.AreEqual(
            Convert.FromBase64String("iVBORw0KGgo="),
            result.Data);
        Assert.AreEqual(612.0, result.Width);
        Assert.AreEqual(792.0, result.Height);
        Assert.AreEqual(2, result.CurrentPage);
        Assert.AreEqual(5, result.TotalPages);
    }

    [TestMethod]
    public void ResultDtos_DeserializeExactGoContracts()
    {
        const string certificateJson =
            """
            [{
              "id":"cert-1",
              "subject":"CN=Persona",
              "subjectName":"Persona",
              "issuer":"CN=Emisor",
              "issuerName":"Emisor",
              "notAfter":"2027-01-01T00:00:00Z",
              "validTo":"2027-01-01T00:00:00Z",
              "fingerprint":"AA:BB",
              "serialNumber":"123",
              "status":"Válido",
              "canSign":true,
              "tipo":"fisica",
              "organizacion":"Ejemplo",
              "nif":"12345678Z",
              "caducado":false,
              "diasCaducidad":120
            }]
            """;
        var certificates = JsonSerializer.Deserialize<
            IReadOnlyList<CertificateInfo>>(certificateJson);

        Assert.IsNotNull(certificates);
        Assert.AreEqual("cert-1", certificates![0].Id);
        Assert.AreEqual("Persona", certificates[0].SubjectName);
        Assert.AreEqual(120, certificates[0].DaysUntilExpiration);

        var smartcards = JsonSerializer.Deserialize<SmartcardStatusResult>(
            """{"readers":[{"name":"Lector de prueba","present":true,"isDnie":true}]}""");
        Assert.IsNotNull(smartcards);
        Assert.AreEqual("Lector de prueba", smartcards.Readers[0].Name);
        Assert.IsTrue(smartcards.Readers[0].Present);
        Assert.IsTrue(smartcards.Readers[0].IsDnie);
        Assert.AreEqual(
            """{"readers":[{"name":"Lector de prueba","present":true,"isDnie":true}]}""",
            JsonSerializer.Serialize(smartcards));
        Assert.AreEqual("{}", JsonSerializer.Serialize(new SmartcardStatusParameters()));

        var sign = JsonSerializer.Deserialize<SignResult>(
            """{"OutputPath":"C:\\signed.pdf","signature_b64":"U0lHTg=="}""");
        Assert.IsNotNull(sign);
        Assert.AreEqual(@"C:\signed.pdf", sign!.OutputPath);
        Assert.AreEqual("U0lHTg==", sign.SignatureBase64);

        const string verifyJson =
            """
            {
              "valid":true,
              "reason":"Firma válida",
              "details":["Integridad correcta"],
              "signers":["CN=Persona"],
              "format":"PAdES",
              "coverage":"whole_document",
              "integrity":{"status":"valid","reason":"Correcta","details":[]},
              "certificate":{"status":"valid","reason":"Vigente","details":[]},
              "trust":{"status":"indeterminate","reason":"Sin raíz","details":[]},
              "signerSummaries":[{
                "id":"signer-1",
                "subject":"CN=Persona",
                "issuer":"CN=Emisor",
                "fingerprint":"AA:BB"
              }],
              "warnings":["No se comprobó OCSP"],
              "errors":[],
              "evidence":[{"type":"signature","summary":"CMS correcto"}]
            }
            """;
        var verify = JsonSerializer.Deserialize<VerifyResult>(verifyJson);
        Assert.IsNotNull(verify);
        Assert.IsTrue(verify!.IsValid);
        Assert.AreEqual("indeterminate", verify.Trust.Status);
        Assert.AreEqual("signer-1", verify.SignerSummaries.Single().Id);

        var createdFile = JsonSerializer.Deserialize<HashCreateResult>(
            """
            {"algorithm":"SHA-256","format":"hex","hash":"AABB","outputPath":"doc.sha256"}
            """);
        var createdDirectory = JsonSerializer.Deserialize<HashCreateResult>(
            """
            {
              "algorithm":"SHA-256",
              "format":"sfv",
              "entries":3,
              "recursive":true,
              "outputPath":"manifest.sfv",
              "manifestBase64":"TUFOSUZFU1Q="
            }
            """);
        Assert.AreEqual("AABB", createdFile!.Hash);
        Assert.AreEqual(3, createdDirectory!.Entries);
        Assert.AreEqual(true, createdDirectory.Recursive);

        var checkedFile = JsonSerializer.Deserialize<HashCheckResult>(
            """
            {
              "valid":true,
              "algorithm":"SHA-256",
              "format":"hex",
              "expectedHash":"AABB",
              "actualHash":"AABB"
            }
            """);
        var checkedDirectory = JsonSerializer.Deserialize<HashCheckResult>(
            """
            {
              "valid":false,
              "algorithm":"SHA-256",
              "recursive":true,
              "matching_hash":["a.txt"],
              "not_matching_hash":["b.txt"],
              "hash_without_file":["c.txt"],
              "file_without_hash":["d.txt"],
              "reportBase64":"UkVQT1JU",
              "reportOutputPath":"report.json"
            }
            """);
        Assert.IsTrue(checkedFile!.IsValid);
        Assert.AreEqual("AABB", checkedFile.ExpectedHash);
        Assert.AreEqual("b.txt", checkedDirectory!.NotMatchingHash.Single());
        Assert.AreEqual("report.json", checkedDirectory.ReportOutputPath);

        var recipients = JsonSerializer.Deserialize<ProtectionRecipientsResult>(
            """
            {
              "recipients":[{
                "id":"recipient-1",
                "label":"Persona destinataria",
                "profile":"compat",
                "algorithm":"rsa-oaep",
                "authEnvelopedDataCompatible":true
              }]
            }
            """);
        var protectedResult = JsonSerializer.Deserialize<ProtectionResult>(
            """
            {
              "outputPath":"C:\\docs\\protected.authenveloped.p7m",
              "profile":"compat-rsa-oaep-aes256gcm",
              "recipientCount":1,
              "documentName":"protected.authenveloped.p7m",
              "mimeType":"application/pkcs7-mime",
              "certificateId":"cert-1"
            }
            """);
        var unprotectedResult = JsonSerializer.Deserialize<UnprotectResult>(
            """
            {
              "outputPath":"C:\\docs\\document-unprotected.pdf",
              "profile":"compat-rsa-oaep-aes256gcm",
              "recipientId":"recipient-1",
              "documentName":"document.pdf",
              "mimeType":"application/pdf"
            }
            """);

        Assert.AreEqual(
            "recipient-1",
            recipients!.VisibleRecipients.Single().Id);
        Assert.IsTrue(
            recipients.VisibleRecipients.Single()
                .AuthEnvelopedDataCompatible);
        Assert.AreEqual(1, protectedResult!.RecipientCount);
        Assert.AreEqual("cert-1", protectedResult.CertificateId);
        Assert.AreEqual(
            "recipient-1",
            unprotectedResult!.RecipientId);
        Assert.AreEqual("document.pdf", unprotectedResult.DocumentName);

        var certificateCounts =
            JsonSerializer.Deserialize<CheckCertificatesSummaryResult>(
                """
                {
                  "certificates":[{
                    "id":"must-not-be-retained",
                    "subject":"CN=Must not be retained"
                  }],
                  "okCount":2,
                  "failCount":1
                }
                """);
        var certificateAccess =
            JsonSerializer.Deserialize<CertificateAccessInventoryResult>(
                """
                {
                  "managers":[{
                    "id":"must-not-be-retained",
                    "label":"Must not be retained"
                  }],
                  "importTargets":[{
                    "id":"must-not-be-retained",
                    "label":"Must not be retained",
                    "browser":"must-not-be-retained"
                  }],
                  "detectedBrowser":"must-not-be-retained",
                  "preferredManager":"must-not-be-retained"
                }
                """);
        var certificateManagers =
            JsonSerializer.Deserialize<CertificateManagerOptionsResult>(
                """
                {
                  "managers":[{
                    "id":"windows-certmgr",
                    "label":"Gestor de certificados de Windows",
                    "recommended":true
                  },{
                    "id":"",
                    "label":"Entrada inválida"
                  }],
                  "importTargets":[{
                    "id":"must-not-be-retained",
                    "label":"Must not be retained"
                  }],
                  "preferredManager":"windows-certmgr"
                }
                """);
        var onlineValidation =
            JsonSerializer.Deserialize<CertificateOnlineValidationResult>(
                """
                {
                  "status":"valid",
                  "userMessage":"El certificado no está revocado.",
                  "reason":"OCSP correcto",
                  "method":"ocsp",
                  "checkedAt":"2026-07-27T08:00:00Z",
                  "revokedAt":"",
                  "ocspUrl":"https://must-not-be-retained.invalid",
                  "crlUrl":"https://must-not-be-retained.invalid"
                }
                """);
        var proxyStore =
            JsonSerializer.Deserialize<ProxySecretStoreStatusResult>(
                """
                {
                  "available":true,
                  "platform":"windows",
                  "backend":"dpapi",
                  "reason":"",
                  "runtimeProxyMode":"manual-secure-store"
                }
                """);

        Assert.AreEqual(2, certificateCounts!.OkCount);
        Assert.AreEqual(1, certificateCounts.FailCount);
        Assert.IsTrue(certificateCounts.HasCoherentCounts);
        Assert.AreEqual(1, certificateAccess!.VisibleManagerCount);
        Assert.AreEqual(1, certificateAccess.VisibleImportTargetCount);
        Assert.AreEqual(
            "windows-certmgr",
            certificateManagers!.Managers.Single().Id);
        Assert.AreEqual(
            "windows-certmgr",
            certificateManagers.PreferredManager);
        Assert.AreEqual("valid", onlineValidation!.Status);
        Assert.AreEqual("ocsp", onlineValidation.Method);
        Assert.IsTrue(proxyStore!.Available);
        Assert.AreEqual(
            "manual-secure-store",
            proxyStore.RuntimeMode);
    }

    [TestMethod]
    public void ResultDtos_DeclareExactGoJsonNames()
    {
        AssertJsonPropertyNames<CertificateInfo>(
            "id",
            "subject",
            "subjectName",
            "issuer",
            "issuerName",
            "notAfter",
            "validTo",
            "fingerprint",
            "serialNumber",
            "status",
            "canSign",
            "needsUnlock",
            "tipo",
            "organizacion",
            "nif",
            "caducado",
            "diasCaducidad",
            "remote",
            "remotePin",
            "remoteOtp",
            "remoteOtpOnline");
        AssertJsonPropertyNames<ValidateCertificateOnlineParameters>(
            "certificateId");
        AssertJsonPropertyNames<CertificateOnlineValidationResult>(
            "status",
            "userMessage",
            "reason",
            "method",
            "checkedAt",
            "revokedAt");
        AssertJsonPropertyNames<OpenCertificateManagerParameters>(
            "managerId");
        AssertJsonPropertyNames<SignResult>(
            "OutputPath",
            "signature_b64",
            "format");
        AssertJsonPropertyNames<VerifyResult>(
            "valid",
            "reason",
            "details",
            "signers",
            "format",
            "coverage",
            "integrity",
            "certificate",
            "trust",
            "signerSummaries",
            "warnings",
            "errors",
            "evidence");
        AssertJsonPropertyNames<VerifyAspect>(
            "status",
            "reason",
            "details");
        AssertJsonPropertyNames<VerifySignerSummary>(
            "id",
            "subject",
            "issuer",
            "fingerprint");
        AssertJsonPropertyNames<VerifyEvidence>(
            "type",
            "summary");
        AssertJsonPropertyNames<HashCreateResult>(
            "algorithm",
            "format",
            "hash",
            "outputPath",
            "entries",
            "recursive",
            "manifestBase64");
        AssertJsonPropertyNames<HashCheckResult>(
            "valid",
            "algorithm",
            "format",
            "expectedHash",
            "actualHash",
            "recursive",
            "matching_hash",
            "not_matching_hash",
            "hash_without_file",
            "file_without_hash",
            "reportBase64",
            "reportOutputPath");
        AssertJsonPropertyNames<ProtectionRecipientInfo>(
            "id",
            "label",
            "origin",
            "profile",
            "algorithm",
            "authEnvelopedDataCompatible");
        AssertJsonPropertyNames<ProtectionRecipientsResult>(
            "recipients");
        AssertJsonPropertyNames<ProtectionRecipientImportParameters>("path");
        AssertJsonPropertyNames<ProtectionRecipientRemoveParameters>("id");
        AssertJsonPropertyNames<ProtectionRecipientChangeResult>("id");
        using var publicImport = Serialize(new ProtectionRecipientImportParameters
        {
            Path = @"C:\docs\persona.cer",
        });
        AssertPropertyNames(publicImport.RootElement, "path");
        Assert.AreEqual(@"C:\docs\persona.cer", publicImport.RootElement.GetProperty("path").GetString());
        using var publicRemove = Serialize(new ProtectionRecipientRemoveParameters
        {
            Id = "x509-abc",
        });
        AssertPropertyNames(publicRemove.RootElement, "id");
        Assert.AreEqual("x509-abc", publicRemove.RootElement.GetProperty("id").GetString());
        AssertJsonPropertyNames<ProtectionParameters>(
            "inputPath",
            "outputPath",
            "certificateId",
            "certificateIndex",
            "profile",
            "recipientIds",
            "overwrite",
            "saveToDisk",
            "returnProtectedB64",
            "returnUnprotectedB64",
            "options",
            "secretB64");
        AssertJsonPropertyNames<ProtectionResult>(
            "outputPath",
            "protectedContentBase64",
            "profile",
            "recipientCount",
            "documentName",
            "mimeType",
            "certificateId");
        AssertJsonPropertyNames<UnprotectResult>(
            "outputPath",
            "unprotectedContentBase64",
            "profile",
            "recipientId",
            "documentName",
            "mimeType");
        AssertJsonPropertyNames<CheckCertificatesSummaryResult>(
            "okCount",
            "failCount");
        AssertJsonPropertyNames<CertificateAccessInventoryResult>(
            "managers",
            "importTargets");
        AssertJsonPropertyNames<CertificateManagerInfo>(
            "id",
            "label",
            "recommended");
        AssertJsonPropertyNames<CertificateManagerOptionsResult>(
            "managers",
            "preferredManager");
        AssertJsonPropertyNames<ProxySecretStoreStatusResult>(
            "available",
            "platform",
            "backend",
            "reason",
            "runtimeProxyMode");
        AssertJsonPropertyNames<ProxySecretStoreParameters>(
            "realm",
            "username",
            "password");
        AssertJsonPropertyNames<ProxySecretMutationResult>(
            "configured",
            "realm",
            "username",
            "rotated");
        AssertJsonPropertyNames<TlsStoreDiagnosticResult>(
            "state",
            "artifactCount",
            "certificateCount",
            "keyCount");
        AssertJsonPropertyNames<ClockDiagnosticStepResult>(
            "code",
            "label",
            "status",
            "owner",
            "userMessage",
            "suggestedAction",
            "evidenceRef");
        AssertJsonPropertyNames<ClockDiagnosticResult>(
            "thresholdSeconds",
            "steps");
        AssertJsonPropertyNames<DesktopDiagnosticSummaryResult>(
            "certificates",
            "canSign",
            "tlsStore");
    }

    [TestMethod]
    public void ClockDiagnosticContracts_RequireExactBoundedPhases()
    {
        ClockDiagnosticStepResult Step(string code) => new()
        {
            Code = code,
            Label = new string('x', 500),
            Status = DiagnosticStepStatus.Unknown,
            Owner = "unknown",
            UserMessage = new string('m', 1000),
            SuggestedAction = new string('a', 1000),
            EvidenceRef = new string('e', 500),
        };

        var coherent = new ClockDiagnosticResult
        {
            ThresholdSeconds = 5,
            Steps =
            [
                Step("local_clock"),
                Step("remote_clock"),
                Step("government_afirma"),
            ],
        };

        Assert.IsTrue(coherent.IsCoherent);
        Assert.AreEqual(160, coherent.Steps[0].Label.Length);
        Assert.AreEqual(512, coherent.Steps[0].UserMessage.Length);
        Assert.AreEqual(160, coherent.Steps[0].EvidenceRef.Length);
        Assert.IsFalse((coherent with
        {
            Steps =
            [
                Step("local_clock"),
                Step("remote_clock"),
                Step("remote_clock"),
            ],
        }).IsCoherent);
        Assert.IsFalse((coherent with
        {
            ThresholdSeconds = 0,
        }).IsCoherent);
    }

    [TestMethod]
    public void TlsDiagnosticContracts_RejectIncoherentCountsAndStates()
    {
        var coherent = new TlsStoreDiagnosticResult
        {
            State = "available",
            ArtifactCount = 3,
            CertificateCount = 2,
            KeyCount = 1,
        };
        Assert.IsTrue(coherent.IsCoherent);
        Assert.IsTrue(new DesktopDiagnosticSummaryResult
        {
            CertificateCount = 4,
            CanSignCount = 2,
            TlsStore = coherent,
        }.IsCoherent);

        Assert.IsFalse((coherent with
        {
            State = "unexpected",
        }).IsCoherent);
        Assert.IsFalse((coherent with
        {
            CertificateCount = 4,
        }).IsCoherent);
        Assert.IsFalse(new DesktopDiagnosticSummaryResult
        {
            CertificateCount = 1,
            CanSignCount = 2,
            TlsStore = coherent,
        }.IsCoherent);
    }

    [TestMethod]
    public async Task ProxySecretStore_UsesBinaryPasswordAndAlwaysZeroesIt()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        var password = "contraseña-segura"u8.ToArray();

        await client.StoreProxySecretAsync(new()
        {
            Realm = "  corp-proxy  ",
            Username = "  alberto  ",
            Password = password,
        });

        Assert.IsTrue(password.All(value => value == 0));
        var sent = (ProxySecretStoreParameters)
            transport.Calls.Single().Parameters;
        Assert.AreEqual("corp-proxy", sent.Realm);
        Assert.AreEqual("alberto", sent.Username);
        Assert.AreSame(password, sent.Password);
    }

    [TestMethod]
    public async Task ProxySecretStore_RejectsExactBackendLimitsAndZeroes()
    {
        var client = new DesktopOperationsClient(
            new RecordingIpcClient());
        foreach (var invalid in new[]
        {
            Array.Empty<byte>(),
            Enumerable.Repeat(
                (byte)'x',
                DesktopOperationsClient.MaximumPasswordBytes + 1)
                .ToArray(),
            new byte[] { 0xff },
            new byte[] { (byte)'a', (byte)'\n' },
        })
        {
            await Assert.ThrowsExactlyAsync<ArgumentException>(
                async () => await client.StoreProxySecretAsync(new()
                {
                    Realm = "corp-proxy",
                    Username = "alberto",
                    Password = invalid,
                }));
            Assert.IsTrue(invalid.All(value => value == 0));
        }

        var password = "secreta"u8.ToArray();
        await Assert.ThrowsExactlyAsync<ArgumentException>(
            async () => await client.StoreProxySecretAsync(new()
            {
                Realm = new string(
                    'x',
                    DesktopOperationsClient
                        .MaximumProxyCredentialTextBytes + 1),
                Username = "alberto",
                Password = password,
            }));
        Assert.IsTrue(password.All(value => value == 0));
    }

    [TestMethod]
    public void VisibleResultText_IsBoundedAndControlCharactersAreRemoved()
    {
        var longSubject = new string('A', 700);
        var certificate = JsonSerializer.Deserialize<CertificateInfo>(
            $$"""
            {
              "id":"stable-id",
              "subject":"{{longSubject}}\u0000",
              "subjectName":"Nombre\u0001",
              "issuer":"Emisor",
              "issuerName":"Emisor",
              "fingerprint":"AA",
              "canSign":true,
              "tipo":"fisica",
              "caducado":false,
              "diasCaducidad":1
            }
            """);

        Assert.IsNotNull(certificate);
        Assert.AreEqual("stable-id", certificate!.Id);
        Assert.AreEqual(512, certificate.Subject.Length);
        Assert.IsFalse(certificate.SubjectName.Any(char.IsControl));

        var rawDetails = Enumerable.Range(0, 200)
            .Select(index => $"Detalle {index}\u0000")
            .ToArray();
        var verify = JsonSerializer.Deserialize<VerifyResult>(
            JsonSerializer.Serialize(new
            {
                valid = false,
                details = rawDetails,
                integrity = new { status = "invalid" },
                certificate = new { status = "unknown" },
                trust = new { status = "unknown" },
            }));

        Assert.IsNotNull(verify);
        Assert.AreEqual(200, verify!.Details.Count);
        Assert.AreEqual(128, verify.VisibleDetails.Count);
        Assert.IsTrue(verify.VisibleDetails.All(
            detail => !detail.Any(char.IsControl)));

        var longId = new string('I', 1200);
        var boundedCertificate = JsonSerializer.Deserialize<CertificateInfo>(
            $$"""{"id":"{{longId}}\u0000","canSign":true}""");
        var emptyCertificate = JsonSerializer.Deserialize<CertificateInfo>(
            """{"id":"\u0000\u0001","canSign":true}""");
        var boundedRecipient =
            JsonSerializer.Deserialize<ProtectionRecipientInfo>(
                $$"""
                {
                  "id":"{{longId}}\u0000",
                  "label":"Destinatario\u0001",
                  "profile":"compat"
                }
                """);

        Assert.AreEqual(1024, boundedCertificate!.Id.Length);
        Assert.IsFalse(boundedCertificate.Id.Any(char.IsControl));
        Assert.AreEqual(string.Empty, emptyCertificate!.Id);
        Assert.AreEqual(1024, boundedRecipient!.Id.Length);
        Assert.IsFalse(boundedRecipient.Label.Any(char.IsControl));

        var oversizedInventory =
            JsonSerializer.Deserialize<CertificateAccessInventoryResult>(
                JsonSerializer.Serialize(new
                {
                    managers = Enumerable.Range(0, 500)
                        .Select(index => new
                        {
                            id = $"private-{index}",
                            label = new string('L', 500),
                        }),
                    importTargets = Enumerable.Range(0, 500)
                        .Select(index => new
                        {
                            id = $"private-{index}",
                            browser = "private-browser",
                        }),
                }));

        Assert.AreEqual(128, oversizedInventory!.Managers.Count);
        Assert.AreEqual(128, oversizedInventory.ImportTargets.Count);
        Assert.IsFalse(
            typeof(CertificateAccessInventoryEntry)
                .GetProperties()
                .Any());

        var oversizedManagers =
            JsonSerializer.Deserialize<CertificateManagerOptionsResult>(
                JsonSerializer.Serialize(new
                {
                    managers = Enumerable.Range(0, 500)
                        .Select(index => new
                        {
                            id = $"manager-{index}\u0000",
                            label = new string('L', 500) + "\u0001",
                            recommended = index == 0,
                        }),
                    preferredManager =
                        new string('P', 500) + "\u0000",
                }));

        Assert.AreEqual(128, oversizedManagers!.Managers.Count);
        Assert.AreEqual(256, oversizedManagers.Managers[0].Label.Length);
        Assert.IsFalse(
            oversizedManagers.Managers.Any(manager =>
                manager.Id.Any(char.IsControl) ||
                manager.Label.Any(char.IsControl)));
        Assert.AreEqual(256, oversizedManagers.PreferredManager.Length);
    }

    private static SignParameters NewSignParameters() => new()
    {
        InputPath = @"C:\docs\document.pdf",
        OutputPath = @"C:\docs\signed.pdf",
        CertificateId = "cert-1",
        CertificateIndex = 2,
        AdditionalCertificateIds = ["cert-2"],
        Format = "pades",
        Action = "sign",
        Overwrite = "force",
        SaveToDisk = true,
        ReturnSignatureBase64 = false,
        VisibleSeal = new()
        {
            Page = "all",
            X = 0.1,
            Y = 0.2,
            Width = 0.3,
            Height = 0.1,
            PageWidth = 595.28,
            PageHeight = 841.89,
            Rotation = 0,
            KeepText = true,
            ImagePath = @"C:\docs\seal.png",
        },
        AllowInvalidPdf = false,
        StrictCompatibility = true,
        QrContent = "https://example.invalid/verify",
        Reason = "Aprobación",
        Location = "Granada",
        ContactInfo = "soporte@example.invalid",
        ExtraOptions = new Dictionary<string, string>
        {
            ["profile"] = "baseline_b",
        },
    };

    private static ProtectionParameters NewProtectionParameters() => new()
    {
        InputPath = @"C:\docs\document.pdf",
        OutputPath = @"C:\docs\document.authenveloped.p7m",
        CertificateId = "cert-1",
        CertificateIndex = 0,
        Profile = "compat",
        RecipientIds = ["recipient-1"],
        Overwrite = "force",
        SaveToDisk = true,
        ReturnProtectedBase64 = false,
        ReturnUnprotectedBase64 = false,
        Options = new Dictionary<string, string>
        {
            ["container"] = "authenvelopeddata",
        },
        SymmetricKey = new byte[32],
    };

    private static JsonDocument Serialize<T>(T value) =>
        JsonDocument.Parse(JsonSerializer.Serialize(value, WireOptions));

    private static void AssertPropertyNames(
        JsonElement element,
        params string[] expected)
    {
        var actual = element
            .EnumerateObject()
            .Select(property => property.Name)
            .Order(StringComparer.Ordinal)
            .ToArray();
        var orderedExpected = expected
            .Order(StringComparer.Ordinal)
            .ToArray();
        CollectionAssert.AreEqual(orderedExpected, actual);
    }

    private static void AssertJsonPropertyNames<T>(params string[] expected)
    {
        var actual = typeof(T)
            .GetProperties(BindingFlags.Instance | BindingFlags.Public)
            .Select(property =>
                property.GetCustomAttribute<JsonPropertyNameAttribute>()?.Name)
            .Where(name => name is not null)
            .Cast<string>()
            .Order(StringComparer.Ordinal)
            .ToArray();
        var orderedExpected = expected
            .Order(StringComparer.Ordinal)
            .ToArray();
        CollectionAssert.AreEqual(orderedExpected, actual);
    }

    [TestMethod]
    public async Task AdministrativeOperations_UseDesktopIpcActionsAndDecodeReports()
    {
        var transport = new RecordingIpcClient();
        var client = new DesktopOperationsClient(transport);
        await client.ValidateInvoiceAsync(@"C:\docs\factura.xml");
        await client.GenerateEniDocumentAsync(@"C:\docs\firma.pdf", null,
            @"C:\docs\documento-eni.xml", new Dictionary<string, string> { ["eni.organo"] = "L12345678" });
        await client.GenerateEniFileAsync(@"C:\docs\eni", @"C:\docs\expediente.xml",
            "cert-1", new Dictionary<string, string> { ["exp.clasificacion"] = "123456" });
        CollectionAssert.AreEqual(new[] { "validate_invoice", "generate_eni_document", "generate_eni_file" },
            transport.Calls.Select(call => call.Action).ToArray());
        Assert.AreEqual(typeof(InvoiceValidationResult), transport.Calls[0].DataType);
        Assert.AreEqual(typeof(EniGenerationResult), transport.Calls[1].DataType);
        Assert.AreEqual(typeof(EniGenerationResult), transport.Calls[2].DataType);
        var result = JsonSerializer.Deserialize<InvoiceValidationResult>(
            """{"format":"UBL","valid":false,"errors":1,"warnings":1,"issues":[{"level":"error","field":"Total","message":"Descuadre"}],"report":"[error] Total: Descuadre"}""");
        Assert.IsNotNull(result);
        Assert.AreEqual("UBL", result.Format);
        Assert.AreEqual(1, result.Errors);
        Assert.AreEqual("Total", result.Issues[0].Field);
    }

    private sealed class RecordingIpcClient : IIpcClient
    {
        public IpcHello? ServerHello => null;
        public List<RecordedCall> Calls { get; } = [];
        public object? LastResult { get; private set; }

        public Task<IpcCallResult<TData>> SendAsync<TParameters, TData>(
            string action,
            TParameters parameters,
            CancellationToken cancellationToken = default)
        {
            Calls.Add(new(
                action,
                typeof(TParameters),
                typeof(TData),
                parameters!,
                cancellationToken));
            var result = new IpcCallResult<TData>
            {
                Protocol = DesktopIpcProtocol.Name,
                RequestId = "request-real-42",
                TraceId = "trace-real-42",
                Action = action,
                IsSuccess = true,
                Outcome = "success",
                Phase = "operation",
                Retryable = false,
            };
            LastResult = result;
            return Task.FromResult(result);
        }

        public ValueTask DisposeAsync() => ValueTask.CompletedTask;
    }

    private sealed record RecordedCall(
        string Action,
        Type ParametersType,
        Type DataType,
        object Parameters,
        CancellationToken CancellationToken);
}
