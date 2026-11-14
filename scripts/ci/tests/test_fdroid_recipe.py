# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""G-09: la receta de F-Droid apunta a un commit real y a un versionCode coherente."""

from __future__ import annotations

import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
RECIPE = ROOT / "packaging/fdroid/io.github.aavidad.grxfirma.yml"


def version_code(version: str) -> int:
    # Misma fórmula que el paso «Fijar versión Android» de release.yml.
    major, minor, patch = map(int, re.fullmatch(r"(\d+)\.(\d+)\.(\d+)", version).groups())
    return major * 1_000_000 + minor * 1_000 + patch


class FdroidRecipeTests(unittest.TestCase):
    def test_builds_pin_commit_hash_and_version_code(self) -> None:
        text = RECIPE.read_text(encoding="utf-8")
        builds = re.findall(
            r"- versionName: (\S+)\n\s+versionCode: (\d+)\n\s+commit: (\S+)", text
        )
        self.assertTrue(builds)
        for name, code, commit in builds:
            with self.subTest(version=name):
                self.assertRegex(commit, r"^[0-9a-f]{40}$")
                self.assertEqual(int(code), version_code(name))

    def test_release_formula_is_unchanged(self) -> None:
        workflow = (ROOT / ".github/workflows/release.yml").read_text(encoding="utf-8")
        self.assertIn("code = major * 1_000_000 + minor * 1_000 + patch", workflow)


if __name__ == "__main__":
    unittest.main()
