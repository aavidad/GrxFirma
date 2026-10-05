// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "ThemeContrast.js" as Contrast

// Área de texto con el mismo aspecto que ThemedTextField: borde de 3:1 en todos
// los temas, foco de 2 px, error con errorColor y texto plano por defecto.
TextArea {
    id: control
    property bool hasError: false
    property color errorColor: "#b42318"
    property color focusColor: control.palette.highlight
    property color fieldColor: control.palette.base
    // Color de partida del borde; se ajusta hasta 3:1 con el fondo del campo.
    property color mutedColor: control.palette.mid
    readonly property color borderColor: Contrast.accentOn(control.fieldColor, control.mutedColor, 3.0)
    readonly property color focusBorderColor: Contrast.accentOn(control.fieldColor, control.focusColor, 3.0)
    readonly property color errorBorderColor: Contrast.accentOn(control.fieldColor, control.errorColor, 3.0)
    textFormat: TextEdit.PlainText
    selectByMouse: true
    color: Contrast.readableOn(control.fieldColor, control.palette.text)
    placeholderTextColor: Contrast.accentOn(control.fieldColor, control.palette.placeholderText, 4.5)
    selectionColor: control.palette.highlight
    selectedTextColor: control.palette.highlightedText
    background: Rectangle {
        implicitWidth: 200
        implicitHeight: 40
        radius: 4
        color: control.fieldColor
        opacity: control.enabled ? 1 : 0.6
        border.color: control.hasError ? control.errorBorderColor
                                       : (control.activeFocus ? control.focusBorderColor : control.borderColor)
        border.width: control.hasError || control.activeFocus ? 2 : 1
    }
}
