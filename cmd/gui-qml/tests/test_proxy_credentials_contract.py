# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
BRIDGE_CPP = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
BRIDGE_H = (ROOT / "cmd/gui-qml/ipcbridge.h").read_text(encoding="utf-8")
HANDLER = (
    ROOT / "internal/adapters/inbound/desktop/ipc/handler.go"
).read_text(encoding="utf-8")
SERVER = (
    ROOT / "internal/adapters/inbound/desktop/ipc/server.go"
).read_text(encoding="utf-8")


class ProxyCredentialsContractTest(unittest.TestCase):
    def test_qml_uses_password_mode_and_clears_the_field(self) -> None:
        self.assertIn("id: proxyPasswordField", QML)
        self.assertIn("echoMode: TextInput.Password", QML)
        self.assertIn("maximumLength: 4096", QML)
        self.assertIn("backend.storeProxyCredentials(", QML)
        self.assertIn("backend.deleteProxyCredentials()", QML)
        self.assertGreaterEqual(QML.count("proxyPasswordField.clear()"), 3)

    def test_visual_security_editor_does_not_claim_backend_enforcement(self) -> None:
        self.assertNotIn("property string securityAccessPassword", QML)
        self.assertNotIn("property bool secureConnections", QML)
        self.assertNotIn("property var secureDomainsList", QML)
        self.assertNotIn("securityAccessPassword: window.securityAccessPassword", QML)
        self.assertNotIn("s.securityAccessPassword", QML)
        self.assertIn(
            "Esta pantalla es informativa. La confianza web no se configura aquí",
            QML,
        )

    def test_sensitive_action_is_never_queued_or_logged_unredacted(self) -> None:
        self.assertIn(
            'action == QStringLiteral("proxy_secret_store")', BRIDGE_CPP
        )
        self.assertIn(
            "IncidentFormatIpcLogEvent(", BRIDGE_CPP
        )
        self.assertNotIn("ipcRedactJsonValue", BRIDGE_CPP)
        self.assertNotIn("ipcJsonForLog", BRIDGE_CPP)
        self.assertIn("passwordBytes.fill('\\0')", BRIDGE_CPP)
        self.assertIn("encodedPassword.fill('\\0')", BRIDGE_CPP)
        self.assertIn("params.clear()", BRIDGE_CPP)

    def test_bridge_exposes_only_store_delete_and_redacted_result(self) -> None:
        self.assertIn("Q_INVOKABLE void storeProxyCredentials", BRIDGE_H)
        self.assertIn("Q_INVOKABLE void deleteProxyCredentials", BRIDGE_H)
        self.assertIn("void proxyCredentialsFinished(", BRIDGE_H)
        self.assertNotIn("proxySecretId", BRIDGE_H)

    def test_backend_updates_only_typed_reference_transactionally(self) -> None:
        self.assertIn('case "proxy_secret_store":', HANDLER)
        self.assertIn('case "proxy_secret_delete":', HANDLER)
        self.assertIn("next.Proxy.SecretID = stringPtrIPC(newID)", HANDLER)
        self.assertIn("next.Proxy.Realm = stringPtrIPC(realm)", HANDLER)
        self.assertIn("m.saveSettingsDocument(ctx, next)", HANDLER)
        self.assertIn("m.ProxySecrets.Delete(ctx, oldID)", HANDLER)
        self.assertIn("m.ProxySecrets.Delete(ctx, newID)", HANDLER)

    def test_server_zeroes_scanner_buffer_for_sensitive_actions(self) -> None:
        self.assertIn("isSensitiveIPCAction(p.Action)", SERVER)
        self.assertIn("secmem.Zeroize(linea)", SERVER)
        self.assertIn('"proxy_secret_store"', SERVER)


if __name__ == "__main__":
    unittest.main()
