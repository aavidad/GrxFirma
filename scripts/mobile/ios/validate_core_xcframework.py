#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Validate the binary and Objective-C contract of Mobilebind.xcframework."""

from __future__ import annotations

import argparse
import hashlib
import os
import pathlib
import plistlib
import re
import shutil
import subprocess
import sys


REQUIRED_HEADER_TOKENS = (
    "MobilebindNewIOSFacade",
    "mobileContractJSON",
    "clearSession",
    "selectCertificateJSON",
    "importCertificateJSON",
    "signJSON",
    "verifyJSON",
    "resolvePlatformProfileJSON",
)
REQUIRED_HEADER_PATTERNS = (
    r"MobilebindNewIOSFacade\s*\([^;]*applicationSupportDir[^;]*appGroupDir[^;]*keychainAccessGroup[^;]*NSError",
    r"mobileContractJSON\s*;",
    r"clearSession\s*;",
    r"selectCertificateJSON\s*:[^;]*\berror\s*:",
    r"importCertificateJSON\s*:[^;]*\berror\s*:",
    r"signJSON\s*:[^;]*\berror\s*:",
    r"verifyJSON\s*:[^;]*\berror\s*:",
    r"resolvePlatformProfileJSON\s*:[^;]*NSError",
)
MAX_FRAMEWORK_BYTES = 250 * 1024 * 1024
APPLE_BINARY_MAGICS = {
    b"\xfe\xed\xfa\xcf",
    b"\xcf\xfa\xed\xfe",
    b"\xca\xfe\xba\xbe",
    b"\xbe\xba\xfe\xca",
    b"\xca\xfe\xba\xbf",
    b"\xbf\xba\xfe\xca",
}


class ValidationError(RuntimeError):
    pass


def canonical_digest(root: pathlib.Path) -> str:
    digest = hashlib.sha256()
    total = 0
    for path in sorted(
        root.rglob("*"), key=lambda item: item.relative_to(root).as_posix()
    ):
        relative = path.relative_to(root).as_posix()
        digest.update(relative.encode("utf-8") + b"\0")
        if path.is_symlink():
            target = os.readlink(path)
            if os.path.isabs(target) or ".." in pathlib.PurePosixPath(target).parts:
                raise ValidationError(
                    f"Enlace simbólico inseguro: {relative} -> {target}"
                )
            digest.update(b"L\0" + target.encode("utf-8") + b"\0")
        elif path.is_file():
            size = path.stat().st_size
            total += size
            if total > MAX_FRAMEWORK_BYTES:
                raise ValidationError("El XCFramework supera 250 MiB")
            digest.update(b"F\0" + str(size).encode("ascii") + b"\0")
            with path.open("rb") as handle:
                for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                    digest.update(chunk)
        elif path.is_dir():
            digest.update(b"D\0")
        else:
            raise ValidationError(f"Tipo de entrada no admitido: {relative}")
    return digest.hexdigest()


def load_plist(path: pathlib.Path) -> dict:
    try:
        with path.open("rb") as handle:
            value = plistlib.load(handle)
    except (OSError, plistlib.InvalidFileException) as error:
        raise ValidationError(f"Plist inválido: {path}") from error
    if not isinstance(value, dict):
        raise ValidationError(f"El plist no contiene un diccionario: {path}")
    return value


def safe_child(root: pathlib.Path, relative: str) -> pathlib.Path:
    pure = pathlib.PurePosixPath(relative)
    if pure.is_absolute() or ".." in pure.parts:
        raise ValidationError(f"Ruta interna insegura: {relative}")
    result = root.joinpath(*pure.parts)
    try:
        result.resolve(strict=True).relative_to(root.resolve(strict=True))
    except (FileNotFoundError, ValueError) as error:
        raise ValidationError(
            f"Ruta interna ausente o fuera del XCFramework: {relative}"
        ) from error
    return result


