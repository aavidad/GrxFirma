// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android

import android.content.res.Configuration
import android.view.ViewGroup
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.scrollTo
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.ViewMatchers.*
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import io.github.aavidad.grxfirma.android.core.UnsafeExceptionState
import io.github.aavidad.grxfirma.android.core.UnsafeExceptions
import io.github.aavidad.grxfirma.android.databinding.DialogPreferencesBinding
import io.github.aavidad.grxfirma.android.ui.MainUiState
import io.github.aavidad.grxfirma.android.ui.SafeWindowInsets
import io.github.aavidad.grxfirma.android.ui.UnsafeExceptionDialog
import io.github.aavidad.grxfirma.android.ui.UnsafeExceptionsPanel
import io.github.aavidad.grxfirma.android.ui.UnsafeExceptionTexts
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
            onView(withId(R.id.unsafeActivate)).check(matches(not(isEnabled())))
            onView(withId(R.id.unsafeAcceptance)).check(matches(not(isChecked()))).perform(scrollTo(), click())
            onView(withId(R.id.unsafeActivate)).check(matches(isEnabled())).perform(scrollTo(), click())
            assertTrue(accepted)
            scenario.onActivity { dialog.render(true, null) }
            onView(withId(R.id.unsafeActivate)).check(matches(not(isEnabled())))
            onView(withId(R.id.unsafeCancel)).check(matches(not(isEnabled())))
            scenario.onActivity { dialog.render(false, null) }
            onView(withId(R.id.unsafeCancel)).perform(scrollTo(), click())
            assertTrue(cancelled)
            scenario.onActivity {
                dialog = UnsafeExceptionDialog(it, item, {}, {})
                dialog.show()
            }
            onView(withId(R.id.unsafeAcceptance)).check(matches(not(isChecked())))
            onView(withId(R.id.unsafeActivate)).check(matches(not(isEnabled())))
        }
    }

    @Test fun organizationBlockHasNoAcceptanceAndStillOffersScrollableHelp() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                UnsafeExceptionDialog(it, item.copy(activable = false, fixed = true),
                    { fail("Una directiva no permite activar") }, {}).show()
            }
            onView(withId(R.id.unsafeActivate)).check(matches(not(isDisplayed())))
            // La casilla no se añade al árbol de accesibilidad si está prohibida.
            onView(withId(R.id.unsafeAcceptance)).check(androidx.test.espresso.assertion.ViewAssertions.doesNotExist())
            onView(withId(R.id.unsafeMore)).perform(scrollTo(), click())
            onView(withId(R.id.unsafeHelpText)).check(matches(isDisplayed()))
            onView(withId(R.id.unsafeHelpText)).check(matches(withText(org.hamcrest.Matchers.containsString("CMS"))))
        }
    }

    @Test fun preferencesEntryExistsInTheActivityAndExplainsAnUnavailableCore() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            TestMenus.open(R.id.action_preferences, R.string.preferences_title)
            lateinit var title: String
            scenario.onActivity { title = UnsafeExceptionTexts.text(it, "seccion") }
            onView(withText(title)).perform(scrollTo()).check(matches(isDisplayed()))
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
            onView(withTagValue(org.hamcrest.Matchers.`is`("unsafe-change-cms_cbc" as Any))).perform(scrollTo(), click())
            assertEquals("cms_cbc", disabled)
            onView(withTagValue(org.hamcrest.Matchers.`is`("unsafe-change-sha1_leer" as Any))).perform(scrollTo(), click())
            assertEquals("sha1_leer", activated)
            onView(withId(R.id.unsafeActivate)).check(matches(not(isEnabled())))
        }
    }

    @Test fun consentRemainsScrollableAtTwoHundredPercentTextSize() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val configuration = Configuration(activity.resources.configuration).apply { fontScale = 2f }
                val context = android.view.ContextThemeWrapper(activity, R.style.Theme_GrxFirma).apply {
                    applyOverrideConfiguration(configuration)
                }
                UnsafeExceptionDialog(context, item, {}, {}).show()
            }
            onView(withId(R.id.unsafeAcceptance)).perform(scrollTo(), click())
            onView(withId(R.id.unsafeActivate)).perform(scrollTo()).check(matches(isEnabled()))
            onView(withId(R.id.unsafeCancel)).perform(scrollTo()).check(matches(isDisplayed()))
        }
    }
}
