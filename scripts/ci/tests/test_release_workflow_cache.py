# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""G-01: los flujos que publican o firman no restauran caché de Actions."""

from __future__ import annotations

import importlib.util
import re
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
WORKFLOWS = ROOT / ".github/workflows"
SIGNING_WORKFLOWS = ("release.yml", "msix.yml")
AUDIT = ROOT / "scripts/ci/audit_workflows.py"


def load_audit():
    spec = importlib.util.spec_from_file_location("audit_workflows", AUDIT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def steps_using(lines: list[str], action: str) -> list[str]:
    audit = load_audit()
    found = []
    for index, line in enumerate(lines):
        if re.match(rf"^\s*uses:\s*{re.escape(action)}@", line):
            found.append("\n".join(audit.step_lines(lines, index)))
    return found


class SigningWorkflowCacheTests(unittest.TestCase):
    def test_signing_workflows_disable_every_cache(self) -> None:
        for name in SIGNING_WORKFLOWS:
            lines = (WORKFLOWS / name).read_text(encoding="utf-8").splitlines()
            text = "\n".join(lines)
            with self.subTest(workflow=name):
                self.assertNotRegex(text, r"(?m)^\s*cache:\s*true\s*$")
                self.assertNotIn("actions/cache@", text)
                for action in ("actions/setup-go", "jurplel/install-qt-action"):
                    for step in steps_using(lines, action):
                        self.assertRegex(step, r"(?m)^\s+cache:\s*false\s*$", step)
                for step in steps_using(lines, "actions/setup-node"):
                    self.assertRegex(
                        step, r"(?m)^\s+package-manager-cache:\s*false\s*$", step
                    )

    def test_audit_rejects_cache_in_signing_workflow(self) -> None:
        audit = load_audit()
        bad = (
            "jobs:\n"
            "  build:\n"
            "    steps:\n"
            "      - name: Setup Go\n"
            "        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e\n"
            "        with:\n"
            "          go-version-file: go.mod\n"
        )
        violations = audit.signing_cache_violations(Path("release.yml"), bad.splitlines())
        self.assertTrue(violations)
        good = bad + "          cache: false\n"
        self.assertEqual(
            audit.signing_cache_violations(Path("release.yml"), good.splitlines()), []
        )

    def test_qt_provenance_is_recorded_before_signing(self) -> None:
        text = (WORKFLOWS / "release.yml").read_text(encoding="utf-8")
        build_windows = text.split("  build-windows:", 1)[1].split("\n  verify-windows:", 1)[0]
        qt = build_windows.index("jurplel/install-qt-action@")
        provenance = build_windows.index("write-qt-provenance.ps1")
        finalize = build_windows.index("finalize-windows-release.ps1")
        self.assertLess(qt, provenance)
        self.assertLess(provenance, finalize)
        self.assertIn("GRXFIRMA_QT_PROVENANCE_MANIFEST", build_windows)

    def test_finalizer_checks_provenance_before_vendor_signing(self) -> None:
        text = (ROOT / "scripts/release/finalize-windows-release.ps1").read_text(
            encoding="utf-8"
        )
        unsigned = text.index("SignatureStatus]::NotSigned")
        guard = text.index("Assert-WindowsUnsignedVendorProvenance", unsigned)
        sign = text.index("Invoke-WindowsAuthenticodeSign", unsigned)
        self.assertLess(guard, sign)


if __name__ == "__main__":
    unittest.main()
