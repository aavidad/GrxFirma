# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import re
import unittest


WINDOWS = pathlib.Path(__file__).resolve().parents[1]


class SilentTlsTrustContracts(unittest.TestCase):
    def test_nsis_routes_silent_mode_and_warns_before_install(self):
        for name, script in (
            ("grxfirma-afirmauri.nsi", "install-afirmauri.ps1"),
            ("grxfirma-suite.nsi", "install-suite.ps1"),
        ):
            with self.subTest(name=name):
                source = (WINDOWS / name).read_text(encoding="utf-8-sig")
                section = source.split('Section "Uninstall"', 1)[0]
                self.assertRegex(
                    section,
                    r'StrCpy \$SilentInstallArg ""\s+'
                    r'IfSilent 0 \+2\s+'
                    r'StrCpy \$SilentInstallArg "-SilentInstall"',
                )
                self.assertRegex(
                    section,
                    r'GrxFirmaShowWarning[\s\\]+"Windows pedirá confirmar'
                    r'[^\n]*retirar el anterior[^\n]*Pulse \'Sí\'',
                )
                self.assertLess(
                    section.index("GrxFirmaShowWarning"),
                    section.index(f'-File "$INSTDIR\\{script}"'),
                )
                self.assertRegex(
                    section,
                    rf'-File "\$INSTDIR\\{re.escape(script)}"[^\n]*'
                    r'\$SilentInstallArg',
                )

    def test_powershell_propagates_switch_and_bounds_wait(self):
        suite = (WINDOWS / "install-suite.ps1").read_text(encoding="utf-8-sig")
        afirma = (WINDOWS / "install-afirmauri.ps1").read_text(
            encoding="utf-8-sig"
        )
        self.assertIn("[switch]$SilentInstall", suite)
        self.assertIn(
            "& $afirmaInstaller -InstallDir $afirmaDir -SilentInstall:$SilentInstall",
            suite,
        )
        self.assertIn("[switch]$SilentInstall", afirma)
        self.assertRegex(
            afirma, r"if \(\$SilentInstall\) \{[\s\S]*?return[\s\S]*?"
            r"Start-Process[\s\S]*?--install-local-tls-trust",
        )
        self.assertIn("WaitForExit(300000)", afirma)
        self.assertIn("$trustProcess.Kill()", afirma)
        self.assertNotIn("-Wait `", afirma)

    def test_non_ascii_sources_keep_utf8_bom(self):
        for name in (
            "grxfirma-afirmauri.nsi",
            "grxfirma-suite.nsi",
            "install-afirmauri.ps1",
            "install-suite.ps1",
        ):
            with self.subTest(name=name):
                data = (WINDOWS / name).read_bytes()
                self.assertTrue(data.startswith(b"\xef\xbb\xbf"))
                self.assertTrue(any(byte >= 128 for byte in data[3:]))


if __name__ == "__main__":
    unittest.main()
