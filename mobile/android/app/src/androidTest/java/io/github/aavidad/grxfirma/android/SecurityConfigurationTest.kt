// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

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

    // La red solo la usa el motor para el sello de tiempo (TSA) que configure
    // el usuario; no se pide ningún otro permiso de red ni de ubicación.
    @Test
    fun onlyInternetIsRequestedForTimestamping() {
        assertEquals(
            PackageManager.PERMISSION_GRANTED,
            context.packageManager.checkPermission(Manifest.permission.INTERNET, context.packageName),
        )
        for (permission in listOf(
            Manifest.permission.ACCESS_NETWORK_STATE,
            Manifest.permission.ACCESS_WIFI_STATE,
            Manifest.permission.ACCESS_FINE_LOCATION,
            Manifest.permission.READ_EXTERNAL_STORAGE,
        )) {
            assertEquals(
                permission,
                PackageManager.PERMISSION_DENIED,
                context.packageManager.checkPermission(permission, context.packageName),
            )
        }
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
