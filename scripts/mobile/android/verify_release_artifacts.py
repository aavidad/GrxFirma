#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Verify signed Android APK/AAB artifacts and emit public evidence."""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import pathlib
import re
import struct
import subprocess
import sys
import tempfile
import zipfile


FINGERPRINT_RE = re.compile(r"^[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")
SEMVER_RE = re.compile(r"^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$")
PACKAGE_RE = re.compile(
    r"package: name='([^']+)' versionCode='(\d+)' versionName='([^']+)'"
)
APK_FINGERPRINT_RE = re.compile(
    r"Signer #\d+ certificate SHA-256 digest:\s*([0-9a-fA-F: ]+)"
)
PEM_RE = re.compile(
    rb"-----BEGIN CERTIFICATE-----\s*(.*?)\s*-----END CERTIFICATE-----",
    re.DOTALL,
)
APK_SIGNATURE_BLOCK_MAGIC = b"APK Sig Block 42"
APK_V2_BLOCK_ID = 0x7109871A
APK_V3_BLOCK_ID = 0xF05368C0
APK_DEPENDENCY_INFO_BLOCK_ID = 0x504B4453
ANDROID_NDK_VERSION = "28.2.13676358"


def sha256(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def normalize_fingerprint(raw: str) -> str:
    return re.sub(r"[^0-9A-Fa-f]", "", raw).lower()


def require_regular(path: pathlib.Path, suffix: str) -> None:
    if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
        raise ValueError(f"artefacto Android ausente o inseguro: {path}")
    if path.suffix.lower() != suffix:
        raise ValueError(f"extensión Android inesperada: {path}")


def run_text(command: list[str]) -> str:
    result = subprocess.run(command, check=True, capture_output=True, text=True)
    return result.stdout + result.stderr


def build_tool_version(path: pathlib.Path) -> tuple[int, ...]:
    version = path.parent.name
    if not re.fullmatch(r"\d+(?:\.\d+)*", version):
        return ()
    return tuple(int(part) for part in version.split("."))


def find_build_tool(name: str) -> pathlib.Path:
    sdk = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    if not sdk:
        raise ValueError("ANDROID_HOME/ANDROID_SDK_ROOT no está configurado")
    candidates = [
        path
        for path in pathlib.Path(sdk).glob(f"build-tools/*/{name}")
        if build_tool_version(path)
    ]
    if not candidates:
        raise ValueError(f"no se encuentra {name} en Android SDK")
    return max(candidates, key=build_tool_version)


def find_ndk_strip() -> pathlib.Path:
    sdk = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    if not sdk:
        raise ValueError("ANDROID_HOME/ANDROID_SDK_ROOT no está configurado")
    strip_tool = (
        pathlib.Path(sdk)
        / "ndk"
        / ANDROID_NDK_VERSION
        / "toolchains"
        / "llvm"
        / "prebuilt"
        / "linux-x86_64"
        / "bin"
        / "llvm-strip"
    )
    if not strip_tool.is_file() or not os.access(strip_tool, os.X_OK):
        raise ValueError(
            f"falta llvm-strip del NDK Android fijado {ANDROID_NDK_VERSION}"
        )
    return strip_tool


def aab_certificate_der(aab: pathlib.Path) -> bytes:
    output = subprocess.run(
        [
            "keytool",
            "-J-Duser.language=en",
            "-J-Duser.country=US",
            "-printcert",
            "-jarfile",
            str(aab),
            "-rfc",
        ],
        check=True,
        capture_output=True,
    ).stdout
    match = PEM_RE.search(output)
    if not match:
        raise ValueError("no se pudo extraer el certificado firmante del AAB")
    return base64.b64decode(re.sub(rb"\s+", b"", match.group(1)), validate=True)


def verify_aab_signature(aab: pathlib.Path, expected_fingerprint: str) -> None:
    certificate = aab_certificate_der(aab)
    if hashlib.sha256(certificate).hexdigest() != expected_fingerprint:
        raise ValueError("la firma del AAB no coincide con la huella fijada")
    with tempfile.TemporaryDirectory(prefix="grxfirma-aab-trust-") as temp:
        root = pathlib.Path(temp)
        certificate_path = root / "signer.der"
        truststore = root / "verification.p12"
        certificate_path.write_bytes(certificate)
        environment = os.environ.copy()
        environment["GRXFIRMA_ANDROID_VERIFY_TRUSTSTORE_PASSWORD"] = (
            "verification-only"
        )
        subprocess.run(
            [
                "keytool",
                "-J-Duser.language=en",
                "-J-Duser.country=US",
                "-importcert",
                "-noprompt",
                "-storetype",
                "PKCS12",
                "-keystore",
                str(truststore),
                "-storepass:env",
                "GRXFIRMA_ANDROID_VERIFY_TRUSTSTORE_PASSWORD",
                "-alias",
                "expected-android-signer",
                "-file",
                str(certificate_path),
            ],
            check=True,
            capture_output=True,
            env=environment,
        )
        result = subprocess.run(
            [
                "jarsigner",
                "-J-Duser.language=en",
                "-J-Duser.country=US",
                "-verify",
                "-strict",
                "-certs",
                "-keystore",
                str(truststore),
                "-storetype",
                "PKCS12",
                "-storepass:env",
                "GRXFIRMA_ANDROID_VERIFY_TRUSTSTORE_PASSWORD",
                str(aab),
            ],
            check=False,
            capture_output=True,
            text=True,
            env=environment,
        )
        report = result.stdout + result.stderr
        if result.returncode != 0 or "jar verified." not in report.lower():
            raise ValueError(
                f"jarsigner estricto rechazó el AAB (código {result.returncode})"
            )


def verify_embedded_core(
    apk: pathlib.Path,
    core_aar: pathlib.Path,
    expected_sha256: str,
) -> None:
    require_regular(core_aar, ".aar")
    if sha256(core_aar) != expected_sha256:
        raise ValueError("el AAR reconstruido no coincide con la huella esperada")
    strip_tool = find_ndk_strip()
    with tempfile.TemporaryDirectory(prefix="grxfirma-android-libs-") as temp:
        root = pathlib.Path(temp)
        with zipfile.ZipFile(core_aar) as aar_zip, zipfile.ZipFile(apk) as apk_zip:
            for abi in ("armeabi-v7a", "arm64-v8a", "x86_64"):
                aar_name = f"jni/{abi}/libgojni.so"
                apk_name = f"lib/{abi}/libgojni.so"
                try:
                    aar_library = aar_zip.read(aar_name)
                    apk_library = apk_zip.read(apk_name)
                except KeyError as error:
                    raise ValueError(f"falta el núcleo Android para {abi}") from error
                normalized = root / f"{abi}-libgojni.so"
                normalized.write_bytes(aar_library)
                subprocess.run(
                    [str(strip_tool), "--strip-unneeded", str(normalized)],
                    check=True,
                    capture_output=True,
                )
                if hashlib.sha256(normalized.read_bytes()).digest() != hashlib.sha256(
                    apk_library
                ).digest():
                    raise ValueError(
                        f"el APK no contiene el AAR normalizado exacto para {abi}"
                    )


def apk_signing_block_ids(apk: pathlib.Path) -> set[int]:
    with zipfile.ZipFile(apk) as archive:
        central_directory_offset = archive.start_dir
    with apk.open("rb") as stream:
        if central_directory_offset < 32:
            raise ValueError("el APK no contiene un bloque de firma válido")
        stream.seek(central_directory_offset - 24)
        footer = stream.read(24)
        if len(footer) != 24 or footer[8:] != APK_SIGNATURE_BLOCK_MAGIC:
            raise ValueError("el APK no contiene un bloque de firma v2/v3")
        block_size = struct.unpack_from("<Q", footer)[0]
        block_start = central_directory_offset - block_size - 8
        if block_start < 0:
            raise ValueError("el bloque de firma APK declara un tamaño inválido")
        stream.seek(block_start)
        header = stream.read(8)
        if len(header) != 8 or struct.unpack("<Q", header)[0] != block_size:
            raise ValueError("las cabeceras del bloque de firma APK no coinciden")
        encoded_entries = stream.read(central_directory_offset - 24 - block_start - 8)
    identifiers: set[int] = set()
    offset = 0
    while offset < len(encoded_entries):
        if len(encoded_entries) - offset < 12:
            raise ValueError("entrada truncada en el bloque de firma APK")
        entry_size = struct.unpack_from("<Q", encoded_entries, offset)[0]
        if entry_size < 4 or offset + 8 + entry_size > len(encoded_entries):
            raise ValueError("entrada inválida en el bloque de firma APK")
        identifier = struct.unpack_from("<I", encoded_entries, offset + 8)[0]
        if identifier in identifiers:
            raise ValueError("identificador duplicado en el bloque de firma APK")
        identifiers.add(identifier)
        offset += 8 + entry_size
    if offset != len(encoded_entries):
        raise ValueError("longitud inesperada en el bloque de firma APK")
    return identifiers


def verify(
    apk: pathlib.Path,
    aab: pathlib.Path,
    core_aar: pathlib.Path,
    expected_fingerprint: str,
    version_name: str,
    version_code: int,
    source_commit: str,
    core_sha256: str,
) -> dict[str, object]:
    require_regular(apk, ".apk")
    require_regular(aab, ".aab")
    if not FINGERPRINT_RE.fullmatch(expected_fingerprint):
        raise ValueError("huella Android esperada no válida")
    if not SEMVER_RE.fullmatch(version_name):
        raise ValueError("versionName Android no es SemVer")
    if version_code < 1 or version_code > 2_100_000_000:
        raise ValueError("versionCode Android fuera de rango")
    if not COMMIT_RE.fullmatch(source_commit):
        raise ValueError("sourceCommit Android no válido")
    if not FINGERPRINT_RE.fullmatch(core_sha256):
        raise ValueError("SHA-256 del AAR no válido")
    verify_embedded_core(apk, core_aar, core_sha256)
    signing_block_ids = apk_signing_block_ids(apk)
    if APK_V2_BLOCK_ID not in signing_block_ids or APK_V3_BLOCK_ID not in signing_block_ids:
        raise ValueError("faltan los bloques de firma APK v2 o v3")
    if APK_DEPENDENCY_INFO_BLOCK_ID in signing_block_ids:
        raise ValueError("el APK conserva el bloque de dependencias no reproducible")

    apksigner = find_build_tool("apksigner")
    aapt = find_build_tool("aapt")
    apk_report = run_text(
        [str(apksigner), "verify", "--verbose", "--print-certs", str(apk)]
    )
    match = APK_FINGERPRINT_RE.search(apk_report)
    if not match:
        raise ValueError("apksigner no informó la huella SHA-256")
    required_schemes = (
        "Verified using v1 scheme (JAR signing): false",
        "Verified using v2 scheme (APK Signature Scheme v2): true",
        "Verified using v3 scheme (APK Signature Scheme v3): true",
        "Verified using v4 scheme (APK Signature Scheme v4): false",
    )
    if any(line not in apk_report for line in required_schemes):
        raise ValueError("el APK no usa la combinación de firma v2+v3 fijada")
    apk_fingerprint = normalize_fingerprint(match.group(1))
    if apk_fingerprint != expected_fingerprint:
        raise ValueError("la firma del APK no coincide con la huella fijada")

    verify_aab_signature(aab, expected_fingerprint)

    badging = run_text([str(aapt), "dump", "badging", str(apk)])
    package = PACKAGE_RE.search(badging)
    if not package:
        raise ValueError("aapt no pudo leer versión y paquete del APK")
    if (
        package.group(1) != "es.dipgra.grxfirma"
        or package.group(2) != str(version_code)
        or package.group(3) != version_name
    ):
        raise ValueError("metadatos de versión o paquete Android inesperados")
    manifest = run_text([str(aapt), "dump", "xmltree", str(apk), "AndroidManifest.xml"])
    if (
        "es.dipgra.grxfirma.SOURCE_COMMIT" not in manifest
        or source_commit not in manifest.lower()
        or "es.dipgra.grxfirma.CORE_SHA256" not in manifest
        or core_sha256 not in manifest.lower()
    ):
        raise ValueError("el APK no contiene sourceCommit y AAR exactos")

    artifacts = []
    for kind, path in (("apk", apk), ("aab", aab)):
        artifacts.append(
            {
                "kind": kind,
                "path": path.name,
                "sha256": sha256(path),
                "size_bytes": path.stat().st_size,
            }
        )
    return {
        "schema_version": 1,
        "package": "es.dipgra.grxfirma",
        "version_name": version_name,
        "version_code": version_code,
        "source_commit": source_commit,
        "core_aar_sha256": core_sha256,
        "signing_cert_sha256": expected_fingerprint,
        "apk_signing_block_ids": [
            f"0x{identifier:08x}" for identifier in sorted(signing_block_ids)
        ],
        "artifacts": artifacts,
    }


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apk", type=pathlib.Path, required=True)
    parser.add_argument("--aab", type=pathlib.Path, required=True)
    parser.add_argument("--core-aar", type=pathlib.Path, required=True)
    parser.add_argument("--signing-cert-sha256", required=True)
    parser.add_argument("--version-name", required=True)
    parser.add_argument("--version-code", type=int, required=True)
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--core-sha256", required=True)
    parser.add_argument("--output", type=pathlib.Path)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        evidence = verify(
            args.apk.resolve(),
            args.aab.resolve(),
            args.core_aar.resolve(),
            normalize_fingerprint(args.signing_cert_sha256),
            args.version_name,
            args.version_code,
            args.source_commit.lower(),
            args.core_sha256.lower(),
        )
        encoded = json.dumps(evidence, indent=2, sort_keys=True) + "\n"
        if args.output:
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(encoded, encoding="utf-8")
        else:
            print(encoded, end="")
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"ERROR: verificación de release Android falló: {error}", file=sys.stderr)
        return 1
    print(
        "Artefactos Android firmados y trazables: "
        f"{args.version_name} {args.source_commit.lower()}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
