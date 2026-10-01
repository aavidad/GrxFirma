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

XAML = APP / "Views" / "ProtectPage.xaml"
CODE = APP / "Views" / "ProtectPage.xaml.cs"
VIEW_MODEL = APP / "ViewModels" / "ProtectPageViewModel.cs"
PICKER_API = APP / "Services" / "IFilePickerService.cs"
PICKER = APP / "Services" / "WindowsFilePickerService.cs"
CONTRACTS = CORE / "Operations" / "DesktopOperationContracts.cs"
CLIENT = CORE / "Operations" / "DesktopOperationsClient.cs"
GO_TYPES = (
    ROOT
    / "internal"
    / "adapters"
    / "inbound"
    / "desktop"
    / "ipc"
    / "types.go"
)
GO_HANDLER = (
    ROOT
    / "internal"
    / "adapters"
    / "inbound"
    / "desktop"
    / "ipc"
    / "handler.go"
)
QT_MAIN = ROOT / "cmd" / "gui-qml" / "qml" / "main.qml"


class ProtectFunctionalContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.xaml = XAML.read_text(encoding="utf-8")
        cls.code = CODE.read_text(encoding="utf-8")
        cls.view_model = VIEW_MODEL.read_text(encoding="utf-8")
        cls.picker_api = PICKER_API.read_text(encoding="utf-8")
        cls.picker = PICKER.read_text(encoding="utf-8")
        cls.contracts = CONTRACTS.read_text(encoding="utf-8")
        cls.client = CLIENT.read_text(encoding="utf-8")
        cls.go_types = GO_TYPES.read_text(encoding="utf-8")
        cls.go_handler = GO_HANDLER.read_text(encoding="utf-8")
        cls.qt_main = QT_MAIN.read_text(encoding="utf-8")

    def test_xaml_is_well_formed_and_lifecycle_is_bounded(self) -> None:
        root = ET.parse(XAML).getroot()
        self.assertEqual("OnLoaded", root.attrib["Loaded"])
        self.assertEqual("OnUnloaded", root.attrib["Unloaded"])
        self.assertIn(
            "_session.AvailabilityChanged += OnAvailabilityChanged;",
            self.code,
        )
        self.assertIn(
            "_session.AvailabilityChanged -= OnAvailabilityChanged;",
            self.code,
        )
        self.assertIn("ViewModel.CancelCurrentOperation();", self.code)

    def test_mutable_bindings_are_live_and_basic_user_flow_is_visual(self) -> None:
        mutable_names = (
            "PendingMessage",
            "Can",
            "Visible",
            "Selected",
            "IsProtectAndSign",
            "InputDisplayName",
            "ValidationMessage",
            "ResultMessage",
            "IsBusy",
        )
        for line in self.xaml.splitlines():
            if "x:Bind ViewModel." not in line:
                continue
            if any(name in line for name in mutable_names):
                self.assertRegex(line, r"Mode=(?:OneWay|TwoWay)")

        self.assertIn("Destinatarios", self.xaml)
        self.assertIn("Resultado de protección", self.xaml)
        self.assertIn("Resultado de desprotección", self.xaml)
        self.assertIn("AutomationProperties.LiveSetting", self.xaml)
        self.assertIn(
            "new OperationDiagnosticDialog(diagnostic)",
            self.code,
        )

    def test_typed_client_matches_exact_desktop_ipc_contract(self) -> None:
        for action, value in (
            ("ProtectionRecipients", "protection_recipients"),
            ("Protect", "protect"),
            ("ProtectAndSign", "protect_sign"),
            ("Unprotect", "unprotect"),
        ):
            self.assertIn(
                f'public const string {action} = "{value}";',
                self.client,
            )

        for method in (
            "GetProtectionRecipientsAsync",
            "ProtectAsync",
            "ProtectAndSignAsync",
            "UnprotectAsync",
        ):
            self.assertIn(method, self.client)

        for dto in (
            "ProtectionRecipientsResult",
            "ProtectionRecipientInfo",
            "ProtectionParameters",
            "ProtectionResult",
            "UnprotectResult",
        ):
            self.assertIn(f"record {dto}", self.contracts)

        for wire_name in (
            "inputPath",
            "outputPath",
            "certificateId",
            "certificateIndex",
            "profile",
            "recipientIds",
            "overwrite",
            "saveToDisk",
            "returnProtectedB64",
            "returnUnprotectedB64",
            "options",
            "secretB64",
        ):
            self.assertIn(f'JsonPropertyName("{wire_name}")', self.contracts)
            self.assertIn(f'json:"{wire_name}', self.go_types)

    def test_profiles_containers_and_extensions_match_go_and_qt(self) -> None:
        for profile in ("compat", "alto"):
            self.assertIn(f'"{profile}"', self.view_model)
        for container in (
            "json",
            "cms",
            "cms-encrypted",
            "authenvelopeddata",
            "signedandenvelopeddata",
        ):
            self.assertIn(f'"{container}"', self.view_model)
            self.assertIn(f'"{container}"', self.qt_main)

        for profile, extension in (
            ("ProtectedJson", ".afp"),
            ("CmsEnveloped", ".enveloped"),
            ("CmsAuthEnveloped", ".authenveloped.p7m"),
            ("CmsSignedEnveloped", ".signedenveloped.p7m"),
        ):
            self.assertIn(profile, self.picker_api)
            self.assertIn(f'SaveFilePickerProfile.{profile}', self.picker)
            self.assertIn(f'"{extension}"', self.picker)

        self.assertIn(
            "recipient.AuthEnvelopedDataCompatible",
            self.view_model,
        )
        self.assertIn(
            "VisibleContainers = IsProtectAndSign",
            self.view_model,
        )
        self.assertIn(
            'container.Value == "cms"',
            self.view_model,
        )
        self.assertIn(
            "AuthEnvelopedDataCompatible",
            self.go_handler,
        )

    def test_encrypted_data_uses_a_bounded_non_persistent_secret(self) -> None:
        sources = "\n".join((self.xaml, self.code, self.view_model))
        self.assertNotIn("<PasswordBox", self.xaml)
        self.assertNotIn(".Password", self.code)
        self.assertIn(
            'x:Load="{x:Bind ViewModel.IsEncryptedDataSelected, Mode=OneWay}"',
            self.xaml,
        )
        self.assertIn(
            'x:Load="{x:Bind ViewModel.IsEncryptedDataUnprotectSelected, Mode=OneWay}"',
            self.xaml,
        )

        self.assertIn("byte[]? transientSecret", self.view_model)
        self.assertIn("_securePasswordPrompt.CaptureAsync(", self.code)
        self.assertEqual(
            3,
            self.code.count(
                "await _securePasswordPrompt.CaptureAsync("
            ),
        )
        self.assertIn("Base64.DecodeFromUtf8(", self.code)
        self.assertIn("Base64.EncodeToUtf8(", self.code)
        self.assertIn("Marshal.ReadInt16(", self.code)
        self.assertIn(
            "CryptographicOperations.FixedTimeEquals(",
            sources,
        )
        self.assertGreaterEqual(
            sources.count("CryptographicOperations.ZeroMemory("),
            3,
        )
        self.assertIn("SymmetricKey = encryptedData", self.view_model)
        self.assertIn(
            "SymmetricKey = IsEncryptedDataUnprotectSelected",
            self.view_model,
        )
        self.assertIn('JsonPropertyName("secretB64")', self.contracts)
        self.assertIn("public byte[]? SymmetricKey", self.contracts)
        self.assertIn("SecretB64 *[]byte", self.go_types)
        self.assertIn("protectionSymmetricKeyIPC(", self.go_handler)
        self.assertNotIn('["secret_b64"]', self.view_model)
        self.assertNotIn("new string(transientSecret", sources)
        self.assertIn("RecipientIds = encryptedData", self.view_model)
        self.assertIn("? []", self.view_model)
        self.assertNotIn("NormalizeEncryptedDataOutputPath", self.view_model)
        self.assertNotIn("permanece deshabilitado", self.xaml)
        self.assertNotRegex(
            self.view_model,
            r"(?:private|public)\s+string\??\s+\w*[Ss]ecret\w*",
        )

    def test_encrypted_destination_is_selected_before_prompt_and_never_rewritten(self) -> None:
        self.assertRegex(
            self.view_model,
            r'"cms-encrypted",\s*SaveFilePickerProfile\.CmsEncrypted,\s*'
            r'RequiresTransientSecret: true',
        )
        protect = self.view_model[
            self.view_model.index("var container = SelectedContainer!;"):
            self.view_model.index("public async Task<OperationDiagnostic?> UnprotectAsync(")
        ]
        self.assertLess(
            protect.index(": container.SaveProfile;"),
            protect.index("await _filePicker.PickSaveFileAsync("),
        )
        self.assertRegex(protect, r'PickSaveFileAsync\(\s*saveProfile,')
        # One assignment, from the picker; even manually chosen alternative
        # names/casing must remain exact, not overwrite an unconfirmed sibling.
        import re
        self.assertEqual(1, len(re.findall(r'\boutputPath\s*=', protect)))
        self.assertIn("OutputPath = outputPath,", protect)
        self.assertNotIn("NormalizeEncryptedDataOutputPath", self.view_model)
        cancel = protect[
            protect.index("if (string.IsNullOrWhiteSpace(outputPath))"):
            protect.index("options = new Dictionary")
        ]
        self.assertIn("return null;", cancel)

    def test_transient_secret_is_cleared_on_every_page_exit_path(self) -> None:
        self.assertIn("finally", self.code)
        self.assertIn("ViewModel.CancelCurrentOperation();", self.code)
        self.assertIn("ZeroTransientSecret(transientSecret);", self.code)
        self.assertIn(
            "ZeroTransientSecret(transientSecretConfirmation);",
            self.code,
        )
        self.assertGreaterEqual(
            self.view_model.count(
                "ZeroTransientSecret(transientSecret);"
            ),
            2,
        )
        self.assertNotRegex(
            self.code,
            r"private\s+(?:string|char\[\])\??\s+\w*[Ss]ecret",
        )

    def test_recipients_use_stable_sanitized_ids_without_displaying_them(self) -> None:
        self.assertIn("certificate.Id", self.view_model)
        self.assertIn("recipient.Id", self.view_model)
        self.assertIn("GroupBy(recipient =>", self.view_model)
        self.assertIn("OperationResultText.Clean(value, 1024)", self.contracts)
        self.assertIn(
            ".Where(recipient =>\n                !string.IsNullOrWhiteSpace",
            self.contracts,
        )
        self.assertIn(
            'ItemsSource="{x:Bind ViewModel.VisibleRecipients, Mode=OneWay}"',
            self.xaml,
        )
        self.assertIn("SelectionMode=\"Multiple\"", self.xaml)
        self.assertIn("ViewModel.SetSelectedRecipients(", self.code)
        self.assertNotIn("Text=\"{Binding Id}\"", self.xaml)

    def test_empty_compat_catalog_explains_the_decryption_requirement(self) -> None:
        guidance_markers = (
            "CompatRecipientGuidance",
            "identidad RSA con clave privada descifrable",
            "P12/PFX autorizado",
            "almacén de Windows siguen disponibles para firmar",
            "cambie al perfil alto",
        )
        for marker in guidance_markers:
            self.assertIn(marker, self.view_model)
        self.assertIn(
            'SelectedProfile?.Value == "compat"',
            self.view_model,
        )
        self.assertIn(
            "identidad RSA con clave privada descifrable",
            self.qt_main,
        )
        self.assertIn("P12/PFX autorizado", self.qt_main)
        self.assertIn(
            "Importe o configure un certificado X.509 RSA válido",
            self.qt_main,
        )

    def test_calls_use_native_pickers_no_base64_and_verified_outputs(self) -> None:
        for picker_call in (
            "PickOpenFileAsync(",
            "PickSaveFileAsync(",
        ):
            self.assertIn(picker_call, self.view_model)
        self.assertIn(
            "OpenFilePickerProfile.ProtectedContainer",
            self.view_model,
        )
        self.assertIn("ReturnProtectedBase64 = false", self.view_model)
        self.assertIn("ReturnUnprotectedBase64 = false", self.view_model)
        self.assertNotIn(".ProtectedContentBase64", self.view_model)
        self.assertNotIn(".UnprotectedContentBase64", self.view_model)
        self.assertIn("!PathsEqual(result.Data.OutputPath, outputPath)", self.view_model)
        self.assertIn("!HasNonEmptyOutput(outputPath)", self.view_model)
        self.assertIn("new FileInfo(path).Length > 0", self.view_model)
        self.assertIn("ValidateProtectedOutputForOpening()", self.code)
        self.assertIn("ValidateUnprotectedOutputForOpening()", self.code)
        self.assertIn("UseShellExecute = true", self.code)

    def test_double_click_cancel_and_failures_have_visual_diagnostics(self) -> None:
        self.assertIn("Interlocked.CompareExchange(", self.view_model)
        self.assertIn(
            "Volatile.Read(ref _activeCancellation)?.Cancel();",
            self.view_model,
        )
        self.assertIn(
            'IsEnabled="{x:Bind ViewModel.CanCancel, Mode=OneWay}"',
            self.xaml,
        )
        self.assertIn(
            "OperationDiagnosticMapper.FromResult(result)",
            self.view_model,
        )
        self.assertIn(
            "OperationDiagnosticMapper.FromException(exception)",
            self.view_model,
        )

    def test_page_does_not_log_paths_payloads_or_secrets(self) -> None:
        sources = "\n".join((self.xaml, self.code, self.view_model))
        for forbidden in (
            "Console.Write",
            "Debug.Write",
            "Trace.Write",
            "ILogger",
            "ReturnProtectedBase64 = true",
            "ReturnUnprotectedBase64 = true",
        ):
            self.assertNotIn(forbidden, sources)


if __name__ == "__main__":
    unittest.main()
