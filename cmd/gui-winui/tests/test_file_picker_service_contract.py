# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest

from winui_catalog import read_with_catalog


ROOT = Path(__file__).resolve().parents[3]
SERVICES = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
    / "Services"
)
INTERFACE = SERVICES / "IFilePickerService.cs"
IMPLEMENTATION = SERVICES / "WindowsFilePickerService.cs"


class FilePickerServiceContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.api = INTERFACE.read_text(encoding="utf-8")
        cls.implementation = read_with_catalog(IMPLEMENTATION)

    def test_api_is_injectable_and_covers_open_save_folder_and_multiple(self) -> None:
        self.assertIn("interface IFilePickerService", self.api)
        for method in (
            "PickOpenFileAsync",
            "PickOpenFilesAsync",
            "PickSaveFileAsync",
            "PickAndSaveTextFileAsync",
            "PickFolderAsync",
        ):
            self.assertIn(method, self.api)
        self.assertIn("OpenFilePickerProfile profile", self.api)
        self.assertIn("SaveFilePickerProfile profile", self.api)

    def test_native_pickers_are_bound_to_the_real_owner_hwnd(self) -> None:
        source = self.implementation
        self.assertIn("new FileOpenPicker", source)
        self.assertIn("new FileSavePicker", source)
        self.assertIn("new FolderPicker", source)
        self.assertIn("WindowNative.GetWindowHandle(ownerWindow)", source)
        self.assertIn("InitializeWithWindow.Initialize(picker, windowHandle)", source)
        self.assertIn("if (windowHandle == 0)", source)

    def test_operations_are_serialized_and_support_external_cancellation(self) -> None:
        source = self.implementation
        self.assertIn("SemaphoreSlim _pickerGate = new(1, 1)", source)
        self.assertEqual(5, source.count("_pickerGate.WaitAsync(cancellationToken)"))
        self.assertEqual(5, source.count("_pickerGate.Release()"))
        self.assertEqual(5, source.count("IAsyncInfo)state!).Cancel()"))
        self.assertGreaterEqual(
            source.count("cancellationToken.ThrowIfCancellationRequested()"),
            10,
        )

    def test_open_and_save_filters_are_closed_profiles(self) -> None:
        source = self.implementation
        open_policy = source[
            source.index("OpenExtensions("):
            source.index("private static SavePickerPolicy")
        ]
        self.assertNotIn('"*"', open_policy)
        for extension in (
            '".pdf"',
            '".p7s"',
            '".xsig"',
            '".dsig"',
            '".p12"',
            '".pfx"',
            '".hexhash"',
            '".hashb64"',
            '".hashfiles"',
            '".txthashfiles"',
            '".hashreport"',
            '".afp"',
            '".enveloped"',
            '".authenveloped.p7m"',
            '".encrypted.p7m"',
            '".p7m"',
        ):
            self.assertIn(extension, source)
        self.assertNotIn("IReadOnlyList<string> extensions", self.api)
        self.assertIn("ArgumentOutOfRangeException", source)

    def test_user_cancel_is_empty_and_suggested_name_cannot_be_a_path(self) -> None:
        source = self.implementation
        self.assertIn("return LocalPathOrNull(file);", source)
        self.assertIn("return Array.Empty<string>();", source)
        self.assertIn("return LocalPathOrNull(folder);", source)
        self.assertIn("Path.GetFileName(suggestedFileName.Trim())", source)
        self.assertIn("Path.GetInvalidFileNameChars()", source)
        self.assertIn("UnicodeCategory.Format", source)
        self.assertIn("TrimEnd(' ', '.')", source)
        self.assertIn("normalized.Split('.', 2)[0]", source)
        self.assertIn("ReservedWindowsNames.Contains(deviceBaseName)", source)

    def test_cms_encrypted_profile_proposes_only_the_final_extension(self) -> None:
        self.assertIn("CmsEncrypted,", self.api)
        self.assertRegex(
            self.implementation,
            r'SaveFilePickerProfile\.CmsEncrypted\s*=>\s*new\(\s*'
            r'"CMS EncryptedData",\s*"documento-protegido",\s*'
            r'\["\.encrypted\.p7m"\]\)',
        )
        # Both the default suffix and the visible overwrite confirmation belong
        # to the native dialog, before it returns the selected destination.
        self.assertIn("DefaultFileExtension = policy.Extensions[0]", self.implementation)
        self.assertIn("policy.Extensions", self.implementation)

    def test_text_export_writes_the_selected_storage_file_without_reopening_path(self) -> None:
        source = self.implementation
        self.assertIn("MaximumTextFileBytes", source)
        self.assertIn("Encoding.UTF8.GetByteCount(contents)", source)
        self.assertIn("FileIO.WriteTextAsync(", source)
        self.assertIn("UnicodeEncoding.Utf8", source)
        self.assertNotIn("File.WriteAllText", source)
        self.assertNotIn("File.WriteAllBytes", source)

    def test_operation_save_selects_a_path_without_creating_a_storage_file(self) -> None:
        source = self.implementation
        method = source[source.index("public async Task<string?> PickSaveFileAsync("):
                        source.index("public async Task<bool> PickAndSaveTextFileAsync(")]
        self.assertIn("CreatePathSavePicker(profile, suggestedFileName)", method)
        self.assertIn("FullyQualifiedPathOrNull(file?.Path)", method)
        self.assertNotIn("CreateSavePicker(", method)
        self.assertNotIn("FileIO.", method)
        self.assertNotIn("File.Delete", source)
        factory = source[source.index("private Microsoft.Windows.Storage.Pickers.FileSavePicker CreatePathSavePicker("):
                         source.index("private FileSavePicker CreateSavePicker(")]
        self.assertIn("GetWindowIdFromWindow(windowHandle)", factory)
        self.assertIn("ShowOverwritePrompt = true", factory)
        self.assertIn("policy.Extensions.ToArray()", factory)

    def test_service_does_not_emit_paths_or_other_diagnostics(self) -> None:
        source = self.implementation
        for forbidden in (
            "Console.",
            "Debug.",
            "Trace.",
            "ILogger",
            "LogInformation",
            "LogError",
        ):
            self.assertNotIn(forbidden, source)


if __name__ == "__main__":
    unittest.main()
