// SPDX-License-Identifier: EUPL-1.2
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ThemedDialog {
    id: dialog
    required theme
    required property var bridge
    property var item: ({})
    property bool helpOnly: false
    property bool showingHelp: false
    property bool busy: false
    property string message: ""
    property bool activationSucceeded: false
    readonly property bool activable: item.activable === true && !!item.acceptance_version
    readonly property bool canActivate: activable && acceptance.checked && !busy && !showingHelp
    signal activated(var item)
    signal activationFailed()
    signal cancelled(var item)
    modal: true
    focus: true
    closePolicy: busy ? Popup.NoAutoClose : Popup.CloseOnEscape
    width: Math.min(parent.width - 32, 760)
    height: Math.min(parent.height - 32, body.implicitHeight + actions.implicitHeight + header.implicitHeight + 48)
    x: (parent.width - width) / 2
    y: (parent.height - height) / 2
    title: appText(showingHelp ? "excepciones.mas_informacion" : item.operationPending === true ? "excepciones.pantalla.titulo" : "excepciones.pantalla.aceptar_titulo")
    accessibleDescription: caseText("riesgo")
    function appText(key) { return dialog.translate ? dialog.translate(key) : key }
    function caseText(suffix) { return item.id ? appText("excepciones." + item.id + "." + suffix) : "" }
    function present(value, help) {
        item = Object.assign({}, value)
        helpOnly = help === true
        showingHelp = helpOnly
        acceptance.checked = false
        busy = false
        activationSucceeded = false
        message = ""
        dialog.open()
    }
    function activate() {
        if (!canActivate) return
        busy = true
        message = ""
        bridge.activateUnsafeException(item.id, item.acceptance_version, item.retryId || "")
    }
    onOpened: contentItem.enter()
    onClosed: {
        contentItem.restoreFocus()
        if (!activationSucceeded && !helpOnly) cancelled(item)
    }
    Connections {
        target: dialog.bridge && typeof dialog.bridge.activateUnsafeException === "function" ? dialog.bridge : null
        function onUnsafeExceptionsFinished(action, ok, data) {
            if (action !== "unsafe_exceptions_activate" || !dialog.busy) return
            dialog.busy = false
            if (ok) {
                const accepted = dialog.item
                dialog.activationSucceeded = true
                dialog.close()
                dialog.activated(accepted)
            } else {
                acceptance.checked = false
                dialog.message = dialog.appText("excepciones.pantalla.error")
                dialog.activationFailed()
                cancel.forceActiveFocus()
            }
        }
    }
    contentItem: ModalLayer {
        color: dialog.theme.cardColor
        initialFocusItem: dialog.activable && !dialog.showingHelp ? acceptance : cancel
        onEscapePressed: if (!dialog.busy) dialog.close()
        ColumnLayout {
            anchors.fill: parent
            spacing: 16
            ScrollView {
                id: scroll
                Layout.fillWidth: true
                Layout.fillHeight: true
                Layout.minimumHeight: 0
                clip: true
                contentWidth: availableWidth
                ScrollBar.vertical.policy: ScrollBar.AlwaysOn
                ColumnLayout {
                    id: body
                    width: parent.width
                    spacing: 16
                    Text {
                        Layout.fillWidth: true
                        text: dialog.caseText("ajuste")
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.size18
                        font.bold: true
                        color: dialog.theme.textColor
                        wrapMode: Text.WordWrap
                        Accessible.role: Accessible.Heading
                    }
                    Text {
                        Layout.fillWidth: true
                        text: dialog.showingHelp ? dialog.caseText("ayuda").split("\n").join("\n\n") : dialog.caseText("riesgo")
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.control
                        color: dialog.theme.textColor
                        wrapMode: Text.WordWrap
                        Accessible.role: Accessible.StaticText
                        activeFocusOnTab: dialog.showingHelp
                    }
                    Text {
                        Layout.fillWidth: true
                        visible: !dialog.activable && !dialog.showingHelp
                        text: dialog.caseText("organizacion")
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.control
                        color: dialog.theme.textColor
                        wrapMode: Text.WordWrap
                        Accessible.role: Accessible.StaticText
                    }
                    ThemedCheckBox {
                        id: acceptance
                        objectName: "unsafeAcceptance"
                        Layout.fillWidth: true
                        Layout.minimumHeight: 44
                        visible: dialog.activable && !dialog.showingHelp
                        enabled: !dialog.busy
                        text: dialog.caseText("aceptacion")
                        Accessible.description: dialog.appText("excepciones.pantalla.version").replace("{version}", dialog.item.acceptance_version || "")
                    }
                    Text {
                        Layout.fillWidth: true
                        visible: dialog.activable
                        text: dialog.appText("excepciones.pantalla.desactivar_aqui")
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.control
                        color: dialog.theme.textColor
                        wrapMode: Text.WordWrap
                    }
                    Text {
                        Layout.fillWidth: true
                        visible: dialog.message !== "" || dialog.busy
                        text: dialog.busy ? dialog.appText("excepciones.pantalla.guardando") : dialog.message
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.control
                        color: dialog.theme.textColor
                        wrapMode: Text.WordWrap
                        Accessible.role: Accessible.AlertMessage
                    }
                }
            }
            ResponsiveRow {
                id: actions
                Layout.fillWidth: true
                spacing: 12
                ThemedButton {
                    id: cancel
                    objectName: "unsafeCancel"
                    Layout.fillWidth: true
                    Layout.minimumHeight: 44
                    text: dialog.appText(dialog.helpOnly ? "excepciones.pantalla.cerrar" : "excepciones.cancelar")
                    enabled: !dialog.busy
                    onClicked: dialog.close()
                }
                ThemedButton {
                    Layout.fillWidth: true
                    Layout.minimumHeight: 44
                    visible: !dialog.helpOnly
                    text: dialog.appText(dialog.showingHelp ? "excepciones.pantalla.volver" : "excepciones.mas_informacion")
                    enabled: !dialog.busy
                    onClicked: { dialog.showingHelp = !dialog.showingHelp; dialog.contentItem.enter() }
                }
                ThemedButton {
                    objectName: "unsafeActivate"
                    Layout.fillWidth: true
                    Layout.minimumHeight: 44
                    visible: dialog.activable && !dialog.showingHelp
                    enabled: dialog.canActivate
                    text: dialog.appText("excepciones.activar_continuar")
                    onClicked: dialog.activate()
                }
            }
        }
    }
}
