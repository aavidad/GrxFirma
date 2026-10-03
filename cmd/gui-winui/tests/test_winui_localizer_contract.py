# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
import pathlib
import re
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
VISIBLE = {
    "Text", "Content", "Header", "PlaceholderText", "Title", "Message",
    "Description", "PrimaryButtonText", "SecondaryButtonText",
    "CloseButtonText", "ToolTip", "Name", "HelpText", "FullDescription",
}
VISIBLE_XAML_ATTRIBUTES = VISIBLE - {"Name", "HelpText", "FullDescription", "ToolTip"}
VISIBLE_XAML_ATTRIBUTES |= {
    "AutomationProperties.Name", "AutomationProperties.HelpText",
    "AutomationProperties.FullDescription", "ToolTipService.ToolTip",
}
GROUP_B_XAML = {
    "MainWindow.xaml", "OperationDiagnosticDialog.xaml",
    "CertificatesPage.xaml", "SettingsPage.xaml", "DiagnosticsPage.xaml",
    "AboutPage.xaml", "FacturaePage.xaml", "EniPage.xaml",
}
GROUP_B_PAGES = {"CertificatesPage", "SettingsPage", "DiagnosticsPage",
                 "AboutPage", "FacturaePage", "EniPage", "MainWindow"}
GROUP_B_CS = {"App.xaml.cs", "MainWindow.xaml.cs"}
PROPER_NAMES = {"GrxFirma", "FNMT", "FACe", "DIR3", "PAdES", "CAdES",
                "XAdES", "ASiC", "ENI", "CSV", "PKCS#11", "PKCS#12", "DNIe"}


def static_xaml_literals(paths):
    for path in paths:
        for element in ET.parse(path).getroot().iter():
            tag = element.tag.rsplit("}", 1)[-1]
            if tag in {"TextBlock", "Run", "Span", "Hyperlink", "Button",
                       "NavigationViewItem", "MenuFlyoutItem"}:
                value = (element.text or "").strip()
                if (value and not value.startswith("{")
                        and re.search(r"[A-Za-zÀ-ÿ]", value)
                        and value not in PROPER_NAMES):
                    yield path.relative_to(UI).as_posix(), "inner text", value
            for raw_name, value in element.attrib.items():
                name = raw_name.rsplit("}", 1)[-1]
                if (name not in VISIBLE_XAML_ATTRIBUTES or value.startswith("{")
                        or not re.search(r"[A-Za-zÀ-ÿ]", value)
                        or value in PROPER_NAMES):
                    continue
                yield path.relative_to(UI).as_posix(), name, value


def untranslated_xaml(paths):
    spanish = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
    spanish_values = {value: key for key, value in spanish.items()}
    catalogs = [json.loads(path.read_text(encoding="utf-8"))
                for path in LOCALES.glob("*.json")]
    return [(path, name, value) for path, name, value in static_xaml_literals(paths)
            if (key := value if value in spanish else spanish_values.get(value)) is None
            or any(not catalog.get(key) for catalog in catalogs)]


def belongs_to_group_b_cs(path):
    relative = path.relative_to(UI)
    if relative.name in GROUP_B_CS:
        return True
    if relative.parts[0] in {"Views", "ViewModels"}:
        return any(relative.name.startswith(name) for name in GROUP_B_PAGES)
    if relative.parts[0] == "Controls":
        return "Dialog" in relative.name
    return relative.parts[0] == "Services"


def csharp_string_literals(source):
    """Walk C# tokens so quoted characters and comments cannot join two strings."""
    index = 0
    line = 1
    while index < len(source):
        if source.startswith("//", index):
            end = source.find("\n", index)
            index = len(source) if end < 0 else end
            continue
        if source.startswith("/*", index):
            end = source.find("*/", index + 2)
            end = len(source) if end < 0 else end + 2
            line += source.count("\n", index, end)
            index = end
            continue
        if source[index] == "'":
            index += 1
            while index < len(source):
                if source[index] == "\\":
                    index += 2
                elif source[index] == "'":
                    index += 1
                    break
                else:
                    line += source[index] == "\n"
                    index += 1
            continue
        prefix = re.match(r'(?:\$@|@\$|\$|@)?"', source[index:])
        if prefix:
            start_line = line
            verbatim = "@" in prefix.group()
            index += len(prefix.group())
            value = []
            while index < len(source):
                if verbatim and source.startswith('""', index):
                    value.append('""')
                    index += 2
                elif source[index] == '"':
                    index += 1
                    break
                elif not verbatim and source[index] == "\\":
                    value.append(source[index:index + 2])
                    index += 2
                else:
                    value.append(source[index])
                    line += source[index] == "\n"
                    index += 1
            yield start_line, "".join(value)
            continue
        line += source[index] == "\n"
        index += 1


