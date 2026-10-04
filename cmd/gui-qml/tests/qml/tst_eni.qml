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
