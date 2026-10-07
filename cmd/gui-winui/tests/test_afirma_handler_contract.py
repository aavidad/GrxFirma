# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Selector «Firmas desde los portales» (GrxFirma o AutoFirma para afirma://).

La interfaz solo muestra el estado real y pide el cambio al motor; nunca
toca el registro ni ejecuta PowerShell. WinUI y Qt usan las mismas claves.
"""

import json
import pathlib
import re
import unittest
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[3]
UI = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
CORE = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core"
QML = ROOT / "cmd/gui-qml/qml/main.qml"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"
CONTROLS = "{using:GrxFirma.WinUI.Controls}"
XNAME = "{http://schemas.microsoft.com/winfx/2006/xaml}Name"
PRESENTATION = "http://schemas.microsoft.com/winfx/2006/xaml/presentation"

KEYS = [
    "protocolo.titulo", "protocolo.descripcion", "protocolo.selector",
    "protocolo.opcion.grxfirma", "protocolo.opcion.autofirma", "protocolo.comprobar",
    "protocolo.estado.comprobando", "protocolo.estado.cambiando",
    "protocolo.estado.grxfirma", "protocolo.estado.autofirma", "protocolo.estado.otro",
    "protocolo.estado.ninguno", "protocolo.estado.error", "protocolo.autofirma_no_instalada",
    "protocolo.recargar_portal", "protocolo.error.ajeno",
    "protocolo.error.grxfirma_no_instalada", "protocolo.error.no_disponible",
    "protocolo.error.generico", "ayuda.protocolo", "ayuda.protocolo.grxfirma",
    "ayuda.protocolo.autofirma", "ayuda.protocolo.mas",
]


class AfirmaHandlerContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.page = (UI / "Views/SettingsPage.AfirmaHandler.cs").read_text(encoding="utf-8")
        cls.contracts = (CORE / "Operations/AfirmaHandlerContracts.cs").read_text(encoding="utf-8")
        cls.client = (CORE / "Operations/DesktopOperationsClient.cs").read_text(encoding="utf-8")
        cls.xaml = ET.parse(UI / "Views/SettingsPage.xaml").getroot()
        cls.qml = QML.read_text(encoding="utf-8")

    def test_every_key_exists_in_all_catalogs_with_placeholders(self):
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key in KEYS:
                with self.subTest(language=path.stem, key=key):
                    self.assertTrue(catalog.get(key, "").strip())
            self.assertIn("{0}", catalog["protocolo.estado.otro"], path.stem)

    def test_card_has_both_options_and_help_with_every_option(self):
        by_name = {e.get(XNAME): e for e in self.xaml.iter() if e.get(XNAME)}
        self.assertEqual(by_name["AfirmaHandlerCard"].get("Visibility"), "Collapsed")
        radios = [by_name["AfirmaGrxFirmaRadio"], by_name["AfirmaAutoFirmaRadio"]]
        for radio in radios:
            self.assertEqual(radio.tag, "{%s}RadioButton" % PRESENTATION)
            self.assertEqual(radio.get("GroupName"), "AfirmaHandler")
            self.assertEqual(radio.get("Checked"), "OnAfirmaHandlerChecked")
        help_button = by_name["AfirmaHandlerHelp"]
        self.assertEqual(help_button.tag, CONTROLS + "HelpButton")
        self.assertEqual(help_button.get("HelpKey"), "ayuda.protocolo")
        self.assertEqual(
            help_button.get("Options"),
            "protocolo.opcion.grxfirma=ayuda.protocolo.grxfirma "
            "protocolo.opcion.autofirma=ayuda.protocolo.autofirma")

    def test_interface_never_touches_the_registry_or_powershell(self):
        for text in (self.page, self.contracts):
            self.assertNotIn("Microsoft.Win32", text)
            self.assertNotIn("Registry", text)
            self.assertNotRegex(text.lower(), r"powershell|\.ps1")
        self.assertIn("SelectAfirmaHandlerAsync", self.page)
        self.assertIn("GetAfirmaHandlerStatusAsync", self.page)

    def test_client_only_sends_the_two_choices(self):
        self.assertIn('AfirmaHandlerStatus = "afirma_handler_status"', self.client)
        self.assertIn('AfirmaHandlerSelect = "afirma_handler_select"', self.client)
        self.assertIn("AfirmaHandlerChoices.IsSelectable(handler)", self.client)
        self.assertIn("handler is GrxFirma or AutoFirma", self.contracts)

    def test_status_comes_from_the_engine_and_failures_show_the_real_state(self):
        # Tras un fallo se vuelve a leer el estado real.
        self.assertRegex(self.page, r"(?s)failureKey = AfirmaHandlerPresentation\.ErrorKey.*await RefreshAfirmaHandlerAsync\(\)")
        # La opción AutoFirma queda desactivada si no está instalada.
        self.assertIn("CanChooseAutoFirma: status.AutoFirmaInstalled", self.contracts)
        self.assertIn("ShowAutoFirmaMissing: !status.AutoFirmaInstalled", self.contracts)

    def test_qt_uses_the_same_keys_and_engine_actions(self):
        for key in KEYS:
            with self.subTest(key=key):
                self.assertIn('tr("%s")' % key, self.qml)
        self.assertIn("backend.getAfirmaHandlerStatus()", self.qml)
        self.assertIn("backend.selectAfirmaHandler(handler)", self.qml)
        self.assertIn('Qt.platform.os === "windows" && isIpcMode', self.qml)
        status = self.qml[self.qml.index('objectName: "afirmaHandlerStatusText"'):][:400]
        self.assertIn("textFormat: Text.PlainText", status)
        ipc = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
        self.assertIn('handler != QStringLiteral("grxfirma")', ipc)
        self.assertNotRegex(ipc.lower(), r"powershell.*afirma")


if __name__ == "__main__":
    unittest.main()
