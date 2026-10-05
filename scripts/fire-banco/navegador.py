#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
"""Hace de usuario del banco FIRe con Chrome del sistema (Playwright).

Abre la URL de redirección que FIRe devolvió al portal, pulsa «Firmar» en la
página de certificado local de FIRe y deja que el autoscript.js oficial de FIRe
construya y lance la URL afirma://. Como un Chrome sin interfaz no puede
entregar ese enlace al sistema, se captura con el evento CDP
Page.frameRequestedNavigation y se entrega al lanzador indicado (GrxFirma),
exactamente como haría el manejador de protocolo del sistema operativo.

Termina con 0 si FIRe redirige a la URL de éxito del portal.
"""

import argparse
import subprocess
import sys
import time
from pathlib import Path

from playwright.sync_api import sync_playwright


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True, help="URL de redirección devuelta por FIRe")
    ap.add_argument("--ok", required=True, help="Prefijo de la URL de éxito del portal")
    ap.add_argument("--error", required=True, help="Prefijo de la URL de error del portal")
    ap.add_argument("--lanzador", required=True, help="Ejecutable que recibe la URL afirma://")
    ap.add_argument("--registro", required=True, help="Fichero donde guardar la salida del lanzador")
    ap.add_argument("--uri-salida", help="Fichero donde guardar la URL afirma:// capturada")
    ap.add_argument("--captura", help="Captura de pantalla al terminar")
    ap.add_argument("--espera", type=int, default=120, help="Segundos máximos de espera")
    args = ap.parse_args()

    lanzados: list[subprocess.Popen] = []
    uris: list[str] = []
    registro = open(args.registro, "ab")

    def lanzar(uri: str) -> None:
        uris.append(uri)
        if args.uri_salida:
            Path(args.uri_salida).write_text(uri + "\n", encoding="utf-8")
        print(f"[navegador] afirma:// capturada ({len(uri)} caracteres): {uri.split('?')[0]}", flush=True)
        lanzados.append(subprocess.Popen([args.lanzador, uri], stdout=registro, stderr=registro))

    with sync_playwright() as p:
        navegador = p.chromium.launch(channel="chrome", headless=True)
        # La CA del banco es sintética y no está en el almacén de Chrome.
        contexto = navegador.new_context(ignore_https_errors=True, locale="es-ES")
        # Las URL del portal no existen: se responden aquí para detectar el final.
        for prefijo, texto in ((args.ok, "PORTAL_OK"), (args.error, "PORTAL_ERROR")):
            contexto.route(prefijo + "**", lambda ruta, t=texto: ruta.fulfill(
                status=200, content_type="text/html", body=f"<html><body>{t}</body></html>"))
        pagina = contexto.new_page()
        cdp = contexto.new_cdp_session(pagina)
        cdp.send("Page.enable")
        cdp.on("Page.frameRequestedNavigation",
               lambda e: lanzar(e["url"]) if e.get("url", "").startswith("afirma:") else None)
        pagina.on("console", lambda m: print(f"[consola] {m.text[:300]}", flush=True))

        pagina.goto(args.url)
        pagina.wait_for_selector("#buttonSign", state="visible", timeout=30_000)
        pagina.click("#buttonSign")

        limite = time.time() + args.espera
        resultado = None
        while time.time() < limite:
            url = pagina.url
            if url.startswith(args.ok):
                resultado = 0
                break
            if url.startswith(args.error):
                resultado = 1
                break
            pagina.wait_for_timeout(500)
        if args.captura:
            pagina.screenshot(path=args.captura, full_page=True)
        final = pagina.url
        navegador.close()

    for proc in lanzados:
        try:
            proc.wait(timeout=30)
        except subprocess.TimeoutExpired:
            proc.kill()
    registro.close()
    codigos = [proc.returncode for proc in lanzados]
    print(f"[navegador] lanzamientos afirma:// = {len(uris)}; códigos del lanzador = {codigos}", flush=True)
    print(f"[navegador] URL final: {final.split('?')[0]}", flush=True)
    if resultado is None:
        print("[navegador] FIRe no redirigió al portal a tiempo", file=sys.stderr)
        return 2
    return resultado


if __name__ == "__main__":
    sys.exit(main())
