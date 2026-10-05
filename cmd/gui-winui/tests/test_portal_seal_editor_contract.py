# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3] / "cmd/gui-winui/src/GrxFirma.WinUI"


class PortalSealEditorContract(unittest.TestCase):
    def test_portal_moves_existing_surface_and_returns_only_explicit_action(self):
        source = (ROOT / "Views/SignPage.xaml.cs").read_text(encoding="utf-8")
        for expected in (
            "VisibleSealEditorPanel.Children.Remove(VisibleSealPreviewViewbox)",
            "content.Children.Add(VisibleSealPreviewViewbox)",
            'session.Submit("place", [placement])',
            'session.Submit("without")',
            'session.Submit("cancel")',
            "ViewModel.PortalSealPlacement()",
            'AddGeometryField(2, "portal.seal.width"',
            'AddGeometryField(4, "portal.seal.rotation"',
        ):
            self.assertIn(expected, source)

    def test_launch_skips_single_instance_and_tray_in_portal_mode(self):
        app = (ROOT / "App.xaml.cs").read_text(encoding="utf-8")
        portal = app.index("if (portalRequested)")
        guard = app.index("_singleInstance = SingleInstanceSignal.Open()")
        self.assertLess(portal, guard)
        self.assertIn("_portalSealSession.CancelOnClose()", app)


    # Regresión 0.0.117: MainWindow entrega la sesión cuando la página ya está
    # cargada, OnLoaded había pasado sin sesión y nadie pedía la vista del PDF;
    # «Firmar con el sello aquí» quedaba desactivado para siempre.
    def test_session_delivered_after_loaded_still_requests_the_pdf_preview(self):
        window = (ROOT / "MainWindow.xaml.cs").read_text(encoding="utf-8")
        opener = window[window.index("internal void OpenPortalSeal"):]
        self.assertIn("page.Loaded += OnLoaded", opener)
        source = (ROOT / "Views/SignPage.xaml.cs").read_text(encoding="utf-8")
        configure = source[source.index("internal void ConfigurePortalSeal"):
                           source.index("private void ConfigurePortalSealLayout")]
        self.assertIn("_ = RefreshPortalSealAsync()", configure)
        self.assertLess(configure.index("ConfigurePortalSealLayout(session)"),
                        configure.index("RefreshPortalSealAsync()"))

    def test_sign_button_follows_the_preview_and_never_hangs(self):
        source = (ROOT / "Views/SignPage.xaml.cs").read_text(encoding="utf-8")
        changed = source[source.index("private void OnViewModelPropertyChanged"):]
        changed = changed[:changed.index("\n    }\n")]
        self.assertIn("_portalUpdateNavigation?.Invoke()", changed)
        refresh = source[source.index("private async Task RefreshPortalSealAsync"):
                         source.index("public SignPageViewModel ViewModel")]
        self.assertIn("PortalSealPreviewWait.WaitAsync(", refresh)
        self.assertIn("PortalSealPreviewTimeout", refresh)
        self.assertIn("_portalPreviewFailed = !ready", refresh)
        self.assertNotIn("FallbackPortalSeal", source)
        self.assertIn('"portal.seal.preview_failed"', source)
        self.assertIn('"portal.seal.preview_loading"', source)
        self.assertIn("AutomationLiveSetting.Assertive", source)
        model = (ROOT / "ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
        confirm = model[model.index("public void ConfirmPreviewImageRendered"):]
        confirm = confirm[:confirm.index("\n    }\n")]
        self.assertIn("RaisePropertyChanged(nameof(CanDrawVisibleSealArea))", confirm)


    # Regresión 0.0.118: al mover el sello quedaba en cola una traducción
    # diferida (LayoutUpdated + 300 ms). Al pulsar «Firmar con el sello aquí»
    # la ventana se cerraba, el despachador ejecutaba esa pasada sobre el árbol
    # destruido (COMException E_UNEXPECTED en un async void) y el proceso
    # terminaba con error aunque result.json ya estuviera escrito.
    def test_deferred_translation_never_touches_a_closed_window(self):
        localizer = (ROOT / "Services/Localizer.cs").read_text(encoding="utf-8")
        attach = localizer[localizer.index("public static DeferredTreePass Attach"):
                           localizer.index("public static void Apply")]
        self.assertIn("gate.IsClosed", attach)
        self.assertIn("gate.TryRun(() => Apply(root))", attach)
        self.assertNotIn("\n                    Apply(root);", attach)
        self.assertIn("return gate;", attach)
        window = (ROOT / "MainWindow.xaml.cs").read_text(encoding="utf-8")
        self.assertIn("_localization = Localizer.Attach(AppRoot);", window)
        closed = window[window.index("private void OnClosed("):]
        closed = closed[:closed.index("\n    }\n")]
        self.assertIn("_localization.Close();", closed)
        app = (ROOT / "App.xaml.cs").read_text(encoding="utf-8")
        handler = app[app.index("private void OnUnhandledException"):
                      app.index("private static void RegistrarErrorNoControlado")]
        self.assertLess(handler.index("e.Handled = true;"),
                        handler.index("Volatile.Read(ref _windowClosed) != 0"))
        self.assertLess(handler.index("Volatile.Read(ref _windowClosed) != 0"),
                        handler.index("_window.ShowUnexpectedError"))

    # Regresión 0.0.118: la salida normal del editor caía con c000000d al
    # descargar Windows.Data.Pdf (dispositivo D3D11 WARP liberado con el grupo
    # de hilos ya cerrado). En modo portal el proceso termina al cerrarse la
    # ventana, después de dejar escrita la decisión.
    def test_portal_editor_terminates_after_writing_the_decision(self):
        app = (ROOT / "App.xaml.cs").read_text(encoding="utf-8")
        closed = app[app.index("private void OnMainWindowClosed"):
                     app.index("private static async Task DisposeClientQuietlyAsync")]
        portal = closed.index("if (_portalSealSession is not null)")
        self.assertLess(closed.index("Interlocked.Exchange(ref _windowClosed, 1)"), portal)
        self.assertLess(portal, closed.index("_portalSealSession.CancelOnClose();"))
        self.assertLess(closed.index("_portalSealSession.CancelOnClose();"),
                        closed.index("ImmediateProcessExit.Terminate(_portalSealSession.ExitCode);"))
        self.assertLess(closed.index("ImmediateProcessExit.Terminate(_portalSealSession"),
                        closed.index("_lifetimeCancellation.Cancel();"))
        exit_source = (ROOT / "Services/ImmediateProcessExit.cs").read_text(encoding="utf-8")
        self.assertIn("TerminateProcess(GetCurrentProcess(), unchecked((uint)exitCode))",
                      exit_source)
        session = (ROOT.parent / "GrxFirma.WinUI.Core/Operations/PortalSealSession.cs").read_text(
            encoding="utf-8")
        self.assertIn("public int ExitCode => IsCompleted ? 0 : 1;", session)


if __name__ == "__main__":
    unittest.main()
