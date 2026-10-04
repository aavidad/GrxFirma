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
    property bool calendarEnabled: false
    readonly property string errorKey: validation(text)
    readonly property bool invalid: touched && errorKey !== ""
    signal calendarRequested()
    spacing: 4
    function validateNow() { touched = true; return errorKey === "" }
    function focusInput() { input.forceActiveFocus() }
    RowLayout {
        Layout.fillWidth: true
        TextField {
            id: input
            objectName: "eniInput"
            Layout.fillWidth: true
            color: root.theme.textColor
            Accessible.name: root.translate(root.labelKey)
            Accessible.description: root.invalid ? root.translate(root.errorKey) : ""
            onActiveFocusChanged: if (!activeFocus) root.touched = true
            background: Rectangle {
                color: root.theme.cardColor
                radius: 4
                border.color: root.invalid ? "#b42318" : root.theme.secondaryTextColor
                border.width: root.invalid ? 2 : 1
            }
        }
        Button {
            visible: root.calendarEnabled
            text: root.translate("eni.validacion.calendar")
            Accessible.name: text
            onClicked: root.calendarRequested()
        }
    }
    Label {
        Layout.fillWidth: true
        visible: root.invalid
        text: root.invalid ? root.translate(root.errorKey) : ""
        color: "#b42318"
        wrapMode: Text.WordWrap
        Accessible.role: Accessible.StaticText
    }
}
