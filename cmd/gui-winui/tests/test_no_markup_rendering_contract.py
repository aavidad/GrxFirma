# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""LD-01 en WinUI: ningún control interpreta marcado.

TextBlock pinta siempre texto plano, así que un nombre de firmante o de
certificado con etiquetas se ve tal cual. Esta prueba falla si alguien añade un
control que sí interpreta marcado o contenido web (RichTextBlock, RichEditBox,
WebView2, XamlReader.Load) sin revisar antes qué datos le llegan.
WebView2 aparece en packages.lock.json solo como dependencia de Windows App
SDK; lo que se comprueba es que el código no lo use.
"""

import re
import unittest
from pathlib import Path

UI = Path(__file__).resolve().parents[1] / "src" / "GrxFirma.WinUI"

PROHIBIDOS = re.compile(
    r"\b(RichTextBlock|RichEditBox|WebView2|WebView|XamlReader|NavigateToString|"
    r"CoreWebView2|SetText\s*\(\s*TextSetOptions\.FormatRtf)\b"
)


class NoMarkupRenderingContractTests(unittest.TestCase):
    def test_no_rich_text_or_web_controls(self) -> None:
        ficheros = sorted(
            p for p in UI.rglob("*")
            if p.suffix in {".xaml", ".cs"} and "obj" not in p.parts and "bin" not in p.parts
        )
        self.assertGreater(len(ficheros), 20, "no se encuentran las fuentes de WinUI")
        for fichero in ficheros:
            texto = fichero.read_text(encoding="utf-8")
            for numero, linea in enumerate(texto.splitlines(), start=1):
                with self.subTest(fichero=fichero.name, linea=numero):
                    self.assertIsNone(PROHIBIDOS.search(linea), linea.strip())


if __name__ == "__main__":
    unittest.main()
