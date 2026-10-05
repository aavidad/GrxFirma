// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android

import android.content.Context
import android.content.res.Configuration
import android.os.LocaleList
import androidx.appcompat.app.AppCompatDelegate
import androidx.core.os.LocaleListCompat
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.matcher.RootMatchers.isDialog
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.scrollTo
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.ViewMatchers.withId
import androidx.test.espresso.matcher.ViewMatchers.withText
import androidx.test.platform.app.InstrumentationRegistry
import org.hamcrest.Matchers.containsString
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class ParityUiTest {
    @After fun restoreSystemLanguage() {
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            AppCompatDelegate.setApplicationLocales(LocaleListCompat.getEmptyLocaleList())
        }
    }

    @Test fun operationProfilesAboutAndLanguageSelectorAreReachable() {
        ActivityScenario.launch(MainActivity::class.java).use {
            onView(withId(R.id.signatureAction)).perform(scrollTo())
            onView(withId(R.id.signatureProfile)).perform(scrollTo())
            onView(withId(R.id.tsaEnabled)).perform(scrollTo())
            onView(withId(R.id.aboutButton)).perform(scrollTo(), click())
            onView(withId(android.R.id.message)).check(matches(withText(containsString(BuildConfig.VERSION_NAME))))
            onView(withText(R.string.help_close)).perform(click())
            onView(withId(R.id.languageButton)).perform(scrollTo(), click())
            it.onActivity { activity ->
                assertEquals(12, activity.resources.getStringArray(R.array.language_names).size)
            }
        }
    }

    @Test fun choosingEnglishAndSystemUsesTheAppLocaleDelegate() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val names = context.resources.getStringArray(R.array.language_names)
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            onView(withId(R.id.languageButton)).perform(scrollTo(), click())
            onView(withText(names[2])).inRoot(isDialog()).perform(click())
            InstrumentationRegistry.getInstrumentation().waitForIdleSync()
            scenario.onActivity {
                assertEquals("en", AppCompatDelegate.getApplicationLocales()[0]?.language)
            }
            onView(withId(R.id.languageButton)).perform(scrollTo(), click())
            scenario.onActivity { activity ->
                // The system option is localized after the first recreation.
                names[0] = activity.getString(R.string.language_system)
            }
            onView(withText(names[0])).inRoot(isDialog()).perform(click())
            InstrumentationRegistry.getInstrumentation().waitForIdleSync()
            scenario.onActivity {
                assertEquals(0, AppCompatDelegate.getApplicationLocales().size())
            }
        }
    }

    @Test fun everyDesktopLanguageHasReadableAndroidResourcesIncludingValencian() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        for (tag in listOf("es", "en", "ca", "ca-ES-valencia", "gl", "eu", "fr", "de", "it", "pt", "zh")) {
            val configuration = Configuration(context.resources.configuration).apply {
                setLocales(LocaleList.forLanguageTags(tag))
            }
            val localized = context.createConfigurationContext(configuration)
            for (id in listOf(R.string.signature_action, R.string.signature_profile, R.string.about_title,
                R.string.export_verification_report, R.string.post_sign_verification_failed, R.string.dnie_pin_help,
                R.string.seal_edit_hint, R.string.language_system)) {
                assertFalse(localized.getString(id).isBlank())
            }
            assertEquals(4, localized.resources.getStringArray(R.array.signature_profiles).size)
        }
    }
}
