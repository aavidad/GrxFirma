// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class VerificationAssessmentTests
{
    [TestMethod]
    public void RejectsNullResultWithoutThrowing()
    {
        Assert.IsFalse(VerificationAssessment.IsCoherent(null));
        Assert.IsFalse(
            VerificationAssessment.HasValidSignatureEvidence(null));
        Assert.IsFalse(
            VerificationAssessment.HasEstablishedTrust(null));
    }

    [TestMethod]
    public void RejectsIncompleteResultWithoutThrowing()
    {
        var result = new VerifyResult
        {
            Integrity = null!,
            Certificate = null!,
            Trust = null!,
        };

        Assert.IsFalse(VerificationAssessment.IsCoherent(result));
        Assert.IsFalse(
            VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsFalse(
            VerificationAssessment.HasEstablishedTrust(result));
    }

    [TestMethod]
    public void PreservesEvidenceButRejectsMissingTrustAspect()
    {
        var result = new VerifyResult
        {
            Integrity = new VerifyAspect { Status = "valid" },
            Certificate = new VerifyAspect { Status = "valid" },
            Trust = null!,
        };

        Assert.IsFalse(VerificationAssessment.IsCoherent(result));
        Assert.IsTrue(
            VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsFalse(
            VerificationAssessment.HasEstablishedTrust(result));
    }

    [TestMethod]
    public void AcceptsCryptographicIntegrityWithoutInventingTrust()
    {
        var result = Result(
            isValid: false,
            integrity: "valid",
            certificate: "valid",
            trust: "invalid");

        Assert.IsTrue(VerificationAssessment.IsCoherent(result));
        Assert.IsTrue(
            VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsFalse(
            VerificationAssessment.HasEstablishedTrust(result));
    }

    [TestMethod]
    public void RejectsInvalidIntegrityEvenWhenTrustClaimsValid()
    {
        var result = Result(
            isValid: false,
            integrity: "invalid",
            certificate: "valid",
            trust: "valid");

        Assert.IsTrue(VerificationAssessment.IsCoherent(result));
        Assert.IsFalse(
            VerificationAssessment.HasValidSignatureEvidence(result));
        Assert.IsFalse(
            VerificationAssessment.HasEstablishedTrust(result));
    }

    [TestMethod]
    public void RequiresAValidSignerCertificateForPostSignEvidence()
    {
        var result = Result(
            isValid: false,
            integrity: "valid",
            certificate: "invalid",
            trust: "invalid");

        Assert.IsTrue(VerificationAssessment.IsCoherent(result));
        Assert.IsFalse(
            VerificationAssessment.HasValidSignatureEvidence(result));
    }

    [TestMethod]
    public void RejectsContradictoryGlobalValidity()
    {
        var result = Result(
            isValid: true,
            integrity: "invalid",
            certificate: "valid",
            trust: "valid");

        Assert.IsFalse(VerificationAssessment.IsCoherent(result));
    }

    private static VerifyResult Result(
        bool isValid,
        string integrity,
        string certificate,
        string trust) =>
        new()
        {
            IsValid = isValid,
            Integrity = new VerifyAspect { Status = integrity },
            Certificate = new VerifyAspect { Status = certificate },
            Trust = new VerifyAspect { Status = trust },
        };
}
