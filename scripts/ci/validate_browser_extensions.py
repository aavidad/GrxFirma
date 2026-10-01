#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Validate the unpacked contract and reproducibility of browser artifacts."""

from __future__ import annotations

import argparse
import hashlib
import json
import stat
import subprocess
import sys
import zipfile
from html.parser import HTMLParser
from pathlib import Path, PurePosixPath
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
EXTENSION_ROOT = ROOT / "packaging" / "browser-extensions"
SOURCE_ROOT = EXTENSION_ROOT / "src"
BUILD_SCRIPT = EXTENSION_ROOT / "build.py"
FORBIDDEN_SUFFIXES = {".env", ".key", ".p12", ".pfx", ".pem"}
FORBIDDEN_PATHS = {
    "config.example.js",
    "crypto_utils.js",
    "content_scripts/autologin.js",
    "content_scripts/session_sync.js",
    "options.html",
    "options.js",
    "styles.css",
}


class LocalReferenceParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.references: set[str] = set()

    def handle_starttag(self, _tag: str, attrs: list[tuple[str, str | None]]) -> None:
        for name, value in attrs:
            if name not in {"href", "src"} or not value:
                continue
            if value.startswith(("#", "data:", "http://", "https://", "mailto:")):
                continue
            self.references.add(value.split("?", 1)[0].split("#", 1)[0])


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def artifact_paths() -> dict[str, Path]:
    firefox_unsigned = EXTENSION_ROOT / "dipgra-extension-firefox-unsigned.xpi"
    firefox = firefox_unsigned if firefox_unsigned.is_file() else EXTENSION_ROOT / "dipgra-extension-firefox.xpi"
    return {
        "chromium": EXTENSION_ROOT / "dipgra-extension-chromium.zip",
        "firefox": firefox,
    }


def manifest_references(manifest: dict[str, Any]) -> set[str]:
    references: set[str] = set()

    def add(value: Any) -> None:
        if isinstance(value, str) and "*" not in value:
            references.add(value)
        elif isinstance(value, dict):
            for nested in value.values():
                add(nested)
        elif isinstance(value, list):
            for nested in value:
                add(nested)

    add(manifest.get("icons", {}))
    action = manifest.get("action", {})
    if isinstance(action, dict):
        add(action.get("default_icon", {}))
        add(action.get("default_popup"))
    background = manifest.get("background", {})
    if isinstance(background, dict):
        add(background.get("service_worker"))
        add(background.get("scripts", []))
    for entry in manifest.get("content_scripts", []):
        if isinstance(entry, dict):
            add(entry.get("js", []))
            add(entry.get("css", []))
    return references


