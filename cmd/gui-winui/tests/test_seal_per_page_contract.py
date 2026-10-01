# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
from pathlib import Path
import unittest
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[3]
WINUI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


class SealPerPageContractTest(unittest.TestCase):
    def test_ipc_and_editor_share_per_page_list(self):
        contract = (CORE / "Operations/DesktopOperationContracts.cs").read_text()
        settings = (CORE / "Operations/DesktopSettingsContracts.cs").read_text()
        vm = (WINUI / "ViewModels/SignPageViewModel.cs").read_text()
        view = ET.parse(WINUI / "Views/SignPage.xaml")
        events = (WINUI / "Views/SignPage.xaml.cs").read_text()
        self.assertIn('JsonPropertyName("placements")', contract)
        self.assertIn('JsonPropertyName("rect")', contract)
        self.assertIn('JsonPropertyName("signSealPlacements")', settings)
        self.assertIn('SealPlacements = NormalizeSealPlacements(SealPlacements)', settings)
        self.assertIn("Placements = _perPageSealEnabled", vm)
        self.assertIn("_sealPlacements.Remove(_previewCurrentPage)", vm)
        self.assertIn("NavigateVisibleSealPageAsync", events)
        self.assertTrue(any("ApplySealToAllPages" in value for el in view.iter() for value in el.attrib.values()))
        self.assertTrue(any("HasSealOnPreviewPage" in value for el in view.iter() for value in el.attrib.values()))

    def test_qr_field_is_hidden_and_url_is_normalized(self):
        vm = (WINUI / "ViewModels/SignPageViewModel.cs").read_text()
        view = ET.parse(WINUI / "Views/SignPage.xaml")
        field = next(el for el in view.iter() if el.attrib.get("AutomationProperties.Name") == "Dirección de verificación del QR")
        self.assertIn("VisibleSealQrEnabled", field.attrib["Visibility"])
        self.assertIn('value = "https://" + value', vm)
        self.assertIn("uri.Scheme", vm)

    def test_labels_exist_in_eleven_catalogs(self):
        keys = ("sign.seal.one_by_one", "sign.seal.apply_all_pages", "sign.seal.remove_this_page",
                "sign.seal.add_this_page", "sign.seal.qr_https_error", "sign.seal.page_limit")
        for language in ("ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"):
            with self.subTest(language=language):
                catalog = json.loads((LOCALES / f"{language}.json").read_text())
                for key in keys:
                    self.assertTrue(catalog[key].strip())


if __name__ == "__main__":
    unittest.main()
