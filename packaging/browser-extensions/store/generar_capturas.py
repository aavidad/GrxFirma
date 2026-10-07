#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Genera las capturas y gráficos de las fichas de tienda de la extensión.

Carga el ZIP Chromium recién generado en un perfil temporal de Chrome for
Testing (Chrome estable de marca ya no admite ``--load-extension``), conecta
el host de mensajería nativa real compilado desde ``cmd/nativehost`` y sirve
un PDF ficticio mediante interceptación de Playwright: no se contacta con
ningún portal real. Los textos del PDF y del mosaico están en
``capturas.json``.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import zipfile

STORE_DIR = Path(__file__).resolve().parent
EXT_ROOT = STORE_DIR.parent
REPO_ROOT = EXT_ROOT.parents[1]
HOST_NAME = "io.github.aavidad.grxfirma"
VIEWPORT = {"width": 1280, "height": 800}
BRAND_ICON = REPO_ROOT / "assets" / "branding" / "grxfirma-icono-256.png"

sys.path.insert(0, str(EXT_ROOT))
import build  # noqa: E402  pylint: disable=wrong-import-position


def pdf_bytes(lines: list[str]) -> bytes:
    """PDF mínimo de una página con Helvetica y codificación WinAnsi."""

    def escape(text: str) -> str:
        return text.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")

    stream_lines = ["BT", "/F1 22 Tf", "72 760 Td", "28 TL"]
    for index, line in enumerate(lines):
        if index == 1:
            stream_lines.append("/F1 13 Tf")
        stream_lines.append(f"({escape(line)}) Tj T*")
    stream_lines.append("ET")
    stream = "\n".join(stream_lines).encode("cp1252")
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] "
        b"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
        b"<< /Length %d >>\nstream\n" % len(stream) + stream + b"\nendstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
    ]
    out = bytearray(b"%PDF-1.4\n")
    offsets = []
    for number, body in enumerate(objects, start=1):
        offsets.append(len(out))
        out += b"%d 0 obj\n" % number + body + b"\nendobj\n"
    xref = len(out)
    out += b"xref\n0 %d\n0000000000 65535 f \n" % (len(objects) + 1)
    for offset in offsets:
        out += b"%010d 00000 n \n" % offset
    out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objects) + 1, xref)
    return bytes(out)


def data_uri(path_or_bytes, mime: str = "image/png") -> str:
    raw = path_or_bytes if isinstance(path_or_bytes, bytes) else Path(path_or_bytes).read_bytes()
    return f"data:{mime};base64,{base64.b64encode(raw).decode('ascii')}"


def composite_html(background: Path, popup: Path) -> str:
    return f"""<!doctype html><html><head><meta charset="utf-8"><style>
html,body{{margin:0;width:1280px;height:800px;overflow:hidden}}
.bg{{position:absolute;inset:0;width:1280px;height:800px}}
.shade{{position:absolute;inset:0;background:rgba(15,23,42,.18)}}
.popup{{position:absolute;top:12px;right:24px;border-radius:14px;box-shadow:0 18px 48px rgba(15,23,42,.35)}}
</style></head><body><img class="bg" src="{data_uri(background)}"><div class="shade"></div>
<img class="popup" src="{data_uri(popup)}"></body></html>"""


def promo_html(icon: Path, strings: dict) -> str:
    return f"""<!doctype html><html><head><meta charset="utf-8"><style>
html,body{{margin:0;width:440px;height:280px;overflow:hidden}}
body{{display:flex;align-items:center;gap:22px;padding:0 34px;box-sizing:border-box;
background:linear-gradient(135deg,#ffffff 0%,#e3eef6 100%);color:#0f2f4a;font-family:system-ui,sans-serif}}
img{{width:112px;height:112px;flex:none}}
h1{{margin:0 0 8px;font-size:38px;line-height:1}}
p{{margin:0;font-size:19px;line-height:1.3;color:#36556e}}
</style></head><body><img src="{data_uri(icon)}"><div><h1>{strings['promo_title']}</h1>
<p>{strings['promo_subtitle']}</p></div></body></html>"""


def store_icon(source: Path, target: Path) -> None:
    """Icono de tienda de 128 px con 96 px de dibujo y margen transparente."""
    from PIL import Image

    target.parent.mkdir(parents=True, exist_ok=True)
    with Image.open(source) as image:
        artwork = image.convert("RGBA").resize((96, 96), Image.Resampling.LANCZOS)
    canvas = Image.new("RGBA", (128, 128), (0, 0, 0, 0))
    canvas.paste(artwork, (16, 16), artwork)
    canvas.save(target, optimize=True)


def build_host(workdir: Path) -> Path:
    host = workdir / "host" / "grxfirma-nativehost"
    host.parent.mkdir(parents=True)
    env = dict(os.environ)
    env.setdefault("GOCACHE", str(workdir / "gocache"))
    subprocess.run(
        ["go", "build", "-trimpath", "-tags=production", "-o", str(host), "./cmd/nativehost"],
        cwd=REPO_ROOT, env=env, check=True,
    )
    return host


def write_host_manifests(paths: list[Path], host: Path, extension_id: str) -> None:
    payload = {
        "name": HOST_NAME,
        "description": "GrxFirma Native Messaging Host (capturas)",
        "path": str(host),
        "type": "stdio",
        "allowed_origins": [f"chrome-extension://{extension_id}/"],
    }
    for path in paths:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


