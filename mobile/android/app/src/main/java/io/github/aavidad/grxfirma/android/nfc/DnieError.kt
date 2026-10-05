package io.github.aavidad.grxfirma.android.nfc

internal enum class DnieError { NFC_MISSING, NFC_OFF, CAN, PIN, BLOCKED, REMOVED, EXPIRED, CERTIFICATE, LIBRARY, OTHER }

internal object DnieErrors {
    fun from(error: Throwable, retriesLeft: Int = -1): DnieError {
        if (retriesLeft == 0) return DnieError.BLOCKED
        var cause: Throwable? = error
        var pace = false
        var pin = false
        while (cause != null) {
            when (cause.javaClass.simpleName) {
                "TagLostException", "LostChannelException" -> return DnieError.REMOVED
                "InvalidCanOrMrzException", "InvalidAccessCodeException", "PaceException" -> pace = true
                "PinException", "BadPinException" -> pin = true
                "CertificateExpiredException" -> return DnieError.EXPIRED
                "BurnedDnieCardException" -> return DnieError.BLOCKED
            }
            cause = cause.cause
        }
        if (pin) return DnieError.PIN
        if (pace) return DnieError.CAN
        return when (error.message) {
            "NFC_MISSING" -> DnieError.NFC_MISSING
            "NFC_OFF" -> DnieError.NFC_OFF
            "JMULTICARD_ANDROID_HELPER_MISSING" -> DnieError.LIBRARY
            "SIGN_CERT_MISSING" -> DnieError.CERTIFICATE
            "NOT_ISODEP", "CARD_REMOVED" -> DnieError.REMOVED
            else -> DnieError.OTHER
        }
    }
}
