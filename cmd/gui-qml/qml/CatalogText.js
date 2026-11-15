// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

// Textos que llegan como claves o frases del catálogo y se pintan en el idioma
// de la interfaz. Aquí no hay textos: todo sale de la función de traducción.

var keyPattern = /^[a-z0-9_]+(\.[a-z0-9_]+)+$/

// Avisos del motor (claves del catálogo) en el idioma de la persona. Una clave
// que el catálogo no conoce no se enseña: nunca sale la clave cruda.
// Una QStringList del puente llega a QML como secuencia, no como Array: con
// Array.isArray se descartaba entera y el aviso de uso no salía (lote 39a,
// vuelta 2). Se acepta cualquier lista con longitud que no sea un texto.
function warningTexts(keys, translate, limit) {
    if (keys === null || keys === undefined || typeof keys === "string") return []
    const length = Number(keys.length)
    if (!isFinite(length) || length <= 0) return []
    const max = limit > 0 ? limit : 16
    const shown = []
    for (let i = 0; i < length && shown.length < max; i++) {
        const key = typeof keys[i] === "string" ? keys[i] : ""
        if (!keyPattern.test(key)) continue
        const text = translate(key)
        if (text === key || text === "" || shown.indexOf(text) !== -1) continue
        shown.push(text)
    }
    return shown
}

// Notas de versión: cada línea es una frase del catálogo (lote 37). Como en
// WinUI, se quitan el encabezado, la viñeta, ** y ` para buscar la frase; si
// el catálogo la conoce se pinta traducida con su encabezado o viñeta, y si no
// la conoce queda la línea original.
function releaseNotes(markdown, translate) {
    if (typeof markdown !== "string" || markdown === "") return ""
    const lines = markdown.split("\n")
    for (let i = 0; i < lines.length; i++) {
        const raw = lines[i]
        const match = /^(\s*)(#{1,6} |- |\* )?(.*?)\s*$/.exec(raw)
        if (!match) continue
        const prefix = match[1] + (match[2] || "")
        const body = match[3]
        if (body === "" || body.indexOf("<!--") === 0) continue
        const phrase = body.split("**").join("").split("`").join("")
        const text = translate(phrase)
        if (typeof text === "string" && text !== "" && text !== phrase)
            lines[i] = prefix + text
    }
    return lines.join("\n")
}
