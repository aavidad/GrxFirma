// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.nfc

import es.gob.jmulticard.CryptoHelper
import es.gob.jmulticard.DigestAlgorithm
import es.gob.jmulticard.apdu.iso7816four.pace.PaceChat
import es.gob.jmulticard.asn1.icao.CardAccess
import es.gob.jmulticard.crypto.BcCryptoHelper
import java.security.KeyPair
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.security.interfaces.RSAKey
import java.security.interfaces.RSAPublicKey
import java.security.spec.AlgorithmParameterSpec

/** jmulticard 2.0 no siembra su DigestRandomGenerator; CWA usa esta salida como secreto. */
internal class SecureRandomBcCryptoHelper(
    internal val bc: BcCryptoHelper = BcCryptoHelper(),
    private val random: SecureRandom = SecureRandom(),
) : CryptoHelper() {
    override fun generateRandomBytes(numBytes: Int): ByteArray = ByteArray(numBytes).also(random::nextBytes)
    override fun digest(algorithm: DigestAlgorithm, data: ByteArray): ByteArray = bc.digest(algorithm, data)
    override fun desedeEncrypt(data: ByteArray, key: ByteArray): ByteArray = bc.desedeEncrypt(data, key)
    override fun desedeDecrypt(data: ByteArray, key: ByteArray): ByteArray = bc.desedeDecrypt(data, key)
    override fun desEncrypt(data: ByteArray, key: ByteArray): ByteArray = bc.desEncrypt(data, key)
    override fun desDecrypt(data: ByteArray, key: ByteArray): ByteArray = bc.desDecrypt(data, key)
    override fun aesDecrypt(
        data: ByteArray, iv: ByteArray?, key: ByteArray, blockMode: BlockMode, padding: Padding,
    ): ByteArray = bc.aesDecrypt(data, iv, key, blockMode, padding)
    override fun aesEncrypt(
        data: ByteArray, iv: ByteArray?, key: ByteArray, blockMode: BlockMode, padding: Padding,
    ): ByteArray = bc.aesEncrypt(data, iv, key, blockMode, padding)
    override fun rsaDecrypt(data: ByteArray, key: RSAKey): ByteArray = bc.rsaDecrypt(data, key)
    override fun rsaEncrypt(data: ByteArray, key: RSAKey): ByteArray = bc.rsaEncrypt(data, key)
    override fun generateEcKeyPair(curve: EcCurve): KeyPair = bc.generateEcKeyPair(curve)
    override fun doAesCmac(data: ByteArray, key: ByteArray): ByteArray = bc.doAesCmac(data, key)
    override fun getEcPoint(nonce: ByteArray, sharedSecret: ByteArray, curve: EcCurve): AlgorithmParameterSpec =
        bc.getEcPoint(nonce, sharedSecret, curve)
    override fun getCmsSignatureSignedContent(data: ByteArray): ByteArray = bc.getCmsSignatureSignedContent(data)
    override fun validateCmsSignature(data: ByteArray): Array<X509Certificate> = bc.validateCmsSignature(data)
    override fun getRsaPublicKey(cert: X509Certificate): RSAPublicKey = bc.getRsaPublicKey(cert)
    override fun getPaceChannelHelper(cardAccess: CardAccess, paceChat: PaceChat?): PaceChannelHelper =
        bc.getPaceChannelHelper(cardAccess, paceChat)
}
