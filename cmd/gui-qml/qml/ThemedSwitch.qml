// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "AccessibleLabel.js" as AccessibleLabel

// Interruptor con el texto del tema (paleta de la ventana), ajuste de línea y foco visible.
Switch {
    id: control
    // Rótulo para lectores de pantalla cuando el texto visible está fuera
    // del control y no lo precede en la misma fila.
    property string accessibleLabel: ""
    Accessible.name: AccessibleLabel.name(control, text, accessibleLabel)
    Accessible.description: ToolTip.text
    // Pista y mando propios con borde visible en temas claros y oscuros.
    indicator: Rectangle {
        implicitWidth: 42
        implicitHeight: 22
        x: control.mirrored ? control.width - width - control.rightPadding : control.leftPadding
        y: control.topPadding + (control.availableHeight - height) / 2
        radius: height / 2
        color: control.checked ? control.palette.highlight : control.palette.base
        border.color: control.checked ? control.palette.highlight : control.palette.mid
        border.width: 2
        opacity: control.enabled ? 1 : 0.5
        Rectangle {
            width: 14
            height: 14
            radius: 7
            y: (parent.height - height) / 2
            x: control.checked ? parent.width - width - 4 : 4
            color: control.checked ? control.palette.highlightedText : control.palette.mid
            Behavior on x { NumberAnimation { duration: 120 } }
        }
    }
    contentItem: Text {
        text: control.text
        font: control.font
        color: control.palette.windowText
        opacity: control.enabled ? 1 : 0.6
        wrapMode: Text.WordWrap
        verticalAlignment: Text.AlignVCenter
        leftPadding: control.indicator && !control.mirrored ? control.indicator.width + control.spacing : 0
        rightPadding: control.indicator && control.mirrored ? control.indicator.width + control.spacing : 0
    }
    background: Rectangle {
        color: "transparent"
        radius: 4
        border.color: control.palette.highlight
        border.width: control.visualFocus ? 2 : 0
    }
}
