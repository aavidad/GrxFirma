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
    "CloseButtonText", "ToolTip", "Name", "HelpText",
    "FullDescription",
}


class WinUiLocalizerContractTests(unittest.TestCase):
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
            used.update(re.findall(
                r'Localizer\.(?:Text|Format)\("([^"\n]+)"',
                path.read_text(encoding="utf-8")))
        self.assertTrue(used)
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            with self.subTest(locale=path.stem):
                self.assertFalse(used - catalog.keys(), sorted(used - catalog.keys()))

    def test_assigned_xaml_literals_exist_in_shared_catalog(self):
        catalog = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
        known = catalog.keys() | set(catalog.values())
        paths = [UI / "Views" / f"{name}Page.xaml"
                 for name in ("Sign", "Verify", "Hash", "Protect")]
        paths += [UI / "Controls" / name for name in
                  ("SigningMethodsHelp.xaml", "OperationDiagnosticDialog.xaml")]
        xaml_namespace = "{http://schemas.microsoft.com/winfx/2006/xaml}"
        for path in paths:
            for element in ET.parse(path).getroot().iter():
                for raw_name, value in element.attrib.items():
                    if raw_name.startswith(xaml_namespace):
                        continue
                    name = raw_name.rsplit("}", 1)[-1]
                    if name.startswith(("AutomationProperties.", "ToolTipService.")):
                        name = name.split(".", 1)[1]
                    elif "." in name:
                        continue
                    if (name in VISIBLE and re.search(r"[A-Za-zÀ-ÿ]", value)
                            and not value.startswith("{") and value != "GrxFirma"):
                        with self.subTest(path=path.name, text=value[:60]):
                            self.assertIn(value, known)

    def test_composite_message_placeholders_match_every_locale(self):
        spanish = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
        placeholders = re.compile(r"\{\d+\}")
        keys = [key for key in spanish if placeholders.search(key)]
        self.assertTrue(keys)
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key in keys:
                with self.subTest(locale=path.stem, text=key[:60]):
                    self.assertEqual(
                        sorted(placeholders.findall(spanish[key])),
                        sorted(placeholders.findall(catalog[key])))

    def test_readonly_filename_boxes_localize_their_empty_state(self):
        # Localizer skips TextBox.Text to avoid translating user data.
        for page, labels in {
            "Sign": ("Ningún documento seleccionado",),
            "Verify": ("Ningún fichero seleccionado", "No seleccionado"),
            "Hash": ("Ningún origen seleccionado", "Ningún manifiesto seleccionado"),
            "Protect": ("Ningún documento seleccionado", "Ningún contenedor seleccionado"),
        }.items():
            source = (UI / "ViewModels" / f"{page}PageViewModel.cs").read_text(
                encoding="utf-8")
            for label in labels:
                with self.subTest(page=page, label=label):
                    self.assertIn(f'Localizer.Text("{label}")', source)

    def test_localized_suggested_file_names_remain_safe(self):
        names = ("documento-firmado", "-firmado", "documento-protegido",
                 "-protegido", "diagnostico-grxfirma-")
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for name in names:
                with self.subTest(locale=path.stem, name=name):
                    value = catalog[name]
                    self.assertTrue(value)
                    self.assertFalse(any(char in value for char in '/\\:*?"<>|'))

    def test_assigned_csharp_messages_have_catalog_entries(self):
        catalog = json.loads((LOCALES / "es.json").read_text(encoding="utf-8"))
        known = catalog.keys() | set(catalog.values())
        paths = [UI / "Views" / f"{name}Page.xaml.cs"
                 for name in ("Sign", "Verify", "Hash", "Protect")]
        paths += [UI / "ViewModels" / f"{name}PageViewModel.cs"
                  for name in ("Sign", "Verify", "Hash", "Protect")]
        paths += [UI / "Controls" / name for name in
                  ("CertificateCardSelection.cs", "CertificateValidationDialog.cs",
                   "OperationDiagnosticDialog.xaml.cs", "SigningMethodsHelp.xaml.cs")]
        literals = re.compile(r'(?P<inter>\$)?"(?P<body>(?:\\.|[^"\\])*)"')
        spanish = re.compile(
            r"[áéíóúñÁÉÍÓÚÑ]|\b(?:de|del|para|con|sin|una|un|los|las|"
            r"que|se|el|la|en|por|firma|certificado|resultado|documento|"
            r"archivo|fichero|clave|sello|operación|motor|huella|"
            r"manifiesto|destinatarios)\b", re.IGNORECASE)
        fragments = (
            "El perfil compatible requiere una identidad RSA con clave privada descifrable, por ejemplo un P12/PFX autorizado. ",
            "Los certificados opacos del almacén de Windows siguen disponibles para firmar, pero no se ofrecen para cifrado; ",
            "cargue un P12/PFX apto o cambie al perfil alto.",
        )
        self.assertIn("".join(fragments), known)
        for path in paths:
            for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
                if line.lstrip().startswith(("//", "///")):
                    continue
                for match in literals.finditer(line):
                    value = match.group("body")
                    if match.group("inter"):
                        fields = {}

                        def placeholder(found):
                            expression = found.group(1).split(":", 1)[0]
                            return "{" + str(fields.setdefault(expression, len(fields))) + "}"

                        value = re.sub(r"\{([^{}]+)\}", placeholder, value)
                    if (value in known or value in fragments or
                            value in {"canceló", "integridad y confianza"} or
                            re.fullmatch(r"[a-z]+-[a-z]+", value) or
                            not spanish.search(value)):
                        continue
                    with self.subTest(path=path.name, line=line_number, text=value[:60]):
                        self.fail(f"Missing visible message: {value}")


if __name__ == "__main__":
    unittest.main()
