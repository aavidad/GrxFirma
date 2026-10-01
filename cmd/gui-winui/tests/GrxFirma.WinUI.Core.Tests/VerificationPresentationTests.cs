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
}
