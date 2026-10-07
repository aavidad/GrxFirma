# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import hashlib
import struct
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
ICON = ROOT / "packaging" / "windows" / "grxfirma-grx.ico"
# Icono del programa: «GRX» en blanco y verde sobre azul oscuro con un trazo
# de firma. Es el mismo que llevaba la versión 0.0.118; cambiarlo es una
# decisión del responsable, no un efecto de renombrar ficheros.
GRX_ICON_SHA256 = "907036dd4946e741f9935f4b8fb476e35b093bda6d6c91aa1186a584046e1d8b"
GRX_BACKGROUND = "#173a4e"
GRX_GREEN = "#accb49"


class ProductIconContractTests(unittest.TestCase):
    def test_canonical_icon_has_all_trazo_sizes_and_png_alpha(self) -> None:
        data = ICON.read_bytes()
        self.assertEqual(data, (ROOT / "assets/branding/grxfirma-grx.ico").read_bytes())
        reserved, image_type, image_count = struct.unpack_from("<HHH", data)
        self.assertEqual((reserved, image_type, image_count), (0, 1, 9))

        dimensions: set[tuple[int, int]] = set()
        for index in range(image_count):
            width, height = struct.unpack_from("<BB", data, 6 + index * 16)
            dimensions.add((width or 256, height or 256))
            byte_count, offset = struct.unpack_from("<II", data, 6 + index * 16 + 8)
            self.assertEqual(data[offset:offset + 8], b"\x89PNG\r\n\x1a\n")
            self.assertGreater(byte_count, 100)
            self.assertEqual(data[offset + 25], 6)  # PNG RGBA
        self.assertEqual(
            dimensions,
            {
                (16, 16),
                (20, 20),
                (24, 24),
                (32, 32),
                (40, 40),
                (48, 48),
                (64, 64),
                (128, 128),
                (256, 256),
            },
        )

    def test_program_icon_is_the_grx_drawing_everywhere(self) -> None:
        self.assertEqual(hashlib.sha256(ICON.read_bytes()).hexdigest(), GRX_ICON_SHA256)

        branding = ROOT / "assets/branding"
        large = (branding / "grxfirma-icono.svg").read_text(encoding="utf-8")
        small = (branding / "grxfirma-icono-pequeno.svg").read_text(encoding="utf-8")
        self.assertIn("<title>GrxFirma · Trazo</title>", large)
        self.assertIn("<title>GrxFirma · Trazo reducido</title>", small)
        for source in (large, small):
            self.assertIn(GRX_BACKGROUND, source)
            self.assertIn(GRX_GREEN, source)
        self.assertEqual(
            (ROOT / "docs/sitio/grxfirma-icono.svg").read_text(encoding="utf-8"),
            small,
        )
        ios = (ROOT / "mobile/ios/GrxFirma/Resources/AppIcon.svg").read_text(
            encoding="utf-8"
        )
        self.assertIn("<title>GrxFirma · Trazo</title>", ios)

        android = ROOT / "mobile/android/app/src/main/res"
        foreground = (android / "drawable/ic_launcher_foreground.xml").read_text(
            encoding="utf-8"
        )
        self.assertIn(GRX_BACKGROUND.upper(), foreground)
        self.assertIn(GRX_GREEN.upper(), foreground)
        colors = (android / "values/colors.xml").read_text(encoding="utf-8")
        self.assertIn(
            f'<color name="grxfirma_icon_background">{GRX_BACKGROUND.upper()}</color>',
            colors,
        )

        generator = (ROOT / "scripts/branding/generate_app_icons.py").read_text(
            encoding="utf-8"
        )
        self.assertIn(f'BACKGROUND = "{GRX_BACKGROUND}"', generator)

    def test_winui_embeds_and_displays_the_brand(self) -> None:
        project = (
            ROOT
            / "cmd/gui-winui/src/GrxFirma.WinUI/GrxFirma.WinUI.csproj"
        ).read_text(encoding="utf-8")
        window = (
            ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/MainWindow.xaml"
        ).read_text(encoding="utf-8")
        installer = (
            ROOT / "packaging/windows/install-desktop-winui.ps1"
        ).read_text(encoding="utf-8")

        self.assertIn("<ApplicationIcon>", project)
        self.assertIn("grxfirma-grx.ico", project)
        self.assertIn("grxfirma-logo-pluma-pequeno-96.png", project)
        self.assertIn("grxfirma-logo-pluma-pequeno-96.png", window)
        self.assertIn('AutomationProperties.Name="GrxFirma"', window)
        self.assertIn('Text="GrxFirma"', window)
        self.assertIn("$shortcut.IconLocation", installer)

    def test_qt_installer_and_nsis_use_the_brand(self) -> None:
        qt_installer = (
            ROOT / "packaging/windows/install-desktop-qml.ps1"
        ).read_text(encoding="utf-8")

        self.assertIn("$shortcut.IconLocation", qt_installer)

        for name in (
            "grxfirma-suite.nsi",
            "grxfirma-cli.nsi",
            "grxfirma-afirmauri.nsi",
            "grxfirma-desktop-qml.nsi",
        ):
            script = (ROOT / "packaging/windows" / name).read_text(
                encoding="utf-8"
            )
            self.assertIn("MUI_ICON", script, name)
            self.assertIn("MUI_UNICON", script, name)
            self.assertIn("grxfirma-grx.ico", script, name)

        for name in (
            "build-suite.ps1",
            "build-suite.sh",
            "build-cli.ps1",
            "build-cli.sh",
            "build-afirmauri.ps1",
            "build-afirmauri.sh",
            "build-desktop-qml.ps1",
            "build-desktop-qml.sh",
        ):
            builder = (ROOT / "packaging/windows" / name).read_text(
                encoding="utf-8"
            )
            self.assertIn("grxfirma-grx.ico", builder, name)

    def test_suite_offers_desktop_shortcut_and_groups_start_apps(self) -> None:
        suite = (
            ROOT / "packaging/windows/grxfirma-suite.nsi"
        ).read_text(encoding="utf-8")
        winui_installer = (
            ROOT / "packaging/windows/install-desktop-winui.ps1"
        ).read_text(encoding="utf-8")
        qt_installer = (
            ROOT / "packaging/windows/install-desktop-qml.ps1"
        ).read_text(encoding="utf-8")

        self.assertIn(
            'Section "Crear acceso directo en el escritorio"',
            suite,
        )
        self.assertIn("$DESKTOP\\GrxFirma.lnk", suite)
        post_install = suite.split("Section -post", 1)[1].split('Section "Uninstall"', 1)[0]
        self.assertIn('"DisplayName" "GrxFirma"', suite)
        # La instalación no debe borrar los accesos que acaba de crear (el
        # renombrado convirtió una limpieza de nombres antiguos en eso).
        self.assertNotIn('Delete "$DESKTOP', post_install)
        self.assertNotIn('Delete "$SMPROGRAMS\\GrxFirma\\', post_install)
        self.assertIn('CreateShortcut "$SMPROGRAMS\\GrxFirma\\', post_install)
        self.assertIn('$startMenuDir = Join-Path $programsDir "GrxFirma"', winui_installer)
        self.assertIn('$startMenuDir = Join-Path $programsDir "GrxFirma"', qt_installer)
        # Una actualización retira solo los accesos propios de la carpeta que
        # usaban las versiones anteriores y la elimina únicamente si queda vacía.
        legacy = "$SMPROGRAMS\\Diputación de Granada"
        self.assertIn(f'Delete "{legacy}\\GrxFirma - Documentación.lnk"', post_install)
        self.assertIn(f'Delete "{legacy}\\Desinstalar GrxFirma.lnk"', post_install)
        self.assertIn(f'RMDir "{legacy}"', post_install)
        self.assertNotIn(f'RMDir /r "{legacy}', suite)
        for installer in (winui_installer, qt_installer):
            self.assertIn(
                '$legacyStartMenuDir = Join-Path $programsDir "Diputación de Granada"',
                installer,
            )
            self.assertIn("$legacyRemaining.Count -eq 0", installer)


    def test_icon_name_changes_with_the_image_and_shell_cache_is_refreshed(self) -> None:
        # De la 0.0.119 a la 0.0.121 el ICO se llamaba grxfirma.ico con otra
        # imagen; Windows cachea por ruta y seguiría mostrándola.
        windows = ROOT / "packaging/windows"
        self.assertFalse((windows / "grxfirma.ico").exists())
        self.assertFalse((ROOT / "assets/branding/grxfirma.ico").exists())
        refresh = (windows / "shell-icon-refresh.nsh").read_bytes()
        self.assertTrue(refresh.startswith(b"\xef\xbb\xbf"))
        refresh_text = refresh.decode("utf-8-sig")
        self.assertIn("ie4uinit.exe\" -show", refresh_text)
        self.assertIn("$WINDIR\\Sysnative\\ie4uinit.exe", refresh_text)
        self.assertIn("SHChangeNotify(i 0x08000000", refresh_text)
        for name in (
            "grxfirma-suite.nsi",
            "grxfirma-cli.nsi",
            "grxfirma-afirmauri.nsi",
            "grxfirma-desktop-qml.nsi",
        ):
            script = (windows / name).read_text(encoding="utf-8-sig")
            self.assertIn('!include "shell-icon-refresh.nsh"', script, name)
            post = script.split("Section -post", 1)[1].split("SectionEnd", 1)[0]
            self.assertIn("!insertmacro GrxFirmaRefreshShellIcons", post, name)
            self.assertNotIn('"DisplayIcon" "$INSTDIR\\grxfirma.ico"', script, name)
        for name in ("grxfirma-suite.nsi", "grxfirma-cli.nsi", "grxfirma-afirmauri.nsi"):
            script = (windows / name).read_text(encoding="utf-8-sig")
            install, uninstall = script.split('Section "Uninstall"', 1)
            self.assertIn('Delete "$INSTDIR\\grxfirma.ico"', install, name)
            self.assertIn('Delete "$INSTDIR\\grxfirma.ico"', uninstall, name)
        registration = (windows / "afirmauri-registration.ps1").read_text(
            encoding="utf-8-sig"
        )
        legacy = registration.split("$script:AfirmaLegacyIconFileNames = @(", 1)[1]
        legacy = legacy.split(")", 1)[0]
        self.assertIn('"grxfirma-diputacion.ico"', legacy)
        self.assertIn('"grxfirma.ico"', legacy)
        installer = (windows / "install-afirmauri.ps1").read_text(encoding="utf-8-sig")
        self.assertIn("$script:AfirmaLegacyIconFileNames", installer)

    def test_hooded_silhouette_is_not_shipped(self) -> None:
        for path in (
            "assets/branding/grxfirma-simbolo.svg",
            "assets/branding/grxfirma-logo-horizontal.svg",
            "assets/branding/grxfirma-logo-horizontal-negativo.svg",
            "assets/branding/grxfirma-emblema-sello.svg",
            "cmd/gui-qml/assets/grxfirma-simbolo.png",
            "cmd/gui-qml/assets/grxfirma-logo-horizontal.png",
            "cmd/gui-qml/assets/grxfirma-logo-horizontal.svg",
            "cmd/gui-qml/assets/grxfirma-logo-horizontal-negativo.png",
            "docs/sitio/grxfirma-logo-horizontal.svg",
            "packaging/browser-extensions/src/chromium/icons/grxfirma-simbolo.png",
            "packaging/browser-extensions/src/firefox/icons/grxfirma-simbolo.png",
        ):
            self.assertFalse((ROOT / path).exists(), path)
        for platform in ("chromium", "firefox"):
            base = ROOT / "packaging/browser-extensions/src" / platform
            for page in ("popup.html", "signer/signer.html", "signer/popup_old.html"):
                html = (base / page).read_text(encoding="utf-8")
                self.assertNotIn("grxfirma-simbolo", html, f"{platform}/{page}")
                self.assertIn("icons/icon128.png", html, f"{platform}/{page}")

if __name__ == "__main__":
    unittest.main()
