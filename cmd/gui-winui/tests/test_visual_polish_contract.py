# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[1] / "src/GrxFirma.WinUI"
XAML_NAME = "{http://schemas.microsoft.com/winfx/2006/xaml}Name"


class VisualPolishContractTests(unittest.TestCase):
    def test_adaptive_states_are_attached_to_first_page_child(self):
        # WinUI discovers StateTriggers on the first child of Page, not a
        # LayoutRoot nested inside its ScrollViewer.
        ns = "{http://schemas.microsoft.com/winfx/2006/xaml/presentation}"
        for path in (ROOT / "Views").glob("*Page.xaml"):
            root = ET.parse(path).getroot()
            content = next(child for child in root if "." not in child.tag.rsplit("}", 1)[-1])
            with self.subTest(page=path.name):
                self.assertIsNotNone(content.find(f"{ns}VisualStateManager.VisualStateGroups"))
                self.assertEqual(len(list(root.iter(f"{ns}VisualStateManager.VisualStateGroups"))), 1)

    def test_page_buttons_keep_usable_hit_targets(self):
        for path in (ROOT / "Views").glob("*Page.xaml"):
            for element in ET.parse(path).getroot().iter():
                if element.tag.endswith("}Button"):
                    with self.subTest(page=path.name, name=element.get("AutomationProperties.Name")):
                        self.assertGreaterEqual(float(element.get("MinHeight", "0")), 40)

    def test_certificate_and_about_action_rows_wrap_instead_of_overflowing(self):
        for page, name in [("Certificates", "CertificateActions"), ("About", "AboutActions"), ("Sign", "SignActions")]:
            root = ET.parse(ROOT / f"Views/{page}Page.xaml").getroot()
            panel = next(element for element in root.iter() if element.get(XAML_NAME) == name)
            self.assertTrue(panel.tag.endswith("}ResponsiveActionPanel"))

    def test_p12_is_named_without_changing_temporary_import_action(self):
        root = ET.parse(ROOT / "Views/SignPage.xaml").getroot()
        button = next(element for element in root.iter() if element.get("Click") == "OnUseTemporaryCredentialClick")
        self.assertTrue(any(element.get("Text") == "Cargar P12/PFX…" for element in button.iter()))
        self.assertIn("sesión", button.get("AutomationProperties.Name"))

    def test_healthy_connection_does_not_consume_permanent_banner_space(self):
        for page in ["Sign", "Verify", "Certificates", "Hash", "Protect", "Settings"]:
            root = ET.parse(ROOT / f"Views/{page}Page.xaml").getroot()
            banner = next(element for element in root.iter() if "ViewModel.PendingMessage" in element.get("Message", ""))
            self.assertIn("IsAvailabilityNoticeOpen", banner.get("IsOpen"))
        workspace = (ROOT / "ViewModels/WorkspacePageViewModel.cs").read_text()
        self.assertIn("IsAvailabilityNoticeOpen => !IsOperationConnected", workspace)
        self.assertIn("RaisePropertyChanged(nameof(IsAvailabilityNoticeOpen))", workspace)

    def test_theme_is_applied_to_window_and_successful_preferences(self):
        window = (ROOT / "MainWindow.xaml.cs").read_text()
        model = (ROOT / "ViewModels/SettingsPageViewModel.cs").read_text()
        self.assertIn("AppRoot.RequestedTheme", window)
        self.assertIn("1 => ElementTheme.Light", window)
        self.assertIn("0 or 2 => ElementTheme.Dark", window)
        self.assertIn("_ => ElementTheme.Dark", window)
        self.assertIn("ThemePreferenceApplied?.Invoke(document.ThemeIndex)", model)
        self.assertIn("ThemePreferenceApplied?.Invoke(result.Data.ThemeIndex)", model)

    def test_seal_editor_has_high_contrast_resources(self):
        root = ET.parse(ROOT / "App.xaml").getroot()
        key = "{http://schemas.microsoft.com/winfx/2006/xaml}Key"
        high_contrast = next(element for element in root.iter() if element.get(key) == "HighContrast")
        for name in ["AppSealBorderBrush", "AppSealTextBrush", "AppSealHandleTextBrush"]:
            brush = next(element for element in high_contrast if element.get(key) == name)
            self.assertIn("ThemeResource SystemColor", brush.get("Color"))


if __name__ == "__main__":
    unittest.main()
