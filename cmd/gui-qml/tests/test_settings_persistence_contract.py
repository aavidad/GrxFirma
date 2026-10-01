# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")


class SettingsPersistenceContractTest(unittest.TestCase):
    def test_theme_updates_both_qsettings_and_typed_backend(self) -> None:
        handler = re.search(
            r"onCurrentThemeIndexChanged:\s*\{(?P<body>.*?)\n\s*\}",
            QML,
            re.DOTALL,
        )
        self.assertIsNotNone(handler)
        body = handler.group("body")
        self.assertIn("appSettings.themeIndex = currentThemeIndex", body)
        self.assertIn("markBackendSettingsDirty()", body)

    def test_factory_reset_marks_typed_backend_dirty(self) -> None:
        reset = re.search(
            r"window\.currentThemeIndex\s*=\s*0\s*"
            r"backend\.expertMode\s*=\s*false\s*"
            r"window\.markBackendSettingsDirty\(\)",
            QML,
        )
        self.assertIsNotNone(reset)

    def test_backend_load_does_not_create_a_pending_save(self) -> None:
        self.assertIn("if (!settingsLoaded || applyingLoadedSettings) return", QML)
        self.assertIn("window.applyingLoadedSettings = true", QML)
        self.assertIn("window.applyingLoadedSettings = false", QML)

    def test_logo_opacity_is_saved_and_restored(self) -> None:
        self.assertIn("property int signSealLogoOpacityPercent: 100", QML)
        self.assertIn("signSealLogoOpacityPercent: window.signSealLogoOpacityPercent", QML)
        self.assertIn("window.signSealLogoOpacityPercent = Number(s.signSealLogoOpacityPercent)", QML)
        self.assertIn("logoOpacityPercent: defaults.logoOpacityPercent", QML)
        self.assertEqual(QML.count('text: window.signSealLogoOpacityPercent + " %"'), 2)


if __name__ == "__main__":
    unittest.main()
