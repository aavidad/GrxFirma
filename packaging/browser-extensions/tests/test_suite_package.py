# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contratos de identidad y origen de las extensiones en las suites."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
import zipfile


ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "packaging/browser-extensions"))
from verify_package import verify_directory  # noqa: E402


class SuitePackageTests(unittest.TestCase):
    def make_packages(self, directory: Path, host: str = "io.github.aavidad.grxfirma", prefix: str = "grxfirma-") -> None:
        for filename in ("grxfirma-extension-chromium.zip", "grxfirma-extension-firefox.xpi"):
            with zipfile.ZipFile(directory / filename, "w") as archive:
                archive.writestr("manifest.json", json.dumps({"version": "1.1.0"}))
                archive.writestr("background.js", f'const HOST = "{host}";')
                archive.writestr("content_scripts/identity_bridge.js", f'const PREFIX = "{prefix}";')
        xpi = directory / "grxfirma-extension-firefox.xpi"
        (directory / "grxfirma-extension-firefox.metadata.json").write_text(
            json.dumps({"version": "1.1.0", "xpi_sha256": hashlib.sha256(xpi.read_bytes()).hexdigest()}),
            encoding="utf-8",
        )

    def test_rejects_previous_host_and_prefix(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            self.make_packages(directory)
            verify_directory(directory)
            for host, prefix in (("com.dipgra.autofirma", "grxfirma-"), ("io.github.aavidad.grxfirma", "autofirmav2-")):
                self.make_packages(directory, host, prefix)
                with self.assertRaises(ValueError):
                    verify_directory(directory)

    def test_rejects_identifiers_of_previous_namespace(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            for legacy in ("com.dipgra.grxfirma", "extension@dipgra.es"):
                self.make_packages(directory, f'io.github.aavidad.grxfirma"; const OLD = "{legacy}')
                with self.subTest(legacy=legacy), self.assertRaises(ValueError):
                    verify_directory(directory)

    def test_rejects_legacy_files(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            self.make_packages(directory)
            with zipfile.ZipFile(directory / "grxfirma-extension-chromium.zip", "a") as archive:
                archive.writestr("signer/signer.js", "// heredado")
            with self.assertRaises(ValueError):
                verify_directory(directory)

    def test_all_suite_builds_generate_and_check_stage_packages(self) -> None:
        for relative in (
            "packaging/linux/build-suite.sh",
            "packaging/windows/build-suite.sh",
            "packaging/windows/build-suite.ps1",
        ):
            with self.subTest(relative=relative):
                script = (ROOT / relative).read_text(encoding="utf-8")
                self.assertIn("browser-extensions/build.py", script)
                self.assertIn("--output-dir", script)
                self.assertIn("browser-extensions/verify_package.py", script)
                self.assertNotIn('packaging/browser-extensions/grxfirma-extension-chromium.zip', script)
                self.assertNotRegex(script, r"(?m)^(?:build_chromium_extension_assets|Build-ChromiumExtensionAsset)[ \t]+")
        linux = (ROOT / "packaging/linux/build-suite.sh").read_text(encoding="utf-8")
        self.assertNotIn('"external_crx"', linux)
        nsis = (ROOT / "packaging/windows/grxfirma-suite.nsi").read_text(encoding="utf-8")
        self.assertIn('File /r "${STAGE_DIR}\\extensions"', nsis)


if __name__ == "__main__":
    unittest.main()
