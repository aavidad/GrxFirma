# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Al actualizar con la ventana abierta, el motor se reinicia: aviso antes que error."""

import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
APP = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/App.xaml.cs"
VIEW_MODEL = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/MainWindowViewModel.cs"
CLIENT = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Ipc/ReconnectingIpcClient.cs"
GRACE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Ipc/EngineReconnectGrace.cs"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


class EngineRestartNoticeContractTests(unittest.TestCase):
    def setUp(self):
        self.app = APP.read_text(encoding="utf-8")
        self.watch = self.app[self.app.index("private async Task WatchEngineAsync"):]
        self.watch = self.watch[:self.watch.index("internal void NotifyUpdateInTray")]

    def test_engine_exit_shows_notice_then_error_only_after_grace(self):
        self.assertIn("_ = WatchEngineAsync(client, launchOptions.BackendProcessId!.Value);", self.app)
        order = [
            "WaitForExitAsync(_lifetimeCancellation.Token)",
            "await client.MarkLostAsync();",
            "OperationSession.Detach(client);",
            "SetReconnecting();",
            "EngineReconnectGrace.WaitForRecoveryAsync(",
            "OperationSession.Attach(client);",
            "SetConnectionFailure(",
        ]
        positions = [self.watch.index(item) for item in order]
        self.assertEqual(positions, sorted(positions))
        self.assertIn('"winui.ventana.el_motor_local_se_ha_detenido"', self.watch)

    def test_cancellation_only_limits_waits(self):
        # Trampa conocida: cancelar una petición IPC ya enviada rompe el canal.
        client = CLIENT.read_text(encoding="utf-8")
        reconnect = client[client.index("public async Task<bool> TryReconnectAsync()"):]
        reconnect = reconnect[:reconnect.index("private async Task<NdjsonIpcClient> EnsureUsableAsync")]
        self.assertIn("EnsureUsableAsync(CancellationToken.None)", reconnect)
        self.assertNotIn("SendAsync", reconnect)
        grace = GRACE.read_text(encoding="utf-8")
        self.assertIn("Window = TimeSpan.FromSeconds(30)", grace)
        self.assertIn("Task.WhenAny(", grace)
        self.assertNotIn("tryRecover(cancellationToken", grace)

    def test_notice_is_informational_not_an_error(self):
        model = VIEW_MODEL.read_text(encoding="utf-8")
        body = model[model.index("public void SetReconnecting()"):]
        body = body[:body.index("}\n") + 1]
        self.assertIn("HasConnectionError = false;", body)
        self.assertIn("IsConnectionNoticeOpen = true;", body)
        self.assertIn('"winui.ventana.grxfirma_se_esta_reiniciando"', body)

    def test_texts_exist_in_every_catalog(self):
        keys = (
            "winui.ventana.grxfirma_se_esta_reiniciando",
            "winui.ventana.el_motor_local_se_ha_detenido",
            "GrxFirma se está actualizando o reiniciando el servicio. Se reconectará sola en unos segundos.",
        )
        catalogs = sorted(LOCALES.glob("*.json"))
        self.assertEqual(len(catalogs), 11)
        for catalog in catalogs:
            values = json.loads(catalog.read_text(encoding="utf-8"))
            for key in keys:
                with self.subTest(catalog=catalog.stem, key=key):
                    self.assertTrue(values.get(key, "").strip())


if __name__ == "__main__":
    unittest.main()
