# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Firma remota CSC en Qt: el motor guarda la sesión y los secretos no se quedan."""

import json
import re
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd" / "gui-qml" / "qml" / "main.qml").read_text(encoding="utf-8")
BRIDGE = (ROOT / "cmd" / "gui-qml" / "ipcbridge.cpp").read_text(encoding="utf-8")
HEADER = (ROOT / "cmd" / "gui-qml" / "ipcbridge.h").read_text(encoding="utf-8")
LOCALES = ROOT / "internal" / "adapters" / "outbound" / "common" / "localizador" / "locales"


def block(source, marker):
    start = source.index(marker)
    if marker.startswith("id: "):
        start = source.rindex("ThemedDialog {", 0, start)
    depth = 0
    for index in range(source.index("{", start), len(source)):
        depth += (source[index] == "{") - (source[index] == "}")
        if depth == 0:
            return source[start:index + 1]
    raise AssertionError(marker)


class CscRemoteContractTests(unittest.TestCase):
    def test_bridge_exposes_only_csc_actions(self) -> None:
        for name in ("cscStatus", "cscConfigure", "cscConnect", "cscDisconnect", "cscSendOtp"):
            self.assertIn("Q_INVOKABLE void " + name, HEADER)
        for action in ("csc_status", "csc_configure", "csc_connect", "csc_disconnect", "csc_send_otp"):
            self.assertIn('"' + action + '"', BRIDGE)
        self.assertIn("void cscFinished(QString action, bool ok, QVariantMap data, QString message);", HEADER)

    def test_remote_secrets_are_base64_wiped_and_never_queued(self) -> None:
        move = block(BRIDGE, "static void ipcMoveRemoteSigningSecrets")
        self.assertIn("toBase64()", move)
        self.assertEqual(3, move.count("TransientSecret::zeroize("))
        queue = block(BRIDGE, "bool IpcBridge::queueDeferredRequest")
        self.assertIn("ipcHasRemoteSigningSecret(params)", queue)
        self.assertIn('action.startsWith(QStringLiteral("csc_"))', queue)
        for function in ("signFileAdvanced", "signFileMultiAdvanced", "signBatchAdvanced"):
            body = block(BRIDGE, "void IpcBridge::" + function + "(")
            self.assertIn("ipcMoveRemoteSigningSecrets(params, options);", body)
            self.assertIn("ipcForgetRemoteSigningSecrets(params);", body)

    def test_secret_dialog_uses_password_fields_and_clears_them(self) -> None:
        dialog = block(QML, "id: cscRemoteSecretDialog")
        self.assertEqual(2, dialog.count("echoMode: TextInput.Password"))
        self.assertIn("window.clearRemoteSecretFields()", block(dialog, "onClosed:"))
        self.assertIn("window.cscPendingSecrets = null", block(QML, "function attachRemoteSecrets("))
        self.assertIn("window.cscPendingSecrets = null", block(QML, "function onSigningFinished("))
        self.assertIn("window.cscPendingSecrets = null", block(QML, "function onBatchSigningFinished("))
        self.assertNotRegex(dialog, r"console\.log")

    def test_protect_and_sign_asks_for_remote_secrets_like_sign(self) -> None:
        protect = block(QML, "function executeProtectSignRequest(")
        self.assertIn("window.certificateNeedsRemoteSecrets(cert) && window.cscPendingSecrets === null", protect)
        self.assertIn('window.openRemoteSecretDialog(index, cert, "protect")', protect)
        self.assertIn("String(window.cscPendingSecrets.certificateId) !== String(cert.id", protect)
        self.assertLess(protect.index("window.attachRemoteSecrets(payload)"), protect.index("backend.protectFileAdvanced("))
        submit = block(QML, "function submitSecrets(")
        self.assertIn('if (purpose === "protect")', submit)
        self.assertIn("window.executeProtectSignRequest()", submit)
        self.assertIn("onClicked: window.executeProtectSignRequest()", QML)
        self.assertIn("window.cscPendingSecrets = null", block(QML, "function onProtectionFinished("))
        bridge = block(BRIDGE, "void IpcBridge::protectFileAdvanced(")
        guarded = block(bridge, "if (signToo) {")
        self.assertIn("ipcMoveRemoteSigningSecrets(params, options);", guarded)
        self.assertIn("ipcForgetRemoteSigningSecrets(params);", bridge)
        self.assertLess(bridge.index("ipcForgetRemoteSigningSecrets(params);"), bridge.index("params.clear();"))

    def test_hosts_are_shown_before_connecting_and_panel_follows_engine(self) -> None:
        dialog = block(QML, "id: cscRemoteDialog")
        self.assertLess(dialog.index("csc.gui.host_servicio"), dialog.index("backend.cscConnect()"))
        self.assertIn("csc.gui.host_oauth", dialog)
        self.assertIn("textFormat: Text.PlainText", dialog)
        self.assertIn("visible: (window.cscAllowed || window.cscProhibited) && !window.rightSidebarCollapsed", QML)
        self.assertIn("window.cscAllowed = ok && data.allowed === true", QML)

    def test_policy_prohibition_is_explained_without_offering_config(self) -> None:
        self.assertIn("window.cscProhibited = ok && data.prohibitedByPolicy === true", QML)
        dialog = block(QML, "id: cscRemoteDialog")
        self.assertIn('visible: window.cscProhibited\n                text: tr("csc.error.prohibida")', dialog)
        self.assertIn("id: cscServiceUrlField\n                visible: window.cscAllowed", dialog)
        self.assertIn("id: cscClientIdField\n                visible: window.cscAllowed", dialog)

    def test_batch_with_otp_needs_multisign_and_must_fit(self) -> None:
        check = block(QML, "function remoteBatchOtpBlockKey(")
        self.assertIn("Number(certificate.remoteMultiSign || 1)", check)
        self.assertIn("window.multiCosignEnabled || !(capacity > 1)", check)
        self.assertIn("currentBatchPaths.length > capacity", check)
        self.assertIn('return "csc.error.otp_lote_excede"', check)
        self.assertIn("window.remoteBatchOtpBlockKey(remoteCertificate)", QML)
        self.assertIn('tr("csc.error.otp_lote_excede")', QML)

    def test_every_csc_text_key_is_in_the_catalog(self) -> None:
        catalog = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
        keys = set(re.findall(r'tr\("(csc\.[a-z_.]+)"\)', QML))
        self.assertTrue(keys)
        self.assertEqual(set(), keys - set(catalog))


if __name__ == "__main__":
    unittest.main()
