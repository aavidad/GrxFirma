#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Migra de forma reproducible las cabeceras legales de ficheros propios.

Solo modifica rutas versionadas clasificadas como ``inline`` por
``legal/licensing.toml``. Las licencias de terceros, los recursos binarios, los
JSON, los ficheros generados y los formatos sin comentarios quedan intactos.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Iterable, Sequence

PROJECT_ROOT = Path(__file__).resolve().parents[2]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

from scripts.compliance.check_legal_headers import (
    DEFAULT_POLICY,
    ROOT,
    CanonicalHeader,
    classify_path,
    list_tracked_files,
    load_policy,
)


UTF8_BOM = b"\xef\xbb\xbf"
XML_LIKE_EXTENSIONS = frozenset(
    {
        ".adml",
        ".admx",
        ".csproj",
        ".entitlements",
        ".html",
        ".manifest",
        ".md",
        ".plist",
        ".props",
        ".qrc",
        ".svg",
        ".tmpl",
        ".xaml",
        ".xcprivacy",
        ".xml",
    }
)
SLASH_EXTENSIONS = frozenset(
    {
        ".c",
        ".cpp",
        ".java",
        ".cs",
        ".go",
        ".h",
        ".js",
        ".kt",
        ".kts",
        ".m",
        ".mjs",
        ".mod",
        ".qml",
        ".rc",
        ".swift",
        ".xcconfig",
    }
)
HASH_EXTENSIONS = frozenset(
    {
        ".bats",
        ".desktop",
        ".env",
        ".pro",
        ".properties",
        ".ps1",
        ".py",
        ".rb",
        ".sh",
        ".toml",
        ".yml",
    }
)
HASH_FILENAMES = frozenset(
    {
        ".gitattributes",
        ".gitignore",
        ".gitleaksignore",
        "Makefile",
        "pre-push",
    }
)

PROJECT_LEGAL_PATTERNS = tuple(
    re.compile(pattern, flags=re.IGNORECASE)
    for pattern in (
        r"SPDX-License-Identifier:\s*(?:GPL-3\.0-or-later|EUPL-1\.2)",
        r"Copyright\s+\(C\)\s+2026\s+Diputacion\s+de\s+Granada\.?",
        (
            r"Derechos\s+de\s+autor\s+\(C\)\s+2026\s+"
            r"Alberto\s+Avidad\s+Fernández\.?"
        ),
        (
            r"Autor(?:ía)?:\s*Alberto\s+Avidad\s+Fern(?:á|a)ndez"
            r"(?:\s*\(Oficina\s+de\s+Software\s+Libre\s+de\s+la\s+"
            r"Diputacion\s+de\s+Granada\))?\.?"
        ),
        (
            r"Autor:\s*Oficina\s+de\s+Software\s+Libre\s+de\s+la\s+"
            r"Diputacion\s+de\s+Granada\.?"
        ),
        r"Licencia:\s*EUPL\s+1\.2\s+o\s+posterior\.?",
    )
)
ENCODING_COOKIE = re.compile(r"coding[:=]\s*[-\w.]+")
RUBY_MAGIC_COMMENT = re.compile(
    r"#\s*(?:frozen_string_literal|warn_indent|shareable_constant_value):.*"
)
XML_DECLARATION = re.compile(r"<\?xml\b.*\?>", flags=re.IGNORECASE)
HTML_DOCTYPE = re.compile(r"<!doctype\s+html\b.*>", flags=re.IGNORECASE)


@dataclass(frozen=True)
class CommentStyle:
    prefix: str
    suffix: str = ""

    def render(self, lines: Sequence[str], newline: str) -> str:
        return "".join(
            f"{self.prefix}{line}{self.suffix}{newline}" for line in lines
        )


@dataclass(frozen=True)
class MigrationResult:
    path: str
    changed: bool


def _effective_suffix(path: str) -> str:
    pure_path = PurePosixPath(path)
    suffix = pure_path.suffix.lower()
    if suffix == ".in":
        suffix = PurePosixPath(pure_path.stem).suffix.lower()
    return suffix


