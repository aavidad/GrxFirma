# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest
import xml.etree.ElementTree as ET


ROOT = pathlib.Path(__file__).resolve().parents[3]
APP = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"


class FacturaeFaceAssistantContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.window_path = APP / "MainWindow.xaml"
        cls.window = cls.window_path.read_text(encoding="utf-8")
        cls.window_code = (APP / "MainWindow.xaml.cs").read_text(
            encoding="utf-8"
        )
        cls.settings = (APP / "Views/SettingsPage.xaml").read_text(
            encoding="utf-8"
        )
        cls.settings_vm = (
            APP / "ViewModels/SettingsPageViewModel.cs"
        ).read_text(encoding="utf-8")
        cls.contracts = (
            CORE / "Operations/DesktopSettingsContracts.cs"
        ).read_text(encoding="utf-8")
        cls.generator = (
            CORE / "Facturae/FacturaeInvoiceGenerator.cs"
        ).read_text(encoding="utf-8")
        cls.page_path = APP / "Views/FacturaePage.xaml"
        cls.page = cls.page_path.read_text(encoding="utf-8")
        cls.page_code = (
            APP / "Views/FacturaePage.xaml.cs"
        ).read_text(encoding="utf-8")
        cls.page_vm = (
            APP / "ViewModels/FacturaePageViewModel.cs"
        ).read_text(encoding="utf-8")
        cls.picker_api = (
            APP / "Services/IFilePickerService.cs"
        ).read_text(encoding="utf-8")
        cls.picker = (
            APP / "Services/WindowsFilePickerService.cs"
        ).read_text(encoding="utf-8")
        cls.launcher = (
            APP / "Services/WindowsFacePortalLauncherService.cs"
        ).read_text(encoding="utf-8")
        cls.guide = (APP / "help/guia-usuario.txt").read_text(
            encoding="utf-8"
        )

    def test_optional_feature_is_hidden_and_disabled_by_default(self):
        ET.parse(self.window_path)
        ET.parse(self.page_path)
        self.assertIn('x:Name="FacturaeNavigationItem"', self.window)
        self.assertIn('Visibility="Collapsed"', self.window)
        self.assertIn(
            "safeSnapshot.FacturaeToolsEnabled ?? false",
            self.settings_vm,
        )
        self.assertIn(
            '[JsonPropertyName("facturaeToolsEnabled")]',
            self.contracts,
        )
        self.assertIn(
            "Activar herramientas Facturae y FACe",
            self.settings,
        )
        self.assertIn(
            "RefreshFacturaeToolsAvailabilityAsync",
            self.window_code,
        )

    def test_guide_explains_the_complete_submission_evidence(self):
        for required in (
            "oficina contable",
            "órgano gestor",
            "unidad tramitadora",
            "XML Facturae",
            "XML firmado",
            "número de registro",
            "CSV",
            "justificante",
            "no sube la factura",
        ):
            self.assertIn(required.casefold(), self.page.casefold())
        self.assertIn("no se considera presentada", self.guide.casefold())

    def test_optional_page_generates_facturae_322_locally(self):
        for required in (
            "Número de factura",
            "NIF del emisor",
            "NIF del receptor",
            "Oficina contable",
            "Órgano gestor",
            "Unidad tramitadora",
            "Precio unitario sin IVA",
            "Crear XML Facturae 3.2.2",
        ):
            self.assertIn(required.casefold(), self.page.casefold())
        self.assertIn(
            "CreateInvoiceAsync",
            self.page_vm,
        )
        self.assertIn(
            "FacturaeInvoiceGenerator",
            self.page_vm,
        )
        self.assertIn(
            "SaveFilePickerProfile.FacturaeXml",
            self.page_vm,
        )
        self.assertIn(
            "OnCreateInvoiceClick",
            self.page_code,
        )
        self.assertIn(
            "Los importes de la factura superan el rango de cálculo admitido.",
            self.generator,
        )
        self.assertIn(
            "catch (OverflowException)",
            self.generator,
        )
        self.assertIn(
            "FacturaeXml",
            self.picker_api,
        )
        self.assertIn(
            '[".xml", ".xsig"]',
            self.picker,
        )

    def test_launcher_uses_only_fixed_official_https_destinations(self):
        self.assertIn(
            'PortalHost = "proveedores.face.gob.es"',
            self.launcher,
        )
        for path in (
            "/proveedores/validar-factura",
            "/administraciones_y_organismos/"
            "buscador-de-organismos-y-relaciones",
            "/proveedores/remitir-factura",
            "/proveedores/consultar-facturas",
            "/proveedores/verificar-codigo-csv",
        ):
            self.assertIn(path, self.launcher)
        self.assertIn("Uri.UriSchemeHttps", self.launcher)
        self.assertIn("expectedPath", self.launcher)
        self.assertNotIn("Process.Start", self.launcher)


if __name__ == "__main__":
    unittest.main()
