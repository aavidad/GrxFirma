// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Layouts

// Avisos del motor tras firmar o firmar un lote (lote 16d): por ejemplo, una
// cofirma sin el documento original o una firma longeva con una lista de
// revocación que no cubre el certificado. Recibe los textos ya traducidos y
// los pinta como texto plano, con el mismo aspecto que los avisos al
// desproteger. Sin avisos no ocupa sitio.
Rectangle {
    id: root
    property string heading: ""
    property var warnings: []

    visible: root.warnings.length > 0
    Layout.fillWidth: true
    radius: 8
    color: "#4d3b12"
    border.color: "#f2c14e"
    border.width: 1
    implicitHeight: column.implicitHeight + 16
    Accessible.role: Accessible.AlertMessage
    Accessible.name: root.heading
    Accessible.description: root.warnings.join(" ")

    ColumnLayout {
        id: column
        anchors.fill: parent
        anchors.margins: 8
        spacing: 4

        Text {
            Layout.fillWidth: true
            textFormat: Text.PlainText
            text: root.heading
            color: "#f2c14e"
            font.bold: true
            wrapMode: Text.WordWrap
        }
        Repeater {
            model: root.warnings
            delegate: Text {
                Layout.fillWidth: true
                textFormat: Text.PlainText
                text: "• " + String(modelData)
                color: "#ffffff"
                wrapMode: Text.WordWrap
            }
        }
    }
}
