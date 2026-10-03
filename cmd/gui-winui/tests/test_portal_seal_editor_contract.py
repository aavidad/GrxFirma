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


if __name__ == "__main__":
    unittest.main()
