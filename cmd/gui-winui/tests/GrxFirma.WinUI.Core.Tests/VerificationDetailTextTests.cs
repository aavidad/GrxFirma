// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class VerificationDetailTextTests
{
    // Catálogo sintético con las claves reales del motor (independiente del texto).
    private const string Spanish = """
        {
          "verificacion.detalle.formato": "%s: %s",
          "verificacion.detalle.error_cadena": "Cadena del certificado",
          "verificacion.detalle.valor.x509_caducado": "caducado",
          "verificacion.detalle.formato_detectado": "Formato detectado",
          "verificacion.detalle.cobertura_firma_pdf": "Cobertura de la firma %d",
          "verificacion.detalle.cadena_firmante": "Certificados del firmante %d",
          "verificacion.detalle.valor.revision_hasta": "hasta el byte %d de %d",
          "verificacion.detalle.revocacion.bueno": "no revocado",
          "report.evidence.certificate.subject": "Titular del certificado",
          "Firma verificada": "Firma verificada"
        }
        """;

    private const string English = """
        {
          "verificacion.detalle.formato": "%s: %s",
          "verificacion.detalle.error_cadena": "Certificate chain",
          "verificacion.detalle.valor.x509_caducado": "expired",
          "verificacion.detalle.formato_detectado": "Detected format",
          "verificacion.detalle.cobertura_firma_pdf": "Signature coverage %d",
          "verificacion.detalle.cadena_firmante": "Certificates for signer %d",
          "verificacion.detalle.valor.revision_hasta": "up to byte %d of %d",
          "verificacion.detalle.revocacion.bueno": "not revoked",
          "report.evidence.certificate.subject": "Certificate holder",
          "Firma verificada": "Signature verified"
        }
        """;

    private static void WithCatalog(Action<CatalogLocalizer> test)
    {
        var directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(directory);
        try
        {
            File.WriteAllText(Path.Combine(directory, "es.json"), Spanish);
            File.WriteAllText(Path.Combine(directory, "en.json"), English);
            test(new CatalogLocalizer(directory, "en"));
        }
        finally
        {
            Directory.Delete(directory, recursive: true);
        }
    }

    [TestMethod]
    public void ChainErrorCode_IsTranslatedWithTheCatalog()
    {
        WithCatalog(localizer =>
        {
            Assert.AreEqual("Certificate chain: expired",
                VerificationDetailText.Detail(localizer, "error_cadena=x509_caducado"));
            // Un código que el catálogo no conoce conserva el valor tal cual.
            Assert.AreEqual("Certificate chain: x509_nuevo",
                VerificationDetailText.Detail(localizer, "error_cadena=x509_nuevo"));
        });
    }

    [TestMethod]
    public void EngineDetailPatterns_MatchQtAndReport()
    {
        WithCatalog(localizer =>
        {
            Assert.AreEqual("Detected format: PAdES",
                VerificationDetailText.Detail(localizer, "formato_detectado=PAdES"));
            Assert.AreEqual("Signature coverage 2: up to byte 10 of 20",
                VerificationDetailText.Detail(localizer, "cobertura_firma_pdf_2=revision_hasta_10_de_20"));
            Assert.AreEqual("Certificates for signer 1: 3",
                VerificationDetailText.Detail(localizer, "signer[0].chain_length=3"));
            Assert.AreEqual("CN=QA: not revoked (OCSP)",
                VerificationDetailText.Detail(localizer, "cert[0] CN=QA: bueno vía ocsp"));
            Assert.AreEqual("Signature verified",
                VerificationDetailText.Detail(localizer, "Firma verificada"));
            Assert.AreEqual("clave_desconocida=1",
                VerificationDetailText.Detail(localizer, "clave_desconocida=1"));
        });
    }

    [TestMethod]
    public void EvidenceType_UsesReportCatalogOrKeepsUnknownType()
    {
        WithCatalog(localizer =>
        {
            Assert.AreEqual("Certificate holder",
                VerificationDetailText.EvidenceType(localizer, "certificate.subject"));
            Assert.AreEqual("timestamp.unknown",
                VerificationDetailText.EvidenceType(localizer, "timestamp.unknown"));
        });
    }
}
