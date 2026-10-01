// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class XmlVerificationCompatibilityTests
{
    private const string CompatibilityType = "xml.canonicalization.compatibility";

    [TestMethod]
    public void WireResult_PreservesHistoricalDiagnosisWithoutPostSignSuccess()
    {
        var result = JsonSerializer.Deserialize<VerifyResult>("""
            {
              "valid": false,
              "format": "XAdES",
              "coverage": "unknown",
              "reason": "Validación estándar no acreditada.",
              "integrity": { "status": "warning" },
              "certificate": { "status": "valid" },
              "trust": { "status": "valid" },
              "signers": ["Firmante sintético QA"],
              "warnings": ["Solo compatibilidad histórica."],
              "evidence": [{
                "type": "xml.canonicalization.compatibility",
                "summary": "Coincidencia con un procedimiento no declarado."
              }]
            }
            """)!;

        Assert.IsTrue(VerificationAssessment.IsCoherent(result));
        Assert.IsTrue(VerificationAssessment.HasXmlCanonicalizationCompatibility(result));
        Assert.IsFalse(VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsFalse(VerificationAssessment.HasEstablishedTrust(result));
        Assert.AreEqual("unknown", result.Coverage);
        Assert.AreEqual("Firmante sintético QA", result.VisibleSigners.Single());
        Assert.AreEqual(CompatibilityType, result.VisibleEvidence.Single().Type);
        Assert.AreEqual("Solo compatibilidad histórica.", result.VisibleWarnings.Single());
    }

    [TestMethod]
    [DataRow(true, "valid")]
    [DataRow(false, "valid")]
    [DataRow(true, "warning")]
    [DataRow(false, "invalid")]
    [DataRow(false, "unknown")]
    public void MarkerWithContradictoryStatus_IsNeverAccepted(bool valid, string integrity)
    {
        var result = Result() with
        {
            IsValid = valid,
            Integrity = new VerifyAspect { Status = integrity },
        };
        Assert.IsFalse(VerificationAssessment.IsCoherent(result));
        Assert.IsFalse(VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsFalse(VerificationAssessment.HasEstablishedTrust(result));
    }

    [TestMethod]
    public void MarkerAfterVisibleLimit_StillPreventsPositiveEvidence()
    {
        // El contrato Core limita a 128; la página limita después a 32.
        // Colocar la advertencia más allá de ambos límites de presentación.
        var evidence = Enumerable.Range(0, 256)
            .Select(_ => new VerifyEvidence { Type = "qa.other" })
            .Append(new VerifyEvidence { Type = CompatibilityType })
            .ToArray();
        var result = Result() with
        {
            IsValid = true,
            Integrity = new VerifyAspect { Status = "valid" },
            Evidence = evidence,
        };
        Assert.IsFalse(result.VisibleEvidence.Any(item => item.Type == CompatibilityType));
        Assert.IsTrue(VerificationAssessment.HasXmlCanonicalizationCompatibility(result));
        Assert.IsFalse(VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsFalse(VerificationAssessment.IsCoherent(result));
    }

    [TestMethod]
    public void MissingMarker_DoesNotInventHistoricalCompatibility()
    {
        Assert.IsFalse(VerificationAssessment.HasXmlCanonicalizationCompatibility(null));
        foreach (var evidence in new IReadOnlyList<VerifyEvidence>?[]
                 {
                     null, [], [null!],
                     [new VerifyEvidence { Type = "xml.other", Summary = CompatibilityType }],
                 })
        {
            var result = Result() with { Evidence = evidence! };
            Assert.IsFalse(VerificationAssessment.HasXmlCanonicalizationCompatibility(result));
            Assert.IsTrue(VerificationAssessment.IsCoherent(result));
            Assert.IsFalse(VerificationAssessment.HasValidSignatureEvidence(result));
        }
    }

    [TestMethod]
    public void StandardValidResult_StillAllowsNormalPostValidation()
    {
        var result = Result() with
        {
            IsValid = true,
            Integrity = new VerifyAspect { Status = "valid" },
            Evidence = [],
        };
        Assert.IsTrue(VerificationAssessment.IsCoherent(result));
        Assert.IsTrue(VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsTrue(VerificationAssessment.HasEstablishedTrust(result));
        Assert.IsFalse(VerificationAssessment.HasXmlCanonicalizationCompatibility(result));
    }

    private static VerifyResult Result() => new()
    {
        IsValid = false,
        Integrity = new VerifyAspect { Status = "warning" },
        Certificate = new VerifyAspect { Status = "valid" },
        Trust = new VerifyAspect { Status = "valid" },
        Evidence = [new VerifyEvidence { Type = CompatibilityType }],
    };
}
