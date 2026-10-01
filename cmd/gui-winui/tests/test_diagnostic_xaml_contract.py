# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import re
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
DIALOG_PATH = (
    ROOT
    / "cmd/gui-winui/src/GrxFirma.WinUI/Controls"
    / "OperationDiagnosticDialog.xaml"
)
APP_PATH = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/App.xaml"
VIEW_MODEL_PATH = (
    ROOT
    / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels"
    / "OperationDiagnosticDialogViewModel.cs"
)
DIALOG = DIALOG_PATH.read_text(encoding="utf-8")
APP = APP_PATH.read_text(encoding="utf-8")
VIEW_MODEL = VIEW_MODEL_PATH.read_text(encoding="utf-8")


class DiagnosticXamlContractTest(unittest.TestCase):
    def test_xaml_is_well_formed(self):
        ET.parse(DIALOG_PATH)
        ET.parse(APP_PATH)

    def test_status_badge_scales_with_text(self):
        self.assertIn('MinWidth="38"', DIALOG)
        self.assertIn('MinHeight="38"', DIALOG)
        self.assertIn('Padding="8"', DIALOG)
        self.assertIsNone(re.search(r'(?<!Min)Width="38"', DIALOG))
        self.assertIsNone(re.search(r'(?<!Min)Height="38"', DIALOG))
        self.assertIn('<ColumnDefinition Width="Auto" />', DIALOG)

    def test_status_uses_semantic_brush_and_text(self):
        self.assertGreaterEqual(
            DIALOG.count(
                "Converter={StaticResource DiagnosticStatusBrushConverter}"
            ),
            3,
        )
        for key in (
            "AppDiagnosticSuccessBrush",
            "AppDiagnosticFailureBrush",
            "AppDiagnosticSkippedBrush",
            "AppDiagnosticUnknownBrush",
        ):
            self.assertIn(f'x:Key="{key}"', APP)
        self.assertIn('Text="{Binding StatusDisplayText}"', DIALOG)

    def test_basic_timeline_includes_owner_and_action(self):
        self.assertIn('Text="{Binding OwnerDisplayText}"', DIALOG)
        self.assertIn('Text="{Binding SuggestedActionDisplayText}"', DIALOG)
        self.assertIn(
            'AutomationProperties.Name="{Binding AutomationSummary}"',
            DIALOG,
        )
        self.assertRegex(
            DIALOG,
            r'<TextBlock\s+AutomationProperties\.HeadingLevel="Level3"\s+'
            r'AutomationProperties\.Name="\{Binding AutomationSummary\}"',
        )
        self.assertGreaterEqual(
            DIALOG.count('AutomationProperties.AccessibilityView="Raw"'),
            5,
        )
        self.assertGreaterEqual(
            DIALOG.count('AutomationProperties.HeadingLevel="'),
            5,
        )

    def test_support_code_is_only_inside_collapsed_technical_expander(self):
        expander_start = DIALOG.index("<Expander")
        code_position = DIALOG.index('Text="Código para soporte"')
        self.assertGreater(code_position, expander_start)
        self.assertIn('IsExpanded="False"', DIALOG[expander_start:])
        self.assertNotIn("Consola técnica", DIALOG)

    def test_cause_severity_comes_from_observed_status(self):
        self.assertIn(
            'Severity="{x:Bind ViewModel.CauseSeverity}"',
            DIALOG,
        )
        self.assertIn("InfoBarSeverity.Error", VIEW_MODEL)
        self.assertIn("InfoBarSeverity.Warning", VIEW_MODEL)
        self.assertNotIn("InfoBarSeverity.Success", VIEW_MODEL)


if __name__ == "__main__":
    unittest.main()
