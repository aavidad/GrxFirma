# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
import pathlib
import subprocess
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]

class EniCatalogsContract(unittest.TestCase):
    def test_generated_lists_match_go_and_are_bound_to_dropdowns(self):
        subprocess.run([sys.executable, str(ROOT / "scripts/generar_catalogos_eni.py"), "--check"], check=True, cwd=ROOT)
        page = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/EniPage.xaml.cs").read_text()
        for name in ("EstadosElaboracion", "TiposDocumentales", "EstadosExpediente"):
            self.assertIn("EniCatalog." + name, page)
        self.assertIn('T("eni.codigo." + code)', page)
        # DatePicker: día, mes y año con el teclado (recorrido 0.0.117, M9).
        self.assertIn("new DatePicker", page)
        self.assertNotIn("CalendarDatePicker", page)
        self.assertIn("TimePicker", page)
        self.assertIn("first.Focus(FocusState.Programmatic)", page)
        self.assertIn("field.ClearValue(Control.BorderBrushProperty)", page)
        self.assertNotIn("_docState.Text", page)
        self.assertNotIn("_fileState.Text", page)


    def test_missing_input_is_explained_next_to_the_button(self):
        code = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/EniPage.xaml.cs").read_text(encoding="utf-8")
        for key in ("required_signature", "required_folder", "required_certificate", "no_file", "no_folder",
                    "organ_document", "organ_file", "identifier_file", "capture_time", "open_time"):
            self.assertIn('"paridad.lote3.eni.' + key + '"', code)
        self.assertNotIn('"paridad.lote3.eni.required"', code)
        self.assertIn('T("eni.codigo." + code) + " (" + code + ")"', code)

    def test_page_reflows_and_results_stay_next_to_their_button(self):
        # Recorrido Windows 0.0.117 (A5, M8): sin recorte a 1280 px y con el
        # resultado junto al botón que lo produce.
        code = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/EniPage.xaml.cs").read_text(encoding="utf-8")
        xaml = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/EniPage.xaml").read_text(encoding="utf-8")
        self.assertIn("Description = Wrapped(", code)
        self.assertIn("Content = Wrapped(T(key))", code)
        self.assertIn("_certificates.HorizontalAlignment = HorizontalAlignment.Stretch;", code)
        self.assertIn("ShowResult(_documentMessage, _createDocument,", code)
        self.assertIn("ShowResult(_fileMessage, _createFile,", code)
        self.assertNotIn('Description = T("', code)
        self.assertIn('MaxWidth="1000"', xaml)
        self.assertNotIn("EniActions.MaxWidth", xaml)

if __name__ == "__main__":
    unittest.main()
