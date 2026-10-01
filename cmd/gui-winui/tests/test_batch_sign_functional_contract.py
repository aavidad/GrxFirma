# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
WINUI = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
)
CORE = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI.Core"
)


class BatchSignFunctionalContractTests(unittest.TestCase):
    def setUp(self) -> None:
        self.xaml = (
            WINUI / "Views" / "SignPage.xaml"
        ).read_text(encoding="utf-8")
        self.code = (
            WINUI / "Views" / "SignPage.xaml.cs"
        ).read_text(encoding="utf-8")
        self.vm = (
            WINUI / "ViewModels" / "SignPageViewModel.cs"
        ).read_text(encoding="utf-8")
        self.contracts = (
            CORE / "Operations" / "DesktopOperationContracts.cs"
        ).read_text(encoding="utf-8")
        self.client = (
            CORE / "Operations" / "DesktopOperationsClient.cs"
        ).read_text(encoding="utf-8")

    def test_xaml_exposes_multiple_files_folder_output_and_cancel(self) -> None:
        ET.parse(WINUI / "Views" / "SignPage.xaml")
        for fragment in (
            'AutomationProperties.Name="Activar firma por lotes"',
            'Click="OnSelectBatchFilesClick"',
            'Click="OnSelectBatchFolderClick"',
            'Click="OnSelectBatchOutputClick"',
            'Click="OnSignBatchClick"',
            "ViewModel.CanCancel",
            "ViewModel.BatchItems",
            "ViewModel.BatchProgressText",
            "ViewModel.IsBatchProgressIndeterminate",
        ):
            self.assertIn(fragment, self.xaml)

    def test_optional_batch_and_multicosign_details_follow_their_toggles(self) -> None:
        self.assertIn(
            'Visibility="{x:Bind ViewModel.BatchModeEnabled, Mode=OneWay}"',
            self.xaml,
        )
        self.assertIn(
            'Visibility="{x:Bind ViewModel.GuidedMultiCosignEnabled, Mode=OneWay}"',
            self.xaml,
        )
        for detail in (
            'Click="OnSelectBatchFilesClick"',
            'Click="OnSelectBatchFolderClick"',
            'Click="OnSelectBatchOutputClick"',
            'Text="{x:Bind ViewModel.BatchInputSummary, Mode=OneWay}"',
            'Text="{x:Bind ViewModel.BatchOutputSummary, Mode=OneWay}"',
            'x:Name="AdditionalSignersList"',
            'x:Name="BatchSignButton"\n                                Visibility="{x:Bind ViewModel.BatchModeEnabled, Mode=OneWay}"',
            'AutomationProperties.Name="Progreso de firma por lotes"',
            'Text="Resultado del lote"',
        ):
            self.assertIn(detail, self.xaml)

    def test_viewmodel_uses_real_batch_action_and_partial_results(self) -> None:
        for fragment in (
            "PickOpenFilesAsync(",
            "PickFolderAsync(",
            "DesktopOperationActions.SignBatch",
            "operations.SignBatchAsync(",
            "Outcome is not (\"success\" or \"partial\")",
            "TryApplyBatchResult(",
            "result.SuccessCount",
            "result.FailureCount",
            "HasNonEmptyOutput(item.OutputPath)",
            "IsDirectChildPath(",
            "MarkPendingBatchAsCancelled()",
            "MaximumBatchDocuments = 128",
            "MaximumBatchPayloadBytes",
        ):
            self.assertIn(fragment, self.vm)

    def test_batch_keeps_simple_and_multicosign_flows(self) -> None:
        for fragment in (
            "operations.SignAsync(",
            "operations.SignMultiCosignAsync(",
            "GuidedMultiCosignEnabled",
            "CanSign",
            "VisibleSealEnabled",
        ):
            self.assertIn(fragment, self.vm)
        self.assertIn(
            "!BatchModeEnabled &&",
            self.vm,
        )

    def test_typed_wire_contract_matches_go_action(self) -> None:
        for fragment in (
            'public const string SignBatch = "sign_batch";',
            "SignBatchAsync(",
            "BatchSignParameters",
            "BatchSignResult",
        ):
            self.assertIn(fragment, self.client)
        for wire_name in (
            "inputPaths",
            "directoryPath",
            "outputDir",
            "certificateId",
            "additionalCertificateIds",
            "format",
            "action",
            "overwrite",
            "documentOverrides",
            "results",
            "okCount",
            "failCount",
        ):
            self.assertIn(
                f'[JsonPropertyName("{wire_name}")]',
                self.contracts,
            )
        self.assertIn(
            "BatchSignItemResultListConverter",
            self.contracts,
        )

    def test_code_behind_routes_diagnostics_and_opens_only_validated_folder(
        self,
    ) -> None:
        for fragment in (
            "ViewModel.SelectBatchFilesAsync(",
            "ViewModel.SelectBatchFolderAsync(",
            "ViewModel.SelectBatchOutputDirectoryAsync(",
            "ViewModel.SignBatchAsync(",
            "ViewModel.ValidateBatchOutputForOpening()",
            "ShowDiagnosticIfPresentAsync(diagnostic)",
        ):
            self.assertIn(fragment, self.code)


if __name__ == "__main__":
    unittest.main()
