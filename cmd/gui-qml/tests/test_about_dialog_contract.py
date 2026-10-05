#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato visible y accesible del diálogo «Acerca de» de Qt/QML."""

import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
MAIN = (ROOT / "cmd/gui-qml/main.cpp").read_text(encoding="utf-8")
QRC = (ROOT / "cmd/gui-qml/qml.qrc").read_text(encoding="utf-8")
BRIDGE = (ROOT / "cmd/gui-qml/ipcbridge.cpp").read_text(encoding="utf-8")
LOCALES = (
    ROOT / "internal/adapters/outbound/common/localizador/locales"
)


class AboutDialogContractTest(unittest.TestCase):
    def test_basic_navigation_exposes_about_dialog(self) -> None:
        navigation = QML.split('text: tr("ACERCA DE")', 1)[1].split(
            "NavButton {", 1
        )[0]
        self.assertIn("onClicked: aboutDialog.open()", navigation)
        self.assertIn("active: aboutDialog.visible", navigation)
        self.assertNotIn("backend.expertMode", navigation)

    def test_dialog_displays_requested_identity_and_license(self) -> None:
        about = QML.split("id: aboutDialog", 1)[1].split(
            "id: adminLoginDialog", 1
        )[0]
        for visible_text in (
            "GrxFirma",
            "Autoría: Alberto Avidad Fernández",
            "Derechos de autor (C) 2026 Alberto Avidad Fernández.",
            "Licencia: EUPL 1.2 o posterior",
        ):
            self.assertIn(visible_text, about)
        self.assertIn(
            'source: "../assets/grxfirma-logo-carbon-256.png"',
            about,
        )
        self.assertIn("Layout.preferredWidth: 144", about)
        self.assertIn(
            "<file>assets/grxfirma-logo-carbon-256.png</file>",
            QRC,
        )

    def test_sidebar_shows_small_logo_next_to_text_name(self) -> None:
        sidebar = QML.split('text: tr("NAVEGACIÓN")', 1)[1].split(
            "// Navegación", 1
        )[0]
        self.assertIn('source: "../assets/grxfirma-logo-carbon-96.png"', sidebar)
        self.assertIn("Layout.preferredWidth: 44", sidebar)
        self.assertIn('text: tr("GrxFirma")', sidebar)
        self.assertIn('Accessible.name: tr("GrxFirma")', sidebar)
        self.assertNotIn("grxfirma-logo-horizontal", QML)
        self.assertIn(
            "<file>assets/grxfirma-logo-carbon-96.png</file>",
            QRC,
        )

    def test_version_is_shown_only_when_available(self) -> None:
        self.assertIn(
            "property string aboutApplicationVersion:",
            QML,
        )
        self.assertIn('typeof appVersion !== "undefined"', QML)
        self.assertIn("Qt.application.version", QML)
        self.assertIn(
            'visible: window.aboutApplicationVersion !== ""',
            QML,
        )
        self.assertIn(
            'text: tr("Versión %1").arg(window.aboutApplicationVersion)',
            QML,
        )
        self.assertIn(
            "app.setApplicationVersion(resolveGuiVersion(binDir));",
            MAIN,
        )
        self.assertIn(
            'setContextProperty("appVersion"',
            MAIN,
        )

    def test_dialog_and_logo_have_accessible_names(self) -> None:
        about = QML.split("id: aboutDialog", 1)[1].split(
            "id: adminLoginDialog", 1
        )[0]
        self.assertIn(
            'accessibleName: tr("Acerca de GrxFirma")',
            about,
        )
        self.assertIn("Accessible.role: Accessible.Graphic", about)
        self.assertIn(
            'Accessible.name: tr("GrxFirma")',
            about,
        )
        self.assertGreaterEqual(
            about.count("Accessible.role: Accessible.StaticText"),
            5,
        )
        self.assertIn("Comprobar actualizaciones", about)
        self.assertIn("Ver versión en GitHub", about)
        self.assertIn("requestUpdateCheck(true)", about)
        self.assertIn(
            r"releases(?:$|\/tag\/[^/?#]+$)",
            QML,
        )
        self.assertNotIn(
            r"releases(?:\/|$)",
            QML,
        )

    def test_visible_text_is_available_in_every_supported_locale(self) -> None:
        required_keys = {
            "ACERCA DE",
            "Acerca de",
            "Acerca de GrxFirma",
            "Información de versión, autoría y licencia de la aplicación",
            "GrxFirma",
            "Versión %1",
            "Autoría: Alberto Avidad Fernández",
            "Derechos de autor (C) 2026 Alberto Avidad Fernández.",
            "Licencia: EUPL 1.2 o posterior",
        }
        for locale_path in sorted(LOCALES.glob("*.json")):
            with self.subTest(locale=locale_path.stem):
                catalog = json.loads(locale_path.read_text(encoding="utf-8"))
                self.assertTrue(
                    required_keys.issubset(catalog),
                    required_keys.difference(catalog),
                )

    def test_update_notice_is_localized_in_every_supported_locale(self) -> None:
        required_keys = {
            "Todavía no se ha comprobado si existe una versión nueva.",
            "Consultando la última versión publicada en GitHub…",
            "El enlace recibido no pertenece al repositorio oficial y se ha bloqueado. Repita la comprobación o reinstale la aplicación desde GitHub.",
            "Comprobando…",
            "Comprobar actualizaciones",
            "Consulta GitHub sin descargar ni instalar archivos.",
            "Ver versión en GitHub",
            "Abre la página oficial de la versión; la aplicación no descarga ni ejecuta archivos.",
            "Nueva versión disponible",
            "Nueva versión de GrxFirma disponible",
            "GrxFirma no descargará ni instalará nada automáticamente. Revise la versión y sus notas en el repositorio oficial.",
            "Hay una versión nueva de GrxFirma: %1. Tiene instalada la %2.",
            "La última versión publicada es %1, pero este build de desarrollo no se puede comparar automáticamente.",
            "GrxFirma está actualizado (%1).",
            "Avisar de nuevas versiones",
            "Al iniciar, consulta una vez la última GitHub Release. Solo envía a GitHub la conexión HTTPS y la ruta del repositorio oficial; no envía documentos, certificados ni datos de firma.",
            "Comprobación de versiones finalizada.",
            "La comprobación segura de versiones necesita el motor local IPC. Abra GrxFirma desde el lanzador instalado y vuelva a intentarlo; la firma local no queda bloqueada.",
            "No se puede comprobar la versión porque el servicio de actualizaciones no está configurado. Reinstale GrxFirma desde el repositorio oficial.",
            "No se pudo consultar GitHub de forma segura: %s. Compruebe la conexión a Internet o el proxy y vuelva a intentarlo; la firma local sigue disponible.",
            "Todavía no hay versiones publicadas en el canal oficial.",
            "Sin versiones publicadas",
            "La consulta de versiones ha tardado demasiado. Vuelva a intentarlo más tarde.",
            "GitHub ha limitado temporalmente la consulta de versiones. Vuelva a intentarlo más tarde.",
            "El servicio de versiones de GitHub no está disponible ahora. Vuelva a intentarlo más tarde.",
            "No se pudo conectar mediante el proxy. Revise su configuración y vuelva a intentarlo.",
            "No se pudo conectar para comprobar versiones. Revise su conexión a Internet y vuelva a intentarlo.",
            "No se pudo comprobar la versión. Vuelva a intentarlo más tarde.",
        }
        for locale_path in sorted(LOCALES.glob("*.json")):
            with self.subTest(locale=locale_path.stem):
                catalog = json.loads(locale_path.read_text(encoding="utf-8"))
                self.assertTrue(
                    required_keys.issubset(catalog),
                    required_keys.difference(catalog),
                )
                for key in required_keys:
                    self.assertTrue(catalog[key].strip(), key)

    def test_no_publications_is_shown_before_version_comparison(self) -> None:
        updates = QML.split('function onUpdateCheckFinished(ok, message, result)', 1)[1]
        self.assertIn('result.estado === "sin_publicaciones"', updates)
        self.assertIn('result.mensaje || tr("Todavía no hay versiones publicadas en el canal oficial.")', updates)
        self.assertLess(updates.index('result.estado === "sin_publicaciones"'), updates.index('result.comparable === false'))

    def test_update_failure_displays_safe_ipc_message(self) -> None:
        self.assertIn('emit updateCheckFinished(false, errMsg, QVariantMap());', BRIDGE)
        self.assertIn('window.updateStatusMessage = message', QML)


if __name__ == "__main__":
    unittest.main()
