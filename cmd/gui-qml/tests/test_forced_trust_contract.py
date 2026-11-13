#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Lote 14: aceptar en este equipo una firma que GrxFirma no da por válida."""

import json
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
IPC = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
IPC_H = (ROOT / "cmd/gui-qml/ipcbridge.h").read_text(encoding="utf-8")
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


def qml_function(name: str) -> str:
    start = re.search(rf"function\s+{re.escape(name)}\s*\([^)]*\)\s*\{{", QML)
    if start is None:
        raise AssertionError(f"Falta la función {name}")
    depth = 1
    pos = start.end()
    while depth:
        if QML[pos] == "{":
            depth += 1
        elif QML[pos] == "}":
            depth -= 1
        pos += 1
    return QML[start.start():pos]


def bloque(texto: str, inicio: str, fin: str) -> str:
    return texto.split(inicio, 1)[1].split(fin, 1)[0]


class ForcedTrustContractTest(unittest.TestCase):
    def test_forced_signature_is_never_shown_as_trusted(self) -> None:
        kind = qml_function("verificationOutcomeKind")
        forced = kind.index('return "forced"')
        trusted = kind.index('return "trusted"')
        invalid = kind.index('return "invalid"')
        # Lo inválido sigue ganando; lo forzado se decide antes que «confiable».
        self.assertLess(invalid, forced)
        self.assertLess(forced, trusted)
        applied = qml_function("verificationForcedTrustApplied")
        self.assertIn('details.valid !== true', applied)
        self.assertIn('"trust.user_forced"', applied)
        self.assertIn('tr("verify.forced_trust.ui.outcome")', qml_function("verificationOutcomeLabel"))

    def test_button_only_when_backend_offers_and_policy_allows(self) -> None:
        acceptable = qml_function("forcedTrustAcceptable")
        self.assertIn("ft.available !== true", acceptable)
        self.assertIn("ft.forbidden === true", acceptable)
        card = bloque(QML, "// Lote 14: la firma solo falla por algo", 'text: tr("Asistente")')
        self.assertIn("isIpcMode && window.forcedTrustAcceptable(verifyTab.verifyDetails).length > 0", card)
        self.assertIn('tr("verify.forced_trust.ui.accept_button")', card)
        self.assertIn("window.openForcedTrustDialog(verifyTab.verifyDetails)", card)
        # Abrir el diálogo no guarda nada.
        self.assertNotIn("acceptForcedTrust", qml_function("openForcedTrustDialog"))

    def test_dialog_shows_warning_and_cancel_is_default(self) -> None:
        dialog = bloque(QML, "id: forcedTrustDialog", "id: certificateValidationDialog")
        self.assertIn("modal: true", dialog)
        self.assertIn("forcedTrustCancelButton.forceActiveFocus()", dialog)
        self.assertIn("modelData.notice", dialog)
        self.assertIn("modelData.label", dialog)
        self.assertIn("modelData.fingerprint", dialog)
        self.assertIn('tr("verify.forced_trust.ui.all_signatures")', dialog)
        self.assertIn("backend.acceptForcedTrust(window.forcedTrustPendingItems)", dialog)
        self.assertLess(dialog.index("id: forcedTrustCancelButton"), dialog.index("backend.acceptForcedTrust"))

    def test_accept_reverifies_and_settings_can_remove(self) -> None:
        handler = bloque(QML, "function onForcedTrustFinished", "// Lote 14: confirmación antes")
        self.assertIn("window.requestVerification(verifyTab.verifyFilePath, verifyTab.verifyOriginalPath)", handler)
        card = bloque(QML, "id: forcedTrustCard", "// ── TSA (Sellado de Tiempo)")
        self.assertIn("backend.removeForcedTrust(", card)
        self.assertIn('tr("verify.forced_trust.ui.remove_accessible")', card)
        self.assertIn("window.forcedTrustForbidden", card)
        self.assertIn("if (isIpcMode) backend.loadForcedTrust()", QML)

    def test_bridge_actions_and_no_replay(self) -> None:
        for action in ("forced_trust_list", "forced_trust_accept", "forced_trust_remove"):
            self.assertIn(f'"{action}"', IPC)
        for method in ("loadForcedTrust", "acceptForcedTrust", "removeForcedTrust"):
            self.assertIn(f"Q_INVOKABLE void {method}(", IPC_H)
        cola = bloque(IPC, "bool IpcBridge::queueDeferredRequest", "ipcHasTransientProtectionSecret")
        self.assertIn('"forced_trust_accept"', cola)
        self.assertIn('"forced_trust_remove"', cola)

    def test_visible_keys_exist_in_every_locale(self) -> None:
        keys = set(re.findall(r'tr\("(verify\.forced_trust\.[a-z_.]+)"\)', QML))
        self.assertGreaterEqual(len(keys), 10)
        tipos = ("ca_desconocida", "clave_rsa_corta", "certificado_componente", "revocacion_no_comprobable", "algoritmo_sha1")
        for tipo in tipos:
            keys.update({f"verify.forced_trust.label.{tipo}", f"verify.forced_trust.notice.{tipo}", f"verify.forced_trust.applied.{tipo}"})
        locale_paths = sorted(LOCALES.glob("*.json"))
        self.assertEqual(11, len(locale_paths))
        for path in locale_paths:
            with self.subTest(locale=path.stem):
                data = json.loads(path.read_text(encoding="utf-8"))
                faltan = sorted(k for k in keys if not str(data.get(k, "")).strip())
                self.assertEqual([], faltan)


if __name__ == "__main__":
    unittest.main()
