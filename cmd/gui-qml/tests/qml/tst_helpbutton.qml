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

    // Los catorce temas de main.qml (cardColor y textColor), más un fondo
    // gris medio en el que ni el texto del tema ni el negro llegan a 4,5:1.
    readonly property var themes: [
        { card: "#1c1f26", text: "#ffffff" }, { card: "#ffffff", text: "#2c3e50" },
        { card: "#0d0d0d", text: "#ffffff" }, { card: "#415a77", text: "#ffffff" },
        { card: "#12141a", text: "#ffffff" }, { card: "#1a241a", text: "#ecf0f1" },
        { card: "#3a2228", text: "#fff9f9" }, { card: "#112240", text: "#e6f1ff" },
        { card: "#1e0f0f", text: "#ffffff" }, { card: "#241738", text: "#ffffff" },
        { card: "#f0ebe1", text: "#2c2a26" }, { card: "#2a213a", text: "#f8f5fd" },
        { card: "#0a140a", text: "#00ff00" }, { card: "#3a2822", text: "#f4f1de" },
        { card: "#8a8a8a", text: "#9a9a9a" }
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

    // Uso habitual: plantilla del catálogo, etiqueta visible y un párrafo.
    Component {
        id: contextualComponent
        Column {
            property alias help: contextualHelp
            property alias other: contextOther
            Button { id: contextOther; text: "x" }
            HelpButton {
                id: contextualHelp
                nameTemplate: "Ayuda sobre {0}"
                controlLabel: "⏳  Servidor TSA:"
                helpText: "Dirección del servicio de sellado de tiempo."
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

        function test_keyboard_focus_shows_the_name_without_opening() {
            const column = createTemporaryObject(helpComponent, root)
            waitForRendering(column)
            column.other.forceActiveFocus()
            keyClick(Qt.Key_Tab)
            tryCompare(column.help, "activeFocus", true)
            // Recorrer un formulario con el tabulador no abre cada «?».
            wait(50)
            compare(column.help.popupOpen, false)
            keyClick(Qt.Key_Space)
            tryCompare(column.help, "popupOpen", true)
            keyClick(Qt.Key_Tab)
            tryCompare(column.help, "popupOpen", false)
        }

        function test_name_template_space_enter_and_escape() {
            const column = createTemporaryObject(contextualComponent, root)
            waitForRendering(column)
            const help = column.help
            // Sin icono inicial ni dos puntos finales.
            compare(help.Accessible.name, "Ayuda sobre Servidor TSA")
            compare(help.Accessible.description, "Dirección del servicio de sellado de tiempo.")
            help.forceActiveFocus()
            keyClick(Qt.Key_Space)
            tryCompare(help, "popupOpen", true)
            const popup = findChild(help, "helpPopup")
            verify(popup.contentItem.children.length > 0)
            let found = false
            for (let i = 0; i < popup.contentItem.children.length; i++) {
                const child = popup.contentItem.children[i]
                if (child.text === "Dirección del servicio de sellado de tiempo." && child.visible) found = true
            }
            verify(found, "el texto de ayuda se ve en la ventana flotante")
            keyClick(Qt.Key_Escape)
            tryCompare(help, "popupOpen", false)
            verify(help.activeFocus, "el foco vuelve al «?» al cerrar")
            keyClick(Qt.Key_Return)
            tryCompare(help, "popupOpen", true)
            keyClick(Qt.Key_Escape)
            tryCompare(help, "popupOpen", false)
            keyClick(Qt.Key_Enter)
            tryCompare(help, "popupOpen", true)
            keyClick(Qt.Key_Enter)
            tryCompare(help, "popupOpen", false)
        }

        function test_popup_fits_a_narrow_window() {
            const column = createTemporaryObject(contextualComponent, root)
            waitForRendering(column)
            column.x = root.width - column.help.width
            column.help.forceActiveFocus()
            keyClick(Qt.Key_Space)
            const popup = findChild(column.help, "helpPopup")
            tryCompare(popup, "opened", true)
            verify(popup.width <= root.width, "no más ancha que la ventana")
            const topLeft = popup.contentItem.mapToItem(root, 0, 0)
            verify(topLeft.x >= 0, "no se sale por la izquierda")
            verify(topLeft.x + popup.contentItem.width <= root.width, "no se sale por la derecha")
            verify(column.help.implicitWidth >= 24 && column.help.implicitHeight >= 24, "objetivo táctil")
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
