// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.nfc

import es.gob.jmulticard.CryptoHelper
import es.gob.jmulticard.card.PrivateKeyReference
import es.gob.jmulticard.card.dnie.DnieNfc
import es.gob.jmulticard.connection.ApduConnection
import es.gob.jmulticard.connection.cwa14890.Cwa14890Connection
import javax.security.auth.callback.CallbackHandler

/**
 * Estado del PIN en una operación de varias firmas (lote). El PIN se pide una
 * vez y se conserva solo en memoria mientras dura la operación. Al primer
 * fallo no se vuelve a hablar con la tarjeta: repetir un PIN erróneo en cada
 * documento gastaría los intentos y bloquearía el DNIe.
 */
internal class DniePinHold {
    var holding: Boolean = false
        private set
    var aborted: Boolean = false
        private set

    fun begin() {
        holding = true
        aborted = false
    }

    /** Fuera de una operación, cada firma borra el PIN como hasta ahora. */
    fun keepPinAfterSuccess(): Boolean = holding

    fun failed() {
        if (holding) aborted = true
    }

    fun mayContactCard(): Boolean = !aborted

    fun end() {
        holding = false
        aborted = false
    }
}

/**
 * DnieNfc que, tras cada firma, cierra el canal seguro de PIN como hace el
 * propio DnieNfc.sign de jmulticard. La firma siguiente vuelve a abrir PACE y
 * a verificar el PIN con el PasswordCallback de la sesión, que es lo que exige
 * la clave de FIRMA del DNIe para cada operación.
 */
internal class GrxDnieNfc(
    connection: ApduConnection,
    helper: CryptoHelper,
    handler: CallbackHandler,
) : DnieNfc(connection, null, helper, handler) {
    fun signDigestInfo(block: ByteArray, key: PrivateKeyReference): ByteArray = try {
        cipherData(block, key)
    } finally {
        closeSecureChannel()
    }

    private fun closeSecureChannel() {
        try {
            val current = getConnection()
            if (current is Cwa14890Connection) setConnection(current.subConnection)
            selectMasterFile()
        } catch (_: Exception) {
            // Si la tarjeta se ha retirado, la firma siguiente fallará al abrir el canal.
        }
    }
}
