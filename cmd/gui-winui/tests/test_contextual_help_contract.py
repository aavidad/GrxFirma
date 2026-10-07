# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato de los botones «?» de ayuda contextual (WinUI).

docs/AYUDA_CONTEXTUAL.md asigna a cada opción técnica una clave ``ayuda.*`` y
el control de WinUI junto al que se muestra. Estas pruebas comprueban que cada
clave asignada a WinUI se usa, que cada pantalla tiene sus «?», que el botón
forma su nombre accesible con ``ayuda.boton_nombre`` y que ningún texto queda
fijado en el código.
"""

import json
import pathlib
import re
import unittest
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
VIEWS = UI / "Views"
DOC = ROOT / "docs/AYUDA_CONTEXTUAL.md"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
CONTROLS = "{using:GrxFirma.WinUI.Controls}"
XNAME = "{http://schemas.microsoft.com/winfx/2006/xaml}Name"
# Reparto de la fase 2 en docs/AYUDA_CONTEXTUAL.md. El «?» del perfil de
# firma (SignProfileHelpButton) es de otra rama y no cuenta aquí.
EXPECTED = {
    "SignPage.xaml": 19,
    "VerifyPage.xaml": 6,
    "SettingsPage.xaml": 8,
    "CertificatesPage.xaml": 3,
    "ProtectPage.xaml": 7,
    "HashPage.xaml": 4,
    "EniPage.xaml.cs": 5,
    "FacturaePage.xaml": 3,
}
NOT_IN_WINUI = "no aparece"
WITH_HELP = re.compile(r'WithHelp\(\s*\w+,\s*"([^"]+)",\s*"([^"]+)"')


def catalogs():
    return {path.stem: json.loads(path.read_text(encoding="utf-8"))
            for path in sorted(LOCALES.glob("*.json"))}


def winui_keys():
    """Claves cuya columna WinUI no dice «no aparece»."""
    keys = set()
    header = None
    for line in DOC.read_text(encoding="utf-8").splitlines():
        if not line.startswith("|"):
            header = None
            continue
        cells = [cell.strip() for cell in line.strip("|").split("|")]
        if header is None:
            header = cells
            continue
        if set(line) <= set("|-: "):
            continue
        if "Clave" not in header or "WinUI" not in header:
            continue
        key = cells[header.index("Clave")].strip("`")
        if key.startswith("ayuda.") and cells[header.index("WinUI")] != NOT_IN_WINUI:
            keys.add(key)
    return keys


def options(button):
    """Pares (nombre, clave) del atributo Options de un «?» de selector."""
    return [tuple(item.split("=", 1)) for item in button.get("Options", "").split()]


def option_keys(button):
    return " ".join(key for _, key in options(button))


def button_by_topic(screen, topic):
    return next(b for b in help_buttons(VIEWS / screen) if b.get("Topic") == topic)


OPERATION_KEYS = ["ayuda.operacion.firma", "ayuda.operacion.cofirma", "ayuda.operacion.contrafirma"]
FORMAT_KEYS = ["ayuda.formato.automatico", "ayuda.formato.pades", "ayuda.formato.cades",
               "ayuda.formato.xades", "ayuda.formato.xmldsig", "ayuda.formato.odf",
               "ayuda.formato.ooxml", "ayuda.formato.facturae", "ayuda.formato.asic"]


def help_buttons(path):
    root = ET.parse(path).getroot()
    return [element for element in root.iter(CONTROLS + "HelpButton")]


def sources():
    for path in sorted(UI.rglob("*")):
        if path.suffix in {".xaml", ".cs"} and "obj" not in path.parts and "bin" not in path.parts:
            yield path, path.read_text(encoding="utf-8")


def translatable(text, catalog_map):
    spanish = catalog_map["es"]
    key = text if text in spanish else {v: k for k, v in spanish.items()}.get(text)
    return key is not None and all(catalog.get(key, "").strip() for catalog in catalog_map.values())


class ContextualHelpContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.catalogs = catalogs()
        cls.keys = winui_keys()

    def test_document_assigns_keys_to_winui(self):
        # Una tabla mal leída dejaría la prueba vacía.
        self.assertGreaterEqual(len(self.keys), 50)
        self.assertIn("ayuda.formato.verifactu", self.keys)
        self.assertNotIn("ayuda.sobrescribir", self.keys)

    def test_every_winui_key_is_used_in_xaml_or_csharp(self):
        used = set()
        for _, text in sources():
            used.update(re.findall(r"ayuda\.[a-z0-9_.]*[a-z0-9_]", text))
        self.assertEqual(sorted(self.keys - used), [])

    def test_every_screen_has_its_help_buttons(self):
        for name, expected in EXPECTED.items():
            with self.subTest(screen=name):
                path = VIEWS / name
                if name.endswith(".cs"):
                    count = len(WITH_HELP.findall(path.read_text(encoding="utf-8")))
                else:
                    count = len([b for b in help_buttons(path)
                                 if b.get(XNAME) != "SignProfileHelpButton"])
                self.assertEqual(count, expected)

    def test_help_buttons_name_their_control_and_use_catalog_texts(self):
        for name in EXPECTED:
            path = VIEWS / name
            if name.endswith(".cs"):
                pairs = WITH_HELP.findall(path.read_text(encoding="utf-8"))
                entries = [(topic, key, "", "") for topic, key in pairs]
            else:
                entries = [(b.get("Topic", ""), b.get("HelpKey", ""), option_keys(b),
                            b.get("HeadingKey", "")) for b in help_buttons(path)]
            for topic, *keys in entries:
                with self.subTest(screen=name, topic=topic):
                    self.assertTrue(translatable(topic, self.catalogs), topic)
                    for value in keys:
                        if not value or value.startswith("{"):
                            continue
                        for key in value.split():
                            for language, catalog in self.catalogs.items():
                                self.assertTrue(catalog.get(key, "").strip(), (language, key))

    def test_selector_help_lists_every_option_without_a_selection(self):
        # La ayuda de Operación y de Formato no depende de la opción elegida:
        # muestra el texto general y todas las opciones, cada una con su nombre.
        operation = button_by_topic("SignPage.xaml", "Operación")
        self.assertEqual(operation.get("HelpKey"), "ayuda.operacion")
        self.assertEqual([key for _, key in options(operation)], OPERATION_KEYS)
        self.assertEqual([name for name, _ in options(operation)],
                         ["winui.firmar.firma", "winui.firmar.cofirma", "winui.firmar.contrafirma"])
        sign_format = button_by_topic("SignPage.xaml", "Formato")
        self.assertEqual(sign_format.get("HelpKey"), "ayuda.formato")
        self.assertEqual([key for _, key in options(sign_format)], FORMAT_KEYS + ["ayuda.formato.verifactu"])
        settings_format = button_by_topic("SettingsPage.xaml", "Formato")
        self.assertEqual([key for _, key in options(settings_format)], FORMAT_KEYS)
        for screen in EXPECTED:
            if screen.endswith(".cs"):
                continue
            for button in help_buttons(VIEWS / screen):
                for attribute in ("HelpKey", "Options"):
                    self.assertFalse((button.get(attribute) or "").startswith("{"), (screen, attribute))
                self.assertIsNone(button.get("DetailKey"))
                for name, _ in options(button):
                    self.assertTrue(name in self.catalogs["es"] or name.isascii(), name)
        model = (UI / "ViewModels/SignPageViewModel.cs").read_text(encoding="utf-8")
        self.assertNotIn("SelectedActionHelpKey", model)
        self.assertNotIn("SelectedFormatHelpKey", model)

    def test_extended_help_exists_in_every_language_next_to_its_short_text(self):
        extended = [key for key in self.catalogs["es"] if key.startswith("ayuda.") and key.endswith(".mas")]
        self.assertGreaterEqual(len(extended), 8)
        for key in extended:
            for language, catalog in self.catalogs.items():
                with self.subTest(key=key, language=language):
                    self.assertTrue(catalog.get(key, "").strip())
                    self.assertTrue(catalog.get(key[:-len(".mas")], "").strip())
        for key in ("ayuda.mas_nombre", "ayuda.opcion"):
            for language, catalog in self.catalogs.items():
                self.assertIn("{0}", catalog[key], (language, key))
        for language, catalog in self.catalogs.items():
            self.assertIn("{1}", catalog["ayuda.opcion"], language)

    def test_italian_help_uses_informal_register_like_the_rest(self):
        # El catálogo italiano tutea; las ayudas no deben tratar de «Lei».
        formal = re.compile(r"\b(?:Lei|Le|Suo|Sua|Suoi|Sue|Gliel\w*|Glieli|contattarLa|Premere)\b")
        italian = self.catalogs["it"]
        for key, value in italian.items():
            if key.startswith(("ayuda.", "perfil_firma.ayuda.")):
                with self.subTest(key=key):
                    self.assertIsNone(formal.search(value), value)

    def test_more_button_is_a_real_expandable_button(self):
        code = (UI / "Controls/HelpButton.cs").read_text(encoding="utf-8")
        # «+» al final de la frase cuando el catálogo tiene «<clave>.mas».
        self.assertIn('MoreSuffix = ".mas"', code)
        self.assertIn("Localizer.Has(moreKey)", code)
        self.assertIn("new MoreInfoButton()", code)
        self.assertIn("AutomationProperties.SetName(more, Localizer.Format(MoreNameKey", code)
        self.assertIn("Visibility = Visibility.Collapsed", code)
        # Nombre de la opción en negrita.
        self.assertIn("FontWeight = Microsoft.UI.Text.FontWeights.Bold", code)
        # El texto ampliado puede no caber: el desplegable se desplaza.
        self.assertIn("VerticalScrollBarVisibility = ScrollBarVisibility.Auto", code)
        more = (UI / "Controls/MoreInfoButton.cs").read_text(encoding="utf-8")
        self.assertIn("class MoreInfoButton : Button", more)
        self.assertIn("IExpandCollapseProvider", more)
        self.assertIn("PatternInterface.ExpandCollapse", more)
        self.assertIn("ExpandCollapsePatternIdentifiers.ExpandCollapseStateProperty", more)
        self.assertIn("MinWidth = 32", more)
        self.assertIn("MinHeight = 32", more)
        self.assertNotIn("Foreground =", more)
        # Igual que el «?»: solo el glifo en círculo, sin fondo ni borde.
        self.assertIn("Microsoft.UI.Colors.Transparent", more)
        self.assertIn("BorderThickness = new Thickness(0)", more)
        self.assertIn("CornerRadius = new CornerRadius(16)", more)
        self.assertIn("Content = _icon", more)
        self.assertIn("_icon.Glyph = value ? ExpandedGlyph : CollapsedGlyph", more)
        self.assertNotIn("UseSystemFocusVisuals = false", more)
        literals = re.findall(r'"((?:[^"\\]|\\.)*)"', re.sub(r"//[^\n]*", "", more))
        self.assertEqual(literals, ["\\uECC8", "\\uECC9"])

    def test_help_button_is_accessible_and_has_no_fixed_text(self):
        code = (UI / "Controls/HelpButton.cs").read_text(encoding="utf-8")
        self.assertIn('"ayuda.boton_nombre"', code)
        self.assertIn("AutomationProperties.SetName(this", code)
        self.assertIn("ToolTipService.SetToolTip(this", code)
        self.assertIn("AutomationProperties.SetHelpText(this", code)
        # Objetivo táctil de 40 px (WCAG 2.5.8 pide al menos 24).
        self.assertIn("MinWidth = 40", code)
        self.assertIn("MinHeight = 40", code)
        # Se abre al pulsar (ratón, táctil o teclado), no al pasar el ratón.
        self.assertIn("Flyout = flyout", code)
        self.assertNotIn("PointerEntered", code)
        # Solo el círculo: fondo transparente y sin borde. El color del glifo,
        # el foco visible y el resaltado al pasar el ratón siguen siendo del tema.
        self.assertIn("Microsoft.UI.Colors.Transparent", code)
        self.assertIn("BorderThickness = new Thickness(0)", code)
        self.assertIn("CornerRadius = new CornerRadius(20)", code)
        self.assertNotIn("Foreground =", code)
        self.assertNotIn("UseSystemFocusVisuals = false", code)
        code_only = re.sub(r"//[^\n]*", "", code)
        literals = re.findall(r'"((?:[^"\\]|\\.)*)"', code_only)
        self.assertEqual(literals, ["\\uE9CE", "ayuda.boton_nombre", "ayuda.mas_nombre", "ayuda.opcion",
                                    ".mas", " ", "{0}", "{1}", "(\\{[01]\\})"])

    def test_every_xaml_help_button_has_a_focus_position(self):
        for name in EXPECTED:
            if name.endswith(".cs"):
                continue
            for button in help_buttons(VIEWS / name):
                with self.subTest(screen=name, topic=button.get("Topic")):
                    self.assertIsNotNone(button.get("TabIndex"))

    def test_help_buttons_sit_next_to_their_control(self):
        for name in EXPECTED:
            if name.endswith(".cs"):
                continue
            root = ET.parse(VIEWS / name).getroot()
            parents = {child: parent for parent in root.iter() for child in parent}
            for button in root.iter(CONTROLS + "HelpButton"):
                parent = parents[button]
                with self.subTest(screen=name, topic=button.get("Topic")):
                    children = list(parent)
                    self.assertGreater(children.index(button), 0)
                    if parent.tag == CONTROLS + "HelpRow":
                        self.assertEqual(len(children), 2)
                        self.assertEqual(children.index(button), 1)
                    else:
                        self.assertEqual(parent.get("Orientation"), "Horizontal")


if __name__ == "__main__":
    unittest.main()