def comment_style_for_path(path: str) -> CommentStyle:
    """Devuelve el comentario seguro para una ruta ``inline``."""
    pure_path = PurePosixPath(path)
    suffix = _effective_suffix(path)
    if suffix in XML_LIKE_EXTENSIONS:
        return CommentStyle("<!-- ", " -->")
    if suffix in SLASH_EXTENSIONS:
        return CommentStyle("// ")
    if suffix in HASH_EXTENSIONS or pure_path.name in HASH_FILENAMES:
        return CommentStyle("# ")
    if suffix == ".css":
        return CommentStyle("/* ", " */")
    if suffix in {".nsh", ".nsi"}:
        return CommentStyle("; ")
    if suffix in {".1", ".7"}:
        return CommentStyle('.\\" ')
    if suffix == ".bat":
        return CommentStyle("@rem ")
    raise ValueError(f"no hay sintaxis de comentario definida para {path}")


def _line_body(line: str) -> str:
    return line.rstrip("\r\n")


def _unwrap_comment(line: str) -> str:
    value = line.strip()
    wrappers = (
        ("<!--", "-->"),
        ("/*", "*/"),
    )
    for prefix, suffix in wrappers:
        if value.startswith(prefix) and value.endswith(suffix):
            return value[len(prefix) : -len(suffix)].strip()
    for prefix in ("//", "#", ";", '.\\"', "@rem", "REM"):
        if value.lower().startswith(prefix.lower()):
            return value[len(prefix) :].strip()
    return value


def _is_project_legal_line(line: str) -> bool:
    content = _unwrap_comment(line)
    return any(pattern.fullmatch(content) for pattern in PROJECT_LEGAL_PATTERNS)


def _normalize_ruby_magic_comments(lines: list[str]) -> list[str]:
    """Mantiene los comentarios mágicos Ruby inmediatamente tras el shebang."""
    if not lines:
        return lines
    insert_at = 1 if _line_body(lines[0]).strip().startswith("#!") else 0
    magic_indexes = [
        index
        for index, line in enumerate(lines[:16])
        if RUBY_MAGIC_COMMENT.fullmatch(_line_body(line).strip())
        or ENCODING_COOKIE.search(_line_body(line))
    ]
    if not magic_indexes:
        return lines
    magic_lines = [lines[index] for index in magic_indexes]
    remaining = [
        line for index, line in enumerate(lines) if index not in magic_indexes
    ]
    return remaining[:insert_at] + magic_lines + remaining[insert_at:]


def _normalize_roff_title(lines: list[str]) -> list[str]:
    """Conserva la macro .TH como primera línea del manual."""
    title_index = next(
        (
            index
            for index, line in enumerate(lines[:12])
            if _line_body(line).startswith(".TH ")
        ),
        None,
    )
    if title_index is None or title_index == 0:
        return lines
    return [lines[title_index], *lines[:title_index], *lines[title_index + 1 :]]


def _prologue_end(lines: Sequence[str], suffix: str) -> int:
    """Sitúa la cabecera tras los prólogos que deben ser la primera línea."""
    if not lines:
        return 0

    first = _line_body(lines[0]).strip()
    if suffix in {".1", ".7"} and first.startswith(".TH "):
        return 1
    if suffix == ".md" and first == "---":
        for index, line in enumerate(lines[1:80], start=1):
            if _line_body(line).strip() == "---":
                return index + 1

    index = 0
    if suffix == ".go" and first.startswith("//go:build "):
        index = 1
        if index < len(lines) and _line_body(lines[index]).startswith("// +build "):
            index += 1
        if index < len(lines) and not _line_body(lines[index]).strip():
            index += 1
        return index
    if first.startswith("#!"):
        index = 1
        while index < len(lines) and (
            ENCODING_COOKIE.search(_line_body(lines[index]))
            or (
                suffix == ".rb"
                and RUBY_MAGIC_COMMENT.fullmatch(_line_body(lines[index]).strip())
            )
        ):
            index += 1
        return index
    if ENCODING_COOKIE.search(first):
        return 1
    if XML_DECLARATION.fullmatch(first) or HTML_DOCTYPE.fullmatch(first):
        return 1
    return 0


def _legacy_header_end(lines: Sequence[str], start: int) -> int:
    """Encuentra solo un bloque legal propio, sin consumir otros avisos."""
    cursor = start
    while cursor < len(lines) and not _line_body(lines[cursor]).strip():
        cursor += 1
    if cursor >= len(lines) or not _is_project_legal_line(
        _line_body(lines[cursor])
    ):
        return start

    saw_legal_line = False
    while cursor < len(lines):
        body = _line_body(lines[cursor])
        if _is_project_legal_line(body):
            saw_legal_line = True
            cursor += 1
            continue
        if not body.strip():
            cursor += 1
            continue
        break
    return cursor if saw_legal_line else start


