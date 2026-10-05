# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""La espera de la vista del PDF antes de firmar se ve junto al botón, se
anuncia y bloquea «Firmar ahora»; su fallo tiene un titular propio
(revisión de usabilidad de 0.0.118)."""

import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")


class SignWaitFeedbackContractTest(unittest.TestCase):
    def test_waiting_flag_follows_the_pending_signature(self):
        self.assertIn("readonly property bool signWaitingForPreview: signAfterPreviewCertIndex >= 0", QML)

    def test_sign_button_is_disabled_while_waiting(self):
        button = QML.split('tr("Firmar ahora"))', 1)[1].split("onClicked:", 1)[0]
        self.assertIn("!window.signWaitingForPreview", button)

    def test_waiting_text_is_an_alert_next_to_the_button(self):
        after = QML.split('tr("Firmar ahora"))', 1)[1]
        block = after.split("visible: window.signWaitingForPreview", 1)[1].split("}", 1)[0]
        self.assertIn('text: tr("sign.seal.preview_loading_before_sign")', block)
        self.assertIn("Accessible.role: Accessible.AlertMessage", block)
        self.assertIn("Accessible.name: text", block)

    def test_unavailable_preview_has_its_own_heading(self):
        self.assertEqual(QML.count('signValidationErrorDialog.heading = tr("sign.seal.preview_unavailable_heading")'), 2)
        dialog = QML.split("id: signValidationErrorDialog", 1)[1].split("ThemedDialog {", 1)[0]
        self.assertIn("onClosed: heading = \"\"", dialog)
        self.assertIn('tr("⚠️ Requisitos faltantes")', dialog)


if __name__ == "__main__":
    unittest.main()
