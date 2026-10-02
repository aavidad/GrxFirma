#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
AGENT = (ROOT / "cmd/gui-qml/residentagent.cpp").read_text(encoding="utf-8")
PARSER = (ROOT / "cmd/gui-qml/releasenotes.h").read_text(encoding="utf-8")


class ReleaseNotesUpdateContract(unittest.TestCase):
    def test_update_dialog_and_hidden_tray_are_connected(self):
        for marker in (
            'property string lastSeenVersion: ""',
            'releaseNotes.since(current, previous)',
            'residentAgent.notifyReleaseNotes',
            'onPendingReleaseNotesRequested()',
            'textFormat: TextEdit.MarkdownText',
            'tr("Entendido")',
            'window.openReleaseNotes(true)',
        ):
            self.assertIn(marker, QML)
        self.assertIn("QSystemTrayIcon::messageClicked", AGENT)
        self.assertIn("emit pendingReleaseNotesRequested()", AGENT)

    def test_parser_has_limits_and_safe_rendering(self):
        for marker in ("64 * 1024", "result.size() < 64",
                       "QVersionNumber::compare", "safeMarkdown",
                       "text.remove(comments)", "text.remove(tags)"):
            self.assertIn(marker, PARSER)

    def test_new_text_exists_in_all_locales(self):
        keys = {"Novedades", "Novedades de GrxFirma %1", "Entendido",
                "Actualizado a %1: ver novedades"}
        for path in (ROOT / "internal/adapters/outbound/common/localizador/locales").glob("*.json"):
            with self.subTest(locale=path.stem):
                self.assertTrue(keys <= json.loads(path.read_text(encoding="utf-8")).keys())


if __name__ == "__main__":
    unittest.main()
