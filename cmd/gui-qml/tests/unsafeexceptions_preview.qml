// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Deterministic preview: no engine, documents, credentials or network.
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "../qml"

ApplicationWindow {
    id: preview
    width: 1200; height: 850
    visible: true
    title: i18n.t("excepciones.pantalla.preferencias")
    property var theme: ({cardColor: "#ffffff", textColor: "#202124", secondaryTextColor: "#555555"})
    color: "#f3f4f6"
    palette.windowText: theme.textColor
    palette.buttonText: theme.textColor
    palette.base: theme.cardColor
    palette.button: theme.cardColor
    palette.highlight: "#005a9e"
    QtObject {
        id: fake
        signal unsafeExceptionsFinished(string action, bool ok, var data)
        function loadUnsafeExceptions() { unsafeExceptionsFinished("unsafe_exceptions_list", true, previewItems) }
        function activateUnsafeException(id, version, origin, retry) { }
        function disableUnsafeException(id) { }
    }
    ScrollView {
        anchors.fill: parent
        anchors.margins: 24
        contentWidth: availableWidth
        clip: true
        UnsafeExceptionsPanel {
            width: parent.width
            theme: preview.theme
            bridge: fake
            localIpc: true
            translate: function(key) { return i18n.t(key) }
            localeName: i18n.locale
            items: previewItems
            loaded: true
            onAcceptanceRequested: function(item) { consent.present(item, false) }
            onHelpRequested: function(item) { consent.present(item, true) }
        }
    }
    UnsafeExceptionDialog {
        id: consent
        parent: Overlay.overlay
        theme: preview.theme
        bridge: fake
        translate: function(key) { return i18n.t(key) }
    }
    Component.onCompleted: {
        Typography.textScalePercent = captureScale
        if (captureScenario !== "preferences") {
            const item = Object.assign({}, previewItems[1])
            item.operationPending = true
            item.activable = captureScenario !== "organization"
            consent.present(item, captureScenario === "help")
        }
    }
}