def _detect_newline(text: str) -> str:
    first_lf = text.find("\n")
    if first_lf >= 0 and first_lf > 0 and text[first_lf - 1] == "\r":
        return "\r\n"
    if first_lf >= 0:
        return "\n"
    if "\r" in text:
        return "\r"
    return "\n"


def migrate_content(
    path: str,
    raw: bytes,
    canonical_header: CanonicalHeader,
) -> bytes:
    """Devuelve el contenido normalizado conservando BOM, saltos y prólogos."""
    has_bom = raw.startswith(UTF8_BOM)
    encoded_text = raw[len(UTF8_BOM) :] if has_bom else raw
    text = encoded_text.decode("utf-8", errors="strict")
    newline = _detect_newline(text)
    lines = text.splitlines(keepends=True)
    suffix = _effective_suffix(path)
    if suffix == ".rb":
        lines = _normalize_ruby_magic_comments(lines)
    if suffix in {".1", ".7"}:
        lines = _normalize_roff_title(lines)
    insertion_index = _prologue_end(lines, suffix)
    legacy_end = _legacy_header_end(lines, insertion_index)

    body_index = legacy_end
    while body_index < len(lines) and not _line_body(lines[body_index]).strip():
        body_index += 1

    style = comment_style_for_path(path)
    rendered_header = style.render(canonical_header.required_lines, newline)
    prefix = "".join(lines[:insertion_index])
    body = "".join(lines[body_index:])
    if prefix and not prefix.endswith(("\n", "\r")):
        prefix += newline
    migrated = prefix + rendered_header
    if body:
        migrated += newline + body

    result = migrated.encode("utf-8")
    return (UTF8_BOM + result) if has_bom else result


def migrate_repository(
    root: Path,
    *,
    policy_path: Path,
    check_only: bool = False,
    tracked_files: Iterable[str] | None = None,
) -> tuple[MigrationResult, ...]:
    """Migra únicamente los ficheros propios con comentarios."""
    policy = load_policy(policy_path)
    paths = tuple(
        sorted(
            tracked_files
            if tracked_files is not None
            else list_tracked_files(root)
        )
    )
    results: list[MigrationResult] = []
    for relative_path in paths:
        if classify_path(relative_path, policy).category != "inline":
            continue
        disk_path = root / relative_path
        original = disk_path.read_bytes()
        migrated = migrate_content(
            relative_path,
            original,
            policy.canonical_header,
        )
        changed = migrated != original
        if changed and not check_only:
            disk_path.write_bytes(migrated)
        results.append(MigrationResult(path=relative_path, changed=changed))
    return tuple(results)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Normaliza las cabeceras legales de los ficheros propios que "
            "admiten comentarios."
        )
    )
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--policy", type=Path)
    parser.add_argument(
        "--check",
        action="store_true",
        help="no escribe; falla si alguna cabecera necesita migración",
    )
    parser.add_argument("--json", action="store_true", dest="as_json")
    args = parser.parse_args(argv)

    root = args.root.resolve()
    policy_path = (
        args.policy.resolve()
        if args.policy
        else root / DEFAULT_POLICY.relative_to(ROOT)
    )
    try:
        results = migrate_repository(
            root,
            policy_path=policy_path,
            check_only=args.check,
        )
    except (OSError, UnicodeError, ValueError) as error:
        print(f"No se pudo ejecutar la migración legal: {error}", file=sys.stderr)
        return 2

    changed = tuple(result.path for result in results if result.changed)
    if args.as_json:
        print(
            json.dumps(
                {
                    "inline_files": len(results),
                    "changed_files": len(changed),
                    "paths": changed,
                },
                ensure_ascii=False,
                indent=2,
                sort_keys=True,
            )
        )
    else:
        action = "pendientes" if args.check else "actualizados"
        print(
            f"Ficheros inline: {len(results)}. "
            f"Ficheros {action}: {len(changed)}."
        )
    return 1 if args.check and changed else 0


if __name__ == "__main__":
    raise SystemExit(main())