def inspect_binary(binary: pathlib.Path, declared_architectures: set[str]) -> None:
    if binary.stat().st_size < 4096:
        raise ValidationError(f"Binario vacío o anormalmente pequeño: {binary}")
    with binary.open("rb") as handle:
        prefix = handle.read(8)
    if prefix[:4] not in APPLE_BINARY_MAGICS and prefix != b"!<arch>\n":
        raise ValidationError(f"El binario no tiene formato Mach-O/archive: {binary}")
    if sys.platform != "darwin":
        return
    xcrun = shutil.which("xcrun")
    if not xcrun:
        raise ValidationError("xcrun no está disponible en macOS")
    result = subprocess.run(
        [xcrun, "lipo", "-archs", str(binary)],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise ValidationError(
            f"lipo no pudo inspeccionar {binary}: {result.stderr.strip()}"
        )
    actual = set(result.stdout.split())
    if actual != declared_architectures:
        raise ValidationError(
            f"Arquitecturas reales {sorted(actual)} no coinciden con Info.plist {sorted(declared_architectures)}"
        )
    symbols = subprocess.run(
        [xcrun, "nm", "-gU", str(binary)],
        check=False,
        capture_output=True,
        text=True,
    )
    if symbols.returncode != 0 or "MobilebindNewIOSFacade" not in symbols.stdout:
        raise ValidationError("El binario no exporta MobilebindNewIOSFacade")


def validate_slice(root: pathlib.Path, library: dict) -> tuple[str, str]:
    identifier = library.get("LibraryIdentifier")
    library_path = library.get("LibraryPath")
    architectures = library.get("SupportedArchitectures")
    platform = library.get("SupportedPlatform")
    variant = library.get("SupportedPlatformVariant", "device")
    if not isinstance(identifier, str) or not isinstance(library_path, str):
        raise ValidationError("Una slice no declara identificador y ruta")
    if platform != "ios" or variant not in {"device", "simulator"}:
        raise ValidationError(f"Slice Apple no admitida: {identifier}")
    if (
        not isinstance(architectures, list)
        or not architectures
        or not all(isinstance(item, str) for item in architectures)
    ):
        raise ValidationError(f"Arquitecturas inválidas en {identifier}")

    framework = safe_child(root, f"{identifier}/{library_path}")
    if framework.suffix != ".framework" or not framework.is_dir():
        raise ValidationError(f"La slice no contiene un framework: {framework}")
    name = framework.stem
    if name != "Mobilebind":
        raise ValidationError(f"Nombre de framework inesperado: {name}")
    header = safe_child(framework, "Headers/Mobilebind.h")
    modulemap = safe_child(framework, "Modules/module.modulemap")
    binary = safe_child(framework, "Mobilebind")
    text = header.read_text(encoding="utf-8")
    missing = [token for token in REQUIRED_HEADER_TOKENS if token not in text]
    if missing:
        raise ValidationError(
            f"Falta contrato Objective-C en {identifier}: {', '.join(missing)}"
        )
    missing_signatures = [
        pattern
        for pattern in REQUIRED_HEADER_PATTERNS
        if not re.search(pattern, text, re.DOTALL)
    ]
    if missing_signatures:
        raise ValidationError(f"Firmas Objective-C incompatibles en {identifier}")
    if "context.Context" in text or "internal/" in text:
        raise ValidationError("La cabecera filtra tipos internos de Go")
    module_text = modulemap.read_text(encoding="utf-8")
    if not re.search(r"framework\s+module\s+Mobilebind\b", module_text):
        raise ValidationError(f"module.modulemap no declara Mobilebind: {identifier}")
    framework_plist = load_plist(safe_child(framework, "Info.plist"))
    if framework_plist.get("CFBundlePackageType") != "FMWK":
        raise ValidationError(f"CFBundlePackageType inválido en {identifier}")
    inspect_binary(binary, set(architectures))
    return variant, hashlib.sha256(header.read_bytes()).hexdigest()


def validate(root: pathlib.Path, expected_digest: str | None) -> str:
    if not root.is_dir() or root.suffix != ".xcframework":
        raise ValidationError(f"XCFramework ausente o inválido: {root}")
    info = load_plist(root / "Info.plist")
    if info.get("XCFrameworkFormatVersion") != "1.0":
        raise ValidationError("XCFrameworkFormatVersion debe ser 1.0")
    libraries = info.get("AvailableLibraries")
    if not isinstance(libraries, list) or len(libraries) < 2:
        raise ValidationError("El XCFramework debe contener device y simulator")

    variants: dict[str, list[dict]] = {"device": [], "simulator": []}
    header_digests: set[str] = set()
    for library in libraries:
        if not isinstance(library, dict):
            raise ValidationError("AvailableLibraries contiene una entrada inválida")
        variant, header_digest = validate_slice(root, library)
        variants[variant].append(library)
        header_digests.add(header_digest)
    if len(variants["device"]) != 1 or len(variants["simulator"]) != 1:
        raise ValidationError(
            "Debe existir exactamente una slice iOS device y una simulator"
        )
    device_arch = set(variants["device"][0]["SupportedArchitectures"])
    simulator_arch = set(variants["simulator"][0]["SupportedArchitectures"])
    if device_arch != {"arm64"}:
        raise ValidationError("La slice device debe ser arm64")
    if simulator_arch != {"arm64", "x86_64"}:
        raise ValidationError("La slice simulator debe contener solo arm64 y x86_64")
    if len(header_digests) != 1:
        raise ValidationError("Las slices exponen cabeceras diferentes")

    actual = canonical_digest(root)
    if expected_digest is not None:
        expected = expected_digest.strip().lower()
        if not re.fullmatch(r"[0-9a-f]{64}", expected):
            raise ValidationError("El SHA-256 esperado no tiene 64 hexadecimales")
        if actual != expected:
            raise ValidationError(
                f"SHA-256 del XCFramework no coincide: esperado={expected} actual={actual}"
            )
    return actual


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--xcframework", required=True, type=pathlib.Path)
    expected = parser.add_mutually_exclusive_group()
    expected.add_argument("--sha256")
    expected.add_argument("--sha256-file", type=pathlib.Path)
    parser.add_argument("--write-sha256", type=pathlib.Path)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    expected = args.sha256
    if args.sha256_file:
        try:
            expected = args.sha256_file.read_text(encoding="ascii").split()[0]
        except (OSError, IndexError):
            print(f"ERROR: checksum ilegible: {args.sha256_file}", file=sys.stderr)
            return 1
    try:
        actual = validate(args.xcframework.resolve(), expected)
        if args.write_sha256:
            args.write_sha256.write_text(
                f"{actual}  {args.xcframework.name}\n", encoding="ascii"
            )
    except (ValidationError, OSError) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print(f"XCFramework iOS válido: {args.xcframework} sha256={actual}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
