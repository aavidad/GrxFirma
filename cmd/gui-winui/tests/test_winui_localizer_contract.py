# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
import pathlib
import re
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
VISIBLE = {
    "Text", "Content", "Header", "PlaceholderText", "Title", "Message",
    "Description", "PrimaryButtonText", "SecondaryButtonText",
    "CloseButtonText", "ToolTip", "Name", "HelpText",
}


class WinUiLocalizerContractTests(unittest.TestCase):
    def test_visible_xaml_property_types_are_observed(self):
        implementation = (UI / "Services/Localizer.cs").read_text(encoding="utf-8")
        for path in UI.rglob("*.xaml"):
            for element in ET.parse(path).getroot().iter():
                for raw_name, value in element.attrib.items():
                    name = raw_name.rsplit("}", 1)[-1]
                    if name.startswith("AutomationProperties."):
                        name = name.split(".", 1)[1]
                    if name not in VISIBLE or not re.search(r"[A-Za-zÀ-ÿ]", value):
                        continue
                    if value.startswith("{") or value == "GrxFirma":
                        continue
                    with self.subTest(path=path.name, property=name, text=value[:60]):
                        if name == "ToolTip":
                            self.assertIn("ToolTipService.ToolTipProperty", implementation)
                        elif name in {"Name", "HelpText"}:
                            self.assertIn(f"AutomationProperties.{name}Property", implementation)
                        else:
                            self.assertIn(f'"{name}"', implementation)

    def test_dynamic_dialogs_pass_through_localizer(self):
        for path in UI.rglob("*.cs"):
            if path.name == "Localizer.cs":
                continue
            source = path.read_text(encoding="utf-8")
            with self.subTest(path=str(path.relative_to(UI))):
                self.assertNotRegex(source, r"\b(?:dialog|confirmation)\.ShowAsync\(\)")

    def test_selected_language_refreshes_page_and_menu(self):
        window = (UI / "MainWindow.xaml.cs").read_text(encoding="utf-8")
        settings = (UI / "Views/SettingsPage.xaml.cs").read_text(encoding="utf-8")
        catalog = (UI / "Services/SealUiCatalog.cs").read_text(encoding="utf-8")
        self.assertIn("ApplyLanguagePreference(result.Data.Language)", window)
        self.assertIn("RootNavigation.FooterMenuItems", window)
        self.assertIn("ContentFrame.Navigate(pageType)", window)
        self.assertIn("_app.RefreshTrayLanguage()", window)
        self.assertIn("app.ApplyLanguagePreference(ViewModel.SelectedLanguage.Value)", settings)
        self.assertIn("Localizer.Text(key)", catalog)
        self.assertNotIn("se muestra actualmente en español", (UI / "Views/SettingsPage.xaml").read_text(encoding="utf-8"))

    def test_explicit_localizer_keys_exist_in_every_catalog(self):
        used = set()
        for path in UI.rglob("*.cs"):
            if path.name == "Localizer.cs":
                continue
            used.update(re.findall(r'Localizer\.Text\("([^"\n]+)"\)', path.read_text(encoding="utf-8")))
        self.assertTrue(used)
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            with self.subTest(locale=path.stem):
                self.assertFalse(used - catalog.keys(), sorted(used - catalog.keys()))


if __name__ == "__main__":
    unittest.main()
