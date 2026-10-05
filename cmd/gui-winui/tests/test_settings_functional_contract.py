# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
APP = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"
GO_SETTINGS = ROOT / "internal/ports/user_settings.go"
QML = ROOT / "cmd/gui-qml/qml/main.qml"


class SettingsFunctionalContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.contracts = (
            CORE / "Operations/DesktopSettingsContracts.cs"
        ).read_text(encoding="utf-8")
        cls.proxy_contracts = (
            CORE / "Operations/DesktopProxyContracts.cs"
        ).read_text(encoding="utf-8")
        cls.client = (
            CORE / "Operations/DesktopOperationsClient.cs"
        ).read_text(encoding="utf-8")
        cls.vm = (
            APP / "ViewModels/SettingsPageViewModel.cs"
        ).read_text(encoding="utf-8")
        cls.network_validation = (
            CORE / "Operations/NetworkFormValidation.cs"
        ).read_text(encoding="utf-8")
        cls.page = (
            APP / "Views/SettingsPage.xaml.cs"
        ).read_text(encoding="utf-8")
        cls.xaml_path = APP / "Views/SettingsPage.xaml"
        cls.xaml = cls.xaml_path.read_text(encoding="utf-8")
        cls.go_settings = GO_SETTINGS.read_text(encoding="utf-8")
        cls.go_settings_language = (
            ROOT / "internal/ports/seal_language.go"
        ).read_text(encoding="utf-8")
        cls.qml = QML.read_text(encoding="utf-8")

    def test_core_uses_exact_get_and_save_actions(self):
        self.assertIn('GetSettings = "get_settings"', self.client)
        self.assertIn('SaveSettings = "save_settings"', self.client)
        self.assertIn("GetSettingsAsync(", self.client)
        self.assertIn("SaveSettingsAsync(", self.client)
        self.assertIn(
            "SendAsync<DesktopSettingsDocument, string>",
            self.client,
        )

    def test_typed_fields_match_current_go_and_qt_wire_names(self):
        wire_names = (
            "idioma",
            "themeIndex",
            "confirmToSign",
            "closeBehavior",
            "signFormat",
            "signStrictCompat",
            "signVisibleSeal",
            "facturaeToolsEnabled",
            "checkForUpdates",
            "preferDefaultCertificate",
            "defaultCertificateId",
            "proxyEnabled",
            "proxyType",
            "proxyHost",
            "proxyPort",
            "proxyExcludedUrls",
        )
        for name in wire_names:
            self.assertIn(
                f'[JsonPropertyName("{name}")]',
                self.contracts,
            )
            self.assertIn(f'"{name}"', self.go_settings)
            self.assertIn(f"{name}:", self.qml)

    def test_partial_save_preserves_safe_legacy_values_only(self):
        self.assertIn("[JsonExtensionData]", self.contracts)
        self.assertIn("CreateSafeSaveSnapshot()", self.contracts)
        self.assertIn("IsSafeAdditionalName", self.contracts)
        self.assertIn("IsSafeAdditionalValue", self.contracts)
        self.assertIn("SafeAdditionalSettingNames", self.contracts)
        self.assertIn("MaximumPreservedStringCharacters", self.contracts)
        self.assertIn("MaximumPreservedArrayStringCharacters", self.contracts)
        self.assertIn("MaximumPreservedDocumentBytes", self.contracts)
        self.assertIn("Encoding.UTF8.GetByteCount", self.contracts)
        for known_safe_name in (
            "tema",
            "signReason",
            "certificateTypeFilter",
            "tsaEnabled",
        ):
            self.assertIn(f'"{known_safe_name}"', self.contracts)
        for typed_name in (
            "preferDefaultCertificate",
            "defaultCertificateId",
        ):
            self.assertIn(
                f'[JsonPropertyName("{typed_name}")]',
                self.contracts,
            )
            safe_names = self.contracts.split(
                "SafeAdditionalSettingNames",
                maxsplit=1,
            )[1].split(
                "StringComparer.Ordinal);",
                maxsplit=1,
            )[0]
            self.assertNotIn(f'"{typed_name}"', safe_names)
        for forbidden_name in (
            "proxyPassword",
            "proxyUsername",
            "securityAccessPassword",
            "webCompatibilityActive",
            "webCompatibilityExpiresAt",
        ):
            self.assertNotIn(f'"{forbidden_name}"', self.contracts)
        self.assertIn(
            '[JsonPropertyName("proxySecretId")]',
            self.contracts,
        )
        self.assertIn(
            '[JsonPropertyName("proxyRealm")]',
            self.contracts,
        )
        self.assertIn("ProxySecretId = null", self.contracts)
        self.assertIn("ProxyRealm = null", self.contracts)
        self.assertNotIn("AdditionalSettings", self.xaml)

    def test_view_model_loads_saves_validates_and_requires_coherent_data(self):
        for fragment in (
            "DesktopOperationActions.GetSettings",
            "DesktopOperationActions.SaveSettings",
            "operations.GetSettingsAsync(",
            "operations.SaveSettingsAsync(",
            "result.Data is null",
            "string.IsNullOrWhiteSpace(result.Data)",
            "TryBuildSaveDocument",
            "NetworkFormValidation.ProxyPortError(",
            "NetworkFormValidation.ProxyHostError(",
        ):
            self.assertIn(fragment, self.vm)
        for fragment in (
            "port is >= 1 and <= 65535",
            'host.Contains("://", StringComparison.Ordinal)',
        ):
            self.assertIn(fragment, self.network_validation)

    def test_operations_are_cancelable_and_double_click_is_bounded(self):
        for fragment in (
            "Interlocked.CompareExchange(",
            "CancellationTokenSource.CreateLinkedTokenSource(",
            "CancelCurrentOperation",
            "Interlocked.Exchange(ref _operationInProgress, 0)",
            "OperationDiagnosticMapper.FromResult",
            "OperationDiagnosticMapper.FromException",
        ):
            self.assertIn(fragment, self.vm)
        self.assertIn("OperationDiagnosticDialog", self.page)

    def test_options_match_current_qt_enumerations(self):
        for language in (
            '"es"',
            '"ca"',
            '"va"',
            '"eu"',
            '"gl"',
            '"en"',
            '"de"',
            '"fr"',
            '"pt"',
            '"it"',
            '"zh"',
        ):
            self.assertIn(language, self.vm)
        for value in (
            '"pades"',
            '"cades"',
            '"xades"',
            '"xmldsig"',
            '"odf"',
            '"ooxml"',
            '"facturae"',
            '"asic-xades"',
            '"none"',
            '"manual"',
            '"resident"',
            '"exit"',
        ):
            self.assertIn(value, self.vm)

    def test_page_has_live_bindings_lifecycle_and_no_secret_inputs(self):
        ET.parse(self.xaml_path)
        self.assertIn('Loaded="OnLoaded"', self.xaml)
        self.assertIn('Unloaded="OnUnloaded"', self.xaml)
        self.assertIn("app.OperationSession", self.page)
        self.assertIn("ViewModel.Activate()", self.page)
        self.assertIn("ViewModel.Deactivate()", self.page)
        self.assertIn("Mode=OneWay", self.xaml)
        self.assertIn("Mode=TwoWay", self.xaml)
        self.assertNotIn("<PasswordBox", self.xaml)
        self.assertNotIn("proxyPassword", self.xaml)
        self.assertIn("SecurePasswordPromptService", self.page)
        self.assertIn("NativePasswordEncoding.ToUtf8", self.page)
        self.assertIn(
            "CryptographicOperations.ZeroMemory(password)",
            self.page,
        )
        self.assertIn("captured?.Dispose()", self.page)

    def test_proxy_secret_wire_is_binary_and_managed_separately(self):
        for action in (
            'ProxySecretStore =\n        "proxy_secret_store"',
            'ProxySecretDelete =\n        "proxy_secret_delete"',
            'ProxySecretStoreStatus =\n        "proxy_secret_store_status"',
        ):
            self.assertIn(action, self.client)
        for wire_name in (
            "realm",
            "username",
            "password",
            "configured",
            "rotated",
        ):
            self.assertIn(
                f'[JsonPropertyName("{wire_name}")]',
                self.proxy_contracts,
            )
        self.assertIn("required byte[] Password", self.proxy_contracts)
        self.assertNotIn("string Password", self.proxy_contracts)
        self.assertIn("StoreProxySecretAsync(", self.client)
        self.assertIn("DeleteProxySecretAsync(", self.client)
        self.assertIn(
            "CryptographicOperations.ZeroMemory(parameters.Password)",
            self.client,
        )
        self.assertIn("ContentDialog", self.page)
        self.assertIn("ContentDialogResult.Primary", self.page)
        delete_state = self.vm.split(
            "CanDeleteProxyCredentials =",
            maxsplit=1,
        )[1].split("CanSave =", maxsplit=1)[0]
        self.assertIn("CanEdit &&", delete_state)
        self.assertNotIn("CanEditManualProxy &&", delete_state)

    def test_save_confirms_and_explains_why_it_cannot_save(self):
        # Recorrido Windows 0.0.117 (M11).
        can_save = self.vm.split("CanSave =", maxsplit=1)[1].split(";", maxsplit=1)[0]
        self.assertNotIn("Localizer.Text", can_save)
        self.assertIn("_validationError = ValidationError();", self.vm)
        self.assertIn('? Localizer.Text("winui.ajustes.preferencias_guardadas")', self.vm)
        self.assertIn("_justSaved = true;", self.vm)
        self.assertIn("if (CanSave && _validationError is not null)", self.vm)
        self.assertNotIn("editor seguro de sello PDF", self.xaml)

    def test_settings_sources_do_not_log_or_write_sensitive_values(self):
        combined = "\n".join(
            (
                self.contracts,
                self.proxy_contracts,
                self.vm,
                self.page,
            )
        )
        for forbidden in (
            "Console.",
            "Debug.",
            "Trace.",
            "File.Write",
            "Directory.Create",
        ):
            self.assertNotIn(forbidden, combined)

    def test_seal_language_is_editable_saved_and_used_by_signing(self):
        self.assertIn('[JsonPropertyName("signSealLanguage")]', self.contracts)
        self.assertIn('"signSealLanguage"', self.go_settings_language)
        self.assertIn("SealLanguage = SelectedSealLanguage.Value", self.vm)
        self.assertIn("DesktopSettingsDocument.SealLanguageFollowsInterface", self.vm)
        root = ET.fromstring(self.xaml)
        combo = next(
            element for element in root.iter()
            if element.tag.endswith("}ComboBox")
            and element.get("{http://schemas.microsoft.com/winfx/2006/xaml}Name") == "SealLanguageCombo"
        )
        self.assertEqual(combo.get("AutomationProperties.Name"), "Idioma del sello")
        self.assertIn("SelectedSealLanguage", combo.get("SelectedItem", ""))
        self.assertIn("ViewModel.SealLanguages", combo.get("ItemsSource", ""))
        sign_vm = (APP / "ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
        self.assertIn("DesktopSettingsDocument.EffectiveSealLanguage(", sign_vm)
        sign_page = (APP / "Views/SignPage.xaml.cs").read_text(encoding="utf-8")
        self.assertIn("ViewModel.SetSealLanguagePreference(result.Data.SealLanguage);", sign_page)
        self.assertIn('signSealLanguage: window.signSealLanguage', self.qml)


if __name__ == "__main__":
    unittest.main()
