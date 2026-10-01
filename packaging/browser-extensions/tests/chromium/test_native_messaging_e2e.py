# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import importlib.util
import json
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest
from unittest import mock
import zipfile


MODULE_PATH = Path(__file__).with_name("native_messaging_e2e.py")
SPEC = importlib.util.spec_from_file_location("chromium_native_messaging_e2e", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
E2E = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = E2E
SPEC.loader.exec_module(E2E)


class Worker:
    def __init__(self, url: str) -> None:
        self.url = url


class ChromiumHarnessTests(unittest.TestCase):
    def test_native_host_build_and_metadata_require_production(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "nativehost"
            calls: list[list[str]] = []

            def run_command(
                command: list[str],
                **_kwargs: object,
            ) -> subprocess.CompletedProcess[str]:
                calls.append(command)
                if command[1:3] == ["version", "-m"]:
                    return subprocess.CompletedProcess(
                        command,
                        0,
                        stdout="\tbuild\t-tags=production\n",
                    )
                output.write_bytes(b"production host")
                return subprocess.CompletedProcess(command, 0, stdout="")

            with mock.patch.object(E2E, "run_command", side_effect=run_command):
                source = E2E.prepare_native_host(Path("/usr/bin/go"), output)

            self.assertEqual(source, "production-build")
            self.assertIn("-tags=production", calls[0])
            self.assertEqual(calls[1][1:3], ["version", "-m"])

    def test_native_host_rejects_missing_production_metadata(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "nativehost"

            def run_command(
                command: list[str],
                **_kwargs: object,
            ) -> subprocess.CompletedProcess[str]:
                if command[1:3] == ["version", "-m"]:
                    return subprocess.CompletedProcess(command, 0, stdout="")
                output.write_bytes(b"development host")
                return subprocess.CompletedProcess(command, 0, stdout="")

            with mock.patch.object(E2E, "run_command", side_effect=run_command):
                with self.assertRaisesRegex(E2E.E2EError, "production"):
                    E2E.prepare_native_host(Path("/usr/bin/go"), output)

    def test_service_worker_must_belong_to_an_extension(self) -> None:
        self.assertTrue(
            E2E.is_extension_service_worker(
                Worker("chrome-extension://abcdefghijkl/background.js")
            )
        )
        self.assertFalse(
            E2E.is_extension_service_worker(
                Worker("https://example.invalid/background.js")
            )
        )
        self.assertFalse(
            E2E.is_extension_service_worker(
                Worker("chrome-extension://abcdefghijkl/other.js")
            )
        )

    def test_extract_rejects_parent_traversal(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            package = root / "extension.zip"
            destination = root / "extension"
            with zipfile.ZipFile(package, "w") as archive:
                archive.writestr("../escape", b"not allowed")

            with self.assertRaisesRegex(E2E.E2EError, "ruta insegura"):
                E2E.extract_package(package, destination)
            self.assertFalse((root / "escape").exists())

    def test_atomic_manifest_is_private_and_complete(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            manifest = Path(directory) / "NativeMessagingHosts" / "host.json"
            payload = {"name": E2E.HOST_NAME, "path": "/tmp/nativehost"}

            E2E.atomic_write_json(manifest, payload)

            self.assertEqual(
                stat.S_IMODE(manifest.stat().st_mode),
                0o600,
            )
            self.assertEqual(json.loads(manifest.read_text(encoding="utf-8")), payload)


if __name__ == "__main__":
    unittest.main()
