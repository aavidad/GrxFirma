# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[1] / "src/GrxFirma.WinUI"
NS = "{http://schemas.microsoft.com/winfx/2006/xaml/presentation}"


class SigningMethodsHelpContractTests(unittest.TestCase):
    def setUp(self):
        self.root = ET.parse(ROOT / "Controls/SigningMethodsHelp.xaml").getroot()

    def test_native_collapsed_expander_exposes_accessible_name(self):
        expander = self.root.find(NS + "Expander")
        self.assertEqual(expander.get("IsExpanded"), "False")
        self.assertEqual(expander.get("AutomationProperties.Name"), "Otras formas de firmar")
        self.assertEqual(expander.get("HorizontalContentAlignment"), "Stretch")
        self.assertGreaterEqual(int(expander.get("MinHeight")), 44)
        self.assertIsNone(expander.get("IsTabStop"))
        header = expander.find(NS + "Expander.Header").find(NS + "TextBlock")
        self.assertEqual(header.get("AutomationProperties.HeadingLevel"), "Level2")

    def test_help_wraps_and_inherits_native_theme_without_side_effects(self):
        for element in self.root.iter():
            if element.tag == NS + "TextBlock":
                self.assertEqual(element.get("TextWrapping"), "Wrap")
            self.assertNotIn(element.tag, [NS + "Button", NS + "HyperlinkButton", NS + "ScrollViewer"])
            for attribute in ["Click", "Command", "Width", "Height", "Foreground", "Background", "RequestedTheme"]:
                self.assertNotIn(attribute, element.attrib)

    def test_methods_are_explanations_not_claims_of_universal_support(self):
        text = " ".join(element.get("Text", "") for element in self.root.iter())
        for phrase in ["otros emisores compatibles", "no es exclusivo de FNMT", "sin instalarlo", "tras su confirmación", "DNIe", "tokens USB", "no se garantiza compatibilidad universal", "Cl@ve Firma", "no puede añadirse a este selector"]:
            self.assertIn(phrase, text)

    def test_both_pages_reuse_one_component(self):
        for page in ["Sign", "Certificates"]:
            root = ET.parse(ROOT / f"Views/{page}Page.xaml").getroot()
            instances = [element for element in root.iter() if element.tag.endswith("}SigningMethodsHelp")]
            self.assertEqual(len(instances), 1, page)


if __name__ == "__main__":
    unittest.main()
