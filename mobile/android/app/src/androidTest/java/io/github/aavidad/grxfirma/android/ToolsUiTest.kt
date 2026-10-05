// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package io.github.aavidad.grxfirma.android

import android.view.View
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.scrollTo
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.isNotEnabled
import androidx.test.espresso.matcher.ViewMatchers.withId
import org.junit.Assert.assertEquals
import org.junit.Test

class ToolsUiTest {
    @Test fun toolGroupsExpandAndStayDisabledWithoutAProductionCore() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                assertEquals(View.GONE, it.findViewById<View>(R.id.batchGroup).visibility)
                assertEquals(View.GONE, it.findViewById<View>(R.id.hashGroup).visibility)
                assertEquals(View.GONE, it.findViewById<View>(R.id.protectGroup).visibility)
            }
            TestMenus.expand(R.id.toggleOtherToolsButton)
            onView(withId(R.id.toggleBatchButton)).perform(scrollTo(), click())
            onView(withId(R.id.selectBatchButton)).perform(scrollTo()).check(matches(isDisplayed()))
            onView(withId(R.id.signBatchButton)).check(matches(isNotEnabled()))
            onView(withId(R.id.toggleHashButton)).perform(scrollTo(), click())
            onView(withId(R.id.createHashButton)).perform(scrollTo()).check(matches(isNotEnabled()))
            onView(withId(R.id.toggleProtectButton)).perform(scrollTo(), click())
            onView(withId(R.id.protectButton)).perform(scrollTo()).check(matches(isNotEnabled()))
            onView(withId(R.id.unprotectButton)).perform(scrollTo()).check(matches(isNotEnabled()))
            scenario.recreate()
            scenario.onActivity {
                assertEquals(View.VISIBLE, it.findViewById<View>(R.id.protectGroup).visibility)
            }
        }
    }

    @Test fun keyFieldsAreExcludedFromAutofillAndNotSaved() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                val key = it.findViewById<View>(R.id.protectKey)
                assertEquals(View.IMPORTANT_FOR_AUTOFILL_NO_EXCLUDE_DESCENDANTS, key.importantForAutofill)
                assertEquals(false, key.isSaveEnabled)
            }
        }
    }
}
