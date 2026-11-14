#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 16d: los avisos del motor al firmar (cofirma de una CAdES separada
sin el original, KD-03, y firma LT con una CRL que no cubre al firmante,
14-5) llegan por IPC en «warnings» y Qt los enseña tras firmar, traducidos
con el catálogo del idioma de la persona."""

import json
import re
import shutil
import subprocess
import unittest
from pathlib import Path

from test_recorrido_118_contract import qml_function

ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd/gui-qml"
MAIN = (QT / "qml/main.qml").read_text(encoding="utf-8")
BOX = QT / "qml/SigningWarningsBox.qml"
BRIDGE = (QT / "ipcbridge.cpp").read_text(encoding="utf-8")
HEADER = (QT / "ipcbridge.h").read_text(encoding="utf-8")
REST_HEADER = (QT / "backendbridge.h").read_text(encoding="utf-8")
QRC = (QT / "qml.qrc").read_text(encoding="utf-8")
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
ENGINE_KEYS = ("firma.cofirma_sin_original.aviso", "firma.lt.crl_no_aplicable.aviso")


def cpp_block(source, marker):
    start = source.index(marker)
    depth = 0
    for index in range(source.index("{", start), len(source)):
        depth += (source[index] == "{") - (source[index] == "}")
        if depth == 0:
            return source[start:index + 1]
    raise AssertionError(marker)


class SignWarningsContract(unittest.TestCase):
    def test_los_dos_puentes_exponen_los_avisos(self):
        self.assertIn("Q_INVOKABLE QStringList lastSigningWarnings() const", HEADER)
        # El puente REST también los recibe (lote 17b, 3d).
        self.assertIn("Q_INVOKABLE QStringList lastSigningWarnings() const", REST_HEADER)

    def test_el_puente_ipc_lee_warnings_del_resultado(self):
        for function in ("signFileAdvanced", "signFileMultiAdvanced", "signBatchAdvanced"):
            body = cpp_block(BRIDGE, "void IpcBridge::" + function + "(")
            self.assertLess(body.index("m_lastSigningWarnings.clear();"), body.index("sendRequest("))
        batch = BRIDGE[BRIDGE.index('if (action == "sign_batch") {\n      QJsonObject res'):]
        batch = batch[:batch.index("continue;")]
        self.assertLess(batch.index("ipcSigningWarnings(res)"), batch.index("emit batchSigningFinished("))
        single = BRIDGE[BRIDGE.index('if (res.contains("OutputPath")) {'):]
        single = single[:single.index("} else if")]
        self.assertLess(single.index("ipcSigningWarnings(res)"), single.index("emit signingFinished(true"))
        helper = cpp_block(BRIDGE, "static QStringList ipcSigningWarnings(")
        self.assertIn('value(QStringLiteral("warnings"))', helper)
        # Solo claves del catálogo, con tope: nada de texto libre del motor.
        self.assertIn("QRegularExpression", helper)
        self.assertIn("kMaxSigningWarnings", helper)

    def test_qml_los_ensena_tras_firmar(self):
        single = qml_function(MAIN, "onSigningFinished")
        self.assertIn("window.signResultWarnings = success ? window.visibleSigningWarnings(backend.lastSigningWarnings()) : []", single)
        batch = qml_function(MAIN, "onBatchSigningFinished")
        self.assertIn("window.batchSigningWarnings = window.visibleSigningWarnings(backend.lastSigningWarnings())", batch)
        self.assertIn("signResultWarnings = []", qml_function(MAIN, "clearSignResult"))
        self.assertIn("warnings: window.signResultKind === \"success\" ? window.signResultWarnings : []", MAIN)
        self.assertIn("warnings: window.batchSigningWarnings", MAIN)
        self.assertIn("signResultWarnings", qml_function(MAIN, "signResultAnnouncement"))
        self.assertIn("<file>qml/SigningWarningsBox.qml</file>", QRC)
        box = BOX.read_text(encoding="utf-8")
        self.assertEqual(box.count("Text {"), box.count("textFormat: Text.PlainText"))
        self.assertIn("Accessible.role: Accessible.AlertMessage", box)

    def test_las_claves_del_motor_estan_en_los_catalogos(self):
        for language in ("es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh"):
            catalog = json.loads((LOCALES / (language + ".json")).read_text(encoding="utf-8"))
            for key in ENGINE_KEYS:
                self.assertTrue(catalog.get(key), (language, key))
            self.assertTrue(catalog.get("Advertencias"), language)

    def test_solo_se_ensenan_claves_conocidas_y_traducidas(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        js = "const catalog = " + (LOCALES / "en.json").read_text(encoding="utf-8") + ";\n"
        js += "function tr(key) { return catalog[key] !== undefined ? catalog[key] : key; }\n"
        js += "const window = { tr: tr };\n"
        js += qml_function(MAIN, "visibleSigningWarnings").strip() + "\n"
        js += r"""
const assert = require('assert');
const shown = visibleSigningWarnings(["firma.lt.crl_no_aplicable.aviso", "clave.que.no.existe",
  "<img src=http://x>", "firma.lt.crl_no_aplicable.aviso", 7, null]);
assert.deepStrictEqual(shown, [catalog["firma.lt.crl_no_aplicable.aviso"]]);
assert.ok(shown[0].startsWith("The revocation list"), shown[0]);
assert.deepStrictEqual(visibleSigningWarnings(undefined), []);
assert.deepStrictEqual(visibleSigningWarnings("firma.lt.crl_no_aplicable.aviso"), []);
"""
        subprocess.run([node], input=js, text=True, check=True, cwd=ROOT, timeout=15)


if __name__ == "__main__":
    unittest.main()
