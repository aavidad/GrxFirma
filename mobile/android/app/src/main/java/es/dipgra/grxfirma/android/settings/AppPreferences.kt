// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android.settings

import android.content.Context
import androidx.appcompat.app.AppCompatDelegate
import androidx.core.content.edit
import java.util.Locale

/**
 * Preferencias de la persona. Nada de esto es secreto: formato, perfil, TSA
 * (sin credenciales, validada al firmar), nombre de salida y tema. Nunca se
 * guardan PIN, contraseñas, claves, certificados ni documentos, por eso basta
 * SharedPreferences privadas sin cifrar y excluidas de las copias.
 */
data class AppSettings(
    val defaultFormat: String = "auto",
    val defaultProfile: String = "baseline",
    val tsaEnabled: Boolean = false,
    val tsaUrl: String = "",
    val outputName: String = OutputNames.SUFFIX,
    val theme: String = THEME_SYSTEM,
    /** Minutos en segundo plano tras los que se cierra el PKCS#12; 0 = no se cierra solo. */
    val sessionTimeoutMinutes: Int = DEFAULT_TIMEOUT,
    /** Idioma del sello visible: vacío sigue a la app; si no, uno de [SEAL_LANGUAGES]. */
    val sealLanguage: String = "",
) {
    /** Añade el idioma fijo del sello a unas opciones de firma con sello visible. */
    fun withSealLanguage(options: Map<String, String>): Map<String, String> =
        if (sealLanguage.isEmpty() || options["visibleSeal"] != "true") options
        else options + (SEAL_LANGUAGE_OPTION to sealLanguage)

    /** Corrige valores fuera de rango (preferencias manipuladas o antiguas). */
    fun sanitized(): AppSettings = copy(
        defaultFormat = defaultFormat.takeIf { it in FORMATS } ?: "auto",
        defaultProfile = defaultProfile.takeIf { it in PROFILES } ?: "baseline",
        tsaUrl = tsaUrl.take(MAX_TSA_URL).filterNot(es.dipgra.grxfirma.android.core.DisplayText::hidden),
        outputName = outputName.takeIf { it in OutputNames.POLICIES } ?: OutputNames.SUFFIX,
        theme = theme.takeIf { it in THEMES } ?: THEME_SYSTEM,
        sessionTimeoutMinutes = sessionTimeoutMinutes.takeIf { it in TIMEOUTS } ?: DEFAULT_TIMEOUT,
        sealLanguage = sealLanguage.takeIf { it in SEAL_LANGUAGES } ?: "",
    )

    companion object {
        const val THEME_SYSTEM = "system"
        const val THEME_LIGHT = "light"
        const val THEME_DARK = "dark"
        const val MAX_TSA_URL = 2048
        const val DEFAULT_TIMEOUT = 5
        val TIMEOUTS = listOf(1, 5, 15, 0)
        /** Opción del motor que fija el idioma del sello en una firma. */
        const val SEAL_LANGUAGE_OPTION = "sealLanguage"
        /** «» = como la app; después, los idiomas del catálogo del motor («va» = valenciano). */
        val SEAL_LANGUAGES = listOf("", "es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh")
        val THEMES = listOf(THEME_SYSTEM, THEME_LIGHT, THEME_DARK)
        val PROFILES = listOf("baseline", "t", "lt", "lta")
        val FORMATS = listOf("auto", "pades", "cades", "xades", "xmldsig", "odf", "ooxml", "facturae", "asic-xades", "verifactu")

        fun nightMode(theme: String): Int = when (theme) {
            THEME_LIGHT -> AppCompatDelegate.MODE_NIGHT_NO
            THEME_DARK -> AppCompatDelegate.MODE_NIGHT_YES
            else -> AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM
        }
    }
}

interface AppSettingsStore {
    fun load(): AppSettings
    fun save(settings: AppSettings)
    fun reset()
}

class AppPreferences(context: Context) : AppSettingsStore {
    private val preferences = context.applicationContext.getSharedPreferences(FILE, Context.MODE_PRIVATE)

    override fun load(): AppSettings = AppSettings(
        defaultFormat = preferences.getString("default_format", "auto").orEmpty(),
        defaultProfile = preferences.getString("default_profile", "baseline").orEmpty(),
        tsaEnabled = preferences.getBoolean("tsa_enabled", false),
        tsaUrl = preferences.getString("tsa_url", "").orEmpty(),
        outputName = preferences.getString("output_name", OutputNames.SUFFIX).orEmpty(),
        theme = preferences.getString("theme", AppSettings.THEME_SYSTEM).orEmpty(),
        sessionTimeoutMinutes = preferences.getInt("session_timeout_minutes", AppSettings.DEFAULT_TIMEOUT),
        sealLanguage = preferences.getString("seal_language", "").orEmpty(),
    ).sanitized()

    override fun save(settings: AppSettings) {
        val clean = settings.sanitized()
        preferences.edit {
            putString("default_format", clean.defaultFormat)
            putString("default_profile", clean.defaultProfile)
            putBoolean("tsa_enabled", clean.tsaEnabled)
            putString("tsa_url", clean.tsaUrl)
            putString("output_name", clean.outputName)
            putString("theme", clean.theme)
            putInt("session_timeout_minutes", clean.sessionTimeoutMinutes)
            putString("seal_language", clean.sealLanguage)
        }
    }

    override fun reset() {
        preferences.edit { clear() }
    }

    companion object {
        const val FILE = "app_preferences"

        /** Aplica el tema guardado antes de crear la actividad. */
        fun applyTheme(context: Context) {
            AppCompatDelegate.setDefaultNightMode(AppSettings.nightMode(AppPreferences(context).load().theme))
        }
    }
}

/** Política del nombre que se propone al guardar una firma. */
object OutputNames {
    const val SUFFIX = "suffix"
    const val DESKTOP = "desktop"
    const val ORIGINAL = "original"
    val POLICIES = listOf(SUFFIX, DESKTOP, ORIGINAL)

    /**
     * [suffixPattern] y [desktopPattern] son los recursos traducidos
     * («%1$s-firmado.%2$s» y «%1$s_firmado.%2$s»).
     */
    fun name(policy: String, base: String, extension: String, suffixPattern: String, desktopPattern: String): String {
        val pattern = when (policy) {
            DESKTOP -> desktopPattern
            ORIGINAL -> return "$base.$extension"
            else -> suffixPattern
        }
        // Un documento ya firmado no acaba en «-firmado-firmado» al cofirmarlo.
        val marker = String.format(Locale.ROOT, pattern, "", extension).removeSuffix(".$extension")
        if (marker.isNotEmpty() && base.endsWith(marker, ignoreCase = true)) return "$base.$extension"
        return String.format(Locale.ROOT, pattern, base, extension)
    }
}

/** Almacén en memoria para pruebas y para el modelo sin contexto. */
class InMemorySettingsStore(private var current: AppSettings = AppSettings()) : AppSettingsStore {
    var resets = 0
        private set

    override fun load(): AppSettings = current

    override fun save(settings: AppSettings) {
        current = settings.sanitized()
    }

    override fun reset() {
        resets++
        current = AppSettings()
    }
}
