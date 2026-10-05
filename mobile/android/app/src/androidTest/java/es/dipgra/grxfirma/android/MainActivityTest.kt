// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android

import android.content.ContentValues
import android.content.Context
import android.view.View
import android.content.Intent
import android.os.Environment
import android.provider.MediaStore
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.isEnabled
import androidx.test.espresso.matcher.ViewMatchers.withId
import androidx.test.espresso.matcher.ViewMatchers.withText
import java.util.ArrayList
import org.hamcrest.Matchers.containsString
import org.hamcrest.Matchers.not
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class MainActivityTest {
    private val productionCore: Boolean
        get() = BuildConfig.CORE_MODE == "production"

    @Test
    fun buildShowsHonestBackendState() {
        ActivityScenario.launch(MainActivity::class.java).use {
            // Con todo correcto la tarjeta se oculta; si no puede firmar, lo dice.
            if (productionCore) {
                it.onActivity { activity ->
                    assertEquals(View.GONE, activity.findViewById<View>(R.id.backendStatusCard).visibility)
                }
            } else {
                onView(withId(R.id.backendStatusTitle)).check(matches(withText(R.string.backend_unavailable_title)))
            }
            onView(withId(R.id.selectDocumentButton)).check(matches(isDisplayed()))
            onView(withId(R.id.selectOriginalDocumentButton)).check(matches(isDisplayed()))
            onView(withId(R.id.signButton)).check(matches(not(isEnabled())))
            onView(withId(R.id.verifyButton)).check(matches(not(isEnabled())))
            onView(withId(R.id.actionHint)).check(matches(withText(
                if (productionCore) R.string.hint_need_document else R.string.hint_unavailable)))
            TestMenus.open(R.id.action_help, R.string.help_title)
            onView(withText(R.string.help_title)).check(matches(isDisplayed()))
        }
    }

    @Test
    fun sharedContentUriIsPreparedWithoutResolvingAFileSystemPath() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val values = ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, "contrato-compartido.pdf")
            put(MediaStore.MediaColumns.MIME_TYPE, "application/pdf")
            put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
        }
        val uri = checkNotNull(
            context.contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values),
        )
        try {
            context.contentResolver.openOutputStream(uri)?.use {
                it.write("%PDF-1.7\n".encodeToByteArray())
            }
            val intent = Intent(context, MainActivity::class.java).apply {
                action = Intent.ACTION_SEND
                type = "application/pdf"
                putExtra(Intent.EXTRA_STREAM, uri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }

            ActivityScenario.launch<MainActivity>(intent).use {
                onView(withId(R.id.documentSummary))
                    .check(matches(withText(containsString("contrato-compartido.pdf"))))
                onView(withId(R.id.signButton)).check(matches(not(isEnabled())))
                onView(withId(R.id.verifyButton)).check(
                    matches(if (productionCore) isEnabled() else not(isEnabled())),
                )
            }
        } finally {
            context.contentResolver.delete(uri, null, null)
        }
    }

    @Test
    fun multipleShareIsNotAdvertisedAndExplicitIntentGetsClearRejection() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val uris = (1..2).map { index ->
            val values = ContentValues().apply {
                put(MediaStore.MediaColumns.DISPLAY_NAME, "contrato-$index.pdf")
                put(MediaStore.MediaColumns.MIME_TYPE, "application/pdf")
                put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
            }
            checkNotNull(
                context.contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values),
            )
        }
        try {
            val sharedMultiple = Intent(Intent.ACTION_SEND_MULTIPLE).apply {
                type = "application/pdf"
                putParcelableArrayListExtra(Intent.EXTRA_STREAM, ArrayList(uris))
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            val advertised = context.packageManager
                .queryIntentActivities(sharedMultiple, android.content.pm.PackageManager.MATCH_DEFAULT_ONLY)
                .any { it.activityInfo.packageName == context.packageName }
            assertFalse(
                "La APK no debe anunciar un flujo múltiple que esta pantalla todavía no ejecuta.",
                advertised,
            )

            val explicit = Intent(sharedMultiple).setClass(context, MainActivity::class.java)
            ActivityScenario.launch<MainActivity>(explicit).use {
                onView(withId(R.id.resultTitle)).check(matches(withText(R.string.result_error)))
                onView(withId(R.id.resultDetail))
                    .check(matches(withText(R.string.error_multiple_documents)))
                onView(withId(R.id.documentSummary))
                    .check(matches(withText(R.string.no_document)))
            }
        } finally {
            uris.forEach { context.contentResolver.delete(it, null, null) }
        }
    }
}
