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
    property var theme: ({cardColor: "#ffffff", textColor: "#202124", secondaryTextColor: "#555555", primaryColor: "#2980b9"})
    property var dialog
    SignalSpy { id: cancelled; target: tests.dialog; signalName: "cancelled" }
    QtObject {
        id: fake
        property var calls: []
        signal unsafeExceptionsFinished(string action, bool ok, var data)
        function loadUnsafeExceptions() { calls.push({action: "list"}) }
        function activateUnsafeException(id, version, origin, retry) { calls.push({action: "activate", id: id, version: version, origin: origin, retry: retry}) }
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
        // Desde Configuración no hay operación detenida: sin reintento.
        compare(fake.calls[0].origin, "ajustes")
        compare(fake.calls[0].retry, "")
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
    // Lote 39a: desde Configuración el botón solo activa; «Activar y
    // continuar» se reserva para un bloqueo que se puede repetir.
    function test_activate_label_depends_on_pending_operation() {
        const fromPreferences = entry(true)
        dialog.present(fromPreferences, false)
        compare(findChild(dialog, "unsafeActivate").text, "excepciones.pantalla.activar")
        const blockedRetry = entry(true); blockedRetry.operationPending = true
        dialog.present(blockedRetry, false)
        compare(findChild(dialog, "unsafeActivate").text, "excepciones.activar_continuar")
        const blockedResend = entry(true); blockedResend.operationPending = true
        blockedResend.retryId = ""; blockedResend.resend = true
        dialog.present(blockedResend, false)
        compare(findChild(dialog, "unsafeActivate").text, "excepciones.activar_continuar")
        const blockedNoRepeat = entry(true); blockedNoRepeat.operationPending = true; blockedNoRepeat.retryId = ""
        dialog.present(blockedNoRepeat, false)
        compare(dialog.title, "excepciones.pantalla.titulo")
        compare(findChild(dialog, "unsafeActivate").text, "excepciones.pantalla.activar")
    }
    function test_primary_action_stands_out_from_secondary() {
        dialog.present(entry(true), false)
        const primary = findChild(dialog, "unsafeActivate")
        const cancel = findChild(dialog, "unsafeCancel")
        verify(primary.font.bold)
        verify(!cancel.font.bold)
        verify(!Qt.colorEqual(primary.palette.button, cancel.palette.button))
        verify(Qt.colorEqual(primary.palette.button, tests.theme.primaryColor))
    }
    function test_help_200_percent_and_escape() {
        Typography.textScalePercent = 200
        dialog.present(entry(true), true)
        verify(dialog.showingHelp)
        verify(!findChild(dialog, "unsafeAcceptance").visible)
        verify(dialog.width <= tests.width)
        verify(dialog.height <= tests.height)
        const cancel = findChild(dialog, "unsafeCancel")
        tryVerify(function() { return findChild(dialog, "unsafeHeading").activeFocus })
        const position = cancel.mapToItem(tests, 0, 0)
        verify(position.y >= 0 && position.y + cancel.height <= tests.height)
        keyClick(Qt.Key_Escape)
        tryCompare(dialog, "visible", false)
    }
    // Lote 39f: el recibo sabe si se aceptó en el aviso o en Configuración, y
    // el reintento solo viaja con el aviso de una operación detenida.
    function test_activation_sends_origin_and_retry_only_from_a_block() {
        const blocked = entry(true); blocked.operationPending = true
        dialog.present(blocked, false)
        findChild(dialog, "unsafeAcceptance").checked = true
        dialog.activate()
        compare(fake.calls[0].origin, "aviso")
        compare(fake.calls[0].retry, "token")
    }
    // Lote 39f: el foco empieza en el nombre del caso, no en «Más información»
    // ni en la casilla, también al abrir la ayuda sola.
    function test_initial_focus_is_the_heading() {
        for (const help of [false, true]) {
            const value = entry(true); value.operationPending = !help
            dialog.present(value, help)
            const heading = findChild(dialog, "unsafeHeading")
            tryVerify(function() { return heading.activeFocus })
            compare(heading.Accessible.role, Accessible.Heading)
            compare(heading.Accessible.name, "excepciones.cms_cbc.ajuste")
            dialog.close()
            tryCompare(dialog, "visible", false)
        }
    }
    // Con teclado: Tab llega a la casilla, Espacio la marca, Intro no la
    // cambia ni activa, Tab llega a los botones y Mayús+Tab vuelve atrás.
    function test_keyboard_marks_activates_and_goes_back() {
        dialog.present(entry(true), false)
        const heading = findChild(dialog, "unsafeHeading")
        const box = findChild(dialog, "unsafeAcceptance")
        const cancel = findChild(dialog, "unsafeCancel")
        const activate = findChild(dialog, "unsafeActivate")
        tryVerify(function() { return heading.activeFocus })
        keyClick(Qt.Key_Tab)
        tryVerify(function() { return box.activeFocus })
        keyClick(Qt.Key_Return)
        keyClick(Qt.Key_Enter)
        verify(!box.checked)
        compare(fake.calls.length, 0)
        keyClick(Qt.Key_Space)
        verify(box.checked)
        keyClick(Qt.Key_Return)
        verify(box.checked)
        compare(fake.calls.length, 0)
        keyClick(Qt.Key_Backtab, Qt.ShiftModifier)
        tryVerify(function() { return heading.activeFocus })
        keyClick(Qt.Key_Tab)
        tryVerify(function() { return box.activeFocus })
        keyClick(Qt.Key_Tab)
        tryVerify(function() { return cancel.activeFocus })
        keyClick(Qt.Key_Tab)
        keyClick(Qt.Key_Tab)
        tryVerify(function() { return activate.activeFocus })
        keyClick(Qt.Key_Space)
        compare(fake.calls.length, 1)
        compare(fake.calls[0].action, "activate")
    }
    function test_keyboard_cancel_and_escape_cancel() {
        dialog.present(entry(true), false)
        const box = findChild(dialog, "unsafeAcceptance")
        tryVerify(function() { return findChild(dialog, "unsafeHeading").activeFocus })
        keyClick(Qt.Key_Tab)
        tryVerify(function() { return box.activeFocus })
        keyClick(Qt.Key_Space)
        keyClick(Qt.Key_Escape)
        tryCompare(dialog, "visible", false)
        tryCompare(cancelled, "count", 1)
        compare(fake.calls.length, 0)
        dialog.present(entry(true), false)
        tryVerify(function() { return findChild(dialog, "unsafeHeading").activeFocus })
        keyClick(Qt.Key_Tab)
        keyClick(Qt.Key_Tab)
        tryVerify(function() { return findChild(dialog, "unsafeCancel").activeFocus })
        keyClick(Qt.Key_Space)
        tryCompare(dialog, "visible", false)
        tryCompare(cancelled, "count", 2)
        compare(fake.calls.length, 0)
    }
    // Con letra grande y poca altura, la casilla queda a la vista al llegar
    // con Tab, y el nombre del caso al volver con Mayús+Tab.
    function test_focused_item_is_scrolled_into_view() {
        Typography.textScalePercent = 200
        dialog.present(entry(true), false)
        dialog.height = 340
        const heading = findChild(dialog, "unsafeHeading")
        const box = findChild(dialog, "unsafeAcceptance")
        const flick = box.parent.parent.parent
        tryVerify(function() { return heading.activeFocus })
        // Espera a que la ventana quepa justa: el texto desborda y hay barra.
        tryVerify(function() { return flick.height > box.height + 16 && flick.contentHeight > flick.height })
        keyClick(Qt.Key_Tab)
        tryVerify(function() { return box.activeFocus })
        function visibleIn(item) {
            const y = item.mapToItem(dialog.contentItem, 0, 0).y
            const area = flick.mapToItem(dialog.contentItem, 0, 0).y
            return y >= area - 1 && y + Math.min(item.height, flick.height) <= area + flick.height + 1
        }
        tryVerify(function() { return visibleIn(box) })
        keyClick(Qt.Key_Backtab, Qt.ShiftModifier)
        tryVerify(function() { return heading.activeFocus })
        tryVerify(function() { return visibleIn(heading) })
    }
}
