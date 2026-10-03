# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
APP_PATH = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/App.xaml"
WINDOW_PATH = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/MainWindow.xaml"
)
WINDOW_CODE_PATH = (
    ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/MainWindow.xaml.cs"
)

PRESENTATION = "http://schemas.microsoft.com/winfx/2006/xaml/presentation"


def local_name(element):
    return element.tag.rsplit("}", 1)[-1]


class MainWindowAccessibilityContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.app = ET.parse(APP_PATH).getroot()
        cls.window = ET.parse(WINDOW_PATH).getroot()
        cls.window_code = WINDOW_CODE_PATH.read_text(encoding="utf-8")

    def test_navigation_destinations_have_unique_access_keys(self):
        navigation_items = [
            element
            for element in self.window.iter()
            if local_name(element) == "NavigationViewItem"
        ]
        self.assertEqual(len(navigation_items), 11)

        access_keys = []
        for item in navigation_items:
            access_key = item.get("AccessKey", "").strip()
            self.assertEqual(
                len(access_key),
                1,
                f"{item.get('Content')}: AccessKey inválida",
            )
            access_keys.append(access_key.casefold())

        self.assertEqual(len(access_keys), len(set(access_keys)))

    def test_brand_fits_expanded_sidebar_in_both_themes(self):
        navigation = next(
            element for element in self.window.iter()
            if local_name(element) == "NavigationView"
        )
        self.assertGreaterEqual(int(navigation.get("OpenPaneLength", "0")), 250)
        brand = next(
            element for element in navigation.iter()
            if local_name(element) == "TextBlock" and element.get("Text") == "GrxFirma"
        )
        self.assertEqual(brand.get("TextWrapping"), "NoWrap")
        self.assertEqual(brand.get("TextTrimming"), "None")
        self.assertIsNone(brand.get("Foreground"))

    def test_route_change_moves_focus_into_new_page(self):
        frame = next(
            element
            for element in self.window.iter()
            if local_name(element) == "Frame"
        )
        self.assertEqual(
            frame.get("Navigated"),
            "OnContentFrameNavigated",
        )
        self.assertIn(
            "FocusManager.FindFirstFocusableElement(page)",
            self.window_code,
        )
        self.assertIn(
            "FocusManager.TryFocusAsync(",
            self.window_code,
        )
        self.assertIn(
            "FocusState.Programmatic",
            self.window_code,
        )
        self.assertIn(
            "page.Loaded += loadedHandler",
            self.window_code,
        )
        self.assertIn(
            "page.Loaded -= loadedHandler",
            self.window_code,
        )

    def test_high_contrast_uses_user_selected_system_colors(self):
        high_contrast = next(
            dictionary
            for dictionary in self.app.iter()
            if local_name(dictionary) == "ResourceDictionary"
            and dictionary.get(
                "{http://schemas.microsoft.com/winfx/2006/xaml}Key"
            )
            == "HighContrast"
        )
        brushes = {
            brush.get(
                "{http://schemas.microsoft.com/winfx/2006/xaml}Key"
            ): brush.get("Color")
            for brush in high_contrast
            if local_name(brush) == "SolidColorBrush"
        }

        self.assertEqual(
            brushes["AppPageBackgroundBrush"],
            "{ThemeResource SystemColorWindowColor}",
        )
        self.assertEqual(
            brushes["AppCardBackgroundBrush"],
            "{ThemeResource SystemColorWindowColor}",
        )
        self.assertEqual(
            brushes["AppCardBorderBrush"],
            "{ThemeResource SystemColorWindowTextColor}",
        )
        self.assertEqual(
            brushes["AppMutedTextBrush"],
            "{ThemeResource SystemColorWindowTextColor}",
        )
        self.assertNotIn("Black", brushes.values())
        self.assertNotIn("White", brushes.values())


if __name__ == "__main__":
    unittest.main()
