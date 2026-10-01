// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public static class VerificationAssessment
{
    public static bool IsCoherent(VerifyResult? result) =>
        result is not null &&
        result.Integrity is not null &&
        result.Certificate is not null &&
        result.Trust is not null &&
        IsKnownStatus(result.Integrity.Status) &&
        IsKnownStatus(result.Certificate.Status) &&
        IsKnownStatus(result.Trust.Status) &&
        (!HasXmlCanonicalizationCompatibility(result) ||
            (!result.IsValid && IsStatus(result.Integrity.Status, "warning"))) &&
        (!result.IsValid || HasValidSignatureEvidence(result));

    public static bool HasValidSignatureEvidence(VerifyResult? result) =>
        result?.Integrity is not null &&
        result.Certificate is not null &&
        !HasXmlCanonicalizationCompatibility(result) &&
        IsStatus(result.Integrity.Status, "valid") &&
        IsStatus(result.Certificate.Status, "valid");

    // Es evidencia de diagnóstico, nunca de conformidad XMLDSig. Examinar
    // toda la colección: el límite de elementos visibles no es una política
    // de validación ni debe ocultar una degradación del motor.
    public static bool HasXmlCanonicalizationCompatibility(VerifyResult? result) =>
        result?.Evidence?.Any(item => string.Equals(
            item?.Type,
            "xml.canonicalization.compatibility",
            StringComparison.Ordinal)) == true;

    public static bool HasEstablishedTrust(VerifyResult? result) =>
        HasValidSignatureEvidence(result) &&
        result?.Trust is not null &&
        result.IsValid &&
        IsStatus(result.Trust.Status, "valid");

    private static bool IsKnownStatus(string? value) =>
        value?.Trim().ToLowerInvariant() is
            "valid" or "invalid" or "warning" or "unknown";

    private static bool IsStatus(
        string? value,
        string expected) =>
        string.Equals(
            value?.Trim(),
            expected,
            StringComparison.OrdinalIgnoreCase);
}
