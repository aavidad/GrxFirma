// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

// Vista: origen arriba; PDF: origen abajo. Nunca se altera el giro.
function normalize(x1, y1, x2, y2) {
    const values = [x1, y1, x2, y2]
    if (!values.every(Number.isFinite)) throw new RangeError()
    x1 = Math.max(0, Math.min(1, x1)); y1 = Math.max(0, Math.min(1, y1))
    x2 = Math.max(0, Math.min(1, x2)); y2 = Math.max(0, Math.min(1, y2))
    return {x: Math.min(x1, x2), y: 1 - Math.max(y1, y2),
            w: Math.abs(x2 - x1), h: Math.abs(y2 - y1)}
}

function isLargeEnough(rect, pageWidth, pageHeight) {
    // El mismo mínimo que el tirador Qt: 40 × 25 píxeles de vista previa.
    return rect.w * pageWidth + 1e-9 >= 40 && rect.h * pageHeight + 1e-9 >= 25
}
