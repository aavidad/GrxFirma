#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Genera los iconos de aplicación a partir de los dos SVG Trazo."""

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


def render(size: int, *, small: bool | None = None) -> Image.Image:
    source = SMALL if (size <= 32 if small is None else small) else LARGE
    result = subprocess.run(
        ["inkscape", str(source), "--export-type=png", f"--export-width={size}",
         f"--export-height={size}", "--export-filename=-"],
        check=True, capture_output=True,
    )
    with Image.open(BytesIO(result.stdout)) as image:
        return image.convert("RGBA")


def render_round(size: int) -> Image.Image:
    scaled_size = size * 4
    icon = Image.new("RGBA", (scaled_size, scaled_size), "#173a4e")
    icon.alpha_composite(render(scaled_size))
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
    ico = BRANDING / "grxfirma-diputacion.ico"
    write_ico(ico, (16, 20, 24, 32, 40, 48, 64, 128, 256))
    shutil.copyfile(ico, ROOT / "packaging/windows/grxfirma-diputacion.ico")

    for size in (48, 128, 256):
        save_png(BRANDING / f"grxfirma-icono-{size}.png", render(size))
    shutil.copyfile(BRANDING / "grxfirma-icono-256.png",
                    ROOT / "cmd/gui-qml/assets/grxfirma-icono-256.png")

    for platform in ("chromium", "firefox"):
        base = ROOT / "packaging/browser-extensions/src" / platform
        for size in (16, 32, 48, 128):
            save_png(base / f"icon{size}.png", render(size))
        for size in (16, 48, 128):
            save_png(base / "icons" / f"icon{size}.png", render(size))

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
        title, title + '\n  <rect width="256" height="256" fill="#173a4e"/>', 1
    ), encoding="utf-8")
    ios = Image.new("RGB", (1024, 1024), "#173a4e")
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
