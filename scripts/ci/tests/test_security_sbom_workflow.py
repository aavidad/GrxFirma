# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
WORKFLOW = ROOT / ".github/workflows/security-sbom.yml"


class SecuritySbomWorkflowContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.workflow = WORKFLOW.read_text(encoding="utf-8")

    def test_complete_inventory_is_generated_before_runtime_staging(self) -> None:
        inventory = self.workflow.index(
            "spdx-json=grxfirma-source.spdx.json"
        )
        staging = self.workflow.index(
            "scripts/ci/prepare_runtime_sbom_scope.py"
        )

        self.assertLess(inventory, staging)

    def test_grype_blocks_on_runtime_sbom_only(self) -> None:
        self.assertIn(
            'grype" sbom:grxfirma-runtime.spdx.json',
            self.workflow,
        )
        self.assertNotIn(
            'grype" sbom:grxfirma-source.spdx.json',
            self.workflow,
        )
        self.assertIn("--fail-on high", self.workflow)
        self.assertIn("--only-fixed", self.workflow)

    def test_android_runtime_coverage_is_verified_and_preserved(self) -> None:
        self.assertIn(
            "scripts/ci/validate_runtime_sbom.py",
            self.workflow,
        )
        self.assertIn(
            "grxfirma-runtime-scope/mobile/android/app/gradle.lockfile",
            self.workflow,
        )
        self.assertIn(
            "grxfirma-runtime-scope/grxfirma-runtime-scope.json",
            self.workflow,
        )


if __name__ == "__main__":
    unittest.main()
