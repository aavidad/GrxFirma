# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from scripts.ci.filter_gradle_lockfile import LockfileError, filter_lockfile


class FilterGradleLockfileTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        self.lockfile = Path(self.temporary_directory.name) / "gradle.lockfile"

    def write_lockfile(self, contents: str) -> None:
        self.lockfile.write_text(contents, encoding="utf-8")

    def test_keeps_only_requested_runtime_configurations(self) -> None:
        self.write_lockfile(
            """\
# Gradle generated
example:runtime:1.0=productionReleaseRuntimeClasspath,androidLintTool
example:shared:2.0=productionReleaseRuntimeClasspath,verificationReleaseRuntimeClasspath
example:tooling:3.0=androidLintTool
empty=unusedConfiguration
"""
        )

        result = filter_lockfile(
            self.lockfile,
            [
                "productionReleaseRuntimeClasspath",
                "verificationReleaseRuntimeClasspath",
            ],
        )

        self.assertIn(
            "example:runtime:1.0=productionReleaseRuntimeClasspath",
            result,
        )
        self.assertIn(
            "example:shared:2.0=productionReleaseRuntimeClasspath,"
            "verificationReleaseRuntimeClasspath",
            result,
        )
        self.assertNotIn("androidLintTool", result)
        self.assertNotIn("example:tooling", result)

    def test_missing_configuration_fails_closed(self) -> None:
        self.write_lockfile(
            "example:runtime:1.0=productionReleaseRuntimeClasspath\n"
        )

        with self.assertRaisesRegex(
            LockfileError,
            "verificationReleaseRuntimeClasspath",
        ):
            filter_lockfile(
                self.lockfile,
                [
                    "productionReleaseRuntimeClasspath",
                    "verificationReleaseRuntimeClasspath",
                ],
            )

    def test_malformed_entry_is_rejected(self) -> None:
        self.write_lockfile("not-a-gradle-lock-entry\n")

        with self.assertRaisesRegex(LockfileError, "malformed"):
            filter_lockfile(
                self.lockfile,
                ["productionReleaseRuntimeClasspath"],
            )

    def test_repeated_configuration_is_rejected(self) -> None:
        self.write_lockfile(
            "example:runtime:1.0=productionReleaseRuntimeClasspath\n"
        )

        with self.assertRaisesRegex(LockfileError, "must not be repeated"):
            filter_lockfile(
                self.lockfile,
                [
                    "productionReleaseRuntimeClasspath",
                    "productionReleaseRuntimeClasspath",
                ],
            )


if __name__ == "__main__":
    unittest.main()
