// SPDX-License-Identifier: EUPL-1.2
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

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
        if (item.enabled) { busy = true; bridge.disableUnsafeException(item.id) }
        else acceptanceRequested(item)
    }
    function stateText(item) {
        if (item.policy_unreadable) return t("excepciones.pantalla.directiva_ilegible")
        if (item.fixed) return t(item.enabled ? "excepciones.pantalla.fijada_activa" : "excepciones.pantalla.fijada_inactiva")
        if (!item.enabled) return t("excepciones.pantalla.desactivada")
        const receipt = item.last_acceptance
        if (receipt && receipt.accepted_at) return t("excepciones.pantalla.activada_fecha").replace("{date}", new Date(receipt.accepted_at).toLocaleString(Qt.locale(localeName), Locale.ShortFormat))
        return t("excepciones.pantalla.activada")
    }
    Connections {
        target: panel.localIpc ? panel.bridge : null
        function onUnsafeExceptionsFinished(action, ok, data) {
            if (action !== "unsafe_exceptions_list" && action !== "unsafe_exceptions_set") return
            panel.busy = false
            panel.loaded = ok
            panel.message = ok ? "" : panel.t("excepciones.pantalla.error")
            if (ok) panel.items = data
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
            model: panel.items
            delegate: ColumnLayout {
                id: row
                required property var modelData
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: 8
                Text {
                    Layout.fillWidth: true
                    text: panel.t("excepciones." + row.modelData.id + ".ajuste")
                    textFormat: Text.PlainText
                    font.pixelSize: Typography.control
                    font.bold: true
                    color: panel.theme.textColor
                    wrapMode: Text.WordWrap
                    Accessible.role: Accessible.Heading
                }
                Text {
                    Layout.fillWidth: true
                    text: panel.stateText(row.modelData)
                    textFormat: Text.PlainText
                    font.pixelSize: Typography.control
                    color: panel.theme.textColor
                    wrapMode: Text.WordWrap
                }
                Text {
                    Layout.fillWidth: true
                    visible: row.modelData.fixed || row.modelData.activable === false
                    text: row.modelData.policy_unreadable ? panel.t("excepciones.pantalla.directiva_ilegible")
                        : panel.t("excepciones.pantalla.solo_lectura") + (row.modelData.enabled ? "" : "\n" + panel.t("excepciones." + row.modelData.id + ".organizacion"))
                    textFormat: Text.PlainText
                    font.pixelSize: Typography.control
                    color: panel.theme.textColor
                    wrapMode: Text.WordWrap
                }
                ResponsiveRow {
                    Layout.fillWidth: true
                    spacing: 12
                    ThemedButton {
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
                        Layout.fillWidth: true
                        Layout.minimumHeight: 44
                        text: panel.t("excepciones.mas_informacion")
                        Accessible.description: panel.t("excepciones." + row.modelData.id + ".ajuste")
                        onClicked: panel.helpRequested(row.modelData)
                    }
                }
                Rectangle { Layout.fillWidth: true; height: 1; color: panel.theme.secondaryTextColor }
            }
        }
    }
}
