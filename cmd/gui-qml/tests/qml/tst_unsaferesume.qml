// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtTest
import "../../qml/UnsafeResume.js" as UnsafeResume

// Lote 39f: tras «Activar y continuar», una firma nunca se repite sin pasar
// por la confirmación de firma, tampoco si el motor ofrece un reintento.
TestCase {
    name: "UnsafeResume"
    function blocked(action, retryId) {
        const block = {operationPending: true, operationAction: action, retryId: retryId}
        block.resend = UnsafeResume.canResend(block)
        return block
    }
    function test_signatures_go_back_through_the_form() {
        for (const action of ["sign", "sign_multicosign", "sign_batch"]) {
            for (const retryId of ["", "token"]) {
                const block = blocked(action, retryId)
                verify(block.resend, action)
                compare(UnsafeResume.next(block), "resend", action + " " + retryId)
            }
        }
    }
    function test_other_operations_use_the_engine_retry() {
        compare(UnsafeResume.next(blocked("verify", "token")), "retry")
        compare(UnsafeResume.next(blocked("unprotect", "token")), "retry")
        verify(!UnsafeResume.canResend(blocked("verify", "token")))
    }
    function test_without_retry_or_resend_asks_to_press_again() {
        compare(UnsafeResume.next(blocked("verify", "")), "notice_resend")
        compare(UnsafeResume.next({operationPending: true, operationAction: "sign", resend: false, retryId: "token"}), "notice_resend")
    }
    function test_from_settings_only_confirms() {
        compare(UnsafeResume.next({id: "sha1_leer"}), "notice_activated")
        compare(UnsafeResume.next(null), "notice_activated")
        verify(!UnsafeResume.canResend({operationPending: false, operationAction: "sign"}))
    }
}
