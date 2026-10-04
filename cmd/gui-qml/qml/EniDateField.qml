// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "EniValidation.js" as Validation

EniValidatedField {
    id: root
    calendarEnabled: true
    maximumLength: 35
    text: Validation.rfc3339(new Date())
    validation: Validation.dateError
    onCalendarRequested: {
        const current = errorKey === "" ? new Date(text) : new Date()
        calendar.month = current.getMonth()
        calendar.year = current.getFullYear()
        calendarPopup.open()
    }
    Popup {
        id: calendarPopup
        objectName: "calendarPopup"
        width: Math.min(360, root.width)
        padding: 12
        modal: true
        focus: true
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
        // Mismo fondo y colores que el resto de diálogos del tema actual.
        palette.windowText: root.theme ? root.theme.textColor : "black"
        palette.text: root.theme ? root.theme.textColor : "black"
        palette.buttonText: root.theme ? root.theme.textColor : "black"
        palette.button: root.theme ? root.theme.cardColor : "white"
        background: Rectangle {
            radius: 8
            color: root.theme ? root.theme.cardColor : "white"
            border.color: root.theme ? root.theme.secondaryTextColor : "#767676"
            border.width: 1
        }
        ColumnLayout {
            width: calendarPopup.availableWidth
            RowLayout {
                Layout.fillWidth: true
                Button {
                    text: "‹"
                    Accessible.name: root.translate("eni.validacion.previous")
                    enabled: calendar.year > 1 || calendar.month > 0
                    onClicked: {
                        if (calendar.month === 0) { calendar.month = 11; calendar.year-- }
                        else calendar.month--
                    }
                }
                Label {
                    Layout.fillWidth: true
                    horizontalAlignment: Text.AlignHCenter
                    text: calendar.locale.toString(new Date(calendar.year, calendar.month, 1), "MMMM yyyy")
                }
                Button {
                    text: "›"
                    Accessible.name: root.translate("eni.validacion.next")
                    enabled: calendar.year < 9999 || calendar.month < 11
                    onClicked: {
                        if (calendar.month === 11) { calendar.month = 0; calendar.year++ }
                        else calendar.month++
                    }
                }
            }
            DayOfWeekRow {
                Layout.fillWidth: true
                locale: calendar.locale
            }
            MonthGrid {
                id: calendar
                objectName: "calendarGrid"
                Layout.fillWidth: true
                locale: Qt.locale()
                onClicked: function(date) {
                    const current = root.errorKey === "" ? new Date(root.text) : new Date()
                    const selected = new Date(date.getFullYear(), date.getMonth(), date.getDate(),
                                              current.getHours(), current.getMinutes(), current.getSeconds())
                    root.text = Validation.rfc3339(selected)
                    root.touched = true
                    calendarPopup.close()
                    root.focusInput()
                }
            }
        }
    }
}
