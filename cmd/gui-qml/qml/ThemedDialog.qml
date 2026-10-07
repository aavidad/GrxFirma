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
    // Traductor de la aplicación (clave -> texto). Los botones estándar de Qt
    // salen en el idioma del sistema; con él salen en el de la aplicación.
    property var translate: null
    readonly property var standardButtonKeys: [
        [Dialog.Ok, "Aceptar"],
        [Dialog.Cancel, "Cancelar"],
        [Dialog.Close, "Cerrar"],
        [Dialog.Save, "Guardar"],
        [Dialog.Yes, "Sí"],
        [Dialog.No, "No"]
    ]

    function relabelStandardButtons() {
        const translator = dialog.translate
        if (typeof translator !== "function")
            return
        for (let i = 0; i < dialog.standardButtonKeys.length; i++) {
            const button = dialog.standardButton(dialog.standardButtonKeys[i][0])
            if (button)
                button.text = translator(dialog.standardButtonKeys[i][1])
        }
    }

    // El rol, el nombre y la descripción van en la ventana emergente del propio
    // diálogo (el elemento que contiene el fondo). Ponerlos en la cabecera creaba
    // un segundo diálogo anidado para los lectores de pantalla.
    function syncPopupAccessibility() {
        const popupItem = dialog.background ? dialog.background.parent : null
        if (!popupItem)
            return
        popupItem.Accessible.role = Accessible.Dialog
        popupItem.Accessible.name = dialog.accessibleName
        popupItem.Accessible.description = dialog.accessibleDescription
    }

    onTranslateChanged: relabelStandardButtons()
    onStandardButtonsChanged: Qt.callLater(relabelStandardButtons)
    onAccessibleNameChanged: syncPopupAccessibility()
    onAccessibleDescriptionChanged: syncPopupAccessibility()
    onAboutToShow: relabelStandardButtons()
    Component.onCompleted: {
        relabelStandardButtons()
        syncPopupAccessibility()
    }

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
        Label {
            id: titleLabel
            Accessible.role: Accessible.Heading
            Accessible.name: dialog.title
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
