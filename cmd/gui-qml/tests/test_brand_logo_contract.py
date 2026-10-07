#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato del logotipo de la aplicación en todas las plataformas.

El logotipo es la pluma estilográfica que escribe la rúbrica sobre la hoja
(assets/branding/grxfirma-logo-pluma*.svg). El antiguo boceto a carbón basado
en una fotografía no debe volver: ni sus ficheros ni referencias a ellos.
"""

from pathlib import Path
import struct
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[3]
BRANDING = ROOT / "assets/branding"
QT_ASSETS = ROOT / "cmd/gui-qml/assets"
ANDROID_LOGO = (
    ROOT
    / "mobile/android/app/src/main/res/drawable-nodpi/grxfirma_logo_pluma.png"
)
WEB_LOGO = ROOT / "docs/sitio/grxfirma-logo-pluma-128.png"
RETIRED_MARKERS = ("logo-carbon", "logo_carbon")


def png_size(path: Path) -> tuple[int, int]:
    data = path.read_bytes()[:24]
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise AssertionError(f"{path} no es un PNG")
    return struct.unpack(">II", data[16:24])


def tracked_files() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files", "-z"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )
    return [name for name in result.stdout.decode("utf-8").split("\0") if name]


class BrandLogoContractTest(unittest.TestCase):
    def test_vector_sources_exist_and_are_accessible(self) -> None:
        for name in ("grxfirma-logo-pluma.svg", "grxfirma-logo-pluma-pequeno.svg"):
            svg = (BRANDING / name).read_text(encoding="utf-8")
            self.assertIn('aria-label="GrxFirma"', svg)
            self.assertIn("<title>GrxFirma</title>", svg)
            self.assertNotIn("<image", svg)
            self.assertNotIn("<text", svg)

    def test_exported_pngs_have_the_expected_sizes(self) -> None:
        self.assertEqual(png_size(QT_ASSETS / "grxfirma-logo-pluma-256.png"), (256, 256))
        self.assertEqual(
            png_size(QT_ASSETS / "grxfirma-logo-pluma-pequeno-96.png"), (96, 96)
        )
        self.assertEqual(png_size(ANDROID_LOGO), (576, 576))
        self.assertEqual(png_size(WEB_LOGO), (128, 128))

    def test_every_platform_uses_the_pen_logo(self) -> None:
        qml = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
        self.assertIn('"../assets/grxfirma-logo-pluma-256.png"', qml)
        self.assertIn('"../assets/grxfirma-logo-pluma-pequeno-96.png"', qml)
        winui = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI"
        self.assertIn(
            "Assets/grxfirma-logo-pluma-pequeno-96.png",
            (winui / "MainWindow.xaml").read_text(encoding="utf-8"),
        )
        self.assertIn(
            "Assets/grxfirma-logo-pluma-256.png",
            (winui / "Views/AboutPage.xaml").read_text(encoding="utf-8"),
        )
        activity = (
            ROOT
            / "mobile/android/app/src/main/java/io/github/aavidad/grxfirma/android/MainActivity.kt"
        ).read_text(encoding="utf-8")
        self.assertIn("R.drawable.grxfirma_logo_pluma", activity)
        self.assertIn("contentDescription = getString(R.string.app_name)", activity)
        web = (ROOT / "docs/sitio/index.html").read_text(encoding="utf-8")
        self.assertIn('src="grxfirma-logo-pluma-128.png" alt="GrxFirma"', web)

    def test_charcoal_sketch_does_not_come_back(self) -> None:
        this_file = Path(__file__).resolve().relative_to(ROOT).as_posix()
        offenders = []
        for name in tracked_files():
            lowered = name.lower()
            if "carbon" in lowered:
                offenders.append(name)
                continue
            if name == this_file or name.startswith("docs/VALIDACION"):
                continue
            path = ROOT / name
            if not path.is_file():
                continue
            try:
                text = path.read_text(encoding="utf-8")
            except (UnicodeDecodeError, OSError):
                continue
            if any(marker in text for marker in RETIRED_MARKERS):
                offenders.append(name)
        self.assertEqual(offenders, [])


if __name__ == "__main__":
    unittest.main()
