// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Layouts

// Fila de botones o casillas que pasa a la línea siguiente cuando no cabe, para
// que los paneles no obliguen a desplazarse en horizontal en ventanas estrechas.
// Dentro de un ColumnLayout ocupa como mucho su ancho natural; con «centered» se
// centra en la columna.
Flow {
    id: row
    property bool centered: false
    readonly property real naturalWidth: {
        let total = 0
        let shown = 0
        for (let i = 0; i < row.children.length; i++) {
            const child = row.children[i]
            if (!child.visible) continue
            total += child.implicitWidth
            shown++
        }
        return total + Math.max(0, shown - 1) * row.spacing
    }
    spacing: 8
    Layout.fillWidth: !row.centered
    Layout.alignment: row.centered ? Qt.AlignHCenter : Qt.AlignLeft
    Layout.preferredWidth: row.centered && row.parent ? Math.min(row.parent.width, row.naturalWidth) : -1
}
