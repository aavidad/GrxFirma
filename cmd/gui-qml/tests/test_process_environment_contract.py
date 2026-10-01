#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresión del límite de secretos en el entorno de procesos Qt/QML."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
MAIN_SOURCE = ROOT / "cmd/gui-qml/main.cpp"
BACKEND_SOURCE = ROOT / "cmd/gui-qml/backendbridge.cpp"
IPC_SOURCE = ROOT / "cmd/gui-qml/ipcbridge.cpp"
ENVIRONMENT_HEADER = ROOT / "cmd/gui-qml/processenvironment.h"
PROJECT = ROOT / "cmd/gui-qml/grxfirma_qt.pro"


def source(path: Path) -> str:
    return path.read_text(encoding="utf-8")


class ProcessEnvironmentContractTest(unittest.TestCase):
    def test_frontend_scrubs_inherited_secrets_before_qt_starts(self) -> None:
        main = source(MAIN_SOURCE)
        scrub = main.index("ChildProcessEnvironment::scrubCurrentProcess()")
        application = main.index("QApplication app(argc, argv);")
        self.assertLess(scrub, application)
        self.assertIn(
            "if (!ChildProcessEnvironment::scrubCurrentProcess())", main
        )
        self.assertIn("return 1;", main[scrub:application])

    def test_helper_removes_every_sensitive_variable(self) -> None:
        helper = source(ENVIRONMENT_HEADER)
        for name in (
            "GRXFIRMA_PKCS12_PASSWORD",
            "GRXFIRMA_REST_TOKEN",
            "GRXFIRMA_PROTECTION_SECRET_B64",
        ):
            self.assertIn(name, helper)
        self.assertIn("environment.remove(name)", helper)
        self.assertIn("qunsetenv(", helper)

    def test_children_never_copy_the_raw_system_environment(self) -> None:
        backend = source(BACKEND_SOURCE)
        ipc = source(IPC_SOURCE)
        self.assertNotIn("QProcessEnvironment::systemEnvironment()", backend)
        self.assertNotIn("QProcessEnvironment::systemEnvironment()", ipc)
        self.assertGreaterEqual(
            backend.count("ChildProcessEnvironment::sanitized()"), 1
        )
        self.assertGreaterEqual(
            ipc.count("ChildProcessEnvironment::sanitized()"), 2
        )

    def test_only_rest_channels_explicitly_receive_the_token(self) -> None:
        backend = source(BACKEND_SOURCE)
        ipc = source(IPC_SOURCE)
        self.assertEqual(
            backend.count("ChildProcessEnvironment::forRestToken("), 1
        )
        self.assertEqual(ipc.count("ChildProcessEnvironment::forRestToken("), 2)

    def test_helper_is_part_of_the_qml_project(self) -> None:
        self.assertIn("processenvironment.h", source(PROJECT))


if __name__ == "__main__":
    unittest.main()
