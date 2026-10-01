# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import re
import unittest

WINDOWS = pathlib.Path(__file__).resolve().parents[1]


def source(name):
    return (WINDOWS / name).read_text(encoding="utf-8-sig")


class UninstallSilentContract(unittest.TestCase):
    def test_nsis_waits_and_propagates_failure(self):
        for nsi, script in (("grxfirma-suite.nsi", "uninstall-suite.ps1"),
                            ("grxfirma-afirmauri.nsi", "uninstall-afirmauri.ps1")):
            with self.subTest(nsi=nsi):
                section = source(nsi).split('Section "Uninstall"', 1)[1]
                self.assertRegex(section, r'StrCpy \$SilentInstallArg ""\s+IfSilent 0 \+2\s+StrCpy \$SilentInstallArg "-Silent"')
                self.assertRegex(section, rf'nsExec::ExecToLog[^\n]*{re.escape(script)}[^\n]*\$SilentInstallArg[^\n]*\n  Pop \$0\n  !insertmacro GrxFirmaExitOnExecFailure \$0')
        self.assertIn("-Silent:$Silent", source("uninstall-suite.ps1"))
        launcher = source("invoke-uninstall-silent.ps1")
        self.assertIn("$exitCode = $process.ExitCode", launcher)
        self.assertIn("WaitForExit(300000)", launcher)
        self.assertIn("$process.Kill()", launcher)
        self.assertIn("exit $exitCode", launcher)
        self.assertNotIn("$exitCode -eq 1", launcher)

    def test_silent_cleanup_only_retires_key_and_wait_is_bounded(self):
        script = source("uninstall-afirmauri.ps1")
        self.assertIn("[switch]$Silent", script)
        self.assertIn('if ($Silent) {\n        $cleanupArguments += "--no-prompt"', script)
        self.assertIn("WaitForExit(120000)", script)
        self.assertIn("$cleanupProcess.Kill()", script)
        self.assertNotIn("-Wait `", script)
        self.assertIn("retirada de ROOT pendiente", script)

    def test_all_executable_installers_stop_only_verified_directory(self):
        for name in ("install-suite.ps1", "install-afirmauri.ps1", "install-nativehost.ps1",
                     "install-desktop-qml.ps1", "install-desktop-winui.ps1"):
            with self.subTest(name=name):
                self.assertIn("Stop-GrxFirmaInstalledProcesses", source(name))
        helper = source("install-path-safety.ps1")
        self.assertIn("Resolve-GrxFirmaInstallPath -Path $Path -Component $Component", helper)
        self.assertIn("$executable.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)", helper)
        self.assertIn("WaitForExit(10000)", helper)
        self.assertNotIn("Stop-Process -Name", helper)

    def test_non_ascii_nsis_powershell_have_bom(self):
        for path in WINDOWS.glob("*.nsi"):
            data = path.read_bytes()
            if any(b >= 128 for b in data):
                self.assertTrue(data.startswith(b"\xef\xbb\xbf"), path.name)
        for path in WINDOWS.glob("*.ps1"):
            data = path.read_bytes()
            if any(b >= 128 for b in data):
                self.assertTrue(data.startswith(b"\xef\xbb\xbf"), path.name)


if __name__ == "__main__":
    unittest.main()
