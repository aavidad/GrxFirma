# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Source contracts; actual scrolling/focus still requires the Windows runner."""
import pathlib
import unittest
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[1] / "src/GrxFirma.WinUI"


class SignResultRevealContracts(unittest.TestCase):
    def setUp(self):
        self.view = (ROOT / "Views/SignPage.xaml.cs").read_text(encoding="utf-8")
        self.model = (ROOT / "ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
        self.sign = self.model.split("public async Task<OperationDiagnostic?> SignAsync(", 1)[1].split(
            "public OperationDiagnostic? ValidateOutputForOpening", 1)[0]
        self.click = self.view.split("private async void OnSignClick(", 1)[1].split(
            "private async void OnSignBatchClick(", 1)[0]

    def test_only_terminal_success_creates_completion_identity(self):
        self.assertEqual(self.model.count("CompletedSignPresentationId = Guid.NewGuid();"), 1)
        self.assertIn("CompletedSignPresentationId = Guid.NewGuid();", self.sign)
        self.assertLess(self.sign.index("PostValidationSuccessMessage("), self.sign.index("Guid.NewGuid()"))
        self.assertIn("operationCancellation.Token.ThrowIfCancellationRequested();\n            CompletedSignPresentationId", self.sign)
        self.assertNotIn("Guid.NewGuid()", self.sign.split("catch (OperationCanceledException)", 1)[1])

    def test_cancel_new_operation_and_changed_selection_invalidate(self):
        for start, end in [("public void CancelCurrentOperation()", "public async Task"),
                           ("private bool TryBeginOperation(", "private void EndOperation"),
                           ("private void ClearOutput()", "private void SetBusy")]:
            body = self.model.split(start, 1)[1].split(end, 1)[0]
            self.assertIn("CompletedSignPresentationId = Guid.Empty;", body)

    def test_stale_output_and_diagnostics_are_not_success(self):
        self.assertIn("diagnostic is null && completion != Guid.Empty && completion != previousCompletion", self.click)
        self.assertIn("completion != ViewModel.CompletedSignPresentationId", self.click)
        self.assertIn("await ShowDiagnosticIfPresentAsync(diagnostic);", self.click)

    def test_queued_reveal_checks_lifetime_and_operation(self):
        for guard in ["!_isSubscribed", "!ReferenceEquals(_pageCancellation, cancellation)",
                      "cancellation.IsCancellationRequested", "revealRevision != _resultRevealRevision",
                      "ViewModel.IsBusy", "!ViewModel.CanOpenOutput", "XamlRoot is null"]:
            self.assertIn(guard, self.click)
        for start in ["private void OnUnloaded(", "private async Task ShowDiagnosticAsync("]:
            self.assertIn("++_resultRevealRevision;", self.view.split(start, 1)[1].split("\n    }", 1)[0])

    def test_reveal_focuses_primary_action_without_opening_it(self):
        self.assertIn("StartBringIntoView", self.click)
        self.assertIn("AnimationDesired = false", self.click)
        self.assertIn("ResultButton.Focus(FocusState.Programmatic)", self.click)
        self.assertIn("ReviewSignButton.Focus(FocusState.Programmatic)", self.click)
        for forbidden in [".Navigate(", "OnOpenResultClick(", "LaunchFileAsync("]:
            self.assertNotIn(forbidden, self.click)

    def test_result_contains_wrapped_trust_detail_without_duplicate_live_announcement(self):
        root = ET.parse(ROOT / "Views/SignPage.xaml").getroot()
        parent = next(e for e in root.iter() if any(c.get("Text") == "Resultado" for c in e))
        detail = next(c for c in parent.iter() if c.get("AutomationProperties.Name") == "Detalle de validación del resultado")
        self.assertIn("ViewModel.ValidationMessage", detail.get("Text"))
        self.assertEqual(detail.get("TextWrapping"), "Wrap")
        self.assertIsNone(detail.get("AutomationProperties.LiveSetting"))
        notice = next(c for c in parent.iter() if c.get("{http://schemas.microsoft.com/winfx/2006/xaml}Name") == "SignResultNotice")
        self.assertEqual(notice.get("AutomationProperties.LiveSetting"), "Assertive")
        self.assertEqual(sum(e.get("AutomationProperties.LiveSetting") == "Assertive" for e in parent.iter()), 1)

    def test_previous_output_cannot_be_presented_as_new_success(self):
        self.assertIn("ShowSignResult(null, false)", self.click)
        self.assertIn("outputFromThisAttempt && ViewModel.CanOpenOutput", self.click)
        self.assertIn("signAttemptStarted && ViewModel.CanOpenOutput", self.click)
        self.assertNotIn('ResultMessage.StartsWith("La firma se guardó"', self.click)

    def test_preflight_failure_with_old_completion_is_visible(self):
        self.assertIn("(completion == Guid.Empty || completion == previousCompletion)", self.click)
        self.assertIn("change.PropertyName == nameof(SignPageViewModel.IsBusy)", self.click)
        self.assertIn("ViewModel.PropertyChanged -= ObserveSignProgress", self.click)

    def test_changed_pdf_is_not_announced_as_success(self):
        self.assertIn('"PDF_CHANGED_DURING_SIGN"', self.click)
        self.assertIn("SignResultNotice.Severity = hasOutput && !unsafeOutput", self.click)
        self.assertIn("? InfoBarSeverity.Success : InfoBarSeverity.Error", self.click)
        self.assertIn('"Resultado guardado: no utilice el documento"', self.click)
        self.assertIn("diagnostic.SuggestedAction", self.click)

    def test_live_result_and_output_actions(self):
        self.assertIn("AutomationEvents.LiveRegionChanged", self.view)
        self.assertIn("OnOpenResultFolderClick", self.view)
        self.assertIn("ValidateOutputForOpening()", self.view)
        self.assertIn("UseShellExecute = true", self.view)

    def test_certificate_picker_and_manual_sidebar_reclaim_space(self):
        xaml = (ROOT / "Views/SignPage.xaml").read_text(encoding="utf-8")
        self.assertNotIn('x:Name="CertificateCompactPanel"', xaml)
        self.assertIn('x:Name="CentralCertificatePickerPanel"', xaml)
        self.assertIn('AutomationProperties.Name="Ver todos los certificados"', xaml)
        self.assertIn('Content="Ocultar"', xaml)
        self.assertIn('CertificateSidebar.Visibility = !_certificatePanelExpanded', self.view)
        self.assertIn('CertificatePanelColumn.Width = new GridLength(sidebarWidth)', self.view)

    def test_certificate_layout_does_not_depend_on_selection(self):
        layout = self.view.split("private void UpdateCertificatePanelLayout()", 1)[1].split(
            "private void OnCertificatePanelPropertyChanged(", 1)[0]
        self.assertNotIn("selected is { IsSuitable: true }", layout)
        self.assertIn("SignLayoutGrid.ColumnSpacing = sidebarWidth > 0 ? 20 : 0", layout)
        certificate_model = (ROOT / "ViewModels/CertificatesPageViewModel.cs").read_text(encoding="utf-8")
        self.assertIn("IsSuitable = suitable && !certificate.NeedsUnlock", certificate_model)


if __name__ == "__main__":
    unittest.main()
