# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import re
import unittest
import xml.etree.ElementTree as ET

from winui_catalog import read_with_catalog


ROOT = pathlib.Path(__file__).resolve().parents[3]
APP = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"


class VerifyAndHashFunctionalContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.verify_vm = read_with_catalog(APP / "ViewModels/VerifyPageViewModel.cs")
        cls.hash_vm = read_with_catalog(APP / "ViewModels/HashPageViewModel.cs")
        cls.verify_page = read_with_catalog(APP / "Views/VerifyPage.xaml.cs")
        cls.hash_page = read_with_catalog(APP / "Views/HashPage.xaml.cs")
        cls.verify_xaml_path = APP / "Views/VerifyPage.xaml"
        cls.hash_xaml_path = APP / "Views/HashPage.xaml"
        cls.verify_xaml = cls.verify_xaml_path.read_text(encoding="utf-8")
        cls.hash_xaml = cls.hash_xaml_path.read_text(encoding="utf-8")

    def test_pages_use_app_session_picker_and_explicit_lifecycle(self):
        for source, xaml in (
            (self.verify_page, self.verify_xaml),
            (self.hash_page, self.hash_xaml),
        ):
            self.assertIn("app.OperationSession", source)
            self.assertIn("app.FilePickerService", source)
            self.assertIn("ViewModel.Activate()", source)
            self.assertIn("ViewModel.Deactivate()", source)
            self.assertIn('Loaded="OnLoaded"', xaml)
            self.assertIn('Unloaded="OnUnloaded"', xaml)
            self.assertIn("Mode=OneWay", xaml)

    def test_verify_calls_real_backend_and_never_invents_success(self):
        self.assertIn("DesktopOperationActions.Verify", self.verify_vm)
        self.assertIn("operations.VerifyAsync(", self.verify_vm)
        self.assertIn("new VerifyParameters", self.verify_vm)
        self.assertIn("!result.IsSuccess", self.verify_vm)
        self.assertIn('result.Outcome != "success"', self.verify_vm)
        self.assertIn("!IsCoherent(result.Data)", self.verify_vm)
        self.assertIn("PresentResult(result.Data!)", self.verify_vm)

    def test_explicit_original_is_explained_and_never_silently_ignored(self):
        self.assertIn("OriginalPath = _originalFilePath", self.verify_vm)
        self.assertIn("firma CAdES separada de su contenido", self.verify_xaml)
        self.assertIn("pulse Quitar", self.verify_xaml)
        self.assertIn("no se ignorará silenciosamente", self.verify_xaml)
        self.assertIn('Click="OnClearOriginalFileClick"', self.verify_xaml)

    def test_verify_explains_evidence_and_indeterminate_trust(self):
        for evidence in (
            "IntegritySummary",
            "CertificateSummary",
            "TrustSummary",
            "FormatSummary",
            "CoverageLabel",
            "VisibleSignerSummaries",
            "VisibleWarnings",
        ):
            self.assertIn(evidence, self.verify_vm)
        self.assertIn(
            "La confianza del certificado no ha sido determinada",
            self.verify_vm,
        )
        self.assertIn(
            '"Firma íntegra; confianza no determinada"',
            read_with_catalog(APP.parent / "GrxFirma.WinUI.Core/Operations/VerificationPresentation.cs"),
        )
        self.assertIn(
            "VerificationAssessment.HasEstablishedTrust(data)",
            self.verify_vm,
        )
        self.assertIn(
            "VerificationAssessment.HasValidSignatureEvidence(data)",
            self.verify_vm,
        )
        self.assertNotIn("ResultSeverity = !data.IsValid", self.verify_vm)
        self.assertIn('"unknown"', self.verify_vm)
        self.assertIn(".Take(MaximumVisibleItems)", self.verify_vm)

    def test_hash_only_offers_three_safe_algorithms(self):
        algorithm_block = re.search(
            r"Algorithms\s*\{\s*get;\s*\}\s*=\s*\[(.*?)\];",
            self.hash_vm,
            re.DOTALL,
        )
        self.assertIsNotNone(algorithm_block)
        self.assertEqual(
            re.findall(r'"([^"]+)"', algorithm_block.group(1)),
            ["SHA-256", "SHA-384", "SHA-512"],
        )
        self.assertNotIn("SHA-1", self.hash_xaml)

    def test_xml_compatibility_is_explicit_warning_not_signature_success(self):
        self.assertIn(
            "VerificationAssessment.HasXmlCanonicalizationCompatibility(data)",
            self.verify_vm,
        )
        self.assertIn(
            "ResultTitle = VerificationPresentation.GetTitle(data);",
            self.verify_vm,
        )
        self.assertRegex(
            self.verify_vm,
            r"ResultSeverity = hasXmlCompatibility\s*\? InfoBarSeverity.Warning",
        )
        self.assertIn("no se acredita su validez conforme a XMLDSig", self.verify_vm)
        self.assertIn("data.VisibleEvidence.Select(", self.verify_vm)

    def test_effective_reverification_clears_old_details_before_engine_request(self):
        method = self.verify_vm.split("public async Task VerifyAsync()", 1)[1].split(
            "public void CancelCurrentOperation()", 1)[0]
        reset = method.index("ResetResult();")
        self.assertLess(method.index("_operationCancellation = operationCancellation;"), reset)
        self.assertLess(reset, method.index('ResultTitle = "Verificando…";'))
        self.assertLess(reset, method.index("await operations.VerifyAsync("))
        self.assertNotIn("_signedFilePath =", method)
        self.assertNotIn("_originalFilePath =", method)
        reset_body = self.verify_vm.split("private void ResetResult()", 1)[1].split(
            "private void RequestDiagnostic", 1)[0]
        for cleared in ["Signers = [];", "Warnings = [];", "Details = [];",
                        'IntegritySummary = "Integridad: sin datos";',
                        'CertificateSummary = "Certificado: sin datos";',
                        'TrustSummary = "Confianza: sin datos";']:
            self.assertIn(cleared, reset_body)

    def test_hash_maps_real_file_and_directory_formats(self):
        for value in (
            'new("Hexadecimal GrxFirma (.hexhash)", "hex")',
            'new("Base64 GrxFirma (.hashb64)", "base64")',
            'new("Binario (.hash)", "bin")',
            'new("XML GrxFirma (.hashfiles)", "xml")',
            'new("Texto GrxFirma (.txthashfiles)", "txt")',
            'new("CSV (.csv)", "csv")',
        ):
            self.assertIn(value, self.hash_vm)
        self.assertIn("AvailableFormats", self.hash_xaml)
        self.assertIn('DisplayMemberPath="Label"', self.hash_xaml)

    def test_hash_create_check_and_safe_default_output_are_real(self):
        self.assertIn("operations.CreateHashAsync(", self.hash_vm)
        self.assertIn("operations.CheckHashAsync(", self.hash_vm)
        self.assertIn("new HashCreateParameters", self.hash_vm)
        self.assertIn("new HashCheckParameters", self.hash_vm)
        self.assertIn("OutputPath = string.Empty", self.hash_vm)
        self.assertIn("OutputPath = null", self.hash_vm)
        self.assertIn("SaveReportToDisk = IsDirectoryMode", self.hash_vm)
        self.assertIn("!IsCoherentCreateResult(result.Data)", self.hash_vm)
        self.assertIn("!IsCoherentCheckResult(result.Data)", self.hash_vm)
        self.assertIn("HasNonEmptyOutput(data.OutputPath)", self.hash_vm)
        self.assertIn("new FileInfo(path).Length > 0", self.hash_vm)

    def test_hash_report_is_output_never_an_input_manifest(self):
        report_mentions = [
            line.strip()
            for line in self.hash_vm.splitlines()
            if ".hashreport" in line
        ]
        self.assertTrue(report_mentions)
        self.assertFalse(
            any(
                "ManifestExtensions" in line or
                line.startswith('[".hashreport"')
                for line in report_mentions
            )
        )
        self.assertIn("!IsAllowedManifest(path)", self.hash_vm)
        self.assertIn(
            "Informe de comprobación:",
            self.hash_vm,
        )

    def test_operations_are_bounded_cancelable_and_visual_on_failure(self):
        for source in (self.verify_vm, self.hash_vm):
            self.assertIn("if (!Can", source)
            self.assertIn("SetBusy(true)", source)
            self.assertIn("SetBusy(false)", source)
            self.assertIn("CancelCurrentOperation", source)
            self.assertIn("OperationDiagnosticMapper.FromResult", source)
            self.assertIn("OperationDiagnosticMapper.FromException", source)

        for source in (self.verify_page, self.hash_page):
            self.assertIn("OperationDiagnosticDialog", source)
            self.assertIn("XamlRoot = XamlRoot", source)

        self.assertIn(".Take(MaximumVisibleResults)", self.hash_vm)
        self.assertIn("SafeIpcText.Clean(item", self.hash_vm)

    def test_xaml_is_well_formed_and_visual_results_are_live(self):
        ET.parse(self.verify_xaml_path)
        ET.parse(self.hash_xaml_path)
        for xaml in (self.verify_xaml, self.hash_xaml):
            self.assertIn("ResultSeverity, Mode=OneWay", xaml)
            self.assertIn("HasResult, Mode=OneWay", xaml)
            self.assertIn("AutomationProperties.LiveSetting", xaml)
            self.assertIn("ProgressRing", xaml)


if __name__ == "__main__":
    unittest.main()
