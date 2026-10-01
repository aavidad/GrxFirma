// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.security.NetworkSecurityPolicy
import androidx.test.core.app.ApplicationProvider
import androidx.test.core.app.ActivityScenario
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SecurityConfigurationTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    @Test
    fun applicationDoesNotRequestInternetPermission() {
        assertEquals(
            PackageManager.PERMISSION_DENIED,
            context.packageManager.checkPermission(Manifest.permission.INTERNET, context.packageName),
        )
    }

    @Test
    fun cleartextTrafficIsDisabled() {
        assertFalse(NetworkSecurityPolicy.getInstance().isCleartextTrafficPermitted)
    }

    @Test
    fun applicationBackupsAreDisabled() {
        val applicationInfo = context.packageManager.getApplicationInfo(context.packageName, 0)
        assertEquals(0, applicationInfo.flags and android.content.pm.ApplicationInfo.FLAG_ALLOW_BACKUP)
    }

    @Test
    fun applicationDoesNotRetainDocumentProviderPermissions() {
        ActivityScenario.launch(MainActivity::class.java).use {
            assertTrue(context.contentResolver.persistedUriPermissions.isEmpty())
        }
    }
}
