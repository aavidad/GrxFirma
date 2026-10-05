# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import unittest
import xml.etree.ElementTree as ET

from winui_catalog import read_with_catalog


ROOT = pathlib.Path(__file__).resolve().parents[3]
APP = ROOT / "cmd" / "gui-winui" / "src" / "GrxFirma.WinUI"

CERTIFICATES_XAML = APP / "Views" / "CertificatesPage.xaml"
CERTIFICATES_CODE = APP / "Views" / "CertificatesPage.xaml.cs"
CERTIFICATES_VM = APP / "ViewModels" / "CertificatesPageViewModel.cs"
SIGN_XAML = APP / "Views" / "SignPage.xaml"
SIGN_CODE = APP / "Views" / "SignPage.xaml.cs"
SIGN_VM = APP / "ViewModels" / "SignPageViewModel.cs"
CERTIFICATE_DIALOG = APP / "Controls" / "CertificateValidationDialog.cs"
HELP_LAUNCHER = APP / "Services" / "WindowsHelpLauncherService.cs"
CREDENTIAL_FILE = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI.Core"
    / "Operations"
    / "DesktopCertificateCredentialFile.cs"
)
GO_IPC_HANDLER = (
    ROOT
    / "internal"
    / "adapters"
    / "inbound"
    / "desktop"
    / "ipc"
    / "handler.go"
)
QT_MAIN = ROOT / "cmd" / "gui-qml" / "qml" / "main.qml"


