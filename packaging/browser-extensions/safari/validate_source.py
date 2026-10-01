#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Validate Safari converter inputs without requiring macOS or Xcode."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Optional


class SourceValidationError(RuntimeError):
    pass


ALLOWED_HOST_PATTERNS = {
    "https://*.dipgra.es/*",
    "https://portal.example/*",
    "https://*.savia.net/*",
    "https://127.0.0.1/*",
}


def parse_version(value: str) -> tuple[int, int, int]:
    if not re.fullmatch(r"[0-9]+(?:\.[0-9]+){1,2}", value):
        raise SourceValidationError(
            "SAFARI_MIN_MACOS debe ser una version numerica valida"
        )
    parts = tuple(int(part) for part in value.split("."))
    return (parts + (0, 0, 0))[:3]


def list_field(manifest: dict[str, object], name: str) -> list[object]:
    value = manifest.get(name, [])
    if not isinstance(value, list):
        raise SourceValidationError(f"{name} debe ser una lista")
    return value


def validate_manifest(path: Path, minimum_macos: str) -> dict[str, object]:
    try:
        manifest = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise SourceValidationError(f"manifest Safari no valido: {exc}") from exc
    if not isinstance(manifest, dict):
        raise SourceValidationError("manifest.json debe contener un objeto JSON")

    permissions = manifest.get("permissions")
    if not isinstance(permissions, list) or "nativeMessaging" not in permissions:
        raise SourceValidationError(
            "manifest.json debe declarar el permiso nativeMessaging"
        )
    manifest_version = manifest.get("manifest_version")
    if manifest_version not in (2, 3):
        raise SourceValidationError(
            "version de manifest no soportada por el empaquetador Safari"
        )
    minimum = parse_version(minimum_macos)
    if manifest_version == 3 and minimum < (12, 3, 0):
        raise SourceValidationError(
            "Manifest V3 requiere SAFARI_MIN_MACOS=12.3 o posterior (Safari 15.4+)"
        )

    if "externally_connectable" in manifest:
        raise SourceValidationError(
            "externally_connectable no esta permitido en la integracion Safari"
        )
    declared_hosts = list(list_field(manifest, "host_permissions"))
    declared_hosts.extend(
        item for item in permissions if isinstance(item, str) and "://" in item
    )
    if any(
        not isinstance(host, str) or host not in ALLOWED_HOST_PATTERNS
        for host in declared_hosts
    ):
        raise SourceValidationError(
            "el manifest Safari contiene permisos de host fuera de la allowlist"
        )
    optional_hosts = list_field(manifest, "optional_host_permissions")
    if any(
        not isinstance(host, str)
        or host not in (ALLOWED_HOST_PATTERNS | {"https://*/*"})
        for host in optional_hosts
    ):
        raise SourceValidationError(
            "el manifest Safari contiene permisos opcionales no permitidos"
        )
    for content_script in list_field(manifest, "content_scripts"):
        if not isinstance(content_script, dict):
            raise SourceValidationError(
                "content_scripts contiene una entrada no valida"
            )
        matches = content_script.get("matches", [])
        if not isinstance(matches, list) or any(
            not isinstance(match, str) or match not in ALLOWED_HOST_PATTERNS
            for match in matches
        ):
            raise SourceValidationError(
                "content_scripts contiene un origen fuera de la allowlist Safari"
            )
    return {
        "manifest_version": manifest_version,
        "minimum_macos": ".".join(str(part) for part in minimum),
        "native_messaging": True,
    }


def parse_args(argv: Optional[list[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Valida la fuente Safari Web Extension"
    )
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--minimum-macos", required=True)
    return parser.parse_args(argv)


def main(argv: Optional[list[str]] = None) -> int:
    args = parse_args(argv)
    try:
        result = validate_manifest(args.manifest, args.minimum_macos)
    except SourceValidationError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
