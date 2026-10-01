#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresiones del contrato seguro de operaciones Qt/QML sobre REST local."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
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


class RestContentContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.source = source(SOURCE)

    def assert_content_only(self, name: str, fields: tuple[str, ...]) -> str:
        body = cpp_function_body(self.source, name)
        self.assertIn("backendReadRegularFile", body)
        self.assertIn("backendRESTRequestAllowed", body)
        for field in fields:
            self.assertIn(f'"{field}"', body)
        request_construction = re.split(
            r"(?:const\s+)?QByteArray\s+jsonData", body, maxsplit=1
        )[0]
        for forbidden_field in (
            "inputPath",
            "outputPath",
            "originalPath",
            "hashPath",
            "reportOutputPath",
        ):
            self.assertNotIn(f'"{forbidden_field}"', request_construction)
        return body

    def test_verify_sends_signed_and_original_content(self) -> None:
        body = self.assert_content_only(
            "BackendBridge::verifyFileWithOriginal",
            ("content_base64", "original_content_base64"),
        )
        self.assertIn('"/verify"', body)

    def test_protection_saves_returned_payload_locally(self) -> None:
        body = self.assert_content_only(
            "BackendBridge::protectFileAdvanced",
            ("content_base64", "protected_content_base64"),
        )
        self.assertIn("backendWritePrivateFile", body)
        self.assertIn("backendProtectionOutputPath", body)
        self.assertIn('body.insert("saveToDisk", false)', body)

    def test_unprotection_saves_returned_payload_locally(self) -> None:
        body = self.assert_content_only(
            "BackendBridge::unprotectFileAdvanced",
            ("content_base64", "unprotected_content_base64"),
        )
        self.assertIn("backendWritePrivateFile", body)
        self.assertIn("backendUnprotectionOutputPath", body)
        self.assertIn('body.insert("saveToDisk", false)', body)

    def test_hash_create_uses_content_and_writes_locally(self) -> None:
        body = self.assert_content_only(
            "BackendBridge::createHash", ("content_base64",)
        )
        self.assertIn('"/hash"', body)
        self.assertIn("backendWritePrivateFile", body)
        self.assertIn("requieren el modo IPC local", body)

    def test_hash_check_sends_both_files_and_rejects_directories(self) -> None:
        body = self.assert_content_only(
            "BackendBridge::checkHash",
            ("content_base64", "hash_content_base64"),
        )
        self.assertIn('"/hash/check"', body)
        self.assertIn("requiere el modo IPC local", body)

    def test_bridge_never_enables_rest_filesystem_paths(self) -> None:
        self.assertNotIn("WithFileSystemPaths", self.source)


if __name__ == "__main__":
    unittest.main()
