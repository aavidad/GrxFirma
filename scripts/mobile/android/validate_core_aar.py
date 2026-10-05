#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Validate that a gomobile AAR implements the Android production contract."""

from __future__ import annotations

import argparse
import hashlib
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile


REQUIRED_AAR_ENTRIES = (
    "classes.jar",
    "jni/armeabi-v7a/libgojni.so",
    "jni/arm64-v8a/libgojni.so",
    "jni/x86_64/libgojni.so",
)
REQUIRED_CLASSES = (
    "mobilebind/Mobilebind.class",
    "mobilebind/Facade.class",
)
FACADE_METHODS = (
    "mobileContractJSON()",
    "clearSession()",
    "selectCertificateJSON(java.lang.String)",
    "importCertificateJSON(java.lang.String)",
    "importCertificateBytesJSON(byte[], java.lang.String)",
    "importCertificateSecretBytesJSON(byte[], byte[])",
    "signJSON(java.lang.String)",
    "sealPreviewJSON(java.lang.String)",
    "verifyJSON(java.lang.String)",
    "inspectSignatureJSON(java.lang.String)",
    "processBatchJSON(java.lang.String)",
    "createHashJSON(java.lang.String)",
    "checkHashJSON(java.lang.String)",
    "protectJSON(java.lang.String, byte[])",
    "unprotectJSON(java.lang.String, byte[])",
    "validateVeriFactuJSON(java.lang.String)",
    "createENIDocumentJSON(java.lang.String)",
    "validateENIJSON(java.lang.String)",
    "eniCatalogsJSON()",
    "csvLegendJSON(java.lang.String)",
)


class ValidationError(RuntimeError):
    pass


def sha256(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def validate_archive(aar: pathlib.Path) -> bytes:
    if not aar.is_file():
        raise ValidationError(f"AAR ausente: {aar}")
    try:
        with zipfile.ZipFile(aar) as archive:
            names = set(archive.namelist())
            unsafe = [
                name
                for name in names
                if name.startswith(("/", "\\"))
                or ".." in pathlib.PurePosixPath(name).parts
            ]
            if unsafe:
                raise ValidationError(f"El AAR contiene rutas inseguras: {unsafe[:3]}")
            missing = [name for name in REQUIRED_AAR_ENTRIES if name not in names]
            if missing:
                raise ValidationError(
                    f"Faltan entradas obligatorias en el AAR: {', '.join(missing)}"
                )
            for name in REQUIRED_AAR_ENTRIES[1:]:
                if archive.getinfo(name).file_size == 0:
                    raise ValidationError(f"La biblioteca nativa está vacía: {name}")
            return archive.read("classes.jar")
    except zipfile.BadZipFile as error:
        raise ValidationError("El AAR no es un ZIP válido") from error


def validate_classes_jar(classes_jar: bytes) -> None:
    with tempfile.TemporaryDirectory(prefix="grxfirma-aar-") as temp:
        jar_path = pathlib.Path(temp, "classes.jar")
        jar_path.write_bytes(classes_jar)
        try:
            with zipfile.ZipFile(jar_path) as archive:
                names = set(archive.namelist())
        except zipfile.BadZipFile as error:
            raise ValidationError("classes.jar no es válido") from error
        missing = [name for name in REQUIRED_CLASSES if name not in names]
        if missing:
            raise ValidationError(
                f"Faltan clases gomobile obligatorias: {', '.join(missing)}"
            )

        javap = java_tool("javap")
        if not javap:
            raise ValidationError("javap no está disponible; use un JDK 17 completo")
        result = subprocess.run(
            [
                javap,
                "-classpath",
                str(jar_path),
                "-public",
                "mobilebind.Mobilebind",
                "mobilebind.Facade",
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            raise ValidationError(
                f"javap no pudo inspeccionar el AAR: {result.stderr.strip()}"
            )
        validate_javap_output(result.stdout)


def java_tool(name: str) -> str | None:
    java_home = os.environ.get("JAVA_HOME")
    if java_home:
        candidate = pathlib.Path(java_home, "bin", name)
        if candidate.is_file():
            return str(candidate)
    return shutil.which(name)


def validate_javap_output(output: str) -> None:
    compact = re.sub(r"\s+", " ", output)
    factory = "newAndroidFacade(java.lang.String, java.lang.String)"
    if factory not in compact:
        raise ValidationError(
            "Falta mobilebind.Mobilebind.newAndroidFacade(String,String); "
            "la fachada Go aún no monta los servicios Android reales."
        )
    missing = [method for method in FACADE_METHODS if method not in compact]
    if missing:
        raise ValidationError(
            f"Faltan métodos del contrato mobile v2: {', '.join(missing)}"
        )


def validate(aar: pathlib.Path, expected_sha256: str | None) -> str:
    if not aar.is_file():
        raise ValidationError(f"AAR ausente: {aar}")
    actual = sha256(aar)
    if expected_sha256 is not None:
        expected = expected_sha256.strip().lower()
        if not re.fullmatch(r"[0-9a-f]{64}", expected):
            raise ValidationError(
                "El SHA-256 esperado debe contener exactamente 64 hexadecimales"
            )
        if actual != expected:
            raise ValidationError(
                f"SHA-256 no coincide: esperado={expected} actual={actual}"
            )
    validate_classes_jar(validate_archive(aar))
    return actual


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--aar", required=True, type=pathlib.Path)
    parser.add_argument("--sha256")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        actual = validate(args.aar.resolve(), args.sha256)
    except ValidationError as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print(f"AAR Android válido: {args.aar} sha256={actual}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
