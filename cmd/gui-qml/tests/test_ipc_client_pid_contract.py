# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
IPC_BRIDGE = ROOT / "cmd" / "gui-qml" / "ipcbridge.cpp"


class IpcClientPidContractTests(unittest.TestCase):
    def test_qt_publishes_its_pid_on_every_platform(self) -> None:
        # D-03: el backend en modo --server exige el PID del frontend y lo
        # contrasta con el que da el sistema (también en macOS). Ya no puede
        # quedar dentro de un #if por plataforma.
        source = IPC_BRIDGE.read_text(encoding="utf-8")

        binding = source.index('args << "--ipc-client-pid"')
        launch = source.rfind("void IpcBridge::launchBackendProcess()", 0, binding)
        self.assertNotEqual(launch, -1)
        preceding = source[launch:binding]
        self.assertNotIn("#if defined(Q_OS_WIN) || defined(Q_OS_LINUX)", preceding)
        self.assertEqual(preceding.count("#if"), preceding.count("#endif"))
        self.assertIn(
            "QString::number(QCoreApplication::applicationPid())",
            source[binding:binding + 200],
        )

if __name__ == "__main__":
    unittest.main()
