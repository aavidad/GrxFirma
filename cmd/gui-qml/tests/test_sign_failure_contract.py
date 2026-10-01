# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
IPC = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")


class SignFailureContractTest(unittest.TestCase):
    def test_sign_request_keeps_the_selected_document_and_certificate(self):
        self.assertIn(
            "backend.signFileAdvanced(window.currentFilePath, window.currentOutputPath, selectedCertIndex, payload)",
            QML,
        )
        body = IPC.split("void IpcBridge::signFileAdvanced(", 1)[1].split(
            "void IpcBridge::signFileMultiAdvanced(", 1
        )[0]
        for field in ("inputPath", "outputPath", "certificateIndex", "certificateId", "format", "extraOptions"):
            self.assertIn(f'params["{field}"]', body)
        self.assertIn('payload.certificateId = certificateId(window.certificates[selectedCertIndex])', QML)
        self.assertIn('sendRequest("sign", params)', body)

    def test_sign_failure_shows_redacted_engine_cause(self):
        self.assertIn(
            'QString errMsg = IncidentSanitizeText(obj.value("error").toString());',
            IPC,
        )
        self.assertRegex(
            IPC,
            re.compile(r"emit signingFinished\(false, errMsg, \"\"\);"),
        )
        self.assertIn("const QString safeMessage = IncidentSanitizeText(message);", IPC)
        self.assertIn("emit signingFinished(false, safeMessage, \"\");", IPC)
        self.assertIn("activeFailureContext.message", QML.split("function supportAssistantFriendlySummary()", 1)[1].split("function ", 1)[0])


if __name__ == "__main__":
    unittest.main()
