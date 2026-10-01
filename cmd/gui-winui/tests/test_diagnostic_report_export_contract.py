# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[3]
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Diagnostics"
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
DIALOG_XAML = UI / "Controls/OperationDiagnosticDialog.xaml"
DIALOG_CODE = UI / "Controls/OperationDiagnosticDialog.xaml.cs"
REPORT = CORE / "DiagnosticIncidentReport.cs"


class DiagnosticReportExportContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.xaml = DIALOG_XAML.read_text(encoding="utf-8")
        cls.code = DIALOG_CODE.read_text(encoding="utf-8")
        cls.report = REPORT.read_text(encoding="utf-8")

    def test_dialog_exposes_accessible_non_destructive_export_action(self) -> None:
        ET.parse(DIALOG_XAML)
        self.assertIn('PrimaryButtonText="Exportar informe"', self.xaml)
        self.assertIn('PrimaryButtonClick="OnExportReport"', self.xaml)
        self.assertIn(
            'AutomationProperties.Name="Estado de la exportación del informe"',
            self.xaml,
        )
        self.assertIn('AutomationProperties.LiveSetting="Polite"', self.xaml)
        self.assertIn("args.Cancel = true", self.code)
        self.assertIn("IsPrimaryButtonEnabled = false", self.code)

    def test_export_uses_closed_diagnostic_profile_and_safe_messages(self) -> None:
        self.assertIn(
            "SaveFilePickerProfile.DiagnosticReport",
            self.code,
        )
        self.assertIn("DiagnosticIncidentReport.CreateJson", self.code)
        self.assertIn("PickAndSaveTextFileAsync", self.code)
        for forbidden in (
            "exception.Message",
            "exception.StackTrace",
            "File.Write",
            "Process.Start",
        ):
            self.assertNotIn(forbidden, self.code)

    def test_report_is_bounded_to_sanitized_presentation_contract(self) -> None:
        self.assertIn(
            "OperationDiagnosticMapper.Sanitize(diagnostic)",
            self.report,
        )
        self.assertIn(
            "OperationDiagnosticPresentation.From(",
            self.report,
        )
        self.assertIn(
            'Schema = "grxfirma-winui-incident-v1"',
            self.report,
        )
        for forbidden in (
            "EvidenceRef",
            "UnsafeError",
            "Payload",
            "Certificate",
            "Signature",
            "StackTrace",
        ):
            self.assertNotIn(forbidden, self.report)


if __name__ == "__main__":
    unittest.main()
