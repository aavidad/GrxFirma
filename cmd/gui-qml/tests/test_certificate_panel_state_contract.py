# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import re
import importlib.util
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
# Las fuentes WinUI nombran claves del catálogo; se leen con su texto español.
# Se carga por ruta para no mezclar los módulos de prueba de ambas interfaces.
_SPEC = importlib.util.spec_from_file_location(
    "winui_catalog", ROOT / "cmd/gui-winui/tests/winui_catalog.py")
_WINUI_CATALOG = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(_WINUI_CATALOG)
read_with_catalog = _WINUI_CATALOG.read_with_catalog
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
WINUI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"


def luminance(color: str) -> float:
    values = [int(color[index:index + 2], 16) / 255 for index in (1, 3, 5)]
    linear = [value / 12.92 if value <= 0.04045 else
              ((value + 0.055) / 1.055) ** 2.4 for value in values]
    return sum(component * weight for component, weight in
               zip(linear, (0.2126, 0.7152, 0.0722)))


def contrast(first: str, second: str) -> float:
    high, low = sorted((luminance(first), luminance(second)), reverse=True)
    return (high + 0.05) / (low + 0.05)


class CertificatePanelStateContract(unittest.TestCase):
    def test_qt_manual_panel_and_signing_guard(self) -> None:
        self.assertIn("property bool signCertificatePanelCollapsed: true", QML)
        self.assertIn("model: window.signingCertificates()", QML)
        self.assertIn('Accessible.name: tr("Certificado de firma")', QML)
        self.assertIn('Accessible.name: tr("Ver todos los certificados")', QML)
        self.assertIn('text: tr("Ocultar")', QML)
        self.assertIn("appSettings.signCertificatePanelCollapsed = signCertificatePanelCollapsed", QML)
        self.assertIn("signCertificatePanelExpanded: !window.signCertificatePanelCollapsed", QML)
        self.assertIn("if (!window.certificateCanSign(certificate))", QML)
        self.assertIn("&& window.selectedCertificateUsable", QML)
        self.assertIn('return "⚠ " + tr("No válido")', QML)
        self.assertIn('return tr("Caducado el %1").arg(certificateExpiry(cert))', QML)
        self.assertIn("return Number(window.certificateCanSign(b)) - Number(window.certificateCanSign(a))", QML)
        self.assertEqual(QML.count("height: Math.max(92, cardContents.implicitHeight + 24)"), 2)

    def test_qt_status_palette_reaches_aa_on_every_card_and_sidebar(self) -> None:
        themes = QML.split("property var themes:", 1)[1].split(
            "property var currentTheme:", 1)[0]
        surfaces = re.findall(r'(?:sidebarColor|cardColor): "(#[0-9a-fA-F]{6})"', themes)
        self.assertGreaterEqual(len(surfaces), 20)
        for dark, light in (("#750010", "#ffd9d5"),
                            ("#5c3900", "#ffe7a0"),
                            ("#064c2a", "#a8f5c1")):
            for surface in surfaces:
                with self.subTest(surface=surface, state=dark):
                    self.assertGreaterEqual(max(contrast(surface, dark),
                                                contrast(surface, light)), 4.5)
        self.assertEqual(QML.count("? window.certificateSelectionColor() : currentTheme.cardColor\n                                    border.color: selected\n"), 2)
        self.assertEqual(QML.count("color: window.certificateDividerColor()"), 3)
        for foreground, background in (("#17344d", "#dcecf6"),
                                       ("#ffffff", "#243c54")):
            self.assertGreaterEqual(contrast(foreground, background), 4.5)

    def test_winui_picker_cards_and_preference(self) -> None:
        xaml = (WINUI / "Views/SignPage.xaml").read_text(encoding="utf-8")
        certificates_xaml = (WINUI / "Views/CertificatesPage.xaml").read_text(encoding="utf-8")
        view = read_with_catalog(WINUI / "Views/SignPage.xaml.cs")
        cards = read_with_catalog(WINUI / "ViewModels/CertificatesPageViewModel.cs")
        signing = read_with_catalog(WINUI / "ViewModels/SignPageViewModel.cs")
        settings = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/DesktopSettingsContracts.cs").read_text(encoding="utf-8")
        self.assertIn('AutomationProperties.Name="Certificado de firma"', xaml)
        self.assertIn('AutomationProperties.Name="Ver todos los certificados"', xaml)
        self.assertIn('Content="Ocultar"', xaml)
        self.assertIn('Text="{Binding StatusReason}"', xaml)
        self.assertIn('Text="{Binding StatusReason}"', certificates_xaml)
        self.assertIn("CertificateSidebar.Visibility = !_certificatePanelExpanded", view)
        self.assertIn("SignCertificatePanelExpanded = _certificatePanelExpanded", view)
        self.assertIn('JsonPropertyName("signCertificatePanelExpanded")', settings)
        self.assertIn('CardStatusText = !suitable\n                ? "⚠ No válido"', cards)
        self.assertIn("VisibleCertificates = visible.OrderBy(item => item.CanSign ? 0 : 1)", cards)
        self.assertIn(".OrderBy(item => item.CanSign ? 0 : 1)", signing)
        self.assertIn("SelectedCertificate?.CanSign == true", signing)
        self.assertIn('Localizer.Format("No válido para firmar. {0}", SelectedCertificate.StatusReason)', signing)


if __name__ == "__main__":
    unittest.main()
