// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element

/**
 * Contratos de accesibilidad (WCAG 2.1 AA) que se comprueban sobre los
 * recursos, sin dispositivo: contraste de la paleta, foco visible, regiones
 * vivas y textos de ayuda acordes con lo que ofrece Android.
 */
class AccessibilityContractTest {
    private val main: File = listOf(File("src/main"), File("app/src/main")).first { it.isDirectory }
    private val res = File(main, "res")
    private val android = "http://schemas.android.com/apk/res/android"

    private fun xml(path: String): Element =
        DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
            .newDocumentBuilder().parse(File(res, path)).documentElement

    private fun elements(root: Element): Sequence<Element> = sequence {
        yield(root)
        val children = root.childNodes
        for (i in 0 until children.length) (children.item(i) as? Element)?.let { yieldAll(elements(it)) }
    }

    private fun byId(root: Element, id: String): Element =
        elements(root).single { it.getAttributeNS(android, "id") == "@+id/$id" }

    private fun colors(folder: String): Map<String, String> = elements(xml("$folder/colors.xml"))
        .filter { it.tagName == "color" }.associate { it.getAttribute("name") to it.textContent.trim() }

    private fun style(name: String): Map<String, String> = elements(xml("values/themes.xml"))
        .single { it.tagName == "style" && it.getAttribute("name") == name }
        .let { s -> elements(s).filter { it.tagName == "item" }.associate { it.getAttribute("name") to it.textContent.trim() } }

    private fun luminance(hex: String): Double {
        val value = hex.removePrefix("#").takeLast(6)
        return listOf(0, 2, 4).map { value.substring(it, it + 2).toInt(16) / 255.0 }
            .map { if (it <= 0.03928) it / 12.92 else Math.pow((it + 0.055) / 1.055, 2.4) }
            .let { (r, g, b) -> 0.2126 * r + 0.7152 * g + 0.0722 * b }
    }

    private fun contrast(a: String, b: String): Double {
        val (light, dark) = listOf(luminance(a), luminance(b)).sortedDescending()
        return (light + 0.05) / (dark + 0.05)
    }

    @Test fun `palette roles keep AA contrast in light and dark themes`() {
        for (folder in listOf("values", "values-night")) {
            val c = colors(folder) + colors(folder).let { night -> colors("values").filterKeys { it !in night } }
            fun check(fg: String, bg: String, minimum: Double) =
                assertTrue("$folder: $fg sobre $bg = ${contrast(c.getValue(fg), c.getValue(bg))}",
                    contrast(c.getValue(fg), c.getValue(bg)) >= minimum)
            check("on_primary", "primary", 4.5) // título e iconos de la barra superior
            check("on_surface", "surface", 4.5)
            check("on_surface_variant", "surface", 4.5)
            check("on_surface_variant", "surface_variant", 4.5)
            check("outline", "surface", 3.0)
            check("outline_variant", "surface", 3.0)
            check("inverse_on_surface", "inverse_surface", 4.5) // Snackbar
            check("inverse_primary", "inverse_surface", 4.5) // acción del Snackbar
        }
        val theme = style("Theme.GrxFirma")
        assertEquals("@color/on_surface_variant", theme["colorOnSurfaceVariant"])
        assertEquals("@color/outline_variant", theme["colorOutlineVariant"])
        assertEquals("@color/on_surface", theme["colorOnBackground"])
        assertEquals("@color/inverse_surface", theme["colorSurfaceInverse"])
        assertEquals("@color/inverse_on_surface", theme["colorOnSurfaceInverse"])
        assertEquals("@color/inverse_primary", theme["colorPrimaryInverse"])
    }

    @Test fun `toolbar icons use on_primary instead of a fixed dark overlay`() {
        val toolbar = byId(xml("layout/activity_main.xml"), "toolbar")
        assertEquals("@style/ThemeOverlay.GrxFirma.Toolbar", toolbar.getAttributeNS(android, "theme"))
        assertEquals("@color/on_primary", style("ThemeOverlay.GrxFirma.Toolbar")["colorControlNormal"])
    }

