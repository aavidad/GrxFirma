// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.os.Build
import android.view.ContextThemeWrapper
import android.view.KeyEvent
import android.view.View
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.view.ViewCompat
import androidx.test.core.app.ActivityScenario
import com.google.android.material.appbar.MaterialToolbar
import com.google.android.material.button.MaterialButton
import com.google.android.material.checkbox.MaterialCheckBox
import com.google.android.material.textfield.TextInputEditText
import com.google.android.material.textfield.TextInputLayout
import io.github.aavidad.grxfirma.android.core.CoreReadiness
import io.github.aavidad.grxfirma.android.model.CertificateSummary
import io.github.aavidad.grxfirma.android.seal.SealAdjustment
import io.github.aavidad.grxfirma.android.seal.SealCanvasView
import io.github.aavidad.grxfirma.android.seal.SealRect
import io.github.aavidad.grxfirma.android.seal.SealSettings
import io.github.aavidad.grxfirma.android.ui.FieldErrors
import io.github.aavidad.grxfirma.android.ui.IdentityPanel
import io.github.aavidad.grxfirma.android.ui.MainUiState
import io.github.aavidad.grxfirma.android.ui.SessionIdentity
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Hallazgos de la auditoría WCAG 2.1 AA de Android, comprobados en el dispositivo. */
class AccessibilityUiTest {
    @Test fun sealCanvasStatesPositionAndAnswersKeysAndActions() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val canvas = SealCanvasView(activity)
                val steps = mutableListOf<SealAdjustment>()
                canvas.onAdjust = { steps += it }
                canvas.pageCount = 3
                canvas.pageNumber = 2
                canvas.settings = SealSettings(enabled = true, rect = SealRect(0.10f, 0.05f, 0.30f, 0.12f), rotation = 15)
                val expected = activity.getString(R.string.seal_canvas_state, 2, 3, 10, 5, 30, 12, 15)
                assertEquals(expected, canvas.stateText())
                assertEquals(expected, ViewCompat.getStateDescription(canvas)?.toString())
                assertEquals(activity.getString(R.string.seal_canvas_description), canvas.contentDescription)

