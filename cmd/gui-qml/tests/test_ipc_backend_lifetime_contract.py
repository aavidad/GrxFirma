# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""D-04: el backend que lanza la GUI Qt no sobrevive a la GUI."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
IPC_BRIDGE = ROOT / "cmd" / "gui-qml" / "ipcbridge.cpp"
LAUNCHER = ROOT / "cmd" / "grxfirma-gui" / "main.go"


class IpcBackendLifetimeContractTests(unittest.TestCase):
    def test_qt_ties_backend_lifetime_to_inherited_stdin(self) -> None:
        source = IPC_BRIDGE.read_text(encoding="utf-8")
        launch = source.index("void IpcBridge::launchBackendProcess()")
        launch_end = source.index("void IpcBridge::tryConnect()", launch)
        body = source[launch:launch_end]
        self.assertIn('args << "--ipc-parent-stdin";', body)
        # Si se cerrara el canal de escritura, el backend se pararía al
        # momento; si se redirigiera la entrada, no se pararía nunca.
        self.assertNotIn("->closeWriteChannel(", source)
        self.assertNotIn("setStandardInputFile", body)
        self.assertNotIn("setInputChannelMode", body)

    def test_launcher_watches_inherited_stdin_in_server_mode(self) -> None:
        source = LAUNCHER.read_text(encoding="utf-8")
        self.assertIn('flag.Bool("ipc-parent-stdin"', source)
        self.assertIn("ctx = vigilarEntradaHeredada(ctx, os.Stdin)", source)


if __name__ == "__main__":
    unittest.main()
