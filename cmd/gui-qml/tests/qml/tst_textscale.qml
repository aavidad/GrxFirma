// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtTest
import "../../qml"

Item {
    id: root
    width: 800
    height: 600
    Button { id: outside; text: "Fuera" }
    ModalLayer {
        id: layer
        visible: false
        anchors.fill: parent
        initialFocusItem: first
        onEscapePressed: visible = false
        Column {
            ThemedButton { id: first; text: "Primero" }
            ThemedComboBox { id: combo; model: ["Uno", "Dos"] }
            ThemedSpinBox { id: number; from: 0; to: 100 }
            ThemedButton { id: last; text: "Último" }
        }
    }
    ThemedButton { id: growing; y: 500; text: "Texto que debe crecer" }
    TestCase {
        name: "TextScaleAndModalLayer"
        when: windowShown
        function cleanup() { layer.visible = false; Typography.textScalePercent = 100 }
        function test_sizes_and_control_heights_follow_the_scale() {
            const original = Typography.size12
            const originalFont = growing.font.pixelSize
            const originalHeight = growing.implicitHeight
            for (let i = 0; i < Typography.percentages.length; ++i) {
                Typography.textScalePercent = Typography.percentages[i]
                compare(Typography.size12, Math.round(12 * Typography.systemScale * Typography.percentages[i] / 100))
                compare(growing.font.pixelSize, Math.round(Typography.systemPixelSize * Typography.percentages[i] / 100))
            }
            verify(Typography.size12 >= original * 2 - 1)
            verify(growing.font.pixelSize >= originalFont * 2 - 1)
            verify(growing.implicitHeight > originalHeight)
            Typography.textScalePercent = 999
            compare(Typography.userScale, 1)
        }
        function test_system_font_is_respected() {
            const original = Typography.systemMetrics.font
            try {
                Typography.systemMetrics.font = Qt.font({ pixelSize: 26 })
                compare(Typography.systemScale, 2)
                compare(Typography.size12, 24)
                compare(growing.font.pixelSize, 26)
            } finally {
                Typography.systemMetrics.font = original
            }
        }
        function test_tab_backtab_escape_and_return_focus() {
            outside.forceActiveFocus()
            layer.visible = true
            tryCompare(first, "activeFocus", true)
            keyClick(Qt.Key_Tab)
            tryCompare(combo, "activeFocus", true)
            keyClick(Qt.Key_Tab)
            tryCompare(number, "activeFocus", true)
            for (let i = 0; i < 30; ++i) {
                keyClick(Qt.Key_Tab)
                verify(layer.containsItem(root.Window.window.activeFocusItem))
            }
            for (let i = 0; i < 30; ++i) {
                keyClick(Qt.Key_Backtab)
                verify(layer.containsItem(root.Window.window.activeFocusItem))
            }
            keyClick(Qt.Key_Escape)
            tryCompare(layer, "visible", false)
            tryCompare(outside, "activeFocus", true)
        }
        function test_combobox_popup_closes_before_layer() {
            layer.visible = true
            tryCompare(first, "activeFocus", true)
            combo.forceActiveFocus()
            keyClick(Qt.Key_Space)
            tryCompare(combo.popup, "visible", true)
            keyClick(Qt.Key_Escape)
            tryCompare(combo.popup, "visible", false)
            compare(layer.visible, true)
            keyClick(Qt.Key_Escape)
            tryCompare(layer, "visible", false)
        }
    }
}
