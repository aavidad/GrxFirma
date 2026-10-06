// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtTest
import "../../qml"
import "../../qml/ThemeContrast.js" as Contrast

Item {
    id: root
    width: 480
    height: 480

    readonly property var themes: [
        { card: "#1c1f26", text: "#ffffff", muted: "#bdc3c7" },
        { card: "#ffffff", text: "#2c3e50", muted: "#5f6c6d" },
        { card: "#415a77", text: "#ffffff", muted: "#e0e1dd" }
    ]

    Component {
        id: helpComponent
        Column {
            property alias help: helpButton
            property alias other: otherButton
            Button { id: otherButton; text: "x" }
            HelpButton {
                id: helpButton
                accessibleName: "Nombre de prueba"
                title: "Título"
                paragraphs: ["Uno.", "Dos."]
                emphasizeLast: true
            }
        }
    }

    TestCase {
        name: "HelpButton"
        when: windowShown

        function test_accessible_name_and_description_carry_the_help() {
            const column = createTemporaryObject(helpComponent, root)
            compare(column.help.Accessible.name, "Nombre de prueba")
            compare(column.help.Accessible.description, "Título Uno. Dos.")
            verify(column.help.implicitWidth >= 24 && column.help.implicitHeight >= 24)
        }

        function test_click_toggles_and_escape_closes() {
            const column = createTemporaryObject(helpComponent, root)
            waitForRendering(column)
            mouseClick(column.help)
            tryCompare(column.help, "popupOpen", true)
            column.help.forceActiveFocus()
            keyClick(Qt.Key_Escape)
            tryCompare(column.help, "popupOpen", false)
            mouseClick(column.help)
            tryCompare(column.help, "popupOpen", true)
            mouseClick(column.help)
            tryCompare(column.help, "popupOpen", false)
        }

        function test_keyboard_focus_opens_and_leaving_closes() {
            const column = createTemporaryObject(helpComponent, root)
            waitForRendering(column)
            column.other.forceActiveFocus()
            keyClick(Qt.Key_Tab)
            tryCompare(column.help, "activeFocus", true)
            tryCompare(column.help, "popupOpen", true)
            keyClick(Qt.Key_Tab)
            tryCompare(column.help, "popupOpen", false)
        }

        function test_label_and_popup_text_reach_aa_in_every_theme() {
            for (let i = 0; i < root.themes.length; i++) {
                const t = root.themes[i]
                const column = createTemporaryObject(helpComponent, root)
                column.help.palette.window = t.card
                column.help.palette.windowText = t.text
                column.help.palette.base = t.card
                column.help.palette.text = t.text
                verify(Contrast.ratio(t.card, column.help.labelColor) >= 4.5, "rótulo " + t.card)
                const popup = findChild(column.help, "helpPopup")
                verify(popup !== null)
                verify(Contrast.ratio(t.card, popup.textColor) >= 4.5, "texto " + t.card)
            }
        }
    }
}
