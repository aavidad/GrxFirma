# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
APP = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"

class VeriFactuContract(unittest.TestCase):
    def test_explicit_query_uses_only_the_url_returned_by_the_engine(self):
        code = (APP / "Views/FacturaePage.xaml.cs").read_text(encoding="utf-8")
        self.assertEqual(code.count("operations.QueryVeriFactuQrAsync("), 1)
        self.assertIn("operations.QueryVeriFactuQrAsync(_verifactuQrUrl)", code)
        self.assertIn("_verifactuQrUrl = qr.Url", code)
        self.assertIn("VeriFactuQrInput.EnsureAllowedAuthority(url)", (CORE / "Operations/DesktopOperationsClient.cs").read_text(encoding="utf-8"))
        self.assertIn("StringComparison.Ordinal", (CORE / "Operations/VeriFactuQrInput.cs").read_text(encoding="utf-8"))
        ET.parse(APP / "Views/FacturaePage.xaml")

    def test_format_uses_engine_detection_and_refreshes_binding(self):
        code = (APP / "ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
        self.assertIn("operations.DetectVeriFactuAsync(path)", code)
        self.assertIn("StandardFormats.Concat(new[] { VeriFactuFormat })", code)
        self.assertIn("PathsEqual(_inputPath ?? string.Empty, path)", code)
        self.assertIn('ItemsSource="{x:Bind ViewModel.Formats, Mode=OneWay}"', (APP / "Views/SignPage.xaml").read_text(encoding="utf-8"))
