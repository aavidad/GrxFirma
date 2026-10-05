// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

// Contraste WCAG entre colores del tema. Acepta colores QML o cadenas «#rrggbb»
// (los temas se definen como cadenas).
function toColor(c) {
    return typeof c === "string" ? Qt.darker(c, 1.0) : c
}

function channel(v) {
    return v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
}

function luminance(value) {
    const c = toColor(value)
    return 0.2126 * channel(c.r) + 0.7152 * channel(c.g) + 0.0722 * channel(c.b)
}

function ratio(a, b) {
    const la = luminance(a)
    const lb = luminance(b)
    return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

// Devuelve el color preferido si se lee bien sobre el fondo (AA, 4,5:1);
// si no, el más legible entre casi negro y blanco.
function readableOn(background, preferred) {
    if (preferred !== undefined && preferred !== null && ratio(background, preferred) >= 4.5)
        return preferred
    const dark = Qt.rgba(0.07, 0.08, 0.1, 1)
    const light = Qt.rgba(1, 1, 1, 1)
    return ratio(background, dark) >= ratio(background, light) ? dark : light
}

// Fondo de botón ajustado: si ni el texto casi negro ni el blanco llegan a 4,5:1,
// se oscurece poco a poco hasta que el blanco lo alcance.
function legibleFill(background) {
    let c = toColor(background)
    const light = Qt.rgba(1, 1, 1, 1)
    const dark = Qt.rgba(0.07, 0.08, 0.1, 1)
    for (let i = 0; i < 6; i++) {
        if (ratio(c, light) >= 4.5 || ratio(c, dark) >= 4.5) return c
        c = Qt.darker(c, 1.12)
    }
    return c
}

// Color de acento (títulos, estados) ajustado al fondo: conserva el tono si ya
// llega al contraste pedido (4,5:1 por defecto); si no, lo oscurece sobre fondos
// claros o lo aclara sobre oscuros hasta alcanzarlo, y como último recurso usa
// el texto más legible entre casi negro y blanco.
function accentOn(background, accent, minimum) {
    const target = minimum === undefined ? 4.5 : minimum
    let c = toColor(accent)
    if (ratio(background, c) >= target) return c
    const lightBackground = luminance(background) > 0.18
    for (let i = 0; i < 12; i++) {
        c = lightBackground ? Qt.darker(c, 1.15) : Qt.lighter(c, 1.15)
        if (ratio(background, c) >= target) return c
    }
    return readableOn(background)
}
