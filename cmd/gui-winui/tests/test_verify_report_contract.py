# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""Verificar: informe HTML como acción principal, firmante y foco (0.0.117, M2-M3)."""

import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
UI = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"
CORE = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI.Core"


class VerifyReportContractTests(unittest.TestCase):
    def test_engine_html_report_is_the_primary_export(self) -> None:
        vm = (UI / "ViewModels" / "VerifyPageViewModel.cs").read_text(encoding="utf-8")
        contracts = (CORE / "Operations" / "DesktopOperationContracts.cs").read_text(encoding="utf-8")
        picker = (UI / "Services" / "WindowsFilePickerService.cs").read_text(encoding="utf-8")
        self.assertIn('[JsonPropertyName("reportLanguage")]', contracts)
        self.assertIn('[JsonPropertyName("reportHtml")]', contracts)
        self.assertIn("ReportLanguage = Localizer.Language,", vm)
        self.assertIn("SaveFilePickerProfile.VerificationReportHtml, _reportResult.ReportHtml", vm)
        self.assertIn("public async Task ExportJsonReportAsync()", vm)
        self.assertIn('"winui.parity.verify.filename"), [".html"]', picker)

    def test_result_names_the_signer_and_takes_the_focus(self) -> None:
        vm = (UI / "ViewModels" / "VerifyPageViewModel.cs").read_text(encoding="utf-8")
        page = (UI / "Views" / "VerifyPage.xaml.cs").read_text(encoding="utf-8")
        xaml = (UI / "Views" / "VerifyPage.xaml").read_text(encoding="utf-8")
        self.assertIn("DistinguishedNameText.CommonName", vm)
        self.assertIn('"winui.verificar.firmado_por"', vm)
        self.assertIn("masculine: true", vm)
        self.assertIn("ViewModel.SignedBySummary", xaml)
        self.assertIn("ResultInfoBar.Focus(FocusState.Programmatic);", page)


if __name__ == "__main__":
    unittest.main()
