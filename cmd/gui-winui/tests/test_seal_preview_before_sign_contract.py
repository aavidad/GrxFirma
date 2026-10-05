# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""Con el sello visible activado se puede firmar sin abrir «Opciones
avanzadas» (recorrido de Windows 0.0.118, R1)."""

import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
UI = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"
LOCALES = ROOT / "internal" / "adapters" / "outbound" / "common" / "localizador" / "locales"


class SealPreviewBeforeSignContractTests(unittest.TestCase):
    def setUp(self) -> None:
        self.page = (UI / "Views" / "SignPage.xaml.cs").read_text(encoding="utf-8")
        self.vm = (UI / "ViewModels" / "SignPageViewModel.cs").read_text(encoding="utf-8")

    def test_preview_is_loaded_before_signing(self) -> None:
        start = self.page.index("private async void OnSignClick(")
        body = self.page[start:self.page.index("private void ShowSignResult(", start)]
        ensure = body.index("EnsureVisibleSealPreviewBeforeSigningAsync(cancellation.Token)")
        self.assertLess(ensure, body.index("ViewModel.SignAsync(cancellation.Token)"))

    def test_wait_has_a_deadline_and_never_cancels_sent_requests(self) -> None:
        start = self.page.index("private async Task<bool> EnsureVisibleSealPreviewBeforeSigningAsync(")
        body = self.page[start:self.page.index("private void ShowSignResult(", start)]
        self.assertIn("PortalSealPreviewWait.WaitAsync(", body)
        self.assertIn("SealPreviewBeforeSignTimeout", body)
        self.assertNotIn(".Cancel()", body)
        self.assertIn("ReportVisibleSealPreviewUnavailableBeforeSigning()", body)
        self.assertIn("AdvancedSignExpander.IsExpanded = true;", body)
        self.assertIn("VisibleSealPreviewButton.Focus(", body)

    def test_format_change_reloads_the_preview(self) -> None:
        start = self.page.index("private void OnViewModelPropertyChanged(")
        body = self.page[start:self.page.index("ScheduleAutomaticVisibleSealPreview();", start)]
        self.assertIn("nameof(SignPageViewModel.SelectedFormat)", body)

    def test_same_check_for_signing_and_for_loading(self) -> None:
        self.assertEqual(self.vm.count("IsVisibleSealPreviewCurrent(previewPage)"), 2)

    def test_messages_say_where_to_load_the_preview(self) -> None:
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key in ("winui.firmar.sello_sin_vista_previa_antes_de_firmar",
                        "winui.firmar.la_previsualizacion_no_corresponde_al"):
                self.assertIn(catalog["Cargar previsualización"], catalog[key], f"{path.name}: {key}")
                self.assertIn(catalog["Opciones avanzadas"], catalog[key], f"{path.name}: {key}")


if __name__ == "__main__":
    unittest.main()
