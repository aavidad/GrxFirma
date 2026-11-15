// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android.ui

import android.view.View
import android.view.ViewGroup
import android.view.Window
import android.view.WindowManager
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import io.github.aavidad.grxfirma.android.R

/** Conserva el relleno de diseño y reserva el área ocupada por el sistema. */
object SafeWindowInsets {
    private val types = WindowInsetsCompat.Type.systemBars() or
        WindowInsetsCompat.Type.ime() or WindowInsetsCompat.Type.displayCutout()

    fun main(window: Window, topBar: View, scroll: View) {
        prepare(window)
        val topPadding = Padding(topBar)
        val scrollPadding = Padding(scroll)
        val bottomMargin = (scroll.layoutParams as ViewGroup.MarginLayoutParams).bottomMargin
        // CoordinatorLayout gestiona su propio listener de insets. Instalamos
        // el nuestro en el FrameLayout de la ventana, antes de sus descendientes.
        val root = window.findViewById<View>(android.R.id.content)
        ViewCompat.setOnApplyWindowInsetsListener(root) { _, insets ->
            val safe = insets.getInsets(types)
            topPadding.apply(topBar, safe.left, safe.top, safe.right, 0)
            scrollPadding.apply(scroll, safe.left, 0, safe.right, 0)
            // NestedScrollView desplaza el foco dentro de su altura exterior:
            // el padding inferior no limita esa área. Reducir el viewport evita
            // que scrollTo y la entrada de texto coloquen el campo bajo el IME.
            val params = scroll.layoutParams as ViewGroup.MarginLayoutParams
            if (params.bottomMargin != bottomMargin + safe.bottom) {
                params.bottomMargin = bottomMargin + safe.bottom
                scroll.layoutParams = params
            }
            // Snackbar necesita también los insets para colocar sus acciones sobre las barras.
            // AppBarLayout no los suma otra vez porque no usa fitsSystemWindows.
            insets
        }
        ViewCompat.requestApplyInsets(root)
    }

    /** El contenedor común protege también el título y la barra inferior del diálogo. */
    fun fullScreenDialog(window: Window) {
        prepare(window)
        val content = window.findViewById<View>(android.R.id.content)
        // Un DialogFragment puede volver a STARTED sin reconstruir su ventana.
        if (content.getTag(R.id.safeWindowInsetsInstalled) == true) return
        content.setTag(R.id.safeWindowInsetsInstalled, true)
        val padding = Padding(content)
        ViewCompat.setOnApplyWindowInsetsListener(content) { _, insets ->
            val safe = insets.getInsets(types)
            padding.apply(content, safe.left, safe.top, safe.right, safe.bottom)
            WindowInsetsCompat.CONSUMED
        }
        ViewCompat.requestApplyInsets(content)
    }

    private fun prepare(window: Window) {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        // Necesario para recibir los insets del teclado también antes de Android 11.
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)
    }

    private class Padding(view: View) {
        private val left = view.paddingLeft
        private val top = view.paddingTop
        private val right = view.paddingRight
        private val bottom = view.paddingBottom

        fun apply(view: View, left: Int, top: Int, right: Int, bottom: Int) {
            view.setPadding(this.left + left, this.top + top, this.right + right, this.bottom + bottom)
        }
    }
}
