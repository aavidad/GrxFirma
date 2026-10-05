# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest
import xml.etree.ElementTree as ET

from winui_catalog import read_with_catalog


ROOT = pathlib.Path(__file__).resolve().parents[3]
VIEWS_DIRECTORY = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views"
EXPECTED_PAGES = {
    "AboutPage.xaml",
    "CertificatesPage.xaml",
    "DiagnosticsPage.xaml",
    "FacturaePage.xaml",
    "EniPage.xaml",
    "HashPage.xaml",
    "HelpPage.xaml",
    "ProtectPage.xaml",
    "SettingsPage.xaml",
    "SignPage.xaml",
    "VerifyPage.xaml",
}

PRESENTATION = "http://schemas.microsoft.com/winfx/2006/xaml/presentation"
XAML = "http://schemas.microsoft.com/winfx/2006/xaml"
XAML_NAME = f"{{{XAML}}}Name"

FOCUSABLE_CONTROLS = {
    "Button",
    "CheckBox",
    "ComboBox",
    "DatePicker",
    "Expander",
    "ListView",
    "NumberBox",
    "PasswordBox",
    "RadioButton",
    "Slider",
    "TextBox",
    "ToggleSwitch",
}

REQUIRED_ADAPTIVE_TARGET = {
    "AboutPage.xaml": "AboutActions.Orientation",
    "CertificatesPage.xaml": "CertificateActions.Orientation",
    "DiagnosticsPage.xaml": "DiagnosticActions.Orientation",
    "FacturaePage.xaml": "ValidationActions.Orientation",
    "EniPage.xaml": "EniActions.MaxWidth",
    "HashPage.xaml": "OriginBrowseButton.(Grid.Row)",
    "HelpPage.xaml": "LayoutRoot.Padding",
    "ProtectPage.xaml": "ContainerCombo.(Grid.Row)",
    "SettingsPage.xaml": "ThemeCombo.(Grid.Row)",
    "SignPage.xaml": "DocumentBrowseButton.(Grid.Row)",
    "VerifyPage.xaml": "SignedBrowseButton.(Grid.Row)",
}


def local_name(element):
    return element.tag.rsplit("}", 1)[-1]


class PageXamlAccessibilityContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        page_paths = sorted(VIEWS_DIRECTORY.glob("*Page.xaml"))
        cls.pages = {
            path.name: (path, ET.parse(path).getroot())
            for path in page_paths
        }

    def test_all_ten_pages_are_covered(self):
        self.assertEqual(set(self.pages), EXPECTED_PAGES)

    def test_each_page_has_a_scrollable_narrow_first_layout(self):
        for page_name, (_, root) in self.pages.items():
            with self.subTest(page=page_name):
                scroll_viewers = [
                    element
                    for element in root.iter()
                    if local_name(element) == "ScrollViewer"
                ]
                self.assertEqual(len(scroll_viewers), 1)
                scroll_viewer = scroll_viewers[0]
                self.assertEqual(
                    scroll_viewer.get("HorizontalScrollMode"),
                    "Disabled",
                )
                self.assertEqual(
                    scroll_viewer.get("HorizontalScrollBarVisibility"),
                    "Disabled",
                )
                self.assertEqual(
                    scroll_viewer.get("VerticalScrollBarVisibility"),
                    "Auto",
                )
                self.assertTrue(
                    scroll_viewer.get("AutomationProperties.Name", "").strip()
                )

                layout_root = next(
                    element
                    for element in root.iter()
                    if element.get(XAML_NAME) == "LayoutRoot"
                )
                self.assertEqual(layout_root.get("Padding"), "16")

    def test_each_page_has_level_one_title_and_heading_hierarchy(self):
        for page_name, (_, root) in self.pages.items():
            with self.subTest(page=page_name):
                titles = [
                    element
                    for element in root.iter()
                    if local_name(element) == "TextBlock"
                    and element.get("Style")
                    == "{StaticResource PageTitleTextStyle}"
                ]
                self.assertEqual(len(titles), 1)
                self.assertEqual(
                    titles[0].get("AutomationProperties.HeadingLevel"),
                    "Level1",
                )

                section_titles = [
                    element
                    for element in root.iter()
                    if local_name(element) == "TextBlock"
                    and element.get("Style")
                    == "{StaticResource SectionTitleTextStyle}"
                ]
                for section_title in section_titles:
                    self.assertEqual(
                        section_title.get(
                            "AutomationProperties.HeadingLevel"
                        ),
                        "Level2",
                    )

    def test_each_page_has_an_adaptive_wide_state(self):
        for page_name, (_, root) in self.pages.items():
            with self.subTest(page=page_name):
                states = {
                    element.get(XAML_NAME)
                    for element in root.iter()
                    if local_name(element) == "VisualState"
                }
                self.assertTrue({"Narrow", "Wide"}.issubset(states))

                triggers = [
                    element
                    for element in root.iter()
                    if local_name(element) == "AdaptiveTrigger"
                ]
                self.assertEqual(
                    [trigger.get("MinWindowWidth") for trigger in triggers],
                    ["0", "900" if page_name == "FacturaePage.xaml" else "760"],
                )

                setter_targets = {
                    element.get("Target")
                    for element in root.iter()
                    if local_name(element) == "Setter"
                }
                self.assertIn("LayoutRoot.Padding", setter_targets)
                self.assertIn(
                    REQUIRED_ADAPTIVE_TARGET[page_name],
                    setter_targets,
                )

    def test_facturae_and_proxy_fields_reflow_before_the_wide_state(self):
        expected_reflow = {
            "FacturaePage.xaml": (
                "InvoiceSeries",
                "ReferenceContract",
                "SellerName",
                "SellerSecondSurname",
                "SellerTown",
                "SellerProvince",
                "BuyerName",
                "BuyerTown",
                "BuyerProvince",
                "ManagingBody",
                "ProcessingUnit",
                "UnitPrice",
                "VatRate",
                "TaxSummary",
                "TotalSummary",
                "Iban",
            ),
            "SettingsPage.xaml": ("ProxyUsername",),
            "SignPage.xaml": (
                "VisibleSealPreviewButton",
                "VisibleSealY",
                "VisibleSealWidth",
                "VisibleSealHeight",
            ),
        }
        for page_name, control_names in expected_reflow.items():
            with self.subTest(page=page_name):
                _, root = self.pages[page_name]
                named = {
                    element.get(XAML_NAME): element
                    for element in root.iter()
                    if element.get(XAML_NAME)
                }
                setter_targets = {
                    element.get("Target")
                    for element in root.iter()
                    if local_name(element) == "Setter"
                }
                for control_name in control_names:
                    with self.subTest(control=control_name):
                        control = named[control_name]
                        self.assertNotEqual(control.get("Grid.Row"), "0")
                        self.assertIn(
                            f"{control_name}.(Grid.Row)",
                            setter_targets,
                        )
                        self.assertIn(
                            f"{control_name}.(Grid.Column)",
                            setter_targets,
                        )

    def test_focusable_controls_have_names_and_stable_tab_order(self):
        for page_name, (_, root) in self.pages.items():
            with self.subTest(page=page_name):
                controls = [
                    element
                    for element in root.iter()
                    if local_name(element) in FOCUSABLE_CONTROLS
                    and element.get("IsTabStop") != "False"
                ]
                tab_indices = []
                for control in controls:
                    if control.get(XAML_NAME) == "ReleaseNotesButton":
                        code = read_with_catalog(VIEWS_DIRECTORY / (page_name + ".cs"))
                        self.assertIn("AutomationProperties.SetName(", code)
                        self.assertIn("AutomationProperties.SetHelpText(", code)
                        self.assertIn('"Novedades"', code)
                    else:
                        self.assertTrue(
                            control.get("AutomationProperties.Name", "").strip(),
                            f"{page_name}: {local_name(control)} sin nombre",
                        )
                    tab_index = control.get("TabIndex")
                    self.assertIsNotNone(
                        tab_index,
                        f"{page_name}: {local_name(control)} sin TabIndex",
                    )
                    tab_indices.append(int(tab_index))

                self.assertEqual(tab_indices, sorted(tab_indices))
                self.assertEqual(len(tab_indices), len(set(tab_indices)))
                if tab_indices:
                    self.assertEqual(tab_indices, list(range(len(tab_indices))))

    def test_text_and_lists_do_not_use_fixed_heights(self):
        for page_name, (_, root) in self.pages.items():
            with self.subTest(page=page_name):
                for element in root.iter():
                    name = local_name(element)
                    if name in {"TextBlock", "ListView"}:
                        self.assertIsNone(
                            element.get("Height"),
                            f"{page_name}: {name} no debe tener altura fija",
                        )

                    if name != "TextBlock" or element.get("Text") is None:
                        continue
                    style = element.get("Style", "")
                    if style in {
                        "{StaticResource PageTitleTextStyle}",
                        "{StaticResource PageSubtitleTextStyle}",
                        "{StaticResource SectionTitleTextStyle}",
                    }:
                        continue
                    if element.get("TextTrimming") == "CharacterEllipsis":
                        self.assertEqual(
                            element.get("TextWrapping"),
                            "NoWrap",
                            f"{page_name}: el texto recortado debe ocupar una sola línea",
                        )
                        continue
                    self.assertEqual(
                        element.get("TextWrapping"),
                        "Wrap",
                        f"{page_name}: texto literal sin ajuste de línea",
                    )


if __name__ == "__main__":
    unittest.main()
