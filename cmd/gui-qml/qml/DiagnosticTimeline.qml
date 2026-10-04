// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15

Item {
    id: timeline

    property var steps: []
    property var theme: ({})
    property string emptyTitle: ""
    property string emptyMessage: ""
    property string accessibleName: ""

    implicitHeight: timelineColumn.implicitHeight
    Accessible.role: Accessible.List
    Accessible.name: !steps || steps.length === 0
                     ? emptyTitle : accessibleName

    ColumnLayout {
        id: timelineColumn
        anchors.left: parent.left
        anchors.right: parent.right
        spacing: 10

        Rectangle {
            Layout.fillWidth: true
            visible: !timeline.steps || timeline.steps.length === 0
            radius: 8
            color: timeline.theme.cardColor
            border.color: timeline.theme.secondaryTextColor || "#718096"
            border.width: 1
            implicitHeight: emptyColumn.implicitHeight + 20

            ColumnLayout {
                id: emptyColumn
                anchors.fill: parent
                anchors.margins: 10
                spacing: 4

                Text {
                    Layout.fillWidth: true
                    text: timeline.emptyTitle
                    textFormat: Text.PlainText
                    color: timeline.theme.textColor || "#ffffff"
                    font.bold: true
                    wrapMode: Text.WordWrap
                }

                Text {
                    Layout.fillWidth: true
                    text: timeline.emptyMessage
                    textFormat: Text.PlainText
                    color: timeline.theme.secondaryTextColor || "#d3d8e0"
                    wrapMode: Text.WordWrap
                }
            }
        }

        Repeater {
            model: timeline.steps || []

            delegate: RowLayout {
                id: stepRow
                Layout.fillWidth: true
                spacing: 10
                Accessible.role: Accessible.ListItem
                Accessible.name: String(modelData.statusText || "") + ". "
                                 + String(modelData.label || "")

                ColumnLayout {
                    Layout.alignment: Qt.AlignTop
                    Layout.preferredWidth: 34
                    spacing: 0

                    Rectangle {
                        Layout.alignment: Qt.AlignHCenter
                        width: 30
                        height: 30
                        radius: 15
                        color: timeline.theme.textColor
                        border.color: modelData.statusBorderColor
                                      || timeline.theme.secondaryTextColor
                                      || "#ffffff"
                        border.width: 1

                        Text {
                            anchors.centerIn: parent
                            text: modelData.statusIcon || "?"
                            textFormat: Text.PlainText
                            color: timeline.theme.cardColor
                            font.bold: true
                            font.pixelSize: 17
                            Accessible.ignored: true
                        }
                    }

                    Rectangle {
                        Layout.alignment: Qt.AlignHCenter
                        width: 2
                        Layout.preferredHeight: 16
                        visible: index < timeline.steps.length - 1
                        color: timeline.theme.secondaryTextColor || "#718096"
                        Accessible.ignored: true
                    }
                }

                Rectangle {
                    Layout.fillWidth: true
                    radius: 8
                    color: timeline.theme.cardColor
                    border.color: timeline.theme.secondaryTextColor || "#718096"
                    border.width: 1
                    implicitHeight: stepColumn.implicitHeight + 18

                    ColumnLayout {
                        id: stepColumn
                        anchors.fill: parent
                        anchors.margins: 9
                        spacing: 4

                        RowLayout {
                            Layout.fillWidth: true
                            spacing: 8

                            Text {
                                Layout.fillWidth: true
                                text: modelData.label || ""
                                textFormat: Text.PlainText
                                color: timeline.theme.textColor || "#ffffff"
                                font.bold: true
                                wrapMode: Text.WordWrap
                            }

                            Label {
                                text: modelData.statusText || ""
                                color: timeline.theme.textColor || "#ffffff"
                                font.bold: true
                                leftPadding: 7
                                rightPadding: 7
                                topPadding: 3
                                bottomPadding: 3
                                background: Rectangle {
                                    radius: 9
                                    color: timeline.theme.cardColor
                                    border.color: modelData.statusBorderColor
                                                  || timeline.theme.secondaryTextColor
                                                  || "#718096"
                                    border.width: 1
                                }
                            }
                        }

                        Text {
                            Layout.fillWidth: true
                            visible: String(modelData.ownerText || "") !== ""
                            text: modelData.ownerText || ""
                            textFormat: Text.PlainText
                            color: timeline.theme.secondaryTextColor || "#d3d8e0"
                            font.pixelSize: 11
                            wrapMode: Text.WordWrap
                        }

                        Text {
                            Layout.fillWidth: true
                            visible: String(modelData.message || "") !== ""
                            text: modelData.message || ""
                            textFormat: Text.PlainText
                            color: timeline.theme.secondaryTextColor || "#d3d8e0"
                            wrapMode: Text.WordWrap
                        }

                        Text {
                            Layout.fillWidth: true
                            visible: String(modelData.actionText || "") !== ""
                            text: modelData.actionText || ""
                            textFormat: Text.PlainText
                            color: timeline.theme.secondaryTextColor || "#d3d8e0"
                            font.italic: true
                            wrapMode: Text.WordWrap
                        }
                    }
                }
            }
        }
    }
}
