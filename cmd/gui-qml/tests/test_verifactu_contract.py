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

    def test_qr_can_be_read_from_an_image_or_pdf_without_querying(self):
        panel = (ROOT / "cmd/gui-qml/qml/FacturaePanel.qml").read_text(encoding="utf-8")
        bridge = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
        self.assertIn('text: tr("verifactu.qr_from_file")', panel)
        self.assertIn('nameFilters: [tr("verifactu.qr_file_filter")]', panel)
        self.assertEqual(panel.count("panel.bridge.readVeriFactuQRFile("), 1)
        self.assertIn("panel.bridge.readVeriFactuQRFile(panel.localPath(selectedFile))", panel)
        # La lectura desde fichero viaja como inputPath y nunca dispara el cotejo.
        body = bridge.split("void IpcBridge::readVeriFactuQRFile", 1)[1].split("}", 1)[0]
        self.assertIn('QStringLiteral("read_verifactu_qr")', body)
        self.assertIn('QStringLiteral("inputPath")', body)
        self.assertNotIn("query_verifactu_qr", body)

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
