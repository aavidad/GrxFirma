// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package io.github.aavidad.grxfirma.android

import android.content.ContentValues
import android.content.Context
import android.content.ContextWrapper
import android.app.Activity
import android.graphics.Rect
import android.graphics.pdf.PdfDocument
import android.os.Environment
import android.os.SystemClock
import android.provider.MediaStore
import android.view.View
import android.view.ViewGroup
import android.widget.ScrollView
import android.widget.CompoundButton
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.widget.NestedScrollView
import androidx.lifecycle.ViewModelProvider
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.IdlingRegistry
import androidx.test.espresso.IdlingResource
import androidx.test.espresso.UiController
import androidx.test.espresso.ViewAction
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.actionWithAssertions
import androidx.test.espresso.action.ViewActions.scrollTo
import androidx.test.espresso.matcher.RootMatchers.isDialog
import androidx.test.espresso.matcher.RootMatchers.isPlatformPopup
import androidx.test.espresso.matcher.ViewMatchers.hasDescendant
import androidx.test.espresso.matcher.ViewMatchers.isAssignableFrom
import androidx.test.espresso.matcher.ViewMatchers.withId
import androidx.test.espresso.matcher.ViewMatchers.withText
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.isEnabled
import androidx.test.espresso.assertion.ViewAssertions.matches
import com.google.android.material.button.MaterialButton
import com.google.android.material.datepicker.MaterialDatePicker
import io.github.aavidad.grxfirma.android.model.SelectedFile
import io.github.aavidad.grxfirma.android.seal.SealEditorDialog
import io.github.aavidad.grxfirma.android.seal.SealSettings
import io.github.aavidad.grxfirma.android.ui.MainViewModel
import org.hamcrest.Matcher
import org.hamcrest.Matchers.allOf
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import android.view.ViewTreeObserver

