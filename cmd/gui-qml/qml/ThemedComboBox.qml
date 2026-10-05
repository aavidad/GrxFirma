// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "ThemeContrast.js" as Contrast

// Desplegable (sobre todo editable) con el borde de ThemedTextField: 3:1 con el
// fondo en todos los temas, foco de 2 px y error con errorColor.
ComboBox {
    id: control
    property bool hasError: false
    property color errorColor: "#b42318"
    readonly property color fieldColor: control.palette.base
    readonly property color borderColor: Contrast.accentOn(control.fieldColor, control.palette.mid, 3.0)
    readonly property color focusBorderColor: Contrast.accentOn(control.fieldColor, control.palette.highlight, 3.0)
    readonly property color errorBorderColor: Contrast.accentOn(control.fieldColor, control.errorColor, 3.0)
    background: Rectangle {
        implicitWidth: 120
        implicitHeight: 36
        radius: 4
        color: control.fieldColor
        opacity: control.enabled ? 1 : 0.6
        border.color: control.hasError ? control.errorBorderColor
                                       : (control.activeFocus ? control.focusBorderColor : control.borderColor)
        border.width: control.hasError || control.activeFocus ? 2 : 1
    }
}
