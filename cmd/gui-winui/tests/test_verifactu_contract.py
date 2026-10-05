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

    def test_qr_from_image_or_pdf_reuses_the_engine_and_explicit_query(self):
        code = (APP / "Views/FacturaePage.xaml.cs").read_text(encoding="utf-8")
        client = (CORE / "Operations/DesktopOperationsClient.cs").read_text(encoding="utf-8")
        picker = (APP / "Services/WindowsFilePickerService.cs").read_text(encoding="utf-8")
        xaml = (APP / "Views/FacturaePage.xaml").read_text(encoding="utf-8")
        button = xaml.split('x:Name="ReadVeriFactuQrFileButton"', 1)[1].split("/>", 1)[0]
        self.assertIn('Click="OnReadVeriFactuQrFileClick"', button)
        self.assertIn('ReadVeriFactuQrFileButton.Content = T("verifactu.qr_from_file")', code)
        self.assertIn("OpenFilePickerProfile.VeriFactuQrSource", code)
        self.assertIn("OpenFilePickerProfile.VeriFactuQrSource =>", picker)
        # Las imágenes las lee el motor; los PDF se rasterizan con Windows.Data.Pdf.
        self.assertIn("operations.ReadVeriFactuQrFromFileAsync(path)", code)
        self.assertIn("preview.RenderPageForCodeReadingAsync(path, page, CancellationToken.None)", code)
        self.assertIn("operations.ReadVeriFactuQrFromImageAsync(rendered.Data)", code)
        # Solo «no encontrado» pasa a la página siguiente; otro fallo para el bucle.
        loop = code.split("for (var page = 1;", 1)[1].split("if (last is null)", 1)[0]
        self.assertIn("if (!string.Equals(last.ErrorCode, VeriFactuQrNotFoundCode, StringComparison.Ordinal)) break;", loop)
        self.assertIn('private const string VeriFactuQrNotFoundCode = "verifactu_qr_not_found";', code)
        self.assertIn("global::GrxFirma.WinUI.Core.Operations.VeriFactuQrInput.IsPdfSource(path)", code)
        self.assertIn("VeriFactuQrInput.EnsureImagePayload(imageB64)", client)
        self.assertIn("new { imageB64 }", client)
        # Leer nunca coteja: la consulta sigue en su propio botón.
        handler = code.split("private async void OnReadVeriFactuQrFileClick", 1)[1].split("private async void OnQueryVeriFactuQrClick", 1)[0]
        self.assertNotIn("QueryVeriFactuQrAsync", handler)

    def test_format_uses_engine_detection_and_refreshes_binding(self):
        code = (APP / "ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
        self.assertIn("operations.DetectVeriFactuAsync(path)", code)
        self.assertIn("StandardFormats.Concat(new[] { VeriFactuFormat })", code)
        self.assertIn("PathsEqual(_inputPath ?? string.Empty, path)", code)
        self.assertIn('ItemsSource="{x:Bind ViewModel.Formats, Mode=OneWay}"', (APP / "Views/SignPage.xaml").read_text(encoding="utf-8"))

    def test_verifactu_is_its_own_card_with_results_next_to_its_actions(self):
        xaml = (APP / "Views/FacturaePage.xaml").read_text(encoding="utf-8")
        code = (APP / "Views/FacturaePage.xaml.cs").read_text(encoding="utf-8")
        root = ET.parse(APP / "Views/FacturaePage.xaml").getroot()
        name = "{http://schemas.microsoft.com/winfx/2006/xaml}Name"
        card = next(e for e in root.iter() if e.get(name) == "VeriFactuCard")
        inside = {e.get(name) for e in card.iter() if e.get(name)}
        for control in ("ValidateVeriFactuButton", "VeriFactuSummary", "VeriFactuReport", "VeriFactuQrInput", "VeriFactuQrReport"):
            self.assertIn(control, inside)
        # Fuera del paso 3 de FACe: la tarjeta no contiene la casilla del validador oficial.
        self.assertNotIn("El validador oficial no muestra errores", ET.tostring(card, encoding="unicode"))
        names = [e.get("AutomationProperties.Name") for e in card.iter() if e.get("AutomationProperties.Name")]
        self.assertEqual(len(names), len(set(names)) + 1)  # El Expander y su texto comparten nombre.
        self.assertNotIn("InvoiceValidationSummary.Text = T(result.Data.Valid ? \"verifactu.valid\"", code)
        self.assertIn("VeriFactuQrResponse.Classify(raw)", code)
        # Recorrido Windows 0.0.117 (M4, M5, B8).
        self.assertIn("result.Data.Summary.Length > 0", code)
        self.assertIn('T("verifactu.qr_empty")', code)
        self.assertIn('catch (ArgumentException) { VeriFactuQrReport.Text = T("verifactu.qr_url"); }', code)
        self.assertIn("VeriFactuQrInput.Text = qr.Url;", code)
        self.assertIn("string.Equals(VeriFactuQrInput.Text, _verifactuQrUrl, StringComparison.Ordinal)) return;", code)
        self.assertIn("VeriFactuQrDisplay.Amount(qr.Amount, culture)", code)
        self.assertNotIn("Response.ToString() : result.SafeUserMessage", code)
        self.assertNotIn('AutomationProperties.LiveSetting="Assertive"\n                    IsClosable="False"\n                    IsOpen="True"', xaml)

