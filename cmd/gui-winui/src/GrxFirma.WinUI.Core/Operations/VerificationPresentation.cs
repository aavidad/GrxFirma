// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Operations;

public static class VerificationPresentation
{
    public static string GetTitle(VerifyResult? result)
    {
        if (!VerificationAssessment.IsCoherent(result))
        {
            return "Resultado no utilizable";
        }
        if (VerificationAssessment.HasXmlCanonicalizationCompatibility(result))
        {
            return "Compatibilidad histórica; validación estándar no acreditada";
        }
        if (!VerificationAssessment.HasValidSignatureEvidence(result))
        {
            return "Validez de firma no acreditada";
        }
        if (VerificationAssessment.HasEstablishedTrust(result))
        {
            return "Firma válida y de confianza";
        }
        return result!.Trust.Status?.Trim().ToLowerInvariant() switch
        {
            "invalid" => "Firma íntegra, pero no confiable",
            "warning" => "Firma íntegra con avisos de confianza",
            "unknown" => "Firma íntegra; confianza no determinada",
            _ => "Firma íntegra; confianza no acreditada",
        };
    }
}
