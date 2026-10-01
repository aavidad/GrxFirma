#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Reject in-process runtime plugin and interpreter dependencies.

GrxFirma deliberately integrates reviewed capabilities at build time.  This
gate prevents accidentally reintroducing user-supplied executable code through
Go plugins, RPC plugin frameworks or embedded interpreters.
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]

SKIPPED_DIRECTORIES = {
    ".git",
    ".idea",
    ".vscode",
    "__pycache__",
    "build",
    "dist",
    "node_modules",
    "release",
}

FORBIDDEN_IMPORT_PREFIXES = {
    "github.com/aarzilli/golua": "intérprete Lua embebido",
    "github.com/arnodel/golua": "intérprete Lua embebido",
    "github.com/glycerine/golua": "intérprete Lua embebido",
    "github.com/hashicorp/go-plugin": "framework de plugins RPC",
    "github.com/mattn/go-lua": "intérprete Lua embebido",
    "github.com/shopify/go-lua": "intérprete Lua embebido",
    "github.com/tetratelabs/wazero": "runtime WebAssembly embebido",
    "github.com/traefik/yaegi": "intérprete Go embebido",
    "github.com/yuin/gopher-lua": "intérprete Lua embebido",
}
LUA_IMPORT_SEGMENTS = {"go-lua", "golua", "gopher-lua", "lua"}

GO_SINGLE_IMPORT = re.compile(
    r'^\s*import\s+(?:(?:[._A-Za-z]\w*)\s+)?["`]([^"`]+)["`]'
)
GO_IMPORT_BLOCK_START = re.compile(r"^\s*import\s*\(")
GO_BLOCK_IMPORT = re.compile(
    r'^\s*(?:(?:[._A-Za-z]\w*)\s+)?["`]([^"`]+)["`]'
)
NATIVE_LUA_PATTERNS = (
    (re.compile(r"(?i)\bfind_package\s*\(\s*lua(?:\s|\)|[0-9.])"), "CMake carga Lua"),
    (
        re.compile(
            r"(?i)\bpkg_(?:check_modules|search_module)\s*\([^)]*\blua(?:[0-9.-]*)\b"
        ),
        "pkg-config carga Lua",
    ),
    (
        re.compile(r"(?i)(?:^|\s)-llua(?:[0-9.-]*)?(?=$|[\s;)])"),
        "enlace nativo con Lua",
    ),
)
NATIVE_BUILD_NAMES = {"CMakeLists.txt", "Makefile"}
NATIVE_BUILD_SUFFIXES = {".cmake", ".mk", ".pri", ".pro"}
GO_MODULE_NAMES = {"go.mod", "go.work"}


@dataclass(frozen=True)
class Finding:
    path: Path
    line: int
    reason: str

    def render(self, root: Path) -> str:
        try:
            display_path = self.path.relative_to(root)
        except ValueError:
            display_path = self.path
        return f"{display_path}:{self.line}: {self.reason}"


def _is_skipped(path: Path, root: Path) -> bool:
    try:
        relative = path.relative_to(root)
    except ValueError:
        return True
    return any(part in SKIPPED_DIRECTORIES for part in relative.parts[:-1])


def _candidate_files(root: Path) -> list[Path]:
    candidates: list[Path] = []
    for path in root.rglob("*"):
        if not path.is_file() or path.is_symlink() or _is_skipped(path, root):
            continue
        if (
            path.suffix == ".go"
            or path.name in GO_MODULE_NAMES
            or path.name in NATIVE_BUILD_NAMES
            or path.suffix in NATIVE_BUILD_SUFFIXES
        ):
            candidates.append(path)
    return sorted(candidates)


def _read_lines(path: Path) -> list[str]:
    try:
        return path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeDecodeError) as exc:
        raise ValueError(f"no se puede inspeccionar {path}: {exc}") from exc


