// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import android.content.res.Configuration
import android.graphics.Rect
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.scrollTo
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.ViewMatchers.*
import androidx.test.espresso.matcher.RootMatchers.isDialog
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import io.github.aavidad.grxfirma.android.core.UnsafeExceptionState
import io.github.aavidad.grxfirma.android.core.UnsafeExceptions
import io.github.aavidad.grxfirma.android.databinding.DialogPreferencesBinding
import io.github.aavidad.grxfirma.android.ui.MainUiState
import io.github.aavidad.grxfirma.android.ui.SafeWindowInsets
import io.github.aavidad.grxfirma.android.ui.UnsafeExceptionDialog
import io.github.aavidad.grxfirma.android.ui.UnsafeExceptionsPanel
import io.github.aavidad.grxfirma.android.ui.UnsafeExceptionTexts
import io.github.aavidad.grxfirma.android.ui.UiText
import io.github.aavidad.grxfirma.android.ui.resolve
import org.hamcrest.Matchers.not
import org.junit.Assert.*
import org.junit.Test

class UnsafeExceptionsUiTest : AccessibilityTest() {
    private val item = UnsafeExceptionState("cms_cbc", activable = true, fixed = false,
        acceptanceVersion = "2026-11-15.1")

    @Test fun consentStartsUncheckedAndOnlyEnablesActivationAfterAcceptance() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            var accepted = false
            var cancelled = false
            lateinit var dialog: UnsafeExceptionDialog
            scenario.onActivity {
                dialog = UnsafeExceptionDialog(it, item, true, { value -> accepted = value }, { cancelled = true; dialog.close() })
                dialog.show()
            }
            assertConsentWindowHasFocus()
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).check(matches(not(isChecked()))).perform(click())
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(isEnabled())).perform(click())
            assertTrue(accepted)
            scenario.onActivity { dialog.render(true, null) }
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).check(matches(not(isEnabled())))
            scenario.onActivity { dialog.render(false, null) }
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).perform(click())
            assertTrue(cancelled)
            scenario.onActivity {
                dialog = UnsafeExceptionDialog(it, item, true, {}, {})
                dialog.show()
            }
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).check(matches(not(isChecked())))
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
        }
    }

    @Test fun organizationBlockHasNoAcceptanceAndStillOffersScrollableHelp() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                UnsafeExceptionDialog(it, item.copy(activable = false, fixed = true), true,
                    { fail("Una directiva no permite activar") }, {}).show()
            }
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isDisplayed())))
            // Sin activación posible la casilla no se añade al diálogo: ni se ve ni la lee TalkBack.
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).check(androidx.test.espresso.assertion.ViewAssertions.doesNotExist())
            onView(withId(R.id.unsafeMore)).inRoot(isDialog()).perform(click())
            onView(withId(R.id.unsafeHelpText)).inRoot(isDialog()).check(matches(isDisplayed()))
            onView(withId(R.id.unsafeHelpText)).inRoot(isDialog()).check(matches(withText(org.hamcrest.Matchers.containsString("CMS"))))
        }
    }

    @Test fun preferencesEntryExistsInTheActivityAndExplainsAnUnavailableCore() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            TestMenus.open(R.id.action_preferences, R.string.preferences_title)
            lateinit var title: String
            scenario.onActivity { title = UnsafeExceptionTexts.text(it, "seccion") }
            onView(withText(title)).inRoot(isDialog()).perform(scrollTo()).check(matches(isDisplayed()))
        }
    }

    @Test fun preferencesShowsAllCasesAndDirectDisableUsesTheSameAcceptanceDialog() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            var disabled = ""
            var activated = ""
            lateinit var binding: DialogPreferencesBinding
            lateinit var panel: UnsafeExceptionsPanel
            val cases = UnsafeExceptions.ids.map { id -> item.copy(id = id, enabled = id == "cms_cbc") }
            scenario.onActivity { activity ->
                binding = DialogPreferencesBinding.inflate(activity.layoutInflater)
                panel = UnsafeExceptionsPanel(binding.unsafePreferences, {}, { selected ->
                    activated = selected.id
                    UnsafeExceptionDialog(activity, selected, false, {}, {}).show()
                }, { selected -> disabled = selected.id })
                panel.render(MainUiState(backend = io.github.aavidad.grxfirma.android.core.CoreReadiness(true, "ready", ""),
                    unsafeExceptionsAvailable = true, unsafeExceptions = cases))
                MaterialAlertDialogBuilder(activity).setView(binding.root).create().also { dialog ->
                    dialog.show()
                    dialog.window!!.setLayout(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
                    SafeWindowInsets.fullScreenDialog(dialog.window!!)
                }
                cases.forEach { assertNotNull(binding.root.findViewWithTag<android.view.View>("unsafe-case-${it.id}")) }
            }
            onView(withTagValue(org.hamcrest.Matchers.`is`("unsafe-change-cms_cbc" as Any))).inRoot(isDialog()).perform(scrollTo(), click())
            assertEquals("cms_cbc", disabled)
            onView(withTagValue(org.hamcrest.Matchers.`is`("unsafe-change-sha1_leer" as Any))).inRoot(isDialog()).perform(scrollTo(), click())
            assertEquals("sha1_leer", activated)
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
        }
    }

    /** Ejecutar con gestos y tres botones: se usan los insets reales de cada navegación. */
    @Test fun consentRemainsScrollableAtTwoHundredPercentTextSize() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            var accepted = false
            var cancelled = false
            lateinit var expectedStatement: String
            lateinit var expectedCheckbox: String
            lateinit var consent: UnsafeExceptionDialog
            scenario.onActivity { activity ->
                val configuration = Configuration(activity.resources.configuration).apply { fontScale = 2f }
                val context = android.view.ContextThemeWrapper(activity, R.style.Theme_GrxFirma).apply {
                    applyOverrideConfiguration(configuration)
                }
                expectedStatement = UiText.Engine(item.key("aceptacion")).resolve(context)
                expectedCheckbox = UnsafeExceptionTexts.text(context, "aceptacion_casilla")
                consent = UnsafeExceptionDialog(context, item, true, { accepted = it }, { cancelled = true; consent.close() })
                consent.show()
            }
            assertConsentWindowHasFocus()
            // El aviso permanece íntegro y asociado al control, aunque se lea por partes al desplazarlo.
            onView(withId(R.id.unsafeAcceptanceText)).inRoot(isDialog()).check(matches(withText(expectedStatement)))
                .check { view, error ->
                    if (error != null) throw error
                    val text = view as android.widget.TextView
                    assertEquals(R.id.unsafeAcceptance, text.labelFor)
                    assertNull("El aviso no se debe elidir", text.ellipsize)
                    assertEquals("El aviso no debe limitar sus líneas", Int.MAX_VALUE, text.maxLines)
                }
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog())
                .check(matches(withText(expectedCheckbox))).check(matches(isCompletelyDisplayed()))
                .check { view, error ->
                    if (error != null) throw error
                    assertControlInsideSafeArea(view)
                }.perform(click())
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog())
                .check(matches(isEnabled())).check(matches(isCompletelyDisplayed()))
                .check { view, error ->
                    if (error != null) throw error
                    assertControlInsideSafeArea(view)
                }.perform(click())
            assertTrue(accepted)
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog())
                .check(matches(isCompletelyDisplayed()))
                .check { view, error ->
                    if (error != null) throw error
                    assertControlInsideSafeArea(view)
                }.perform(click())
            assertTrue(cancelled)
        }
    }

    /** Lote 38: desde Preferencias nada se ha bloqueado; la ventana se anuncia con el título visible. */
    @Test fun titleDependsOnWhereTheDialogOpensAndNamesTheWindow() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            for (fromBlock in listOf(false, true)) {
                lateinit var expected: String
                lateinit var dialog: UnsafeExceptionDialog
                scenario.onActivity {
                    expected = UiText.Engine(UnsafeExceptionDialog.titleKey(item, fromBlock)).resolve(it)
                    dialog = UnsafeExceptionDialog(it, item, fromBlock, {}, {})
                    dialog.show()
                }
                onView(withText(expected)).inRoot(isDialog()).check(matches(isDisplayed()))
                // El título que anuncia TalkBack es el de la ventana en el árbol de
                // accesibilidad, no LayoutParams.title (el nombre interno de la actividad).
                assertTrue("Título accesible de la ventana: ${accessibleWindowTitles()}",
                    waitForWindowTitle(expected))
                // La versión del texto viaja al núcleo; no se enseña a la persona.
                onView(withText(org.hamcrest.Matchers.containsString(item.acceptanceVersion))).inRoot(isDialog())
                    .check(androidx.test.espresso.assertion.ViewAssertions.doesNotExist())
                scenario.onActivity { dialog.close() }
            }
        }
    }

    private fun accessibleWindowTitles(): List<String> {
        val automation = androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().uiAutomation
        automation.serviceInfo = automation.serviceInfo.apply {
            flags = flags or android.accessibilityservice.AccessibilityServiceInfo.FLAG_RETRIEVE_INTERACTIVE_WINDOWS
        }
        return automation.windows.mapNotNull { it.title?.toString() }
    }

    /** Las ventanas del árbol de accesibilidad se actualizan aparte; se esperan hasta 5 s. */
    private fun waitForWindowTitle(expected: String): Boolean {
        repeat(50) {
            if (expected in accessibleWindowTitles()) return true
            Thread.sleep(100)
        }
        return false
    }

    /** La principal va rellena y, deshabilitada, conserva su color; las secundarias, con borde o solo texto. */
    @Test fun primaryActionKeepsItsWeightAndSecondariesAreSeparate() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { UnsafeExceptionDialog(it, item, true, {}, {}).show() }
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check { view, error ->
                if (error != null) throw error
                val button = view as com.google.android.material.button.MaterialButton
                val primary = androidx.core.content.ContextCompat.getColor(view.context, R.color.primary)
                val disabled = button.backgroundTintList!!.getColorForState(intArrayOf(-android.R.attr.state_enabled), 0)
                assertFalse(button.isEnabled)
                assertEquals("Mismo tono que la principal", primary and 0xFFFFFF, disabled and 0xFFFFFF)
                assertTrue("Visible aunque esté deshabilitada", android.graphics.Color.alpha(disabled) > 0x40)
            }
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).check { view, error ->
                if (error != null) throw error
                val cancel = view as com.google.android.material.button.MaterialButton
                assertTrue("Cancelar lleva borde", cancel.strokeWidth > 0)
                val activate = view.rootView.findViewById<View>(R.id.unsafeActivate)
                val more = view.rootView.findViewById<com.google.android.material.button.MaterialButton>(R.id.unsafeMore)
                assertEquals("Más información va como texto", 0, more.strokeWidth)
                assertTrue("Las secundarias van por debajo y separadas", locationY(cancel) >= locationY(activate) + activate.height)
                assertFalse("Las secundarias no van rellenas", cancel.backgroundTintList?.defaultColor ==
                    (activate as com.google.android.material.button.MaterialButton).backgroundTintList?.defaultColor)
            }
        }
    }

    /** Con letra al 200 % el texto se desplaza y la casilla y los botones quedan fijos abajo. */
    @Test fun atTwoHundredPercentOnlyTheTextScrolls() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val configuration = Configuration(activity.resources.configuration).apply { fontScale = 2f }
                val context = android.view.ContextThemeWrapper(activity, R.style.Theme_GrxFirma).apply {
                    applyOverrideConfiguration(configuration)
                }
                UnsafeExceptionDialog(context, item, true, {}, {}).show()
            }
            onView(withId(R.id.unsafeScroll)).inRoot(isDialog()).check { view, error ->
                if (error != null) throw error
                assertTrue("Hay más texto debajo y se indica", view.canScrollVertically(1))
                assertTrue(view.isVerticalScrollBarEnabled)
                assertFalse("La barra no se oculta", view.isScrollbarFadingEnabled)
            }
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).check(matches(isCompletelyDisplayed()))
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(isCompletelyDisplayed()))
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).check(matches(isCompletelyDisplayed()))
        }
    }

    /** Con letra grande los «?» de Confianza y Cobertura no salen de su fila: el texto se ajusta. */
    @Test fun verificationHelpButtonsStayInsideTheirRowWithLargeText() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val group = activity.findViewById<ViewGroup>(R.id.verificationHelpGroup)
                group.visibility = View.VISIBLE
                listOf(R.id.verificationIntegrityLabel, R.id.verificationTrustLabel, R.id.verificationCoverageLabel).forEach { id ->
                    activity.findViewById<android.widget.TextView>(id).apply {
                        textSize = 28f // equivale a BodyMedium al 200 %
                        text = (text.toString() + " ").repeat(6)
                    }
                }
                group.requestLayout()
            }
            for (id in listOf(R.id.verificationIntegrityHelp, R.id.verificationTrustHelp, R.id.verificationCoverageHelp)) {
                onView(withId(id)).check { view, error ->
                    if (error != null) throw error
                    val row = view.parent as View
                    val density = view.resources.displayMetrics.density
                    assertTrue("El «?» conserva sus 48 dp", view.width >= (48 * density).toInt() - 1)
                    assertTrue("El «?» queda dentro de la fila", view.right <= row.width && view.left >= 0)
                    assertEquals(View.VISIBLE, view.visibility)
                }
            }
        }
    }

    private fun locationY(view: View): Int = IntArray(2).also { view.getLocationOnScreen(it) }[1]

    /** Las barras pueden tener alturas diferentes; ningún control debe quedar debajo de ellas. */
    private fun assertControlInsideSafeArea(view: View) {
        val decor = view.rootView
        val insets = ViewCompat.getRootWindowInsets(decor)
        assertNotNull("La ventana debe exponer sus insets", insets)
        val safe = insets!!.getInsets(WindowInsetsCompat.Type.systemBars() or
            WindowInsetsCompat.Type.ime() or WindowInsetsCompat.Type.displayCutout())
        val location = IntArray(2)
        decor.getLocationInWindow(location)
        val area = Rect(location[0] + safe.left, location[1] + safe.top,
            location[0] + decor.width - safe.right, location[1] + decor.height - safe.bottom)
        view.getLocationInWindow(location)
        val bounds = Rect(location[0], location[1], location[0] + view.width, location[1] + view.height)
        assertTrue("El control ${view.id} debe caber entero: caja=$bounds, área segura=$area", area.contains(bounds))
    }

    /** inRoot espera la ventana del diálogo; estas aserciones no permiten esconder una pérdida real de foco. */
    private fun assertConsentWindowHasFocus() {
        onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).check { view, error ->
            if (error != null) throw error
            val root = view.rootView
            assertTrue("La ventana del consentimiento debe tener foco para teclado y TalkBack", root.hasWindowFocus())
            val params = root.layoutParams as WindowManager.LayoutParams
            assertEquals("El diálogo debe aceptar foco de ventana", 0, params.flags and WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE)
        }
    }

}