def wait_for_worker(context, timeout: float = 15.0):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        for worker in context.service_workers:
            if worker.url.startswith("chrome-extension://") and worker.url.endswith("/background.js"):
                return worker
        time.sleep(0.2)
    raise RuntimeError("no arrancó el service worker: el navegador no cargó la extensión")


def capture_language(playwright, chrome: Path, host: Path, workdir: Path, out_dir: Path,
                     config: dict, lang: str) -> None:
    strings = config["languages"][lang]
    origin = config["test_origin"].rstrip("/")
    lang_dir = workdir / lang
    extension = lang_dir / "extension"
    profile = lang_dir / "profile"
    xdg = lang_dir / "xdg"
    package = lang_dir / "grxfirma-extension-chromium.zip"
    build.build_archive(build.SRC_DIR / "chromium", package)
    with zipfile.ZipFile(package) as archive:
        archive.extractall(extension)

    env = dict(os.environ, XDG_CONFIG_HOME=str(xdg), LANGUAGE=strings["browser_locale"].replace("-", "_"))
    context = playwright.chromium.launch_persistent_context(
        str(profile),
        executable_path=str(chrome),
        headless=True,
        env=env,
        locale=strings["browser_locale"],
        viewport=VIEWPORT,
        device_scale_factor=1,
        args=[
            f"--disable-extensions-except={extension}",
            f"--load-extension={extension}",
            f"--lang={strings['browser_locale']}",
            "--no-first-run",
            "--no-default-browser-check",
        ],
    )
    try:
        worker = wait_for_worker(context)
        extension_id = worker.url.split("/")[2]
        write_host_manifests([
            profile / "NativeMessagingHosts" / f"{HOST_NAME}.json",
            xdg / "google-chrome-for-testing" / "NativeMessagingHosts" / f"{HOST_NAME}.json",
            host.parent / "manifests" / f"{HOST_NAME}.json",
        ], host, extension_id)

        pdf = pdf_bytes(strings["pdf_lines"])
        pdf_url = f"{origin}/docs/{strings['pdf_name']}"

        def serve(route):
            if route.request.url == pdf_url:
                route.fulfill(status=200, body=pdf, headers={"Content-Type": "application/pdf"})
            else:
                route.fulfill(status=404, body="")

        context.route(f"{origin}/**", serve)
        lang_out = out_dir / "capturas" / lang
        lang_out.mkdir(parents=True, exist_ok=True)

        page = context.pages[0] if context.pages else context.new_page()
        page.goto(pdf_url)
        page.locator("#grxfirma-sign-trigger").wait_for(timeout=15000)
        page.wait_for_timeout(2500)
        page.mouse.move(5, 5)
        signing = lang_out / "01-firmar-pdf.png"
        page.screenshot(path=str(signing))

        popup = context.new_page()
        popup.set_viewport_size({"width": 416, "height": 600})
        popup.goto(f"chrome-extension://{extension_id}/popup.html")
        popup.locator("#status.ok").wait_for(timeout=15000)
        popup_png = lang_dir / "popup.png"
        popup.locator("main.shell").screenshot(path=str(popup_png))
        popup.close()

        composite = context.new_page()
        composite.set_content(composite_html(signing, popup_png))
        composite.wait_for_timeout(300)
        composite.screenshot(path=str(lang_out / "02-ventana-extension.png"))

        worker.evaluate(
            "site => chrome.storage.local.set({ sitiosConfianza: [site] })", config["user_site"]
        )
        composite.goto(f"chrome-extension://{extension_id}/options.html")
        composite.locator("#siteList li").nth(2).wait_for(timeout=10000)
        composite.screenshot(path=str(lang_out / "03-sitios-de-confianza.png"))

        composite.set_viewport_size({"width": 440, "height": 280})
        composite.set_content(promo_html(BRAND_ICON, strings))
        composite.wait_for_timeout(300)
        graphics = out_dir / "graficos"
        graphics.mkdir(parents=True, exist_ok=True)
        composite.screenshot(path=str(graphics / f"mosaico-440x280-{lang}.png"))
    finally:
        context.close()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--chrome", type=Path, default=os.environ.get("GRXFIRMA_CFT_CHROME"),
                        help="Ejecutable de Chrome for Testing")
    parser.add_argument("--host", type=Path, help="Host nativo ya compilado con -tags=production")
    parser.add_argument("--output-dir", type=Path, default=STORE_DIR)
    parser.add_argument("--lang", action="append", choices=["es", "en"])
    args = parser.parse_args(argv)
    if not args.chrome or not Path(args.chrome).is_file():
        parser.error("indique --chrome o GRXFIRMA_CFT_CHROME con Chrome for Testing")

    from playwright.sync_api import sync_playwright

    config = json.loads((STORE_DIR / "capturas.json").read_text(encoding="utf-8"))
    output = args.output_dir.resolve()
    store_icon(BRAND_ICON, output / "graficos" / "icono-128.png")
    with tempfile.TemporaryDirectory(prefix="grxfirma-capturas-") as tmp:
        workdir = Path(tmp)
        if args.host:
            host = workdir / "host" / "grxfirma-nativehost"
            host.parent.mkdir(parents=True)
            shutil.copy2(args.host, host)
        else:
            host = build_host(workdir)
        with sync_playwright() as playwright:
            for lang in args.lang or ["es", "en"]:
                capture_language(playwright, Path(args.chrome), host, workdir, output, config, lang)
                print(f"Capturas generadas ({lang}): {output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
