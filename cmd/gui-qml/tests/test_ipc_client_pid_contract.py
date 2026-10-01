# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
IPC_BRIDGE = ROOT / "cmd" / "gui-qml" / "ipcbridge.cpp"


class IpcClientPidContractTests(unittest.TestCase):
    def test_qt_publishes_its_pid_only_on_supported_platforms(self) -> None:
        source = IPC_BRIDGE.read_text(encoding="utf-8")

        binding = source.index('args << "--ipc-client-pid"')
        platform_guard = source.rfind(
            "#if defined(Q_OS_WIN) || defined(Q_OS_LINUX)",
            0,
            binding,
        )
        platform_end = source.index("#endif", binding)

        self.assertNotEqual(platform_guard, -1)
        self.assertGreater(platform_end, binding)
        self.assertIn(
            "QString::number(QCoreApplication::applicationPid())",
            source[binding:platform_end],
        )


if __name__ == "__main__":
    unittest.main()
