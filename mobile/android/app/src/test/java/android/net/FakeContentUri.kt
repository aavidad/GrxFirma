// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package android.net

import android.os.Parcel

/**
 * Uri de pruebas para los tests unitarios de la JVM, donde `Uri.parse` no
 * existe (android.jar solo trae los stubs). Está en `android.net` porque el
 * constructor de Uri es de paquete. Solo sirve de identificador opaco.
 */
class FakeContentUri(private val value: String) : Uri() {
    override fun buildUpon(): Builder = throw UnsupportedOperationException()
    override fun getAuthority(): String = "test"
    override fun getEncodedAuthority(): String = "test"
    override fun getEncodedFragment(): String? = null
    override fun getEncodedPath(): String = value.substringAfter("content://test")
    override fun getEncodedQuery(): String? = null
    override fun getEncodedSchemeSpecificPart(): String = value.substringAfter("content:")
    override fun getEncodedUserInfo(): String? = null
    override fun getFragment(): String? = null
    override fun getHost(): String = "test"
    override fun getLastPathSegment(): String = value.substringAfterLast('/')
    override fun getPath(): String = encodedPath
    override fun getPathSegments(): List<String> = encodedPath.split('/').filter { it.isNotEmpty() }
    override fun getPort(): Int = -1
    override fun getQuery(): String? = null
    override fun getScheme(): String = "content"
    override fun getSchemeSpecificPart(): String = encodedSchemeSpecificPart
    override fun getUserInfo(): String? = null
    override fun isHierarchical(): Boolean = true
    override fun isRelative(): Boolean = false
    override fun toString(): String = value
    override fun equals(other: Any?): Boolean = other is FakeContentUri && other.value == value
    override fun hashCode(): Int = value.hashCode()
    override fun describeContents(): Int = 0
    override fun writeToParcel(dest: Parcel, flags: Int) = throw UnsupportedOperationException()
}
