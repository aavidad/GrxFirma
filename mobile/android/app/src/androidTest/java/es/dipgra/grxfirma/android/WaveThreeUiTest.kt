// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android

import android.content.Context
import android.view.View
import androidx.appcompat.app.AppCompatDelegate
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.espresso.Espresso.onData
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.scrollTo
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.RootMatchers.isDialog
import androidx.test.espresso.matcher.RootMatchers.isPlatformPopup
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.withId
import androidx.test.espresso.matcher.ViewMatchers.withText
import androidx.test.platform.app.InstrumentationRegistry
import es.dipgra.grxfirma.android.settings.AppPreferences
import es.dipgra.grxfirma.android.settings.AppSettings
import org.hamcrest.Matchers.anything
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/**
 * Preferencias, restauración y servicios ocultos en la compilación
 * verificable. Cada cambio de tema recrea la actividad, así que cada paso
 * consulta la actividad viva y no reutiliza vistas antiguas.
 */
class WaveThreeUiTest {
    private val context: Context get() = ApplicationProvider.getApplicationContext()

    @Before fun cleanPreferences() = AppPreferences(context).reset()

    @After fun restoreDefaults() {
        AppPreferences(context).reset()
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            AppCompatDelegate.setDefaultNightMode(AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM)
        }
    }

    @Test fun preferencesSaveThemeAndRestoreDefaults() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            onView(withId(R.id.preferencesButton)).perform(scrollTo(), click())
            onView(withId(R.id.prefTheme)).inRoot(isDialog()).perform(scrollTo(), click())
            // Lista del desplegable: onData elige la opción «Oscuro» por posición.
            onData(anything()).inRoot(isPlatformPopup()).atPosition(2).perform(click())
            onView(withText(R.string.preferences_save)).inRoot(isDialog()).perform(click())
            InstrumentationRegistry.getInstrumentation().waitForIdleSync()
            assertEquals(AppSettings.THEME_DARK, AppPreferences(context).load().theme)
            scenario.onActivity {
                assertEquals(AppCompatDelegate.MODE_NIGHT_YES, AppCompatDelegate.getDefaultNightMode())
            }
        }
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            onView(withId(R.id.preferencesButton)).perform(scrollTo(), click())
            onView(withId(R.id.prefRestore)).inRoot(isDialog()).perform(scrollTo(), click())
            onView(withText(R.string.preferences_restore_action)).inRoot(isDialog()).perform(click())
            InstrumentationRegistry.getInstrumentation().waitForIdleSync()
            assertEquals(AppSettings(), AppPreferences(context).load())
            scenario.onActivity {
                assertEquals(AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM, AppCompatDelegate.getDefaultNightMode())
            }
        }
    }

    @Test fun timestampProfileWithoutTsaIsRejectedInTheDialog() {
        ActivityScenario.launch(MainActivity::class.java).use {
            onView(withId(R.id.preferencesButton)).perform(scrollTo(), click())
            onView(withId(R.id.prefProfile)).inRoot(isDialog()).perform(scrollTo(), click())
            onData(anything()).inRoot(isPlatformPopup()).atPosition(2).perform(click())
            onView(withText(R.string.preferences_save)).inRoot(isDialog()).perform(click())
            onView(withId(R.id.prefTsaUrlLayout)).inRoot(isDialog()).check(matches(isDisplayed()))
            assertEquals("baseline", AppPreferences(context).load().defaultProfile)
            onView(withText(R.string.preferences_cancel)).inRoot(isDialog()).perform(click())
        }
    }

    @Test fun verificationBuildHidesNetworkServices() {
        if (BuildConfig.CORE_MODE == "production") return
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                assertEquals(View.GONE, activity.findViewById<View>(R.id.diagnosticsButton).visibility)
                assertEquals(View.GONE, activity.findViewById<View>(R.id.qrGroup).visibility)
                assertEquals(View.GONE, activity.findViewById<View>(R.id.checkCertificateOnlineButton).visibility)
            }
        }
    }
}
