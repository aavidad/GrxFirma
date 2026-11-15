// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

// Presentación de los textos de una excepción insegura (lote 39h). Aquí no hay
// ningún texto para personas: solo se trocean, eligen y ordenan las claves y
// los textos del catálogo. La misma lógica está en WinUI
// (GrxFirma.WinUI.Core/Operations/UnsafeExceptionText.cs).

// Formato de fecha del catálogo, como en el motor: «02/01/2006 15:04».
var defaultDateLayout = "02/01/2006 15:04"

// Con el caso fijado por la organización la ayuda no explica cómo activarlo
// ni cómo deshacerlo: quedan los cuatro primeros apartados (qué hace, por qué
// es inseguro, cuándo tiene sentido y la alternativa) y el de la organización.
var sectionsKeptForOrganization = 4

// Un título más largo que esto no es un título: el apartado va sin él.
var maximumTitleLength = 80

// «Qué hace este ajuste: mientras…» pasa a {title, body}. Vale también para
// los dos puntos de ancho completo del chino y el espacio previo del francés.
function splitSection(text) {
    const clean = String(text || "").trim()
    let cut = -1
    for (let i = 0; i < clean.length && i <= maximumTitleLength; i++) {
        if (clean[i] === ":" || clean[i] === "：") { cut = i; break }
    }
    if (cut <= 0) return {title: "", body: clean}
    let body = clean.substring(cut + 1).trim()
    if (body.length > 0) body = body.charAt(0).toUpperCase() + body.substring(1)
    return {title: clean.substring(0, cut).trim(), body: body}
}

// Los apartados de la ayuda van separados por una línea en blanco (lote 39d).
function sections(help) {
    const parts = String(help || "").replace(/\r\n/g, "\n").split(/\n[ \t]*\n/)
    const out = []
    for (let i = 0; i < parts.length; i++) {
        if (parts[i].trim() === "") continue
        out.push(splitSection(parts[i]))
    }
    return out
}

// Lo decide la organización: la directiva fija el caso o lo prohíbe en un
// bloqueo. Un directorio de ajustes no disponible no es cosa suya.
function organizationDecides(item) {
    if (!item) return false
    if (item.fixed === true) return true
    return item.activable !== true && item.config_unavailable !== true && !!item.organization_key
}

function helpSections(help, organizationSection, decidedByOrganization) {
    const all = sections(help)
    if (!decidedByOrganization) return all
    const kept = all.slice(0, sectionsKeptForOrganization)
    if (String(organizationSection || "").trim() !== "") kept.push(splitSection(organizationSection))
    return kept
}

// Nota de una fila de Configuración que no se puede cambiar. Cada cosa se dice
// una vez: el estado ya dice «Desactivado por la organización» o que la
// directiva no se puede leer, así que la nota no lo repite.
function policyNoteKey(item) {
    if (!item || item.policy_unreadable === true) return ""
    if (item.fixed === true) {
        if (item.enabled === true) return "excepciones.pantalla.solo_lectura"
        return item.organization_key || ""
    }
    if (item.activable === false && item.config_unavailable === true) return "excepciones.directorio_no_disponible"
    return ""
}

function pad(value) { return value < 10 ? "0" + value : String(value) }

// Fecha y hora locales con el formato del idioma de la interfaz
// («format.datetime_minutes», disposición de Go): el año siempre con cuatro
// cifras y el mismo resultado que WinUI. Sin fecha válida, nada.
function dateText(raw, layout) {
    const date = new Date(String(raw || ""))
    if (String(raw || "") === "" || isNaN(date.getTime())) return ""
    const tokens = {
        "2006": String(date.getFullYear()),
        "01": pad(date.getMonth() + 1),
        "02": pad(date.getDate()),
        "15": pad(date.getHours()),
        "04": pad(date.getMinutes()),
        "05": pad(date.getSeconds())
    }
    const pattern = String(layout || "")
    const valid = /2006/.test(pattern) && /01/.test(pattern) && /02/.test(pattern)
    return (valid ? pattern : defaultDateLayout).replace(/2006|01|02|15|04|05/g, function(token) { return tokens[token] })
}