/** Ejecutar con gestos y con tres botones; la geometría usa los insets reales del dispositivo. */
class WindowInsetsUiTest : AccessibilityTest() {
    @Test fun mainAndExpandedSectionsKeepTheirLastControlInsideTheSafeArea() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            checkLastInMainScroll()
            // Las herramientas documentales solo existen si las declara el núcleo.
            // En producción este mismo recorrido incluye VeriFactu, ENI y expediente.
            for ((id, group) in listOf(R.id.toggleSigningOptionsButton to R.id.signingOptionsGroup,
                R.id.toggleOtherToolsButton to R.id.otherToolsGroup,
                R.id.toggleBatchButton to R.id.batchGroup, R.id.toggleHashButton to R.id.hashGroup,
                R.id.toggleProtectButton to R.id.protectGroup,
                R.id.toggleVerifactuButton to R.id.verifactuGroup, R.id.toggleEniButton to R.id.eniGroup,
                R.id.toggleExpedienteButton to R.id.expedienteGroup)) {
                var present = false
                scenario.onActivity { present = it.findViewById<View>(id).isShown }
                if (present) {
                    TestMenus.expand(id)
                    onView(withId(group)).perform(revealLastControl())
                    checkLastInMainScroll()
                }
            }
            scenario.recreate()
            checkLastInMainScroll()
        }
    }

    @Test fun lastFieldAndLastButtonRemainReachableWithTheKeyboardOpen() {
        ActivityScenario.launch(MainActivity::class.java).use {
            TestMenus.expand(R.id.toggleOtherToolsButton)
            TestMenus.expand(R.id.toggleProtectButton)
            onView(withId(R.id.protectionContainer)).perform(scrollTo(), click())
            onView(withText(R.string.container_encrypted)).inRoot(isPlatformPopup()).perform(click())
            onView(withId(R.id.protectKeyConfirm)).perform(scrollTo(), showKeyboard())
            onView(withId(R.id.protectKeyConfirm)).perform(scrollTo(), awaitStableGeometry(requireIme = true)).check { view, error ->
                if (error != null) throw error
                assertInsideSafeArea(view)
            }
            checkLastInMainScroll(requireIme = true)
        }
    }

    @Test fun preferencesAndInformationDialogsKeepTheirLastButtonVisible() {
        ActivityScenario.launch(MainActivity::class.java).use {
            val dialogs = mutableListOf(R.id.action_preferences to R.string.preferences_button,
                R.id.action_help to R.string.help_title, R.id.action_about to R.string.about_title)
            if (BuildConfig.CORE_MODE == "production") dialogs += R.id.action_diagnostics to R.string.diagnostics_button
            for ((action, title) in dialogs) {
                TestMenus.open(action, title)
                // Acerca de usa una columna propia de acciones, incluido Cerrar.
                val button = if (action == R.id.action_about) allOf(withText(R.string.help_close), isDisplayed())
                    else withId(android.R.id.button1)
                onView(button).inRoot(isDialog())
                    .perform(awaitStableGeometry()).check { view, error ->
                        if (error != null) throw error
                        assertInsideSafeArea(view)
                    }
                onView(button).inRoot(isDialog()).perform(click())
            }
        }
    }

    @Test fun preferencesKeepTheFieldAndSaveButtonVisibleWithTheKeyboardOpen() {
        ActivityScenario.launch(MainActivity::class.java).use {
            TestMenus.open(R.id.action_preferences, R.string.preferences_button)
            onView(withId(R.id.prefTsaEnabled)).inRoot(isDialog()).perform(scrollTo(),
                action("Activar el campo del servidor TSA") { _, view ->
                    val toggle = view as CompoundButton
                    if (!toggle.isChecked) toggle.performClick()
                })
            onView(withId(R.id.prefTsaUrl)).inRoot(isDialog())
                .perform(scrollTo(), showKeyboard(), scrollTo(), awaitStableGeometry(requireIme = true))
                .check { view, error ->
                    if (error != null) throw error
                    assertInsideSafeArea(view)
                }
            onView(withId(android.R.id.button1)).inRoot(isDialog())
                .perform(awaitStableGeometry(requireIme = true)).check { view, error ->
                    if (error != null) throw error
                    assertInsideSafeArea(view)
                }
            onView(withId(android.R.id.button2)).inRoot(isDialog()).perform(click())
        }
    }

    @Test fun fullScreenCalendarKeepsConfirmationInsideTheSafeArea() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                // Mismo componente y callbacks que los dos calendarios ENI.
                MaterialDatePicker.Builder.datePicker()
                    .setTheme(com.google.android.material.R.style.ThemeOverlay_Material3_MaterialCalendar_Fullscreen)
                    .setSelection(MaterialDatePicker.todayInUtcMilliseconds())
                    .build().show(activity.supportFragmentManager, "insets_calendar")
                activity.supportFragmentManager.executePendingTransactions()
            }
            onView(withId(com.google.android.material.R.id.confirm_button)).inRoot(isDialog())
                .check(matches(isEnabled()))
                .perform(action("Comprobar la accesibilidad del calendario") { ui, _ -> ui.loopMainThreadUntilIdle() })
                .check { view, error ->
                    if (error != null) throw error
                    assertInsideSafeArea(view)
                }
        }
    }

    @Test fun fullScreenSealKeepsItsLastSettingAndBottomButtonsVisible() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val resolver = context.contentResolver
        val uri = checkNotNull(resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, "insets-sello.pdf")
            put(MediaStore.MediaColumns.MIME_TYPE, "application/pdf")
            put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
        }))
        try {
            val pdf = PdfDocument()
            try {
                val page = pdf.startPage(PdfDocument.PageInfo.Builder(595, 842, 1).create())
                pdf.finishPage(page)
                checkNotNull(resolver.openOutputStream(uri)).use(pdf::writeTo)
            } finally {
                pdf.close()
            }
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                onView(withId(R.id.rootLayout)).check(matches(isDisplayed()))
                lateinit var opened: DialogWindowIdler
                scenario.onActivity { activity ->
                    opened = DialogWindowIdler(activity.window.decorView)
                    SealEditorDialog(activity, ViewModelProvider(activity)[MainViewModel::class.java],
                        SelectedFile(uri, "insets-sello.pdf", "application/pdf", null), SealSettings(),
                        File(activity.cacheDir, "insets-logo.png"), {}, { _, _, _, _ -> }, {},
                        { error -> throw AssertionError("No se pudo abrir el editor: $error") }).show()
                }
                // La preparación del PDF se hace en IO; Espresso espera el cambio de foco de la ventana.
                IdlingRegistry.getInstance().register(opened)
                try {
                    onView(allOf(isAssignableFrom(ScrollView::class.java),
                        hasDescendant(withText(R.string.seal_edit_hint))))
                        .inRoot(isDialog()).perform(scrollToLast())
                    onView(withId(android.R.id.button1)).inRoot(isDialog()).check { view, error ->
                        if (error != null) throw error
                        assertInsideSafeArea(view)
                    }
                    onView(withId(android.R.id.button2)).inRoot(isDialog()).perform(click())
                } finally {
                    IdlingRegistry.getInstance().unregister(opened)
                    scenario.onActivity { opened.close() }
                }
            }
        } finally {
            resolver.delete(uri, null, null)
        }
    }

    private fun checkLastInMainScroll(requireIme: Boolean = false) {
        onView(withId(R.id.contentScroll)).perform(scrollToLast(requireIme))
    }

    private fun scrollToLast(requireIme: Boolean = false) = action("Desplazar hasta el último control y comprobar su área visible") { ui, view ->
        val last = descendants(view).lastOrNull {
            it.isShown && it !is ViewGroup && (it.isClickable || it.isFocusable)
        } ?: throw AssertionError("No hay controles en el contenedor")
        // Lleva el control completo al área útil, incluido el relleno por insets.
        when (view) {
            is NestedScrollView -> {
                view.isSmoothScrollingEnabled = false
                view.fullScroll(View.FOCUS_DOWN)
            }
            is ScrollView -> {
                view.isSmoothScrollingEnabled = false
                view.fullScroll(View.FOCUS_DOWN)
            }
            else -> throw AssertionError("El contenedor no permite desplazamiento")
        }
        waitForStableGeometry(ui, last, requireIme)
        assertInsideSafeArea(last)
    }

    private fun revealLastControl() = action("Mostrar el último control de la sección") { ui, view ->
        val last = descendants(view).lastOrNull {
            it.isShown && it !is ViewGroup && (it.isClickable || it.isFocusable)
        } ?: throw AssertionError("La sección no tiene controles")
        last.requestRectangleOnScreen(Rect(0, 0, last.width, last.height), true)
        waitForStableGeometry(ui, last)
        assertInsideSafeArea(last)
    }

    private fun showKeyboard() = action("Mostrar el teclado y esperar sus insets") { ui, view ->
        assertTrue("El campo debe admitir el foco", view.requestFocus())
        ViewCompat.getWindowInsetsController(view)?.show(WindowInsetsCompat.Type.ime())
        waitForStableGeometry(ui, view, requireIme = true)
    }

    private fun awaitStableGeometry(requireIme: Boolean = false) = action("Esperar a que se estabilice la geometría de la ventana") { ui, view ->
        waitForStableGeometry(ui, view, requireIme)
    }

    private fun waitForStableGeometry(ui: UiController, view: View, requireIme: Boolean = false) {
        // Los diálogos pueden recibir varios layouts al cambiar la navegación y el IME.
        val deadline = SystemClock.uptimeMillis() + 15_000
        var previous: List<Int>? = null
        var stableSince = SystemClock.uptimeMillis()
        var settled = false
        var state = "Sin muestras"
        while (!settled && SystemClock.uptimeMillis() < deadline) {
            // Bombea la interfaz; el plazo no sustituye la comprobación de insets y geometría.
            ui.loopMainThreadForAtLeast(16)
            val decor = view.rootView
            val insets = ViewCompat.getRootWindowInsets(decor)
            val safe = insets?.getInsets(WindowInsetsCompat.Type.systemBars() or
                WindowInsetsCompat.Type.ime() or WindowInsetsCompat.Type.displayCutout())
            val origin = IntArray(2)
            decor.getLocationInWindow(origin)
            val screenOrigin = IntArray(2)
            decor.getLocationOnScreen(screenOrigin)
            val visible = visibleInWindow(view)
            val shown = !visible.isEmpty
            val position = IntArray(2)
            view.getLocationInWindow(position)
            val geometry = listOf(origin[0], origin[1], screenOrigin[0], screenOrigin[1], decor.width, decor.height,
                position[0], position[1], view.width, view.height,
                visible.left, visible.top, visible.right, visible.bottom,
                safe?.left ?: -1, safe?.top ?: -1, safe?.right ?: -1, safe?.bottom ?: -1)
            // Un diálogo flotante con ADJUST_RESIZE puede quedar entero sobre
            // el IME y recibir solapamiento cero. La actividad anfitriona sigue
            // recibiendo el área física del teclado; la geometría se mide aquí
            // en la ventana del control, como siempre.
            val host = generateSequence(view.context) { (it as? ContextWrapper)?.baseContext }
                .filterIsInstance<Activity>().firstOrNull()
            val keyboardInsets = host?.window?.decorView?.let(ViewCompat::getRootWindowInsets) ?: insets
            val imeReady = !requireIme || (keyboardInsets != null &&
                keyboardInsets.isVisible(WindowInsetsCompat.Type.ime()) &&
                keyboardInsets.getInsets(WindowInsetsCompat.Type.ime()).bottom > 0)
            if (geometry != previous || !shown || !imeReady || insets == null ||
                view.isLayoutRequested || decor.isLayoutRequested) {
                stableSince = SystemClock.uptimeMillis()
            }
            // El desplazamiento interno del texto (p. ej. el rótulo de button1)
            // no cambia el área pulsable y no debe reiniciar la espera.
            state = "geometría=$geometry, visible=$shown, ime=$imeReady, " +
                "layoutControl=${view.isLayoutRequested}, layoutVentana=${decor.isLayoutRequested}"
            previous = geometry
            settled = SystemClock.uptimeMillis() - stableSince >= 300
        }
        assertTrue("No se han estabilizado los insets y la geometría del control ${view.id}: $state", settled)
        ui.loopMainThreadUntilIdle()
    }

    private fun assertInsideSafeArea(view: View) {
        val decor = view.rootView
        val insets = ViewCompat.getRootWindowInsets(decor)
        assertNotNull("Faltan los insets de la ventana", insets)
        val safe = insets!!.getInsets(WindowInsetsCompat.Type.systemBars() or
            WindowInsetsCompat.Type.ime() or WindowInsetsCompat.Type.displayCutout())
        val origin = IntArray(2)
        decor.getLocationInWindow(origin)
        val area = Rect(origin[0] + safe.left, origin[1] + safe.top,
            origin[0] + decor.width - safe.right, origin[1] + decor.height - safe.bottom)
        view.getLocationInWindow(origin)
        val bounds = Rect(origin[0], origin[1], origin[0] + view.width, origin[1] + view.height)
        val visible = visibleInWindow(view)
        assertTrue("El control ${view.id} no es visible", !visible.isEmpty)
        // Firmar lote queda deshabilitado sin documentos/certificado. Su caja
        // debe seguir visible; el estado habilitado no determina su geometría.
        assertTrue("La vista ${view.id} no es un control", view.isClickable || view.isFocusable)
        val required = Rect(bounds)
        if (view is MaterialButton) {
            // Material incluye un margen transparente en la caja táctil del botón.
            // Solo se permite recortar ese margen, nunca el fondo ni el rótulo.
            required.top += view.insetTop
            required.bottom -= view.insetBottom
        }
        assertTrue("El control ${view.id} no tiene área de interacción", !required.isEmpty)
        assertTrue("El control ${view.id} queda recortado: caja=$bounds, " +
            "área necesaria=$required, visible=$visible", visible.contains(required))
        assertTrue("El control ${view.id} ($required) invade los insets ($area)", area.contains(required))
        assertTrue("El centro pulsable del control ${view.id} queda oculto",
            visible.contains(bounds.centerX(), bounds.centerY()) &&
                area.contains(bounds.centerX(), bounds.centerY()))
    }

    /** Caja visible de la vista, sin su scroll de texto, en su propia ventana. */
    private fun visibleInWindow(view: View): Rect {
        val visible = Rect()
        if (!view.getLocalVisibleRect(visible)) return Rect()
        // getLocalVisibleRect incluye scrollX/Y del propio TextView. Se quitan
        // antes de convertir desde la caja exterior a coordenadas de la ventana.
        visible.offset(-view.scrollX, -view.scrollY)
        if (!visible.intersect(0, 0, view.width, view.height)) return Rect()
        val position = IntArray(2)
        view.getLocationInWindow(position)
        visible.offset(position[0], position[1])
        return visible
    }

    private fun descendants(view: View): List<View> = listOf(view) +
        if (view is ViewGroup) (0 until view.childCount).flatMap { descendants(view.getChildAt(it)) } else emptyList()

    private fun action(description: String, block: (UiController, View) -> Unit): ViewAction = actionWithAssertions(object : ViewAction {
        override fun getConstraints(): Matcher<View> = isAssignableFrom(View::class.java)
        override fun getDescription(): String = description
        override fun perform(uiController: UiController, view: View) = block(uiController, view)
    })

    private class DialogWindowIdler(private val decor: View) : IdlingResource,
        ViewTreeObserver.OnWindowFocusChangeListener {
        @Volatile private var opened = false
        @Volatile private var callback: IdlingResource.ResourceCallback? = null

        init { decor.viewTreeObserver.addOnWindowFocusChangeListener(this) }

        override fun getName(): String = "Apertura del editor del sello"
        override fun isIdleNow(): Boolean = opened
        override fun registerIdleTransitionCallback(callback: IdlingResource.ResourceCallback) {
            this.callback = callback
            if (opened) callback.onTransitionToIdle()
        }
        override fun onWindowFocusChanged(hasFocus: Boolean) {
            if (!hasFocus) {
                opened = true
                callback?.onTransitionToIdle()
            }
        }
        fun close() { decor.viewTreeObserver.removeOnWindowFocusChangeListener(this) }
    }
}
