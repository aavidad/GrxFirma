# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""G-03: web-ext se instala desde lockfile, sin scripts y sin credenciales."""

from __future__ import annotations

import json
import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
WORKFLOWS = ROOT / ".github/workflows"
TOOLS = ROOT / "packaging/browser-extensions/tools"


def step_containing(text: str, needle: str) -> str:
    index = text.index(needle)
    start = text.rfind("      - name:", 0, index)
    end = text.find("      - name:", index)
    return text[start : end if end != -1 else len(text)]


class WebExtLockfileTests(unittest.TestCase):
    def test_workflows_never_resolve_npm_trees_on_the_fly(self) -> None:
        for workflow in sorted(WORKFLOWS.glob("*.yml")):
            text = workflow.read_text(encoding="utf-8")
            with self.subTest(workflow=workflow.name):
                self.assertNotRegex(text, r"npm\s+install\s+(--global|-g)\b")
                self.assertNotRegex(text, r"\bnpx\s+(--yes|-y)\b")

    def test_release_installs_web_ext_without_scripts_or_secrets(self) -> None:
        text = (WORKFLOWS / "release.yml").read_text(encoding="utf-8")
        step = step_containing(text, "--prefix packaging/browser-extensions/tools")
        self.assertIn("npm ci --ignore-scripts", step)
        self.assertNotIn("secrets.", step)
        self.assertIn("GRXFIRMA_WEB_EXT:", text)

    def test_compatibility_lints_with_locked_web_ext(self) -> None:
        text = (WORKFLOWS / "compatibility.yml").read_text(encoding="utf-8")
        self.assertIn("npm ci --ignore-scripts", text)
        self.assertIn("tools/node_modules/.bin/web-ext lint", text)

    def test_lockfile_pins_whole_tree_with_integrity(self) -> None:
        lock_path = TOOLS / "package-lock.json"
        self.assertTrue(
            lock_path.is_file(),
            "Falta packaging/browser-extensions/tools/package-lock.json; "
            "generarlo con red: npm install --package-lock-only --ignore-scripts "
            "--prefix packaging/browser-extensions/tools",
        )
        lock = json.loads(lock_path.read_text(encoding="utf-8"))
        packages = lock["packages"]
        self.assertEqual(packages["node_modules/web-ext"]["version"], "10.8.0")
        for name, entry in packages.items():
            if not name or entry.get("link"):
                continue
            with self.subTest(package=name):
                self.assertRegex(entry.get("integrity", ""), r"^sha512-")
                self.assertTrue(
                    re.match(r"^https://registry\.npmjs\.org/", entry.get("resolved", ""))
                )

    def test_release_signing_doc_describes_lockfile(self) -> None:
        text = (ROOT / "docs/RELEASE_SIGNING.md").read_text(encoding="utf-8")
        self.assertIn("package-lock.json", text)
        self.assertNotIn("Instalar web-ext fijado", text)


if __name__ == "__main__":
    unittest.main()
