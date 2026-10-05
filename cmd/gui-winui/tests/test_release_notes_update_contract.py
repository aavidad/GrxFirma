# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest

from winui_catalog import read_with_catalog


ROOT = Path(__file__).resolve().parents[3] / "cmd/gui-winui/src/GrxFirma.WinUI"


class ReleaseNotesUpdateContract(unittest.TestCase):
    def test_startup_tray_and_dialog_are_connected(self):
        app = read_with_catalog(ROOT / "App.xaml.cs")
        window = read_with_catalog(ROOT / "MainWindow.xaml.cs")
        tray = read_with_catalog(ROOT / "Services/WindowsTrayIcon.cs")
        manager = read_with_catalog(ROOT / "Services/ReleaseNotesManager.cs")
        for marker in ("ReleaseNotesManager", "ShowReleaseNotesNotification",
                       "ShowPendingReleaseNotesAsync", "ShowReleaseNotesHistoryAsync"):
            self.assertIn(marker, app)
        self.assertIn("ContentDialog", window)
        self.assertIn("new TextBlock", window)
        self.assertNotIn("new Hyperlink", window)
        self.assertIn("BalloonUserClick", tray)
        self.assertIn('"lastSeenVersion"', manager)
        self.assertIn("File.Move(temporary, _settingsPath, true)", manager)

    def test_about_help_and_tray_open_history(self):
        self.assertIn('Click="OnReleaseNotesClick"',
                      (ROOT / "Views/AboutPage.xaml").read_text(encoding="utf-8"))
        self.assertIn('Click="OnReleaseNotesClick"',
                      (ROOT / "Views/HelpPage.xaml").read_text(encoding="utf-8"))
        self.assertIn('Label("Novedades")',
                      read_with_catalog(ROOT / "Services/WindowsTrayIcon.cs"))


if __name__ == "__main__":
    unittest.main()
