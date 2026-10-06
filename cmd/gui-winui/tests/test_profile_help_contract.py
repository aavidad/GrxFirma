# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato del botón «?» junto al selector de perfil de firma (WinUI)."""

import json
import pathlib
import unittest
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[1] / "src/GrxFirma.WinUI"
LOCALES = pathlib.Path(__file__).resolve().parents[3] / "internal/adapters/outbound/common/localizador/locales"
NS = "{http://schemas.microsoft.com/winfx/2006/xaml/presentation}"
XKEY = "{http://schemas.microsoft.com/winfx/2006/xaml}Key"
XNAME = "{http://schemas.microsoft.com/winfx/2006/xaml}Name"
TEXT_KEYS = ["titulo", "b", "t", "lt", "lta", "internet"]


CONTROLS = "{using:GrxFirma.WinUI.Controls}"


class ProfileHelpContractTests(unittest.TestCase):
    def setUp(self):
        self.root = ET.parse(ROOT / "Views/SignPage.xaml").getroot()

    def button(self):
        return next(e for e in self.root.iter(CONTROLS + "HelpButton")
                    if e.get(XNAME) == "SignProfileHelpButton")

    def test_button_is_next_to_the_profile_selector(self):
        row = next(g for g in self.root.iter(CONTROLS + "HelpRow") if self.button() in list(g))
        combo = next(c for c in row if c.tag == NS + "ComboBox")
        self.assertEqual(combo.get("AutomationProperties.Name"), "Perfil de firma")
        self.assertEqual(list(row).index(self.button()), 1)

    def test_button_has_accessible_name_and_focus_order(self):
        # El tamaño mínimo y la apertura con teclado los da HelpButton.
        button = self.button()
        self.assertEqual(button.get("AutomationProperties.Name"), "perfil_firma.ayuda.nombre")
        self.assertEqual(button.get("AutomationProperties.HelpText"), "perfil_firma.ayuda.resumen")
        self.assertEqual(button.get("ToolTipService.ToolTip"), "perfil_firma.ayuda.nombre")
        self.assertIsNotNone(button.get("TabIndex"))

    def test_flyout_shows_catalog_texts(self):
        button = self.button()
        self.assertEqual(button.get("HeadingKey"), "perfil_firma.ayuda.titulo")
        self.assertEqual(button.get("HelpKey").split(),
                         ["perfil_firma.ayuda." + k for k in TEXT_KEYS[1:]])

    def test_keys_exist_in_every_catalog(self):
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key in TEXT_KEYS + ["nombre", "resumen"]:
                with self.subTest(locale=path.stem, key=key):
                    self.assertTrue(catalog.get("perfil_firma.ayuda." + key, "").strip())


if __name__ == "__main__":
    unittest.main()
