#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Reutiliza las entradas GO oficiales de la base OSV local con govulncheck."""

import argparse
import hashlib
import json
import re
import zipfile
from pathlib import Path


def prepare(archive: Path, output: Path) -> dict:
    if output.exists() and any(output.iterdir()):
        raise ValueError("el destino de la base debe estar vacío")
    entries = {}
    with zipfile.ZipFile(archive) as source:
        for name in source.namelist():
            if not re.fullmatch(r"GO-\d{4}-\d+\.json", name):
                continue
            payload = source.read(name)
            entry = json.loads(payload)
            if entry["id"] + ".json" != name or name in entries:
                raise ValueError(f"identidad OSV incoherente o repetida: {name}")
            entries[name] = payload
    if not entries:
        raise ValueError("el archivo no contiene entradas GO oficiales; no se genera una base vacía")
    output.mkdir(parents=True, exist_ok=True)
    for name, payload in entries.items():
        (output / name).write_bytes(payload)
    return {"archive_sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
            "entries": len(entries), "source": "OSV Go/all.zip local; solo entradas GO, sin modificar"}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(prepare(args.archive, args.output), ensure_ascii=False, indent=2))
