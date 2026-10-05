// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls

Dialog {
    id: dialog
    required property var theme
    property string accessibleName: title
    property string accessibleDescription: ""

    palette.window: theme.cardColor
    palette.windowText: theme.textColor
    palette.base: theme.cardColor
    palette.alternateBase: theme.cardColor
    palette.text: theme.textColor
    palette.button: theme.cardColor
    palette.buttonText: theme.textColor
    palette.highlight: theme.textColor
    palette.highlightedText: theme.cardColor
    palette.placeholderText: theme.secondaryTextColor
    palette.toolTipBase: theme.cardColor
    palette.toolTipText: theme.textColor
    palette.disabled.text: theme.secondaryTextColor
    palette.disabled.buttonText: theme.secondaryTextColor

    background: Rectangle {
        color: dialog.theme.cardColor
        border.color: dialog.theme.secondaryTextColor
        radius: 8
    }
    header: Item {
        implicitWidth: 320
        implicitHeight: titleLabel.implicitHeight
        visible: dialog.title !== ""
        property alias color: titleLabel.color
        Accessible.role: Accessible.Dialog
        Accessible.name: dialog.accessibleName
        Accessible.description: dialog.accessibleDescription
        Label {
            id: titleLabel
            width: parent.width
            text: dialog.title
            color: dialog.theme.textColor
            padding: 12
            font.bold: true
            wrapMode: Text.Wrap
        }
    }
    footer: DialogButtonBox {
        visible: dialog.standardButtons !== Dialog.NoButton
        standardButtons: dialog.standardButtons
        delegate: Button {
            id: button
            implicitHeight: 44
            background: Rectangle {
                color: button.down ? dialog.theme.textColor : dialog.theme.cardColor
                // Foco de teclado visible y distinto del borde normal.
                border.color: button.activeFocus ? (dialog.theme.focusColor ? dialog.theme.focusColor : dialog.theme.textColor)
                                                 : dialog.theme.secondaryTextColor
                border.width: button.activeFocus ? 2 : 1
                radius: 4
            }
            contentItem: Text {
                text: button.text
                font: button.font
                color: button.down ? dialog.theme.cardColor : dialog.theme.textColor
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
            }
        }
    }
}
