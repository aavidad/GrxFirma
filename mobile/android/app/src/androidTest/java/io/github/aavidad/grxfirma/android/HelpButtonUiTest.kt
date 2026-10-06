// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import android.view.View
import androidx.core.view.ViewCompat
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.assertion.ViewAssertions.doesNotExist
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.RootMatchers.isDialog
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.withContentDescription
import androidx.test.espresso.matcher.ViewMatchers.withText
import org.hamcrest.Matchers.containsString
import org.hamcrest.Matchers.not
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class HelpButtonUiTest {
    /**
     * El «?» de la operación se anuncia con el nombre del control y, sin elegir
     * ninguna opción, explica las tres. La cofirma lleva un «+» con nombre y
     * estado que despliega su explicación ampliada.
     */
    @Test fun operationHelpListsEveryOptionAndExpandsCosign() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            lateinit var texts: List<String>
            lateinit var cosignMore: String
            lateinit var moreName: String
            lateinit var collapsed: String
            lateinit var expanded: String
            scenario.onActivity { activity ->
                val help = activity.findViewById<View>(R.id.signatureActionHelp)
                assertEquals(activity.getString(R.string.ayuda_boton_nombre, activity.getString(R.string.signature_action)),
                    help.contentDescription)
                assertTrue(help.isClickable)
                assertTrue(help.minimumWidth >= help.resources.displayMetrics.density * 48 - 1 && help.minimumHeight >= help.minimumWidth)
                texts = listOf(R.string.ayuda_operacion_firma, R.string.ayuda_operacion_cofirma,
                    R.string.ayuda_operacion_contrafirma).map(activity::getString)
                cosignMore = activity.getString(R.string.ayuda_operacion_cofirma_mas)
                moreName = activity.getString(R.string.ayuda_mas_nombre, activity.getString(R.string.action_cosign))
                collapsed = activity.getString(R.string.ayuda_mas_plegado)
                expanded = activity.getString(R.string.ayuda_mas_desplegado)
                help.performClick()
            }
            texts.forEach { onView(withText(containsString(it))).inRoot(isDialog()).check(matches(isDisplayed())) }
            onView(withText(cosignMore)).inRoot(isDialog()).check(matches(not(isDisplayed())))
            onView(withContentDescription(moreName)).inRoot(isDialog())
                .check(matches(isDisplayed()))
                .check { view, _ -> assertEquals(collapsed, ViewCompat.getStateDescription(view)) }
                .perform(click())
            onView(withText(cosignMore)).inRoot(isDialog()).check(matches(isDisplayed()))
            onView(withContentDescription(moreName)).inRoot(isDialog())
                .check { view, _ -> assertEquals(expanded, ViewCompat.getStateDescription(view)) }
                .perform(click())
            onView(withText(cosignMore)).inRoot(isDialog()).check(matches(not(isDisplayed())))
            onView(withText(R.string.help_close)).inRoot(isDialog()).perform(click())
            onView(withText(containsString(texts[1]))).check(doesNotExist())
        }
    }

    /** Todos los «?» de la pantalla principal tienen nombre accesible «Ayuda sobre …». */
    @Test fun everyHelpButtonHasAnAccessibleName() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val empty = activity.getString(R.string.ayuda_boton_nombre, "")
                val ids = listOf(R.id.selectOriginalDocumentHelp, R.id.selectCertificateFileHelp, R.id.selectDnieNfcHelp,
                    R.id.visibleSealHelp, R.id.signatureActionHelp, R.id.tsaEnabledHelp, R.id.tsaUrlHelp,
                    R.id.signatureFormatHelp, R.id.exportReportHelp, R.id.verificationIntegrityHelp,
                    R.id.verificationTrustHelp, R.id.verificationCoverageHelp, R.id.certificateKindHelp,
                    R.id.checkCertificateOnlineHelp, R.id.batchHelp, R.id.hashAlgorithmHelp, R.id.hashFormatHelp,
                    R.id.createHashHelp, R.id.protectionContainerHelp, R.id.addRecipientHelp, R.id.protectKeyHelp,
                    R.id.protectHelp, R.id.protectSignHelp, R.id.unprotectHelp, R.id.verifactuHelp, R.id.eniHelp,
                    R.id.eniMetadataHelp, R.id.eniOriginHelp, R.id.eniStateHelp, R.id.expedienteHelp)
                ids.forEach { id ->
                    val description = activity.findViewById<View>(id).contentDescription?.toString().orEmpty()
                    // «Ayuda sobre » más el nombre del control: más largo que la plantilla vacía.
                    assertTrue(activity.resources.getResourceEntryName(id), description.length > empty.length)
                }
            }
        }
    }
}
