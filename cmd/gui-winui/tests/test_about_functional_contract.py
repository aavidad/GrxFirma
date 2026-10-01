# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import unittest
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[3]
WINUI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
MAIN_XAML = WINUI / "MainWindow.xaml"
MAIN_CODE = WINUI / "MainWindow.xaml.cs"
VIEW = WINUI / "Views/AboutPage.xaml"
CODE_BEHIND = WINUI / "Views/AboutPage.xaml.cs"
VIEW_MODEL = WINUI / "ViewModels/AboutPageViewModel.cs"
INTERFACE = WINUI / "Services/IHelpLauncherService.cs"
SERVICE = WINUI / "Services/WindowsHelpLauncherService.cs"
PROJECT = WINUI / "GrxFirma.WinUI.csproj"


def local_name(element):
    return element.tag.rsplit("}", 1)[-1]


class AboutFunctionalContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main_xaml = MAIN_XAML.read_text(encoding="utf-8")
        cls.main_code = MAIN_CODE.read_text(encoding="utf-8")
        cls.xaml = VIEW.read_text(encoding="utf-8")
        cls.code_behind = CODE_BEHIND.read_text(encoding="utf-8")
        cls.view_model = VIEW_MODEL.read_text(encoding="utf-8")
        cls.api = INTERFACE.read_text(encoding="utf-8")
        cls.service = SERVICE.read_text(encoding="utf-8")
        cls.project = PROJECT.read_text(encoding="utf-8")
        cls.main_root = ET.parse(MAIN_XAML).getroot()
        cls.root = ET.parse(VIEW).getroot()

    def test_about_is_a_real_sidebar_destination(self) -> None:
        about_items = [
            element
            for element in self.main_root.iter()
            if local_name(element) == "NavigationViewItem"
            and element.get("Tag") == "about"
        ]
        self.assertEqual(len(about_items), 1)
        self.assertEqual(
            about_items[0].get("Content"),
            "Acerca de",
        )
        self.assertIn('"about" => "Acerca de"', self.main_code)
        self.assertIn('"about" => typeof(AboutPage)', self.main_code)

    def test_identity_authorship_and_license_are_visible(self) -> None:
        for visible_text in (
            "GrxFirma",
            "Autor: Alberto Avidad Fernández",
            "Copyright © 2026 Alberto Avidad Fernández",
            "Software libre bajo la licencia EUPL 1.2 o posterior.",
        ):
            self.assertIn(visible_text, self.xaml)
        self.assertIn('Source="ms-appx:///Assets/logo-dipgra.png"', self.xaml)
        self.assertIn("ViewModel.VersionText", self.xaml)

    def test_canonical_logo_and_version_are_published(self) -> None:
        for project_contract in (
            r"<Link>Assets\logo-dipgra.png</Link>",
            r"<TargetPath>Assets\logo-dipgra.png</TargetPath>",
            r'<Content Include="..\..\..\..\VERSION.txt">',
            r"<TargetPath>VERSION.txt</TargetPath>",
            "<CopyToPublishDirectory>Always</CopyToPublishDirectory>",
        ):
            self.assertIn(project_contract, self.project)
        for safe_version_contract in (
            "AppContext.BaseDirectory",
            '"VERSION.txt"',
            "Path.GetRelativePath",
            "FileAttributes.ReparsePoint",
            "value.Length is < 1 or > 64",
            "char.IsAsciiLetterOrDigit",
            '"Versión no disponible"',
        ):
            self.assertIn(safe_version_contract, self.view_model)

    def test_license_and_repository_actions_use_closed_destinations(self) -> None:
        self.assertIn("OpenOfficialLicenseAsync", self.api)
        self.assertIn("OpenOfficialProjectAsync", self.api)
        self.assertIn(
            '"https://interoperable-europe.ec.europa.eu/collection/eupl/eupl-text-eupl-12"',
            self.service,
        )
        self.assertIn(
            '"https://github.com/aavidad/GrxFirma"',
            self.service,
        )
        self.assertIn(
            'OfficialProjectUrl + "/releases"',
            self.service,
        )
        self.assertIn("OpenOfficialReleasesAsync", self.api)
        self.assertIn(
            '"/aavidad/GrxFirma/releases"',
            self.service,
        )
        self.assertIn(
            '"interoperable-europe.ec.europa.eu"',
            self.service,
        )
        self.assertIn(
            '"/collection/eupl/eupl-text-eupl-12"',
            self.service,
        )
        self.assertNotIn("Process.Start", self.service)
        self.assertNotIn("UseShellExecute", self.service)

    def test_actions_are_accessible_and_lifetime_bounded(self) -> None:
        buttons = [
            element
            for element in self.root.iter()
            if local_name(element) == "Button" and element.get("TabIndex") is not None
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
        self.assertIn('Click="OnReleaseNotesClick"', self.xaml)
        self.assertIn("ShowReleaseNotesHistoryAsync", self.code_behind)
        self.assertIn("AutomationProperties.SetName(", self.code_behind)

        for lifecycle_contract in (
            'Loaded="OnLoaded"',
            'Unloaded="OnUnloaded"',
        ):
            self.assertIn(lifecycle_contract, self.xaml)
        self.assertIn("ViewModel.Activate()", self.code_behind)
        self.assertIn("ViewModel.Deactivate()", self.code_behind)
        self.assertIn("CancellationTokenSource", self.view_model)
        self.assertIn("Interlocked.CompareExchange", self.view_model)
        self.assertIn("IsCurrentLifetime", self.view_model)
        self.assertIn("CheckUpdatesAsync", self.view_model)
        self.assertIn("DesktopOperationActions.CheckUpdates", self.view_model)

    def test_update_result_exposes_official_channel_state(self) -> None:
        contract = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/DesktopUpdateContracts.cs").read_text(encoding="utf-8")
        self.assertIn('[JsonPropertyName("estado")]', contract)
        self.assertIn('[JsonPropertyName("mensaje")]', contract)
        self.assertIn('[JsonPropertyName("titulo")]', contract)
        self.assertIn('result.Data.State == "sin_publicaciones"', self.view_model)
        self.assertIn('StatusMessage = result.Data.Message;', self.view_model)
        self.assertIn('StatusTitle = result.Data.Title;', self.view_model)
        self.assertLess(self.view_model.index('result.Data.State == "sin_publicaciones"'), self.view_model.index('result.Data.HasNewVersion'))

    def test_update_failure_displays_safe_ipc_message(self) -> None:
        self.assertIn('StatusMessage = result.SafeUserMessage;', self.view_model)


if __name__ == "__main__":
    unittest.main()
