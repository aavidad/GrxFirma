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
