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
    width: 400
    height: 400

    // Paletas de un tema oscuro y uno claro, como las fija main.qml.
    readonly property var themes: [
        { card: "#1c1f26", text: "#ffffff", muted: "#bdc3c7", focus: "#3498db" },
        { card: "#415a77", text: "#ffffff", muted: "#e0e1dd", focus: "#ffd166" },
        { card: "#ffffff", text: "#2c3e50", muted: "#5f6c6d", focus: "#2980b9" },
        { card: "#0a140a", text: "#00ff00", muted: "#00aa00", focus: "#00ff00" }
    ]

    Component {
        id: fieldComponent
        ThemedTextField { width: 200 }
    }
    Component {
        id: areaComponent
        ThemedTextArea { width: 200 }
    }
    Component {
        id: rowComponent
        AdaptiveRow {
            width: 200
            Rectangle { implicitWidth: 120; width: implicitWidth; height: 20 }
            Rectangle { implicitWidth: 120; width: implicitWidth; height: 20 }
        }
    }

    // Fila de Configuración: el rótulo es un Text aparte del interruptor.
    Component {
        id: settingsRowComponent
        Row {
            property alias toggle: rowSwitch
            property alias box: rowCheck
            property alias label: rowLabel
            Text { id: rowLabel; text: "Recordar último certificado"; wrapMode: Text.WordWrap }
            ThemedSwitch { id: rowSwitch }
            Text { text: "Otro rótulo posterior"; wrapMode: Text.WordWrap }
            ThemedCheckBox { id: rowCheck }
        }
    }
    Component {
        id: labelledSwitchComponent
        ThemedSwitch { text: "Sello visible"; accessibleLabel: "No se usa" }
    }

    TestCase {
        name: "ThemedFields"
        when: windowShown

        function applyTheme(control, t) {
            control.palette.base = t.card
            control.palette.text = t.text
            control.palette.mid = t.muted
            control.palette.placeholderText = t.muted
            control.palette.highlight = t.focus
        }

        function test_border_and_text_contrast_in_every_theme() {
            for (let i = 0; i < root.themes.length; i++) {
                const t = root.themes[i]
                const comps = [fieldComponent, areaComponent]
                for (let c = 0; c < comps.length; c++) {
                    const control = createTemporaryObject(comps[c], root)
                    applyTheme(control, t)
                    verify(Contrast.ratio(t.card, control.background.border.color) >= 3.0, "borde " + t.card)
                    verify(Contrast.ratio(t.card, control.focusBorderColor) >= 3.0, "foco " + t.card)
                    verify(Contrast.ratio(t.card, control.color) >= 4.5, "texto " + t.card)
                    verify(Contrast.ratio(t.card, control.placeholderTextColor) >= 4.5, "marcador " + t.card)
                }
            }
        }

        function test_focus_and_error_change_the_border() {
            const field = createTemporaryObject(fieldComponent, root)
            applyTheme(field, root.themes[0])
            compare(field.background.border.width, 1)
            field.forceActiveFocus()
            compare(field.background.border.width, 2)
            compare(String(field.background.border.color), String(field.focusBorderColor))
            field.errorColor = "#ff8a80"
            field.hasError = true
            compare(String(field.background.border.color), String(field.errorBorderColor))
            verify(Contrast.ratio(root.themes[0].card, field.errorBorderColor) >= 3.0)
        }

        function test_textarea_is_plain_text_by_default() {
            const area = createTemporaryObject(areaComponent, root)
            compare(area.textFormat, TextEdit.PlainText)
        }

        function test_switch_and_checkbox_take_the_row_label_as_accessible_name() {
            const row = createTemporaryObject(settingsRowComponent, root)
            compare(row.toggle.Accessible.name, "Recordar último certificado")
            compare(row.box.Accessible.name, "Otro rótulo posterior")
            row.label.text = "Rótulo cambiado"
            compare(row.toggle.Accessible.name, "Rótulo cambiado")
            row.toggle.accessibleLabel = "Rótulo explícito"
            compare(row.toggle.Accessible.name, "Rótulo explícito")
            const own = createTemporaryObject(labelledSwitchComponent, root)
            compare(own.Accessible.name, "Sello visible")
        }

        function test_adaptive_row_wraps_when_narrow() {
            const row = createTemporaryObject(rowComponent, root)
            waitForRendering(row)
            verify(row.children[1].y > row.children[0].y)
            row.width = 300
            waitForRendering(row)
            compare(row.children[1].y, row.children[0].y)
        }
    }
}
