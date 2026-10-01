# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd/gui-qml"


class TokenSettingsContractTest(unittest.TestCase):
    def test_local_only_and_no_automatic_write(self):
        panel = (QT / "qml/TokenSettingsPanel.qml").read_text()
        bridge = (QT / "ipcbridge.cpp").read_text()
        self.assertIn("localIpc && bridge", panel)
        self.assertIn("FileDialog.OpenFiles", panel)
        self.assertIn("next.length > 8", panel)
        self.assertIn("confirmed: true, replaceInvalid:", panel)
        self.assertIn("!confirmation.checked", panel)
        self.assertIn('!url.isLocalFile() || !url.host().isEmpty()', bridge)
        self.assertIn('obj.value("requestId").toString().isEmpty()', bridge)
        self.assertIn("requestId != m_tokenSettingsRequestId", bridge)
        for forbidden in ("Qt.labs.settings", "saveBackendSettings", "XMLHttpRequest", "QProcess", "openUrlExternally"):
            self.assertNotIn(forbidden, panel)
        rest = (QT / "backendbridge.cpp").read_text()
        for action in ("get_token_settings", "save_token_settings", "diagnose_token_settings"):
            self.assertNotIn(action, rest)

    def test_component_registered_and_translation_parity(self):
        self.assertIn("qml/TokenSettingsPanel.qml", (QT / "qml.qrc").read_text())
        self.assertIn("TokenSettingsPanel {", (QT / "qml/main.qml").read_text())
        locales = ROOT / "internal/adapters/outbound/common/localizador/locales"
        expected = None
        for lang in ("es", "en", "fr", "pt", "de", "it", "gl", "ca", "va", "eu", "zh"):
            values = json.loads((locales / (lang + ".json")).read_text())
            selected = {k: v for k, v in values.items() if k.startswith("token_settings.")}
            self.assertEqual(len(selected), 46, lang)
            self.assertTrue(all(isinstance(v, str) and v.strip() for v in selected.values()), lang)
            if expected is None:
                expected = selected.keys()
            self.assertEqual(selected.keys(), expected, lang)


if __name__ == "__main__":
    unittest.main()
