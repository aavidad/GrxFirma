// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import android.view.View
import io.github.aavidad.grxfirma.android.ui.DropdownField
import androidx.test.core.app.ActivityScenario
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

class DocumentsUiTest : AccessibilityTest() {
    /** La compilación de verificación no declara servicios: la sección se oculta y lo explica. */
    @Test fun documentToolsAreHiddenWithoutADeclaringCore() {
        assumeTrue("solo la compilación de verificación carece de motor que declare servicios", BuildConfig.CORE_MODE == "verification")
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                assertEquals(View.GONE, it.findViewById<View>(R.id.verifactuRow).visibility)
                assertEquals(View.GONE, it.findViewById<View>(R.id.eniRow).visibility)
                assertEquals(View.GONE, it.findViewById<View>(R.id.verifactuGroup).visibility)
                assertEquals(View.VISIBLE, it.findViewById<View>(R.id.documentsSectionTitle).visibility)
            }
        }
    }

    /** Sin AAR que los declare, el desplegable ofrece solo automático y los tres formatos básicos. */
    @Test fun signatureFormatsFollowTheContract() {
        assumeTrue("solo la compilación de verificación carece de motor que declare formatos", BuildConfig.CORE_MODE == "verification")
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                val field = it.findViewById<DropdownField>(R.id.signatureFormat)
                assertEquals(4, field.adapter.count)
                assertEquals(it.getString(R.string.format_auto), field.adapter.getItem(0))
                assertEquals(it.getString(R.string.format_auto), field.text.toString())
            }
        }
    }

    /** Con el motor de producción, que declara los servicios, las herramientas se ven y hay más formatos. */
    @Test fun productionCoreShowsDocumentToolsAndFormats() {
        assumeTrue("solo la compilación de producción lleva el motor completo", BuildConfig.CORE_MODE == "production")
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                assertEquals(View.VISIBLE, it.findViewById<View>(R.id.documentsSectionTitle).visibility)
                assertEquals(View.VISIBLE, it.findViewById<View>(R.id.verifactuRow).visibility)
                assertEquals(View.VISIBLE, it.findViewById<View>(R.id.eniRow).visibility)
                val field = it.findViewById<DropdownField>(R.id.signatureFormat)
                assertTrue(field.adapter.count > 4)
                assertEquals(it.getString(R.string.format_auto), field.adapter.getItem(0))
                assertEquals(it.getString(R.string.format_auto), field.text.toString())
            }
        }
    }

    @Test fun eniFieldsAreExcludedFromAutofill() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity {
                for (id in listOf(R.id.eniOrgans, R.id.eniIdentifier, R.id.eniSource, R.id.eniContentFormat)) {
                    assertTrue(it.findViewById<View>(id).importantForAutofill == View.IMPORTANT_FOR_AUTOFILL_NO)
                }
            }
        }
    }
}
