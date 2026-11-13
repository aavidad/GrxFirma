# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""D-05: en Windows, explorer.exe y rundll32.exe se lanzan por ruta absoluta."""

from __future__ import annotations

import re
import unittest
from pathlib import Path


QML = Path(__file__).resolve().parents[1]


class WindowsSystemToolsTests(unittest.TestCase):
    def test_no_lanza_programas_del_sistema_por_nombre(self) -> None:
        by_name = re.compile(
            r'startDetached\(\s*(?:QStringLiteral\()?"(?:explorer|rundll32)\.exe"'
        )
        for name in ("ipcbridge.cpp", "backendbridge.cpp"):
            source = (QML / name).read_text(encoding="utf-8")
            with self.subTest(file=name):
                self.assertIsNone(by_name.search(source))
                self.assertIn("WindowsSystemPath::explorer()", source)
                self.assertIn("WindowsSystemPath::rundll32()", source)

    def test_ruta_sale_del_directorio_de_windows(self) -> None:
        header = (QML / "windowssystempath.h").read_text(encoding="utf-8")
        self.assertIn('join(&GetSystemDirectoryW, L"rundll32.exe")', header)
        self.assertIn('join(&GetWindowsDirectoryW, L"explorer.exe")', header)
        self.assertNotIn("SetCurrentDirectory", header)


if __name__ == "__main__":
    unittest.main()
