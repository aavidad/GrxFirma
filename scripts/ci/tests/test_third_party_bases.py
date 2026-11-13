# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""G-08: cada copia en third_party/ documenta su base y OSV la vigila."""

from __future__ import annotations

import importlib.util
import re
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "scripts/ci/third_party_bases.py"


def load():
    spec = importlib.util.spec_from_file_location("third_party_bases", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class ThirdPartyBaseTests(unittest.TestCase):
    def test_every_copy_declares_its_base(self) -> None:
        bases, errors = load().read_bases(ROOT)
        self.assertEqual(errors, [])
        self.assertIn(("github.com/digitorus/pdf", "v0.1.2"), bases)
        self.assertIn(
            (
                "github.com/digitorus/pdfsign",
                "v0.0.0-20260102145623-a2800162ff5c",
            ),
            bases,
        )

    def test_base_matches_root_go_mod_require(self) -> None:
        go_mod = (ROOT / "go.mod").read_text(encoding="utf-8")
        for module, version in load().read_bases(ROOT)[0]:
            with self.subTest(module=module):
                self.assertRegex(go_mod, rf"(?m)^\s+{re.escape(module)} {re.escape(version)}\s*$")

    def test_missing_base_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            copy = Path(tmp) / "third_party" / "lib"
            copy.mkdir(parents=True)
            (copy / "README.md").write_text("# lib\n", encoding="utf-8")
            _, errors = load().read_bases(Path(tmp))
        self.assertTrue(errors)

    def test_synthetic_go_mod_has_no_replace(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "go.mod"
            self.assertEqual(load().main(["--write-gomod", str(target)]), 0)
            text = target.read_text(encoding="utf-8")
        self.assertNotIn("replace", text)
        self.assertIn("github.com/digitorus/pdfsign v0.0.0-20260102145623-a2800162ff5c", text)

    def test_dependency_workflow_scans_third_party_bases(self) -> None:
        workflow = (ROOT / ".github/workflows/security-dependencies.yml").read_text(
            encoding="utf-8"
        )
        self.assertIn("scripts/ci/third_party_bases.py --write-gomod", workflow)
        self.assertIn('--lockfile "$RUNNER_TEMP/third-party-bases/go.mod"', workflow)


if __name__ == "__main__":
    unittest.main()
