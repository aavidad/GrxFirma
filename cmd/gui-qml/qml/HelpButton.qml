// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "ThemeContrast.js" as Contrast

// Botón «?» de ayuda contextual. Al pulsarlo o al llegar a él con el teclado
// muestra una explicación breve junto al control; Escape o salir del botón la
// cierran. El lector de pantalla lee el nombre y, como descripción, el mismo
// texto que se ve. Los textos llegan ya traducidos desde quien lo usa.
Button {
    id: control

    property string accessibleName: ""
    property string title: ""
    // Párrafos de la explicación; el último se resalta (avisos o requisitos).
    property var paragraphs: []
    property bool emphasizeLast: false
    readonly property alias popupOpen: helpPopup.opened

    text: "?"
    hoverEnabled: true
    focusPolicy: Qt.StrongFocus
    implicitWidth: 40
    implicitHeight: 40
    Accessible.role: Accessible.Button
    Accessible.name: accessibleName
    Accessible.description: (title !== "" ? title + " " : "") + (paragraphs || []).join(" ")
    ToolTip.visible: hovered && !helpPopup.opened
    ToolTip.delay: 500
    ToolTip.text: accessibleName

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

    onClicked: helpPopup.opened ? helpPopup.close() : helpPopup.open()
    onActiveFocusChanged: {
        if (activeFocus && (focusReason === Qt.TabFocusReason || focusReason === Qt.BacktabFocusReason))
            helpPopup.open()
        else if (!activeFocus)
            helpPopup.close()
    }
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
        width: Math.min(360, Math.max(240, (control.Window.width || 360) - 32))
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
