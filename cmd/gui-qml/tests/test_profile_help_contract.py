#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato del botón «?» junto al perfil de firma por defecto (Qt)."""

import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd/gui-qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
KEYS = ["nombre", "titulo", "b", "t", "lt", "lta", "internet"]


class ProfileHelpContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.main = (QT / "qml/main.qml").read_text(encoding="utf-8")

    def help_block(self) -> str:
        start = self.main.index("id: settingsSignProfileCombo")
        block = self.main[start:].split("HelpButton {", 1)[1]
        return block.split("emphasizeLast", 1)[0]

    def test_help_button_sits_next_to_the_profile_selector(self) -> None:
        start = self.main.index("id: settingsSignProfileCombo")
        between = self.main[start:self.main.index("HelpButton {", start)]
        self.assertNotIn("ComboBox {", between, "el «?» debe ir justo tras el selector")
        self.assertIn('accessibleName: tr("perfil_firma.ayuda.nombre")', self.help_block())

    def test_every_paragraph_comes_from_the_catalog(self) -> None:
        block = self.help_block()
        used = re.findall(r'tr\("perfil_firma\.ayuda\.([a-z]+)"\)', block)
        self.assertEqual(used, KEYS)
        self.assertNotRegex(block, r'tr\("(?!perfil_firma\.ayuda\.)')

    def test_keys_exist_in_every_catalog(self) -> None:
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key in KEYS + ["resumen"]:
                with self.subTest(locale=path.stem, key=key):
                    self.assertTrue(catalog.get("perfil_firma.ayuda." + key, "").strip())
        spanish = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
        self.assertEqual(spanish["perfil_firma.ayuda.nombre"], "Ayuda sobre el perfil de firma")
        self.assertIn("recomendado", spanish["perfil_firma.ayuda.t"])
        self.assertIn("Internet", spanish["perfil_firma.ayuda.internet"])

    def test_component_is_packaged_and_accessible(self) -> None:
        self.assertIn("<file>qml/HelpButton.qml</file>", (QT / "qml.qrc").read_text(encoding="utf-8"))
        component = (QT / "qml/HelpButton.qml").read_text(encoding="utf-8")
        self.assertIn("Accessible.name: accessibleName", component)
        self.assertIn("Accessible.description", component)
        self.assertIn("Qt.TabFocusReason", component)
        self.assertIn("Keys.onEscapePressed", component)


if __name__ == "__main__":
    unittest.main()
