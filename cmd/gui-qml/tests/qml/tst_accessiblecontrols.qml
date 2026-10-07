// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtTest
import "../../qml"
import "../../qml/ThemeContrast.js" as Contrast

// Controles temáticos de la auditoría WCAG: borde a 3:1 (1.4.11), un único
// rol de diálogo con nombre y desplazamiento de la ayuda con el teclado.
Item {
    id: root
    width: 500
    height: 500

    // Mismas superficies que main.qml: oscuro, intermedio, claro y terminal.
    readonly property var themes: [
        { card: "#1c1f26", text: "#ffffff", muted: "#bdc3c7", focus: "#3498db" },
        { card: "#415a77", text: "#ffffff", muted: "#e0e1dd", focus: "#ffd166" },
        { card: "#ffffff", text: "#2c3e50", muted: "#5f6c6d", focus: "#2980b9" },
        { card: "#0a140a", text: "#00ff00", muted: "#00aa00", focus: "#00ff00" }
    ]
    readonly property var theme: ({ cardColor: "#ffffff", textColor: "#2c3e50", secondaryTextColor: "#5f6c6d", focusColor: "#2980b9" })

    Component { id: spinComponent; ThemedSpinBox { from: 0; to: 10 } }
    Component { id: sliderComponent; ThemedSlider { width: 200; from: 0; to: 100; value: 40 } }
    Component { id: comboComponent; ThemedComboBox { model: ["Uno", "Dos"] } }
    Component { id: checkComponent; ThemedCheckBox { text: "Casilla" } }
    Component {
        id: dialogComponent
        ThemedDialog {
            theme: root.theme
            title: "Contraseña del certificado"
            accessibleDescription: "Introduzca la contraseña"
            standardButtons: Dialog.Ok
            contentItem: Label { text: "Contenido" }
        }
    }
    Component {
        id: helpComponent
        HelpButton {
            nameTemplate: "Ayuda sobre %1"
            controlLabel: "Campo"
            helpText: "Primera frase.\nSegunda frase.\nTercera frase.\nCuarta frase.\nQuinta frase.\nSexta frase.\nSéptima frase.\nOctava frase.\nNovena frase.\nDécima frase.\nUndécima frase.\nDuodécima frase.\nDecimotercera frase.\nDecimocuarta frase.\nDecimoquinta frase."
        }
    }

    function applyTheme(control, t) {
        control.palette.base = t.card
        control.palette.window = t.card
        control.palette.text = t.text
        control.palette.mid = t.muted
        control.palette.highlight = t.focus
    }

    TestCase {
        name: "AccessibleControls"
        when: windowShown

        function test_spinbox_and_combobox_borders_reach_three_to_one() {
            const comps = [spinComponent, comboComponent]
            for (let c = 0; c < comps.length; ++c) {
                for (let i = 0; i < root.themes.length; ++i) {
                    const control = createTemporaryObject(comps[c], root)
                    root.applyTheme(control, root.themes[i])
                    verify(Contrast.ratio(control.background.color, control.background.border.color) >= 3.0,
                           "borde " + c + " en el tema " + i)
                }
            }
        }

        function test_slider_track_and_handle_reach_three_to_one() {
            for (let i = 0; i < root.themes.length; ++i) {
                const slider = createTemporaryObject(sliderComponent, root)
                root.applyTheme(slider, root.themes[i])
                verify(Contrast.ratio(root.themes[i].card, slider.background.color) >= 3.0, "carril en el tema " + i)
                verify(Contrast.ratio(root.themes[i].card, slider.handle.border.color) >= 3.0, "tirador en el tema " + i)
            }
        }

        function test_checkbox_indicator_reaches_three_to_one() {
            for (let i = 0; i < root.themes.length; ++i) {
                const box = createTemporaryObject(checkComponent, root)
                root.applyTheme(box, root.themes[i])
                verify(Contrast.ratio(root.themes[i].card, box.indicator.border.color) >= 3.0, "casilla en el tema " + i)
                compare(box.Accessible.name, "Casilla")
            }
        }

        function test_dialog_has_one_named_dialog_and_a_heading() {
            const dialog = createTemporaryObject(dialogComponent, root)
            dialog.open()
            tryCompare(dialog, "opened", true)
            const popupItem = dialog.background.parent
            compare(popupItem.Accessible.role, Accessible.Dialog)
            compare(popupItem.Accessible.name, "Contraseña del certificado")
            compare(popupItem.Accessible.description, "Introduzca la contraseña")
            verify(dialog.header.Accessible.role !== Accessible.Dialog)
            compare(dialog.header.children[0].Accessible.role, Accessible.Heading)
            dialog.accessibleName = "Otro nombre"
            compare(popupItem.Accessible.name, "Otro nombre")
            dialog.close()
        }

        function test_help_text_scrolls_with_the_keyboard() {
            const button = createTemporaryObject(helpComponent, root)
            button.forceActiveFocus()
            button.toggle()
            const popup = findChild(button, "helpPopup")
            tryCompare(popup, "opened", true)
            const scroll = popup.contentItem
            // Sin ampliar ya cabe; se fuerza una altura pequeña para desplazar.
            scroll.height = 60
            verify(scroll.contentHeight > scroll.height)
            compare(scroll.contentY, 0)
            keyClick(Qt.Key_Down)
            verify(scroll.contentY > 0, "la flecha abajo desplaza la ayuda")
            keyClick(Qt.Key_PageDown)
            const afterPage = scroll.contentY
            keyClick(Qt.Key_PageUp)
            verify(scroll.contentY < afterPage, "Re Pág sube")
            popup.close()
        }
    }
}
