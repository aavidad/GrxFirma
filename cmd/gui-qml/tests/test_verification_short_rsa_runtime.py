#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""La firma con clave RSA corta sin excepción sale en Qt como no válida y en rojo.

Ejecuta con Node las funciones reales de main.qml que deciden el veredicto
y el color de la verificación (orden de Alberto, 16/11/2026).
"""

import json
from pathlib import Path
import shutil
import subprocess
import unittest

from test_verification_outcome_contract import qml_function_body


ROOT = Path(__file__).resolve().parents[3]
QML = ROOT / "cmd/gui-qml/qml/main.qml"
FUNCTIONS = (
    "verificationAspectStatus",
    "verificationForcedTrustApplied",
    "verificationOutcomeKind",
    "verificationOutcomeLabel",
    "verificationOutcomeColor",
)

# Lo que entrega el motor por IPC para un firmante con clave RSA de 1024 bits
# sin la excepción clave_rsa_corta (signer/cades_verify.go).
SHORT_RSA = {
    "valid": False,
    "reason": "clave RSA del firmante de menos de 2048 bits",
    "integrity": {"status": "valid"},
    "certificate": {
        "status": "invalid",
        "reason": "clave RSA del firmante de menos de 2048 bits",
        "details": ["subject=Firmante sintético", "rsa_bits=1024"],
    },
    "trust": {"status": "valid"},
    "evidence": [],
}

# Con la excepción aceptada el motor da la firma por válida y lo deja a la
# vista con la evidencia trust.user_forced.
SHORT_RSA_ACCEPTED = {
    "valid": True,
    "integrity": {"status": "valid"},
    "certificate": {"status": "valid", "details": ["rsa_bits=1024"]},
    "trust": {"status": "valid"},
    "evidence": [{"type": "trust.user_forced", "summary": "clave_rsa_corta"}],
}


def evaluate(details: dict) -> dict:
    node = shutil.which("node")
    if not node:
        raise unittest.SkipTest("Node no instalado")
    qml = QML.read_text(encoding="utf-8")
    functions = "\n".join(
        f"function {name}(details, name) {{{qml_function_body(qml, name)}}}"
        if name == "verificationAspectStatus"
        else f"function {name}(details) {{{qml_function_body(qml, name)}}}"
        for name in FUNCTIONS
    )
    script = (
        "function tr(text) { return text }\n"
        + functions
        + "\nconst details = JSON.parse(process.argv[1]);\n"
        + "process.stdout.write(JSON.stringify({"
        + "kind: verificationOutcomeKind(details),"
        + "label: verificationOutcomeLabel(details),"
        + "color: verificationOutcomeColor(details)}));\n"
    )
    out = subprocess.run(
        [node, "-e", script, json.dumps(details)],
        check=True,
        capture_output=True,
        text=True,
        cwd=ROOT,
    )
    return json.loads(out.stdout)


class ShortRsaVerificationRuntimeTest(unittest.TestCase):
    def test_short_rsa_without_exception_is_red_and_invalid(self) -> None:
        outcome = evaluate(SHORT_RSA)
        self.assertEqual("invalid", outcome["kind"])
        self.assertEqual("No válida", outcome["label"])
        self.assertEqual("#e74c3c", outcome["color"])

    def test_short_rsa_with_accepted_exception_is_not_red(self) -> None:
        outcome = evaluate(SHORT_RSA_ACCEPTED)
        self.assertEqual("forced", outcome["kind"])
        self.assertNotEqual("#e74c3c", outcome["color"])


if __name__ == "__main__":
    unittest.main()
