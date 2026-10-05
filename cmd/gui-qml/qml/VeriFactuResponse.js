// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
.pragma library

// Traduce la respuesta JSON del cotejo de la AEAT a una frase. La AEAT no
// publica un esquema estable de esa respuesta, así que solo se afirma un
// resultado cuando el texto es inequívoco; en otro caso se dice que no se
// reconoce y se ofrece la respuesta técnica. Mismas reglas que WinUI
// (VeriFactuQrResponse.cs).
const negative = /\bno\s*(?:se\s*)?(?:ha\s*)?(?:encontrad|encuentr|consta|existe|registrad|identificad)|noencontrad|not[\s_-]*found|incorrect|"ko"/
const positive = /encontrad|correct|consta|registrad|identificad|"ok"|"found"/
function normalize(response) {
    const text = typeof response === "string" ? response : JSON.stringify(response)
    return String(text || "").toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "")
}
function classify(response) {
    const text = normalize(response)
    if (negative.test(text)) return "verifactu.qr_aeat_not_found"
    if (positive.test(text)) return "verifactu.qr_aeat_found"
    return "verifactu.qr_aeat_unknown"
}
