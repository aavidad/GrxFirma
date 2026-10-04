// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Operations;

public static class VerificationPresentation
{
    public static string GetTitle(VerifyResult? result)
    {
        string title;
        if (!VerificationAssessment.IsCoherent(result))
        {
            title = "Resultado no utilizable";
        }
        else if (VerificationAssessment.HasXmlCanonicalizationCompatibility(result))
        {
            title = "Compatibilidad histórica; validación estándar no acreditada";
        }
        else if (!VerificationAssessment.HasValidSignatureEvidence(result))
        {
            title = "Validez de firma no acreditada";
        }
        else if (VerificationAssessment.HasEstablishedTrust(result))
        {
            title = "Firma válida y de confianza";
        }
        else title = result!.Trust.Status?.Trim().ToLowerInvariant() switch
        {
            "invalid" => "Firma íntegra, pero no confiable",
            "warning" => "Firma íntegra con avisos de confianza",
            "unknown" => "Firma íntegra; confianza no determinada",
            _ => "Firma íntegra; confianza no acreditada",
        };
        return CatalogLocalizer.Shared.TranslateVisibleText(title);
    }
}