def untranslated_csharp(paths):
    spanish = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
    known = set(spanish) | set(spanish.values())
    value_to_key = {value: key for key, value in spanish.items()}
    catalogs = [json.loads(path.read_text(encoding="utf-8"))
                for path in LOCALES.glob("*.json")]
    missing = []
    language_names = {"Català", "Valencià", "Français", "Português"}
    for path in paths:
        source = path.read_text(encoding="utf-8")
        for line, value in csharp_string_literals(source):
            without_interpolation = re.sub(r"\{[^{}]*\}", "", value)
            key = value if value in spanish else value_to_key.get(value)
            if ((value in known and all(catalog.get(key) for catalog in catalogs))
                    or value in PROPER_NAMES or value in language_names
                    or not re.search(r"[A-Za-zÀ-ÿ]", without_interpolation)
                    or re.fullmatch(r"[\s0-9xX:.+\-]*", without_interpolation)
                    or not (re.search(r"[À-ÿ¿¡]", without_interpolation)
                            or " " in without_interpolation)
                    or value.startswith(("http://", "https://", "/"))
                    or "/" in value and " " not in value):
                continue
            missing.append((path.relative_to(UI).as_posix(), line, value))
    return missing


class WinUiLocalizerContractTests(unittest.TestCase):
    def test_csharp_scanner_ignores_comments_and_character_literals(self):
        source = """// \"comentario\"\nvar quote = '\"';\nvar label = \"Texto visible\";"""
        self.assertEqual(list(csharp_string_literals(source)),
                         [(3, "Texto visible")])

    def test_group_b_visible_xaml_literals_are_in_the_shared_catalog(self):
        paths = (path for path in UI.rglob("*.xaml") if path.name in GROUP_B_XAML)
        missing = untranslated_xaml(paths)
        if missing:
            self.fail(f"{len(missing)} visible group B literals lack a catalog key: {missing[:20]}")

    def test_group_b_csharp_human_text_is_in_the_shared_catalog(self):
        paths = (path for path in UI.rglob("*.cs") if belongs_to_group_b_cs(path))
        missing = untranslated_csharp(paths)
        if missing:
            self.fail(f"{len(missing)} group B C# text candidates lack a catalog key: {missing[:20]}")

    @unittest.expectedFailure  # Grupo A se integra desde la otra rama.
    def test_pending_group_a_visible_xaml_literals_are_in_the_shared_catalog(self):
        paths = (path for path in UI.rglob("*.xaml") if path.name not in GROUP_B_XAML)
        missing = untranslated_xaml(paths)
        if missing:
            self.fail(f"{len(missing)} pending group A literals: {missing[:20]}")

    @unittest.expectedFailure  # Archivos C# fuera del grupo B pertenecen a la integración pendiente.
    def test_pending_other_csharp_human_text_is_in_the_shared_catalog(self):
        paths = (path for path in UI.rglob("*.cs") if not belongs_to_group_b_cs(path))
        missing = untranslated_csharp(paths)
        if missing:
            self.fail(f"{len(missing)} pending C# text candidates: {missing[:20]}")

    def test_visible_xaml_property_types_are_observed(self):
        implementation = (UI / "Services/Localizer.cs").read_text(encoding="utf-8")
        for path in UI.rglob("*.xaml"):
            for element in ET.parse(path).getroot().iter():
                for raw_name, value in element.attrib.items():
                    name = raw_name.rsplit("}", 1)[-1]
                    if name.startswith("AutomationProperties."):
                        name = name.split(".", 1)[1]
                    if name not in VISIBLE or not re.search(r"[A-Za-zÀ-ÿ]", value):
                        continue
                    if value.startswith("{") or value == "GrxFirma":
                        continue
                    with self.subTest(path=path.name, property=name, text=value[:60]):
                        if name == "ToolTip":
                            self.assertIn("ToolTipService.ToolTipProperty", implementation)
                        elif name in {"Name", "HelpText", "FullDescription"}:
                            self.assertIn(f"AutomationProperties.{name}Property", implementation)
                        else:
                            self.assertIn(f'"{name}"', implementation)

    def test_dynamic_dialogs_pass_through_localizer(self):
        for path in UI.rglob("*.cs"):
            if path.name == "Localizer.cs":
                continue
            source = path.read_text(encoding="utf-8")
            with self.subTest(path=str(path.relative_to(UI))):
                self.assertNotRegex(source, r"\b(?:dialog|confirmation)\.ShowAsync\(\)")

    def test_selected_language_refreshes_page_and_menu(self):
        window = (UI / "MainWindow.xaml.cs").read_text(encoding="utf-8")
        settings = (UI / "Views/SettingsPage.xaml.cs").read_text(encoding="utf-8")
        catalog = (UI / "Services/SealUiCatalog.cs").read_text(encoding="utf-8")
        self.assertIn("ApplyLanguagePreference(result.Data.Language)", window)
        self.assertIn("RootNavigation.FooterMenuItems", window)
        self.assertIn("ContentFrame.Navigate(pageType)", window)
        self.assertIn("_app.RefreshTrayLanguage()", window)
        self.assertIn("app.ApplyLanguagePreference(ViewModel.SelectedLanguage.Value)", settings)
        self.assertIn("Localizer.Text(key)", catalog)
        self.assertNotIn("se muestra actualmente en español", (UI / "Views/SettingsPage.xaml").read_text(encoding="utf-8"))

    def test_explicit_localizer_keys_exist_in_every_catalog(self):
        used = set()
        for path in UI.rglob("*.cs"):
            if path.name == "Localizer.cs":
                continue
            used.update(re.findall(r'Localizer\.Text\("([^"\n]+)"\)', path.read_text(encoding="utf-8")))
        self.assertTrue(used)
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            with self.subTest(locale=path.stem):
                self.assertFalse(used - catalog.keys(), sorted(used - catalog.keys()))


if __name__ == "__main__":
    unittest.main()
