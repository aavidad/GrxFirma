// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "ThemeContrast.js" as Contrast

// Botón «?» de ayuda contextual. Al pulsarlo (ratón, Espacio o Intro) muestra
// una explicación breve junto al control; Escape, volver a pulsarlo o salir del
// botón la cierran. Al llegar con el tabulador solo aparece su nombre, para no
// abrir una ventana flotante en cada «?» al recorrer un formulario. El lector
// de pantalla lee el nombre y, como descripción, el mismo texto que se ve.
// Los textos llegan ya traducidos desde quien lo usa.
//
// Uso habitual (ayuda contextual de un control):
//   HelpButton {
//       nameTemplate: tr("ayuda.boton_nombre")   // «Ayuda sobre {0}»
//       controlLabel: tr("Formato")              // etiqueta visible del control
//       helpText: tr("ayuda.formato")
//   }
//
// Un selector enumera todas sus opciones, sin depender de la elegida: cada
// una con su nombre en negrita y su frase breve («Firma: firma…»). Si una
// opción o el texto suelto tiene ampliación («<clave>.mas»), al final de su
// frase aparece un «+» que la despliega y pasa a «−» para plegarla:
//   HelpButton {
//       nameTemplate: tr("ayuda.boton_nombre"); controlLabel: tr("Operación")
//       helpText: tr("ayuda.operacion")
//       optionTemplate: tr("ayuda.opcion")          // «{0}: {1}»
//       moreNameTemplate: tr("ayuda.mas_nombre")    // «Más información sobre {0}»
//       options: [{ name: tr("Cofirmar"), text: tr("ayuda.operacion.cofirma"),
//                   more: tr("ayuda.operacion.cofirma.mas") }]
//       moreText: ...                               // ampliación de helpText
//   }
Button {
    id: control

    // Nombre accesible explícito. Si está vacío se compone con nameTemplate,
    // sustituyendo {0} por controlLabel (tr() no sustituye marcadores).
    property string accessibleName: ""
    property string nameTemplate: ""
    property string controlLabel: ""
    // Texto de un solo párrafo; si se usa paragraphs, este se ignora.
    property string helpText: ""
    property string title: ""
    // Párrafos de la explicación; el último se resalta (avisos o requisitos).
    property var paragraphs: helpText !== "" ? [helpText] : []
    property bool emphasizeLast: false
    // Ampliación del texto suelto (helpText); vacía, sin «+».
    property string moreText: ""
    // Todas las opciones de un selector: [{ name, text, more }].
    property var options: []
    property string optionTemplate: "{0}: {1}"
    property string moreNameTemplate: ""
    readonly property alias popupOpen: helpPopup.opened

    // Párrafos sueltos y opciones, en el orden en que se muestran.
    readonly property var entries: {
        const list = []
        const texts = paragraphs || []
        for (let i = 0; i < texts.length; i++)
            list.push({ name: "", text: String(texts[i]), more: i === 0 ? moreText : "", bold: emphasizeLast && i === texts.length - 1 })
        const opts = options || []
        for (let j = 0; j < opts.length; j++)
            list.push({ name: String(opts[j].name || ""), text: String(opts[j].text || ""), more: String(opts[j].more || ""), bold: false })
        return list
    }

    function plainEntry(entry) {
        return entry.name === "" ? entry.text
            : optionTemplate.replace("{0}", entry.name).replace("{1}", entry.text)
    }

    function escapeHtml(value) {
        return String(value).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    }

    // Nombre de la opción en negrita; el resto, texto normal.
    function styledEntry(entry) {
        if (entry.name === "") return escapeHtml(entry.text)
        return escapeHtml(optionTemplate).replace("{0}", "<b>" + escapeHtml(entry.name) + "</b>")
            .replace("{1}", escapeHtml(entry.text))
    }

    function moreName(entry) {
        const subject = entry.name !== "" ? entry.name : cleanLabel
        return moreNameTemplate !== "" ? moreNameTemplate.replace("{0}", subject) : subject
    }

    // Botones «+» visibles, para recorrerlos con el tabulador.
    function moreButtons() {
        const list = []
        for (let i = 0; i < entryRepeater.count; i++) {
            const item = entryRepeater.itemAt(i)
            if (item && item.moreButton.visible) list.push(item.moreButton)
        }
        return list
    }

    function focusInsidePopup() {
        let item = control.Window.activeFocusItem
        while (item) {
            if (item === helpPopup.contentItem) return true
            item = item.parent
        }
        return false
    }

    function moveFocusFrom(button, forward) {
        const list = moreButtons()
        const index = list.indexOf(button) + (forward ? 1 : -1)
        if (index >= 0 && index < list.length) list[index].forceActiveFocus(forward ? Qt.TabFocusReason : Qt.BacktabFocusReason)
        else control.forceActiveFocus(Qt.TabFocusReason)
    }

    text: "?"
    hoverEnabled: true
    focusPolicy: Qt.StrongFocus
    // 32 px: por encima del mínimo táctil de 24 px (WCAG 2.2) sin descuadrar
    // una etiqueta pequeña.
    implicitWidth: 32
    implicitHeight: 32
    // Etiqueta sin iconos iniciales (emoji de los títulos) ni dos puntos finales,
    // para que el lector de pantalla diga «Ayuda sobre Servidor TSA».
    readonly property string cleanLabel: String(controlLabel || "")
        .replace(/^[\s\u2000-\u2BFF\uD800-\uDFFF\uFE0F]+/, "")
        .replace(/[\s:\uFF1A]+$/, "")
    readonly property string effectiveName: accessibleName !== ""
        ? accessibleName
        : (nameTemplate !== "" ? nameTemplate.replace("{0}", cleanLabel) : cleanLabel)
    Accessible.role: Accessible.Button
    Accessible.name: effectiveName
    Accessible.description: (title !== "" ? title + " " : "") + entries.map(plainEntry).join(" ")
    ToolTip.visible: (hovered || visualFocus) && !helpPopup.opened && effectiveName !== ""
    ToolTip.delay: 500
    ToolTip.text: effectiveName

    readonly property color labelColor: Contrast.readableOn(control.palette.window, control.palette.windowText)

    contentItem: Item {
        Rectangle {
            anchors.centerIn: parent
            width: 22
            height: 22
            radius: 11
            color: "transparent"
            border.color: control.labelColor
            border.width: 1.5
            Text {
                anchors.centerIn: parent
                text: control.text
                color: control.labelColor
                font.bold: true
                font.pixelSize: 14
            }
        }
    }
    background: Rectangle {
        radius: 4
        color: control.down || control.hovered ? Qt.darker(control.palette.window, 1.08) : "transparent"
        border.color: control.activeFocus ? control.palette.highlight : "transparent"
        border.width: 2
    }

    function toggle() { helpPopup.opened ? helpPopup.close() : helpPopup.open() }
    onClicked: toggle()
    // Se cierra al salir del «?», salvo que el foco pase a un «+» de la ayuda.
    onActiveFocusChanged: if (!activeFocus) Qt.callLater(function() { if (!control.activeFocus && !control.focusInsidePopup()) helpPopup.close() })
    // Con la ayuda abierta, el tabulador entra en sus «+».
    Keys.onTabPressed: function(event) {
        const list = moreButtons()
        if (helpPopup.opened && list.length > 0) {
            list[0].forceActiveFocus(Qt.TabFocusReason)
            event.accepted = true
        } else {
            event.accepted = false
        }
    }
    // Con la ayuda abierta, las flechas y Re Pág/Av Pág desplazan su texto
    // aunque el foco siga en el «?» (la ayuda larga no cabe sin desplazarse).
    function scrollHelp(delta) {
        if (!helpPopup.opened)
            return false
        const maximum = Math.max(0, helpScroll.contentHeight - helpScroll.height)
        helpScroll.contentY = Math.max(0, Math.min(maximum, helpScroll.contentY + delta))
        return true
    }
    // Lleva a la vista el «+» que recibe el foco.
    function ensureHelpVisible(item) {
        if (!item || !helpPopup.opened)
            return
        const top = item.mapToItem(helpColumn, 0, 0).y
        const bottom = top + item.height
        if (top < helpScroll.contentY)
            helpScroll.contentY = Math.max(0, top - 4)
        else if (bottom > helpScroll.contentY + helpScroll.height)
            helpScroll.contentY = Math.min(Math.max(0, helpScroll.contentHeight - helpScroll.height),
                                           bottom - helpScroll.height + 4)
    }
    Keys.onUpPressed: function(event) { event.accepted = control.scrollHelp(-40) }
    Keys.onDownPressed: function(event) { event.accepted = control.scrollHelp(40) }
    Keys.onPressed: function(event) {
        if (event.key === Qt.Key_PageUp)
            event.accepted = control.scrollHelp(-helpScroll.height * 0.9)
        else if (event.key === Qt.Key_PageDown)
            event.accepted = control.scrollHelp(helpScroll.height * 0.9)
    }
    // Button solo se activa con Espacio; Intro también abre la ayuda.
    Keys.onReturnPressed: function(event) { toggle(); event.accepted = true }
    Keys.onEnterPressed: function(event) { toggle(); event.accepted = true }
    Keys.onEscapePressed: function(event) {
        if (helpPopup.opened) {
            helpPopup.close()
            event.accepted = true
        } else {
            event.accepted = false
        }
    }

    Popup {
        id: helpPopup
        objectName: "helpPopup"
        y: control.height + 4
        x: Math.min(0, control.width - width)
        width: Math.min(360, Math.max(200, (control.Window.width || 360) - 32))
        // Sin salirse de la ventana aunque el «?» esté en un borde (ventanas estrechas).
        margins: 8
        padding: 12
        focus: false
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutsideParent
        readonly property color textColor: Contrast.readableOn(control.palette.base, control.palette.text)
        background: Rectangle {
            color: control.palette.base
            border.color: control.palette.mid
            border.width: 1
            radius: 6
        }
        // Con las ampliaciones desplegadas puede no caber: se desplaza.
        readonly property real maxContentHeight: Math.max(120, Math.min(440, (control.Window.height || 480) - 48))
        contentItem: Flickable {
            id: helpScroll
            implicitWidth: helpColumn.implicitWidth
            implicitHeight: Math.min(helpColumn.implicitHeight, helpPopup.maxContentHeight)
            contentWidth: width
            contentHeight: helpColumn.implicitHeight
            clip: true
            boundsBehavior: Flickable.StopAtBounds
            ScrollBar.vertical: ScrollBar { policy: helpScroll.contentHeight > helpScroll.height ? ScrollBar.AlwaysOn : ScrollBar.AsNeeded }

            ColumnLayout {
                id: helpColumn
                width: helpScroll.width - (helpScroll.contentHeight > helpScroll.height ? 10 : 0)
                spacing: 8
                Text {
                    objectName: "helpTitle"
                    Layout.fillWidth: true
                    visible: control.title !== ""
                    text: control.title
                    color: helpPopup.textColor
                    font.bold: true
                    wrapMode: Text.WordWrap
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }
                Repeater {
                    id: entryRepeater
                    model: control.entries
                    delegate: ColumnLayout {
                        id: entryItem
                        required property var modelData
                        property bool expanded: false
                        property alias moreButton: moreButton
                        property alias sentence: sentence
                        property alias detail: detail
                        Layout.fillWidth: true
                        spacing: 4
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: 4
                            Text {
                                id: sentence
                                objectName: "helpEntry"
                                Layout.fillWidth: true
                                text: control.styledEntry(entryItem.modelData)
                                textFormat: Text.StyledText
                                color: helpPopup.textColor
                                font.bold: entryItem.modelData.bold
                                wrapMode: Text.WordWrap
                                Accessible.role: Accessible.StaticText
                                Accessible.name: control.plainEntry(entryItem.modelData)
                            }
                            Button {
                                id: moreButton
                                objectName: "helpMoreButton"
                                visible: entryItem.modelData.more !== ""
                                Layout.alignment: Qt.AlignTop
                                implicitWidth: 28
                                implicitHeight: 28
                                focusPolicy: Qt.StrongFocus
                                text: entryItem.expanded ? "\u2212" : "+"
                                Accessible.role: Accessible.Button
                                Accessible.name: control.moreName(entryItem.modelData)
                                Accessible.checkable: true
                                Accessible.checked: entryItem.expanded
                                onClicked: entryItem.expanded = !entryItem.expanded
                                Keys.onReturnPressed: function(event) { entryItem.expanded = !entryItem.expanded; event.accepted = true }
                                Keys.onEnterPressed: function(event) { entryItem.expanded = !entryItem.expanded; event.accepted = true }
                                Keys.onTabPressed: function(event) { control.moveFocusFrom(moreButton, true); event.accepted = true }
                                Keys.onBacktabPressed: function(event) { control.moveFocusFrom(moreButton, false); event.accepted = true }
                                Keys.onEscapePressed: function(event) {
                                    helpPopup.close()
                                    control.forceActiveFocus()
                                    event.accepted = true
                                }
                                Keys.onUpPressed: function(event) { event.accepted = control.scrollHelp(-40) }
                                Keys.onDownPressed: function(event) { event.accepted = control.scrollHelp(40) }
                                onActiveFocusChanged: {
                                    if (activeFocus)
                                        control.ensureHelpVisible(moreButton)
                                    else
                                        Qt.callLater(function() { if (!control.activeFocus && !control.focusInsidePopup()) helpPopup.close() })
                                }
                                contentItem: Text {
                                    text: moreButton.text
                                    color: helpPopup.textColor
                                    font.bold: true
                                    font.pixelSize: 16
                                    horizontalAlignment: Text.AlignHCenter
                                    verticalAlignment: Text.AlignVCenter
                                }
                                background: Rectangle {
                                    radius: 14
                                    color: moreButton.down || moreButton.hovered ? Qt.darker(control.palette.base, 1.08) : "transparent"
                                    border.color: moreButton.activeFocus ? control.palette.highlight : helpPopup.textColor
                                    border.width: moreButton.activeFocus ? 2 : 1
                                }
                            }
                        }
                        Text {
                            id: detail
                            objectName: "helpMoreText"
                            Layout.fillWidth: true
                            visible: entryItem.expanded
                            text: entryItem.modelData.more
                            color: helpPopup.textColor
                            wrapMode: Text.WordWrap
                            Accessible.role: Accessible.StaticText
                            Accessible.name: text
                        }
                    }
                }
            }
        }
    }
}
