# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import re
import unittest
import xml.etree.ElementTree as ET


PROPS = Path(__file__).resolve().parents[1] / "Directory.Build.props"


class ProductVersionContractTests(unittest.TestCase):
    def setUp(self):
        self.root = ET.parse(PROPS).getroot()
        self.group = self.root.find("PropertyGroup")

    def test_only_product_projects_receive_metadata(self):
        condition = self.group.get("Condition")
        self.assertEqual(condition.count("$(MSBuildProjectName)"), 2)
        self.assertIn("'GrxFirma.WinUI'", condition)
        self.assertIn("'GrxFirma.WinUI.Core'", condition)
        self.assertNotIn("Tests", condition)
        self.assertEqual(self.root.find("Target").get("Condition"), condition)

    def test_canonical_version_and_numeric_pe_metadata(self):
        self.assertEqual(self.group.findtext("_GrxFirmaVersionFile"),
                         "$(MSBuildThisFileDirectory)../../VERSION.txt")
        for name in ("Version", "InformationalVersion"):
            self.assertEqual(self.group.findtext(name), "$(_GrxFirmaVersion)")
        for name in ("FileVersion", "AssemblyVersion"):
            self.assertEqual(self.group.findtext(name), "$(_GrxFirmaNumericVersion).0")
        self.assertEqual(self.group.findtext("IncludeSourceRevisionInInformationalVersion"), "false")
        self.assertIsNone(self.group.find("TargetFramework"))
        self.assertIsNone(self.group.find("PackageVersion"))

    def test_semver_release_prerelease_and_build_metadata(self):
        condition = self.root.find("Target/Error").get("Condition")
        pattern = condition.split(", '", 1)[1].rsplit("'", 1)[0]
        for value in ("2.0.1", "2.0.2-rc.1", "2.0.1+qa.42", "2.0.1-rc.1+qa.42"):
            self.assertIsNotNone(re.fullmatch(pattern, value), value)
            self.assertEqual(re.match(r"^[0-9]+\.[0-9]+\.[0-9]+", value)[0], value[:5])
        for value in ("", "dev", "v2.0.1", "02.0.1", "2.0", "2.0.1-01", "2.0.1-", "2.0.1+"):
            self.assertIsNone(re.fullmatch(pattern, value), value)


if __name__ == "__main__":
    unittest.main()
