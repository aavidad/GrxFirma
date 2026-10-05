# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
WINUI = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"
SERVICE = WINUI / "Services" / "WindowsPdfPreviewService.cs"
INTERFACE = WINUI / "Services" / "IPdfPreviewService.cs"
PAGE = WINUI / "Views" / "SignPage.xaml.cs"
VIEW_MODEL = WINUI / "ViewModels" / "SignPageViewModel.cs"


class NativePdfPreviewContractTests(unittest.TestCase):
    def test_windows_renderer_has_bounded_input_output_and_page_count(self) -> None:
        source = SERVICE.read_text(encoding="utf-8")
        for contract in (
            "Windows.Data.Pdf",
            "PdfDocument.LoadFromFileAsync",
            "PdfPageRenderOptions",
            "MaximumInputBytes",
            "MaximumPngBytes",
            "MaximumRenderDimension",
            "document.PageCount is 0 or > 1_000_000",
            "FileAttributes.ReparsePoint",
        ):
            self.assertIn(contract, source)
        self.assertNotIn("pdftoppm", source)
        self.assertNotIn("pdfinfo", source)

    def test_windows_renderer_measures_the_page_as_seen(self) -> None:
        # El motor coloca el sello sobre la página tal como se ve (CropBox y
        # /Rotate aplicados); RenderToStreamAsync dibuja así la página y las
        # medidas devueltas deben ser las mismas, no la MediaBox sin girar.
        source = SERVICE.read_text(encoding="utf-8")
        self.assertIn("pdfPage.Size", source)
        self.assertNotIn("MediaBox.Width", source)
        self.assertNotIn("mediaBox", source)

    def test_view_model_uses_injected_native_preview_with_safe_fallback(self) -> None:
        interface = INTERFACE.read_text(encoding="utf-8")
        view_model = VIEW_MODEL.read_text(encoding="utf-8")
        self.assertIn("Task<PdfPreviewResult> RenderPageAsync(", interface)
        self.assertIn("IPdfPreviewService? pdfPreview = null", view_model)
        self.assertIn("await _pdfPreview.RenderPageAsync(", view_model)
        self.assertIn("WINDOWS_PDF_PREVIEW_FAILED", view_model)
        self.assertIn("INVALID_WINDOWS_PDF_PREVIEW_RESULT", view_model)
        self.assertIn("operations!.GetPdfPreviewAsync(", view_model)

    def test_sign_page_wires_the_native_windows_renderer(self) -> None:
        source = PAGE.read_text(encoding="utf-8")
        self.assertIn("new WindowsPdfPreviewService()", source)


if __name__ == "__main__":
    unittest.main()
