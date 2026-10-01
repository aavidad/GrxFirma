# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
WORKFLOW = ROOT / ".github/workflows/compatibility.yml"


class CompatibilityWorkflowContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.workflow = WORKFLOW.read_text(encoding="utf-8")

    def test_winui_tests_keep_build_artifacts_outside_source_tree(self) -> None:
        step = self.workflow.split(
            "- name: Test WinUI core and application contracts",
            maxsplit=1,
        )[1].split(
            "- name: Test native protocol fallback without OpenGL",
            maxsplit=1,
        )[0]

        self.assertIn('$env:RUNNER_TEMP "winui-contract-tests"', step)
        self.assertEqual(step.count("--artifacts-path $testArtifacts"), 2)
        self.assertNotIn("Remove-Item", step)


if __name__ == "__main__":
    unittest.main()