class CertificatesAndSignFunctionalContractTests(unittest.TestCase):
    def setUp(self) -> None:
        self.certificates_xaml = CERTIFICATES_XAML.read_text(encoding="utf-8")
        self.certificates_code = read_with_catalog(CERTIFICATES_CODE)
        self.certificates_vm = read_with_catalog(CERTIFICATES_VM)
        self.sign_xaml = SIGN_XAML.read_text(encoding="utf-8")
        self.sign_code = read_with_catalog(SIGN_CODE)
        self.sign_vm = read_with_catalog(SIGN_VM)
        self.credential_file = read_with_catalog(CREDENTIAL_FILE)

    def test_xaml_is_well_formed_and_lifecycle_is_explicit(self) -> None:
        for path in (CERTIFICATES_XAML, SIGN_XAML):
            root = ET.parse(path).getroot()
            self.assertEqual("OnLoaded", root.attrib["Loaded"])
            self.assertEqual("OnUnloaded", root.attrib["Unloaded"])

        for code in (self.certificates_code, self.sign_code):
            self.assertIn(
                "_session.AvailabilityChanged += OnAvailabilityChanged;",
                code,
            )
            self.assertIn(
                "_session.AvailabilityChanged -= OnAvailabilityChanged;",
                code,
            )
            self.assertIn("ViewModel.CancelCurrentOperation();", code)

    def test_every_mutable_xbind_declares_a_live_mode(self) -> None:
        mutable_names = (
            "PendingMessage",
            "IsOperationConnected",
            "Can",
            "FilterText",
            "CatalogMessage",
            "VisibleCertificates",
            "SelectedCertificate",
            "Certificates",
            "AdditionalSignerCandidates",
            "SelectedAction",
            "SelectedFormat",
            "SelectedProfile",
            "InputDisplayName",
            "ValidationMessage",
            "ResultMessage",
            "IsBusy",
        )
        for xaml in (self.certificates_xaml, self.sign_xaml):
            for line in xaml.splitlines():
                if "x:Bind ViewModel." not in line:
                    continue
                if not any(name in line for name in mutable_names):
                    continue
                self.assertRegex(line, r"Mode=(?:OneWay|TwoWay)")

    def test_certificate_catalog_is_real_filterable_and_id_stable(self) -> None:
        self.assertIn(
            "operations.GetCertificatesAsync(",
            self.certificates_vm,
        )
        self.assertIn(
            "certificate.Id",
            self.certificates_vm,
        )
        self.assertIn(
            "GroupBy(certificate =>",
            self.certificates_vm,
        )
        self.assertIn(
            "StringComparison.CurrentCultureIgnoreCase",
            self.certificates_vm,
        )
        self.assertIn(
            'ItemsSource="{x:Bind ViewModel.VisibleCertificates, Mode=OneWay}"',
            self.certificates_xaml,
        )
        self.assertIn(
            'SelectedItem="{x:Bind ViewModel.SelectedCertificate, Mode=TwoWay}"',
            self.certificates_xaml,
        )

    def test_sign_certificate_panel_keeps_the_signing_selector_and_shared_actions(
        self,
    ) -> None:
        root = ET.parse(SIGN_XAML).getroot()
        names = {
            element.attrib.get("AutomationProperties.Name")
            for element in root.iter()
        }
        for name in (
            "Certificado de firma",
            "Buscar certificado",
            "Certificados para firma",
            "Refrescar validez online del certificado",
            "Abrir VALIDe",
        ):
            self.assertIn(name, names)
        self.assertIn('SelectedItem="{x:Bind ViewModel.SelectedCertificate, Mode=TwoWay}"', self.sign_xaml)
        self.assertIn('SelectedItem="{x:Bind CertificatePanel.SelectedCertificate, Mode=TwoWay}"', self.sign_xaml)
        self.assertIn("CertificatePanel.ValidateSelectedCertificateAsync(", self.sign_code)
        self.assertIn("CertificatePanel.SetSelectedAsDefaultAsync(", self.sign_code)
        self.assertIn("CertificateValidationDialog.ShowAsync(", self.sign_code)
        self.assertIn("certificate.Details", CERTIFICATE_DIALOG.read_text(encoding="utf-8"))
        launcher = HELP_LAUNCHER.read_text(encoding="utf-8")
        self.assertIn('"https://valide.redsara.es/valide/"', launcher)
        self.assertIn('uri.Host, "valide.redsara.es"', launcher)

    def test_import_uses_native_secret_flow_and_modern_actions(self) -> None:
        self.assertIn('x:Name="ImportButton"', self.certificates_xaml)
        import_fragment = self.certificates_xaml.split(
            'x:Name="ImportButton"',
            maxsplit=1,
        )[1].split("/>", maxsplit=1)[0]
        self.assertIn(
            "ViewModel.CanSelectCredential",
            import_fragment,
        )
        self.assertIn(
            'Click="OnSelectImportCredentialClick"',
            import_fragment,
        )
        for fragment in (
            "ISecurePasswordPromptService",
            "_securePasswordPrompt.CaptureAsync(",
            "NativePasswordEncoding.ToUtf8(captured)",
            "CryptographicOperations.ZeroMemory(password)",
            "ViewModel.ImportSelectedCredentialAsync(",
        ):
            self.assertIn(fragment, self.certificates_code)
        for fragment in (
            "operations.UseTemporaryCertificateAsync(",
            "operations.ImportCertificateToStoreAsync(",
            "CryptographicOperations.ZeroMemory(credential)",
            "CryptographicOperations.ZeroMemory(password)",
            "DesktopCertificateCredentialFile.ReadAsync(",
        ):
            self.assertIn(fragment, self.certificates_vm)
        for fragment in (
            "DesktopOperationsClient.MaximumCredentialBytes",
            "CryptographicOperations.ZeroMemory(credential)",
            "FileShare.Read",
            "FileOptions.SequentialScan",
        ):
            self.assertIn(fragment, self.credential_file)
        self.assertNotIn("import_certificate\"", self.certificates_vm)

    def test_sign_selector_offers_system_temporary_and_windows_import(self) -> None:
        for fragment in (
            'AutomationProperties.Name="Buscar en Windows"',
            'AutomationProperties.Name="Cargar P12/PFX…"',
            'AutomationProperties.Name="Importar archivo en Windows"',
            'Click="OnRefreshCertificatesClick"',
            'Click="OnUseTemporaryCredentialClick"',
            'Click="OnImportCredentialToWindowsClick"',
            'Text="{Binding TemporaryDisplay}"',
        ):
            self.assertIn(fragment, self.sign_xaml)

        for fragment in (
            "_securePasswordPrompt.CaptureAsync(",
            "NativePasswordEncoding.ToUtf8(captured)",
            "ConfirmWindowsCredentialImportAsync()",
            "CryptographicOperations.ZeroMemory(password)",
            "ViewModel.UsePreparedCredentialAsync(",
            "ViewModel.DiscardPreparedCredential();",
        ):
            self.assertIn(fragment, self.sign_code)

        for fragment in (
            'WindowsPersonalStoreTargetId = "windows-my"',
            "operations.UseTemporaryCertificateAsync(",
            "operations.GetCertificateImportOptionsAsync(",
            "operations.ImportCertificateToStoreAsync(",
            "DesktopCertificateCredentialFile.ReadAsync(",
            "_session.TrackTemporaryCertificate(",
            "_session.IsTemporaryCertificateTracked(group.Key)",
            "CryptographicOperations.ZeroMemory(credential)",
            "CryptographicOperations.ZeroMemory(password)",
        ):
            self.assertIn(fragment, self.sign_vm)
        self.assertNotIn("PasswordBox", self.sign_xaml)

    def test_temporary_credentials_follow_the_shared_session_lifecycle(
        self,
    ) -> None:
        self.assertNotIn(
            "HashSet<string> _temporaryCertificateIds",
            self.certificates_vm,
        )
        for fragment in (
            "_session.TrackTemporaryCertificate(result.Data.Id)",
            "_session.IsTemporaryCertificateTracked(group.Key)",
            "_session.UntrackTemporaryCertificate(certificate.Id)",
            "_session.ClearTrackedTemporaryCertificates()",
            "RemoveCertificateFromCatalog(certificate.Id)",
            "RemoveTrackedTemporaryCertificatesFromCatalog()",
        ):
            self.assertIn(fragment, self.certificates_vm)

        clear_state = self.certificates_vm.split(
            "CanClearTemporaryCertificates =",
            maxsplit=1,
        )[1].split(";", maxsplit=1)[0]
        self.assertIn(
            "DesktopOperationActions.ClearTemporaryCertificates",
            clear_state,
        )
        self.assertNotIn("Count", clear_state)

    def test_certificate_actions_use_typed_ipc_and_visual_diagnostics(self) -> None:
        self.assertIn(
            'Click="OnValidateCertificateClick"',
            self.certificates_xaml,
        )
        self.assertIn(
            "ViewModel.CanValidateSelectedCertificate",
            self.certificates_xaml,
        )
        self.assertIn(
            'Click="OnOpenCertificateManagerClick"',
            self.certificates_xaml,
        )
        self.assertIn(
            "ViewModel.CanOpenCertificateManager",
            self.certificates_xaml,
        )
        self.assertIn(
            "operations.ValidateCertificateOnlineAsync(",
            self.certificates_vm,
        )
        self.assertIn(
            "operations.GetCertificateImportOptionsAsync(",
            self.certificates_vm,
        )
        self.assertIn(
            "operations.OpenCertificateManagerAsync(",
            self.certificates_vm,
        )
        self.assertIn(
            'WindowsCertificateManagerId =\n        "windows-certmgr"',
            self.certificates_vm,
        )
        self.assertIn(
            "OperationDiagnosticMapper.FromResult(result)",
            self.certificates_vm,
        )
        self.assertIn(
            "await ShowDiagnosticAsync(diagnostic);",
            self.certificates_code,
        )
        self.assertNotIn("Process.Start", self.certificates_vm)
        self.assertNotIn("UseShellExecute", self.certificates_vm)

    def test_default_certificate_uses_typed_conservative_persistence(self) -> None:
        default_fragment = self.certificates_xaml.split(
            'AutomationProperties.Name="Usar como predeterminado"',
            maxsplit=1,
        )[1].split("</Button>", maxsplit=1)[0]
        self.assertIn('Click="OnSetDefaultCertificateClick"', default_fragment)
        self.assertIn(
            "ViewModel.CanSetSelectedAsDefault",
            default_fragment,
        )
        for fragment in (
            "ViewModel.SetSelectedAsDefaultAsync(",
            "operations.GetSettingsAsync(",
            "settingsResult.Data.CreateSafeSaveSnapshot() with",
            "PreferDefaultCertificate = true",
            "DefaultCertificateId = certificate.Id",
            "operations.SaveSettingsAsync(",
            "string.IsNullOrWhiteSpace(saveResult.Data)",
            "ApplyDefaultCertificate(certificate.Id)",
            "Guardado cancelado",
        ):
            self.assertIn(
                fragment,
                self.certificates_code
                if fragment.startswith("ViewModel.")
                else self.certificates_vm,
            )
        self.assertIn(
            'Text="{Binding DefaultDisplay}"',
            self.certificates_xaml,
        )

    def test_sign_uses_explicit_wire_values_and_stable_certificate_id(self) -> None:
        for label, value in (
            ("Firma", "sign"),
            ("Cofirma", "cosign"),
            ("Contrafirma", "countersign"),
            ("PAdES", "pades"),
            ("CAdES", "cades"),
            ("XAdES", "xades"),
            ("ODF", "odf"),
            ("OOXML", "ooxml"),
            ("FacturaE", "facturae"),
            ("ASiC-XAdES", "asic-xades"),
        ):
            self.assertIn(f'new("{label}", "{value}"', self.sign_vm)

        self.assertIn(
            'new("XMLdSig", "xmldsig", SaveFilePickerProfile.XmlDsigSignature)',
            self.sign_vm,
        )
        self.assertIn("CertificateId = certificate.Id", self.sign_vm)
        self.assertRegex(
            self.sign_vm,
            r'visibleSeal is null\s+\? format\.Value\s+: "pades"',
        )
        self.assertIn("Action = action.Value", self.sign_vm)
        self.assertNotIn("Format = format.Label", self.sign_vm)
        self.assertNotIn("Action = action.Label", self.sign_vm)
        self.assertIn("ReturnSignatureBase64 = false", self.sign_vm)

    def test_guided_multicosign_uses_real_ipc_and_distinct_stable_ids(
        self,
    ) -> None:
        self.assertIn(
            'AutomationProperties.Name="Activar cofirma múltiple guiada"',
            self.sign_xaml,
        )
        self.assertIn(
            'IsChecked="{x:Bind ViewModel.GuidedMultiCosignEnabled, Mode=TwoWay}"',
            self.sign_xaml,
        )
        self.assertIn(
            'ItemsSource="{x:Bind ViewModel.AdditionalSignerCandidates, Mode=OneWay}"',
            self.sign_xaml,
        )
        self.assertIn(
            'SelectionChanged="OnAdditionalSignersSelectionChanged"',
            self.sign_xaml,
        )
        self.assertIn('SelectionMode="Multiple"', self.sign_xaml)
        self.assertIn(
            "ViewModel.SetSelectedAdditionalSigners(",
            self.sign_code,
        )
        self.assertIn(
            "AdditionalSignersList.SelectedItems",
            self.sign_code,
        )

        self.assertIn(
            "_session.Supports(DesktopOperationActions.SignMultiCosign)",
            self.sign_vm,
        )
        self.assertIn(
            "DesktopOperationActions.SignMultiCosign",
            self.sign_vm,
        )
        self.assertIn(
            "operations.SignMultiCosignAsync(",
            self.sign_vm,
        )
        self.assertIn(
            "AdditionalCertificateIds = useGuidedMultiCosign",
            self.sign_vm,
        )
        self.assertIn(
            "_selectedAdditionalCertificateIds.ToArray()",
            self.sign_vm,
        )
        self.assertIn(
            "!string.Equals(\n                    certificate.Id,\n                    primaryId,",
            self.sign_vm,
        )
        self.assertIn(".Distinct(StringComparer.Ordinal)", self.sign_vm)
        self.assertIn(
            '"pades",\n                StringComparison.OrdinalIgnoreCase',
            self.sign_vm,
        )
        self.assertIn(
            '"odf",\n                StringComparison.OrdinalIgnoreCase',
            self.sign_vm,
        )
        self.assertIn(
            '"ooxml",\n                StringComparison.OrdinalIgnoreCase',
            self.sign_vm,
        )
        self.assertIn(
            '"La cofirma múltiple guiada no admite contrafirma."',
            self.sign_vm,
        )
        self.assertIn(
            "Seleccione al menos un certificado adicional distinto del firmante principal.",
            self.sign_vm,
        )
        self.assertIn("Action = action.Value", self.sign_vm)

    def test_wire_actions_and_output_extensions_match_go_and_qt(self) -> None:
        go_handler = GO_IPC_HANDLER.read_text(encoding="utf-8")
        qt_main = QT_MAIN.read_text(encoding="utf-8")

        for value in ("sign", "cosign", "countersign"):
            self.assertIn(f'valor: "{value}"', qt_main)
            self.assertIn(f'"{value}"', self.sign_vm)

        for go_case, suffix in (
            ('case "pades":', '_firmado.pdf'),
            ('case "xmldsig":', '_firmado.dsig'),
            ('case "xades":', '_firmado.xsig'),
        ):
            self.assertIn(go_case, go_handler)
            self.assertIn(suffix, go_handler)
            self.assertIn(suffix, qt_main)
        self.assertIn('_firmado.p7s', go_handler)
        self.assertIn('_firmado.p7s', qt_main)

        self.assertNotIn("HasSafeOutputExtension", self.sign_vm)
        self.assertIn(
            'new("ASiC-XAdES", "asic-xades", SaveFilePickerProfile.CadesSignature)',
            self.sign_vm,
        )

    def test_sign_requires_pickers_and_verified_output_before_success(self) -> None:
        self.assertIn(
            "PickOpenFileAsync(",
            self.sign_vm,
        )
        self.assertIn(
            "OpenFilePickerProfile.SignedOrOriginalDocument",
            self.sign_vm,
        )
        self.assertIn(
            "PickSaveFileAsync(",
            self.sign_vm,
        )
        self.assertIn(
            "!PathsEqual(result.Data.OutputPath, outputPath)",
            self.sign_vm,
        )
        self.assertIn("!HasNonEmptyOutput(outputPath)", self.sign_vm)
        self.assertIn("new FileInfo(path).Length > 0", self.sign_vm)
        self.assertIn(
            'new("ASiC-XAdES", "asic-xades", SaveFilePickerProfile.CadesSignature)',
            self.sign_vm,
        )
        self.assertIn('".xsig"', self.sign_vm)
        self.assertLess(
            self.sign_vm.index("!HasNonEmptyOutput(outputPath)"),
            self.sign_vm.index("Firma completada y guardada como"),
        )
        self.assertIn(
            "OperationDiagnosticMapper.FromResult(result)",
            self.sign_vm,
        )
        self.assertIn(
            "OperationDiagnosticMapper.FromException(exception)",
            self.sign_vm,
        )
        self.assertIn(
            "new OperationDiagnosticDialog(diagnostic)",
            self.sign_code,
        )

    def test_visible_pdf_seal_uses_real_preview_and_typed_sign_payload(
        self,
    ) -> None:
        self.assertIn(
            'IsChecked="{x:Bind ViewModel.VisibleSealEnabled, Mode=TwoWay}"',
            self.sign_xaml,
        )
        self.assertIn(
            'Click="OnRefreshVisibleSealPreviewClick"',
            self.sign_xaml,
        )
        self.assertIn(
            "ViewModel.CanConfigureVisibleSeal",
            self.sign_xaml,
        )
        self.assertNotIn(
            "Añadir sello visible en PDF — no disponible todavía",
            self.sign_xaml,
        )
        for accessible_name in (
            "Páginas del sello visible",
            "Posición horizontal del sello en porcentaje",
            "Posición vertical del sello en porcentaje",
            "Ancho del sello en porcentaje",
            "Alto del sello en porcentaje",
            "Mostrar identidad ubicación y fecha en el sello",
        ):
            self.assertIn(
                f'AutomationProperties.Name="{accessible_name}"',
                self.sign_xaml,
            )

        self.assertIn(
            "_session.Supports(DesktopOperationActions.PdfPreview)",
            self.sign_vm,
        )
        self.assertIn(
            "await _pdfPreview.RenderPageAsync(",
            self.sign_vm,
        )
        self.assertIn(
            "operations!.GetPdfPreviewAsync(",
            self.sign_vm,
        )
        self.assertLess(
            self.sign_vm.index("await _pdfPreview.RenderPageAsync("),
            self.sign_vm.index("operations!.GetPdfPreviewAsync("),
        )
        self.assertIn(
            "Path = inputPath!",
            self.sign_vm,
        )
        self.assertIn(
            "VisibleSeal = visibleSeal",
            self.sign_vm,
        )
        self.assertRegex(
            self.sign_vm,
            r'visibleSeal is null\s+\? format\.Value\s+: "pades"',
        )
        self.assertIn(
            "PageWidth = _previewPageWidth",
            self.sign_vm,
        )
        self.assertIn(
            "PageHeight = _previewPageHeight",
            self.sign_vm,
        )
        self.assertIn(
            "!PathsEqual(_previewInputPath, _inputPath)",
            self.sign_vm,
        )
        self.assertIn(
            "VISIBLE_SEAL_GEOMETRY_INVALID",
            self.sign_vm,
        )
        self.assertIn(
            "La firma no se inició porque la configuración no era segura.",
            self.sign_vm,
        )
        self.assertIn(
            "ViewModel.PreviewImageRenderingFailed()",
            self.sign_code,
        )
        self.assertIn(
            "CryptographicOperations.FixedTimeEquals(",
            self.sign_vm,
        )
        self.assertIn(
            "ComputeSha256Async(",
            self.sign_vm,
        )
        self.assertIn(
            "Share = FileShare.Read",
            self.sign_vm,
        )
        self.assertIn(
            "CryptographicOperations.ZeroMemory(",
            self.sign_vm,
        )
        self.assertIn(
            "ClearBytes(ipcResult.Data?.Data);",
            self.sign_vm,
        )
        self.assertIn(
            "ClearBytes(result.Data);",
            self.sign_vm,
        )
        self.assertIn(
            "ViewModel.DiscardVisibleSealPreview();",
            self.sign_code,
        )

    def test_post_sign_validation_uses_real_verify_and_keeps_failures_visible(
        self,
    ) -> None:
        self.assertIn(
            'IsChecked="{x:Bind ViewModel.ValidateAfterSigning, Mode=TwoWay}"',
            self.sign_xaml,
        )
        self.assertIn(
            'IsEnabled="{x:Bind ViewModel.CanValidateAfterSigning, Mode=OneWay}"',
            self.sign_xaml,
        )
        self.assertNotIn(
            "Validar el resultado al terminar — no disponible todavía",
            self.sign_xaml,
        )
        self.assertIn(
            "_session.Supports(DesktopOperationActions.Verify)",
            self.sign_vm,
        )
        self.assertIn("operations.VerifyAsync(", self.sign_vm)
        self.assertIn(
            "PostSignVerification.Create(\n"
            "                        outputPath,\n"
            "                        inputPath,\n"
            "                        result.Data.Format,\n"
            "                        signParameters.Action)",
            self.sign_vm,
        )
        self.assertNotIn("OriginalPath = inputPath", self.sign_vm)
        self.assertIn("IsCoherentVerification(verification.Data)", self.sign_vm)
        self.assertIn(
            "VerificationAssessment.HasValidSignatureEvidence(",
            self.sign_vm,
        )
        self.assertNotIn(
            "if (!verification.Data!.IsValid)",
            self.sign_vm,
        )
        self.assertIn(
            "La integridad se validó, pero la confianza no quedó establecida.",
            self.sign_vm,
        )
        self.assertIn("POST_SIGN_VERIFICATION_FAILED", self.sign_vm)
        self.assertIn(
            "INVALID_POST_SIGN_VERIFICATION_RESULT",
            self.sign_vm,
        )
        self.assertIn("var postValidationStarted = false;", self.sign_vm)
        self.assertIn("postValidationStarted = true;", self.sign_vm)
        self.assertIn(
            "El fichero firmado existe, pero no terminó la validación posterior.",
            self.sign_vm,
        )
        self.assertIn(
            "la firma no se presenta como validada.",
            self.sign_vm,
        )
        self.assertLess(
            self.sign_vm.index("!HasNonEmptyOutput(outputPath)"),
            self.sign_vm.index("operations.VerifyAsync("),
        )
        self.assertLess(
            self.sign_vm.index("operations.VerifyAsync("),
            self.sign_vm.index("Firma completada, guardada y validada"),
        )

    def test_double_click_cancel_and_output_opening_are_bounded(self) -> None:
        self.assertIn(
            "Interlocked.CompareExchange(",
            self.sign_vm,
        )
        self.assertIn(
            "Volatile.Read(ref _activeCancellation)?.Cancel();",
            self.sign_vm,
        )
        self.assertIn(
            'IsEnabled="{x:Bind ViewModel.CanCancel, Mode=OneWay}"',
            self.sign_xaml,
        )
        self.assertIn(
            "ViewModel.ValidateOutputForOpening();",
            self.sign_code,
        )
        self.assertIn("UseShellExecute = true", self.sign_code)

    def test_pages_do_not_log_paths_payloads_or_secrets(self) -> None:
        all_sources = "\n".join(
            (
                self.certificates_code,
                self.certificates_vm,
                self.sign_code,
                self.sign_vm,
            )
        )
        for forbidden in (
            "Console.Write",
            "Debug.Write",
            "Trace.Write",
            "ReturnSignatureBase64 = true",
            ".SignatureBase64",
            "secret_b64",
        ):
            self.assertNotIn(forbidden, all_sources)


if __name__ == "__main__":
    unittest.main()
