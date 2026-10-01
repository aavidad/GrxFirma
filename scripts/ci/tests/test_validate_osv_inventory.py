# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from scripts.ci.validate_osv_inventory import InventoryError, validate_inventory


class ValidateOSVInventoryTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        self.root = Path(self.temporary_directory.name)
        self.report = self.root / "osv.json"

    def write_report(self, sources: list[Path]) -> None:
        payload = {
            "results": [
                {
                    "source": {"path": str(source), "type": "lockfile"},
                    "packages": [],
                }
                for source in sources
            ]
        }
        self.report.write_text(json.dumps(payload), encoding="utf-8")

    def test_accepts_exact_gated_manifest_set(self) -> None:
        go_mod = self.root / "go.mod"
        gradle_lock = self.root / "mobile" / "gradle.lockfile"
        self.write_report([go_mod, gradle_lock])

        validate_inventory(
            self.report,
            self.root,
            ["go.mod", "mobile/gradle.lockfile"],
        )

    def test_rejects_new_ungated_manifest(self) -> None:
        self.write_report([self.root / "go.mod", self.root / "package-lock.json"])

        with self.assertRaisesRegex(InventoryError, "new ungated"):
            validate_inventory(self.report, self.root, ["go.mod"])

    def test_rejects_missing_expected_manifest(self) -> None:
        self.write_report([self.root / "go.mod"])

        with self.assertRaisesRegex(InventoryError, "missing expected"):
            validate_inventory(
                self.report,
                self.root,
                ["go.mod", "third_party/pdfsign/go.mod"],
            )

    def test_rejects_source_outside_repository(self) -> None:
        outside = self.root.parent / "outside-gradle.lockfile"
        self.write_report([outside])

        with self.assertRaisesRegex(InventoryError, "outside the repository"):
            validate_inventory(self.report, self.root, ["go.mod"])


if __name__ == "__main__":
    unittest.main()