def _forbidden_import_reason(import_path: str) -> str | None:
    if import_path == "plugin":
        return 'stdlib "plugin" permite cargar código Go en proceso'
    lowered = import_path.lower()
    for prefix, reason in FORBIDDEN_IMPORT_PREFIXES.items():
        if lowered == prefix or lowered.startswith(prefix + "/"):
            return reason
    if any(segment in LUA_IMPORT_SEGMENTS for segment in lowered.split("/")):
        return "intérprete Lua embebido"
    return None


def _go_imports(lines: list[str]) -> list[tuple[int, str]]:
    imports: list[tuple[int, str]] = []
    in_block = False
    for line_number, line in enumerate(lines, start=1):
        if in_block:
            if line.lstrip().startswith(")"):
                in_block = False
                continue
            match = GO_BLOCK_IMPORT.match(line)
            if match:
                imports.append((line_number, match.group(1)))
            continue

        match = GO_SINGLE_IMPORT.match(line)
        if match:
            imports.append((line_number, match.group(1)))
            continue
        if GO_IMPORT_BLOCK_START.match(line):
            in_block = True
    return imports


def _scan_go(path: Path, lines: list[str]) -> list[Finding]:
    findings: list[Finding] = []
    for line_number, import_path in _go_imports(lines):
        reason = _forbidden_import_reason(import_path)
        if reason:
            findings.append(
                Finding(path, line_number, f'import prohibido {import_path!r}: {reason}')
            )

    for line_number, line in enumerate(lines, start=1):
        if not line.lstrip().startswith("#cgo"):
            continue
        for pattern, reason in NATIVE_LUA_PATTERNS:
            if pattern.search(line):
                findings.append(Finding(path, line_number, reason))
    return findings


def _scan_go_module(path: Path, lines: list[str]) -> list[Finding]:
    findings: list[Finding] = []
    for line_number, line in enumerate(lines, start=1):
        declaration = line.split("//", 1)[0].strip()
        if not declaration:
            continue
        for token in re.findall(r"[A-Za-z0-9._~+-]+(?:/[A-Za-z0-9._~+-]+)+", declaration):
            reason = _forbidden_import_reason(token)
            if reason:
                findings.append(
                    Finding(path, line_number, f'módulo prohibido {token!r}: {reason}')
                )
    return findings


def _scan_native_build(path: Path, lines: list[str]) -> list[Finding]:
    findings: list[Finding] = []
    for line_number, line in enumerate(lines, start=1):
        declaration = line.split("#", 1)[0]
        for pattern, reason in NATIVE_LUA_PATTERNS:
            if pattern.search(declaration):
                findings.append(Finding(path, line_number, reason))
    return findings


def check_repository(root: Path) -> list[Finding]:
    root = root.resolve()
    findings: list[Finding] = []
    for path in _candidate_files(root):
        lines = _read_lines(path)
        if path.suffix == ".go":
            findings.extend(_scan_go(path, lines))
        if path.name in GO_MODULE_NAMES:
            findings.extend(_scan_go_module(path, lines))
        if path.name in NATIVE_BUILD_NAMES or path.suffix in NATIVE_BUILD_SUFFIXES:
            findings.extend(_scan_native_build(path, lines))
    return sorted(findings, key=lambda item: (str(item.path), item.line, item.reason))


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Impide introducir cargadores de plugins runtime en GrxFirma."
    )
    parser.add_argument(
        "--root",
        type=Path,
        default=ROOT,
        help="Raíz del repositorio que se inspeccionará.",
    )
    args = parser.parse_args()

    try:
        findings = check_repository(args.root)
    except ValueError as exc:
        print(f"gate de plugins runtime: ERROR: {exc}", file=sys.stderr)
        return 2

    if findings:
        print(
            "gate de plugins runtime: se detectó carga de código dinámico prohibida:",
            file=sys.stderr,
        )
        for finding in findings:
            print(f"  - {finding.render(args.root.resolve())}", file=sys.stderr)
        print(
            "Integra la capacidad en el producto o abre una ADR nueva con aislamiento "
            "fuera de proceso antes de cambiar esta política.",
            file=sys.stderr,
        )
        return 1

    print("gate de plugins runtime: OK (sin cargadores dinámicos en proceso).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
