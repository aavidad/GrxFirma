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
    readonly property alias popupOpen: helpPopup.opened

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
    Accessible.description: (title !== "" ? title + " " : "") + (paragraphs || []).join(" ")
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
    onActiveFocusChanged: if (!activeFocus) helpPopup.close()
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
        contentItem: ColumnLayout {
            spacing: 8
            Text {
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
                model: control.paragraphs
                delegate: Text {
                    required property string modelData
                    required property int index
                    Layout.fillWidth: true
                    text: modelData
                    color: helpPopup.textColor
                    font.bold: control.emphasizeLast && index === control.paragraphs.length - 1
                    wrapMode: Text.WordWrap
                    Accessible.role: Accessible.StaticText
                    Accessible.name: text
                }
            }
        }
    }
}
