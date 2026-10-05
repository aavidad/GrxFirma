#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "scripts" / "release" / "release_artifacts.py"


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write_checksums(directory: Path, name: str, excluded: set[str]) -> None:
    lines = []
    for path in sorted(item for item in directory.rglob("*") if item.is_file()):
        relative = path.relative_to(directory).as_posix()
        if relative in excluded:
            continue
        lines.append(f"{digest(path)}  ./{relative}")
    (directory / name).write_text("\n".join(lines) + "\n", encoding="ascii")


class ReleaseArtifactsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.dist = self.root / "dist"
        self.output = self.root / "release" / "installers"
        for name in (
            "linux-installers",
            "windows-installers",
            "macos-installers",
            "android-installers",
            "sbom-artifact",
        ):
            (self.dist / name).mkdir(parents=True)

        (self.dist / "linux-installers" / "GrxFirma-1.2.3-linux-amd64.tar.gz").write_bytes(b"tar")
        (self.dist / "linux-installers" / "grxfirma_1.2.3_amd64.deb").write_bytes(b"deb")
        windows = self.dist / "windows-installers"
        for name in (
            "GrxFirma-1.2.3-desktop-qml-windows-amd64.zip",
            "GrxFirma-1.2.3-desktop-qml-windows-amd64-setup.exe",
            "GrxFirma-1.2.3-windows-amd64.zip",
            "GrxFirma-1.2.3-windows-amd64-setup.exe",
        ):
            (windows / name).write_text(name, encoding="utf-8")
        (windows / "WINDOWS-SIGNATURES.json").write_text(
            json.dumps(
                {
                    "schema_version": 2,
                    "expected_signer_thumbprint": "A" * 40,
                    "containers": [
                        {"path": name, "sha256": digest(windows / name)}
                        for name in (
                            "GrxFirma-1.2.3-desktop-qml-windows-amd64.zip",
                            "GrxFirma-1.2.3-desktop-qml-windows-amd64-setup.exe",
                            "GrxFirma-1.2.3-windows-amd64.zip",
                            "GrxFirma-1.2.3-windows-amd64-setup.exe",
                        )
                    ],
                    "signatures": [
                        {
                            "path": "signed.exe",
                            "sha256": "c" * 64,
                            "status": "Valid",
                            "signer_thumbprint": "A" * 40,
                            "timestamp_thumbprint": "E" * 40,
                            "file_digest_algorithm": "SHA256",
                            "timestamp_protocol": "RFC3161",
                            "timestamp_digest_algorithm": "SHA256",
                        }
                    ],
                }
            )
            + "\n",
            encoding="utf-8",
        )
        write_checksums(windows, "SHA256SUMS-windows.txt", set())
        macos_package = self.dist / "macos-installers" / "suite.pkg"
        macos_package.write_bytes(b"pkg")
        (self.dist / "macos-installers" / "MACOS-NOTARIZATION.json").write_text(
            json.dumps(
                {
                    "id": "test-submission",
                    "package": macos_package.name,
                    "sha256": digest(macos_package),
                    "size_bytes": macos_package.stat().st_size,
                    "status": "Accepted",
                }
            )
            + "\n",
            encoding="utf-8",
        )
        android = self.dist / "android-installers"
        android_apk = android / "GrxFirma-1.2.3-android.apk"
        android_aab = android / "GrxFirma-1.2.3-android.aab"
        android_apk.write_bytes(b"apk")
        android_aab.write_bytes(b"aab")
        (android / "ANDROID-SIGNATURES.json").write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "package": "io.github.aavidad.grxfirma",
                    "version_name": "1.2.3",
                    "version_code": 1_002_003,
                    "source_commit": "a" * 40,
                    "core_aar_sha256": "b" * 64,
                    "signing_cert_sha256": "c" * 64,
                    "artifacts": [
                        {
                            "kind": kind,
                            "path": path.name,
                            "sha256": digest(path),
                            "size_bytes": path.stat().st_size,
                        }
                        for kind, path in (("apk", android_apk), ("aab", android_aab))
                    ],
                }
            )
            + "\n",
            encoding="utf-8",
        )
        (self.dist / "sbom-artifact" / "suite.spdx.json").write_text(
            json.dumps({"spdxVersion": "SPDX-2.3", "SPDXID": "SPDXRef-DOCUMENT"})
            + "\n",
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.temp.cleanup()

    def run_script(
        self, *args: str, expect_success: bool = True
    ) -> subprocess.CompletedProcess[str]:
        result = subprocess.run(
            [sys.executable, str(SCRIPT), *args],
            check=False,
            capture_output=True,
            text=True,
        )
        if expect_success and result.returncode != 0:
            self.fail(result.stderr or result.stdout)
        return result

    def prepare_complete_release(self) -> None:
        self.run_script("copy", "--dist", str(self.dist), "--output", str(self.output), "--version", "1.2.3")
        linux = self.output / "linux"
        write_checksums(linux, "SHA256SUMS-linux.txt", set())
        (linux / "SHA256SUMS-linux.txt.asc").write_text("signature\n", encoding="ascii")
        self.run_script(
            "metadata",
            "--root",
            str(self.output),
            "--tag",
            "v1.2.3",
            "--commit",
            "a" * 40,
            "--repository",
            "example/grxfirma",
            "--workflow",
            "Release",
            "--run-id",
            "1",
            "--run-attempt",
            "1",
            "--ref",
            "refs/tags/v1.2.3",
        )
        (self.output / "RELEASE-SIGNING-KEY.asc").write_text(
            "public key\n", encoding="ascii"
        )
        write_checksums(
            self.output,
            "SHA256SUMS.txt",
            {"SHA256SUMS.txt", "SHA256SUMS.txt.asc"},
        )
        (self.output / "SHA256SUMS.txt.asc").write_text("signature\n", encoding="ascii")

    def test_complete_release_is_accepted_and_tampering_is_rejected(self) -> None:
        self.prepare_complete_release()
        self.run_script("validate", "--root", str(self.output))

        package = self.output / "macos" / "suite.pkg"
        package.write_bytes(b"tampered")
        result = self.run_script(
            "validate", "--root", str(self.output), expect_success=False
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("manifest metadata mismatch", result.stderr)

    def test_non_notarized_macos_tar_is_rejected_from_official_input(self) -> None:
        (self.dist / "macos-installers" / "suite.tar.gz").write_bytes(b"tar")
        result = self.run_script(
            "copy",
            "--version",
            "1.2.3",
            "--dist",
            str(self.dist),
            "--output",
            str(self.output),
            expect_success=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exactly one notarized .pkg and its evidence", result.stderr)

    def test_android_evidence_must_cover_exact_apk_and_aab(self) -> None:
        evidence_path = (
            self.dist / "android-installers" / "ANDROID-SIGNATURES.json"
        )
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["artifacts"][0]["sha256"] = "0" * 64
        evidence_path.write_text(json.dumps(evidence) + "\n", encoding="utf-8")
        self.prepare_complete_release()
        result = self.run_script(
            "validate", "--root", str(self.output), expect_success=False
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Android signature evidence mismatch", result.stderr)

    def test_android_version_must_match_release_tag(self) -> None:
        evidence_path = (
            self.dist / "android-installers" / "ANDROID-SIGNATURES.json"
        )
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["version_name"] = "9.9.9"
        evidence_path.write_text(json.dumps(evidence) + "\n", encoding="utf-8")
        self.prepare_complete_release()
        result = self.run_script(
            "validate", "--root", str(self.output), expect_success=False
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ANDROID-SIGNATURES.json is incomplete", result.stderr)

    def test_android_kind_must_match_each_extension(self) -> None:
        evidence_path = (
            self.dist / "android-installers" / "ANDROID-SIGNATURES.json"
        )
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        for artifact in evidence["artifacts"]:
            artifact["kind"] = "apk"
        evidence_path.write_text(json.dumps(evidence) + "\n", encoding="utf-8")
        self.prepare_complete_release()
        result = self.run_script(
            "validate", "--root", str(self.output), expect_success=False
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Android signature evidence mismatch", result.stderr)

    def test_project_authenticode_requires_sha256_rfc3161(self) -> None:
        windows = self.dist / "windows-installers"
        evidence_path = windows / "WINDOWS-SIGNATURES.json"
        evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
        evidence["signatures"][0]["timestamp_protocol"] = "LEGACY"
        evidence_path.write_text(
            json.dumps(evidence) + "\n",
            encoding="utf-8",
        )
        write_checksums(windows, "SHA256SUMS-windows.txt", set())
        self.prepare_complete_release()
        result = self.run_script(
            "validate",
            "--root",
            str(self.output),
            expect_success=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(
            "WINDOWS-SIGNATURES.json is incomplete or invalid",
            result.stderr,
        )

    def test_copy_refuses_a_non_dedicated_output_directory(self) -> None:
        unsafe = self.root / "do-not-delete"
        unsafe.mkdir()
        marker = unsafe / "marker.txt"
        marker.write_text("preserve\n", encoding="utf-8")
        result = self.run_script(
            "copy",
            "--version",
            "1.2.3",
            "--dist",
            str(self.dist),
            "--output",
            str(unsafe),
            expect_success=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(marker.is_file())


if __name__ == "__main__":
    unittest.main()
