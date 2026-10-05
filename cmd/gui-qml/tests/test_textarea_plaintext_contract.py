# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Las áreas de texto (también ThemedTextArea) que muestran datos externos no
interpretan HTML."""

from pathlib import Path
import re
import unittest

QML = Path(__file__).resolve().parents[1] / "qml"


def bloques_textarea(texto):
    for coincidencia in re.finditer(r"\b(?:Themed)?TextArea\s*\{", texto):
        inicio = coincidencia.end()
        nivel, i = 1, inicio
        while nivel and i < len(texto):
            nivel += {"{": 1, "}": -1}.get(texto[i], 0)
            i += 1
        bloque = texto[inicio:i - 1]
        # Solo las propiedades propias, no las de elementos hijos.
        propio = re.sub(r"\{[^{}]*\}", "", bloque)
        yield texto.count("\n", 0, coincidencia.start()) + 1, propio


class TextAreaPlainTextContract(unittest.TestCase):
    def test_textareas_declaran_texto_plano(self):
        for fichero in sorted(QML.rglob("*.qml")):
            texto = fichero.read_text(encoding="utf-8")
            for linea, bloque in bloques_textarea(texto):
                with self.subTest(fichero=fichero.name, linea=linea):
                    formatos = re.findall(r"textFormat\s*:\s*([\w.]+)", bloque)
                    self.assertEqual(len(formatos), 1, "falta textFormat o está repetido")
                    if "releaseNotesDialog.notesText" in bloque:
                        # Novedades: Markdown del propio paquete instalado.
                        self.assertEqual(formatos[0], "TextEdit.MarkdownText")
                    else:
                        self.assertEqual(formatos[0], "TextEdit.PlainText")

    def test_facturae_informe_y_qr_en_texto_plano(self):
        panel = (QML / "FacturaePanel.qml").read_text(encoding="utf-8")
        qr = panel.split("id: qrArea", 1)[1].split("}", 1)[0]
        self.assertIn("textFormat: TextEdit.PlainText", qr)
        informe = panel.split('objectName: "verifactuReport"', 1)[1].split("}", 1)[0]
        self.assertIn("textFormat: TextEdit.PlainText", informe)


if __name__ == "__main__":
    unittest.main()
