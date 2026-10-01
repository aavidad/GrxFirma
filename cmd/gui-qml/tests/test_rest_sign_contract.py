#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresiones del contrato de firma Qt/QML sobre REST local."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
HEADER = ROOT / "cmd/gui-qml/backendbridge.h"
SOURCE = ROOT / "cmd/gui-qml/backendbridge.cpp"


def source(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def cpp_function_body(text: str, qualified_name: str) -> str:
    match = re.search(
        rf"\bvoid\s+{re.escape(qualified_name)}\s*\([^)]*\)\s*\{{",
        text,
        re.DOTALL,
    )
    if not match:
        raise AssertionError(f"función C++ ausente: {qualified_name}")
    depth = 1
    cursor = match.end()
    while cursor < len(text) and depth:
        if text[cursor] == "{":
            depth += 1
        elif text[cursor] == "}":
            depth -= 1
        cursor += 1
    if depth:
        raise AssertionError(f"función C++ sin cierre: {qualified_name}")
    return text[match.end() : cursor - 1]


class RestSignContractTest(unittest.TestCase):
    def test_bridge_exposes_same_advanced_signing_surface_as_ipc(self) -> None:
        header = source(HEADER)
        self.assertIn("signFileMultiAdvanced(", header)
        self.assertIn("signBatchAdvanced(", header)
        self.assertIn("batchSigningFinished(bool success", header)

    def test_single_sign_sends_content_and_saves_locally(self) -> None:
        body = cpp_function_body(source(SOURCE), "BackendBridge::signFileAdvanced")
        self.assertIn('"content_base64"', body)
        self.assertNotIn('body.insert("inputPath"', body)
        self.assertNotIn('body.insert("outputPath"', body)
        self.assertIn("kBackendMaxRESTRequestBytes", body)
        self.assertIn("backendWritePrivateFile", body)
        self.assertIn('"signed_content_base64"', body)

    def test_batch_sign_uses_content_mode_and_bounded_request(self) -> None:
        body = cpp_function_body(source(SOURCE), "BackendBridge::signBatchAdvanced")
        self.assertIn('"/sign-batch"', body)
        self.assertIn('"content_base64"', body)
        self.assertIn("kBackendMaxRESTRequestBytes", body)
        self.assertIn("backendWritePrivateFile", body)
        self.assertIn('"signed_content_base64"', body)
        self.assertNotIn('item.insert(QStringLiteral("inputPath")', body)
        self.assertNotIn('item.insert(QStringLiteral("outputPath")', body)


if __name__ == "__main__":
    unittest.main()
