#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Validate the immutable version contract for an official release tag."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


SEMVER_TAG = re.compile(
    r"^v(0|[1-9][0-9]*)\."
    r"(0|[1-9][0-9]*)\."
    r"(0|[1-9][0-9]*)"
    r"(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)"
    r"(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$"
)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("tag")
    parser.add_argument("--version-file", type=Path, default=Path("VERSION.txt"))
    args = parser.parse_args()

    if not SEMVER_TAG.fullmatch(args.tag):
        raise SystemExit(
            "tag oficial no valido; usa SemVer estricto: "
            "vMAJOR.MINOR.PATCH o vMAJOR.MINOR.PATCH-PRERELEASE"
        )
    if not args.version_file.is_file():
        raise SystemExit(f"no existe el fichero de version: {args.version_file}")

    version = args.version_file.read_text(encoding="utf-8").strip()
    if version != args.tag.removeprefix("v"):
        raise SystemExit(
            f"VERSION.txt ({version!r}) no coincide con el tag oficial ({args.tag!r})"
        )

    print(f"Contrato tag/version valido: {args.tag}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
