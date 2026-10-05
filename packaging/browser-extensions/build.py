#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Genera los artefactos versionados de las extensiones de navegador."""

from __future__ import annotations

from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile


ROOT = Path(__file__).resolve().parent
SRC_DIR = ROOT / "src"

CHROMIUM_ZIP = ROOT / "dipgra-extension-chromium.zip"
FIREFOX_XPI = ROOT / "dipgra-extension-firefox.xpi"
FIREFOX_UNSIGNED_XPI = ROOT / "dipgra-extension-firefox-unsigned.xpi"
FIREFOX_METADATA = ROOT / "dipgra-extension-firefox.metadata.json"

EXCLUDED_NAMES = {
    ".DS_Store",
    "Thumbs.db",
    "dipgra_extension_v1.zip",
}

EXCLUDED_DIR_NAMES = {
    "__MACOSX",
    ".git",
}

EXCLUDED_RELATIVE_PATHS = {
    "PRIVACY_POLICY.md",
    "config.example.js",
    "crypto_utils.js",
    "content_scripts/autologin.js",
    "content_scripts/session_sync.js",
    "version.json",
    "alarm16.png",
    "alarm48.png",
    "alarm128.png",
    "icons/icon256.png",
    "icons/icon512.png",
}

EXCLUDED_PREFIXES = (
    "signer/",
)

FORBIDDEN_PACKAGE_PATHS = {
    "crypto_utils.js",
    "content_scripts/autologin.js",
    "content_scripts/session_sync.js",
}
FORBIDDEN_PACKAGE_PREFIXES = ("signer/",)

ZIP_MIN_EPOCH = 315532800  # 1980-01-01, limite inferior del formato ZIP.
ZIP_MAX_EPOCH = 4354819198  # 2107-12-31 23:59:58 UTC.


def iter_source_files(source_dir: Path):
    for path in sorted(source_dir.rglob("*")):
        if path.is_dir():
            continue
        rel = path.relative_to(source_dir)
        rel_str = rel.as_posix()
        if any(part in EXCLUDED_DIR_NAMES for part in rel.parts):
            continue
        if path.name in EXCLUDED_NAMES:
            continue
        if rel_str in EXCLUDED_RELATIVE_PATHS:
            continue
        if any(rel_str.startswith(prefix) for prefix in EXCLUDED_PREFIXES):
            continue
        yield path, rel


def validate_manifest(source_dir: Path) -> dict:
    manifest_path = source_dir / "manifest.json"
    if not manifest_path.is_file():
        raise FileNotFoundError(f"Falta {manifest_path}")
    with manifest_path.open("r", encoding="utf-8") as fh:
        data = json.load(fh)
    version = data.get("version")
    if not version or not isinstance(version, str):
        raise ValueError(f"manifest.json sin version valida en {source_dir}")
    return data


def validate_config(source_dir: Path) -> None:
    """Impide distribuir credenciales REST legibles dentro de la extensión."""
    config_path = source_dir / "config.js"
    if not config_path.exists():
        return
    config = config_path.read_text(encoding="utf-8")
    bearer = re.findall(r"\bLOCAL_REST_BEARER\s*:\s*([^,}\n]+)", config)
    if config.count("LOCAL_REST_BEARER") != 1 or len(bearer) != 1 or bearer[0].strip() not in ('""', "''"):
        raise ValueError(f"{config_path}: LOCAL_REST_BEARER debe estar vacio")


def validate_package_paths(paths: set[str]) -> None:
    forbidden = FORBIDDEN_PACKAGE_PATHS & paths
    forbidden.update(
        path for path in paths
        if any(path.startswith(prefix) for prefix in FORBIDDEN_PACKAGE_PREFIXES)
    )
    if forbidden:
        raise ValueError(f"El paquete incluye ficheros heredados: {sorted(forbidden)}")


def resolve_source_date_epoch() -> int:
    raw = os.environ.get("SOURCE_DATE_EPOCH", "").strip()
    if not raw:
        try:
            raw = subprocess.check_output(
                ["git", "-C", str(ROOT.parents[1]), "log", "-1", "--format=%ct"],
                text=True,
                stderr=subprocess.DEVNULL,
            ).strip()
        except (OSError, subprocess.CalledProcessError):
            raw = "0"
    try:
        epoch = int(raw, 10)
    except ValueError as exc:
        raise ValueError("SOURCE_DATE_EPOCH debe ser un entero no negativo") from exc
    if epoch < 0:
        raise ValueError("SOURCE_DATE_EPOCH debe ser un entero no negativo")
    if epoch > ZIP_MAX_EPOCH:
        raise ValueError("SOURCE_DATE_EPOCH excede el rango representable por ZIP")
    return max(epoch, ZIP_MIN_EPOCH)


