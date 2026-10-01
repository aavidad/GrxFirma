# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Comprueba que la versión que se va a compilar figura en las novedades."""

from pathlib import Path
import re
import sys


def comprobar(raiz: Path) -> None:
    version_path = raiz / "VERSION.txt"
    novedades_path = raiz / "docs/NOVEDADES.md"
    if version_path.is_symlink() or novedades_path.is_symlink():
        raise ValueError("VERSION.txt y docs/NOVEDADES.md deben ser archivos normales")
    version = version_path.read_text(encoding="utf-8").strip()
    match = re.fullmatch(
        r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
        r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
        r"(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?",
        version,
    )
    if match is None or any(
        identifier.isdecimal() and len(identifier) > 1 and identifier.startswith("0")
        for identifier in (match.group(4) or "").split(".")
    ):
        raise ValueError("VERSION.txt debe contener una versión SemVer válida")
    if novedades_path.stat().st_size > 65536:
        raise ValueError("docs/NOVEDADES.md supera el límite de 64 KiB de la ayuda")
    novedades = novedades_path.read_text(encoding="utf-8")
    patron = re.compile(rf"^## {re.escape(version)} — \d{{4}}-\d{{2}}-\d{{2}}\s*$", re.MULTILINE)
    if len(patron.findall(novedades)) != 1:
        raise ValueError(f"docs/NOVEDADES.md debe tener una sección única '## {version} — AAAA-MM-DD'")


if __name__ == "__main__":
    try:
        comprobar(Path(__file__).resolve().parents[1])
    except (OSError, UnicodeError, ValueError) as error:
        print(f"error de versión: {error}", file=sys.stderr)
        sys.exit(1)
