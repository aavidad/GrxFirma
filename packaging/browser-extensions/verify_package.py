#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Comprueba la identidad de los paquetes que incorpora la suite."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import zipfile


ARCHIVES = ("grxfirma-extension-chromium.zip", "grxfirma-extension-firefox.xpi")
HOST = "io.github.aavidad.grxfirma"
PREFIX = "grxfirma-"
FORBIDDEN = (b"com.dipgra.", b"extension@dipgra.es", b"autofirmav2-")
FORBIDDEN_FILES = {
    "crypto_utils.js",
    "content_scripts/autologin.js",
    "content_scripts/session_sync.js",
}


def verify_archive(path: Path) -> str:
    with zipfile.ZipFile(path) as archive:
        names = set(archive.namelist())
        if names & FORBIDDEN_FILES or any(name.startswith("signer/") for name in names):
            raise ValueError(f"{path}: contiene módulos heredados excluidos")
        required = {"manifest.json", "background.js", "content_scripts/identity_bridge.js"}
        if not required <= names:
            raise ValueError(f"{path}: faltan ficheros: {sorted(required - names)}")
        manifest = json.loads(archive.read("manifest.json"))
        version = manifest.get("version")
        if version != "1.1.0":
            raise ValueError(f"{path}: versión de extensión inesperada: {version}")
        background = archive.read("background.js")
        identity = archive.read("content_scripts/identity_bridge.js")
        if HOST.encode() not in background or PREFIX.encode() not in identity:
            raise ValueError(f"{path}: host o prefijo de GrxFirma ausente")
        for name in names:
            if name.endswith((".js", ".json", ".html")):
                data = archive.read(name)
                if any(old in data for old in FORBIDDEN):
                    raise ValueError(f"{path}: identidad antigua en {name}")
    return version


def verify_directory(directory: Path) -> None:
    versions = {verify_archive(directory / name) for name in ARCHIVES}
    if len(versions) != 1:
        raise ValueError("Las extensiones tienen versiones distintas")
    metadata_path = directory / "grxfirma-extension-firefox.metadata.json"
    metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
    xpi = directory / ARCHIVES[1]
    if metadata.get("version") != versions.pop() or metadata.get("xpi_sha256") != hashlib.sha256(xpi.read_bytes()).hexdigest():
        raise ValueError("Los metadatos Firefox no corresponden al XPI")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    verify_directory(args.directory)


if __name__ == "__main__":
    main()
