#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contracts for the fail-closed dependency scanning workflow."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
WORKFLOW = ROOT / ".github" / "workflows" / "security-dependencies.yml"

WINUI_LOCKFILES = (
    "cmd/gui-winui/src/GrxFirma.WinUI.Core/packages.lock.json",
    "cmd/gui-winui/src/GrxFirma.WinUI/packages.lock.json",
    "cmd/gui-winui/tests/GrxFirma.WinUI.Core.Tests/packages.lock.json",
    "scripts/windows-qa/GrxFirma.WindowCapture/packages.lock.json",
)
INVENTORIED_WINUI_LOCKFILES = WINUI_LOCKFILES[1:3]
EXPLICIT_ONLY_WINUI_LOCKFILES = (
    WINUI_LOCKFILES[0],
    WINUI_LOCKFILES[3],
)


class SecurityDependenciesWorkflowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.workflow = WORKFLOW.read_text(encoding="utf-8")

    def test_inventory_requires_every_winui_lockfile(self) -> None:
        inventory = self.workflow.split(
            "Verify every dependency manifest has an explicit gate",
            maxsplit=1,
        )[1].split(
            "Gate reachable Go dependencies",
            maxsplit=1,
        )[0]
        for lockfile in INVENTORIED_WINUI_LOCKFILES:
            self.assertIn(
                f"--expected-source {lockfile}",
                inventory,
            )
        for lockfile in EXPLICIT_ONLY_WINUI_LOCKFILES:
            self.assertNotIn(
                f"--expected-source {lockfile}",
                inventory,
            )

    def test_winui_gate_scans_exact_locked_dependency_graphs(self) -> None:
        gate = self.workflow.split(
            "Gate WinUI runtime and test dependencies",
            maxsplit=1,
        )[1].split(
            "Build Android runtime dependency scope",
            maxsplit=1,
        )[0]
        self.assertIn("osv-scanner", gate)
        for lockfile in WINUI_LOCKFILES:
            self.assertIn(f"--lockfile {lockfile}", gate)
        self.assertIn("--all-packages", gate)
        self.assertIn(
            '--output-file "$RUNNER_TEMP/osv-winui.json"',
            gate,
        )


if __name__ == "__main__":
    unittest.main()
