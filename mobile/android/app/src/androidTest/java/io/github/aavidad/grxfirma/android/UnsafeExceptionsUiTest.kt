// Derechos de autor (C) 2026 Alberto Avidad Fernández.
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
                dialog = UnsafeExceptionDialog(it, item, { value -> accepted = value }, { cancelled = true; dialog.close() })
                dialog.show()
            }
            assertConsentWindowHasFocus()
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).check(matches(not(isChecked()))).perform(scrollTo(), click())
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(isEnabled())).perform(scrollTo(), click())
            assertTrue(accepted)
            scenario.onActivity { dialog.render(true, null) }
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).check(matches(not(isEnabled())))
            scenario.onActivity { dialog.render(false, null) }
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).perform(scrollTo(), click())
            assertTrue(cancelled)
            scenario.onActivity {
                dialog = UnsafeExceptionDialog(it, item, {}, {})
                dialog.show()
            }
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).check(matches(not(isChecked())))
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isEnabled())))
        }
    }

    @Test fun organizationBlockHasNoAcceptanceAndStillOffersScrollableHelp() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                UnsafeExceptionDialog(it, item.copy(activable = false, fixed = true),
                    { fail("Una directiva no permite activar") }, {}).show()
            }
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).check(matches(not(isDisplayed())))
            // La casilla no se añade al árbol de accesibilidad si está prohibida.
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).check(androidx.test.espresso.assertion.ViewAssertions.doesNotExist())
            onView(withId(R.id.unsafeMore)).inRoot(isDialog()).perform(scrollTo(), click())
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
                    UnsafeExceptionDialog(activity, selected, {}, {}).show()
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
                consent = UnsafeExceptionDialog(context, item, { accepted = it }, { cancelled = true; consent.close() })
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
            onView(withId(R.id.unsafeAcceptance)).inRoot(isDialog()).perform(scrollTo())
                .check(matches(withText(expectedCheckbox))).check(matches(isCompletelyDisplayed()))
                .check { view, error ->
                    if (error != null) throw error
                    assertControlInsideSafeArea(view)
                }.perform(click())
            onView(withId(R.id.unsafeActivate)).inRoot(isDialog()).perform(scrollTo())
                .check(matches(isEnabled())).check(matches(isCompletelyDisplayed()))
                .check { view, error ->
                    if (error != null) throw error
                    assertControlInsideSafeArea(view)
                }.perform(click())
            assertTrue(accepted)
            onView(withId(R.id.unsafeCancel)).inRoot(isDialog()).perform(scrollTo())
                .check(matches(isCompletelyDisplayed()))
                .check { view, error ->
                    if (error != null) throw error
                    assertControlInsideSafeArea(view)
                }.perform(click())
            assertTrue(cancelled)
        }
    }

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