                assertTrue(canvas.onKeyDown(KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_DPAD_LEFT)))
                assertTrue(canvas.onKeyDown(KeyEvent.KEYCODE_PLUS, KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_PLUS)))
                assertFalse(canvas.onKeyDown(KeyEvent.KEYCODE_TAB, KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_TAB)))
                assertEquals(listOf(SealAdjustment.LEFT, SealAdjustment.LARGER), steps)

                val labels = canvas.createAccessibilityNodeInfo().actionList.mapNotNull { it.label?.toString() }
                SealAdjustment.entries.forEach { assertTrue(labels.contains(activity.getString(it.label))) }
                assertNotNull("el lienzo debe mostrar el foco del teclado", canvas.foreground)
            }
        }
    }

    /** El icono «⋮» de la barra contrasta con el verde primario en tema claro y oscuro (WCAG 1.4.11). */
    @Test fun toolbarOverflowIconContrastsInLightAndDarkThemes() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                for (night in listOf(Configuration.UI_MODE_NIGHT_NO, Configuration.UI_MODE_NIGHT_YES)) {
                    val configuration = Configuration(activity.resources.configuration).apply {
                        uiMode = (uiMode and Configuration.UI_MODE_NIGHT_MASK.inv()) or night
                    }
                    val themed = ContextThemeWrapper(activity.createConfigurationContext(configuration), R.style.Theme_GrxFirma)
                    val toolbar = MaterialToolbar(ContextThemeWrapper(themed, R.style.ThemeOverlay_GrxFirma_Toolbar))
                    toolbar.inflateMenu(R.menu.main_menu)
                    val icon = toolbar.overflowIcon!!
                    val size = 48
                    val bitmap = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
                    icon.setBounds(0, 0, size, size)
                    icon.draw(Canvas(bitmap))
                    val ink = (0 until size * size).map { bitmap.getPixel(it % size, it / size) }.filter { Color.alpha(it) > 200 }
                    assertTrue("el icono no se ha dibujado", ink.isNotEmpty())
                    val primary = themed.getColor(R.color.primary)
                    ink.forEach { assertTrue("contraste ${contrast(it, primary)} en modo $night", contrast(it, primary) >= 3.0) }
                }
            }
        }
    }

    private fun inkedPixels(drawable: android.graphics.drawable.Drawable, state: IntArray): Int {
        val bitmap = Bitmap.createBitmap(120, 60, Bitmap.Config.ARGB_8888)
        drawable.state = state
        drawable.setBounds(0, 0, bitmap.width, bitmap.height)
        drawable.draw(Canvas(bitmap))
        return (0 until bitmap.width * bitmap.height).count { Color.alpha(bitmap.getPixel(it % bitmap.width, it / bitmap.width)) > 0 }
    }

    private fun contrast(a: Int, b: Int): Double {
        fun luminance(color: Int) = listOf(Color.red(color), Color.green(color), Color.blue(color))
            .map { it / 255.0 }.map { if (it <= 0.03928) it / 12.92 else Math.pow((it + 0.055) / 1.055, 2.4) }
            .let { (r, g, bl) -> 0.2126 * r + 0.7152 * g + 0.0722 * bl }
        val (light, dark) = listOf(luminance(a), luminance(b)).sortedDescending()
        return (light + 0.05) / (dark + 0.05)
    }

    @Test fun buttonsAndCheckboxesDrawAFocusRingOnlyWithFocus() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                listOf(
                    MaterialButton(activity),
                    MaterialButton(activity, null, com.google.android.material.R.attr.materialButtonOutlinedStyle),
                    activity.findViewById<View>(R.id.selectDocumentButton),
                    activity.findViewById<View>(R.id.signatureActionHelp),
                    MaterialCheckBox(activity),
                ).forEach { view ->
                    assertNotNull("${view.javaClass.simpleName} sin anillo de foco", view.foreground)
                    val ring = view.foreground!!.constantState!!.newDrawable(view.resources, view.context.theme).mutate()
                    assertTrue(ring.isStateful)
                    assertEquals("sin foco no cambia el aspecto", 0, inkedPixels(ring, intArrayOf(android.R.attr.state_enabled)))
                    assertTrue("con foco se ve el anillo",
                        inkedPixels(ring, intArrayOf(android.R.attr.state_enabled, android.R.attr.state_focused)) > 0)
                }
            }
        }
    }

    @Test fun headingsLiveRegionsAndFocusStopsMatchTheirRole() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    assertTrue(activity.findViewById<View>(R.id.backendStatusTitle).isAccessibilityHeading)
                    assertTrue(activity.findViewById<View>(R.id.qrTitle).isAccessibilityHeading)
                }
                assertEquals(View.ACCESSIBILITY_LIVE_REGION_POLITE,
                    activity.findViewById<TextView>(R.id.progressText).accessibilityLiveRegion)
                assertFalse(activity.findViewById<View>(R.id.resultCard).isFocusable)
            }
        }
    }

    @Test fun firstFieldWithAnErrorReceivesFocus() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val column = activity.findViewById<LinearLayout>(R.id.contentColumn)
                val fields = List(3) {
                    TextInputLayout(activity).also { layout ->
                        layout.addView(TextInputEditText(layout.context))
                        column.addView(layout)
                    }
                }
                assertFalse(FieldErrors.focusFirst(fields))
                fields[1].error = "x"
                fields[2].error = "y"
                assertTrue(FieldErrors.focusFirst(fields))
                assertTrue(fields[1].editText!!.isFocused)
                fields.forEach(column::removeView)
            }
        }
    }

    @Test fun openCertificatesAreASingleChoiceCollection() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val identities = listOf("a", "b").map { SessionIdentity(CertificateSummary(it, "Persona $it", "CA", "ff"), false) }
                val state = MainUiState(CoreReadiness(true, "ready", ""), identities = identities,
                    certificate = identities[1].certificate)
                val list = LinearLayout(activity)
                IdentityPanel.render(activity, state, View(activity), TextView(activity), list, {}, {})
                val collection = list.createAccessibilityNodeInfo().collectionInfo
                assertNotNull(collection)
                assertEquals(2, collection!!.rowCount)
                assertEquals(android.view.accessibility.AccessibilityNodeInfo.CollectionInfo.SELECTION_MODE_SINGLE,
                    collection.selectionMode)
                val radios = (0 until list.childCount).map { list.getChildAt(it) }.filterIsInstance<LinearLayout>()
                    .map { it.getChildAt(0) }
                val items = radios.map { it.createAccessibilityNodeInfo().collectionItemInfo }
                assertEquals(listOf(0, 1), items.map { it!!.rowIndex })
                assertEquals(listOf(false, true), items.map { it!!.isSelected })
                assertNull(list.contentDescription)
            }
        }
    }
}
