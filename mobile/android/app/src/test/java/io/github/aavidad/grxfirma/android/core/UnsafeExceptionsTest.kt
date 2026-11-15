// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.core

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class UnsafeExceptionsTest {
    private fun state(id: String) = JSONObject().put("id", id).put("enabled", false)
        .put("fixed", false).put("activable", true).put("acceptance_version", "2026-11-15.1")
    private fun block(id: String = "cms_cbc", activable: Boolean = true) = JSONObject()
        .put("exceptionId", id).put("activable", activable)
        .put("blockKey", "excepciones.$id.bloqueo").put("settingKey", "excepciones.$id.ajuste")

    @Test fun `reads minimal typed error and optional metadata without requiring a new core`() {
        val minimal = UnsafeExceptions.parseBlock(block().toString())!!
        assertEquals("cms_cbc", minimal.id)
        assertTrue(minimal.activable)
        assertFalse(minimal.canActivate) // Se consulta la versión vigente, nunca se inventa.
        val full = block().put("acceptanceVersion", "next.2").put("future", JSONObject())
        assertEquals("next.2", UnsafeExceptions.parseBlock(full.toString())!!.acceptanceVersion)
        assertFalse(UnsafeExceptions.parseBlock(block(activable = false).toString())!!.activable)
        assertNotNull(UnsafeExceptions.parseBlock(JSONObject().put("error", full).toString()))
        assertNotNull(UnsafeExceptions.parseBlock(JSONObject().put("error", JSONObject().put("unsafeException", full)).toString()))
        assertNotNull(UnsafeExceptions.block(CoreContractException("generic", Exception(full.toString()))))
    }

    @Test fun `accepts snake case and rejects arbitrary ids keys and malformed permissions`() {
        val snake = JSONObject().put("id", "sha1_leer").put("activable", false)
            .put("block_key", "excepciones.sha1_leer.bloqueo").put("setting_key", "excepciones.sha1_leer.ajuste")
        assertEquals("sha1_leer", UnsafeExceptions.parseBlock(snake.toString())!!.id)
        listOf("", "no JSON", "{}", "[]", block("not_a_case").toString(),
            block().put("blockKey", "excepciones.sha1_leer.bloqueo").toString(),
            block().put("settingKey", "arbitrary").toString(),
            block().put("activable", "true").toString(),
            block().apply { remove("activable") }.toString(),
            "x".repeat(65537), "error [cms_cbc; excepciones.cms_cbc.bloqueo]",
        ).forEach { assertNull(it.take(100), UnsafeExceptions.parseBlock(it)) }
        assertNull(UnsafeExceptions.parseBlock(null))
    }

    @Test fun `all 26 cases reflect user machine and unreadable policy states`() {
        assertEquals(26, UnsafeExceptions.ids.size)
        UnsafeExceptions.ids.forEach { id ->
            val off = UnsafeExceptions.parseState(state(id))
            assertTrue(off.canActivate)
            assertFalse(off.canDisable)
            assertEquals("android.inseguro.desactivada", off.statusKey)
            val on = UnsafeExceptions.parseState(state(id).put("enabled", true))
            assertTrue(on.canDisable)
            assertEquals("android.inseguro.activada", on.statusKey)
            val denied = UnsafeExceptions.parseState(state(id).put("fixed", true).put("activable", false))
            assertFalse(denied.canActivate)
            assertFalse(denied.canDisable)
            assertEquals("android.inseguro.fijada_inactiva", denied.statusKey)
            val permitted = UnsafeExceptions.parseState(state(id).put("fixed", true).put("enabled", true))
            assertFalse(permitted.canDisable)
            assertFalse(permitted.canActivate)
            assertEquals("android.inseguro.fijada_activa", permitted.statusKey)
            val unreadable = UnsafeExceptions.parseState(state(id).put("policy_unreadable", true))
            assertFalse(unreadable.canActivate)
            assertFalse(unreadable.canDisable)
            assertEquals("android.inseguro.directiva_ilegible", unreadable.statusKey)
            assertFalse(UnsafeExceptions.parseState(JSONObject().put("id", id)).canActivate)
        }
    }

    @Test fun `activation sends explicit acceptance and the exact displayed version`() {
        val item = UnsafeExceptions.parseState(state("tsa_http"))
        val request = JSONObject(UnsafeExceptions.activationRequest(item, true, UnsafeAcceptanceOrigin.FROM_PREFERENCES))
        assertEquals(4, request.length())
        assertTrue(request.getBoolean("accepted"))
        assertEquals(item.id, request.getString("id"))
        assertEquals(item.acceptanceVersion, request.getString("acceptanceVersion"))
        // Lote 39f: el recibo distingue Preferencias del aviso de un bloqueo.
        assertEquals("ajustes", request.getString("origin"))
        assertEquals("aviso", JSONObject(UnsafeExceptions.activationRequest(item, true, UnsafeAcceptanceOrigin.FROM_BLOCK)).getString("origin"))
        assertThrows(IllegalArgumentException::class.java) { UnsafeExceptions.activationRequest(item, false, UnsafeAcceptanceOrigin.FROM_BLOCK) }
        assertThrows(IllegalArgumentException::class.java) { UnsafeExceptions.activationRequest(item.copy(activable = false), true, UnsafeAcceptanceOrigin.FROM_BLOCK) }
        assertThrows(IllegalArgumentException::class.java) { UnsafeExceptions.activationRequest(item.copy(acceptanceVersion = ""), true, UnsafeAcceptanceOrigin.FROM_BLOCK) }
        val disable = JSONObject(UnsafeExceptions.disableRequest(item.copy(enabled = true)))
        assertFalse(disable.getBoolean("enabled"))
        assertEquals(2, disable.length())
        assertThrows(IllegalArgumentException::class.java) { UnsafeExceptions.disableRequest(item.copy(enabled = true, fixed = true)) }
    }

    @Test fun `catalogue must contain every case exactly once and retains core order`() {
        val states = UnsafeExceptions.ids.reversed().map(::state)
        val parsed = UnsafeExceptions.parseList(JSONArray(states).toString())
        assertEquals(UnsafeExceptions.ids, parsed.map { it.id })
        assertThrows(CoreContractException::class.java) { UnsafeExceptions.parseList(JSONArray(states.drop(1)).toString()) }
        assertThrows(CoreContractException::class.java) { UnsafeExceptions.parseList(JSONArray(states + states.first()).toString()) }
    }

    /** Mensaje exacto que deja la fachada Go (mobilebind.bloqueoTipado) al verificar con SHA-1 bloqueado. */
    @Test fun `reads the typed block that the Go facade returns for a blocked verification`() {
        val fromGo = """{"errorCode":"unsafe_exception_blocked","exceptionId":"sha1_leer","activable":true,""" +
            """"blockKey":"excepciones.sha1_leer.bloqueo","settingKey":"excepciones.sha1_leer.ajuste",""" +
            """"titleKey":"excepciones.sha1_leer.titulo","riskKey":"excepciones.sha1_leer.riesgo",""" +
            """"acceptanceKey":"excepciones.sha1_leer.aceptacion","helpKey":"excepciones.sha1_leer.ayuda",""" +
            """"organizationKey":"excepciones.sha1_leer.organizacion","acceptanceVersion":"2026-11-15.3"}"""
        val parsed = UnsafeExceptions.block(RuntimeException(fromGo))!!
        assertEquals("sha1_leer", parsed.id)
        assertTrue(parsed.activable)
        assertEquals("2026-11-15.3", parsed.acceptanceVersion)
        // Un nombre de certificado con la marca antigua no es un bloqueo.
        assertNull(UnsafeExceptions.block(RuntimeException("CN=Ana [sha1_leer; excepciones.sha1_leer.bloqueo]")))
    }
}
