# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Optional


SAFARI_DIR = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "grxfirma_safari_validate_source", SAFARI_DIR / "validate_source.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class SourceValidationTests(unittest.TestCase):
    def manifest(
        self,
        root: Path,
        version: int = 3,
        native: bool = True,
        extra: Optional[dict] = None,
    ) -> Path:
        permissions = ["storage"]
        if native:
            permissions.append("nativeMessaging")
        path = root / "manifest.json"
        payload = {"manifest_version": version, "permissions": permissions}
        payload.update(extra or {})
        path.write_text(json.dumps(payload), encoding="utf-8")
        return path

    def test_accepts_manifest_v3_from_macos_12_3(self):
        with tempfile.TemporaryDirectory() as temporary:
            result = MODULE.validate_manifest(self.manifest(Path(temporary)), "12.3")
            self.assertEqual(result["manifest_version"], 3)
            self.assertEqual(result["minimum_macos"], "12.3.0")

    def test_accepts_current_chromium_source(self):
        source_manifest = SAFARI_DIR.parent / "src" / "chromium" / "manifest.json"
        result = MODULE.validate_manifest(source_manifest, "12.3")
        self.assertEqual(result["manifest_version"], 3)

    def test_rejects_manifest_v3_before_safari_15_4_baseline(self):
        with tempfile.TemporaryDirectory() as temporary:
            with self.assertRaises(MODULE.SourceValidationError):
                MODULE.validate_manifest(self.manifest(Path(temporary)), "12.2")

    def test_accepts_manifest_v2_on_older_macos_baseline(self):
        with tempfile.TemporaryDirectory() as temporary:
            result = MODULE.validate_manifest(
                self.manifest(Path(temporary), version=2), "11.0"
            )
            self.assertEqual(result["manifest_version"], 2)

    def test_rejects_missing_native_messaging_permission(self):
        with tempfile.TemporaryDirectory() as temporary:
            with self.assertRaises(MODULE.SourceValidationError):
                MODULE.validate_manifest(
                    self.manifest(Path(temporary), native=False), "12.3"
                )

    def test_rejects_malformed_minimum_version(self):
        with tempfile.TemporaryDirectory() as temporary:
            with self.assertRaises(MODULE.SourceValidationError):
                MODULE.validate_manifest(self.manifest(Path(temporary)), "latest")

    def test_rejects_broad_host_permission(self):
        with tempfile.TemporaryDirectory() as temporary:
            manifest = self.manifest(
                Path(temporary), extra={"host_permissions": ["https://*/*"]}
            )
            with self.assertRaises(MODULE.SourceValidationError):
                MODULE.validate_manifest(manifest, "12.3")

    def test_rejects_externally_connectable(self):
        with tempfile.TemporaryDirectory() as temporary:
            manifest = self.manifest(
                Path(temporary),
                extra={
                    "externally_connectable": {"matches": ["https://*.dipgra.es/*"]}
                },
            )
            with self.assertRaises(MODULE.SourceValidationError):
                MODULE.validate_manifest(manifest, "12.3")

    def test_rejects_malformed_host_permission_field_cleanly(self):
        with tempfile.TemporaryDirectory() as temporary:
            manifest = self.manifest(
                Path(temporary), extra={"host_permissions": "https://*/*"}
            )
            with self.assertRaises(MODULE.SourceValidationError):
                MODULE.validate_manifest(manifest, "12.3")


if __name__ == "__main__":
    unittest.main()
