# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"


class SealDrawContract(unittest.TestCase):
    def test_accessible_toggle_and_keyboard_target_use_catalogs(self):
        source = (UI / "Views/SignPage.xaml").read_text(encoding="utf-8")
        xaml = ET.fromstring(source)
        name = "{http://schemas.microsoft.com/winfx/2006/xaml}Name"
        for control_name in ("VisibleSealDrawToggle", "VisibleSealDrawInput"):
            control = next(c for c in xaml.iter() if c.get(name) == control_name)
            self.assertIn("ViewModel.VisibleSealDrawAreaLabel", control.get("AutomationProperties.Name"))
            self.assertIn("ViewModel.VisibleSealDrawAreaHelp", control.get("AutomationProperties.HelpText"))
            self.assertEqual("OnVisibleSealDrawKeyDown", control.get("KeyDown"))
        self.assertIn('AutomationProperties.LiveSetting="Polite"', source)

    def test_pointer_preview_does_not_apply_geometry_before_release(self):
        source = (UI / "Views/SignPage.xaml.cs").read_text(encoding="utf-8")
        pressed = source[source.index("private void OnVisibleSealDrawPointerPressed"):source.index("private Point NormalizeSealDrawPoint")]
        self.assertIn("CapturePointer(args.Pointer)", pressed)
        self.assertNotIn("ApplyDrawnSealArea", pressed)
        moved = source[source.index("if (_visibleSealPointerMode == VisibleSealPointerMode.Draw)", source.index("private void OnVisibleSealPreviewPointerMoved")):]
        moved = moved[:moved.index("var canvasWidth")]
        self.assertIn("UpdateSealDrawRubberBand()", moved)
        self.assertNotIn("ApplyDrawnSealArea", moved)
        for expected in ("VirtualKey.Escape", "VirtualKey.Enter", "IsShiftPressed()",
                         "SealDrawGeometry.Normalize", "AutomationEvents.LiveRegionChanged"):
            self.assertIn(expected, source)

    def test_portal_moves_controls_only_after_loaded(self):
        source = (UI / "Views/SignPage.xaml.cs").read_text(encoding="utf-8")
        configure = source[source.index("internal void ConfigurePortalSeal"):source.index("private void ConfigurePortalSealLayout")]
        self.assertIn("if (IsLoaded) ConfigurePortalSealLayout(session)", configure)
        self.assertNotIn("Children.Remove", configure)
        loaded = source[source.index("private async void OnLoaded"):source.index("private void OnUnloaded")]
        self.assertIn("ConfigurePortalSealLayout(_portalSealSession)", loaded)
        self.assertIn("content.Children.Add(VisibleSealDrawControls)", source)

    def test_commit_validates_then_updates_geometry_once_and_adds_page(self):
        source = (UI / "ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
        method = source[source.index("public bool ApplyDrawnSealArea"):source.index("public string VisibleSealRotateHandleLabel")]
        self.assertLess(method.index("SealDrawGeometry.IsLargeEnough"), method.index("_visibleSealXPercent ="))
        self.assertEqual(1, method.count("OnVisibleSealGeometryChanged();"))
        self.assertIn("_sealPlacements[_previewCurrentPage] = CurrentSealPlacement();", method)
        self.assertNotIn("_visibleSealRotationDegrees =", method)


if __name__ == "__main__":
    unittest.main()
