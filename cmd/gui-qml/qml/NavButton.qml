// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtQuick.Layouts 1.15

Rectangle {
    id: navButtonRoot
    property string text: ""
    property string iconTxt: ""
    property bool active: false
    signal clicked()

    Layout.fillWidth: true
    height: 45
    radius: 8
    color: active ? currentTheme.primaryColor : "transparent"
    border.color: active ? "white" : "transparent"
    border.width: active ? 1 : 0

    MouseArea {
        anchors.fill: parent
        onClicked: navButtonRoot.clicked()
        hoverEnabled: true
        onEntered: if (!navButtonRoot.active) navButtonRoot.opacity = 0.7
        onExited: navButtonRoot.opacity = 1.0
    }

    RowLayout {
        anchors.fill: parent
        anchors.margins: 10
        Text { text: iconTxt; color: "white"; font.bold: true; Layout.preferredWidth: 20 }
        Text { text: navButtonRoot.text; color: "white"; font.bold: active; Layout.fillWidth: true }
    }
}
