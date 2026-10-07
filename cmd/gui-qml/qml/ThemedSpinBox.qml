// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "ThemeContrast.js" as Contrast

// Selector numérico con el borde de ThemedTextField: el de Fusion se queda
// entre 1:1 y 2:1 con el fondo en varios temas. Aquí llega a 3:1 (WCAG 1.4.11)
// y el foco se ve con un borde de 2 px.
SpinBox {
    id: control
    readonly property color fieldColor: control.palette.base
    readonly property color borderColor: Contrast.accentOn(control.fieldColor, control.palette.mid, 3.0)
    readonly property color focusBorderColor: Contrast.accentOn(control.fieldColor, control.palette.highlight, 3.0)
    background: Rectangle {
        implicitWidth: 120
        implicitHeight: 36
        radius: 4
        color: control.fieldColor
        opacity: control.enabled ? 1 : 0.6
        border.color: control.activeFocus ? control.focusBorderColor : control.borderColor
        border.width: control.activeFocus ? 2 : 1
    }
}
