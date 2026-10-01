#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresión del descubrimiento multiplataforma de ejecutables Qt/QML."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
BACKEND_SOURCE = ROOT / "cmd/gui-qml/backendbridge.cpp"
IPC_SOURCE = ROOT / "cmd/gui-qml/ipcbridge.cpp"
LOCATOR_HEADER = ROOT / "cmd/gui-qml/executablelocator.h"
PROJECT = ROOT / "cmd/gui-qml/grxfirma_qt.pro"


def source(path: Path) -> str:
    return path.read_text(encoding="utf-8")


class ExecutableResolutionContractTest(unittest.TestCase):
    def test_windows_suffix_is_explicit_and_platform_scoped(self) -> None:
        locator = source(LOCATOR_HEADER)
        self.assertIn("#ifdef Q_OS_WIN", locator)
        self.assertIn('QStringLiteral(".exe")', locator)
        self.assertIn("candidatesForPlatform(baseNames, true)", locator)
        self.assertIn("candidatesForPlatform(baseNames, false)", locator)

    def test_all_qml_backend_launch_paths_use_the_locator(self) -> None:
        backend = source(BACKEND_SOURCE)
        ipc = source(IPC_SOURCE)
        self.assertIn('#include "executablelocator.h"', backend)
        self.assertIn('#include "executablelocator.h"', ipc)
        self.assertGreaterEqual(
            ipc.count("ExecutableLocator::bundledExecutable("),
            3,
            "IPC, REST y compatibilidad web deben resolver el binario",
        )
        self.assertIn("ExecutableLocator::bundledExecutable(", backend)

    def test_launchers_forbid_path_and_legacy_fallbacks(self) -> None:
        launchers = source(BACKEND_SOURCE) + source(IPC_SOURCE)
        self.assertNotIn("QStandardPaths::findExecutable(", launchers)
        self.assertNotIn("QDir::current().filePath(", launchers)
        self.assertNotIn("ExecutableLocator::fallbackName(", launchers)

    def test_locator_enforces_the_bundled_executable_boundary(self) -> None:
        locator = source(LOCATOR_HEADER)
        self.assertIn("QDir::isAbsolutePath(applicationDir)", locator)
        self.assertIn("canonicalFilePath()", locator)
        self.assertIn("info.isFile()", locator)
        self.assertIn("info.isExecutable()", locator)
        self.assertIn("info.isSymLink()", locator)
        self.assertIn("isWithinDirectory(", locator)
        self.assertIn("hasWindowsPESignature(", locator)
        self.assertIn('QByteArray("PE\\0\\0", 4)', locator)

    def test_locator_is_part_of_the_qml_project(self) -> None:
        self.assertIn("executablelocator.h", source(PROJECT))


if __name__ == "__main__":
    unittest.main()
