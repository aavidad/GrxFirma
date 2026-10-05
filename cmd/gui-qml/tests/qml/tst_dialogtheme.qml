// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtTest
import "../../qml"

TestCase {
    id: testCase
    name: "DialogTheme"
    width: 700
    height: 800
    when: windowShown
    property var theme: ({cardColor: "#1c1f26", textColor: "#ffffff", secondaryTextColor: "#bdc3c7"})
    property var dialog
    Component {
        id: factory
        ThemedDialog {
            theme: testCase.theme
            title: "Test"
            standardButtons: Dialog.Ok | Dialog.Cancel
            contentItem: Label { text: "Test" }
        }
    }
    Component {
        id: translatedFactory
        ThemedDialog {
            theme: testCase.theme
            title: "Test"
            standardButtons: Dialog.Ok | Dialog.Cancel
            translate: function(key) { return "[" + key + "]" }
            contentItem: Label { text: "Test" }
        }
    }
    SignalSpy { id: acceptedSpy; target: testCase.dialog; signalName: "accepted" }
    SignalSpy { id: rejectedSpy; target: testCase.dialog; signalName: "rejected" }
    function init() {
        theme = {cardColor: "#1c1f26", textColor: "#ffffff", secondaryTextColor: "#bdc3c7"}
        dialog = createTemporaryObject(factory, testCase)
        verify(dialog !== null)
        acceptedSpy.clear()
        rejectedSpy.clear()
        dialog.open()
        tryCompare(dialog, "opened", true)
    }
    function test_colors_follow_dark_and_light_theme() {
        compare(dialog.background.color, theme.cardColor)
        compare(dialog.header.color, theme.textColor)
        compare(dialog.contentItem.palette.text, theme.textColor)
        compare(dialog.standardButton(Dialog.Ok).contentItem.color, theme.textColor)
        theme = {cardColor: "#ffffff", textColor: "#2c3e50", secondaryTextColor: "#5f6c6d"}
        compare(dialog.background.color, theme.cardColor)
        compare(dialog.header.color, theme.textColor)
        compare(dialog.contentItem.palette.text, theme.textColor)
        compare(dialog.standardButton(Dialog.Ok).contentItem.color, theme.textColor)
    }
    function test_standard_button_accepts() {
        mouseClick(dialog.standardButton(Dialog.Ok))
        compare(acceptedSpy.count, 1)
    }
    function test_standard_button_rejects() {
        mouseClick(dialog.standardButton(Dialog.Cancel))
        compare(rejectedSpy.count, 1)
    }
    // Los botones estándar salen en el idioma de la aplicación, no en el del sistema.
    function test_standard_buttons_use_application_translator() {
        const translated = createTemporaryObject(translatedFactory, testCase)
        translated.open()
        tryCompare(translated, "opened", true)
        compare(translated.standardButton(Dialog.Ok).text, "[Aceptar]")
        compare(translated.standardButton(Dialog.Cancel).text, "[Cancelar]")
        translated.translate = function(key) { return "<" + key + ">" }
        compare(translated.standardButton(Dialog.Ok).text, "<Aceptar>")
        translated.close()
    }
}
