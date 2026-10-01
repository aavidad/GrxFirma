#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Assemble and validate the public artifact set for an official release."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shutil
from collections import defaultdict
from pathlib import Path, PurePosixPath


def windows_files(version: str) -> set[str]:
    if not VERSION_RE.fullmatch(version):
        raise ValueError(f"invalid artifact version: {version!r}")
    return {
        f"GrxFirma-{version}-desktop-qml-windows-amd64.zip",
        f"GrxFirma-{version}-desktop-qml-windows-amd64-setup.exe",
        f"GrxFirma-{version}-windows-amd64.zip",
        f"GrxFirma-{version}-windows-amd64-setup.exe",
        "WINDOWS-SIGNATURES.json",
        "SHA256SUMS-windows.txt",
    }


def expected_linux_files(version: str) -> set[str]:
    return {
        f"GrxFirma-{version}-linux-amd64.tar.gz",
        f"grxfirma_{version}_amd64.deb",
    }


def expected_android_files(version: str) -> set[str]:
    return {
        f"GrxFirma-{version}-android.apk",
        f"GrxFirma-{version}-android.aab",
    }


ROOT_METADATA = {
    "ARTIFACTS.md",
    "manifest.json",
    "release-metadata.json",
    "RELEASE-SIGNING-KEY.asc",
    "SHA256SUMS.txt",
    "SHA256SUMS.txt.asc",
}
SAFE_NAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+-]*$")
CHECKSUM_RE = re.compile(r"^([0-9a-fA-F]{64}) [ *](.+)$")
TAG_RE = re.compile(r"^v[0-9A-Za-z.+-]+$")
VERSION_RE = re.compile(r"^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def regular_files(directory: Path) -> list[Path]:
    if not directory.is_dir():
        raise ValueError(f"missing artifact directory: {directory}")
    entries = list(directory.iterdir())
    invalid = [path for path in entries if path.is_symlink() or not path.is_file()]
    if invalid:
        raise ValueError(
            f"only regular files are allowed in artifact directory: {invalid}"
        )
    return sorted(entries)


def validate_source_file(path: Path) -> None:
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"artifact is not a regular file: {path}")
    if path.stat().st_size == 0:
        raise ValueError(f"empty artifact: {path}")
    if not SAFE_NAME_RE.fullmatch(path.name):
        raise ValueError(f"unsafe public artifact name: {path.name!r}")


def copy_files(files: list[Path], destination: Path) -> None:
    destination.mkdir(parents=True, exist_ok=True)
    for source in files:
        validate_source_file(source)
        target = destination / source.name
        if target.exists():
            raise ValueError(f"duplicate artifact destination: {target}")
        shutil.copy2(source, target)


