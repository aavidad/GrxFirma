#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""KD-01: el motor da la fecha de un sello de tiempo que no ha podido
comprobar con su propio origen (``timestamp_unverified``). Qt la enseña con
ese origen, en todas las vistas, y nunca como «según el sello de tiempo»."""

import shutil
import subprocess
import unittest
from pathlib import Path

from test_recorrido_118_contract import qml_function

ROOT = Path(__file__).resolve().parents[3]
QML_DIR = ROOT / "cmd/gui-qml/qml"
MAIN = QML_DIR / "main.qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
DOMAIN = ROOT / "internal/domain/verification.go"


class SigningTimeUnverifiedContract(unittest.TestCase):
    def test_qt_conoce_las_tres_fuentes_del_motor(self):
        domain = DOMAIN.read_text(encoding="utf-8")
        for value in ('"timestamp"', '"timestamp_unverified"', '"signed_attribute"'):
            self.assertIn(value, domain)
        signers = (QML_DIR / "VerificationSigners.js").read_text(encoding="utf-8")
        self.assertIn('"timestamp_unverified"', signers)
        entries = qml_function(MAIN.read_text(encoding="utf-8"), "verificationSignerEntries")
        self.assertIn("report.signer.time_from_unverified_timestamp", entries)

    def test_fecha_de_sello_sin_comprobar_lleva_su_origen(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        source = MAIN.read_text(encoding="utf-8")
        module = (QML_DIR / "VerificationSigners.js").read_text(encoding="utf-8").replace(".pragma library", "")
        names = ("catalogFormat", "verificationSignerEntries", "verificationSignerLabelLine",
                 "verificationSignerSummariesText", "verificationSignerShortText")
        js = "const catalog = " + (LOCALES / "es.json").read_text(encoding="utf-8") + ";\n"
        js += "function tr(key) { return catalog[key] !== undefined ? catalog[key] : key; }\n"
        js += "String.prototype.arg = function(v) { let done = false; return this.replace(/%[1-9]/, m => { if (done) return m; done = true; return String(v); }); };\n"
        js += "const Signers = (function() {\n" + module + "\nreturn {parseDistinguishedName, readableName, readableIssuer, signingTimeText};\n})();\n"
        js += "\n".join(qml_function(source, name).strip() for name in names)
        js += r"""
const assert = require('assert');
function details(source) {
  return {signerSummaries: [{subject: "CN=Ana Prueba,O=Pruebas,C=ES", issuer: "CN=AC PRUEBAS",
    signingTime: "2026-10-05T08:30:00Z", signingTimeSource: source}]};
}
const unverified = details("timestamp_unverified");
const readable = verificationSignerSummariesText(unverified);
assert.ok(/Fecha de la firma: 0[45]\/10\/2026 \d\d:30:00 \(UTC[^)]*\), fecha que declara un sello de tiempo que no se ha podido comprobar/.test(readable), readable);
assert.ok(!readable.includes("según el sello de tiempo"), readable);
// El resumen breve tampoco la enseña como una fecha comprobada.
const short = verificationSignerShortText(unverified);
assert.ok(short.startsWith("Ana Prueba, el "), short);
assert.ok(short.includes("fecha que declara un sello de tiempo que no se ha podido comprobar"), short);
// Las otras dos fuentes no cambian.
assert.ok(verificationSignerSummariesText(details("timestamp")).includes(", según el sello de tiempo"));
assert.ok(verificationSignerSummariesText(details("signed_attribute")).includes(", hora del equipo de quien firmó"));
assert.ok(!verificationSignerShortText(details("timestamp")).includes("sello de tiempo que no"));
// Una fuente desconocida sigue sin fecha.
assert.strictEqual(verificationSignerEntries(details("pdf_m"))[0].date, "");
"""
        subprocess.run([node], input=js, text=True, check=True, cwd=ROOT, timeout=15)


if __name__ == "__main__":
    unittest.main()
