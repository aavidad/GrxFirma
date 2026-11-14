# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresiones de tamaño de texto, capa modal y catálogos sin pictogramas."""

import json
from pathlib import Path
import re
import unicodedata
import unittest

ROOT = Path(__file__).resolve().parents[3]
QML = ROOT / "cmd/gui-qml/qml"
MAIN = (QML / "main.qml").read_text()
CATALOGS = ROOT / "internal/adapters/outbound/common/localizador/locales"


def pictograma(text):
    return bool(text) and (unicodedata.category(text[0]) == "So" or text[0] in "ℹ⤢")


class TextAccessibility(unittest.TestCase):
    def test_catalog_values_and_qt_translation_keys_have_no_leading_pictogram(self):
        # Las claves antiguas se conservan como alias para clientes de fuera de
        # Qt. Sus valores también deben estar limpios para los lectores.
        for path in CATALOGS.glob("*.json"):
            catalog = json.loads(path.read_text())
            with self.subTest(locale=path.stem):
                self.assertEqual([key for key, value in catalog.items() if pictograma(value)], [])
                self.assertIn("qt.accessibility.text_size", catalog)
                self.assertIn("qt.accessibility.percent", catalog)
        for path in QML.glob("*.qml"):
            for key in re.findall(r'(?:tr|bt|it)\((?:QStringLiteral\()?"([^"\n]*)"', path.read_text()):
                with self.subTest(file=path.name, key=key):
                    self.assertFalse(pictograma(key))

    def test_explicit_ui_font_sizes_come_from_typography(self):
        for path in QML.glob("*.qml"):
            if path.name == "Typography.qml":
                continue
            # La previsualización del sello calcula su letra según el PDF, no
            # según el tamaño de texto de la interfaz.
            self.assertNotRegex(path.read_text(), r"font\.pixelSize:\s*\d+\b", path.name)

    def test_text_preference_uses_the_existing_qsettings_category(self):
        settings = MAIN[MAIN.index("    Settings {"):MAIN.index("// --- Ajustes del Backend ---")]
        self.assertIn('category: "General"', settings)
        self.assertIn("property int textScalePercent: 100", settings)
        self.assertIn('target: Typography; property: "textScalePercent"', settings)
        self.assertIn("appSettings.textScalePercent = Typography.percentages[index]", MAIN)
        self.assertIn("appSettings.sync()", MAIN)

    def test_portal_layer_uses_heading_contrast_controls_and_hides_status(self):
        portal = MAIN[MAIN.index("    ModalLayer {"):MAIN.index("        id: updateBanner")]
        self.assertIn('onEscapePressed: window.portalSealSubmit("cancel")', portal)
        self.assertIn("Accessible.role: Accessible.Heading", portal)
        self.assertIn("initialFocusItem: portalCancelButton", portal)
        self.assertNotRegex(portal, r"(?<!Themed)\b(?:ComboBox|SpinBox|Slider)\s*\{")
        status = MAIN[MAIN.index("        id: statusBar"):]
        self.assertIn("visible: !portalSealMode", status)
        self.assertIn("enabled: !portalSealMode", status)
        self.assertIn("Accessible.ignored: portalSealMode", status)


if __name__ == "__main__":
    unittest.main()
