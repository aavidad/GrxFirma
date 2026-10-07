# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contratos de la auditoría WCAG 2.1 AA de la aplicación WinUI (W-01 a W-15).

Cada prueba protege una corrección concreta; si se revierte, la prueba falla.
"""

import json
import pathlib
import re
import unittest
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
XAML_NS = "http://schemas.microsoft.com/winfx/2006/xaml"
KEY = f"{{{XAML_NS}}}Key"
STATUS_BRUSHES = (
    "AppDiagnosticSuccessBrush",
    "AppDiagnosticFailureBrush",
    "AppDiagnosticUnknownBrush",
    "AppDiagnosticSkippedBrush",
)
# Fondos reales sobre los que se pinta el texto de estado, además de la
# tarjeta y la página de App.xaml: LayerFillColorDefaultBrush de las tarjetas
# de certificado (claro: #80FFFFFF sobre la página; oscuro: medido en la VM).
EXTRA_BACKGROUNDS = {"Light": ("#FFFBFBFC",), "Dark": ("#FF36373A",)}


def read(path):
    return pathlib.Path(path).read_text(encoding="utf-8")


def local_name(element):
    return element.tag.rsplit("}", 1)[-1]


def rgb(color):
    value = color.lstrip("#")
    if len(value) == 8:
        alpha = int(value[:2], 16)
        self_check = alpha == 255
        if not self_check:
            raise AssertionError(f"color de texto con transparencia: {color}")
        value = value[2:]
    return tuple(int(value[index:index + 2], 16) / 255 for index in (0, 2, 4))


def luminance(color):
    def channel(component):
        return component / 12.92 if component <= 0.03928 else ((component + 0.055) / 1.055) ** 2.4
    red, green, blue = (channel(component) for component in rgb(color))
    return 0.2126 * red + 0.7152 * green + 0.0722 * blue


def contrast(foreground, background):
    lighter, darker = sorted((luminance(foreground), luminance(background)), reverse=True)
    return (lighter + 0.05) / (darker + 0.05)


def theme_dictionaries():
    root = ET.parse(UI / "App.xaml").getroot()
    themes = {}
    for element in root.iter():
        if local_name(element) == "ResourceDictionary" and element.get(KEY) in {"Light", "Dark", "HighContrast"}:
            themes[element.get(KEY)] = {
                child.get(KEY): child.get("Color")
                for child in element
                if local_name(child) == "SolidColorBrush"
            }
    return themes


def csharp_sources():
    return sorted(UI.rglob("*.cs"))


def method(source, signature):
    start = source.index(signature)
    brace = source.index("{", start)
    depth = 0
    for index in range(brace, len(source)):
        depth += {"{": 1, "}": -1}.get(source[index], 0)
        if depth == 0:
            return source[start:index + 1]
    raise AssertionError(signature)


class StatusColourContrastTests(unittest.TestCase):
    """W-01/W-02 (1.4.3): colores de estado por tema y con 4,5:1."""

    def test_status_brushes_live_only_in_theme_dictionaries(self):
        root = ET.parse(UI / "App.xaml").getroot()
        resources = next(element for element in root.iter() if local_name(element) == "ResourceDictionary")
        top_level = {child.get(KEY) for child in resources}
        for brush in STATUS_BRUSHES:
            self.assertNotIn(brush, top_level, f"{brush} fuera de ThemeDictionaries")
        themes = theme_dictionaries()
        for theme in ("Light", "Dark", "HighContrast"):
            for brush in STATUS_BRUSHES:
                self.assertIn(brush, themes[theme], f"{theme}: falta {brush}")

    def test_status_text_reaches_four_and_a_half_to_one_on_every_background(self):
        themes = theme_dictionaries()
        for theme in ("Light", "Dark"):
            backgrounds = (
                themes[theme]["AppCardBackgroundBrush"],
                themes[theme]["AppPageBackgroundBrush"],
                *EXTRA_BACKGROUNDS[theme],
            )
            for brush in STATUS_BRUSHES:
                for background in backgrounds:
                    with self.subTest(theme=theme, brush=brush, background=background):
                        ratio = contrast(themes[theme][brush], background)
                        self.assertGreaterEqual(round(ratio, 2), 4.5, f"{ratio:.2f}:1")

    def test_high_contrast_uses_system_text_colour(self):
        themes = theme_dictionaries()
        for brush in STATUS_BRUSHES:
            self.assertEqual(themes["HighContrast"][brush], "{ThemeResource SystemColorWindowTextColor}")

    def test_contrast_formula_matches_wcag_reference_values(self):
        self.assertAlmostEqual(contrast("#FF000000", "#FFFFFFFF"), 21.0, places=2)
        self.assertAlmostEqual(contrast("#FF767676", "#FFFFFFFF"), 4.54, places=2)

    def test_code_reads_theme_brushes_through_the_theme_helper(self):
        theme_keys = set().union(*(set(values) for values in theme_dictionaries().values()))
        lookup = re.compile(r'Application\.Current\.Resources\[\s*"([^"]+)"\s*\]')
        for path in csharp_sources():
            if path.name == "ThemeBrushes.cs":
                continue
            source = read(path)
            with self.subTest(file=path.name):
                for key in lookup.findall(source):
                    self.assertNotIn(key, theme_keys, f"{path.name} lee {key} sin ThemeBrushes")
                self.assertNotRegex(source, r'Application\.Current\.Resources\[\s*resourceKey')
                self.assertNotRegex(source, r'Resources\[\s*\n?\s*\w*\s*\?\s*"AppDiagnostic')
        helper = read(UI / "Controls/ThemeBrushes.cs")
        for expected in ("ThemeDictionaries", "ActualTheme", "AccessibilitySettings", "HighContrast"):
            self.assertIn(expected, helper)
        self.assertIn("ThemeBrushes.SetThemeRoot(AppRoot)", read(UI / "MainWindow.xaml.cs"))
        for converter in ("CertificateStatusBrushConverter.cs", "DiagnosticStatusBrushConverter.cs"):
            self.assertIn("ThemeBrushes.Get(resourceKey)", read(UI / "Controls" / converter))


class LiveRegionTests(unittest.TestCase):
    """W-03 (4.1.3): todo texto con LiveSetting lanza LiveRegionChanged."""

    def test_every_live_text_block_in_xaml_is_watched(self):
        checked = 0
        for path in sorted(UI.rglob("*.xaml")):
            source = read(path)
            for match in re.finditer(r"<TextBlock\b[^<>]*?/?>", source, re.S):
                tag = match.group(0)
                if "LiveSetting=" not in tag:
                    continue
                checked += 1
                line = source[:match.start()].count("\n") + 1
                with self.subTest(file=path.name, line=line):
                    self.assertIn('controls:LiveAnnouncer.Watch="True"', tag)
                    self.assertIn('xmlns:controls="using:GrxFirma.WinUI.Controls"', source)
        self.assertGreaterEqual(checked, 38)

    def test_every_live_text_block_created_in_code_is_watched(self):
        set_live = re.compile(r"SetLiveSetting\(\s*(\w+)\s*,")
        for name in ("EniPage.xaml.cs", "SignPage.xaml.cs"):
            source = read(UI / "Views" / name)
            for variable in set_live.findall(source):
                with self.subTest(file=name, element=variable):
                    self.assertRegex(source, rf"LiveAnnouncer\.Watch\({re.escape(variable)}\)")

    def test_announcer_raises_the_live_region_event_after_text_and_visibility_changes(self):
        helper = read(UI / "Controls/LiveAnnouncer.cs")
        for expected in (
            "RegisterPropertyChangedCallback(TextBlock.TextProperty",
            "RegisterPropertyChangedCallback(UIElement.VisibilityProperty",
            "DispatcherQueuePriority.Low",
            "RaiseAutomationEvent(AutomationEvents.LiveRegionChanged)",
            "element.IsLoaded",
        ):
            self.assertIn(expected, helper)


class FacturaeStatusTests(unittest.TestCase):
    """W-04 y W-05: aviso que se reanuncia, foco en el error y obligatorios."""

    def setUp(self):
        self.vm = read(UI / "ViewModels/FacturaePageViewModel.cs")
        self.xaml = read(UI / "Views/FacturaePage.xaml")
        self.page = read(UI / "Views/FacturaePage.xaml.cs")

    def test_each_message_closes_and_reopens_its_info_bar(self):
        show = method(self.vm, "private void ShowStatus(")
        self.assertLess(show.index("HasStatus = false"), show.index("HasStatus = true"))
        self.assertLess(show.index("HasCreateStatus = false"), show.index("HasCreateStatus = true"))
        begin = method(self.vm, "private CancellationTokenSource? TryBeginOperation(")
        self.assertIn("HasStatus = false", begin)
        self.assertNotIn("HasStatus = true", begin)
        outside = self.vm.replace(show, "")
        self.assertNotIn("HasStatus = true", outside)
        self.assertNotRegex(outside, r"\n\s*StatusTitle = ")

    def test_create_result_is_shown_below_the_create_button(self):
        button = self.xaml.index('Click="OnCreateInvoiceClick"')
        info_bar = self.xaml.index('x:Name="CreateStatusInfoBar"')
        self.assertLess(button, info_bar)
        self.assertIn('IsOpen="{x:Bind ViewModel.HasCreateStatus, Mode=OneWay}"', self.xaml)
        self.assertIn("TryBeginOperation(statusNearCreate: true)", self.vm)

    def test_invalid_data_moves_focus_to_the_first_empty_required_field(self):
        self.assertGreaterEqual(self.vm.count("InvalidDataReported?.Invoke"), 2)
        self.assertIn("ViewModel.InvalidDataReported += (_, _) => FocusFirstEmptyRequiredField()", self.page)
        focus = method(self.page, "private void FocusFirstEmptyRequiredField(")
        for expected in ("GetIsRequiredForForm", "OrderBy(control => control.TabIndex)",
                         "expander.IsExpanded = true", "Focus(FocusState.Programmatic)"):
            self.assertIn(expected, focus)

    def test_starred_fields_are_required_for_form_and_explained(self):
        root = ET.fromstring(self.xaml)
        starred = [element for element in root.iter() if "*" in element.get("Header", "")]
        self.assertEqual(len(starred), 21)
        for element in starred:
            with self.subTest(field=element.get("Header")):
                self.assertEqual(element.get("AutomationProperties.IsRequiredForForm"), "True")
        self.assertIn('x:Name="RequiredFieldsLegend"', self.xaml)
        self.assertIn('Localizer.Text("winui.facturae.campos_obligatorios")', self.page)
        for path in LOCALES.glob("*.json"):
            catalog = json.loads(read(path))
            self.assertIn("*", catalog["winui.facturae.campos_obligatorios"], path.name)


class FieldHelpTests(unittest.TestCase):
    """W-06: el error se suma a la ayuda del campo, no la sustituye."""

    def test_validation_keeps_the_original_help_text(self):
        for name in ("SignPage.xaml.cs", "SettingsPage.xaml.cs", "EniPage.xaml.cs"):
            source = read(UI / "Views" / name)
            with self.subTest(file=name):
                self.assertNotIn("SetHelpText(field, detail)", source)
                self.assertIn("FieldValidationFeedback.Apply(field, detail)", source)
        helper = read(UI / "Controls/FieldValidationFeedback.cs")
        compose = method(helper, "public static string Compose(")
        self.assertIn('detail + " " + baseHelp', compose)
        self.assertIn("ConditionalWeakTable<Control, HelpState>", helper)


class SealEditorTests(unittest.TestCase):
    """W-07 a W-10: foco, portal, giro anunciado y contraste del dibujo."""

    def setUp(self):
        self.xaml = read(UI / "Views/SignPage.xaml")
        self.code = read(UI / "Views/SignPage.xaml.cs")

    def test_rotate_handle_and_methods_help_have_tab_index(self):
        handle = re.search(r"<controls:SealRotateButton\b[^>]*>", self.xaml, re.S).group(0)
        self.assertRegex(handle, r'TabIndex="\d+"')
        for page in ("SignPage.xaml", "CertificatesPage.xaml"):
            self.assertRegex(read(UI / "Views" / page), r'<controls:SigningMethodsHelp TabIndex="\d+" />')

    def test_portal_view_orders_focus_headings_and_reflows(self):
        portal = method(self.code, "private void ConfigurePortalSealLayout(")
        order = portal[portal.index("var portalTabOrder"):]
        order = order[:order.index("};")]
        self.assertLess(order.index("_portalSignButton"), order.index("VisibleSealDrawToggle"))
        self.assertIn("portalTabOrder[index].TabIndex = index", portal)
        self.assertIn("AutomationHeadingLevel.Level1", portal)
        self.assertIn("AutomationHeadingLevel.Level2", portal)
        self.assertNotIn("new ScrollViewer", portal)
        self.assertNotIn("HorizontalScrollMode = ScrollMode.Auto", portal)
        self.assertGreaterEqual(portal.count("new ResponsiveActionPanel"), 3)

    def test_rotation_value_is_announced(self):
        keys = method(self.code, "private void OnVisibleSealRotateKeyDown(")
        self.assertIn("AnnounceSealRotation()", keys)
        announce = method(self.code, "private void AnnounceSealRotation(")
        for expected in ('"winui.firmar.sello_giro_actual"', "SetItemStatus(", "RaiseNotificationEvent("):
            self.assertIn(expected, announce)
        catalog = json.loads(read(LOCALES / "es.json"))
        self.assertEqual(catalog["winui.firmar.sello_giro_actual"], "Giro del sello: {0} grados")

    def test_drawing_rectangle_contrasts_with_the_paper(self):
        band = self.xaml[self.xaml.index('x:Name="VisibleSealDrawRubberBand"'):]
        band = band[:band.index("</Border>")]
        self.assertNotIn("AccentFillColorDefaultBrush", band)
        self.assertIn("AppSealBorderBrush", band)
        self.assertIn("AppSealHandleTextBrush", band)
        self.assertNotIn('Background="White"', self.xaml)
        self.assertIn('Background="{ThemeResource AppSealPaperBrush}"', self.xaml)
        themes = theme_dictionaries()
        self.assertEqual(themes["HighContrast"]["AppSealPaperBrush"], "{ThemeResource SystemColorWindowColor}")
        for theme in ("Light", "Dark"):
            ratio = contrast(themes[theme]["AppSealBorderBrush"], themes[theme]["AppSealPaperBrush"])
            self.assertGreaterEqual(ratio, 3.0, f"{theme}: {ratio:.2f}:1")


class CertificateCardTests(unittest.TestCase):
    """W-11 y W-12: nombre y ayuda en el elemento real; textos sin recortar."""

    def test_list_item_help_is_set_on_the_container(self):
        selection = read(UI / "Controls/CertificateCardSelection.cs")
        self.assertIn("SetHelpText(item", selection)
        self.assertIn("SuitabilitySummary", selection)
        for page in ("SignPage.xaml", "CertificatesPage.xaml"):
            source = read(UI / "Views" / page)
            self.assertNotRegex(source, r'<StackPanel\s+AutomationProperties\.HelpText="\{Binding SuitabilitySummary\}"')
            self.assertNotIn('AutomationProperties.Name="{Binding DisplayName}"', source)

    def test_name_and_status_wrap_and_truncated_text_has_tooltip(self):
        for page in ("SignPage.xaml", "CertificatesPage.xaml"):
            root = ET.parse(UI / "Views" / page).getroot()
            for element in root.iter():
                if local_name(element) != "TextBlock":
                    continue
                text = element.get("Text", "")
                with self.subTest(page=page, text=text):
                    if text in ("{Binding DisplayName}", "{Binding CardStatusText}"):
                        self.assertEqual(element.get("TextWrapping"), "Wrap")
                        self.assertIsNone(element.get("TextTrimming"))
                    if text in ("{Binding ExpirationDisplay}", "{Binding ShortIssuerDisplay}"):
                        self.assertEqual(element.get("ToolTipService.ToolTip"), text)


class MinorFindingsTests(unittest.TestCase):
    """W-14 y W-15."""

    def test_about_logo_is_decorative(self):
        root = ET.parse(UI / "Views/AboutPage.xaml").getroot()
        logo = next(element for element in root.iter()
                    if local_name(element) == "Image" and "logo" in element.get("Source", ""))
        self.assertEqual(logo.get("AutomationProperties.AccessibilityView"), "Raw")
        self.assertIsNone(logo.get("AutomationProperties.Name"))

    def test_diagnostic_status_circle_grows_with_text(self):
        source = read(UI / "Views/DiagnosticsPage.xaml")
        self.assertNotRegex(source, r'Width="40"\s+Height="40"')
        self.assertIn('MinWidth="40"', source)
        self.assertIn('MinHeight="40"', source)
        self.assertNotIn('<ColumnDefinition Width="44" />', source)


if __name__ == "__main__":
    unittest.main()
