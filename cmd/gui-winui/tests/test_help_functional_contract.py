# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest
import xml.etree.ElementTree as ET

from winui_catalog import read_with_catalog


ROOT = Path(__file__).resolve().parents[3]
WINUI = (
    ROOT
    / "cmd"
    / "gui-winui"
    / "src"
    / "GrxFirma.WinUI"
)
INTERFACE = WINUI / "Services" / "IHelpLauncherService.cs"
SERVICE = WINUI / "Services" / "WindowsHelpLauncherService.cs"
VIEW_MODEL = WINUI / "ViewModels" / "HelpPageViewModel.cs"
VIEW = WINUI / "Views" / "HelpPage.xaml"
CODE_BEHIND = WINUI / "Views" / "HelpPage.xaml.cs"
PROJECT = WINUI / "GrxFirma.WinUI.csproj"
USER_GUIDE = WINUI / "help" / "guia-usuario.txt"

PRESENTATION = "http://schemas.microsoft.com/winfx/2006/xaml/presentation"


def local_name(element):
    return element.tag.rsplit("}", 1)[-1]


class HelpFunctionalContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.api = read_with_catalog(INTERFACE)
        cls.service = read_with_catalog(SERVICE)
        cls.view_model = read_with_catalog(VIEW_MODEL)
        cls.xaml = VIEW.read_text(encoding="utf-8")
        cls.code_behind = read_with_catalog(CODE_BEHIND)
        cls.project = PROJECT.read_text(encoding="utf-8")
        cls.user_guide = USER_GUIDE.read_text(encoding="utf-8")
        cls.root = ET.parse(VIEW).getroot()

    def test_launcher_api_is_closed_and_does_not_accept_paths_or_uris(self) -> None:
        self.assertIn("interface IHelpLauncherService", self.api)
        for operation in (
            "OpenInstalledManualAsync",
            "OpenInstallationFolderAsync",
            "OpenOfficialProjectAsync",
            "OpenPrivateSupportAsync",
        ):
            self.assertIn(operation, self.api)
        for unsafe_parameter in (
            "string path",
            "string url",
            "string uri",
            "Uri uri",
        ):
            self.assertNotIn(unsafe_parameter, self.api)

    def test_windows_launcher_is_used_without_shell_or_process_execution(self) -> None:
        source = self.service
        for native_operation in (
            "Launcher.LaunchFileAsync",
            "Launcher.LaunchFolderAsync",
            "Launcher.LaunchUriAsync",
        ):
            self.assertIn(native_operation, source)
        for forbidden in (
            "Process.Start",
            "UseShellExecute",
            "cmd.exe",
            "powershell",
            "ShellExecute",
        ):
            self.assertNotIn(forbidden, source)

    def test_external_destinations_are_fixed_and_validated(self) -> None:
        source = self.service
        self.assertIn(
            '"https://github.com/aavidad/GrxFirma"',
            source,
        )
        self.assertIn(
            '"mailto:avidad@dipgra.es?subject=Soporte%20privado%20GrxFirma"',
            source,
        )
        self.assertIn('uri.Host,', source)
        self.assertIn('"github.com"', source)
        self.assertIn('uri.AbsolutePath.TrimEnd', source)
        self.assertIn('uri.UserInfo,', source)
        self.assertIn('uri.OriginalString,', source)
        self.assertNotIn("DesktopOperationSession", source)
        self.assertNotIn("OperationDiagnostic", source)

    def test_manual_candidates_are_local_closed_and_reparse_safe(self) -> None:
        source = self.service
        for expected in (
            "AppContext.BaseDirectory",
            "SpecialFolder.LocalApplicationData",
            '"help"',
            '"ayuda.pdf"',
            '"README_DESKTOP_WINUI_WINDOWS.md"',
            '"README_WINDOWS_SUITE.md"',
            "AllowedManualExtensions",
            "IsPathUnderRoot",
            "FileAttributes.ReparsePoint",
        ):
            self.assertIn(expected, source)
        self.assertNotIn("GetCommandLineArgs", source)
        self.assertNotIn("Environment.GetEnvironmentVariable", source)

    def test_plain_text_user_guide_is_app_local_and_first_candidate(self) -> None:
        self.assertTrue(USER_GUIDE.is_file())
        self.assertGreater(len(self.user_guide), 1024)
        for required_section in (
            "FIRMAR UN DOCUMENTO",
            "VERIFICAR UNA FIRMA",
            "USO DESDE EL NAVEGADOR",
            "SI UNA OPERACION FALLA",
            "ALGORITMOS ANTIGUOS",
            "PRIVACIDAD Y SOPORTE",
        ):
            self.assertIn(required_section, self.user_guide)
        for forbidden in (
            "http://",
            "https://",
            "mailto:",
            "file://",
            "C:\\",
            "\\\\.\\pipe\\",
            "BEGIN PRIVATE KEY",
        ):
            self.assertNotIn(forbidden, self.user_guide)

        self.assertIn(
            '<None Update="help\\guia-usuario.txt">',
            self.project,
        )
        self.assertIn(
            "<CopyToOutputDirectory>PreserveNewest</CopyToOutputDirectory>",
            self.project,
        )
        self.assertIn(
            "<CopyToPublishDirectory>Always</CopyToPublishDirectory>",
            self.project,
        )
        self.assertIn(
            "<TargetPath>help\\guia-usuario.txt</TargetPath>",
            self.project,
        )
        self.assertLess(
            self.service.index('"guia-usuario.txt"'),
            self.service.index('"ayuda.pdf"'),
        )

    def test_missing_resources_and_windows_associations_are_explained(self) -> None:
        source = self.service
        for message_part in (
            "No se encontró un manual instalado",
            "Windows no encontró una aplicación compatible",
            "Windows no tiene configurado un navegador",
            "Windows no tiene configurada una aplicación de correo",
        ):
            self.assertIn(message_part, source)
        for forbidden_log in (
            "Console.",
            "Debug.",
            "Trace.",
            "ILogger",
        ):
            self.assertNotIn(forbidden_log, source)

    def test_page_lifecycle_cancels_work_and_prevents_double_launch(self) -> None:
        self.assertIn("Loaded=\"OnLoaded\"", self.xaml)
        self.assertIn("Unloaded=\"OnUnloaded\"", self.xaml)
        self.assertIn("ViewModel.Activate()", self.code_behind)
        self.assertIn("ViewModel.Deactivate()", self.code_behind)
        self.assertIn("CancellationTokenSource", self.view_model)
        self.assertIn("Interlocked.CompareExchange", self.view_model)
        self.assertIn("IsCurrentLifetime", self.view_model)
        self.assertIn(
            "IsEnabled=\"{x:Bind ViewModel.CanLaunch, Mode=OneWay}\"",
            self.xaml,
        )

    def test_basic_user_has_five_real_accessible_actions(self) -> None:
        buttons = [
            element
            for element in self.root.iter()
            if local_name(element) == "Button"
        ]
        self.assertEqual(len(buttons), 5)
        self.assertEqual(
            [button.get("TabIndex") for button in buttons],
            ["0", "1", "2", "3", "4"],
        )
        for button in buttons:
            if button.get("Click") != "OnReleaseNotesClick":
                self.assertTrue(button.get("AutomationProperties.Name", "").strip())
                self.assertTrue(button.get("AutomationProperties.HelpText", "").strip())
            self.assertTrue(button.get("Click", "").strip())
        self.assertIn("AutomationProperties.SetName(", self.code_behind)
        self.assertIn("ViewModel.StatusMessage", self.xaml)
        self.assertIn(
            'AutomationProperties.LiveSetting="Assertive"',
            self.xaml,
        )


if __name__ == "__main__":
    unittest.main()
