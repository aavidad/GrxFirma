# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""G-12: install-afirmauri.ps1 no genera lanzadores .cmd que pasen la URI a cmd.exe."""

from __future__ import annotations

import re
import unittest
from pathlib import Path


WINDOWS = Path(__file__).resolve().parents[1]


class AfirmaUriNoCmdHandlerTests(unittest.TestCase):
    def setUp(self) -> None:
        self.script = (WINDOWS / "install-afirmauri.ps1").read_text(encoding="utf-8-sig")

    def test_no_reenvia_argumentos_a_cmd(self) -> None:
        self.assertNotIn("%*", self.script)
        self.assertNotIn("@echo off", self.script)

    def test_no_escribe_ficheros_cmd(self) -> None:
        for match in re.finditer(r"(Set-Content|Out-File|New-Item)[^\n]*", self.script):
            self.assertNotIn(".cmd", match.group(0))

    def test_borra_los_cmd_de_versiones_anteriores(self) -> None:
        self.assertIn('"afirmauri-handler.cmd"', self.script)
        self.assertIn('"afirmauri-handler-debug.cmd"', self.script)
        self.assertIn("Remove-Item -LiteralPath $legacyHandler -Force", self.script)


if __name__ == "__main__":
    unittest.main()
