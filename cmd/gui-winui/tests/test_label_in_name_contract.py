# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""WCAG 2.5.3: el nombre accesible contiene el texto visible.

Quien usa control por voz dice lo que ve («Examinar»); si el nombre era
«Seleccionar documento para firmar», la orden no funcionaba (recorrido
Windows 0.0.117, M10). La explicación pasa a AutomationProperties.HelpText.
"""

import re
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

UI = Path(__file__).resolve().parents[1] / "src" / "GrxFirma.WinUI"
# Controles del sello visible que comparte el editor de portales: se revisan
# junto con ese editor para no pisar su trabajo en curso.
PENDING_SEAL_CONTROLS = {
    "Añadir sello visible en PDF", "Estampar en", "Páginas concretas",
    "X (%)", "Y (%)", "Giro",
}


def normalize(text: str) -> str:
    text = re.sub(r"\((?:opcional|persona física|%|HTTPS)\)", "", text)
    for mark in ("…", "*", "⤢", "+ "):
        text = text.replace(mark, "")
    text = re.sub(r"[,.:]", "", text)
    return re.sub(r"\s+", " ", text).strip().lower()


class LabelInNameContractTests(unittest.TestCase):
    def test_accessible_names_contain_the_visible_label(self) -> None:
        missing = []
        for path in sorted((UI / "Views").glob("*.xaml")) + sorted((UI / "Controls").glob("*.xaml")):
            for element in ET.parse(path).getroot().iter():
                name = element.get("AutomationProperties.Name")
                if not name or name.startswith("{"):
                    continue
                visible = element.get("Content") or element.get("Header")
                if visible is None:
                    texts = [child.get("Text") for child in element.iter()
                             if child.tag.endswith("}TextBlock") and child.get("Text")]
                    visible = texts[0] if texts and element.tag.endswith("}Button") else None
                if not visible or visible.startswith("{") or visible in PENDING_SEAL_CONTROLS:
                    continue
                if normalize(visible) not in normalize(name):
                    missing.append((path.name, visible, name))
        self.assertFalse(missing, missing[:20])


if __name__ == "__main__":
    unittest.main()
