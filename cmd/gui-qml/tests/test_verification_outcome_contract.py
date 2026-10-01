#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresiones del resultado visual seguro de verificación en Qt/QML."""

import json
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = ROOT / "cmd/gui-qml/qml/main.qml"
LOCALES = (
    ROOT / "internal/adapters/outbound/common/localizador/locales"
)


def source(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def qml_function_body(text: str, name: str) -> str:
    match = re.search(rf"function\s+{re.escape(name)}\s*\([^)]*\)\s*\{{", text)
    if not match:
        raise AssertionError(f"función QML ausente: {name}")
    depth = 1
    cursor = match.end()
    while cursor < len(text) and depth:
        if text[cursor] == "{":
            depth += 1
        elif text[cursor] == "}":
            depth -= 1
        cursor += 1
    if depth:
        raise AssertionError(f"función QML sin cierre: {name}")
    return text[match.end() : cursor - 1]


class VerificationOutcomeContractTest(unittest.TestCase):
    def test_green_requires_valid_result_and_valid_trust(self) -> None:
        qml = source(QML)
        body = qml_function_body(qml, "verificationOutcomeKind")
        self.assertIn('details.valid === true', body)
        self.assertIn('integrity === "valid"', body)
        self.assertIn('certificate === "valid"', body)
        self.assertIn('trust === "valid"', body)
        self.assertIn('return "trusted"', body)

    def test_invalid_aspects_are_red_and_unknown_trust_is_amber(self) -> None:
        qml = source(QML)
        kind = qml_function_body(qml, "verificationOutcomeKind")
        color = qml_function_body(qml, "verificationOutcomeColor")
        self.assertIn('details.valid === false', kind)
        self.assertIn('certificate === "invalid"', kind)
        self.assertIn('trust === "invalid"', kind)
        self.assertIn('trust === "unknown"', kind)
        self.assertIn('trust === "warning"', kind)
        self.assertIn('return "#e74c3c"', color)
        self.assertIn('return "#f39c12"', color)

    def test_manual_single_and_batch_flows_use_the_classification(self) -> None:
        qml = source(QML)
        payload = qml_function_body(qml, "verificationPayload")
        auto_message = qml_function_body(qml, "verificationAutoMessage")
        single = qml_function_body(qml, "applySingleAutoVerificationResult")
        batch = qml_function_body(qml, "applyBatchAutoVerificationResult")
        summary = qml_function_body(qml, "refreshBatchVerificationSummary")
        self.assertIn("Object.assign({}, details)", payload)
        self.assertIn("payload.valid = false", payload)
        self.assertIn("verificationAutoMessage", single)
        self.assertIn("verificationOutcomeKind", auto_message)
        self.assertNotIn(".valid", single)
        self.assertIn("item.verifyOutcome = verificationOutcomeKind", batch)
        self.assertNotIn("item.verifyDetails.valid", batch)
        self.assertIn('outcome === "invalid"', summary)
        self.assertIn('outcome !== "trusted"', summary)
        self.assertIn("confianza no evaluada", summary)
        self.assertIn(
            "verificationOutcomeDisplay(verifyTab.verifyDetails)",
            qml,
        )
        self.assertNotIn(
            "(verifyTab.verifyDetails && verifyTab.verifyDetails.valid)",
            qml,
        )
        self.assertRegex(
            qml,
            r"modelData\.verifyDone\s*"
            r"\?\s*verificationOutcomeColor\(modelData\.verifyDetails\)\s*"
            r":\s*\"#f39c12\"",
        )

    def test_new_visible_outcomes_are_localized_in_every_locale(self) -> None:
        required = {
            "Válida y confiable",
            "No válida",
            "Integridad válida; confianza no evaluada",
            "Verificación incompleta",
            "Integridad válida; confianza no evaluada.",
            "Firma completada; integridad válida, confianza no evaluada.",
            (
                "Lote firmado. Integridad válida, pero confianza no evaluada "
                "en algunos resultados."
            ),
        }
        locale_paths = sorted(LOCALES.glob("*.json"))
        self.assertEqual(11, len(locale_paths))
        for path in locale_paths:
            with self.subTest(locale=path.stem):
                data = json.loads(source(path))
                self.assertTrue(required.issubset(data))
                self.assertTrue(all(data[key].strip() for key in required))


if __name__ == "__main__":
    unittest.main()
