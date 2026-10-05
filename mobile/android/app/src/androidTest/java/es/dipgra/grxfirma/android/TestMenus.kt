// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package es.dipgra.grxfirma.android

import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.Espresso.openActionBarOverflowOrOptionsMenu
import androidx.test.espresso.NoMatchingViewException
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.scrollTo
import androidx.test.espresso.matcher.ViewMatchers.withId
import androidx.test.espresso.matcher.ViewMatchers.withText
import androidx.test.platform.app.InstrumentationRegistry

/** Acciones del menú de la barra: visibles como icono o dentro del menú de tres puntos. */
object TestMenus {
    fun open(itemId: Int, title: Int) {
        try {
            onView(withId(itemId)).perform(click())
        } catch (_: NoMatchingViewException) {
            openActionBarOverflowOrOptionsMenu(InstrumentationRegistry.getInstrumentation().targetContext)
            onView(withText(title)).perform(click())
        }
    }

    /** Despliega un grupo plegado de la pantalla principal. */
    fun expand(toggleId: Int) {
        onView(withId(toggleId)).perform(scrollTo(), click())
    }
}
