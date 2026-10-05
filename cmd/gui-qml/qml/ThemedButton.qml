// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "ThemeContrast.js" as Contrast

// Botón con los colores del tema (los toma de la paleta de la ventana) y un anillo
// de foco de 2 px. Los botones principales fijan palette.button; el texto se elige
// para que se lea con contraste AA sobre ese fondo.
Button {
    id: control
    Accessible.name: text
    hoverEnabled: true
    // Color del borde cuando el botón señala un error (transparente si no lo hay).
    property color alertColor: "transparent"
    readonly property color baseFill: Contrast.legibleFill(control.palette.button)
    readonly property color fillColor: control.down
        ? Qt.darker(control.baseFill, 1.25)
        : (control.hovered && control.enabled ? Qt.darker(control.baseFill, 1.08) : control.baseFill)
    readonly property color labelColor: Contrast.readableOn(control.flat ? control.palette.window : control.fillColor,
                                                             control.palette.buttonText)
    contentItem: Text {
        text: control.text
        font: control.font
        color: control.labelColor
        opacity: control.enabled ? 1 : 0.6
        wrapMode: Text.WordWrap
        elide: Text.ElideRight
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
    }
    background: Rectangle {
        implicitWidth: 64
        implicitHeight: 36
        radius: 4
        color: control.flat && !control.down && !control.hovered ? "transparent" : control.fillColor
        opacity: control.enabled ? 1 : 0.7
        border.color: control.activeFocus ? control.palette.highlight
                                          : (control.alertColor.a > 0 ? control.alertColor : control.palette.mid)
        border.width: control.activeFocus || control.alertColor.a > 0 ? 2 : (control.flat ? 0 : 1)
    }
}
