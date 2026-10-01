#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Fail closed unless Android release identity and source are exact."""

from __future__ import annotations

import hashlib
import os
import pathlib
import re
import subprocess
import sys


REQUIRED = (
    "GRXFIRMA_ANDROID_KEYSTORE",
    "GRXFIRMA_ANDROID_KEYSTORE_PASSWORD",
    "GRXFIRMA_ANDROID_KEY_ALIAS",
    "GRXFIRMA_ANDROID_KEY_PASSWORD",
    "GRXFIRMA_ANDROID_SIGNING_CERT_SHA256",
    "GRXFIRMA_ANDROID_SOURCE_COMMIT",
)
ROOT = pathlib.Path(__file__).resolve().parents[3]
SEMVER_RE = re.compile(r"^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$")
COMMIT_RE = re.compile(r"^[0-9a-fA-F]{40}$")


def normalize_fingerprint(raw: str) -> str:
    return re.sub(r"[^0-9A-Fa-f]", "", raw).lower()


def git_text(*args: str) -> str:
    result = subprocess.run(
        ["git", "-C", str(ROOT), *args],
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout.strip()


def export_signing_certificate(keystore: pathlib.Path, alias: str) -> bytes:
    result = subprocess.run(
        [
            "keytool",
            "-exportcert",
            "-keystore",
            str(keystore),
            "-alias",
            alias,
            "-storepass:env",
            "GRXFIRMA_ANDROID_KEYSTORE_PASSWORD",
        ],
        check=True,
        capture_output=True,
    )
    if not result.stdout:
        raise ValueError("el alias Android no contiene un certificado")
    return result.stdout


def main() -> int:
    missing = [name for name in REQUIRED if not os.environ.get(name)]
    if missing:
        print(
            f"ERROR: faltan credenciales Android: {', '.join(missing)}", file=sys.stderr
        )
        return 1
    keystore = pathlib.Path(os.environ["GRXFIRMA_ANDROID_KEYSTORE"]).expanduser()
    if keystore.is_symlink() or not keystore.is_file():
        print(f"ERROR: el almacen Android no existe: {keystore}", file=sys.stderr)
        return 1
    expected_fingerprint = normalize_fingerprint(
        os.environ["GRXFIRMA_ANDROID_SIGNING_CERT_SHA256"]
    )
    if not re.fullmatch(r"[0-9a-f]{64}", expected_fingerprint):
        print(
            "ERROR: GRXFIRMA_ANDROID_SIGNING_CERT_SHA256 debe ser una huella SHA-256.",
            file=sys.stderr,
        )
        return 1
    source_commit = os.environ["GRXFIRMA_ANDROID_SOURCE_COMMIT"].lower()
    if not COMMIT_RE.fullmatch(source_commit):
        print(
            "ERROR: GRXFIRMA_ANDROID_SOURCE_COMMIT debe contener 40 hexadecimales.",
            file=sys.stderr,
        )
        return 1
    version = (ROOT / "VERSION.txt").read_text(encoding="utf-8").strip()
    configured_version = os.environ.get("GRXFIRMA_ANDROID_VERSION_NAME", version)
    if not SEMVER_RE.fullmatch(version) or configured_version != version:
        print(
            "ERROR: la version Android debe coincidir con VERSION.txt y ser SemVer.",
            file=sys.stderr,
        )
        return 1
    try:
        head = git_text("rev-parse", "--verify", "HEAD^{commit}").lower()
        dirty = git_text("status", "--porcelain=v1", "--untracked-files=all")
        certificate = export_signing_certificate(
            keystore,
            os.environ["GRXFIRMA_ANDROID_KEY_ALIAS"],
        )
    except (OSError, subprocess.CalledProcessError, ValueError) as error:
        print(f"ERROR: preflight de firma Android falló: {error}", file=sys.stderr)
        return 1
    if head != source_commit:
        print(
            f"ERROR: sourceCommit Android ({source_commit}) no coincide con HEAD ({head}).",
            file=sys.stderr,
        )
        return 1
    if dirty:
        print(
            "ERROR: el árbol Git debe estar limpio para una release Android.",
            file=sys.stderr,
        )
        return 1
    actual_fingerprint = hashlib.sha256(certificate).hexdigest()
    if actual_fingerprint != expected_fingerprint:
        print(
            "ERROR: la huella del certificado de firma Android no coincide.",
            file=sys.stderr,
        )
        return 1
    print(
        "Preflight Android correcto; "
        f"version={version} sourceCommit={source_commit} "
        f"signerSha256={actual_fingerprint}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
