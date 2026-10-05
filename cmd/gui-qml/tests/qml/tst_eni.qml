// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
import QtQuick
import QtQuick.Controls
import QtTest
import "../../qml"

TestCase {
    id: test
    name: "EniFields"
    width: 700
    height: 900
    when: windowShown
    QtObject { id: backend }
    EniPanel {
        id: panel
        bridge: backend
        anchors.fill: parent
        theme: ({textColor: "black", cardColor: "white", secondaryTextColor: "gray"})
    }
    function field(name) { return findChild(panel, name) }
    function test_defaults_and_dropdowns() {
        compare(field("state").count, 5)
        compare(field("docType").count, 21)
        compare(field("fileState").count, 3)
        compare(panel.documentOptions()["eni.estado"], "EE01")
        compare(panel.documentOptions()["eni.tipoDocumental"], "TD99")
        compare(panel.fileOptions()["exp.estado"], "E01")
        compare(field("capture").errorKey, "")
        compare(field("opened").errorKey, "")
    }
    function test_inline_error_and_first_focus() {
        const organ = field("docOrgan")
        organ.text = "mal"
        verify(!panel.validateFields([organ, field("capture")]))
        verify(organ.invalid)
        verify(findChild(organ, "eniInput").activeFocus)
        organ.text = "L01180877"
        verify(!organ.invalid)
        verify(panel.validateFields([organ, field("capture")]))
        field("capture").text = "2026-02-30T12:00:00Z"
        verify(!panel.validateFields([organ, field("capture")]))
        field("capture").text = "2026-10-04T13:42:00+02:00"
    }
    function test_readable_date_and_time_drive_rfc3339() {
        const capture = field("capture")
        capture.text = "2026-10-04T13:42:00+02:00"
        const date = findChild(capture, "eniDateInput")
        const time = findChild(capture, "eniTimeInput")
        const local = new Date("2026-10-04T13:42:00+02:00")
        compare(date.text, String(local.getDate()).padStart(2, "0") + "/" + String(local.getMonth() + 1).padStart(2, "0") + "/2026")
        date.text = "05/10/2026"
        time.text = "08:15"
        date.textEdited()
        const parsed = new Date(capture.text)
        compare(parsed.getDate(), 5)
        compare(parsed.getHours(), 8)
        compare(parsed.getMinutes(), 15)
        compare(capture.errorKey, "")
        date.text = "31/02/2026"
        date.textEdited()
        compare(capture.errorKey, "eni.validacion.date")
        capture.text = "2026-10-04T13:42:00+02:00"
    }
    function test_calendar_keyboard_moves_and_picks_day() {
        const capture = field("capture")
        capture.text = "2026-10-04T13:42:00+02:00"
        const start = new Date(capture.text)
        capture.calendarRequested()
        const popup = findChild(capture, "calendarPopup")
        tryCompare(popup, "opened", true)
        const grid = findChild(capture, "calendarGrid")
        tryCompare(grid, "activeFocus", true)
        keyClick(Qt.Key_Right)
        keyClick(Qt.Key_Down)
        keyClick(Qt.Key_Return)
        tryCompare(popup, "opened", false)
        const picked = new Date(capture.text)
        compare(picked.getDate(), start.getDate() + 8)
        compare(picked.getHours(), start.getHours())
    }
    function test_missing_input_is_explained_next_to_the_button() {
        panel.signaturePath = ""
        findChild(panel, "createDocumentButton").clicked()
        compare(panel.documentMessageKey, "paridad.lote3.eni.required_signature")
        verify(findChild(panel, "signatureButton").activeFocus)
        panel.directoryPath = ""
        findChild(panel, "createFileButton").clicked()
        compare(panel.fileMessageKey, "paridad.lote3.eni.required_folder")
        verify(findChild(panel, "folderButton").activeFocus)
    }
    function test_empty_certificate_list_is_explained_and_messages_follow_language() {
        panel.certificates = []
        compare(field("certificateCombo").displayText, "paridad.lote3.eni.required_certificate")
        panel.signaturePath = ""
        findChild(panel, "createDocumentButton").clicked()
        // El mensaje se pinta desde la clave: cambia si cambia la traducción.
        panel.translate = function(key) { return "EN:" + key }
        compare(findChild(panel, "documentMessage").text, "EN:paridad.lote3.eni.required_signature")
        panel.translate = function(key) { return key }
    }
    function test_calendar_follows_application_language() {
        panel.localeName = "en"
        compare(findChild(field("capture"), "calendarGrid").locale.name, Qt.locale("en").name)
        panel.localeName = ""
    }
    function test_calendar_opens_and_preserves_time() {
        const capture = field("capture")
        capture.text = "2026-10-04T13:42:00+02:00"
        capture.calendarRequested()
        const popup = findChild(capture, "calendarPopup")
        tryCompare(popup, "opened", true)
        const grid = findChild(capture, "calendarGrid")
        grid.clicked(new Date(2026, 9, 7))
        tryCompare(popup, "opened", false)
        const selected = new Date(capture.text)
        compare(selected.getDate(), 7)
        // La hora local se conserva al seleccionar otro día.
        compare(selected.getHours(), new Date("2026-10-04T13:42:00+02:00").getHours())
        compare(capture.errorKey, "")
    }
}
