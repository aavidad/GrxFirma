# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato de separación entre programas y datos de usuario en Windows."""

import ntpath
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
WINDOWS = ROOT / "packaging/windows"
LOCAL = r"C:\Users\Test\AppData\Local"
ROAMING = r"C:\Users\Test\AppData\Roaming"
COMPONENTS = (
    "CLI", "AfirmaURI", "NativeHost", "DesktopLauncher",
    "DesktopWinUI", "DesktopQML", "Suite",
)


def overlaps(first: str, second: str) -> bool:
    first = ntpath.normcase(ntpath.normpath(first))
    second = ntpath.normcase(ntpath.normpath(second))
    return first == second or first.startswith(second + "\\") or second.startswith(first + "\\")


class InstallDataSeparationTests(unittest.TestCase):
    def test_overlap_detection_rejects_equal_and_nested_roots(self) -> None:
        data = ntpath.join(LOCAL, "GrxFirma")
        self.assertTrue(overlaps(data, data))
        self.assertTrue(overlaps(data, ntpath.join(data, "Cache")))
        self.assertTrue(overlaps(ntpath.join(data, "Cache"), data))

    def test_programs_are_disjoint_from_data_and_config(self) -> None:
        install = ntpath.join(LOCAL, "Programs", "GrxFirma")
        data = ntpath.join(LOCAL, "GrxFirma")
        config = ntpath.join(ROAMING, "GrxFirma")
        for component in COMPONENTS:
            with self.subTest(component=component):
                path = ntpath.join(install, component)
                self.assertFalse(overlaps(path, data))
                self.assertFalse(overlaps(path, config))

        safety = (WINDOWS / "install-path-safety.ps1").read_text(encoding="utf-8-sig")
        self.assertIn('Join-Path $env:LOCALAPPDATA "Programs\\GrxFirma"', safety)
        self.assertIn('Join-Path $env:LOCALAPPDATA "GrxFirma"', safety)
        self.assertIn("$dataRoot.StartsWith($installRoot + $separator", safety)

        appdirs = (ROOT / "internal/appdirs/paths.go").read_text(encoding="utf-8")
        self.assertIn('filepath.Join(base, "GrxFirma")', appdirs)
        self.assertIn('windowsRoot(home, "LOCALAPPDATA", "Local")', appdirs)
        self.assertIn('windowsRoot(home, "APPDATA", "Roaming")', appdirs)
        self.assertNotIn('"Programs"', appdirs)

    def test_installers_use_only_program_directories(self) -> None:
        for script in WINDOWS.glob("*.nsi"):
            source = script.read_text(encoding="utf-8-sig")
            with self.subTest(script=script.name):
                self.assertRegex(source, r'InstallDir "\$LOCALAPPDATA\\Programs\\GrxFirma\\')
                self.assertNotRegex(source, r'\$LOCALAPPDATA\\GrxFirma\\(?:' + '|'.join(COMPONENTS) + r')\b')
                self.assertIn('RMDir "$LOCALAPPDATA\\Programs\\GrxFirma"', source)
                self.assertNotIn('RMDir "$LOCALAPPDATA\\GrxFirma"', source)
        for script in WINDOWS.glob("*install*.ps1"):
            if script.name.startswith("build-"):
                continue
            source = script.read_text(encoding="utf-8-sig")
            with self.subTest(script=script.name):
                self.assertNotRegex(source, r'\$env:LOCALAPPDATA\\GrxFirma\\(?:' + '|'.join(COMPONENTS) + r')\b')

    def test_nsis_and_powershell_with_accents_keep_utf8_bom(self) -> None:
        for script in (*WINDOWS.glob("*.nsi"), *WINDOWS.glob("*.ps1")):
            raw = script.read_bytes()
            if any(byte >= 128 for byte in raw):
                with self.subTest(script=script.name):
                    self.assertTrue(raw.startswith(b"\xef\xbb\xbf"))


if __name__ == "__main__":
    unittest.main()
