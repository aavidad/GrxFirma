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

if __name__ == "__main__":
    unittest.main()
