# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""Lote 39f: los prefijos «Error: » de main.qml pasan por el catálogo.

Antes iban escritos en el código: en alemán la barra de estado decía
«Error: …» y, como no empezaba por «Fehler», ni siquiera se pintaba en rojo.
"""

import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
QML = ROOT / "cmd/gui-qml/qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
LANGUAGES = ("es", "en", "ca", "va", "gl", "eu", "fr", "it", "pt", "de", "zh")


class ErrorPrefixContract(unittest.TestCase):
    def test_no_fixed_error_prefix_outside_the_catalogue(self):
        source = (QML / "main.qml").read_text(encoding="utf-8")
        fixed = [source.count("\n", 0, m.start()) + 1
                 for m in re.finditer(r'(?<!tr\()"Error:\s?"', source)]
        self.assertEqual(fixed, [], "prefijo «Error: » fijo en main.qml, líneas %s" % fixed)
        self.assertGreaterEqual(source.count('tr("Error: ")'), 7)

    def test_prefix_exists_and_keeps_the_error_colour_in_every_language(self):
        # La barra de estado se pinta en rojo si el texto empieza por tr("Error").
        source = (QML / "main.qml").read_text(encoding="utf-8")
        self.assertIn('localizedStatusMessage().startsWith(tr("Error"))', source)
        for language in LANGUAGES:
            catalogue = json.loads((LOCALES / ("%s.json" % language)).read_text(encoding="utf-8"))
            prefix, word = catalogue.get("Error: "), catalogue.get("Error")
            self.assertTrue(prefix and word, language)
            self.assertTrue(prefix.startswith(word), "%s: %r no empieza por %r" % (language, prefix, word))


if __name__ == "__main__":
    unittest.main()
