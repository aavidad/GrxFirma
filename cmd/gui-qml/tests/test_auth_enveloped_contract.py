#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato Qt/QML del perfil CMS AuthEnvelopedData seguro."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = ROOT / "cmd/gui-qml/qml/main.qml"
REST_BRIDGE = ROOT / "cmd/gui-qml/backendbridge.cpp"
IPC_BRIDGE = ROOT / "cmd/gui-qml/ipcbridge.cpp"


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


class AuthEnvelopedQtContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.qml = source(QML)

    def test_qml_exposes_only_the_canonical_authenticated_container(self) -> None:
        self.assertIn(
            '{ texto: tr("CMS AuthEnvelopedData (.authenveloped.p7m)"), '
            'valor: "authenvelopeddata" }',
            self.qml,
        )
        self.assertIn("AES-256-GCM y RSA-OAEP-SHA256/MGF1-SHA256", self.qml)
        self.assertIn('return ".authenveloped.p7m"', self.qml)

    def test_qml_filters_recipients_using_backend_capability(self) -> None:
        self.assertIn("recipient.authEnvelopedDataCompatible === true", self.qml)
        self.assertIn(
            "No hay destinatarios compatibles con AuthEnvelopedData", self.qml
        )
        self.assertIn(
            'window.protectContainer !== "authenvelopeddata"', self.qml
        )

    def test_unprotection_detects_auth_enveloped_suffix(self) -> None:
        self.assertIn('lower.endsWith(".authenveloped.p7m")', self.qml)
        self.assertIn(
            'tr("Contenedor detectado: %1").arg('
            "window.protectedContainerLabel(window.unprotectInputPath))",
            self.qml,
        )

    def test_bridges_preserve_nested_protection_options(self) -> None:
        rest_body = cpp_function_body(
            source(REST_BRIDGE), "BackendBridge::protectFileAdvanced"
        )
        ipc_body = cpp_function_body(
            source(IPC_BRIDGE), "IpcBridge::protectFileAdvanced"
        )
        for body in (rest_body, ipc_body):
            self.assertIn('options.value("options").toMap()', body)
            self.assertIn('"options"', body)
        self.assertNotIn('"signedandenvelopeddata"', rest_body)
        self.assertNotIn('"signedandenvelopeddata"', ipc_body)


if __name__ == "__main__":
    unittest.main()
