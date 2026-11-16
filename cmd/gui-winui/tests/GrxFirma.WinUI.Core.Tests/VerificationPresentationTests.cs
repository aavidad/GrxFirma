// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;
using System.Text.Json;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class VerificationPresentationTests
{
    [TestMethod]
    public void NullWireEntries_AreFilteredOnlyForPresentation()
    {
        var result = JsonSerializer.Deserialize<VerifyResult>("""
            {
              "valid": false,
              "integrity": { "status": "warning" },
              "certificate": { "status": "valid" },
              "trust": { "status": "valid" },
              "signerSummaries": [null, { "subject": "Synthetic QA" }],
              "evidence": [null, { "type": "xml.canonicalization.compatibility", "summary": "Historical only" }]
            }
            """)!;
        Assert.AreEqual("Synthetic QA", result.VisibleSignerSummaries.Select(item => item.Subject).Single());
        Assert.AreEqual("xml.canonicalization.compatibility",
            result.VisibleEvidence.Select(item => item.Type).Single());
        Assert.AreEqual(2, result.Evidence.Count);
        Assert.IsNull(result.Evidence[0]);
        Assert.AreEqual(2, result.SignerSummaries.Count);
        Assert.IsNull(result.SignerSummaries[0]);
        Assert.IsTrue(VerificationAssessment.HasXmlCanonicalizationCompatibility(result));
        Assert.IsFalse(VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.AreEqual("Compatibilidad histórica; validación estándar no acreditada",
            VerificationPresentation.GetTitle(result));
    }

    private static VerifyResult Result(bool valid, string integrity, string certificate, string trust) => new()
    {
        IsValid = valid,
        Integrity = new VerifyAspect { Status = integrity },
        Certificate = new VerifyAspect { Status = certificate },
        Trust = new VerifyAspect { Status = trust },
    };

    [TestMethod]
    [DataRow("unknown")]
    [DataRow("warning")]
    [DataRow("invalid")]
    public void CertificateWithoutValidEvidence_DoesNotClaimValidSignature(string certificate)
    {
        var result = Result(false, "valid", certificate, "valid");
        Assert.IsTrue(VerificationAssessment.IsCoherent(result));
        Assert.AreEqual("Validez de firma no acreditada", VerificationPresentation.GetTitle(result));
    }

    [TestMethod]
    public void OverallFalse_DoesNotClaimEstablishedTrust()
    {
        Assert.AreEqual("Firma íntegra; confianza no acreditada",
            VerificationPresentation.GetTitle(Result(false, "valid", "valid", "valid")));
        Assert.AreEqual("Firma válida y de confianza",
            VerificationPresentation.GetTitle(Result(true, "valid", "valid", "valid")));
    }

    [TestMethod]
    [DataRow("invalid", "Firma íntegra, pero no confiable")]
    [DataRow("warning", "Firma íntegra con avisos de confianza")]
    [DataRow("unknown", "Firma íntegra; confianza no determinada")]
    public void TrustDegradation_IsExplicit(string trust, string expected) =>
        Assert.AreEqual(expected, VerificationPresentation.GetTitle(Result(false, "valid", "valid", trust)));

    [TestMethod]
    public void NullAndContradictoryResults_AreNotUsable()
    {
        Assert.AreEqual("Resultado no utilizable", VerificationPresentation.GetTitle(null));
        Assert.AreEqual("Resultado no utilizable",
            VerificationPresentation.GetTitle(Result(true, "valid", "unknown", "valid")));
        Assert.AreEqual("Resultado no utilizable",
            VerificationPresentation.GetTitle(Result(false, "valid", "valid", "unsupported")));
    }

    [TestMethod]
    public void CompatibilityIsHistoricalOnly_AndContradictionRejected()
    {
        var result = Result(false, "warning", "valid", "valid") with
        {
            Evidence = Enumerable.Repeat(new VerifyEvidence { Type = "qa.other" }, 256)
                .Append(new VerifyEvidence { Type = "xml.canonicalization.compatibility" }).ToArray(),
        };
        Assert.AreEqual("Compatibilidad histórica; validación estándar no acreditada",
            VerificationPresentation.GetTitle(result));
        Assert.AreEqual("Resultado no utilizable", VerificationPresentation.GetTitle(result with { IsValid = true }));
    }

    // Orden de Alberto (16/11/2026): la clave RSA corta sin excepción es firma
    // no válida; caducado y emisor desconocido conservan su título.
    private static VerifyResult ShortRsaResult(string bits = "1024") =>
        JsonSerializer.Deserialize<VerifyResult>($$"""
            {
              "valid": false,
              "reason": "clave RSA del firmante de menos de 2048 bits",
              "integrity": { "status": "valid" },
              "certificate": {
                "status": "invalid",
                "reason": "clave RSA del firmante de menos de 2048 bits",
                "details": ["subject=Firmante sintético", "rsa_bits={{bits}}"]
              },
              "trust": { "status": "valid" },
              "evidence": []
            }
            """)!;

    [TestMethod]
    [DataRow("1024")]
    [DataRow("2047")]
    public void ShortRsaKeyWithoutException_IsInvalidSignature(string bits)
    {
        var result = ShortRsaResult(bits);
        Assert.IsTrue(VerificationAssessment.IsCoherent(result));
        Assert.IsTrue(VerificationAssessment.HasShortRsaKeyWithoutException(result));
        Assert.IsFalse(VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.AreEqual("Firma no válida", VerificationPresentation.GetTitle(result));
    }

    [TestMethod]
    public void ShortRsaKeyWithAcceptedException_IsNotInvalidSignature()
    {
        var result = ShortRsaResult() with
        {
            IsValid = true,
            Certificate = new VerifyAspect { Status = "valid", Details = ["rsa_bits=1024"] },
            Evidence = [new VerifyEvidence { Type = "trust.user_forced", Summary = "clave_rsa_corta" }],
        };
        Assert.IsFalse(VerificationAssessment.HasShortRsaKeyWithoutException(result));
        Assert.IsTrue(VerificationAssessment.HasForcedTrust(result));
        Assert.AreNotEqual("Firma no válida", VerificationPresentation.GetTitle(result));
    }

    [TestMethod]
    public void OtherAcceptedException_DoesNotHideShortRsaKey()
    {
        var result = ShortRsaResult() with
        {
            Evidence = [new VerifyEvidence { Type = "trust.user_forced", Summary = "ca_desconocida" }],
        };
        Assert.IsTrue(VerificationAssessment.HasShortRsaKeyWithoutException(result));
        Assert.AreEqual("Firma no válida", VerificationPresentation.GetTitle(result));
    }

    [TestMethod]
    public void ExpiredOrUnknownIssuer_KeepTheirTitle()
    {
        var expired = Result(false, "valid", "invalid", "valid") with
        {
            Certificate = new VerifyAspect
            {
                Status = "invalid",
                Reason = "certificado caducado: Firmante sintético",
                Details = ["not_after=2020-01-01T00:00:00Z", "rsa_bits=4096"],
            },
        };
        Assert.IsFalse(VerificationAssessment.HasShortRsaKeyWithoutException(expired));
        Assert.AreEqual("Validez de firma no acreditada", VerificationPresentation.GetTitle(expired));
        var unknownIssuer = Result(false, "valid", "valid", "invalid");
        Assert.IsFalse(VerificationAssessment.HasShortRsaKeyWithoutException(unknownIssuer));
        Assert.AreEqual("Firma íntegra, pero no confiable", VerificationPresentation.GetTitle(unknownIssuer));
    }

    [TestMethod]
    public void MalformedOrLongRsaDetail_IsNotShortKey()
    {
        foreach (var bits in new[] { "2048", "4096", "mil", "-1", "0", " 1024x" })
        {
            Assert.IsFalse(
                VerificationAssessment.HasShortRsaKeyWithoutException(ShortRsaResult(bits)),
                bits);
        }
    }
}
