# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Avisos que se ven y se oyen: espera y fallo de la vista previa antes de
firmar, informe guardado en Verificar y destino no disponible en Proteger
(revisión de usabilidad de 0.0.118)."""

import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
UI = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"
LOCALES = ROOT / "internal" / "adapters" / "outbound" / "common" / "localizador" / "locales"


def _between(text: str, start: str, end: str) -> str:
    first = text.index(start)
    return text[first:text.index(end, first)]


class SignPreviewWaitFeedbackTests(unittest.TestCase):
    def setUp(self) -> None:
        self.page = (UI / "Views" / "SignPage.xaml.cs").read_text(encoding="utf-8")
        self.xaml = (UI / "Views" / "SignPage.xaml").read_text(encoding="utf-8")
        self.ensure = _between(self.page,
            "private async Task<bool> EnsureVisibleSealPreviewBeforeSigningAsync(",
            "private void ShowSealPreviewBeforeSignNotice(")

    def test_notice_is_an_infobar_next_to_the_sign_button(self) -> None:
        notice = _between(self.xaml, 'x:Name="SealPreviewBeforeSignNotice"', "/>")
        self.assertIn('AutomationProperties.LiveSetting="Assertive"', notice)
        self.assertLess(self.xaml.index('x:Name="SealPreviewBeforeSignNotice"'),
                        self.xaml.index('x:Name="SignButton"'))
        self.assertNotIn("AppMutedTextBrush", notice)

    def test_waiting_is_shown_and_announced(self) -> None:
        loading = self.ensure.index('"sign.seal.preview_loading_before_sign"')
        self.assertLess(loading, self.ensure.index("PortalSealPreviewWait.WaitAsync("))
        self.assertIn("InfoBarSeverity.Informational", self.ensure)
        show = _between(self.page, "private void ShowSealPreviewBeforeSignNotice(",
                        "private void HideSealPreviewBeforeSignNotice(")
        self.assertIn("RaiseAutomationEvent(AutomationEvents.LiveRegionChanged)", show)

    def test_failure_is_a_warning_described_on_the_focused_button(self) -> None:
        self.assertIn("ShowSealPreviewBeforeSignNotice(InfoBarSeverity.Warning, unavailable)", self.ensure)
        described = self.ensure.index("SetFullDescription(VisibleSealPreviewButton, unavailable)")
        self.assertLess(described, self.ensure.index("VisibleSealPreviewButton.Focus("))
        hide = _between(self.page, "private void HideSealPreviewBeforeSignNotice(",
                        "private void ShowSignResult(")
        self.assertIn("SetFullDescription(VisibleSealPreviewButton, string.Empty)", hide)

    def test_warning_goes_away_when_no_longer_true(self) -> None:
        handler = _between(self.page, "private void OnViewModelPropertyChanged(",
                           "if (args.PropertyName == nameof(SignPageViewModel.SelectedCertificate))")
        self.assertIn("!ViewModel.NeedsVisibleSealPreviewBeforeSigning()", handler)
        self.assertIn("HideSealPreviewBeforeSignNotice()", handler)

    def test_second_click_does_not_start_another_wait(self) -> None:
        click = _between(self.page, "private async void OnSignClick(", "ViewModel.SignAsync(")
        self.assertIn("if (_waitingSealPreviewBeforeSign)", click)


class VerifyReportSavedTests(unittest.TestCase):
    def setUp(self) -> None:
        self.page = (UI / "Views" / "VerifyPage.xaml.cs").read_text(encoding="utf-8")
        self.vm = (UI / "ViewModels" / "VerifyPageViewModel.cs").read_text(encoding="utf-8")

    def test_saved_message_is_announced(self) -> None:
        self.assertIn("nameof(VerifyPageViewModel.ReportSavedMessage)", self.page)
        announce = _between(self.page, "private void AnnounceReportSaved()", "public VerifyPageViewModel ViewModel")
        # El ayudante común lanza LiveRegionChanged (véase test_live_region_contract).
        self.assertIn("LiveAnnouncer.Announce(ReportSavedText)", announce)

    def test_saved_message_includes_the_path(self) -> None:
        self.assertEqual(self.vm.count("PickAndSaveTextFileToPathAsync("), 2)
        self.assertIn('"winui.verificar.informe_guardado_en", savedPath', self.vm)
        self.assertIn('"winui.verificar.datos_tecnicos_guardados_en", savedPath', self.vm)


class ProtectDestinationProblemTests(unittest.TestCase):
    def test_move_or_picker_failure_says_where_the_document_is(self) -> None:
        vm = (UI / "ViewModels" / "ProtectPageViewModel.cs").read_text(encoding="utf-8")
        choose = _between(vm, "ChooseUnprotectedDestinationAsync(\n        string writtenPath",
                          "public OperationDiagnostic? ValidateProtectedOutputForOpening()")
        self.assertIn('"winui.proteger.selector_destino_no_disponible", writtenPath', choose)
        self.assertIn('"winui.proteger.no_se_pudo_guardar_en_destino", chosen, writtenPath', choose)
        self.assertIn("UnprotectResultMessage = destinationProblem ??", vm)


class CatalogTests(unittest.TestCase):
    def test_new_texts_exist_in_every_language_with_their_placeholders(self) -> None:
        expected = {
            "winui.verificar.informe_guardado_en": ["{0}"],
            "winui.verificar.datos_tecnicos_guardados_en": ["{0}"],
            "winui.proteger.no_se_pudo_guardar_en_destino": ["{0}", "{1}"],
            "winui.proteger.selector_destino_no_disponible": ["{0}"],
            "winui.selector.etiqueta_valor": ["{label}", "{value}"],
        }
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key, markers in expected.items():
                for marker in markers:
                    self.assertIn(marker, catalog[key], f"{path.name}: {key}")

    def test_picker_names_use_the_catalog_template(self) -> None:
        source = (UI / "Services" / "PickerLanguage.cs").read_text(encoding="utf-8")
        self.assertIn('"winui.selector.etiqueta_valor"', source)
        self.assertNotIn('": "', source)


if __name__ == "__main__":
    unittest.main()
