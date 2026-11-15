// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

// Ajustes que no se pueden leer o guardar por permisos (lote 39g). El texto
// viene del catálogo; si lleva una orden, va sola en la última línea y aquí se
// enseña en un campo de solo lectura que se selecciona y copia con el ratón o
// con el teclado (al recibir el foco queda seleccionada entera).
ThemedDialog {
    id: dialog
    required theme
    property string action: ""
    property string message: ""
    readonly property int commandBreak: message.lastIndexOf("\n")
    readonly property string explanation: commandBreak < 0 ? message : message.slice(0, commandBreak)
    readonly property string command: commandBreak < 0 ? "" : message.slice(commandBreak + 1).trim()
    function appText(key) { return dialog.translate ? dialog.translate(key) : key }
    function present(operation, text) {
        action = String(operation || "")
        message = String(text || "")
        dialog.open()
    }
    modal: true
    focus: true
    closePolicy: Popup.CloseOnEscape
    anchors.centerIn: parent
    width: Math.min(640, (parent ? parent.width : 640) - 32)
    title: appText(action === "save_settings" ? "winui.ajustes.no_se_pudo_guardar" : "winui.ajustes.no_se_pudo_cargar")
    accessibleName: title
    accessibleDescription: explanation
    onOpened: (dialog.command !== "" ? commandField : closeButton).forceActiveFocus(Qt.TabFocusReason)

    ColumnLayout {
        width: dialog.availableWidth
        spacing: 12
        Text {
            objectName: "settingsAccessExplanation"
            Layout.fillWidth: true
            text: dialog.explanation
            textFormat: Text.PlainText
            font.pixelSize: Typography.control
            color: dialog.theme.textColor
            wrapMode: Text.WordWrap
            Accessible.role: Accessible.StaticText
            Accessible.name: text
        }
        ThemedTextArea {
            id: commandField
            objectName: "settingsAccessCommand"
            Layout.fillWidth: true
            visible: dialog.command !== ""
            text: dialog.command
            textFormat: TextEdit.PlainText
            readOnly: true
            activeFocusOnTab: true
            // Es una orden de consola: letra de ancho fijo y cortes en
            // cualquier carácter para que se vea entera sin desplazarse.
            font.family: "monospace"
            wrapMode: TextEdit.WrapAnywhere
            Accessible.name: dialog.command
            onActiveFocusChanged: if (activeFocus) commandField.selectAll()
        }
        ThemedButton {
            id: closeButton
            objectName: "settingsAccessClose"
            Layout.alignment: Qt.AlignRight
            Layout.minimumHeight: 44
            Layout.minimumWidth: 120
            text: dialog.appText("Cerrar")
            onClicked: dialog.close()
        }
    }
}