def command_copy(args: argparse.Namespace) -> None:
    dist = args.dist.resolve()
    output = args.output.resolve()
    expected_windows = windows_files(args.version)
    if output.name != "installers" or output == dist:
        raise ValueError(
            "official output must be a dedicated directory named 'installers'"
        )
    if output.exists():
        shutil.rmtree(output)
    for name in ("linux", "windows", "macos", "android", "security"):
        (output / name).mkdir(parents=True, exist_ok=True)

    linux_source = dist / "linux-installers"
    linux_files = regular_files(linux_source)
    expected_linux = expected_linux_files(args.version)
    if {path.name for path in linux_files} != expected_linux:
        raise ValueError("unexpected Linux build artifacts")
    copy_files(linux_files, output / "linux")

    windows_source = dist / "windows-installers"
    windows_sources = regular_files(windows_source)
    windows_by_name = {path.name: path for path in windows_sources}
    if set(windows_by_name) != expected_windows:
        raise ValueError(
            "unexpected Windows artifact set: "
            f"missing={sorted(expected_windows - set(windows_by_name))} "
            f"extra={sorted(set(windows_by_name) - expected_windows)}"
        )
    copy_files(
        [windows_by_name[name] for name in sorted(expected_windows)], output / "windows"
    )

    macos_source = dist / "macos-installers"
    macos_files = regular_files(macos_source)
    macos_packages = [path for path in macos_files if path.suffix == ".pkg"]
    macos_evidence = [
        path for path in macos_files if path.name == "MACOS-NOTARIZATION.json"
    ]
    if (
        len(macos_packages) != 1
        or len(macos_evidence) != 1
        or set(macos_files) != set(macos_packages + macos_evidence)
    ):
        raise ValueError(
            "official macOS artifacts must contain exactly one notarized .pkg and its evidence"
        )
    copy_files(macos_packages + macos_evidence, output / "macos")

    android_source = dist / "android-installers"
    android_files = regular_files(android_source)
    expected_android = expected_android_files(args.version)
    android_packages = [
        path for path in android_files if path.name in expected_android
    ]
    android_evidence = [
        path for path in android_files if path.name == "ANDROID-SIGNATURES.json"
    ]
    if (
        len(android_packages) != 2
        or len(android_evidence) != 1
        or set(android_files)
        != set(android_packages + android_evidence)
    ):
        raise ValueError(
            "official Android artifacts must contain exactly one APK, one AAB and signature evidence"
        )
    copy_files(android_packages + android_evidence, output / "android")

    security_source = dist / "sbom-artifact"
    security_files = regular_files(security_source)
    spdx_files = [path for path in security_files if path.name.endswith(".spdx.json")]
    if not spdx_files or set(security_files) != set(spdx_files):
        raise ValueError(
            "security artifacts must contain only non-empty SPDX JSON files"
        )
    copy_files(spdx_files, output / "security")

    print(f"Official artifact candidates copied to {output}")


def payload_files(root: Path) -> list[Path]:
    return sorted(
        path
        for platform in ("linux", "windows", "macos", "android", "security")
        for path in (root / platform).iterdir()
        if path.is_file()
    )


