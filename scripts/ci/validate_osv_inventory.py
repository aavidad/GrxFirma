#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Ensure every manifest discovered by OSV has an explicit CI gate."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


class InventoryError(ValueError):
    """Raised when the OSV inventory does not match the gated manifest set."""


def inventory_sources(report: Path, root: Path) -> set[str]:
    """Return repository-relative source paths from an OSV JSON report."""
    try:
        payload = json.loads(report.read_text(encoding="utf-8"))
    except json.JSONDecodeError as error:
        raise InventoryError(f"invalid OSV JSON report: {error}") from error

    results = payload.get("results")
    if not isinstance(results, list):
        raise InventoryError("OSV report does not contain a results array")

    root = root.resolve()
    sources: set[str] = set()
    for index, result in enumerate(results):
        if not isinstance(result, dict):
            raise InventoryError(f"OSV result {index} is not an object")
        source = result.get("source")
        if not isinstance(source, dict) or not isinstance(source.get("path"), str):
            raise InventoryError(f"OSV result {index} has no source path")
        source_path = Path(source["path"]).resolve()
        try:
            relative = source_path.relative_to(root)
        except ValueError as error:
            raise InventoryError(
                f"OSV source is outside the repository: {source_path}"
            ) from error
        normalized = relative.as_posix()
        if normalized in sources:
            raise InventoryError(f"OSV source is repeated: {normalized}")
        sources.add(normalized)
    return sources


def validate_inventory(
    report: Path,
    root: Path,
    expected_sources: list[str],
) -> None:
    """Fail unless OSV discovered exactly the manifests gated by CI."""
    if not expected_sources:
        raise InventoryError("at least one expected OSV source is required")
    if len(set(expected_sources)) != len(expected_sources):
        raise InventoryError("expected OSV sources must not be repeated")

    expected = {Path(source).as_posix() for source in expected_sources}
    discovered = inventory_sources(report, root)
    missing = sorted(expected - discovered)
    unexpected = sorted(discovered - expected)
    if missing or unexpected:
        details: list[str] = []
        if missing:
            details.append("missing expected sources: " + ", ".join(missing))
        if unexpected:
            details.append(
                "new ungated dependency sources: " + ", ".join(unexpected)
            )
        raise InventoryError("; ".join(details))


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Validate that OSV discovered exactly the dependency manifests "
            "covered by explicit CI gates."
        )
    )
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument(
        "--expected-source",
        action="append",
        dest="expected_sources",
        required=True,
    )
    args = parser.parse_args(argv)

    try:
        validate_inventory(
            args.report,
            args.root,
            args.expected_sources,
        )
    except (OSError, InventoryError) as error:
        print(f"OSV inventory scope validation failed: {error}", file=sys.stderr)
        return 1

    print(
        f"OSV inventory scope validated: {len(args.expected_sources)} "
        "manifests have explicit gates."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
