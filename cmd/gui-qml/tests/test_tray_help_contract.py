#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato del submenú Ayuda de la bandeja nativa en Qt y WinUI."""

import json
from pathlib import Path
import importlib.util
import unittest


ROOT = Path(__file__).resolve().parents[3]
# Las fuentes WinUI nombran claves del catálogo; se leen con su texto español.
# Se carga por ruta para no mezclar los módulos de prueba de ambas interfaces.
_SPEC = importlib.util.spec_from_file_location(
    "winui_catalog", ROOT / "cmd/gui-winui/tests/winui_catalog.py")
_WINUI_CATALOG = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(_WINUI_CATALOG)
read_with_catalog = _WINUI_CATALOG.read_with_catalog
QT = ROOT / "cmd/gui-qml"
WIN = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


class TrayHelpContractTest(unittest.TestCase):
    def test_qt_menu_order_and_actions(self) -> None:
        source = (QT / "residentagent.cpp").read_text(encoding="utf-8")
        menu = source.split("m_menu = new QMenu();", 1)[1].split(
            "m_tray = new QSystemTrayIcon", 1
        )[0]
        ordered = (
            "m_openAction = m_menu->addAction",
            "m_statusAction = m_menu->addAction",
            "m_settingsAction = m_menu->addAction",
            "m_menu->addSeparator();",
            "m_helpMenu = m_menu->addMenu",
            "m_manualAction = m_helpMenu->addAction",
            "m_aboutAction = m_helpMenu->addAction",
            "m_menu->addSeparator();",
            "m_quitAction = m_menu->addAction",
        )
        cursor = 0
        for item in ordered:
            cursor = menu.index(item, cursor) + len(item)
        self.assertIn("&ResidentAgent::helpRequested", source)
        self.assertIn("&ResidentAgent::aboutRequested", source)

        qml = (QT / "qml/main.qml").read_text(encoding="utf-8")
        for key in ("Ayuda", "Manual de ayuda", "Acerca de GrxFirma"):
            self.assertIn(f'tr("{key}")', qml)
        self.assertIn("function onHelpRequested() {\n            backend.openHelpManual()", qml)
        self.assertIn("function onAboutRequested() {", qml)
        self.assertIn("residentAgent.showMainWindow()\n            aboutDialog.open()", qml)

    def test_winui_menu_routes_to_existing_help_and_about(self) -> None:
        tray = read_with_catalog(WIN / "Services/WindowsTrayIcon.cs")
        menu = tray.split("private void ShowMenu()", 1)[1].split(
            "public void Dispose()", 1
        )[0]
        ordered = (
            'Label("Abrir GrxFirma")',
            'Label("Firmas desde portales: activo")',
            'Label("Ajustes")',
            "AppendMenu(menu, MenuSeparator",
            'Label("Manual de ayuda")',
            'Label("Acerca de GrxFirma")',
            'Label("Ayuda")',
            "AppendMenu(menu, MenuSeparator",
            'Label("Salir")',
        )
        cursor = 0
        for item in ordered:
            cursor = menu.index(item, cursor) + len(item)
        self.assertIn("MenuPopup", menu)
        self.assertIn("case 5: _help();", menu)
        self.assertIn("case 6: _about();", menu)
        app = read_with_catalog(WIN / "App.xaml.cs")
        main = read_with_catalog(WIN / "MainWindow.xaml.cs")
        self.assertIn("_window.OpenHelpManualAsync()", app)
        self.assertIn("_window?.ShowAboutPage()", app)
        self.assertIn("page.ViewModel.OpenInstalledManualAsync()", main)
        self.assertIn("ContentFrame.Navigate(typeof(AboutPage))", main)

    def test_labels_exist_in_all_eleven_catalogs(self) -> None:
        keys = (
            "Abrir GrxFirma", "Firmas desde portales: activo", "Ajustes",
            "Ayuda", "Manual de ayuda", "Acerca de GrxFirma", "Salir",
        )
        paths = sorted(LOCALES.glob("*.json"))
        self.assertEqual(len(paths), 11)
        for path in paths:
            with self.subTest(locale=path.stem):
                catalog = json.loads(path.read_text(encoding="utf-8"))
                for key in keys:
                    self.assertTrue(catalog.get(key), key)


if __name__ == "__main__":
    unittest.main()