def command_metadata(args: argparse.Namespace) -> None:
    root = args.root.resolve()
    if not TAG_RE.fullmatch(args.tag):
        raise ValueError(f"invalid release tag in metadata: {args.tag!r}")
    if not re.fullmatch(r"[0-9a-fA-F]{40}", args.commit):
        raise ValueError("release commit must be a full 40-character Git object id")
    if args.ref != f"refs/tags/{args.tag}":
        raise ValueError("release ref does not match the release tag")
    entries = []
    for path in payload_files(root):
        validate_source_file(path)
        entries.append(
            {
                "path": path.relative_to(root).as_posix(),
                "sha256": sha256(path),
                "size_bytes": path.stat().st_size,
            }
        )

    manifest = {"artifacts": entries, "tag": args.tag}
    (root / "manifest.json").write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    metadata = {
        "git_commit": args.commit,
        "ref": args.ref,
        "release_assets_root": "release/installers",
        "repository": args.repository,
        "run_attempt": args.run_attempt,
        "run_id": args.run_id,
        "tag": args.tag,
        "workflow": args.workflow,
    }
    (root / "release-metadata.json").write_text(
        json.dumps(metadata, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )

    report = [
        "# Release Artifacts",
        "",
        f"Tag: {args.tag}",
        "",
        "| Path | Size (bytes) | SHA-256 |",
        "| --- | ---: | --- |",
    ]
    for entry in entries:
        report.append(
            f"| `{entry['path']}` | {entry['size_bytes']} | `{entry['sha256']}` |"
        )
    (root / "ARTIFACTS.md").write_text("\n".join(report) + "\n", encoding="utf-8")

    if args.summary_file:
        summary = [
            f"# Release {args.tag}",
            "",
            f"Artifacts: {len(entries)}",
            "",
            *report[4:],
        ]
        args.summary_file.write_text("\n".join(summary) + "\n", encoding="utf-8")


def parse_checksum_file(path: Path, base: Path) -> dict[str, str]:
    entries: dict[str, str] = {}
    for line_number, line in enumerate(
        path.read_text(encoding="ascii").splitlines(), start=1
    ):
        match = CHECKSUM_RE.fullmatch(line)
        if not match:
            raise ValueError(f"invalid checksum line {path}:{line_number}")
        digest, raw_name = match.groups()
        name = raw_name.removeprefix("./")
        pure = PurePosixPath(name)
        if pure.is_absolute() or ".." in pure.parts or not name:
            raise ValueError(f"unsafe checksum path in {path}: {raw_name!r}")
        if name in entries:
            raise ValueError(f"duplicate checksum path in {path}: {name}")
        artifact = base / pure
        if not artifact.is_file() or artifact.is_symlink():
            raise ValueError(f"checksum references missing or unsafe file: {artifact}")
        actual = sha256(artifact)
        if actual != digest.lower():
            raise ValueError(f"checksum mismatch: {artifact}")
        entries[name] = digest.lower()
    return entries


def assert_checksum_coverage(
    checksum_path: Path, base: Path, expected_paths: set[str]
) -> None:
    actual_paths = set(parse_checksum_file(checksum_path, base))
    if actual_paths != expected_paths:
        raise ValueError(
            f"checksum coverage mismatch for {checksum_path}: "
            f"missing={sorted(expected_paths - actual_paths)} "
            f"extra={sorted(actual_paths - expected_paths)}"
        )


def command_validate(args: argparse.Namespace) -> None:
    root = args.root.resolve()
    if not root.is_dir():
        raise ValueError(f"missing release root: {root}")

    for path in root.rglob("*"):
        if path.is_symlink():
            raise ValueError(f"public symlinks are not allowed: {path}")
        if path.is_file() and (
            path.stat().st_size == 0 or not SAFE_NAME_RE.fullmatch(path.name)
        ):
            raise ValueError(f"empty or unsafe public file: {path}")

    root_files = {path.name for path in root.iterdir() if path.is_file()}
    if root_files != ROOT_METADATA:
        raise ValueError(
            f"unexpected root metadata: missing={sorted(ROOT_METADATA - root_files)} "
            f"extra={sorted(root_files - ROOT_METADATA)}"
        )

    release_tag = json.loads(
        (root / "release-metadata.json").read_text(encoding="utf-8")
    ).get("tag", "")
    expected_windows = windows_files(release_tag.removeprefix("v"))
    expected_linux = expected_linux_files(release_tag.removeprefix("v"))
    expected_android_names = expected_android_files(release_tag.removeprefix("v"))

    allowed = {
        "linux": lambda name: (
            name in expected_linux
            or name in {"SHA256SUMS-linux.txt", "SHA256SUMS-linux.txt.asc"}
        ),
        "windows": lambda name: name in expected_windows,
        "macos": lambda name: (
            name.endswith(".pkg") or name == "MACOS-NOTARIZATION.json"
        ),
        "android": lambda name: (
            name in expected_android_names or name == "ANDROID-SIGNATURES.json"
        ),
        "security": lambda name: name.endswith(".spdx.json"),
    }
    for platform, predicate in allowed.items():
        directory = root / platform
        files = regular_files(directory)
        if not files or any(not predicate(path.name) for path in files):
            raise ValueError(f"unexpected or empty {platform} artifact set")
    if {
        path.name for path in regular_files(root / "linux")
        if path.suffix == ".deb" or path.name.endswith(".tar.gz")
    } != expected_linux:
        raise ValueError("incomplete Linux artifact set")
    if {
        path.name for path in regular_files(root / "android")
        if path.suffix in {".apk", ".aab"}
    } != expected_android_names:
        raise ValueError("incomplete Android artifact set")

    all_files = sorted(path for path in root.rglob("*") if path.is_file())
    by_name: dict[str, list[str]] = defaultdict(list)
    for path in all_files:
        by_name[path.name].append(path.relative_to(root).as_posix())
    duplicates = {name: paths for name, paths in by_name.items() if len(paths) > 1}
    if duplicates:
        raise ValueError(f"duplicate public artifact names: {duplicates}")

    manifest = json.loads((root / "manifest.json").read_text(encoding="utf-8"))
    manifest_entries = manifest.get("artifacts", [])
    manifest_by_path = {entry["path"]: entry for entry in manifest_entries}
    if len(manifest_by_path) != len(manifest_entries):
        raise ValueError("manifest contains duplicate artifact paths")
    release_metadata = json.loads(
        (root / "release-metadata.json").read_text(encoding="utf-8")
    )
    if (
        manifest.get("tag") != release_metadata.get("tag")
        or release_metadata.get("ref") != f"refs/tags/{manifest.get('tag')}"
        or not re.fullmatch(r"[0-9a-fA-F]{40}", release_metadata.get("git_commit", ""))
    ):
        raise ValueError("release metadata tag, ref or commit is inconsistent")
    expected_payload = {
        path.relative_to(root).as_posix() for path in payload_files(root)
    }
    if set(manifest_by_path) != expected_payload:
        raise ValueError("manifest does not cover the complete platform payload")
    for relative, entry in manifest_by_path.items():
        path = root / relative
        if (
            entry.get("sha256") != sha256(path)
            or entry.get("size_bytes") != path.stat().st_size
        ):
            raise ValueError(f"manifest metadata mismatch: {relative}")

    windows_evidence = json.loads(
        (root / "windows" / "WINDOWS-SIGNATURES.json").read_text(encoding="utf-8")
    )
    windows_containers = windows_evidence.get("containers", [])
    windows_signatures = windows_evidence.get("signatures", [])
    if (
        windows_evidence.get("schema_version") != 2
        or not re.fullmatch(
            r"[0-9A-F]{40}", windows_evidence.get("expected_signer_thumbprint", "")
        )
        or {item.get("path") for item in windows_containers}
        != {name for name in expected_windows if name.endswith((".zip", ".exe"))}
        or any(
            not re.fullmatch(r"[0-9a-f]{64}", item.get("sha256", ""))
            for item in windows_containers
        )
        or not windows_signatures
        or len({item.get("path") for item in windows_signatures})
        != len(windows_signatures)
        or any(
            item.get("status") != "Valid"
            or not item.get("file_digest_algorithm")
            or not item.get("timestamp_protocol")
            or not re.fullmatch(r"[0-9A-F]{40}", item.get("signer_thumbprint", ""))
            or not re.fullmatch(r"[0-9A-F]{40}", item.get("timestamp_thumbprint", ""))
            or not re.fullmatch(r"[0-9a-f]{64}", item.get("sha256", ""))
            or (
                item.get("signer_thumbprint")
                == windows_evidence.get("expected_signer_thumbprint")
                and (
                    item.get("file_digest_algorithm") != "SHA256"
                    or item.get("timestamp_protocol") != "RFC3161"
                    or item.get("timestamp_digest_algorithm") != "SHA256"
                )
            )
            for item in windows_signatures
        )
    ):
        raise ValueError("WINDOWS-SIGNATURES.json is incomplete or invalid")
    for container in windows_containers:
        container_path = root / "windows" / container["path"]
        if container["sha256"] != sha256(container_path):
            raise ValueError(
                f"Windows signature evidence hash mismatch: {container['path']}"
            )

    macos_evidence = json.loads(
        (root / "macos" / "MACOS-NOTARIZATION.json").read_text(encoding="utf-8")
    )
    macos_package = next((root / "macos").glob("*.pkg"))
    if (
        macos_evidence.get("status") != "Accepted"
        or not macos_evidence.get("id")
        or macos_evidence.get("package") != macos_package.name
        or macos_evidence.get("sha256") != sha256(macos_package)
        or macos_evidence.get("size_bytes") != macos_package.stat().st_size
    ):
        raise ValueError(
            "MACOS-NOTARIZATION.json does not prove an accepted submission"
        )

    android_evidence = json.loads(
        (root / "android" / "ANDROID-SIGNATURES.json").read_text(encoding="utf-8")
    )
    android_artifacts = android_evidence.get("artifacts", [])
    android_by_path = {
        item.get("path"): item
        for item in android_artifacts
        if isinstance(item, dict) and item.get("path")
    }
    expected_android = {
        path.name
        for path in regular_files(root / "android")
        if path.name != "ANDROID-SIGNATURES.json"
    }
    if (
        android_evidence.get("schema_version") != 1
        or android_evidence.get("package") != "es.dipgra.grxfirma"
        or not VERSION_RE.fullmatch(android_evidence.get("version_name", ""))
        or android_evidence.get("version_name")
        != str(release_metadata.get("tag", "")).removeprefix("v")
        or type(android_evidence.get("version_code")) is not int
        or android_evidence.get("version_code", 0) < 1
        or android_evidence.get("version_code", 0) > 2_100_000_000
        or android_evidence.get("source_commit") != release_metadata.get("git_commit")
        or not re.fullmatch(
            r"[0-9a-f]{64}", android_evidence.get("core_aar_sha256", "")
        )
        or not re.fullmatch(
            r"[0-9a-f]{64}", android_evidence.get("signing_cert_sha256", "")
        )
        or set(android_by_path) != expected_android
        or len(android_by_path) != len(android_artifacts)
    ):
        raise ValueError("ANDROID-SIGNATURES.json is incomplete or invalid")
    for relative, item in android_by_path.items():
        artifact = root / "android" / relative
        expected_kind = artifact.suffix.removeprefix(".")
        if (
            expected_kind not in {"apk", "aab"}
            or item.get("kind") != expected_kind
            or item.get("sha256") != sha256(artifact)
            or item.get("size_bytes") != artifact.stat().st_size
        ):
            raise ValueError(f"Android signature evidence mismatch: {relative}")

    for sbom in (root / "security").glob("*.spdx.json"):
        sbom_data = json.loads(sbom.read_text(encoding="utf-8"))
        if not str(sbom_data.get("spdxVersion", "")).startswith(
            "SPDX-"
        ) or not sbom_data.get("SPDXID"):
            raise ValueError(f"invalid SPDX JSON document: {sbom.name}")

    linux = root / "linux"
    linux_checksum = linux / "SHA256SUMS-linux.txt"
    linux_expected = {
        path.name
        for path in regular_files(linux)
        if path.name not in {"SHA256SUMS-linux.txt", "SHA256SUMS-linux.txt.asc"}
    }
    assert_checksum_coverage(linux_checksum, linux, linux_expected)

    windows = root / "windows"
    windows_checksum = windows / "SHA256SUMS-windows.txt"
    windows_expected = {
        path.name
        for path in regular_files(windows)
        if path.name != windows_checksum.name
    }
    assert_checksum_coverage(windows_checksum, windows, windows_expected)

    root_checksum = root / "SHA256SUMS.txt"
    root_expected = {
        path.relative_to(root).as_posix()
        for path in all_files
        if path.name not in {"SHA256SUMS.txt", "SHA256SUMS.txt.asc"}
    }
    assert_checksum_coverage(root_checksum, root, root_expected)
    print(
        f"Official release artifact set validated: {len(expected_payload)} payload files"
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

    copy_parser = subparsers.add_parser("copy")
    copy_parser.add_argument("--dist", type=Path, required=True)
    copy_parser.add_argument("--output", type=Path, required=True)
    copy_parser.add_argument("--version", required=True)
    copy_parser.set_defaults(func=command_copy)

    metadata_parser = subparsers.add_parser("metadata")
    metadata_parser.add_argument("--root", type=Path, required=True)
    metadata_parser.add_argument("--tag", required=True)
    metadata_parser.add_argument("--commit", required=True)
    metadata_parser.add_argument("--repository", required=True)
    metadata_parser.add_argument("--workflow", required=True)
    metadata_parser.add_argument("--run-id", required=True)
    metadata_parser.add_argument("--run-attempt", required=True)
    metadata_parser.add_argument("--ref", required=True)
    metadata_parser.add_argument("--summary-file", type=Path)
    metadata_parser.set_defaults(func=command_metadata)

    validate_parser = subparsers.add_parser("validate")
    validate_parser.add_argument("--root", type=Path, required=True)
    validate_parser.set_defaults(func=command_validate)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        args.func(args)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        raise SystemExit(f"error: {error}") from error
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
