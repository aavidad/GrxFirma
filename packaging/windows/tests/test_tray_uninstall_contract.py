# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]


class TrayUninstallContractTests(unittest.TestCase):
    def test_both_windows_uninstall_paths_remove_user_run_value(self):
        for name in ("uninstall-desktop-winui.ps1", "uninstall-suite.ps1"):
            source = (ROOT / "packaging/windows" / name).read_text(
                encoding="utf-8-sig"
            )
            self.assertIn("CurrentVersion\\Run", source)
            self.assertIn('"GrxFirma"', source)
            self.assertIn("Remove-ItemProperty", source)
            self.assertNotIn("HKLM:", source)

        nsis = (ROOT / "packaging/windows/grxfirma-suite.nsi").read_text(
            encoding="utf-8"
        )
        self.assertIn(
            'DeleteRegValue HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Run" "GrxFirma"',
            nsis,
        )


if __name__ == "__main__":
    unittest.main()
