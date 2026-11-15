// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

// Qué hace la interfaz después de activar una excepción desde el aviso de una
// operación detenida. Activar una excepción no es confirmar una firma: una
// firma vuelve a salir del formulario y pasa otra vez por la confirmación de
// firma si está activada, aunque el motor haya ofrecido un reintento.

var signingActions = ["sign", "sign_multicosign", "sign_batch"]

function isSigning(action) {
    return signingActions.indexOf(String(action || "")) !== -1
}

// Una firma detenida siempre se puede volver a lanzar desde el formulario.
function canResend(block) {
    return !!block && block.operationPending === true && isSigning(block.operationAction)
}

// «retry»: reintento del motor (no firma). «resend»: la firma vuelve a pasar
// por la confirmación y el formulario. «notice_resend»: se pide pulsar de
// nuevo el botón. «notice_activated»: se activó desde Configuración.
function next(item) {
    if (!item || item.operationPending !== true) return "notice_activated"
    if (isSigning(item.operationAction)) return item.resend === true ? "resend" : "notice_resend"
    if (item.retryId) return "retry"
    return "notice_resend"
}
