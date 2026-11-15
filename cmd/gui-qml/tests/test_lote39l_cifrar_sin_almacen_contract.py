# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 39l en Qt: entrar en Cifrar sin almacén de secretos no abre el aviso
de seguridad ni pone la barra en rojo. La sección explica que falta el
almacén y el aviso con «Activar y continuar» sale solo cuando la persona pide
crear la clave propia en un fichero. El motor calcula en todas las acciones
si habla con la interfaz vinculada, para que el bloqueo llegue activable y no
como «la organización no permite»."""

import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
MAIN = ROOT / "cmd/gui-qml/qml/main.qml"
BRIDGE = ROOT / "cmd/gui-qml/ipcbridge.cpp"
BRIDGE_H = ROOT / "cmd/gui-qml/ipcbridge.h"
BACKEND_H = ROOT / "cmd/gui-qml/backendbridge.h"
SERVER = ROOT / "internal/adapters/inbound/desktop/ipc/server.go"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
KEYS = [
    "proteccion.identidad_propia.sin_almacen." + k
    for k in ("titulo", "texto", "boton", "estado")
]


class CifrarSinAlmacenContract(unittest.TestCase):
    def setUp(self):
        self.qml = MAIN.read_text(encoding="utf-8")
        self.bridge = BRIDGE.read_text(encoding="utf-8")

    def test_entering_cifrar_is_an_automatic_load(self):
        start = self.qml.index('active: activeTab === "cifrar"')
        nav = self.qml[start:start + 400]
        self.assertIn("backend.loadProtectionRecipients()", nav)
        self.assertNotIn("loadProtectionRecipients(true)", nav)

    def test_automatic_block_does_not_open_dialog(self):
        self.assertIn(
            "quietRecipientsBlock = unsafeBlocked && !m_protectionRecipientsUserRequest;",
            self.bridge)
        emit = self.bridge.index("emit unsafeExceptionBlocked(block);")
        guard = self.bridge.rindex("if (!ok && matchesPendingRequest", 0, emit)
        self.assertIn("!quietRecipientsBlock", self.bridge[guard:emit])

    def test_recipients_block_is_not_a_failure(self):
        start = self.bridge.index('action == "protection_recipients" && unsafeBlocked')
        branch = self.bridge[start:start + 400]
        self.assertIn("proteccion.identidad_propia.sin_almacen.estado", branch)
        self.assertIn("emit protectionOwnIdentityUnavailable(", branch)
        self.assertNotIn("signingFinished", branch)
        self.assertNotIn("Error", branch)

    def test_user_request_flag(self):
        self.assertIn("loadProtectionRecipients(bool userRequested = false)",
                      BRIDGE_H.read_text(encoding="utf-8"))
        self.assertIn("loadProtectionRecipients(bool userRequested = false)",
                      BACKEND_H.read_text(encoding="utf-8"))
        self.assertIn("m_protectionRecipientsUserRequest = userRequested;", self.bridge)

    def test_section_shows_calm_notice_with_button(self):
        start = self.qml.index("visible: window.protectOwnIdentityUnavailable")
        notice = self.qml[start:start + 3000]
        for key in KEYS[:3]:
            self.assertIn('tr("%s")' % key, notice)
        self.assertIn("onClicked: backend.loadProtectionRecipients(true)", notice)
        start = self.qml.index("function onProtectionRecipientsLoaded(")
        self.assertIn("window.protectOwnIdentityUnavailable = false",
                      self.qml[start:start + 200])

    def test_activation_retries_recipients_load(self):
        start = self.qml.index("onActivated: function(item) {")
        body = self.qml[start:start + 1200]
        self.assertIn('item.operationAction === "protection_recipients"', body)
        self.assertIn("backend.loadProtectionRecipients(true)", body)

    def test_engine_computes_interface_state_for_every_action(self):
        server = SERVER.read_text(encoding="utf-8")
        self.assertIn("conEstadoInterfazPerezoso(requestCtx", server)

    def test_texts_in_all_catalogs_without_error_word(self):
        files = sorted(LOCALES.glob("*.json"))
        self.assertEqual(len(files), 11)
        for path in files:
            data = json.loads(path.read_text(encoding="utf-8"))
            for key in KEYS:
                self.assertIn(key, data, "%s sin %s" % (path.name, key))
                # Un estado con «error» pinta la barra de estado en rojo.
                self.assertNotRegex(data[key].lower(), re.compile("error"))
        es = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
        self.assertNotIn("organización", es[KEYS[1]].lower())


if __name__ == "__main__":
    unittest.main()