def normalized_zip_datetime() -> tuple[int, int, int, int, int, int]:
    timestamp = datetime.fromtimestamp(resolve_source_date_epoch(), timezone.utc)
    return (
        timestamp.year,
        timestamp.month,
        timestamp.day,
        timestamp.hour,
        timestamp.minute,
        timestamp.second - (timestamp.second % 2),
    )


def build_archive(source_dir: Path, output_path: Path) -> None:
    validate_manifest(source_dir)
    validate_config(source_dir)
    source_files = list(iter_source_files(source_dir))
    validate_package_paths({rel.as_posix() for _, rel in source_files})
    output_path.parent.mkdir(parents=True, exist_ok=True)
    tmp_output = output_path.with_suffix(output_path.suffix + ".tmp")
    if tmp_output.exists():
        tmp_output.unlink()
    if output_path.exists():
        output_path.unlink()

    archive_datetime = normalized_zip_datetime()
    with zipfile.ZipFile(tmp_output, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for full_path, rel_path in source_files:
            archive_name = str(rel_path).replace(os.sep, "/")
            info = zipfile.ZipInfo(archive_name, date_time=archive_datetime)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.create_system = 3
            info.external_attr = 0o100644 << 16
            zf.writestr(info, full_path.read_bytes())

    tmp_output.replace(output_path)


def materialize_filtered_source(source_dir: Path, target_dir: Path) -> None:
    validate_manifest(source_dir)
    validate_config(source_dir)
    source_files = list(iter_source_files(source_dir))
    validate_package_paths({rel.as_posix() for _, rel in source_files})
    for full_path, rel_path in source_files:
        dest = target_dir / rel_path
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(full_path, dest)


def validate_signed_firefox_xpi(
    path: Path, source_manifest: dict, source_dir: Path | None = None
) -> None:
    with zipfile.ZipFile(path) as zf:
        listed_names = zf.namelist()
        names = set(listed_names)
        if len(names) != len(listed_names):
            raise ValueError(f"el XPI firmado contiene rutas duplicadas: {path}")
        signature_files = {
            name.lower()
            for name in names
            if name.lower().startswith("meta-inf/")
        }
        if not any(
            name.endswith(".rsa") or name.endswith("/cose.sig")
            for name in signature_files
        ):
            raise ValueError(f"el XPI indicado no contiene firma Mozilla: {path}")
        try:
            packaged_manifest = json.loads(zf.read("manifest.json"))
        except (KeyError, json.JSONDecodeError) as exc:
            raise ValueError(f"el XPI firmado no contiene manifest.json valido: {path}") from exc

        if source_dir is not None:
            expected = {
                relative.as_posix(): full_path.read_bytes()
                for full_path, relative in iter_source_files(source_dir)
            }
            actual_names = {
                name for name in names
                if not name.endswith("/") and not name.lower().startswith("meta-inf/")
            }
            if actual_names != set(expected):
                raise ValueError("el contenido del XPI firmado no coincide con la fuente filtrada")
            for name, source_bytes in expected.items():
                if zf.read(name) != source_bytes:
                    raise ValueError(f"el XPI firmado difiere de la fuente filtrada: {name}")

    source_gecko = source_manifest.get("browser_specific_settings", {}).get("gecko", {})
    packaged_gecko = packaged_manifest.get("browser_specific_settings", {}).get("gecko", {})
    if packaged_manifest.get("version") != source_manifest.get("version"):
        raise ValueError("la version del XPI firmado no coincide con la fuente")
    if packaged_gecko.get("id") != source_gecko.get("id"):
        raise ValueError("el ID Gecko del XPI firmado no coincide con la fuente")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_firefox_metadata(
    source_dir: Path, artifact: Path, signed: bool, source: str,
    metadata_path: Path | None = None,
) -> None:
    manifest = validate_manifest(source_dir)
    gecko = manifest.get("browser_specific_settings", {}).get("gecko", {})
    metadata = {
        "extension_id": gecko.get("id", ""),
        "signed": signed,
        "source": source,
        "version": manifest["version"],
        "xpi_sha256": sha256_file(artifact),
    }
    (metadata_path or FIREFOX_METADATA).write_text(
        json.dumps(metadata, ensure_ascii=True, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )


def sign_or_stage_firefox_xpi(source_dir: Path, output_dir: Path = ROOT) -> None:
    unsigned_xpi = output_dir / FIREFOX_UNSIGNED_XPI.name
    firefox_xpi = output_dir / FIREFOX_XPI.name
    firefox_metadata = output_dir / FIREFOX_METADATA.name
    build_archive(source_dir, unsigned_xpi)
    if firefox_metadata.exists():
        firefox_metadata.unlink()
    source_manifest = validate_manifest(source_dir)

    signed_xpi = os.environ.get("GRXFIRMA_FIREFOX_SIGNED_XPI", "").strip()
    if signed_xpi:
        signed_path = Path(signed_xpi).expanduser().resolve()
        if not signed_path.is_file():
            raise FileNotFoundError(f"GRXFIRMA_FIREFOX_SIGNED_XPI no existe: {signed_path}")
        validate_signed_firefox_xpi(signed_path, source_manifest, source_dir)
        shutil.copy2(signed_path, firefox_xpi)
        write_firefox_metadata(source_dir, firefox_xpi, True, "prebuilt", firefox_metadata)
        print(f"Usando XPI Firefox firmado: {firefox_xpi}")
        return

    api_key = os.environ.get("WEB_EXT_API_KEY", "").strip()
    api_secret = os.environ.get("WEB_EXT_API_SECRET", "").strip()
    if api_key and api_secret and shutil.which("web-ext"):
        with tempfile.TemporaryDirectory() as tmp:
            artifacts = Path(tmp)
            filtered_source = artifacts / "source"
            materialize_filtered_source(source_dir, filtered_source)
            signed_artifacts = artifacts / "signed"
            signed_artifacts.mkdir(parents=True, exist_ok=True)
            subprocess.run(
                [
                    "web-ext",
                    "sign",
                    "--source-dir",
                    str(filtered_source),
                    "--artifacts-dir",
                    str(signed_artifacts),
                    "--channel",
                    "unlisted",
                    "--api-key",
                    api_key,
                    "--api-secret",
                    api_secret,
                ],
                check=True,
            )
            signed = sorted(signed_artifacts.glob("*.xpi"))
            if not signed:
                raise RuntimeError("web-ext sign no produjo ningun .xpi firmado")
            validate_signed_firefox_xpi(signed[-1], source_manifest, source_dir)
            shutil.copy2(signed[-1], firefox_xpi)
            write_firefox_metadata(source_dir, firefox_xpi, True, "amo-unlisted", firefox_metadata)
            print(f"Generado XPI Firefox firmado: {firefox_xpi}")
            return

    if os.environ.get("GRXFIRMA_REQUIRE_SIGNED_FIREFOX_XPI") == "1":
        raise RuntimeError(
            "Firefox requiere XPI firmado. Define GRXFIRMA_FIREFOX_SIGNED_XPI "
            "o WEB_EXT_API_KEY/WEB_EXT_API_SECRET con web-ext instalado."
        )

    shutil.copy2(unsigned_xpi, firefox_xpi)
    write_firefox_metadata(source_dir, firefox_xpi, False, "development", firefox_metadata)
    print(
        "Aviso: generado XPI Firefox sin firmar para desarrollo. "
        "Firefox Release/ESR exige GRXFIRMA_REQUIRE_SIGNED_FIREFOX_XPI=1 en release.",
        file=sys.stderr,
    )


def main(argv: list[str] | None = None) -> int:
    import argparse

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, default=ROOT,
                        help="Directorio para los paquetes generados")
    args = parser.parse_args(argv)
    output_dir = args.output_dir.resolve()
    chromium_dir = SRC_DIR / "chromium"
    firefox_dir = SRC_DIR / "firefox"

    chromium_zip = output_dir / CHROMIUM_ZIP.name
    build_archive(chromium_dir, chromium_zip)
    print(f"Generado: {chromium_zip}")
    sign_or_stage_firefox_xpi(firefox_dir, output_dir)
    print(f"Generado: {output_dir / FIREFOX_UNSIGNED_XPI.name}")
    print(f"Generado: {output_dir / FIREFOX_XPI.name}")
    print(f"Generado: {output_dir / FIREFOX_METADATA.name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
