#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato de los botones «?» de ayuda contextual en Qt.

Cada clave ayuda.* que docs/AYUDA_CONTEXTUAL.md asigna a Qt debe usarse en
algún QML; las claves usadas deben existir en los once catálogos, y cada
HelpButton debe tener nombre accesible.
"""

import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd/gui-qml"
QML_DIR = QT / "qml"
DOC = ROOT / "docs/AYUDA_CONTEXTUAL.md"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"

# «?» mínimos por fichero (docs/AYUDA_CONTEXTUAL.md, «Reparto de la fase 2»).
# main.qml reúne Firmar 19, Verificar y huellas 10, Preferencias 20 (más el
# del perfil Baseline), Certificados 2 (en las dos barras de certificados) y
# Proteger 7; el panel PKCS#11 completa las 21 de Preferencias.
MIN_BUTTONS = {
    "main.qml": 19 + 10 + 20 + 2 + 7,
    "TokenSettingsPanel.qml": 1,
    "EniPanel.qml": 5,
    "FacturaePanel.qml": 3,
}


def qt_keys_from_doc() -> set[str]:
    keys = set()
    for line in DOC.read_text(encoding="utf-8").splitlines():
        cells = [c.strip() for c in line.split("|")]
        if len(cells) < 8:
            continue
        match = re.fullmatch(r"`(ayuda\.[a-z0-9_.]+)`", cells[2])
        if not match:
            continue
        if cells[4].lower().startswith("no aparece"):
            continue
        keys.add(match.group(1))
    return keys


def qml_sources() -> dict[str, str]:
    return {p.name: p.read_text(encoding="utf-8") for p in sorted(QML_DIR.glob("*.qml"))}


def help_button_blocks(text: str, type_name: str = "HelpButton") -> list[str]:
    """Usos «HelpButton { … }» con sus llaves equilibradas (sin las
    declaraciones «component X: HelpButton»)."""
    blocks = []
    for match in re.finditer(r"\b%s\s*\{" % type_name, text):
        line_start = text.rfind("\n", 0, match.start()) + 1
        if "component " in text[line_start:match.start()]:
            continue
        depth = 0
        for i in range(match.end() - 1, len(text)):
            if text[i] == "{":
                depth += 1
            elif text[i] == "}":
                depth -= 1
                if depth == 0:
                    blocks.append(text[match.start():i + 1])
                    break
    return blocks


class ContextualHelpContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.sources = qml_sources()
        self.all_qml = "\n".join(self.sources.values())

    def test_doc_assigns_keys_to_qt(self) -> None:
        keys = qt_keys_from_doc()
        self.assertGreaterEqual(len(keys), 60)
        self.assertNotIn("ayuda.validar_al_terminar", keys, "en Qt no aparece")
        self.assertNotIn("ayuda.dnie_nfc", keys)

    def test_every_qt_key_is_used_in_some_qml(self) -> None:
        missing = sorted(k for k in qt_keys_from_doc() if f'"{k}"' not in self.all_qml)
        self.assertEqual(missing, [], "claves de Qt sin «?»")

    def test_used_keys_exist_in_every_catalog(self) -> None:
        used = set(re.findall(r'"(ayuda\.[a-z0-9_.]+)"', self.all_qml))
        self.assertIn("ayuda.boton_nombre", used)
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key in sorted(used):
                with self.subTest(locale=path.stem, key=key):
                    self.assertTrue(str(catalog.get(key, "")).strip())
                    if key == "ayuda.boton_nombre":
                        self.assertIn("{0}", catalog[key])

    def test_help_button_component_is_accessible_and_packaged(self) -> None:
        self.assertIn("<file>qml/HelpButton.qml</file>", (QT / "qml.qrc").read_text(encoding="utf-8"))
        component = self.sources["HelpButton.qml"]
        self.assertIn("Accessible.name: effectiveName", component)
        self.assertIn("Accessible.description", component)
        self.assertIn('nameTemplate.replace("{0}"', component)
        self.assertIn("Keys.onEscapePressed", component)
        self.assertIn("Keys.onReturnPressed", component)
        self.assertIn("Keys.onEnterPressed", component)
        self.assertIn("Contrast.readableOn", component)
        self.assertIn("margins:", component, "el texto no debe salirse de ventanas estrechas")

    def test_every_help_button_has_a_name_and_a_text(self) -> None:
        for name, text in self.sources.items():
            types = ["HelpButton"] + re.findall(r"component (\w+): HelpButton", text)
            for block in [b for t in types for b in help_button_blocks(text, t)]:
                with self.subTest(file=name, block=block[:90]):
                    if block.startswith("HelpButton"):
                        self.assertRegex(block, r"\b(nameTemplate|accessibleName):")
                    self.assertRegex(block, r"\b(helpText|paragraphs):")
            # Los componentes locales que derivan de HelpButton fijan el nombre.
            for block in re.findall(r"component \w+: HelpButton \{[^}]*\}", text):
                self.assertIn("nameTemplate", block)

    def test_minimum_number_of_help_buttons_per_file(self) -> None:
        for name, minimum in MIN_BUTTONS.items():
            text = self.sources[name]
            local = re.findall(r"component (\w+): HelpButton", text)
            count = len(re.findall(r"\bHelpButton\s*\{", text))
            for alias in local:
                count += len(re.findall(r"\b%s\s*\{" % alias, text))
            with self.subTest(file=name):
                self.assertGreaterEqual(count, minimum)

    def test_option_help_keys_cover_every_sign_option(self) -> None:
        main = self.sources["main.qml"]
        for fn, values in {
            "signActionHelpKey": ["cosign", "countersign"],
            "signFormatHelpKey": ["pades", "cades", "xades", "xmldsig", "odf", "ooxml", "facturae", "asic-xades", "verifactu"],
        }.items():
            body = main.split(f"function {fn}(", 1)[1].split("\n    }\n", 1)[0]
            for value in values:
                with self.subTest(function=fn, value=value):
                    self.assertIn(f'case "{value}": return "ayuda.', body)


if __name__ == "__main__":
    unittest.main()
