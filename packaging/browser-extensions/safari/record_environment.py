#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Record deterministic Safari build provenance in the integration report."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import stat
import sys
import tempfile
from pathlib import Path
from typing import Optional


class EnvironmentRecordError(RuntimeError):
    pass


def parse_args(argv: Optional[list[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Registra el entorno del build Safari")
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--xcode-version", required=True)
    parser.add_argument("--converter-path", required=True)
    parser.add_argument("--minimum-macos", required=True)
    return parser.parse_args(argv)


def source_tree_sha256(source: Path) -> str:
    if source.is_symlink():
        raise EnvironmentRecordError(
            f"la fuente Safari no puede ser un enlace simbolico: {source}"
        )
    source = source.resolve()
    if not source.is_dir():
        raise EnvironmentRecordError(f"no existe la fuente Safari: {source}")
    digest = hashlib.sha256()
    for path in sorted(source.rglob("*")):
        if path.is_symlink():
            raise EnvironmentRecordError(
                f"la fuente Safari no puede contener enlaces simbolicos: {path}"
            )
        if not path.is_file():
            continue
        relative = path.relative_to(source).as_posix().encode("utf-8")
        data = path.read_bytes()
        digest.update(len(relative).to_bytes(8, "big"))
        digest.update(relative)
        digest.update(len(data).to_bytes(8, "big"))
        digest.update(data)
    return digest.hexdigest()


def atomic_write(path: Path, data: bytes) -> None:
    if path.is_symlink():
        raise EnvironmentRecordError(
            "integration-report.json no puede ser un enlace simbolico"
        )
    mode = stat.S_IMODE(path.stat().st_mode) if path.exists() else 0o600
    descriptor, temporary_name = tempfile.mkstemp(
        dir=path.parent, prefix=f".{path.name}.", suffix=".tmp"
    )
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.chmod(mode)
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def record_environment(
    source: Path,
    report_path: Path,
    xcode_version: str,
    converter_path: str,
    minimum_macos: str,
) -> dict[str, object]:
    if report_path.is_symlink() or not report_path.is_file():
        raise EnvironmentRecordError(
            f"no existe un informe de integracion regular: {report_path}"
        )
    try:
        report = json.loads(report_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise EnvironmentRecordError(
            f"informe de integracion no valido: {exc}"
        ) from exc
    if not isinstance(report, dict) or report.get("schema") != 1:
        raise EnvironmentRecordError("schema de informe de integracion no soportado")
    report["build_environment"] = {
        "converter_path": converter_path,
        "minimum_macos": minimum_macos,
        "source_tree_sha256": source_tree_sha256(source),
        "xcode_version": xcode_version.splitlines(),
    }
    encoded = (json.dumps(report, indent=2, sort_keys=True) + "\n").encode("utf-8")
    atomic_write(report_path, encoded)
    return report


def main(argv: Optional[list[str]] = None) -> int:
    args = parse_args(argv)
    try:
        report = record_environment(
            args.source,
            args.report,
            args.xcode_version,
            args.converter_path,
            args.minimum_macos,
        )
    except EnvironmentRecordError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(report["build_environment"], indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
