#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato de UX segura para la clave transitoria CMS EncryptedData."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
REST_CPP = (ROOT / "cmd/gui-qml/backendbridge.cpp").read_text(encoding="utf-8")
REST_H = (ROOT / "cmd/gui-qml/backendbridge.h").read_text(encoding="utf-8")
IPC_CPP = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
IPC_H = (ROOT / "cmd/gui-qml/ipcbridge.h").read_text(encoding="utf-8")
IPC_SERVER = (
    ROOT / "internal/adapters/inbound/desktop/ipc/server.go"
).read_text(encoding="utf-8")
IPC_HANDLER = (
    ROOT / "internal/adapters/inbound/desktop/ipc/handler.go"
).read_text(encoding="utf-8")


def cpp_function_body(text: str, qualified_name: str) -> str:
    match = re.search(
        rf"\b(?:void|bool)\s+{re.escape(qualified_name)}\s*\([^)]*\)\s*\{{",
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


class EncryptedDataTransientSecretContractTest(unittest.TestCase):
    def test_qml_exposes_encrypted_data_without_recipients_or_signing(self) -> None:
        self.assertIn(
            '{ texto: tr("CMS EncryptedData (.encrypted.p7m)"), '
            'valor: "cms-encrypted" }',
            QML,
        )
        self.assertIn('return ".encrypted.p7m"', QML)
        self.assertIn('lower.endsWith(".encrypted.p7m")', QML)
        self.assertIn('window.protectContainer !== "cms-encrypted"', QML)
        self.assertIn("return []", QML)

    def test_qml_secret_fields_are_bounded_masked_and_non_persistent(self) -> None:
        for field_id in (
            "protectEncryptedSecretField",
            "protectEncryptedSecretConfirmField",
            "unprotectEncryptedSecretField",
        ):
            self.assertIn(f"id: {field_id}", QML)
        self.assertGreaterEqual(QML.count("maximumLength: 44"), 3)
        self.assertGreaterEqual(QML.count("echoMode: TextInput.Password"), 3)
        self.assertGreaterEqual(QML.count("Qt.ImhSensitiveData"), 3)
        self.assertNotRegex(
            QML,
            r"property\s+string\s+\w*(?:secret|claveTransitoria)\w*\s*:",
        )
        self.assertNotIn("secret_b64:", QML)
        self.assertIn(
            r"/^[A-Za-z0-9+\/]{42}[AEIMQUYcgkosw048]=$/.test", QML
        )

    def test_qml_clears_on_dispatch_lifecycle_and_explicit_cancel(self) -> None:
        self.assertIn("backend.protectEncryptedDataFile(", QML)
        self.assertIn("backend.unprotectEncryptedDataFile(", QML)
        self.assertGreaterEqual(QML.count("clearTransientProtectionSecrets()"), 8)
        self.assertGreaterEqual(QML.count("protectEncryptedSecretField.clear()"), 3)
        self.assertGreaterEqual(
            QML.count("protectEncryptedSecretConfirmField.clear()"), 3
        )
        self.assertGreaterEqual(
            QML.count("unprotectEncryptedSecretField.clear()"), 3
        )
        self.assertIn("onClosing: function(close)", QML)
        self.assertIn("onRejected: window.clearTransientProtectionSecrets()", QML)

    def test_generic_p7m_requires_explicit_opt_in(self) -> None:
        self.assertIn("id: unprotectEncryptedDataToggle", QML)
        self.assertIn("window.isGenericCMSPath(window.unprotectInputPath)", QML)
        self.assertIn("window.usesTransientUnprotectionSecret()", QML)

    def test_bridges_validate_and_zero_owned_copies(self) -> None:
        for header in (REST_H, IPC_H):
            self.assertIn("Q_INVOKABLE void protectEncryptedDataFile", header)
            self.assertIn("Q_INVOKABLE void unprotectEncryptedDataFile", header)
        for implementation, prefix in (
            (REST_CPP, "BackendBridge"),
            (IPC_CPP, "IpcBridge"),
        ):
            for suffix in (
                "protectEncryptedDataFile",
                "unprotectEncryptedDataFile",
            ):
                body = cpp_function_body(
                    implementation, f"{prefix}::{suffix}"
                )
                self.assertIn(
                    "TransientSecret::isCanonicalAES256Base64", body
                )
                self.assertIn("TransientSecret::zeroize(secretB64)", body)
            protect_body = cpp_function_body(
                implementation, f"{prefix}::protectEncryptedDataFile"
            )
            self.assertIn(
                "TransientSecret::normalizedEncryptedDataOutputPath(outputPath)",
                protect_body,
            )

        rest_protect = cpp_function_body(
            REST_CPP, "BackendBridge::protectFileAdvanced"
        )
        rest_unprotect = cpp_function_body(
            REST_CPP, "BackendBridge::unprotectFileAdvanced"
        )
        for body in (rest_protect, rest_unprotect):
            self.assertIn(
                'body.insert(QStringLiteral("secret_b64"), secretB64)', body
            )
            self.assertIn("TransientSecret::zeroize(jsonData)", body)
            self.assertNotRegex(body, r"\[[^\]]*\boptions\b[^\]]*\]\s*\(")

    def test_rest_requests_never_automatically_follow_redirects(self) -> None:
        start = REST_CPP.index("static QNetworkRequest BackendBridgeMakeReq(")
        start = REST_CPP.index(
            "static QNetworkRequest BackendBridgeMakeReq(", start + 1
        )
        end = REST_CPP.index("void BackendBridge::getServiceStatus()", start)
        make_request = REST_CPP[start:end]
        self.assertIn(
            "QNetworkRequest::RedirectPolicyAttribute", make_request
        )
        self.assertIn("QNetworkRequest::ManualRedirectPolicy", make_request)

    def test_ipc_never_logs_or_defers_the_transient_secret(self) -> None:
        self.assertIn("ipcHasTransientProtectionSecret(params)", IPC_CPP)
        queue = cpp_function_body(IPC_CPP, "IpcBridge::queueDeferredRequest")
        self.assertIn("ipcHasTransientProtectionSecret(params)", queue)
        self.assertIn("return false", queue)
        self.assertIn("IncidentFormatIpcLogEvent(", IPC_CPP)
        self.assertNotIn("ipcRedactJsonValue", IPC_CPP)
        self.assertNotIn("ipcJsonForLog", IPC_CPP)
        self.assertIn('"protect", "protect_sign", "unprotect"', IPC_SERVER)
        self.assertIn("secmem.Zeroize(p.Params)", IPC_SERVER)

    def test_ipc_preserves_compound_cms_suffixes_safely(self) -> None:
        self.assertIn('".encrypted.p7m"', IPC_HANDLER)
        self.assertIn('".signedenveloped.p7m"', IPC_HANDLER)
        self.assertIn("filepath.Base(strings.TrimSpace(documentName))", IPC_HANDLER)


if __name__ == "__main__":
    unittest.main()
