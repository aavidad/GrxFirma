// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "UnsafeText.js" as UnsafeText

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
    // Desde Configuración no hay operación detenida: el título no habla de un
    // bloqueo y el botón solo activa. «Activar y continuar» se reserva para un
    // bloqueo que se puede repetir (retryId del motor o reenvío del formulario).
    readonly property bool blocked: item.operationPending === true
    readonly property bool continues: blocked && (!!item.retryId || item.resend === true)
    // La ayuda larga se lee por apartados, cada uno con su título como
    // encabezado. Si lo decide la organización, no explica cómo activar ni
    // deshacer: dice quién lo decide y a quién preguntar (lote 39h).
    readonly property var helpSections: showingHelp
        ? UnsafeText.helpSections(caseText("ayuda"), appText("excepciones.pantalla.ayuda_organizacion"),
                                  UnsafeText.organizationDecides(item))
        : []
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
    title: appText(showingHelp ? "excepciones.mas_informacion" : blocked ? "excepciones.pantalla.titulo" : "excepciones.pantalla.aceptar_titulo")
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
        // El recibo guarda dónde se aceptó: en el aviso de una operación
        // detenida o en el apartado Seguridad. Solo un bloqueo lleva reintento.
        bridge.activateUnsafeException(item.id, item.acceptance_version, blocked ? "aviso" : "ajustes",
                                       blocked ? (item.retryId || "") : "")
    }
    // El desplazamiento no sigue al foco por sí solo: con letra grande la
    // casilla podía quedar fuera de la vista al llegar con Tab.
    function revealFocused(target) {
        const flick = scroll.contentItem
        if (!target || !flick || flick.height <= 0) return
        const top = target.mapToItem(flick.contentItem, 0, 0).y - 8
        const bottom = top + target.height + 16
        if (top < flick.contentY) flick.contentY = Math.max(0, top)
        else if (bottom > flick.contentY + flick.height)
            flick.contentY = Math.min(Math.max(0, flick.contentHeight - flick.height), bottom - flick.height)
    }
    function focusFirstSection() {
        const first = helpRepeater.count > 0 ? helpRepeater.itemAt(0) : null
        if (first) (first.sectionHeading.visible ? first.sectionHeading : first.sectionBody).forceActiveFocus(Qt.OtherFocusReason)
        else caseBody.forceActiveFocus(Qt.OtherFocusReason)
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
        // El foco empieza en el nombre del caso, como en Android: el lector
        // lo lee primero y Tab sigue hacia la casilla y los botones.
        initialFocusItem: heading
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
                        id: heading
                        objectName: "unsafeHeading"
                        Layout.fillWidth: true
                        Layout.topMargin: 4
                        Layout.leftMargin: 4
                        Layout.rightMargin: 4
                        text: dialog.caseText("ajuste")
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.size18
                        font.bold: true
                        color: dialog.theme.textColor
                        wrapMode: Text.WordWrap
                        activeFocusOnTab: true
                        onActiveFocusChanged: if (activeFocus) dialog.revealFocused(heading)
                        Accessible.role: Accessible.Heading
                        Accessible.name: text
                        Accessible.focusable: true
                        // Foco de teclado visible también sobre el texto.
                        Rectangle {
                            anchors.fill: parent
                            anchors.margins: -4
                            color: "transparent"
                            radius: 4
                            border.width: 2
                            border.color: dialog.theme.focusColor ? dialog.theme.focusColor : dialog.theme.textColor
                            visible: heading.activeFocus
                        }
                    }
                    Text {
                        id: caseBody
                        objectName: "unsafeBody"
                        Layout.fillWidth: true
                        Layout.leftMargin: 4
                        Layout.rightMargin: 4
                        // Sin apartados (catálogo antiguo), la ayuda entera.
                        visible: !dialog.showingHelp || helpRepeater.count === 0
                        text: dialog.caseText(dialog.showingHelp ? "ayuda" : "riesgo")
                        textFormat: Text.PlainText
                        font.pixelSize: Typography.control
                        color: dialog.theme.textColor
                        wrapMode: Text.WordWrap
                        Accessible.role: Accessible.StaticText
                        Accessible.name: text
                        Accessible.focusable: dialog.showingHelp && visible
                        activeFocusOnTab: dialog.showingHelp && visible
                        onActiveFocusChanged: if (activeFocus) dialog.revealFocused(caseBody)
                        Rectangle {
                            anchors.fill: parent
                            anchors.margins: -4
                            color: "transparent"
                            radius: 4
                            border.width: 2
                            border.color: dialog.theme.focusColor ? dialog.theme.focusColor : dialog.theme.textColor
                            visible: caseBody.activeFocus
                        }
                    }
                    Repeater {
                        id: helpRepeater
                        model: dialog.helpSections
                        delegate: ColumnLayout {
                            id: section
                            required property var modelData
                            required property int index
                            property alias sectionHeading: sectionTitle
                            property alias sectionBody: sectionText
                            objectName: "unsafeHelpSection-" + index
                            Layout.fillWidth: true
                            Layout.leftMargin: 4
                            Layout.rightMargin: 4
                            spacing: 4
                            // Cada apartado es una parada de Tab: el lector
                            // dice el título como encabezado y luego su texto.
                            Text {
                                id: sectionTitle
                                objectName: "unsafeHelpTitle-" + section.index
                                Layout.fillWidth: true
                                visible: text !== ""
                                text: section.modelData.title
                                textFormat: Text.PlainText
                                font.pixelSize: Typography.control
                                font.bold: true
                                color: dialog.theme.textColor
                                wrapMode: Text.WordWrap
                                activeFocusOnTab: visible
                                onActiveFocusChanged: if (activeFocus) dialog.revealFocused(section)
                                Accessible.role: Accessible.Heading
                                Accessible.name: text
                                Accessible.description: section.modelData.body
                                Accessible.focusable: visible
                                Rectangle {
                                    anchors.fill: parent
                                    anchors.margins: -4
                                    color: "transparent"
                                    radius: 4
                                    border.width: 2
                                    border.color: dialog.theme.focusColor ? dialog.theme.focusColor : dialog.theme.textColor
                                    visible: sectionTitle.activeFocus
                                }
                            }
                            Text {
                                id: sectionText
                                objectName: "unsafeHelpBody-" + section.index
                                Layout.fillWidth: true
                                text: section.modelData.body
                                textFormat: Text.PlainText
                                font.pixelSize: Typography.control
                                color: dialog.theme.textColor
                                wrapMode: Text.WordWrap
                                lineHeight: 1.15
                                Accessible.role: Accessible.StaticText
                                Accessible.name: text
                                // Un apartado sin título se lee con Tab igual.
                                activeFocusOnTab: !sectionTitle.visible
                                Accessible.focusable: !sectionTitle.visible
                            }
                        }
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
                        // Intro no marca, no desmarca ni activa: solo Espacio
                        // cambia la casilla y activar exige su botón.
                        onActiveFocusChanged: if (activeFocus) dialog.revealFocused(acceptance)
                        Keys.onReturnPressed: function(event) { event.accepted = true }
                        Keys.onEnterPressed: function(event) { event.accepted = true }
                        Accessible.description: dialog.appText("excepciones.pantalla.version").replace("{version}", dialog.item.acceptance_version || "")
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
                    objectName: "unsafeMore"
                    Layout.fillWidth: true
                    Layout.minimumHeight: 44
                    visible: !dialog.helpOnly
                    // Sin casilla (lo decide la organización) no hay aceptación a
                    // la que volver: el botón dice solo «Volver» (lote 39h).
                    text: dialog.appText(!dialog.showingHelp ? "excepciones.mas_informacion"
                        : dialog.activable ? "excepciones.pantalla.volver" : "Volver")
                    enabled: !dialog.busy
                    // Al abrir la ayuda, el foco va a su primer apartado para
                    // leerlo y desplazarlo; al volver, se queda en este botón.
                    onClicked: {
                        dialog.showingHelp = !dialog.showingHelp
                        if (dialog.showingHelp) Qt.callLater(dialog.focusFirstSection)
                    }
                }
                // Acción principal, rellena con el color primario del tema; los
                // otros dos botones quedan como secundarios, con el relleno de
                // la tarjeta y su borde.
                ThemedButton {
                    objectName: "unsafeActivate"
                    Layout.fillWidth: true
                    Layout.minimumHeight: 44
                    visible: dialog.activable && !dialog.showingHelp
                    enabled: dialog.canActivate
                    font.bold: true
                    palette.button: dialog.theme.primaryColor || palette.highlight
                    palette.buttonText: "white"
                    text: dialog.appText(dialog.continues ? "excepciones.activar_continuar" : "excepciones.pantalla.activar")
                    onClicked: dialog.activate()
                }
            }
        }
    }
}
