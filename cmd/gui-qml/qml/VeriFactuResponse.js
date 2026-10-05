// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
.pragma library

// Traduce la respuesta JSON del cotejo de la AEAT a una frase. Según la
// especificación del QR de Veri*Factu (apartado 9), "status" solo dice si la
// consulta se atendió ("OK") o se rechazó ("KO"); el resultado de la factura
// está en "respuesta.resultado": 00 encontrada, 01 no encontrada (o anulada)
// y 02 no contrastable. Cualquier otra forma se declara no reconocida y se
// ofrece la respuesta técnica. Mismas reglas que WinUI (VeriFactuQrResponse.cs).
function parse(response) {
    if (typeof response !== "string") return response
    try {
        return JSON.parse(response)
    } catch (e) {
        return null
    }
}
function text(value) {
    return typeof value === "string" ? value.trim() : ""
}
function classify(response) {
    const root = parse(response)
    if (!root || typeof root !== "object" || Array.isArray(root)) return "verifactu.qr_aeat_unknown"
    const status = text(root.status).toUpperCase()
    if (status === "KO") return "verifactu.qr_aeat_rejected"
    if (status !== "OK") return "verifactu.qr_aeat_unknown"
    const answer = root.respuesta
    if (!answer || typeof answer !== "object") return "verifactu.qr_aeat_unknown"
    switch (text(answer.resultado)) {
    case "00": return "verifactu.qr_aeat_found"
    case "01": return "verifactu.qr_aeat_not_found"
    case "02": return "verifactu.qr_aeat_not_verifiable"
    default: return "verifactu.qr_aeat_unknown"
    }
}
