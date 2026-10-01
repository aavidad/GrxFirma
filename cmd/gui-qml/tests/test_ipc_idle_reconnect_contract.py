#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Regresión del canal IPC persistente durante interacción humana."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
IPC_CPP = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")


class IPCIdleReconnectContractTest(unittest.TestCase):
    def test_idle_disconnect_reconnects_only_while_owned_backend_is_alive(self) -> None:
        disconnected = IPC_CPP.index(
            "connect(m_socket, &QLocalSocket::disconnected"
        )
        constructor_end = IPC_CPP.index(
            "IpcBridge::~IpcBridge()",
            disconnected,
        )
        body = IPC_CPP[disconnected:constructor_end]

        self.assertIn("QTimer::singleShot(0, this", body)
        self.assertIn("m_shuttingDown", body)
        self.assertIn("!m_process", body)
        self.assertIn("QProcess::NotRunning", body)
        self.assertIn("QLocalSocket::ConnectedState", body)
        self.assertIn("Reconectando con el motor de firma...", body)
        self.assertIn("tryConnect();", body)

    def test_transient_credentials_remain_excluded_from_deferred_queue(self) -> None:
        queue_start = IPC_CPP.index("bool IpcBridge::queueDeferredRequest")
        queue_end = IPC_CPP.index(
            "void IpcBridge::flushDeferredRequest",
            queue_start,
        )
        body = IPC_CPP[queue_start:queue_end]
        for action in (
            "import_certificate",
            "import_certificate_to_store",
            "use_temporary_certificate",
            "proxy_secret_store",
        ):
            self.assertIn(action, body)
        self.assertIn("return false;", body)


if __name__ == "__main__":
    unittest.main()
