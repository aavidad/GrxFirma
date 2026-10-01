#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import os
from pathlib import Path
import pwd
import stat
import subprocess
import tempfile
import unittest
from unittest import mock

import native_messaging_e2e as e2e


class FirefoxLaunchTests(unittest.TestCase):
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

            with mock.patch.object(e2e, "run_command", side_effect=run_command):
                source = e2e.prepare_native_host(Path("/usr/bin/go"), output)

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

            with mock.patch.object(e2e, "run_command", side_effect=run_command):
                with self.assertRaisesRegex(e2e.E2EError, "production"):
                    e2e.prepare_native_host(Path("/usr/bin/go"), output)

    def test_enables_system_access_for_current_marionette(self) -> None:
        self.assertEqual(
            e2e.firefox_launch_arguments(headed=True),
            ["-remote-allow-system-access"],
        )
        self.assertEqual(
            e2e.firefox_launch_arguments(headed=False),
            ["-remote-allow-system-access", "-headless"],
        )

    def test_native_manifest_uses_login_home_not_environment_home(self) -> None:
        login_home = Path(pwd.getpwuid(os.getuid()).pw_dir)
        self.assertEqual(e2e.NATIVE_MANIFEST.parents[2], login_home)


class PrivilegedNavigationTests(unittest.TestCase):
    def new_client(self, fail_execute: bool = False) -> tuple[e2e.WebDriverClient, list]:
        client = e2e.WebDriverClient(
            Path("/usr/bin/geckodriver"),
            Path("/tmp/geckodriver.log"),
            firefox=None,
            headed=False,
        )
        calls: list[tuple[str, str, object | None]] = []

        def request(method: str, suffix: str, payload: object | None = None) -> None:
            calls.append((method, suffix, payload))
            if fail_execute and suffix == "/execute/sync":
                raise e2e.E2EError("fallo simulado")

        client.session_request = request  # type: ignore[method-assign]
        return client, calls

    def test_opens_extension_url_through_chrome_context(self) -> None:
        client, calls = self.new_client()
        url = "moz-extension://test/popup.html"

        client.open_extension_page(url)

        self.assertEqual(calls[0], ("POST", "/moz/context", {"context": "chrome"}))
        self.assertEqual(calls[1][0:2], ("POST", "/execute/sync"))
        execute_payload = calls[1][2]
        self.assertIsInstance(execute_payload, dict)
        assert isinstance(execute_payload, dict)
        self.assertEqual(execute_payload["args"], [url])
        self.assertIn("openTrustedLinkIn", execute_payload["script"])
        self.assertEqual(calls[2], ("POST", "/moz/context", {"context": "content"}))

    def test_restores_content_context_when_navigation_fails(self) -> None:
        client, calls = self.new_client(fail_execute=True)

        with self.assertRaisesRegex(e2e.E2EError, "fallo simulado"):
            client.open_extension_page("moz-extension://test/popup.html")

        self.assertEqual(calls[-1], ("POST", "/moz/context", {"context": "content"}))


class ManifestRegistrationTests(unittest.TestCase):
    def test_restores_existing_manifest_bytes_and_mode(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = root / ".mozilla" / "native-messaging-hosts" / "host.json"
            manifest.parent.mkdir(parents=True)
            original = b'{"original":true}\n'
            manifest.write_bytes(original)
            manifest.chmod(0o640)
            host = root / "nativehost"
            host.write_bytes(b"host")

            with mock.patch.object(e2e, "NATIVE_MANIFEST", manifest):
                with e2e.manifest_registration(host):
                    payload = manifest.read_text(encoding="utf-8")
                    self.assertIn(str(host), payload)
                    self.assertEqual(stat.S_IMODE(manifest.stat().st_mode), 0o600)

            self.assertEqual(manifest.read_bytes(), original)
            self.assertEqual(stat.S_IMODE(manifest.stat().st_mode), 0o640)

    def test_removes_manifest_created_for_test(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = root / ".mozilla" / "native-messaging-hosts" / "host.json"
            host = root / "nativehost"
            host.write_bytes(b"host")

            with mock.patch.object(e2e, "NATIVE_MANIFEST", manifest):
                with e2e.manifest_registration(host):
                    self.assertTrue(manifest.is_file())

            self.assertFalse(manifest.exists())

    def test_rejects_existing_manifest_symlink(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest = root / ".mozilla" / "native-messaging-hosts" / "host.json"
            manifest.parent.mkdir(parents=True)
            target = root / "target.json"
            target.write_text('{"original":true}\n', encoding="utf-8")
            manifest.symlink_to(target)
            host = root / "nativehost"
            host.write_bytes(b"host")

            with mock.patch.object(e2e, "NATIVE_MANIFEST", manifest):
                with self.assertRaisesRegex(e2e.E2EError, "fichero regular"):
                    with e2e.manifest_registration(host):
                        self.fail("el contexto no debe abrirse")

            self.assertTrue(manifest.is_symlink())
            self.assertEqual(target.read_text(encoding="utf-8"), '{"original":true}\n')


if __name__ == "__main__":
    unittest.main()
