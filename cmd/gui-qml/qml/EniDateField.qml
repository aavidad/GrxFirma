// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "EniValidation.js" as Validation

// Fecha y hora legibles (dd/mm/aaaa y hh:mm). El valor que se envía al motor,
// `text`, sigue siendo RFC 3339 con la zona horaria local.
ColumnLayout {
    id: root
    property var theme
    property var translate: function(key) { return key }
    property string labelKey: ""
    property string timeLabelKey: ""
    // Código de idioma de la aplicación («es», «en»…); vacío usa el del sistema.
    property string localeName: ""
    property string text: Validation.rfc3339(new Date())
    property bool touched: false
    property bool syncing: false
    readonly property string errorKey: Validation.dateError(text)
    readonly property bool invalid: touched && errorKey !== ""
    readonly property color errorColor: root.theme && root.theme.errorColor ? root.theme.errorColor : "#b42318"
    readonly property color focusColor: root.theme && root.theme.focusColor ? root.theme.focusColor : "#1f5fa8"
    readonly property color textColor: root.theme ? root.theme.textColor : "black"
    readonly property color cardColor: root.theme ? root.theme.cardColor : "white"
    readonly property color mutedColor: root.theme ? root.theme.secondaryTextColor : "#595959"
    signal calendarRequested()
    spacing: 4

    function validateNow() { touched = true; return errorKey === "" }
    function focusInput() { dateInput.forceActiveFocus() }
    // Se valida aquí y no con errorKey: en onTextChanged el enlace aún puede no estar al día.
    function currentDate() { return Validation.dateError(text) === "" ? new Date(text) : null }
    function sameDay(a, b) {
        return !!a && !!b && a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
    }
    // Copia el valor RFC 3339 a los dos campos visibles.
    function syncInputs() {
        const date = currentDate()
        if (syncing || date === null) return
        syncing = true
        dateInput.text = Validation.displayDate(date)
        timeInput.text = Validation.displayTime(date)
        syncing = false
    }
    // Recalcula el valor RFC 3339 a partir de lo escrito; si no es válido,
    // deja un valor que la validación rechaza para mostrar el error.
    function updateFromInputs() {
        if (syncing) return
        const date = Validation.parseLocal(dateInput.text, timeInput.text)
        syncing = true
        text = date !== null ? Validation.rfc3339(date) : dateInput.text + " " + timeInput.text
        syncing = false
    }
    function pick(day) {
        const current = currentDate() || new Date()
        const selected = new Date(day.getFullYear(), day.getMonth(), day.getDate(),
                                  current.getHours(), current.getMinutes(), current.getSeconds())
        text = Validation.rfc3339(selected)
        touched = true
        calendarPopup.close()
        focusInput()
    }
    function moveFocusDate(days, months) {
        const base = calendar.focusDate
        const next = new Date(base.getFullYear(), base.getMonth() + months, base.getDate() + days)
        if (months !== 0 && next.getDate() !== base.getDate())
            next.setDate(0) // Último día del mes cuando el día no existe.
        if (next.getFullYear() < 1 || next.getFullYear() > 9999) return
        calendar.focusDate = next
        calendar.month = next.getMonth()
        calendar.year = next.getFullYear()
    }
    onTextChanged: syncInputs()
    Component.onCompleted: syncInputs()
    onCalendarRequested: {
        const current = currentDate() || new Date()
        calendar.focusDate = current
        calendar.month = current.getMonth()
        calendar.year = current.getFullYear()
        calendarPopup.open()
    }

    component ThemedInput: ThemedTextField {
        color: root.textColor
        fieldColor: root.cardColor
        mutedColor: root.mutedColor
        focusColor: root.focusColor
        errorColor: root.errorColor
        hasError: root.invalid
        onActiveFocusChanged: if (!activeFocus) root.touched = true
        onTextEdited: root.updateFromInputs()
    }

    RowLayout {
        id: dateRow
        Layout.fillWidth: true
        ThemedInput {
            id: dateInput
            objectName: "eniDateInput"
            Layout.fillWidth: true
            Layout.minimumWidth: 120
            maximumLength: 10
            inputMethodHints: Qt.ImhDate
            placeholderText: root.translate("paridad.lote3.eni.date_hint")
            Accessible.name: root.translate(root.labelKey)
            Accessible.description: root.invalid ? root.translate(root.errorKey) : root.translate("paridad.lote3.eni.date_hint")
        }
        ThemedButton {
            objectName: "calendarButton"
            text: root.translate("eni.validacion.calendar")
            Accessible.name: text
            onClicked: root.calendarRequested()
        }
    }
    Label {
        Layout.fillWidth: true
        text: root.translate("paridad.lote3.eni.date_hint")
        color: root.mutedColor
        wrapMode: Text.WordWrap
    }
    Label {
        Layout.fillWidth: true
        text: root.translate(root.timeLabelKey)
        color: root.textColor
        wrapMode: Text.WordWrap
        visible: root.timeLabelKey !== ""
    }
    RowLayout {
        Layout.fillWidth: true
        ThemedInput {
            id: timeInput
            objectName: "eniTimeInput"
            Layout.preferredWidth: 110
            maximumLength: 5
            inputMethodHints: Qt.ImhTime
            placeholderText: root.translate("paridad.lote3.eni.time_hint")
            Accessible.name: root.translate(root.timeLabelKey)
            Accessible.description: root.invalid ? root.translate(root.errorKey) : root.translate("paridad.lote3.eni.time_hint")
        }
        Label {
            Layout.fillWidth: true
            text: root.translate("paridad.lote3.eni.time_hint")
            color: root.mutedColor
            wrapMode: Text.WordWrap
        }
    }
    Label {
        Layout.fillWidth: true
        visible: root.invalid
        // El icono evita depender solo del color para reconocer el error.
        text: root.invalid ? "⚠ " + root.translate(root.errorKey) : ""
        color: root.errorColor
        wrapMode: Text.WordWrap
        // Rol de alerta: el lector de pantalla anuncia el error al aparecer.
        Accessible.role: Accessible.AlertMessage
        Accessible.name: root.invalid ? root.translate(root.errorKey) : ""
    }

    Popup {
        id: calendarPopup
        objectName: "calendarPopup"
        // Debajo de la fecha para no tapar el propio campo.
        y: dateRow.y + dateRow.height + 4
        width: Math.min(360, Math.max(280, root.width))
        padding: 12
        modal: true
        focus: true
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
        // Mismo fondo y colores que el resto de diálogos del tema actual.
        palette.windowText: root.textColor
        palette.text: root.textColor
        palette.buttonText: root.textColor
        palette.button: root.cardColor
        background: Rectangle {
            radius: 8
            color: root.cardColor
            border.color: root.mutedColor
            border.width: 1
        }
        onOpened: calendar.forceActiveFocus()
        onClosed: root.focusInput()
        ColumnLayout {
            width: calendarPopup.availableWidth
            RowLayout {
                Layout.fillWidth: true
                ThemedButton {
                    text: "‹"
                    Accessible.name: root.translate("eni.validacion.previous")
                    enabled: calendar.year > 1 || calendar.month > 0
                    onClicked: root.moveFocusDate(0, -1)
                }
                Label {
                    Layout.fillWidth: true
                    horizontalAlignment: Text.AlignHCenter
                    color: root.textColor
                    text: calendar.locale.toString(new Date(calendar.year, calendar.month, 1), "MMMM yyyy")
                }
                ThemedButton {
                    text: "›"
                    Accessible.name: root.translate("eni.validacion.next")
                    enabled: calendar.year < 9999 || calendar.month < 11
                    onClicked: root.moveFocusDate(0, 1)
                }
            }
            DayOfWeekRow {
                Layout.fillWidth: true
                locale: calendar.locale
                delegate: Label {
                    required property string shortName
                    text: shortName
                    color: root.mutedColor
                    horizontalAlignment: Text.AlignHCenter
                }
            }
            MonthGrid {
                id: calendar
                objectName: "calendarGrid"
                property date focusDate: new Date()
                Layout.fillWidth: true
                locale: root.localeName !== "" ? Qt.locale(root.localeName) : Qt.locale()
                focus: true
                activeFocusOnTab: true
                Accessible.role: Accessible.Table
                Accessible.name: calendar.locale.toString(calendar.focusDate, "dddd d MMMM yyyy")
                Accessible.description: root.translate("eni.validacion.calendar_help")
                Keys.onPressed: (event) => {
                    if (event.key === Qt.Key_Left) root.moveFocusDate(-1, 0)
                    else if (event.key === Qt.Key_Right) root.moveFocusDate(1, 0)
                    else if (event.key === Qt.Key_Up) root.moveFocusDate(-7, 0)
                    else if (event.key === Qt.Key_Down) root.moveFocusDate(7, 0)
                    else if (event.key === Qt.Key_PageUp) root.moveFocusDate(0, -1)
                    else if (event.key === Qt.Key_PageDown) root.moveFocusDate(0, 1)
                    else if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter || event.key === Qt.Key_Space)
                        root.pick(calendar.focusDate)
                    else return
                    event.accepted = true
                }
                delegate: Rectangle {
                    id: dayCell
                    required property date date
                    required property int day
                    required property int month
                    required property bool today
                    readonly property bool chosen: root.sameDay(dayCell.date, root.currentDate())
                    readonly property bool keyFocus: calendar.activeFocus && root.sameDay(dayCell.date, calendar.focusDate)
                    implicitWidth: 36
                    implicitHeight: 32
                    radius: 4
                    // Foco del teclado: relleno; día elegido: borde grueso; hoy: subrayado.
                    color: keyFocus ? root.textColor : "transparent"
                    border.color: root.focusColor
                    border.width: chosen ? 2 : 0
                    opacity: dayCell.month === calendar.month ? 1 : 0.6
                    Label {
                        anchors.centerIn: parent
                        text: dayCell.day
                        color: dayCell.keyFocus ? root.cardColor : root.textColor
                        font.bold: dayCell.chosen || dayCell.today
                        font.underline: dayCell.today
                    }
                }
                onClicked: (date) => root.pick(date)
            }
        }
    }
}
