# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from scripts.ci.validate_runtime_sbom import (
    RuntimeSbomError,
    validate_android_coverage,
)


class ValidateRuntimeSbomTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        temporary_root = Path(self.temporary_directory.name)
        self.lockfile = temporary_root / "gradle.lockfile"
        self.sbom = temporary_root / "runtime.spdx.json"
        self.lockfile.write_text(
            """\
androidx.example:runtime:1.2.3=productionReleaseRuntimeClasspath
com.example:shared:2.0=productionReleaseRuntimeClasspath,verificationReleaseRuntimeClasspath
""",
            encoding="utf-8",
        )

    def write_sbom(self, purls: list[str]) -> None:
        self.sbom.write_text(
            json.dumps(
                {
                    "spdxVersion": "SPDX-2.3",
                    "packages": [
                        {
                            "name": f"package-{index}",
                            "externalRefs": [
                                {
                                    "referenceCategory": "PACKAGE-MANAGER",
                                    "referenceType": "purl",
                                    "referenceLocator": purl,
                                }
                            ],
                        }
                        for index, purl in enumerate(purls)
                    ],
                }
            ),
            encoding="utf-8",
        )

    def test_accepts_exact_android_runtime_inventory(self) -> None:
        self.write_sbom(
            [
                "pkg:golang/example.org/runtime@v1.0.0",
                "pkg:maven/androidx.example/runtime@1.2.3",
                "pkg:maven/com.example/shared@2.0",
            ]
        )

        self.assertEqual(
            validate_android_coverage(self.sbom, self.lockfile),
            (2, 2),
        )

    def test_missing_runtime_package_fails_closed(self) -> None:
        self.write_sbom(["pkg:maven/androidx.example/runtime@1.2.3"])

        with self.assertRaisesRegex(RuntimeSbomError, "missing:"):
            validate_android_coverage(self.sbom, self.lockfile)

    def test_package_outside_filtered_scope_is_rejected(self) -> None:
        self.write_sbom(
            [
                "pkg:maven/androidx.example/runtime@1.2.3",
                "pkg:maven/com.example/shared@2.0",
                "pkg:maven/com.example/lint-only@9.9",
            ]
        )

        with self.assertRaisesRegex(RuntimeSbomError, "outside runtime scope"):
            validate_android_coverage(self.sbom, self.lockfile)

    def test_empty_lockfile_fails_closed(self) -> None:
        self.lockfile.write_text("# no runtime packages\n", encoding="utf-8")
        self.write_sbom([])

        with self.assertRaisesRegex(RuntimeSbomError, "contains no packages"):
            validate_android_coverage(self.sbom, self.lockfile)


if __name__ == "__main__":
    unittest.main()
