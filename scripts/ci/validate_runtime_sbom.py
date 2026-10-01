#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Validate that an SPDX runtime SBOM covers the filtered Android lockfile."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from urllib.parse import quote


class RuntimeSbomError(ValueError):
    """Raised when the runtime SBOM and its declared scope diverge."""


def _android_purls(lockfile: Path) -> set[str]:
    expected: set[str] = set()
    for line_number, line in enumerate(
        lockfile.read_text(encoding="utf-8").splitlines(), start=1
    ):
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or stripped.startswith("empty="):
            continue
        coordinate, separator, configurations = stripped.partition("=")
        parts = coordinate.split(":")
        if not separator or not configurations or len(parts) != 3 or not all(parts):
            raise RuntimeSbomError(
                f"{lockfile}:{line_number}: malformed Gradle lockfile entry"
            )
        group, artifact, version = parts
        expected.add(
            "pkg:maven/"
            f"{quote(group, safe='.-_~')}/{quote(artifact, safe='.-_~')}"
            f"@{quote(version, safe='.-_~')}"
        )
    if not expected:
        raise RuntimeSbomError("Android runtime lockfile contains no packages")
    return expected


def _maven_purls(sbom: Path) -> set[str]:
    try:
        document = json.loads(sbom.read_text(encoding="utf-8"))
    except json.JSONDecodeError as error:
        raise RuntimeSbomError(f"{sbom}: invalid SPDX JSON: {error}") from error
    if not isinstance(document, dict) or not isinstance(document.get("packages"), list):
        raise RuntimeSbomError(f"{sbom}: SPDX document has no packages array")

    actual: set[str] = set()
    for package in document["packages"]:
        if not isinstance(package, dict):
            raise RuntimeSbomError(f"{sbom}: SPDX package is not an object")
        references = package.get("externalRefs", [])
        if not isinstance(references, list):
            raise RuntimeSbomError(f"{sbom}: SPDX externalRefs is not an array")
        for reference in references:
            if not isinstance(reference, dict):
                continue
            locator = reference.get("referenceLocator")
            if reference.get("referenceType") == "purl" and isinstance(locator, str):
                if locator.startswith("pkg:maven/"):
                    actual.add(locator)
    return actual


def validate_android_coverage(sbom: Path, lockfile: Path) -> tuple[int, int]:
    """Require an exact Maven package match with the filtered runtime lockfile."""
    expected = _android_purls(lockfile)
    actual = _maven_purls(sbom)
    missing = sorted(expected - actual)
    unexpected = sorted(actual - expected)
    if missing or unexpected:
        details: list[str] = []
        if missing:
            details.append("missing: " + ", ".join(missing[:10]))
        if unexpected:
            details.append("outside runtime scope: " + ", ".join(unexpected[:10]))
        raise RuntimeSbomError("Android SBOM coverage mismatch; " + "; ".join(details))
    return len(expected), len(actual)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Fail unless the runtime SPDX inventory exactly covers the filtered "
            "Android release classpaths."
        )
    )
    parser.add_argument("--sbom", type=Path, required=True)
    parser.add_argument("--android-lockfile", type=Path, required=True)
    args = parser.parse_args(argv)

    try:
        expected, actual = validate_android_coverage(
            args.sbom,
            args.android_lockfile,
        )
    except (OSError, RuntimeSbomError) as error:
        print(f"Runtime SBOM validation failed: {error}", file=sys.stderr)
        return 1

    print(
        f"Runtime SBOM covers all {expected} Android release packages "
        f"({actual} Maven packages inventoried)."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
