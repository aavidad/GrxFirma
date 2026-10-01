#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Extract selected Gradle configurations into an OSV-scannable lockfile."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


class LockfileError(ValueError):
    """Raised when the input lockfile cannot prove the requested scope."""


def filter_lockfile(
    source: Path,
    configurations: list[str],
) -> str:
    """Return a lockfile containing only dependencies in the selected scopes."""
    if not configurations:
        raise LockfileError("at least one Gradle configuration is required")
    if len(set(configurations)) != len(configurations):
        raise LockfileError("Gradle configurations must not be repeated")

    requested = set(configurations)
    seen: set[str] = set()
    output: list[str] = []

    for line_number, line in enumerate(
        source.read_text(encoding="utf-8").splitlines(), start=1
    ):
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            output.append(line)
            continue

        coordinate, separator, raw_scopes = stripped.partition("=")
        if not separator or not coordinate or not raw_scopes:
            raise LockfileError(
                f"{source}:{line_number}: malformed Gradle lockfile entry"
            )

        scopes = {scope.strip() for scope in raw_scopes.split(",") if scope.strip()}
        matching = [scope for scope in configurations if scope in scopes]
        seen.update(requested.intersection(scopes))
        if matching:
            output.append(f"{coordinate}={','.join(matching)}")

    missing = [scope for scope in configurations if scope not in seen]
    if missing:
        raise LockfileError(
            "requested Gradle configurations are absent from the lockfile: "
            + ", ".join(missing)
        )

    return "\n".join(output).rstrip() + "\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Create a Gradle lockfile limited to configurations shipped at "
            "runtime, suitable for a scoped OSV-Scanner gate."
        )
    )
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument(
        "--configuration",
        action="append",
        dest="configurations",
        required=True,
    )
    args = parser.parse_args(argv)

    try:
        filtered = filter_lockfile(args.input, args.configurations)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(filtered, encoding="utf-8")
    except (OSError, LockfileError) as error:
        print(f"Gradle lockfile filtering failed: {error}", file=sys.stderr)
        return 1

    package_count = sum(
        1
        for line in filtered.splitlines()
        if line and not line.startswith("#") and not line.startswith("empty=")
    )
    print(
        f"Filtered {package_count} runtime dependencies for "
        f"{', '.join(args.configurations)}."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
