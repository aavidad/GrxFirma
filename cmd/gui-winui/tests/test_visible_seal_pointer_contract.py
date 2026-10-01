# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
XAML = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
    / "Views"
    / "SignPage.xaml"
)
CODE_BEHIND = XAML.with_suffix(".xaml.cs")


class VisibleSealPointerContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.xaml = XAML.read_text(encoding="utf-8")
        cls.code = CODE_BEHIND.read_text(encoding="utf-8")

    def test_preview_exposes_move_and_resize_pointer_surfaces(self) -> None:
        for contract in (
            'x:Name="VisibleSealPreviewSurface"',
            'x:Name="VisibleSealPreviewRegion"',
            'x:Name="VisibleSealResizeHandle"',
            'PointerPressed="OnVisibleSealMovePointerPressed"',
            'PointerPressed="OnVisibleSealResizePointerPressed"',
            'PointerMoved="OnVisibleSealPreviewPointerMoved"',
            'PointerReleased="OnVisibleSealPreviewPointerReleased"',
            "Arrastre la zona azul",
            "alternativa precisa mediante teclado",
        ):
            self.assertIn(contract, self.xaml)

    def test_pointer_geometry_is_normalized_and_bounded(self) -> None:
        for contract in (
            "VisibleSealPointerMode.Move",
            "VisibleSealPointerMode.Resize",
            "CapturePointer(args.Pointer)",
            "ReleasePointerCapture(args.Pointer)",
            "100 - _visibleSealStartWidthPercent",
            "100 - _visibleSealStartHeightPercent",
            "const double minimumSizePercent = 2",
            "RoundPreviewPercent",
            "HasValidVisibleSealPreviewGeometry",
        ):
            self.assertIn(contract, self.code)

    def test_dragging_keeps_the_numeric_accessible_model_authoritative(
        self,
    ) -> None:
        for property_name in (
            "VisibleSealXPercent",
            "VisibleSealYPercent",
            "VisibleSealWidthPercent",
            "VisibleSealHeightPercent",
        ):
            self.assertIn(
                f"ViewModel.{property_name} =",
                self.code,
            )

    def test_rotation_handle_has_capture_keyboard_and_preview_limit(self) -> None:
        view_model = (
            ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/SignPageViewModel.cs"
        ).read_text(encoding="utf-8")
        for expected in (
            '<controls:SealRotateButton',
            'x:Name="VisibleSealRotateHandle"',
            'Width="44"',
            'Height="44"',
            'KeyDown="OnVisibleSealRotateKeyDown"',
        ):
            self.assertIn(expected, self.xaml)
        for expected in (
            "VisibleSealPointerMode.Rotate",
            "new PointerEventHandler(OnVisibleSealRotatePointerPressed), true",
            "captureTarget.CapturePointer(args.Pointer)",
            "Math.Atan2(",
            "Math.Abs(degrees - nearest) <= 4",
            "IsShiftPressed() ? 15 : 1",
            "ViewModel.EndVisibleSealRotation()",
        ):
            self.assertIn(expected, self.code)
        self.assertIn("if (!_rotatingVisibleSeal)", view_model)
        self.assertIn("SaveCurrentSealPlacement();", view_model)


if __name__ == "__main__":
    unittest.main()
