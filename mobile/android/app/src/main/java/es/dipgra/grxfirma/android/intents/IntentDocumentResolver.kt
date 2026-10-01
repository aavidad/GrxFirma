// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.intents

import android.content.Intent
import android.net.Uri
import android.os.Build

sealed interface IncomingDocument {
    data class Single(val uri: Uri) : IncomingDocument
    data object Multiple : IncomingDocument
    data object Invalid : IncomingDocument
    data object None : IncomingDocument
}

object IntentDocumentResolver {
    fun resolve(intent: Intent?): IncomingDocument {
        if (intent == null) return IncomingDocument.None
        return when (intent.action) {
            Intent.ACTION_VIEW -> intent.data
                ?.takeIf { it.scheme == "content" }
                ?.let(IncomingDocument::Single)
                ?: IncomingDocument.Invalid

            Intent.ACTION_SEND -> streamUri(intent)
                ?.takeIf { it.scheme == "content" }
                ?.let(IncomingDocument::Single)
                ?: IncomingDocument.Invalid

            Intent.ACTION_SEND_MULTIPLE -> IncomingDocument.Multiple
            else -> IncomingDocument.None
        }
    }

    private fun streamUri(intent: Intent): Uri? = if (Build.VERSION.SDK_INT >= 33) {
        intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
    } else {
        @Suppress("DEPRECATION")
        intent.getParcelableExtra(Intent.EXTRA_STREAM)
    }
}
