# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Al actualizar se retiran los accesos del escritorio de versiones anteriores
solo si apuntan a un programa instalado por GrxFirma o AutoFirmaV2."""

from pathlib import Path
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "install-suite.ps1"


class LegacyDesktopShortcutsContract(unittest.TestCase):
    def setUp(self) -> None:
        self.text = SCRIPT.read_text(encoding="utf-8-sig")

    def test_removes_known_legacy_names(self) -> None:
        for name in ("GrxFirma Diputación.lnk", "AutoFirma Diputación.lnk", "AutoFirmaV2.lnk"):
            with self.subTest(name=name):
                self.assertIn(f'"{name}"', self.text)

    def test_only_removes_shortcuts_owned_by_grxfirma(self) -> None:
        self.assertIn("StartsWith($_, [System.StringComparison]::OrdinalIgnoreCase)", self.text)
        self.assertIn("Remove-GrxFirmaLegacyDesktopShortcuts -BaseInstallDir $BaseInstallDir", self.text)

    def test_runs_even_with_core_only(self) -> None:
        call = "Remove-GrxFirmaLegacyDesktopShortcuts -BaseInstallDir $BaseInstallDir"
        line = next(l for l in self.text.splitlines() if l.strip() == call)
        self.assertFalse(line.startswith((" ", "\t")), "la limpieza no debe depender de -CoreOnly")

    def test_never_touches_the_current_shortcut(self) -> None:
        names_line = next(line for line in self.text.splitlines() if line.strip().startswith("$names = @("))
        self.assertNotIn('"GrxFirma.lnk"', names_line)


if __name__ == "__main__":
    unittest.main()
