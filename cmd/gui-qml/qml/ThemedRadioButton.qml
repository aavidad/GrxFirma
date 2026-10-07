// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import "AccessibleLabel.js" as AccessibleLabel

// Botón de opción con el texto del tema, ajuste de línea y foco visible
// (mismo criterio de contraste que ThemedCheckBox).
RadioButton {
    id: control
    property string accessibleLabel: ""
    Accessible.name: AccessibleLabel.name(control, text, accessibleLabel)
    Accessible.description: ToolTip.text
    indicator: Rectangle {
        implicitWidth: 20
        implicitHeight: 20
        x: control.mirrored ? control.width - width - control.rightPadding : control.leftPadding
        y: control.topPadding + (control.availableHeight - height) / 2
        radius: 10
        color: control.palette.base
        border.color: control.checked ? control.palette.highlight : control.palette.mid
        border.width: 2
        opacity: control.enabled ? 1 : 0.5
        Rectangle {
            anchors.centerIn: parent
            width: 10
            height: 10
            radius: 5
            color: control.palette.highlight
            visible: control.checked
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
