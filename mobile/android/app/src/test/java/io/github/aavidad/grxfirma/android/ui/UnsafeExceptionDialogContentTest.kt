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
import org.junit.Assert.assertNull
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

    /** Vuelta 2: «Activada» y la fecha decían lo mismo en dos líneas. */
    @Test fun `an enabled case shows one status line with its date`() {
        val on = item.copy(enabled = true, acceptedAt = "2026-11-15T17:06:00Z")
        assertEquals("android.inseguro.aceptada_fecha", UnsafeExceptionsPanel.statusKey(on))
        assertEquals("android.inseguro.activada", UnsafeExceptionsPanel.statusKey(on.copy(acceptedAt = "")))
        assertEquals("android.inseguro.fijada_activa", UnsafeExceptionsPanel.statusKey(on.copy(fixed = true)))
        assertEquals("android.inseguro.desactivada", UnsafeExceptionsPanel.statusKey(item))
    }

    /** Vuelta 2: al activar o desactivar solo se rehace la fila que cambia, y el foco no se pierde. */
    @Test fun `only the changed case is rebuilt`() {
        val cases = listOf(item, item.copy(id = "cms_cbc"))
        assertEquals(setOf("sha1_leer"), UnsafeExceptionsPanel.changedIds(cases, listOf(cases[0].copy(enabled = true), cases[1])))
        assertEquals(emptySet<String>(), UnsafeExceptionsPanel.changedIds(cases, cases))
        assertNull(UnsafeExceptionsPanel.changedIds(null, cases))
        assertNull(UnsafeExceptionsPanel.changedIds(cases, cases.reversed()))
    }

    /** Vuelta 2: la ayuda va por apartados, cada uno con su encabezado para TalkBack. */
    @Test fun `help is split into sections with headings in every script`() {
        val es = UnsafeExceptionDialog.helpSections("Qué hace este ajuste: mientras está desactivado, GrxFirma detiene esto.\n\nAlternativa: pida otra copia.")
        assertEquals(listOf("Qué hace este ajuste" to "Mientras está desactivado, GrxFirma detiene esto.", "Alternativa" to "Pida otra copia."), es)
        assertEquals("Ce que fait ce réglage" to "Tant qu’il est désactivé.", UnsafeExceptionDialog.helpSections("Ce que fait ce réglage : tant qu’il est désactivé.").single())
        assertEquals("此设置的作用" to "停用期间。", UnsafeExceptionDialog.helpSections("此设置的作用：停用期间。").single())
        assertEquals("" to "Sin encabezado.", UnsafeExceptionDialog.helpSections("Sin encabezado.").single())
        val catalogue = JSONObject(File(main, "assets/locales/es.json").readText())
        assertEquals(7, UnsafeExceptionDialog.helpSections(catalogue.getString("excepciones.sha1_leer.ayuda")).count { it.first.isNotEmpty() })
    }

    /** Vuelta 2: la fecha dice lo mismo que en escritorio y la casilla no repite la responsabilidad. */
    @Test fun `date and checkbox texts in every language`() {
        val responsibility = listOf("responsab", "Verantwortung", "erantzukizun", "责任")
        listOf("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh").forEach { lang ->
            val json = JSONObject(File(main, "assets/locales/$lang.json").readText())
            val date = json.getString("android.inseguro.aceptada_fecha")
            assertTrue("$lang: fecha sin {date}: $date", date.contains("{date}") && !date.contains("{fecha}"))
            val box = json.getString("android.inseguro.aceptacion_casilla")
            responsibility.forEach { assertFalse("$lang: la casilla repite la responsabilidad: $box", box.contains(it, ignoreCase = true)) }
        }
        val es = JSONObject(File(main, "assets/locales/es.json").readText())
        assertEquals("Activado el {date}", es.getString("android.inseguro.aceptada_fecha"))
        assertEquals("He leído el aviso y acepto el riesgo.", es.getString("android.inseguro.aceptacion_casilla"))
    }
}
