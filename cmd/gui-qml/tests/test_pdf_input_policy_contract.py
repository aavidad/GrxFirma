#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresión: ninguna interfaz ofrece firmar un PDF estructuralmente inválido."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
SURFACES = (
    ROOT / "cmd/gui-qml/qml/main.qml",
    ROOT / "cmd/gui-qml/backendbridge.cpp",
    ROOT / "cmd/gui-qml/ipcbridge.cpp",
    ROOT / "internal/adapters/inbound/common/rest/websigner.html.tmpl",
)


class PDFInputPolicyContractTest(unittest.TestCase):
    def test_product_surfaces_do_not_offer_or_send_legacy_override(self) -> None:
        for path in SURFACES:
            with self.subTest(path=path.relative_to(ROOT)):
                content = path.read_text(encoding="utf-8")
                self.assertNotIn("allowInvalidPDF", content)
                self.assertNotIn("signAllowInvalidPDF", content)
                self.assertNotIn("Permitir PDF inválido", content)


if __name__ == "__main__":
    unittest.main()
