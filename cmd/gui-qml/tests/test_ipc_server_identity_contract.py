# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""D-01: la GUI Qt solo habla con el backend que ella o el lanzador abrieron."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML_DIR = ROOT / "cmd" / "gui-qml"
IPC_BRIDGE = QML_DIR / "ipcbridge.cpp"
MAIN = QML_DIR / "main.cpp"
LAUNCHER = ROOT / "cmd" / "grxfirma-gui" / "main.go"


def _body(source: str, start: str, end: str) -> str:
    begin = source.index(start)
    return source[begin:source.index(end, begin + len(start))]


class IpcServerIdentityContractTests(unittest.TestCase):
    def test_server_identity_is_checked_before_anything_is_sent(self) -> None:
        source = IPC_BRIDGE.read_text(encoding="utf-8")
        body = _body(source, "void IpcBridge::onConnected() {", "\n}\n")
        verify = body.index("verifyConnectedServer()")
        for later in ("flushDeferredRequest()", "sendRequest("):
            self.assertLess(verify, body.index(later))
        self.assertIn("rejectConnectedServer();", body[verify:verify + 120])

    def test_no_fixed_or_shared_socket_path(self) -> None:
        for path in (IPC_BRIDGE, MAIN):
            source = path.read_text(encoding="utf-8")
            self.assertNotIn('"grxfirma_ipc_" + userName', source, path.name)
            self.assertNotIn("runtimeDir = QDir::tempPath()", source, path.name)

    def test_existing_endpoint_is_never_reused_by_probing(self) -> None:
        source = IPC_BRIDGE.read_text(encoding="utf-8")
        body = _body(source, "void IpcBridge::startBackend(", "void IpcBridge::launchBackendProcess()")
        self.assertNotIn("testSocket", body)
        self.assertIn("IpcPeer::newPrivateSocketPath()", body)
        self.assertIn("m_launcherBackendPid", body)

    def test_response_line_is_bounded(self) -> None:
        source = IPC_BRIDGE.read_text(encoding="utf-8")
        self.assertIn("setReadBufferSize(IpcPeer::kMaxResponseLineBytes)", source)

    def test_launcher_passes_its_pid_to_qt(self) -> None:
        main_source = MAIN.read_text(encoding="utf-8")
        self.assertIn('arg == "--backend-pid"', main_source)
        self.assertIn("setLauncherBackendPid(", main_source)
        launcher = LAUNCHER.read_text(encoding="utf-8")
        body = _body(launcher, "func argumentosFrontend(", "\n}\n")
        self.assertNotIn("frontend == frontendWinUI", body)


if __name__ == "__main__":
    unittest.main()
