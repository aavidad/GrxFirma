# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Firmar con sello visible sin vista del PDF cargada no acaba en «páginas
inválidas»: la vista se carga y la firma continúa (recorrido Windows 0.0.118, R1)."""

import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")


class SealPreviewBeforeSignContractTest(unittest.TestCase):
    def test_missing_preview_is_not_reported_as_invalid_pages(self):
        body = QML.split("function executeSignRequest(selectedCertIndex) {", 1)[1]
        body = body.split("window.clearOperationFailure()", 1)[0]
        waiting = body.index("payload === null && signPayloadNeedsPreview)")
        self.assertLess(waiting, body.index('tr("Selección de páginas inválida. Usa 1, 1,3-5 o all.")'))
        self.assertIn('tr("sign.seal.preview_loading_before_sign")', body)
        self.assertIn("signAfterPreviewTimeout.restart()", body)

    def test_signing_resumes_once_after_the_preview_arrives(self):
        self.assertIn("window.resumeSignAfterPreview(true)", QML)
        self.assertIn("window.resumeSignAfterPreview(false)", QML)
        resume = QML.split("function resumeSignAfterPreview(ok) {", 1)[1].split("function requestPdfPreview()", 1)[0]
        self.assertIn("window.executeSignRequest(certIndex)", resume)
        self.assertIn('tr("sign.seal.preview_unavailable_before_sign")', resume)
        self.assertIn("payload === null && signPayloadNeedsPreview && resumedAfterPreview", QML)

    def test_switching_to_pades_loads_the_preview(self):
        handler = QML.split("onSignFormatChanged: {", 1)[1].split("}", 1)[0]
        self.assertIn("if (signVisibleSeal && supportsVisibleSeal()) Qt.callLater(requestPdfPreview)", handler)


if __name__ == "__main__":
    unittest.main()
