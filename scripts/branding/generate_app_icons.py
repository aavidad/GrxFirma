#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Genera iconos, logotipos y el emblema del sello a partir de los SVG de marca.

Fuentes en assets/branding: grxfirma-icono.svg y grxfirma-icono-pequeno.svg
(icono del programa: «GRX» en blanco y verde sobre azul oscuro con trazo de
firma; la versión reducida se usa hasta 32 px), grxfirma-simbolo.svg,
grxfirma-logo-horizontal.svg y su variante negativo (logotipos) y
grxfirma-emblema-sello.svg (sello PAdES).
"""

from __future__ import annotations

from io import BytesIO
from pathlib import Path
import shutil
import struct
import subprocess

from PIL import Image, ImageDraw


ROOT = Path(__file__).resolve().parents[2]
BRANDING = ROOT / "assets/branding"
LARGE = BRANDING / "grxfirma-icono.svg"
SMALL = BRANDING / "grxfirma-icono-pequeno.svg"
SYMBOL = BRANDING / "grxfirma-simbolo.svg"
SEAL = BRANDING / "grxfirma-emblema-sello.svg"
HORIZONTAL = BRANDING / "grxfirma-logo-horizontal.svg"
HORIZONTAL_DARK = BRANDING / "grxfirma-logo-horizontal-negativo.svg"
# Fondo del icono del programa (también el del lanzador adaptativo Android).
BACKGROUND = "#173a4e"


def render_svg(source: Path, width: int, height: int | None = None) -> Image.Image:
    size = [f"--export-width={width}"]
    if height is not None:
        size.append(f"--export-height={height}")
    result = subprocess.run(
        ["inkscape", str(source), "--export-type=png", *size,
         "--export-filename=-"],
        check=True, capture_output=True,
    )
    with Image.open(BytesIO(result.stdout)) as image:
        return image.convert("RGBA")


def render(size: int, *, small: bool | None = None) -> Image.Image:
    source = SMALL if (size <= 32 if small is None else small) else LARGE
    return render_svg(source, size, size)


def render_round(size: int) -> Image.Image:
    scaled_size = size * 4
    icon = Image.new("RGBA", (scaled_size, scaled_size), BACKGROUND)
    # El marco redondeado ocupa 240 de 256 unidades: se amplía para que la
    # figura llegue al borde del círculo sin dejar una franja de fondo.
    framed = round(scaled_size * 256 / 240)
    margin = (framed - scaled_size) // 2
    icon.alpha_composite(render(framed).crop(
        (margin, margin, margin + scaled_size, margin + scaled_size)))
    mask = Image.new("L", icon.size, 0)
    ImageDraw.Draw(mask).ellipse((0, 0, scaled_size - 1, scaled_size - 1), fill=255)
    icon.putalpha(mask)
    return icon.resize((size, size), Image.Resampling.LANCZOS)


def save_png(path: Path, image: Image.Image) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    image.save(path, format="PNG", optimize=True)


def write_ico(path: Path, sizes: tuple[int, ...]) -> None:
    blobs = []
    for size in sizes:
        buffer = BytesIO()
        render(size).save(buffer, format="PNG", optimize=True)
        blobs.append(buffer.getvalue())
    offset = 6 + 16 * len(blobs)
    with path.open("wb") as output:
        output.write(struct.pack("<HHH", 0, 1, len(blobs)))
        for size, blob in zip(sizes, blobs, strict=True):
            output.write(struct.pack("<BBBBHHII", size % 256, size % 256,
                                     0, 0, 1, 32, len(blob), offset))
            offset += len(blob)
        for blob in blobs:
            output.write(blob)


def main() -> None:
    ico = BRANDING / "grxfirma.ico"
    write_ico(ico, (16, 20, 24, 32, 40, 48, 64, 128, 256))
    shutil.copyfile(ico, ROOT / "packaging/windows/grxfirma.ico")

    for size in (48, 128, 256):
        save_png(BRANDING / f"grxfirma-icono-{size}.png", render(size))
    shutil.copyfile(BRANDING / "grxfirma-icono-256.png",
                    ROOT / "cmd/gui-qml/assets/grxfirma-icono-256.png")

    qml_assets = ROOT / "cmd/gui-qml/assets"
    save_png(qml_assets / "grxfirma-logo-horizontal.png", render_svg(HORIZONTAL, 1400))
    save_png(qml_assets / "grxfirma-logo-horizontal-negativo.png",
             render_svg(HORIZONTAL_DARK, 1400))
    shutil.copyfile(HORIZONTAL, qml_assets / "grxfirma-logo-horizontal.svg")
    save_png(qml_assets / "grxfirma-simbolo.png", render_svg(SYMBOL, 256, 256))
    # Imagen del sello que el escritorio Qt ofrece por defecto.
    save_png(qml_assets / "logo_firma_grxfirma_final.png", render_svg(SEAL, 1024, 1024))
    save_png(ROOT / "internal/adapters/outbound/desktop/signer/recursos/"
             "grxfirma-emblema-sello.png", render_svg(SEAL, 384, 384))

    site = ROOT / "docs/sitio"
    shutil.copyfile(HORIZONTAL, site / "grxfirma-logo-horizontal.svg")
    shutil.copyfile(SMALL, site / "grxfirma-icono.svg")

    for platform in ("chromium", "firefox"):
        base = ROOT / "packaging/browser-extensions/src" / platform
        for size in (16, 32, 48, 128):
            save_png(base / f"icon{size}.png", render(size))
        for size in (16, 48, 128, 256, 512):
            save_png(base / "icons" / f"icon{size}.png", render(size))
        save_png(base / "icons/grxfirma-simbolo.png", render_svg(SYMBOL, 128, 128))

    msix = ROOT / "packaging/windows/msix/Assets"
    for name, size in (("Square44x44Logo", 44), ("StoreLogo", 50),
                       ("Square150x150Logo", 150)):
        save_png(msix / f"{name}.png", render(size))
    wide = Image.new("RGBA", (310, 150), (0, 0, 0, 0))
    wide.alpha_composite(render(150), (80, 0))
    save_png(msix / "Wide310x150Logo.png", wide)

    ios_svg = ROOT / "mobile/ios/GrxFirma/Resources/AppIcon.svg"
    source = LARGE.read_text(encoding="utf-8")
    title = "  <title>GrxFirma · Trazo</title>"
    if source.count(title) != 1:
        raise ValueError("El SVG principal no contiene el título esperado")
    ios_svg.write_text(source.replace(
        title, title + f'\n  <rect width="256" height="256" fill="{BACKGROUND}"/>', 1
    ), encoding="utf-8")
    ios = Image.new("RGB", (1024, 1024), BACKGROUND)
    icon = render(1024, small=False)
    ios.paste(icon, mask=icon.getchannel("A"))
    save_png(ROOT / "mobile/ios/GrxFirma/Resources/Assets.xcassets/"
             "AppIcon.appiconset/AppIcon-1024.png", ios)

    android = ROOT / "mobile/android/app/src/main/res"
    for density, size in (("mdpi", 48), ("hdpi", 72), ("xhdpi", 96),
                          ("xxhdpi", 144), ("xxxhdpi", 192)):
        save_png(android / f"mipmap-{density}/ic_launcher.png", render(size))
        save_png(android / f"mipmap-{density}/ic_launcher_round.png",
                 render_round(size))


if __name__ == "__main__":
    main()
