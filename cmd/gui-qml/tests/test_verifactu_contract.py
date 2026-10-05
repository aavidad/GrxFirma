# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]

class VeriFactuContract(unittest.TestCase):
    def test_lookup_requires_a_separate_button_and_validated_url(self):
        panel = (ROOT / "cmd/gui-qml/qml/FacturaePanel.qml").read_text(encoding="utf-8")
        self.assertEqual(panel.count("panel.bridge.queryVeriFactuQR("), 1)
        self.assertIn("panel.bridge.queryVeriFactuQR(qrArea.readResult.url)", panel)
        self.assertIn('text: tr("verifactu.qr_query")', panel)
        self.assertIn('onTextChanged: { qrArea.readResult = null; qrArea.text = "" }', panel)
        self.assertIn("enabled: !panel.busy", panel)
        self.assertIn("import QtQuick.Dialogs\n", panel)

    def test_profile_is_offered_after_namespace_detection(self):
        qml = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
        self.assertIn("window.verifactuInput ?", qml)
        self.assertIn("backend.detectVeriFactu(currentFilePath)", qml)
        self.assertIn("result.inputPath === window.currentFilePath", qml)

    def test_records_use_the_existing_report_export(self):
        panel = (ROOT / "cmd/gui-qml/qml/FacturaePanel.qml").read_text(encoding="utf-8")
        self.assertIn("panel.invoiceResult = ok ? result : null", panel)
        self.assertIn("panel.reportSaver(path, panel.invoiceResult.report)", panel)
        self.assertIn("verifactuFolderDialog", panel)
