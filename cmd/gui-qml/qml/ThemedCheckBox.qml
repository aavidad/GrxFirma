// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls

// Casilla con el texto del tema (paleta de la ventana), ajuste de línea y foco visible.
CheckBox {
    id: control
    Accessible.name: text
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
