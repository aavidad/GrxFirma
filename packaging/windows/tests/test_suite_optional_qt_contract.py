# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
BUILD_SUITE = ROOT / "packaging/windows/build-suite.ps1"


class OptionalQtStageContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.script = BUILD_SUITE.read_text(encoding="utf-8")

    def test_incomplete_optional_qt_stage_is_not_integrated(self) -> None:
        for contract in (
            "$qtStageReady = $false",
            "Assert-QtSuiteStage -StageDir $DesktopQmlEscenario",
            "$qtStageReady = $true",
            "if ($qtStageReady)",
            "Se ignora la stage Qt/QML incompleta",
        ):
            self.assertIn(contract, self.script)

    def test_required_qt_stage_still_fails_closed(self) -> None:
        catch_block = self.script.split(
            "} catch {",
            maxsplit=1,
        )[1].split(
            "if ($qtStageReady)",
            maxsplit=1,
        )[0]
        self.assertIn("if ($RequireQtStage)", catch_block)
        self.assertIn("throw", catch_block)
        self.assertIn(
            "} elseif ($RequireQtStage) {",
            self.script,
        )
        self.assertIn(
            "No existe la stage Qt/QML completa requerida",
            self.script,
        )


if __name__ == "__main__":
    unittest.main()