def validate_archive(browser: str, path: Path) -> tuple[dict[str, Any], dict[str, Any]]:
    if not path.is_file() or path.stat().st_size == 0:
        raise ValueError(f"missing or empty {browser} artifact: {path}")

    warnings: list[str] = []
    with zipfile.ZipFile(path) as archive:
        infos = archive.infolist()
        names = [info.filename for info in infos]
        if len(names) != len(set(names)):
            raise ValueError(f"{browser}: archive contains duplicate entries")
        if archive.testzip() is not None:
            raise ValueError(f"{browser}: archive CRC validation failed")

        for info in infos:
            member = PurePosixPath(info.filename)
            if member.is_absolute() or ".." in member.parts or "\\" in info.filename:
                raise ValueError(f"{browser}: unsafe archive path: {info.filename}")
            file_type = (info.external_attr >> 16) & 0o170000
            if file_type == stat.S_IFLNK:
                raise ValueError(f"{browser}: symbolic links are not allowed: {info.filename}")
            if member.suffix.lower() in FORBIDDEN_SUFFIXES:
                raise ValueError(f"{browser}: secret-bearing file type in archive: {info.filename}")
            if info.filename in FORBIDDEN_PATHS or info.filename.startswith("signer/"):
                raise ValueError(f"{browser}: development-only file in archive: {info.filename}")

        if "manifest.json" not in names:
            raise ValueError(f"{browser}: manifest.json is missing")
        manifest = json.loads(archive.read("manifest.json"))
        source_manifest = json.loads((SOURCE_ROOT / browser / "manifest.json").read_text(encoding="utf-8"))
        if manifest != source_manifest:
            raise ValueError(f"{browser}: packaged manifest differs from source")
        if manifest.get("manifest_version") != 3:
            raise ValueError(f"{browser}: only Manifest V3 is supported")
        if not isinstance(manifest.get("version"), str) or not manifest["version"]:
            raise ValueError(f"{browser}: invalid extension version")

        available = set(names)
        missing = sorted(reference for reference in manifest_references(manifest) if reference not in available)
        if missing:
            raise ValueError(f"{browser}: manifest references missing files: {missing}")

        for html_name in (name for name in names if name.endswith(".html")):
            parser = LocalReferenceParser()
            parser.feed(archive.read(html_name).decode("utf-8"))
            html_root = PurePosixPath(html_name).parent
            missing_html = sorted(
                reference
                for reference in parser.references
                if (html_root / reference).as_posix() not in available
            )
            if missing_html:
                raise ValueError(f"{browser}: {html_name} references missing files: {missing_html}")

        all_matches = [
            match
            for entry in manifest.get("content_scripts", [])
            if isinstance(entry, dict)
            for match in entry.get("matches", [])
        ]
        if "*://*/*" in all_matches or "<all_urls>" in all_matches:
            warnings.append("content scripts include a global match pattern")

        host_permissions = manifest.get("host_permissions", [])
        insecure_hosts = [host for host in host_permissions if str(host).startswith("http://")]
        if insecure_hosts:
            warnings.append(f"insecure host permissions: {insecure_hosts}")

        if browser == "chromium":
            if not manifest.get("background", {}).get("service_worker"):
                raise ValueError("chromium: Manifest V3 service worker is missing")
        else:
            gecko = manifest.get("browser_specific_settings", {}).get("gecko", {})
            if not gecko.get("id") or not gecko.get("strict_min_version"):
                raise ValueError("firefox: Gecko id or strict_min_version is missing")

    return manifest, {
        "artifact": str(path.relative_to(ROOT)),
        "bytes": path.stat().st_size,
        "files": len(names),
        "sha256": sha256(path),
        "warnings": warnings,
    }


def check_determinism(paths: dict[str, Path]) -> None:
    before = {name: sha256(path) for name, path in paths.items()}
    subprocess.run([sys.executable, str(BUILD_SCRIPT)], cwd=ROOT, check=True)
    rebuilt = artifact_paths()
    after = {name: sha256(path) for name, path in rebuilt.items()}
    if before != after:
        raise ValueError(f"browser artifacts are not reproducible: before={before}, after={after}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check-determinism", action="store_true")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()

    paths = artifact_paths()
    manifests: dict[str, dict[str, Any]] = {}
    report: dict[str, Any] = {"artifacts": {}}
    for browser, path in paths.items():
        manifest, details = validate_archive(browser, path)
        manifests[browser] = manifest
        report["artifacts"][browser] = details

    for shared_field in ("name", "short_name", "version"):
        if manifests["chromium"].get(shared_field) != manifests["firefox"].get(shared_field):
            raise ValueError(f"browser manifests disagree on {shared_field}")

    if args.check_determinism:
        check_determinism(paths)
        report["reproducible"] = True

    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    for browser, details in report["artifacts"].items():
        print(
            f"{browser}: {details['files']} files, {details['bytes']} bytes, "
            f"sha256={details['sha256']}"
        )
        for warning in details["warnings"]:
            print(f"warning: {browser}: {warning}", file=sys.stderr)
    print("Browser extension validation passed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, zipfile.BadZipFile, json.JSONDecodeError) as error:
        print(f"browser extension validation failed: {error}", file=sys.stderr)
        raise SystemExit(1) from error
