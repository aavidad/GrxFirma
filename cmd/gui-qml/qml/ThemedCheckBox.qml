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
    // Recuadro propio: el de Fusion pierde el borde sobre fondos oscuros (contraste < 3:1).
    indicator: Rectangle {
        implicitWidth: 20
        implicitHeight: 20
        x: control.mirrored ? control.width - width - control.rightPadding : control.leftPadding
        y: control.topPadding + (control.availableHeight - height) / 2
        radius: 3
        color: control.checkState !== Qt.Unchecked ? control.palette.highlight : control.palette.base
        border.color: control.checkState !== Qt.Unchecked ? control.palette.highlight : control.palette.mid
        border.width: 2
        opacity: control.enabled ? 1 : 0.5
        Text {
            anchors.centerIn: parent
            text: control.checkState === Qt.PartiallyChecked ? "\u2013" : "\u2713"
            visible: control.checkState !== Qt.Unchecked
            color: control.palette.highlightedText
            font.bold: true
            font.pixelSize: 14
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
