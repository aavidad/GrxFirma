// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.ui

import java.net.URI

object SigningOptions {
    fun create(profile: String, enabled: Boolean, url: String): Map<String, String> {
        require(profile in listOf("baseline", "t", "lt", "lta"))
        require(profile == "baseline" || enabled)
        return buildMap {
            put("profile", profile)
            if (enabled) {
                val endpoint = URI(url)
                require(url.length <= 2048 && url == url.trim() && url.none { it.isISOControl() } &&
                    endpoint.scheme in listOf("http", "https") && !endpoint.host.isNullOrBlank() &&
                    endpoint.rawUserInfo == null && endpoint.rawFragment == null && !endpoint.isOpaque &&
                    (endpoint.port == -1 || endpoint.port in 1..65535))
                put("tsaURL", endpoint.toASCIIString())
            }
        }
    }

    fun supported(format: String, action: String, profile: String): Boolean =
        FormatPolicy.supported(format, action, profile)
}
