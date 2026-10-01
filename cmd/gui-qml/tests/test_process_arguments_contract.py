#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresión del filtro temprano de nombres sensibles en argv Qt/QML."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
MAIN_SOURCE = ROOT / "cmd/gui-qml/main.cpp"
ARGUMENTS_HEADER = ROOT / "cmd/gui-qml/processarguments.h"
PROJECT = ROOT / "cmd/gui-qml/grxfirma_qt.pro"


def source(path: Path) -> str:
    return path.read_text(encoding="utf-8")


class ProcessArgumentsContractTest(unittest.TestCase):
    def test_sensitive_argument_scan_runs_before_qt_starts(self) -> None:
        main = source(MAIN_SOURCE)
        scan = main.index("ProcessArguments::containsSensitiveOptionName(")
        application = main.index("QApplication app(argc, argv);")
        self.assertLess(scan, application)
        self.assertIn("return 1;", main[scan:application])

    def test_gate_covers_direct_and_generic_option_names(self) -> None:
        helper = source(ARGUMENTS_HEADER)
        for marker in (
            "password",
            "contrasena",
            "secret",
            "token",
            "authorization",
            "bearer",
            "credential",
            "apikey",
            "privatekey",
            "protectionkey",
            "secretb64",
            "pin",
        ):
            self.assertIn(marker, helper)
        self.assertIn('QStringLiteral("opcion")', helper)
        self.assertIn('QStringLiteral("option")', helper)
        self.assertIn("inlineOptionValue(", helper)

    def test_main_never_reflects_argument_contents(self) -> None:
        main = source(MAIN_SOURCE)
        scan = main.index("ProcessArguments::containsSensitiveOptionName(")
        rejection_end = main.index("// En V2", scan)
        rejection = main[scan:rejection_end]
        self.assertNotIn("argv[", rejection)
        self.assertNotIn("qDebug", rejection)
        self.assertNotIn("qWarning", rejection)
        self.assertIn("opcion sensible no permitida", rejection)

    def test_helper_is_part_of_the_qml_project(self) -> None:
        self.assertIn("processarguments.h", source(PROJECT))


if __name__ == "__main__":
    unittest.main()
