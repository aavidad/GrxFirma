// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import android.view.View
import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.assertion.ViewAssertions.doesNotExist
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.RootMatchers.isDialog
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.withText
import org.hamcrest.Matchers.containsString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class HelpButtonUiTest {
    /** El «?» de la operación se anuncia con el nombre del control y abre la explicación de la cofirma. */
    @Test fun operationHelpNamesItsControlAndExplainsCosign() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            lateinit var cosign: String
            scenario.onActivity { activity ->
                val help = activity.findViewById<View>(R.id.signatureActionHelp)
                assertEquals(activity.getString(R.string.ayuda_boton_nombre, activity.getString(R.string.signature_action)),
                    help.contentDescription)
                assertTrue(help.isClickable)
                assertTrue(help.minimumWidth >= help.resources.displayMetrics.density * 48 - 1 && help.minimumHeight >= help.minimumWidth)
                cosign = activity.getString(R.string.ayuda_operacion_cofirma)
                help.performClick()
            }
            onView(withText(containsString(cosign))).inRoot(isDialog()).check(matches(isDisplayed()))
            onView(withText(R.string.help_close)).inRoot(isDialog()).perform(click())
            onView(withText(containsString(cosign))).check(doesNotExist())
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
