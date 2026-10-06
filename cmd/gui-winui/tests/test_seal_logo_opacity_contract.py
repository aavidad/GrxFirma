# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
from pathlib import Path
import unittest
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[3]
WINUI = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"
CORE = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI.Core"
LOCALES = (
    ROOT / "internal" / "adapters" / "outbound" / "common"
    / "localizador" / "locales"
)


class SealLogoOpacityContractTests(unittest.TestCase):
    def test_slider_is_named_keyboard_accessible_and_shows_percent(self) -> None:
        xaml = ET.parse(WINUI / "Views" / "SignPage.xaml")
        slider = next(
            element for element in xaml.iter()
            if element.tag.endswith("}Slider")
            and "VisibleSealLogoOpacityPercent" in element.get("Value", "")
        )
        self.assertEqual(slider.get("Minimum"), "0")
        self.assertEqual(slider.get("Maximum"), "100")
        self.assertEqual(slider.get("StepFrequency"), "1")
        self.assertEqual(slider.get("TabIndex"), "35")
        self.assertIn("VisibleSealLogoOpacityLabel", slider.get("Header", ""))
        self.assertIn(
            "VisibleSealLogoOpacityLabel",
            slider.get("AutomationProperties.Name", ""),
        )
        self.assertIn(
            "VisibleSealOpacityHelp",
            slider.get("AutomationProperties.HelpText", ""),
        )
        self.assertIn("VisibleSealEnabled", ET.tostring(xaml.getroot(), encoding="unicode"))

    def test_preview_and_sign_share_explicit_opacity(self) -> None:
        vm = (WINUI / "ViewModels" / "SignPageViewModel.cs").read_text(encoding="utf-8")
        contract = (
            CORE / "Operations" / "DesktopOperationContracts.cs"
        ).read_text(encoding="utf-8")
        self.assertIn('JsonPropertyName("logoOpacityPercent")', contract)
        self.assertEqual(
            vm.count("LogoOpacityPercent = (int)VisibleSealLogoOpacityPercent"),
            2,
        )
        self.assertIn('"visibleSealLogoOpacityPercent"', vm)
        self.assertIn("ScheduleSealStampPreview();", vm)

    def test_preference_is_loaded_saved_and_localized(self) -> None:
        page = (WINUI / "Views" / "SignPage.xaml.cs").read_text(encoding="utf-8")
        settings = (
            CORE / "Operations" / "DesktopSettingsContracts.cs"
        ).read_text(encoding="utf-8")
        project = (WINUI / "GrxFirma.WinUI.csproj").read_text(encoding="utf-8")
        self.assertIn('JsonPropertyName("signSealLogoOpacityPercent")', settings)
        self.assertIn("result.Data.SealLogoOpacityPercent", page)
        self.assertIn("SealLogoOpacityPercent = value", page)
        self.assertIn("SealUiCatalog.LogoOpacityLabel", page)
        self.assertIn("locales\\*.json", project)
        for locale in ("ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"):
            with self.subTest(locale=locale):
                catalog = json.loads((LOCALES / f"{locale}.json").read_text(encoding="utf-8"))
                self.assertTrue(catalog["sign.seal.opacity"].strip())
                self.assertTrue(catalog["sign.seal.opacity_help"].strip())

    def test_rotated_card_keeps_its_unrotated_size(self) -> None:
        xaml = (WINUI / "Views" / "SignPage.xaml").read_text(encoding="utf-8")
        vm = (WINUI / "ViewModels" / "SignPageViewModel.cs").read_text(encoding="utf-8")
        page = (WINUI / "Views" / "SignPage.xaml.cs").read_text(encoding="utf-8")
        self.assertIn('<Border.RenderTransform>', xaml)
        self.assertIn('Angle="{x:Bind ViewModel.VisibleSealPreviewRotation, Mode=OneWay}"', xaml)
        self.assertIn('Rotation = 0, // El editor gira la tarjeta completa.', vm)
        self.assertIn('ConstrainVisibleSealCardToPage();', vm)
        self.assertIn('VisibleSealPreviewWidth - 16', vm)
        self.assertIn('dx * Math.Cos(radians) + dy * Math.Sin(radians)', page)


if __name__ == "__main__":
    unittest.main()