    @Test fun `keyboard focus ring is two dp and invisible without focus`() {
        val selector = elements(xml("color/focus_ring.xml")).filter { it.tagName == "item" }.toList()
        assertEquals("true", selector.first().getAttributeNS(android, "state_focused"))
        assertEquals("@color/on_surface", selector.first().getAttributeNS(android, "color"))
        assertEquals("@android:color/transparent", selector.last().getAttributeNS(android, "color"))
        assertEquals("", selector.last().getAttributeNS(android, "state_focused"))
        for (ring in listOf("drawable/focus_ring_pill.xml", "drawable/focus_ring_box.xml")) {
            val stroke = elements(xml(ring)).single { it.tagName == "stroke" }
            assertEquals("@dimen/focus_ring_width", stroke.getAttributeNS(android, "width"))
            assertEquals("@color/focus_ring", stroke.getAttributeNS(android, "color"))
            assertTrue(elements(xml(ring)).none { it.tagName == "solid" })
        }
        assertTrue(File(res, "values/dimens.xml").readText().contains("<dimen name=\"focus_ring_width\">2dp</dimen>"))
        val theme = style("Theme.GrxFirma")
        for (attribute in listOf("materialButtonStyle", "materialButtonOutlinedStyle", "borderlessButtonStyle",
                "checkboxStyle", "radioButtonStyle", "materialSwitchStyle")) {
            val name = theme.getValue(attribute).removePrefix("@style/")
            assertTrue("$name sin anillo de foco", style(name)["android:foreground"].orEmpty().startsWith("@drawable/focus_ring_"))
        }
        assertTrue(style("Widget.GrxFirma.HelpButton")["android:foreground"].orEmpty().startsWith("@drawable/focus_ring_"))
        // Un estilo Material fijo en un layout se saltaría el anillo del tema.
        File(res, "layout").listFiles()!!.forEach {
            assertFalse("${it.name} usa un botón Material sin anillo", it.readText().contains("@style/Widget.Material3.Button"))
        }
    }

    @Test fun `progress is announced and the result card is not an empty tab stop`() {
        val layout = xml("layout/activity_main.xml")
        assertEquals("polite", byId(layout, "progressText").getAttributeNS(android, "accessibilityLiveRegion"))
        assertEquals("", byId(layout, "resultCard").getAttributeNS(android, "focusable"))
    }

    @Test fun `seal pages help describes only the options Android offers`() {
        res.listFiles { f -> f.name.startsWith("values") }!!.map { File(it, "strings.xml") }.filter { it.isFile }.forEach {
            val text = elements(DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(it).documentElement)
                .single { e -> e.getAttribute("name") == "ayuda_sello_paginas" }.textContent
            assertFalse("${it.parentFile.name}: ayuda con rangos que Android no admite", text.contains("1,3-5"))
            assertTrue("${it.parentFile.name}: ayuda sin el límite de páginas", text.contains("128"))
        }
    }

    @Test fun `ENI validation messages address the person formally`() {
        val es = JSONObject(File(main, "assets/locales/es.json").readText())
        val informal = Regex("^(Indica|Usa|Elige|Escribe|Introduce|Selecciona) ")
        es.keys().asSequence().filter { it.startsWith("eni.validacion.") }.forEach {
            assertFalse("$it tutea: ${es.getString(it)}", informal.containsMatchIn(es.getString(it)))
        }
    }

    @Test fun `no deprecated announcements remain in the app`() {
        File(main, "java").walkTopDown().filter { it.extension == "kt" }.forEach {
            assertFalse("${it.name} usa announceForAccessibility", it.readText().contains("announceForAccessibility"))
        }
    }

    @Test fun `verification checks next to their help button show their result`() {
        val layout = xml("layout/activity_main.xml")
        val activity = File(main, "java/io/github/aavidad/grxfirma/android/MainActivity.kt").readText()
        listOf("verificationIntegrityLabel", "verificationTrustLabel", "verificationCoverageLabel").forEach { id ->
            byId(layout, id)
            assertTrue("$id no recibe el resultado de la comprobación", activity.contains(id))
        }
        assertTrue(activity.contains("R.string.verification_aspect_value"))
    }

    @Test fun `check boxes built in code use the themed Material class with its focus ring`() {
        // Un CheckBox de Android creado con su constructor no toma el estilo del tema
        // (checkboxStyle) y se queda sin el anillo de foco de 2 dp.
        val plain = Regex("""(?<![A-Za-z])(CheckBox|RadioButton)\(""")
        File(main, "java").walkTopDown().filter { it.extension == "kt" }.forEach {
            assertFalse("${it.name} crea un CheckBox o RadioButton sin estilo", plain.containsMatchIn(it.readText()))
        }
    }
}
