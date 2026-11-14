// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtTest
import "../../qml"

Item {
    id: root
    width: 500
    height: 500
    Component {
        id: factory
        ResponsiveRow {
            width: 360
            property alias combo: combo
            Text { text: "Contenedor"; font.pixelSize: Typography.control }
            ThemedComboBox {
                id: combo
                Layout.preferredWidth: 90
                model: ["JSON (.afp)", "AuthEnvelopedData (.csig)"]
            }
        }
    }
    Component {
        id: editorFactory
        ThemedComboBox { width: 300; editable: true; model: ["https://ejemplo.local/tsa"] }
    }
    TestCase {
        name: "ComboTextWidth"
        when: windowShown
        function cleanup() { Typography.textScalePercent = 100 }
        function test_longest_option_grows_the_control_and_rows_reflow() {
            const row = createTemporaryObject(factory, root)
            verify(row)
            const initialWidth = row.combo.implicitWidth
            Typography.textScalePercent = 200
            wait(0)
            verify(row.combo.implicitWidth > initialWidth)
            compare(row.columns, 1)
            // La redistribución por la fuente nueva se completa al pulir la escena.
            tryVerify(function() { return row.combo.width > 90 })
            compare(row.combo.contentItem.wrapMode, Text.WordWrap)
            verify(!row.combo.contentItem.truncated)
            verify(row.combo.contentItem.contentWidth <= row.combo.contentItem.width + 1)
            row.combo.currentIndex = 1
            wait(0)
            verify(!row.combo.contentItem.truncated)
            verify(row.combo.contentItem.contentWidth <= row.combo.contentItem.width + 1)
        }
        function test_native_editor_survives_scale_and_editability_changes() {
            const combo = createTemporaryObject(editorFactory, root)
            const editor = combo.contentItem
            Typography.textScalePercent = 200
            compare(combo.contentItem, editor)
            combo.editText = "https://otro.local/tsa"
            compare(combo.editText, "https://otro.local/tsa")
            combo.editable = false
            wait(0)
            compare(combo.contentItem.wrapMode, Text.WordWrap)
            combo.editable = true
            wait(0)
            compare(combo.contentItem, editor)
            verify(combo.contentItem !== null)
            Typography.textScalePercent = 100
            compare(combo.contentItem, editor)
        }
    }
}
