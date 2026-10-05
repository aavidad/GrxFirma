// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.nfc

import android.nfc.Tag
import android.nfc.tech.IsoDep
import es.gob.jmulticard.CryptoHelper
import es.gob.jmulticard.android.nfc.AndroidNfcConnection
import es.gob.jmulticard.callback.CustomTextInputCallback
import java.security.cert.X509Certificate
import javax.security.auth.callback.Callback
import javax.security.auth.callback.CallbackHandler
import javax.security.auth.callback.PasswordCallback

internal object DnieInput {
    fun validCan(can: CharArray): Boolean = can.size == 6 && can.all { it in '0'..'9' }
    fun validPin(pin: CharArray): Boolean = pin.size in 4..16 && pin.none { Character.isISOControl(it) }
}

internal class DnieNfcSession private constructor(
    private val connection: AndroidNfcConnection,
    private val card: GrxDnieNfc,
    private val alias: String,
    val certificate: X509Certificate,
    val chain: List<X509Certificate>,
) : AutoCloseable {
    private var passwordCallback: PasswordCallback? = null
    @Volatile private var signingError: Throwable? = null
    @Volatile private var signingRetriesLeft: Int = -1
    private val pinHold = DniePinHold()

    /** La tarjeta recibe solo un DigestInfo PKCS#1, nunca un documento ni una clave exportada. */
    @Synchronized fun signDigest(digest: ByteArray, hashName: String): ByteArray {
        // Tras un fallo en una operación de varias firmas no se vuelve a usar el PIN.
        if (!pinHold.mayContactCard()) throw IllegalStateException("OPERATION_ABORTED")
        passwordCallback ?: throw IllegalStateException("PIN_REQUIRED")
        val header = when (hashName) {
            "SHA-256" -> byteArrayOf(0x30,0x31,0x30,0x0d,0x06,0x09,0x60,0x86.toByte(),0x48,0x01,0x65,0x03,0x04,0x02,0x01,0x05,0x00,0x04,0x20)
            "SHA-384" -> byteArrayOf(0x30,0x41,0x30,0x0d,0x06,0x09,0x60,0x86.toByte(),0x48,0x01,0x65,0x03,0x04,0x02,0x02,0x05,0x00,0x04,0x30)
            "SHA-512" -> byteArrayOf(0x30,0x51,0x30,0x0d,0x06,0x09,0x60,0x86.toByte(),0x48,0x01,0x65,0x03,0x04,0x02,0x03,0x05,0x00,0x04,0x40)
            else -> throw IllegalArgumentException("HASH_UNSUPPORTED")
        }
        require(digest.size == header.last().toInt())
        val modulusBytes = (certificate.publicKey as java.security.interfaces.RSAPublicKey).modulus.bitLength().let { (it + 7) / 8 }
        val paddingLength = modulusBytes - header.size - digest.size - 3
        require(paddingLength >= 8)
        val block = ByteArray(modulusBytes)
        block[1] = 1
        block.fill(0xff.toByte(), 2, 2 + paddingLength)
        block[2 + paddingLength] = 0
        header.copyInto(block, 3 + paddingLength)
        digest.copyInto(block, 3 + paddingLength + header.size)
        var signed = false
        try {
            // cipherData abre el canal CWA de PIN y lo verifica con el callback;
            // después se cierra para que cada firma vuelva a verificar el PIN.
            return card.signDigestInfo(block, card.getPrivateKey(alias)).also { signed = true }
        } catch (error: Exception) {
            signingRetriesLeft = try { card.pinRetriesLeft } catch (_: Exception) { -1 }
            if (signingError == null) signingError = error
            pinHold.failed()
            throw error
        } finally {
            block.fill(0)
            if (!signed || !pinHold.keepPinAfterSuccess()) clearPinLocked()
        }
    }

    @Synchronized fun setPin(pin: CharArray) {
        require(DnieInput.validPin(pin))
        signingError = null
        installPinLocked(pin)
    }

    /**
     * Pide el PIN una sola vez para varias firmas seguidas (lote). El PIN queda
     * en memoria, dentro del PasswordCallback, hasta [endOperation].
     */
    @Synchronized fun beginOperation(pin: CharArray) {
        require(DnieInput.validPin(pin))
        signingError = null
        installPinLocked(pin)
        pinHold.begin()
    }

    @Synchronized fun endOperation() {
        pinHold.end()
        clearPinLocked()
    }

    @Synchronized fun clearPin() {
        pinHold.end()
        clearPinLocked()
    }

    private fun installPinLocked(pin: CharArray) {
        passwordCallback?.clearPassword()
        // PasswordCallback guarda su propia copia; quien llama borra la suya.
        passwordCallback = PasswordCallback("", false).also { it.password = pin }
        card.setPasswordCallback(passwordCallback)
    }

    private fun clearPinLocked() {
        passwordCallback?.clearPassword()
        passwordCallback = null
        card.setPasswordCallback(null)
    }

    fun retriesLeft(): Int = signingRetriesLeft

    fun consumeSigningError(): Throwable? = signingError.also { signingError = null }

    @Synchronized override fun close() {
        clearPin()
        try { connection.close() } catch (_: Exception) { }
    }

    companion object {
        internal fun createCryptoHelper(): CryptoHelper = SecureRandomBcCryptoHelper()

        fun open(tag: Tag, can: CharArray): DnieNfcSession {
            require(DnieInput.validCan(can))
            if (IsoDep.get(tag) == null) throw IllegalArgumentException("NOT_ISODEP")
            val connection = AndroidNfcConnection(tag)
            try {
                val helper = createCryptoHelper()
                val handler = CallbackHandler { callbacks: Array<out Callback> ->
                    callbacks.forEach { callback ->
                        when (callback) {
                            is CustomTextInputCallback -> callback.text = String(can)
                            else -> {
                                // La API JSE puede usar TextInputCallback; Android usa CustomTextInputCallback.
                                if (callback.javaClass.name != "javax.security.auth.callback.TextInputCallback") {
                                    throw IllegalArgumentException("UNSUPPORTED_NFC_CALLBACK")
                                }
                                callback.javaClass.getMethod("setText", String::class.java)
                                    .invoke(callback, String(can))
                            }
                        }
                    }
                }
                // getDnieNfc compara un ATR completo, pero AndroidNfcConnection.reset()
                // devuelve solo los bytes históricos de IsoDep. El constructor abre
                // PACE y valida la tarjeta mediante las respuestas APDU reales.
                val card = GrxDnieNfc(connection, helper, handler)
                val candidates = card.aliases.mapNotNull { alias ->
                    card.getCertificate(alias)?.let { alias to it }
                }
                val selected = candidates.firstOrNull { (alias, cert) ->
                    // keyUsage[0] es firma digital y [1] no repudio; el certificado
                    // de FIRMA del DNIe solo lleva no repudio (como admite escritorio).
                    alias.contains("firma", ignoreCase = true) && card.getPrivateKey(alias) != null &&
                        cert.keyUsage.let { it == null || it.getOrNull(0) == true || it.getOrNull(1) == true }
                } ?: throw IllegalArgumentException("SIGN_CERT_MISSING")
                selected.second.checkValidity()
                val availableCa = candidates.map { it.second }.filter { it.basicConstraints >= 0 && it != selected.second }
                val ca = mutableListOf<X509Certificate>()
                var child = selected.second
                while (ca.size < 16) {
                    val issuer = availableCa.firstOrNull { it.subjectX500Principal == child.issuerX500Principal && it !in ca }
                        ?: break
                    ca += issuer
                    child = issuer
                }
                return DnieNfcSession(connection, card, selected.first, selected.second, ca)
            } catch (error: Exception) {
                try { connection.close() } catch (_: Exception) { }
                throw error
            }
        }
    }
}
