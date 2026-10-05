// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

// Nombre accesible de un interruptor o una casilla: su propio texto, una
// etiqueta indicada a mano o, si no hay ninguna, el rótulo que lo precede en
// la misma fila (en Configuración el rótulo es un Text aparte). Sin esto, los
// lectores de pantalla anuncian «casilla» sin decir cuál.
function name(control, ownText, explicitLabel) {
    if (String(ownText || "").trim() !== "") return ownText
    if (String(explicitLabel || "").trim() !== "") return explicitLabel
    const siblings = control && control.parent ? control.parent.children : []
    let label = ""
    for (let i = 0; i < siblings.length; ++i) {
        const item = siblings[i]
        if (item === control) break
        // Solo textos de lectura (Text, Label): tienen wrapMode y no echoMode.
        if (!item || item.visible === false || item.wrapMode === undefined || item.echoMode !== undefined)
            continue
        const text = String(item.text || "").trim()
        if (text !== "") label = text
    }
    return label
}
