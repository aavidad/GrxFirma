// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Layouts

// Conserva la fila habitual; con texto ampliado apila los controles si sus
// anchos naturales no caben. El ancho del contenido nunca manda sobre el panel.
GridLayout {
    id: row
    property real spacing: 5
    readonly property int visibleCount: {
        let count = 0
        for (let i = 0; i < children.length; ++i)
            if (children[i].visible) count++
        return count
    }
    readonly property real naturalWidth: {
        let total = 0
        for (let i = 0; i < children.length; ++i) {
            const child = children[i]
            if (child.visible) total += Math.max(child.implicitWidth, child.Layout.minimumWidth)
        }
        return total + Math.max(0, visibleCount - 1) * spacing
    }
    columns: Typography.scale > 1 && naturalWidth > width ? 1 : Math.max(1, visibleCount)
    columnSpacing: spacing
    rowSpacing: spacing
}
