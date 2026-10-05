// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "ThemeContrast.js" as Contrast

// Campo de texto con los colores del tema (paleta de la ventana). El de Fusion
// pierde el borde sobre fondos oscuros; aquí el borde llega a 3:1 con el fondo
// del campo en todos los temas, el foco se ve con un borde de 2 px y, si el
// campo tiene un error, el borde pasa al color de error.
TextField {
    id: control
    // Marca el campo como erróneo (borde de error de 2 px).
    property bool hasError: false
    property color errorColor: "#b42318"
    property color focusColor: control.palette.highlight
    property color fieldColor: control.palette.base
    // Color de partida del borde; se ajusta hasta 3:1 con el fondo del campo.
    property color mutedColor: control.palette.mid
    readonly property color borderColor: Contrast.accentOn(control.fieldColor, control.mutedColor, 3.0)
    readonly property color focusBorderColor: Contrast.accentOn(control.fieldColor, control.focusColor, 3.0)
    readonly property color errorBorderColor: Contrast.accentOn(control.fieldColor, control.errorColor, 3.0)
    selectByMouse: true
    color: Contrast.readableOn(control.fieldColor, control.palette.text)
    placeholderTextColor: Contrast.accentOn(control.fieldColor, control.palette.placeholderText, 4.5)
    selectionColor: control.palette.highlight
    selectedTextColor: control.palette.highlightedText
    background: Rectangle {
        implicitWidth: 120
        implicitHeight: 36
        radius: 4
        color: control.fieldColor
        opacity: control.enabled ? 1 : 0.6
        border.color: control.hasError ? control.errorBorderColor
                                       : (control.activeFocus ? control.focusBorderColor : control.borderColor)
        border.width: control.hasError || control.activeFocus ? 2 : 1
        // Con error y foco a la vez, el foco se marca con un anillo exterior.
        Rectangle {
            anchors.fill: parent
            anchors.margins: -3
            radius: 6
            color: "transparent"
            border.color: control.focusBorderColor
            border.width: 2
            visible: control.hasError && control.activeFocus
        }
    }
}
