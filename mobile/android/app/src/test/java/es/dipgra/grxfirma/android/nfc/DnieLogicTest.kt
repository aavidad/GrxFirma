package es.dipgra.grxfirma.android.nfc

import es.gob.jmulticard.CryptoHelper
import es.gob.jmulticard.DigestAlgorithm
import es.gob.jmulticard.asn1.icao.CardAccess
import es.gob.jmulticard.crypto.BcCryptoHelper
import es.gob.jmulticard.crypto.BcPaceChannelHelper
import org.junit.Assert.assertEquals
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DnieLogicTest {
    @Test fun sessionBuildsRealCryptoHelper() {
        val helper = DnieNfcSession.createCryptoHelper()
        assertTrue(helper is SecureRandomBcCryptoHelper)
        assertEquals(BcCryptoHelper::class.java, (helper as SecureRandomBcCryptoHelper).bc.javaClass)
        assertFalse(helper.generateRandomBytes(32).contentEquals(helper.generateRandomBytes(32)))
    }

    @Test fun bcPaceBrainpoolAndAesCmacWorkWithoutCard() {
        val helper = DnieNfcSession.createCryptoHelper()
        val access = CardAccess(
            CardAccess.PaceAlgorithm.PACE_ECDH_GM_AES_CBC_CMAC_128,
            CardAccess.PaceAlgorithmParam.BRAINPOOL_256_R1,
            DigestAlgorithm.SHA1,
        )
        assertTrue(helper.getPaceChannelHelper(access, null) is BcPaceChannelHelper)
        val keyPair = helper.generateEcKeyPair(CryptoHelper.EcCurve.BRAINPOOL_P256_R1)
        assertNotNull(keyPair.private)
        assertNotNull(keyPair.public)

        // NIST SP 800-38B, AES-128 CMAC de un bloque; jmulticard devuelve 64 bits para PACE.
        val key = hex("2b7e151628aed2a6abf7158809cf4f3c")
        val data = hex("6bc1bee22e409f96e93d7e117393172a")
        assertArrayEquals(hex("070a16b46b4d4144"), helper.doAesCmac(data, key))
    }

    private fun hex(value: String): ByteArray = value.chunked(2)
        .map { it.toInt(16).toByte() }.toByteArray()

    @Test fun canHasExactlySixAsciiDigits() {
        assertTrue(DnieInput.validCan("012345".toCharArray()))
        assertFalse(DnieInput.validCan("12345".toCharArray()))
        assertFalse(DnieInput.validCan("1234567".toCharArray()))
        assertFalse(DnieInput.validCan("12345A".toCharArray()))
        assertFalse(DnieInput.validCan("١٢٣٤٥٦".toCharArray()))
    }

    @Test fun pinRejectsEmptyOrControlCharacters() {
        assertTrue(DnieInput.validPin("1234".toCharArray()))
        assertFalse(DnieInput.validPin(charArrayOf()))
        assertFalse(DnieInput.validPin("123\n4".toCharArray()))
        assertFalse(DnieInput.validPin(CharArray(17) { '1' }))
    }

    @Test fun mappedErrorsPreferRemovedCardAndBlockedPin() {
        assertEquals(DnieError.CAN, DnieErrors.from(PaceException()))
        assertEquals(DnieError.REMOVED, DnieErrors.from(PaceException(TagLostException())))
        assertEquals(DnieError.PIN, DnieErrors.from(PinException(), 2))
        assertEquals(DnieError.BLOCKED, DnieErrors.from(PinException(), 0))
        assertEquals(DnieError.EXPIRED, DnieErrors.from(CertificateExpiredException()))
    }

    private class PaceException(cause: Throwable? = null) : Exception(cause)
    private class TagLostException : Exception()
    private class PinException : Exception()
    private class CertificateExpiredException : Exception()
}
