# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
MAIN = (ROOT / "cmd/gui-qml/main.cpp").read_text(encoding="utf-8")
IPC = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
BRIDGES = [
    (ROOT / "cmd/gui-qml/backendbridge.cpp").read_text(encoding="utf-8"),
    (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8"),
]


class IncidentPrivacyContractTest(unittest.TestCase):
    def test_both_bridges_enforce_same_privacy_boundary(self):
        for source in BRIDGES:
            self.assertIn("IncidentSanitizePayload(payload, false)", source)
            self.assertIn("IncidentHasExplicitRemoteConsent(report)", source)
            self.assertIn("IncidentLoadEligibleSavedReport(", source)
            self.assertIn("IncidentSanitizePayload(storedPayload, true)", source)
            self.assertIn("IncidentRecordRemoteAttempt(", source)
            self.assertIn("QNetworkRequest::ManualRedirectPolicy", source)
            self.assertIn("IncidentWritePrivateFile(targetPath", source)
            self.assertNotIn("saveIncidentReport(safePayload", source)
            self.assertNotIn("Enviando incidencia por HTTPS a:", source)

    def test_qml_operation_context_contains_names_not_paths_or_certificate_ids(self):
        match = re.search(
            r"function currentOperationContextForIncident\(\) \{(?P<body>.*?)\n    \}",
            QML,
            re.DOTALL,
        )
        self.assertIsNotNone(match)
        body = match.group("body")
        for forbidden in (
            "inputPath:",
            "outputPath:",
            "verifyFilePath:",
            "selectedCertificateId:",
            "selectedCertificateLabel:",
        ):
            self.assertNotIn(forbidden, body)
        self.assertIn("inputFileName: basename(currentFilePath)", body)
        self.assertIn("certificateSelected:", body)

    def test_remote_send_marks_consent_in_payload(self):
        self.assertIn(
            "payload.supportPreview.consent.remoteSend = true",
            QML,
        )
        self.assertIn("backend.sendIncidentReport(payload,", QML)
        self.assertIn("supportIncidentSendDialog.sourceIncidentPath)", QML)

    def test_remote_action_requires_a_saved_detected_failure(self):
        self.assertIn(
            'incidentPath: String(incidentPath || "")',
            QML,
        )
        actions = re.findall(
            r'Button \{\s*text: tr\("Enviar incidencia"\)'
            r"(.*?)\n\s*\}",
            QML,
            re.DOTALL,
        )
        self.assertTrue(actions)
        self.assertTrue(
            any(
                "activeFailureContext !== null" in body
                and "activeFailureContext.incidentPath" in body
                for body in actions
            )
        )

    def test_persistent_log_uses_private_sanitized_boundary(self):
        self.assertIn("IncidentOpenPrivateAppendFile(&initialLog", MAIN)
        self.assertIn("IncidentOpenPrivateAppendFile(&file", MAIN)
        self.assertGreaterEqual(
            MAIN.count("IncidentSanitizePersistentLogMessage(msg)"),
            3,
        )
        self.assertNotIn('<< "[rest]" << msg', MAIN)
        self.assertNotIn('<< "[ipc]" << msg', MAIN)
        self.assertNotIn("Log persistente activado en", MAIN)

    def test_ipc_log_is_allowlisted_not_recursively_serialized(self):
        self.assertGreaterEqual(IPC.count("IncidentFormatIpcLogEvent("), 5)
        for forbidden in (
            "ipcJsonForLog",
            "ipcRedactJsonValue",
            "ipcSensitiveLogKey",
            'QStringLiteral(" | payload=")',
            'QStringLiteral(", pendingRequestId=")',
            'QStringLiteral(" | requestId=")',
            'QStringLiteral(" | traceId=")',
        ):
            self.assertNotIn(forbidden, IPC)


if __name__ == "__main__":
    unittest.main()
