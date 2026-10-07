// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "ThemeContrast.js" as Contrast

// Deslizador horizontal con el carril y el tirador a 3:1 con el fondo
// (WCAG 1.4.11) en todos los temas; el de Fusion no llega en los oscuros.
// El foco se marca engrosando el borde del tirador.
Slider {
    id: control
    readonly property color surfaceColor: control.palette.window
    readonly property color trackColor: Contrast.accentOn(control.surfaceColor, control.palette.mid, 3.0)
    readonly property color fillColor: Contrast.accentOn(control.surfaceColor, control.palette.highlight, 3.0)
    background: Rectangle {
        x: control.leftPadding
        y: control.topPadding + control.availableHeight / 2 - height / 2
        implicitWidth: 200
        implicitHeight: 4
        width: control.availableWidth
        height: implicitHeight
        radius: 2
        color: control.trackColor
        opacity: control.enabled ? 1 : 0.6
        Rectangle {
            width: control.visualPosition * parent.width
            height: parent.height
            radius: 2
            color: control.fillColor
        }
    }
    handle: Rectangle {
        x: control.leftPadding + control.visualPosition * (control.availableWidth - width)
        y: control.topPadding + control.availableHeight / 2 - height / 2
        implicitWidth: 22
        implicitHeight: 22
        radius: 11
        color: control.pressed ? Qt.darker(control.palette.base, 1.1) : control.palette.base
        border.color: control.activeFocus ? control.fillColor : control.trackColor
        border.width: control.activeFocus ? 3 : 2
    }
}
