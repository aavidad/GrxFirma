# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import struct
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
ICON = ROOT / "packaging" / "windows" / "grxfirma-diputacion.ico"


class ProductIconContractTests(unittest.TestCase):
    def test_canonical_icon_has_all_trazo_sizes_and_png_alpha(self) -> None:
        data = ICON.read_bytes()
        self.assertEqual(data, (ROOT / "assets/branding/grxfirma-diputacion.ico").read_bytes())
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
        self.assertIn("grxfirma-diputacion.ico", project)
        self.assertIn("logo-dipgra.png", project)
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
            self.assertIn("grxfirma-diputacion.ico", script, name)

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
            self.assertIn("grxfirma-diputacion.ico", builder, name)

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
        self.assertNotIn('Delete "$SMPROGRAMS', post_install)
        self.assertIn("$SMPROGRAMS\\Diputación de Granada", suite)
        self.assertIn('"Diputación de Granada"', winui_installer)
        self.assertIn('"Diputación de Granada"', qt_installer)


if __name__ == "__main__":
    unittest.main()
