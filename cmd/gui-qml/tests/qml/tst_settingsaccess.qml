// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtTest
import "../../qml"

// Lote 39g: el diálogo de ajustes sin permiso enseña la explicación entera y
// la orden en un campo de solo lectura que se selecciona con el teclado.
TestCase {
    id: tests
    name: "SettingsAccess"
    width: 900; height: 700
    when: windowShown
    property var theme: ({cardColor: "#ffffff", textColor: "#202124", secondaryTextColor: "#555555", primaryColor: "#2980b9"})
    property var dialog
    readonly property string command: "icacls \"%APPDATA%\\GrxFirma\" /grant \"%USERNAME%:(OI)(CI)F\" /T"
    readonly property string explanation: "GrxFirma cannot open your settings. To fix it, run this command and then open GrxFirma again:"
    Component { id: factory; SettingsAccessDialog { theme: tests.theme; translate: function(key) { return "T:" + key } } }
    function init() { dialog = createTemporaryObject(factory, tests); verify(dialog !== null) }
    function cleanup() { if (dialog) dialog.close() }

    function test_command_is_whole_selectable_and_focused() {
        dialog.present("get_settings", explanation + "\n" + command)
        tryCompare(dialog, "opened", true)
        const text = findChild(dialog, "settingsAccessExplanation")
        const field = findChild(dialog, "settingsAccessCommand")
        compare(text.text, explanation)
        verify(field.visible)
        verify(field.readOnly)
        verify(field.selectByMouse)
        verify(field.activeFocusOnTab)
        compare(field.text, command)
        tryCompare(field, "activeFocus", true)
        compare(field.selectedText, command)
        compare(dialog.title, "T:winui.ajustes.no_se_pudo_cargar")
        compare(dialog.accessibleDescription, explanation)
        // Ctrl+A vuelve a seleccionarla entera después de mover el cursor.
        field.deselect()
        keyClick(Qt.Key_A, Qt.ControlModifier)
        compare(field.selectedText, command)
        // Tab lleva al botón de cerrar.
        keyClick(Qt.Key_Tab)
        tryCompare(findChild(dialog, "settingsAccessClose"), "activeFocus", true)
        // Esc cierra.
        keyClick(Qt.Key_Escape)
        tryCompare(dialog, "visible", false)
    }

    function test_message_without_command_has_no_field() {
        dialog.present("save_settings", "GrxFirma did not save the setting.")
        tryCompare(dialog, "opened", true)
        verify(!findChild(dialog, "settingsAccessCommand").visible)
        compare(findChild(dialog, "settingsAccessExplanation").text, "GrxFirma did not save the setting.")
        compare(dialog.title, "T:winui.ajustes.no_se_pudo_guardar")
        const close = findChild(dialog, "settingsAccessClose")
        tryCompare(close, "activeFocus", true)
        compare(close.text, "T:Cerrar")
        close.clicked()
        tryCompare(dialog, "visible", false)
    }

    function test_long_text_wraps_inside_dialog() {
        const longText = "Ein sehr langer Satz ".repeat(30)
        dialog.present("get_settings", longText + "\n" + command)
        tryCompare(dialog, "opened", true)
        const text = findChild(dialog, "settingsAccessExplanation")
        const field = findChild(dialog, "settingsAccessCommand")
        verify(text.lineCount > 1)
        verify(text.width <= dialog.availableWidth + 1)
        verify(field.width <= dialog.availableWidth + 1)
        verify(!text.truncated)
    }
}
