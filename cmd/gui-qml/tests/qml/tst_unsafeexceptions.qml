// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtTest
import "../../qml"

TestCase {
    id: tests
    name: "UnsafeExceptions"
    width: 900; height: 800
    when: windowShown
    property var theme: ({cardColor: "#ffffff", textColor: "#202124", secondaryTextColor: "#555555"})
    property var dialog
    SignalSpy { id: cancelled; target: tests.dialog; signalName: "cancelled" }
    QtObject {
        id: fake
        property var calls: []
        signal unsafeExceptionsFinished(string action, bool ok, var data)
        function loadUnsafeExceptions() { calls.push({action: "list"}) }
        function activateUnsafeException(id, version, retry) { calls.push({action: "activate", id: id, version: version, retry: retry}) }
        function disableUnsafeException(id) { calls.push({action: "disable", id: id}) }
    }
    Component { id: factory; UnsafeExceptionDialog { theme: tests.theme; bridge: fake; translate: function(key) { return key } } }
    Component { id: panelFactory; UnsafeExceptionsPanel { width: 700; theme: tests.theme; bridge: fake; localIpc: true; translate: function(key) { return key } } }
    function entry(activable) { return {id: "cms_cbc", activable: activable, acceptance_version: "2026-11-15.1", retryId: "token", fixed: !activable, enabled: false} }
    function init() { fake.calls = []; dialog = createTemporaryObject(factory, tests); verify(dialog !== null); cancelled.clear() }
    function cleanup() { dialog.close(); Typography.textScalePercent = 100 }
    function test_acceptance_gates_and_versioned_ipc() {
        dialog.present(entry(true), false)
        const box = findChild(dialog, "unsafeAcceptance")
        const button = findChild(dialog, "unsafeActivate")
        verify(box.visible)
        verify(!button.enabled)
        dialog.activate()
        compare(fake.calls.length, 0)
        box.checked = true
        verify(button.enabled)
        dialog.activate()
        compare(fake.calls.length, 1)
        compare(fake.calls[0].version, "2026-11-15.1")
        compare(fake.calls[0].retry, "token")
        verify(!button.enabled)
        fake.unsafeExceptionsFinished("unsafe_exceptions_activate", false, {})
        verify(!box.checked)
        verify(!button.enabled)
        verify(dialog.visible)
    }
    function test_organization_has_no_activation() {
        dialog.present(entry(false), false)
        verify(!findChild(dialog, "unsafeAcceptance").visible)
        verify(!findChild(dialog, "unsafeActivate").visible)
        dialog.activate()
        compare(fake.calls.length, 0)
    }
    function test_reopen_resets_acceptance_and_missing_version_fails_closed() {
        dialog.present(entry(true), false)
        findChild(dialog, "unsafeAcceptance").checked = true
        dialog.close()
        dialog.present(entry(true), false)
        verify(!dialog.canActivate)
        dialog.item = {id: "cms_cbc", activable: true}
        findChild(dialog, "unsafeAcceptance").checked = true
        verify(!dialog.canActivate)
    }
    function test_preferences_fixed_is_readonly() {
        const panel = createTemporaryObject(panelFactory, tests)
        verify(panel !== null)
        panel.load()
        fake.unsafeExceptionsFinished("unsafe_exceptions_list", true, [entry(false)])
        compare(panel.items.length, 1)
        wait(0)
        verify(!findChild(panel, "unsafeChange-cms_cbc").visible)
        panel.change(panel.items[0])
        compare(fake.calls.length, 1)
        compare(panel.stateText(panel.items[0]), "excepciones.pantalla.fijada_inactiva")
        const unreadable = entry(false); unreadable.policy_unreadable = true
        compare(panel.stateText(unreadable), "excepciones.pantalla.directiva_ilegible")
        const editable = entry(true); editable.fixed = false; editable.enabled = true
        panel.change(editable)
        compare(fake.calls[1].action, "disable")
    }
    function test_preferences_heading_scales_and_wraps() {
        const panel = createTemporaryObject(panelFactory, tests, {width: 360})
        verify(panel !== null)
        compare(panel.label.font.pixelSize, Typography.size18)
        const originalSize = panel.label.font.pixelSize
        Typography.textScalePercent = 200
        wait(0)
        compare(panel.label.font.pixelSize, Typography.size18)
        verify(panel.label.font.pixelSize >= originalSize * 1.9)
        verify(panel.label.height > panel.label.font.pixelSize)
        verify(panel.label.x >= 0)
        verify(panel.label.x + panel.label.width <= panel.width)
        verify(panel.label.y + panel.label.height <= panel.contentItem.y)
    }
    function test_cancel_does_not_run_after_successful_activation() {
        dialog.present(entry(true), false)
        dialog.close()
        tryCompare(cancelled, "count", 1)
        dialog.present(entry(true), false)
        findChild(dialog, "unsafeAcceptance").checked = true
        dialog.activate()
        fake.unsafeExceptionsFinished("unsafe_exceptions_activate", true, {})
        tryCompare(dialog, "visible", false)
        compare(cancelled.count, 1)
    }
    function test_dialog_title_reflects_origin() {
        const fromPreferences = entry(true)
        dialog.present(fromPreferences, false)
        compare(dialog.title, "excepciones.pantalla.aceptar_titulo")
        fromPreferences.operationPending = true
        dialog.present(fromPreferences, false)
        compare(dialog.title, "excepciones.pantalla.titulo")
    }
    function test_help_200_percent_and_escape() {
        Typography.textScalePercent = 200
        dialog.present(entry(true), true)
        verify(dialog.showingHelp)
        verify(!findChild(dialog, "unsafeAcceptance").visible)
        verify(dialog.width <= tests.width)
        verify(dialog.height <= tests.height)
        const cancel = findChild(dialog, "unsafeCancel")
        tryVerify(function() { return cancel.activeFocus })
        const position = cancel.mapToItem(tests, 0, 0)
        verify(position.y >= 0 && position.y + cancel.height <= tests.height)
        keyClick(Qt.Key_Escape)
        tryCompare(dialog, "visible", false)
    }
}
