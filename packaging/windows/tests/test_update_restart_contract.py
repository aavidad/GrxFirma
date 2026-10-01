#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""El instalador restaura el residente tras reemplazar sus ejecutables."""

from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]


class UpdateRestartContract(unittest.TestCase):
    def test_capture_precedes_process_stop_and_restore_follows_install(self):
        script = (ROOT / "packaging/windows/install-suite.ps1").read_text(encoding="utf-8-sig")
        nsis = (ROOT / "packaging/windows/grxfirma-suite.nsi").read_text(encoding="utf-8-sig")
        self.assertLess(script.index("InstallerRestartFrontend"), script.index("Stop-GrxFirmaInstalledProcesses"))
        self.assertIn("$process.SessionId -ne $currentSession", script)
        self.assertIn("Registry]::CurrentUser.OpenSubKey", script)
        self.assertIn("Resolve-GrxFirmaInstallPath -Path $launcherDir", script)
        self.assertIn("Test-GrxFirmaInstallMarker", script)
        self.assertIn('Start-Process -FilePath $launcher -ArgumentList "--frontend=$restartFrontend --start-hidden --ui-binary=', script)
        self.assertLess(nsis.index('remove-unselected-desktop.ps1'), nsis.index('-RestoreTray'))
        self.assertIn('GrxFirmaExitOnExecFailure $0', nsis.split('-RestoreTray', 1)[1])
        self.assertNotIn('HKLM', script)


if __name__ == "__main__":
    unittest.main()
