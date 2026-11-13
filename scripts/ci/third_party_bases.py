#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Read the upstream base of every copy under third_party/ (G-08).

Each third_party/<dir>/README.md must declare one line
``Base: <module>@<version>``. With --write-gomod the bases are written to a
synthetic go.mod (without replace directives) so OSV-Scanner can match the
upstream versions against published advisories.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
BASE_RE = re.compile(r"^Base:\s+(?P<module>[A-Za-z0-9./_-]+)@(?P<version>v\S+)\s*$")


def read_bases(root: Path) -> tuple[list[tuple[str, str]], list[str]]:
    bases: list[tuple[str, str]] = []
    errors: list[str] = []
    third_party = root / "third_party"
    for directory in sorted(path for path in third_party.iterdir() if path.is_dir()):
        readme = directory / "README.md"
        if not readme.is_file():
            errors.append(f"{directory.relative_to(root)}: falta README.md")
            continue
        found = [
            match
            for line in readme.read_text(encoding="utf-8").splitlines()
            if (match := BASE_RE.match(line))
        ]
        if len(found) != 1:
            errors.append(
                f"{readme.relative_to(root)}: debe tener exactamente una línea "
                "'Base: <módulo>@<versión>'"
            )
            continue
        bases.append((found[0].group("module"), found[0].group("version")))
    return bases, errors


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--write-gomod", type=Path)
    args = parser.parse_args(argv)

    bases, errors = read_bases(args.root)
    if errors:
        for error in errors:
            print(f"error: {error}", file=sys.stderr)
        return 1
    if args.write_gomod:
        args.write_gomod.parent.mkdir(parents=True, exist_ok=True)
        lines = ["module grxfirma.invalid/third-party-bases", "", "go 1.22", "", "require ("]
        lines += [f"\t{module} {version}" for module, version in bases]
        lines += [")", ""]
        args.write_gomod.write_text("\n".join(lines), encoding="utf-8")
    for module, version in bases:
        print(f"{module} {version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
