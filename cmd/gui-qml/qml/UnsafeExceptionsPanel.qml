// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "UnsafeText.js" as UnsafeText
import "ThemeContrast.js" as Contrast

GroupBox {
    id: panel
    required property var theme
    required property var bridge
    required property var translate
    required property bool localIpc
    property string localeName: Qt.locale().name
    property var items: []
    property bool busy: false
    property bool loaded: false
    property string message: ""
    // Caso que se acaba de activar o desactivar: al llegar la lista nueva se
    // confirma en la región viva («Evaluar firmas SHA-1 antiguas: Desactivado»)
    // y el foco vuelve a su fila, como en WinUI.
    property string changedId: ""
    signal acceptanceRequested(var item)
    signal helpRequested(var item)
    implicitWidth: 320
    title: t("excepciones.pantalla.preferencias")
    Accessible.name: title
    label: Text {
        x: panel.leftPadding
        y: panel.topPadding - implicitHeight - panel.spacing
        width: panel.availableWidth
        text: panel.title
        textFormat: Text.PlainText
        font.pixelSize: Typography.size18
        font.bold: true
        color: panel.theme.textColor
        wrapMode: Text.WordWrap
        Accessible.role: Accessible.Heading
    }
    function t(key) { return panel.translate(key) }
    function load() {
        if (!localIpc || busy) return
        busy = true
        bridge.loadUnsafeExceptions()
    }
    function change(item) {
        if (busy || item.fixed || !localIpc) return
        if (item.enabled) { busy = true; changedId = item.id; bridge.disableUnsafeException(item.id) }
        else acceptanceRequested(item)
    }
    // Tras activar desde el aviso abierto en este apartado.
    function confirmChange(id) {
        if (!localIpc) return
        changedId = id || ""
        if (busy) return
        busy = true
        bridge.loadUnsafeExceptions()
    }
    function announce(text) {
        if (text !== "" && typeof statusLine.Accessible.announce === "function") statusLine.Accessible.announce(text)
    }
    function focusRow(id) {
        for (let i = 0; i < rows.count; i++) {
            const row = rows.itemAt(i)
            if (!row || row.modelData.id !== id) continue
            const target = row.changeButton.visible && row.changeButton.enabled ? row.changeButton : row.helpButton
            target.forceActiveFocus(Qt.OtherFocusReason)
            return
        }
    }
    function stateText(item) {
        if (item.policy_unreadable) return t("excepciones.pantalla.directiva_ilegible")
        if (item.fixed) return t(item.enabled ? "excepciones.pantalla.fijada_activa" : "excepciones.pantalla.fijada_inactiva")
        if (!item.enabled) return t("excepciones.pantalla.desactivada")
        const receipt = item.last_acceptance
        // Mismo formato que WinUI, con el año en cuatro cifras y según el
        // idioma de la interfaz (lote 39h).
        const date = receipt ? UnsafeText.dateText(receipt.accepted_at, t("format.datetime_minutes")) : ""
        if (date !== "") return t("excepciones.pantalla.activada_fecha").replace("{date}", date)
        return t("excepciones.pantalla.activada")
    }
    Connections {
        target: panel.localIpc ? panel.bridge : null
        function onUnsafeExceptionsFinished(action, ok, data) {
            if (action !== "unsafe_exceptions_list" && action !== "unsafe_exceptions_set") return
            panel.busy = false
            panel.loaded = ok
            const changed = panel.changedId
            panel.changedId = ""
            panel.message = ok ? "" : panel.t("excepciones.pantalla.error")
            if (ok) panel.items = data
            if (ok && changed !== "") {
                for (let i = 0; i < panel.items.length; i++) {
                    const item = panel.items[i]
                    if (item.id !== changed) continue
                    panel.message = panel.t("winui.selector.etiqueta_valor")
                        .replace("{label}", panel.t("excepciones." + item.id + ".ajuste"))
                        .replace("{value}", panel.stateText(item))
                    Qt.callLater(panel.focusRow, changed)
                }
            }
            panel.announce(panel.message)
        }
    }
    ColumnLayout {
        width: parent.width
        spacing: 16
        Text {
            Layout.fillWidth: true
            text: panel.t(panel.localIpc ? "excepciones.pantalla.descripcion" : "excepciones.canal_no_local")
            textFormat: Text.PlainText
            font.pixelSize: Typography.control
            color: panel.theme.textColor
            wrapMode: Text.WordWrap
        }
        ThemedButton {
            Layout.minimumHeight: 44
            text: panel.t("excepciones.pantalla.actualizar")
            enabled: panel.localIpc && !panel.busy
            onClicked: panel.load()
        }
        Text {
            id: statusLine
            objectName: "unsafeStatus"
            Layout.fillWidth: true
            visible: panel.busy || panel.message !== ""
            text: panel.busy ? panel.t("excepciones.pantalla.cargando") : panel.message
            textFormat: Text.PlainText
            font.pixelSize: Typography.control
            color: panel.theme.textColor
            wrapMode: Text.WordWrap
            Accessible.role: Accessible.AlertMessage
        }
        Repeater {
            id: rows
            model: panel.items
            delegate: Rectangle {
                id: row
                required property var modelData
                readonly property string policyNote: UnsafeText.policyNoteKey(modelData)
                // Opción insegura activada: recuadro de 2 px, fondo teñido e
                // icono de aviso. Los colores se ajustan a 3:1 frente al fondo
                // de la página y al del propio recuadro, en todos los temas.
                readonly property bool unsafeActive: modelData.enabled === true
                readonly property color pageColor: panel.theme.backgroundColor
                readonly property color fillColor: unsafeActive
                    ? Contrast.tinted(panel.theme.cardColor, panel.theme.errorColor, 0.14)
                    : "transparent"
                readonly property color markColor: Contrast.accentOnBoth(unsafeActive ? fillColor : pageColor,
                                                                         pageColor, panel.theme.errorColor, 3.0)
                readonly property color textColor: unsafeActive ? Contrast.readableOn(fillColor, panel.theme.textColor)
                                                                : panel.theme.textColor
                property alias changeButton: changeButton
                property alias helpButton: helpButton
                objectName: "unsafeRow-" + modelData.id
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                implicitHeight: rowContent.implicitHeight + 2 * rowContent.anchors.margins
                radius: 8
                color: fillColor
                border.width: unsafeActive ? 2 : 0
                border.color: markColor
                Rectangle {
                    // Separador entre filas normales; la activada ya va recuadrada.
                    visible: !row.unsafeActive
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    height: 1
                    color: panel.theme.secondaryTextColor
                }
                ColumnLayout {
                    id: rowContent
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.top: parent.top
                    anchors.margins: 12
                    spacing: 8
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: 10
                        // Triángulo de aviso: marca la opción activada por su
                        // forma, no solo por el color. Decorativo para el lector
                        // de pantalla, que ya lee «Activada» en la línea de estado.
                        Canvas {
                            id: warningIcon
                            objectName: "unsafeActiveIcon-" + row.modelData.id
                            visible: row.unsafeActive
                            readonly property color strokeColor: row.markColor
                            readonly property color markFill: row.fillColor
                            Layout.preferredWidth: Typography.minimum(22)
                            Layout.preferredHeight: Typography.minimum(22)
                            Layout.alignment: Qt.AlignTop
                            Accessible.ignored: true
                            onStrokeColorChanged: warningIcon.requestPaint()
                            onMarkFillChanged: warningIcon.requestPaint()
                            onWidthChanged: warningIcon.requestPaint()
                            onHeightChanged: warningIcon.requestPaint()
                            onPaint: {
                                const ctx = getContext("2d")
                                const w = width, h = height, line = Math.max(2, w / 10)
                                ctx.clearRect(0, 0, w, h)
                                ctx.lineJoin = "round"
                                ctx.lineCap = "round"
                                ctx.lineWidth = line
                                ctx.fillStyle = strokeColor
                                ctx.beginPath()
                                ctx.moveTo(w / 2, line / 2)
                                ctx.lineTo(w - line / 2, h - line / 2)
                                ctx.lineTo(line / 2, h - line / 2)
                                ctx.closePath()
                                ctx.fill()
                                ctx.strokeStyle = markFill
                                ctx.beginPath()
                                ctx.moveTo(w / 2, h * 0.36)
                                ctx.lineTo(w / 2, h * 0.62)
                                ctx.moveTo(w / 2, h * 0.78)
                                ctx.lineTo(w / 2, h * 0.79)
                                ctx.stroke()
                            }
                        }
                        Text {
                            Layout.fillWidth: true
                            text: panel.t("excepciones." + row.modelData.id + ".ajuste")
                            textFormat: Text.PlainText
                            font.pixelSize: Typography.control
                            font.bold: true
                            color: row.textColor
                            wrapMode: Text.WordWrap
                            Accessible.role: Accessible.Heading
                        }
                    }
                    Text {
                        Layout.fillWidth: true
                        text: panel.stateText(row.modelData)
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.control
                        color: row.textColor
                        wrapMode: Text.WordWrap
                    }
                    // Una sola nota: «solo lectura» si la organización lo activa,
                    // su texto si lo prohíbe; nunca las dos (lote 39h).
                    Text {
                        objectName: "unsafePolicy-" + row.modelData.id
                        Layout.fillWidth: true
                        visible: row.policyNote !== ""
                        text: row.policyNote === "" ? "" : panel.t(row.policyNote)
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.control
                        color: row.textColor
                        wrapMode: Text.WordWrap
                    }
                    ResponsiveRow {
                        Layout.fillWidth: true
                        spacing: 12
                        ThemedButton {
                            id: changeButton
                            Layout.fillWidth: true
                            Layout.minimumHeight: 44
                            objectName: "unsafeChange-" + row.modelData.id
                            visible: row.modelData.enabled || (row.modelData.activable === true && !row.modelData.fixed)
                            text: panel.t(row.modelData.enabled ? "excepciones.pantalla.desactivar" : "excepciones.pantalla.activar")
                            enabled: panel.localIpc && !panel.busy && !row.modelData.fixed && (row.modelData.enabled || row.modelData.activable === true)
                            Accessible.description: panel.t("excepciones." + row.modelData.id + ".ajuste")
                            onClicked: panel.change(row.modelData)
                        }
                        ThemedButton {
                            id: helpButton
                            objectName: "unsafeHelp-" + row.modelData.id
                            Layout.fillWidth: true
                            Layout.minimumHeight: 44
                            text: panel.t("excepciones.mas_informacion")
                            Accessible.description: panel.t("excepciones." + row.modelData.id + ".ajuste")
                            onClicked: panel.helpRequested(row.modelData)
                        }
                    }
                }
            }
        }
    }
}
