# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
APP = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"
CORE = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI.Core"

XAML = APP / "Views" / "DiagnosticsPage.xaml"
CODE = APP / "Views" / "DiagnosticsPage.xaml.cs"
VIEW_MODEL = APP / "ViewModels" / "DiagnosticsPageViewModel.cs"
CONTRACTS = CORE / "Operations" / "DesktopDiagnosticContracts.cs"
CLIENT = CORE / "Operations" / "DesktopOperationsClient.cs"
GO_PROTOCOL = (
    ROOT
    / "internal"
    / "adapters"
    / "inbound"
    / "desktop"
    / "ipc"
    / "protocol.go"
)


class ActiveDiagnosticsFunctionalContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.xaml = XAML.read_text(encoding="utf-8")
        cls.code = CODE.read_text(encoding="utf-8")
        cls.view_model = VIEW_MODEL.read_text(encoding="utf-8")
        cls.contracts = CONTRACTS.read_text(encoding="utf-8")
        cls.client = CLIENT.read_text(encoding="utf-8")
        cls.go_protocol = GO_PROTOCOL.read_text(encoding="utf-8")

    def test_page_lifecycle_and_visual_timeline_are_explicit(self) -> None:
        root = ET.parse(XAML).getroot()
        self.assertEqual("OnLoaded", root.attrib["Loaded"])
        self.assertEqual("OnUnloaded", root.attrib["Unloaded"])
        self.assertIn("DiagnosticStatusBrushConverter", self.xaml)
        self.assertIn("Línea de fases del diagnóstico", self.xaml)
        self.assertIn("StatusIcon", self.xaml)
        self.assertIn("StatusDisplayText", self.xaml)
        self.assertIn("OwnerDisplayText", self.xaml)
        self.assertIn("SuggestedActionDisplayText", self.xaml)
        self.assertIn("OperationDiagnosticDialog", self.code)

    def test_honest_read_only_ipc_actions_drive_the_diagnostic(self) -> None:
        for action in (
            "ping",
            "check_certificates",
            "certificate_access_options",
            "proxy_secret_store_status",
            "tls_diagnostics",
            "clock_diagnostics",
            "export_diagnostic",
        ):
            self.assertIn(f'"{action}"', self.client)
            self.assertIn(f'"{action}"', self.go_protocol)

        run_method = self.view_model.split(
            "public async Task RunDiagnosticAsync",
            maxsplit=1,
        )[1].split(
            "private async Task ProbeCertificatesAsync",
            maxsplit=1,
        )[0]
        for forbidden in (
            "service_status",
            "clear_tls_trust",
            "service_install",
            "install_public_roots",
        ):
            self.assertNotIn(forbidden, run_method)

    def test_tls_mutations_are_explicit_and_confirmed(self) -> None:
        for action in (
            "install_public_roots",
            "clear_tls_trust",
        ):
            self.assertIn(f'"{action}"', self.client)
            self.assertIn(f'"{action}"', self.go_protocol)
        self.assertIn("OnInstallTlsTrustClick", self.xaml)
        self.assertIn("OnClearTlsTrustClick", self.xaml)
        self.assertIn("ContentDialog", self.code)
        self.assertIn("ContentDialogResult.Primary", self.code)
        self.assertIn("CanInstallTlsTrust", self.xaml)
        self.assertIn("CanClearTlsTrust", self.xaml)
        self.assertIn("TlsStoreDiagnosticResult", self.contracts)
        for forbidden in (
            "StoreDirectory",
            "CertificatePath",
            "KeyPath",
            "Thumbprint",
        ):
            self.assertNotIn(forbidden, self.contracts)

    def test_remote_portal_and_afirma_are_never_synthesized(self) -> None:
        self.assertIn(
            '"No comprobado: todavía no hay un origen HTTPS observado y autorizado disponible para esta sonda."',
            self.view_model,
        )
        self.assertIn(
            '"No comprobado: ninguna acción publicada prueba @firma de forma aislada."',
            self.view_model,
        )
        self.assertIn("ClockDiagnosticResult", self.contracts)
        self.assertIn("GetClockDiagnosticsAsync", self.client)
        self.assertNotIn("Endpoint", self.contracts)
        self.assertNotIn("Url", self.contracts)
        self.assertIn('"remote_service"', self.view_model)
        self.assertIn('"@firma"', self.view_model)
        self.assertIn(
            "OperationDiagnosticMapper.FromResult",
            self.view_model,
        )
        self.assertIn(
            "OperationDiagnosticMapper.FromException",
            self.view_model,
        )

    def test_unchecked_phases_do_not_claim_evidence(self) -> None:
        unknown_helper = self.view_model.split(
            "private static OperationDiagnosticStep UnknownStep",
            maxsplit=1,
        )[1].split(
            "private static OperationDiagnosticStep Step",
            maxsplit=1,
        )[0]
        self.assertNotIn("phase:", unknown_helper)
        self.assertIn("EvidenceRef = evidenceRef", self.view_model)
        self.assertIn(
            'evidenceRef: "phase:admission"',
            self.view_model,
        )
        self.assertIn(
            'evidenceRef: "phase:operation"',
            self.view_model,
        )

    def test_certificate_contracts_retain_counts_not_identities(self) -> None:
        self.assertIn(
            "record CheckCertificatesSummaryResult",
            self.contracts,
        )
        self.assertIn('JsonPropertyName("okCount")', self.contracts)
        self.assertIn('JsonPropertyName("failCount")', self.contracts)
        summary = self.contracts.split(
            "record CheckCertificatesSummaryResult",
            maxsplit=1,
        )[1].split(
            "record CertificateAccessOptionsParameters",
            maxsplit=1,
        )[0]
        for forbidden in (
            "CertificateInfo",
            "Subject",
            "Fingerprint",
            "CertificateId",
        ):
            self.assertNotIn(forbidden, summary)

        self.assertIn(
            "CertificateAccessInventoryListConverter",
            self.contracts,
        )
        self.assertIn(
            "OperationResultText.MaximumVisibleItems",
            self.contracts,
        )
        self.assertIn("reader.Skip();", self.contracts)

    def test_cancel_and_double_execution_are_bounded(self) -> None:
        self.assertIn(
            "Interlocked.CompareExchange(",
            self.view_model,
        )
        self.assertIn(
            "Volatile.Read(ref _operationCancellation)?.Cancel();",
            self.view_model,
        )
        self.assertIn(
            'IsEnabled="{x:Bind ViewModel.CanCancel, Mode=OneWay}"',
            self.xaml,
        )
        self.assertIn("ViewModel.Deactivate();", self.code)

    def test_mutable_xbinds_have_live_modes(self) -> None:
        mutable_names = (
            "PendingMessage",
            "HasResult",
            "Summary",
            "ResultSeverity",
            "OwnerLabel",
            "Responsibility",
            "SuggestedAction",
            "Steps",
            "Can",
            "IsBusy",
        )
        for line in self.xaml.splitlines():
            if "x:Bind ViewModel." not in line:
                continue
            if any(name in line for name in mutable_names):
                self.assertRegex(line, r"Mode=(?:OneWay|TwoWay)")

    def test_page_does_not_log_or_export_raw_diagnostics(self) -> None:
        sources = "\n".join(
            (self.xaml, self.code, self.view_model, self.contracts)
        )
        for forbidden in (
            "Console.Write",
            "Debug.Write",
            "Trace.Write",
            "StoreDirectory",
            "TrustLines",
            "DetectedBrowser",
            "PreferredManager",
            "Clipboard",
        ):
            self.assertNotIn(forbidden, sources)


if __name__ == "__main__":
    unittest.main()
