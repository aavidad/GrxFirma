// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import io.github.aavidad.grxfirma.android.core.UnsafeExceptionState
import java.io.File
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Lote 39d: lo que dice el diálogo de consentimiento y con qué botón. */
class UnsafeExceptionDialogContentTest {
    private val main: File = listOf(File("src/main"), File("app/src/main")).first { it.isDirectory }
    private val item = UnsafeExceptionState("sha1_leer", fixed = false, activable = true, acceptanceVersion = "core.version.2")

    @Test fun `with acceptance the risk and how to turn it off are said once`() {
        // El texto de aceptación ya nombra el riesgo y cómo desactivarlo.
        assertEquals(listOf("excepciones.sha1_leer.aceptacion"), UnsafeExceptionDialog.bodyKeys(item))
    }

    @Test fun `without acceptance the dialog explains the risk and why it cannot be enabled`() {
        assertEquals(listOf("excepciones.sha1_leer.riesgo", "excepciones.sha1_leer.organizacion"),
            UnsafeExceptionDialog.bodyKeys(item.copy(activable = false, fixed = true)))
        assertEquals(listOf("excepciones.sha1_leer.riesgo", "excepciones.sha1_leer.organizacion"),
            UnsafeExceptionDialog.bodyKeys(item.copy(policyUnreadable = true)))
        assertEquals(listOf("excepciones.sha1_leer.riesgo", "android.inseguro.version_no_disponible"),
            UnsafeExceptionDialog.bodyKeys(item.copy(acceptanceVersion = "")))
    }

    @Test fun `only a blocked operation offers to continue`() {
        assertEquals("android.inseguro.activar_continuar", UnsafeExceptionDialog.activateKey(fromBlock = true))
        assertEquals("android.inseguro.activar", UnsafeExceptionDialog.activateKey(fromBlock = false))
        assertTrue(EngineKeys.isClosed(UnsafeExceptionDialog.activateKey(false)))
    }

    @Test fun `every language has exactly the Android keys the app uses`() {
        val folder = File(main, "assets/locales")
        listOf("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh").forEach { lang ->
            val json = JSONObject(File(folder, "$lang.json").readText())
            val keys = json.keys().asSequence().filter { it.startsWith("android.inseguro.") }.toSet()
            assertEquals("$lang: claves android.inseguro.*", UnsafeExceptionTexts.keys, keys)
            keys.forEach { assertTrue("$lang: $it vacía", json.getString(it).isNotBlank()) }
        }
    }

    @Test fun `the security section says that its changes apply at once`() {
        val es = JSONObject(File(main, "assets/locales/es.json").readText())
        val intro = es.getString("android.inseguro.introduccion")
        assertFalse("la introducción nombra un botón que ya no aparece desde Preferencias", intro.contains("Activar y continuar"))
        assertTrue(intro.contains("Guardar") && intro.contains("Cancelar"))
    }
}
