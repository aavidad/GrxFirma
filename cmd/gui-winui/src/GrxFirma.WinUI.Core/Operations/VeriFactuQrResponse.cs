// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;

namespace GrxFirma.WinUI.Core.Operations;

// Traduce la respuesta JSON del cotejo de la AEAT a la clave de una frase.
// Según la especificación del QR de Veri*Factu (apartado 9), "status" solo
// dice si la consulta se atendió ("OK") o se rechazó ("KO"); el resultado de
// la factura está en "respuesta.resultado": 00 encontrada, 01 no encontrada
// (o anulada) y 02 no contrastable (sistema no verificable). Cualquier otra
// forma se declara no reconocida y la pantalla ofrece la respuesta técnica.
// Mismas reglas que Qt (cmd/gui-qml/qml/VeriFactuResponse.js).
public static class VeriFactuQrResponse
{
    public const string FoundKey = "verifactu.qr_aeat_found";
    public const string NotFoundKey = "verifactu.qr_aeat_not_found";
    public const string NotVerifiableKey = "verifactu.qr_aeat_not_verifiable";
    public const string RejectedKey = "verifactu.qr_aeat_rejected";
    public const string UnknownKey = "verifactu.qr_aeat_unknown";

    public static string Classify(string? json)
    {
        if (string.IsNullOrWhiteSpace(json)) return UnknownKey;
        try
        {
            using var document = JsonDocument.Parse(json);
            var root = document.RootElement;
            if (root.ValueKind != JsonValueKind.Object) return UnknownKey;
            var status = root.TryGetProperty("status", out var statusValue) ? Text(statusValue) : null;
            if (string.Equals(status, "KO", StringComparison.OrdinalIgnoreCase)) return RejectedKey;
            if (!string.Equals(status, "OK", StringComparison.OrdinalIgnoreCase)) return UnknownKey;
            if (!root.TryGetProperty("respuesta", out var answer) ||
                answer.ValueKind != JsonValueKind.Object) return UnknownKey;
            var result = answer.TryGetProperty("resultado", out var resultValue) ? Text(resultValue) : null;
            return result switch
            {
                "00" => FoundKey,
                "01" => NotFoundKey,
                "02" => NotVerifiableKey,
                _ => UnknownKey,
            };
        }
        catch (JsonException)
        {
            return UnknownKey;
        }
    }

    private static string? Text(JsonElement value) =>
        value.ValueKind == JsonValueKind.String ? value.GetString()?.Trim() : null;
}
