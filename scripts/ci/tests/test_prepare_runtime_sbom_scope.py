# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from scripts.ci.prepare_runtime_sbom_scope import (
    ANDROID_LOCKFILE,
    GO_RUNTIME_MANIFESTS,
    RuntimeScopeError,
    prepare_runtime_scope,
)


class PrepareRuntimeSbomScopeTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        temporary_root = Path(self.temporary_directory.name)
        self.root = temporary_root / "source"
        self.output = temporary_root / "runtime"

        for manifest in GO_RUNTIME_MANIFESTS:
            path = self.root / manifest
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(f"fixture for {manifest}\n", encoding="utf-8")

        lockfile = self.root / ANDROID_LOCKFILE
        lockfile.parent.mkdir(parents=True, exist_ok=True)
        lockfile.write_text(
            """\
example:production:1.0=productionReleaseRuntimeClasspath,androidLintTool
example:shared:2.0=productionReleaseRuntimeClasspath,verificationReleaseRuntimeClasspath
example:verification:3.0=verificationReleaseRuntimeClasspath,testRuntimeClasspath
example:lint-only:4.0=androidLintTool
""",
            encoding="utf-8",
        )

    def test_creates_runtime_only_scope_and_attestation(self) -> None:
        attestation = prepare_runtime_scope(self.root, self.output)

        filtered = (self.output / ANDROID_LOCKFILE).read_text(encoding="utf-8")
        self.assertIn("example:production:1.0=productionReleaseRuntimeClasspath", filtered)
        self.assertIn("example:verification:3.0=verificationReleaseRuntimeClasspath", filtered)
        self.assertNotIn("androidLintTool", filtered)
        self.assertNotIn("testRuntimeClasspath", filtered)
        self.assertNotIn("example:lint-only", filtered)
        self.assertEqual(
            attestation["android_configurations"],
            [
                "productionReleaseRuntimeClasspath",
                "verificationReleaseRuntimeClasspath",
            ],
        )
        self.assertEqual(len(attestation["manifests"]), 5)

    def test_missing_required_go_manifest_fails_closed(self) -> None:
        (self.root / "third_party/pdfsign/go.sum").unlink()

        with self.assertRaisesRegex(RuntimeScopeError, "required manifest"):
            prepare_runtime_scope(self.root, self.output)

    def test_missing_android_runtime_configuration_fails_closed(self) -> None:
        (self.root / ANDROID_LOCKFILE).write_text(
            "example:production:1.0=productionReleaseRuntimeClasspath\n",
            encoding="utf-8",
        )

        with self.assertRaisesRegex(
            RuntimeScopeError,
            "verificationReleaseRuntimeClasspath",
        ):
            prepare_runtime_scope(self.root, self.output)

    def test_refuses_nonempty_destination(self) -> None:
        self.output.mkdir(parents=True)
        (self.output / "stale.lock").write_text("stale\n", encoding="utf-8")

        with self.assertRaisesRegex(RuntimeScopeError, "not empty"):
            prepare_runtime_scope(self.root, self.output)

    def test_refuses_symlink_destination(self) -> None:
        external = Path(self.temporary_directory.name) / "external"
        external.mkdir()
        self.output.symlink_to(external, target_is_directory=True)

        with self.assertRaisesRegex(RuntimeScopeError, "not a directory"):
            prepare_runtime_scope(self.root, self.output)


if __name__ == "__main__":
    unittest.main()
