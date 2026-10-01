#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Prepare the dependency manifests that describe shipped runtime code."""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import stat
import sys
from pathlib import Path

if __package__:
    from scripts.ci.filter_gradle_lockfile import LockfileError, filter_lockfile
else:
    from filter_gradle_lockfile import LockfileError, filter_lockfile


GO_RUNTIME_MANIFESTS = (
    Path("go.mod"),
    Path("go.sum"),
    Path("third_party/pdfsign/go.mod"),
    Path("third_party/pdfsign/go.sum"),
)
ANDROID_LOCKFILE = Path("mobile/android/app/gradle.lockfile")
ANDROID_RUNTIME_CONFIGURATIONS = (
    "productionReleaseRuntimeClasspath",
    "verificationReleaseRuntimeClasspath",
)
SCOPE_ATTESTATION = Path("grxfirma-runtime-scope.json")


class RuntimeScopeError(ValueError):
    """Raised when the requested runtime scope cannot be proven."""


def _regular_source(root: Path, relative: Path) -> Path:
    source = root / relative
    try:
        metadata = source.lstat()
    except OSError as error:
        raise RuntimeScopeError(f"required manifest is unavailable: {relative}") from error
    if not stat.S_ISREG(metadata.st_mode):
        raise RuntimeScopeError(f"required manifest is not a regular file: {relative}")
    return source


def _copy_manifest(root: Path, destination: Path, relative: Path) -> dict[str, str]:
    source = _regular_source(root, relative)
    target = destination / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)
    return {
        "source": relative.as_posix(),
        "target": relative.as_posix(),
        "sha256": hashlib.sha256(target.read_bytes()).hexdigest(),
    }


def prepare_runtime_scope(root: Path, destination: Path) -> dict[str, object]:
    """Create a fresh, auditable manifest tree limited to shipped runtimes."""
    root = root.resolve()
    try:
        destination_metadata = destination.lstat()
    except FileNotFoundError:
        destination_metadata = None
    except OSError as error:
        raise RuntimeScopeError(
            f"runtime scope destination cannot be inspected: {destination}"
        ) from error
    if destination_metadata is not None:
        if not stat.S_ISDIR(destination_metadata.st_mode):
            raise RuntimeScopeError(
                f"runtime scope destination is not a directory: {destination}"
            )
        try:
            if any(destination.iterdir()):
                raise RuntimeScopeError(
                    f"runtime scope destination is not empty: {destination}"
                )
        except OSError as error:
            raise RuntimeScopeError(
                f"runtime scope destination cannot be inspected: {destination}"
            ) from error
    destination.mkdir(parents=True, exist_ok=True)

    copied = [
        _copy_manifest(root, destination, relative)
        for relative in GO_RUNTIME_MANIFESTS
    ]

    android_source = _regular_source(root, ANDROID_LOCKFILE)
    try:
        filtered_lockfile = filter_lockfile(
            android_source,
            list(ANDROID_RUNTIME_CONFIGURATIONS),
        )
    except (OSError, LockfileError) as error:
        raise RuntimeScopeError(f"Android runtime scope is invalid: {error}") from error

    android_target = destination / ANDROID_LOCKFILE
    android_target.parent.mkdir(parents=True, exist_ok=True)
    android_target.write_text(filtered_lockfile, encoding="utf-8")
    copied.append(
        {
            "source": ANDROID_LOCKFILE.as_posix(),
            "target": ANDROID_LOCKFILE.as_posix(),
            "sha256": hashlib.sha256(android_target.read_bytes()).hexdigest(),
        }
    )

    attestation: dict[str, object] = {
        "schema": 1,
        "scope": "distributed-runtime-dependency-manifests",
        "go_modules": [".", "third_party/pdfsign"],
        "android_configurations": list(ANDROID_RUNTIME_CONFIGURATIONS),
        "manifests": copied,
    }
    (destination / SCOPE_ATTESTATION).write_text(
        json.dumps(attestation, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    return attestation


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Build the fail-closed manifest scope used by the runtime SBOM gate."
        )
    )
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args(argv)

    try:
        attestation = prepare_runtime_scope(args.root, args.output)
    except (OSError, RuntimeScopeError) as error:
        print(f"Runtime SBOM scope preparation failed: {error}", file=sys.stderr)
        return 1

    print(
        "Prepared runtime SBOM scope with "
        f"{len(attestation['manifests'])} manifests and "
        f"{len(attestation['android_configurations'])} Android release classpaths."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
