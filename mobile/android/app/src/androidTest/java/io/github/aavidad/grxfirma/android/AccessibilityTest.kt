// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import androidx.test.espresso.accessibility.AccessibilityChecks
import androidx.test.espresso.matcher.ViewMatchers.withId
import com.google.android.apps.common.testing.accessibility.framework.AccessibilityCheckResultUtils.matchesCheck
import com.google.android.apps.common.testing.accessibility.framework.AccessibilityCheckResultUtils.matchesViews
import com.google.android.apps.common.testing.accessibility.framework.checks.TouchTargetSizeCheck
import org.hamcrest.Matchers.allOf
import org.junit.Before

/** Base común: los errores de accesibilidad hacen fallar las acciones de Espresso. */
abstract class AccessibilityTest {
    @Before
    fun enableAccessibilityChecks() {
        AccessibilityChecks.enable()
            .setRunChecksFromRootView(true)
            // Material marca como objetivo táctil la fila informativa de días de la semana,
            // que no ofrece una acción. Excepción autorizada solo para su ID y su tamaño táctil.
            .setSuppressingResultMatcher(allOf(
                matchesCheck(TouchTargetSizeCheck::class.java),
                matchesViews(withId(com.google.android.material.R.id.mtrl_calendar_days_of_week)),
            ))
    }
}
