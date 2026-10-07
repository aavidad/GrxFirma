// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property var theme
    property var translate: function(key) { return key }
    property string labelKey: ""
    property var validation: function(text) { return "" }
    property alias text: input.text
    property alias maximumLength: input.maximumLength
    property bool touched: false
    property string hintKey: ""
    readonly property string errorKey: validation(text)
    // Error y foco salen del tema; los valores por defecto cumplen AA sobre fondo claro.
    readonly property color errorColor: root.theme && root.theme.errorColor ? root.theme.errorColor : "#b42318"
    readonly property color focusColor: root.theme && root.theme.focusColor ? root.theme.focusColor : "#1f5fa8"
    readonly property bool invalid: touched && errorKey !== ""
    spacing: 4
    function validateNow() { touched = true; return errorKey === "" }
    function focusInput() { input.forceActiveFocus() }
    RowLayout {
        Layout.fillWidth: true
        ThemedTextField {
            id: input
            objectName: "eniInput"
            Layout.fillWidth: true
            color: root.theme.textColor
            fieldColor: root.theme.cardColor
            mutedColor: root.theme.secondaryTextColor
            focusColor: root.focusColor
            errorColor: root.errorColor
            hasError: root.invalid
            Accessible.name: root.translate(root.labelKey)
            Accessible.description: root.invalid ? root.translate(root.errorKey)
                                                 : (root.hintKey !== "" ? root.translate(root.hintKey) : "")
            onActiveFocusChanged: if (!activeFocus) root.touched = true
        }
    }
    Label {
        Layout.fillWidth: true
        // Con el error a la vista la pista sobra: el error ya dice qué escribir.
        visible: root.hintKey !== "" && !root.invalid
        text: root.hintKey !== "" ? root.translate(root.hintKey) : ""
        color: root.theme.secondaryTextColor
        wrapMode: Text.WordWrap
    }
    Label {
        Layout.fillWidth: true
        visible: root.invalid
        // El icono evita depender solo del color para reconocer el error.
        text: root.invalid ? "\u26A0 " + root.translate(root.errorKey) : ""
        color: root.errorColor
        wrapMode: Text.WordWrap
        // Rol de alerta: el lector de pantalla anuncia el error al aparecer.
        Accessible.role: Accessible.AlertMessage
        Accessible.name: root.invalid ? root.translate(root.errorKey) : ""
    }
}
