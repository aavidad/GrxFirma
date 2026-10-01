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
                "No se ha encontrado ningún lector de tarjetas. Conecte un lector compatible y vuelva a intentarlo.",
                InfoBarSeverity.Warning,
                false);
        }
        if (!status.Readers.Any(reader => reader.Present))
        {
            return new(
                "Inserte el DNIe o la tarjeta en el lector y vuelva a intentarlo.",
                InfoBarSeverity.Informational,
                false);
        }
        if (hasUnlockableCertificate)
        {
            return new(
                "Tarjeta detectada y certificado seleccionado. Al firmar, el controlador de Windows solicitará el PIN.",
                InfoBarSeverity.Success,
                false);
        }
        return new(
            "Tarjeta detectada, pero no aparecen certificados de firma. Falta el controlador del fabricante; para el DNIe, descárguelo de la web oficial.",
            InfoBarSeverity.Warning,
            true);
    }
}
