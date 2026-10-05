// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Services;

internal sealed record SmartcardGuidanceResult(
    string Message,
    InfoBarSeverity Severity,
    bool ShowOfficialLink);

internal static class SmartcardGuidance
{
    public static SmartcardGuidanceResult Describe(
        SmartcardStatusResult status,
        bool hasUnlockableCertificate)
    {
        if (status.Readers.Count == 0)
        {
            return new(
                Localizer.Text("winui.certificados.no_se_ha_encontrado_ningun_lector_de"),
                InfoBarSeverity.Warning,
                false);
        }
        if (!status.Readers.Any(reader => reader.Present))
        {
            return new(
                Localizer.Text("winui.certificados.inserte_el_dnie_o_la_tarjeta_en_el"),
                InfoBarSeverity.Informational,
                false);
        }
        if (hasUnlockableCertificate)
        {
            return new(
                Localizer.Text("winui.certificados.tarjeta_detectada_y_certificado"),
                InfoBarSeverity.Success,
                false);
        }
        return new(
            Localizer.Text("winui.certificados.tarjeta_detectada_pero_no_aparecen"),
            InfoBarSeverity.Warning,
            true);
    }
}
