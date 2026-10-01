# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import subprocess
import tempfile
import unittest
from pathlib import Path

from scripts.compliance.check_legal_headers import (
    CENTRAL_CATEGORIES,
    LICENSE_REQUIRED_MARKERS,
    ROOT,
    audit_repository,
    check_inline_header,
    check_published_license_consistency,
    check_readme_license,
    classify_path,
    header_region,
    list_tracked_files,
    load_policy,
)


class LegalHeaderPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.policy = load_policy()

    def test_policy_uses_the_exact_spanish_canonical_header(self) -> None:
        self.assertEqual(
            self.policy.canonical_header.required_lines,
            (
                "Derechos de autor (C) 2026 Alberto Avidad Fernández.",
                "Autoría: Alberto Avidad Fernández",
                "Licencia: EUPL 1.2 o posterior",
                "SPDX-License-Identifier: EUPL-1.2",
            ),
        )

    def test_canonical_header_is_required_in_inline_files(self) -> None:
        text = """\
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

print("correcto")
"""
        self.assertEqual(
            check_inline_header("script.py", text, self.policy.canonical_header),
            [],
        )

        violations = check_inline_header(
            "sin-cabecera.py",
            'print("falta")\n',
            self.policy.canonical_header,
        )
        self.assertEqual(len(violations), 1)
        self.assertEqual(violations[0].code, "missing-canonical-header")

    def test_old_gpl_header_is_rejected(self) -> None:
        text = """\
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 Diputacion de Granada
# Autor: Alberto Avidad Fernandez
"""
        violations = check_inline_header(
            "legacy.py", text, self.policy.canonical_header
        )
        self.assertEqual(
            {violation.code for violation in violations},
            {"missing-canonical-header", "wrong-license"},
        )

    def test_markdown_frontmatter_remains_before_the_header(self) -> None:
        text = """\
---
id: T001
status: HECHA
---

<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->
"""
        region = header_region(text)
        self.assertNotIn("id: T001", region)
        self.assertEqual(
            check_inline_header("tasks/T001.md", text, self.policy.canonical_header),
            [],
        )

    def test_json_binary_generated_and_uncommentable_use_central_coverage(
        self,
    ) -> None:
        examples = {
            "global.json": "json",
            "logo.png": "binary-resource",
            "docs/generated/go-modules.txt": "generated",
            "go.sum": "uncommentable",
        }
        for path, expected_category in examples.items():
            with self.subTest(path=path):
                classified = classify_path(path, self.policy)
                self.assertEqual(classified.category, expected_category)
                self.assertIn(classified.category, self.policy.central_categories)
        self.assertEqual(self.policy.central_categories, CENTRAL_CATEGORIES)
        self.assertEqual(
            (
                self.policy.central_copyright,
                self.policy.central_authorship,
                self.policy.central_license_notice,
                self.policy.central_spdx,
            ),
            self.policy.canonical_header.required_lines,
        )

    def test_third_party_paths_keep_their_own_policy(self) -> None:
        expected = {
            "third_party/pdfsign/sign.go": ("pdfsign", "BSD-2-Clause"),
            "mobile/android/gradlew": ("Gradle Wrapper", "Apache-2.0"),
            "mobile/android/gradle/wrapper/gradle-wrapper.jar": (
                "Gradle Wrapper",
                "Apache-2.0",
            ),
        }
        policies = {item.name: item for item in self.policy.third_party}
        for path, (name, license_identifier) in expected.items():
            with self.subTest(path=path):
                classified = classify_path(path, self.policy)
                self.assertEqual(classified.category, "third-party")
                self.assertEqual(classified.third_party_name, name)
                self.assertEqual(policies[name].license, license_identifier)

    def test_root_license_text_is_not_attributed_to_the_project_author(self) -> None:
        classified = classify_path("LICENSE", self.policy)
        self.assertEqual(classified.category, "license-text")

    def test_unknown_extensions_fail_closed(self) -> None:
        classified = classify_path(
            "nuevo/formato-sin-politica.desconocido", self.policy
        )
        self.assertEqual(classified.category, "unknown")


class RepositoryLegalInventoryTests(unittest.TestCase):
    def test_every_tracked_file_has_an_explicit_classification(self) -> None:
        result = audit_repository(ROOT, enforce_headers=False)
        self.assertEqual(result.violations, ())
        self.assertEqual(result.tracked_files, list_tracked_files(ROOT))
        self.assertNotIn("unknown", result.counts)
        self.assertGreater(result.counts["inline"], 0)
        self.assertGreater(result.counts["third-party"], 0)
        for category in CENTRAL_CATEGORIES:
            self.assertGreater(result.counts[category], 0)

    def test_root_license_contains_the_official_spanish_sections(self) -> None:
        license_text = (ROOT / "LICENSE").read_text(encoding="utf-8")
        for marker in LICENSE_REQUIRED_MARKERS:
            with self.subTest(marker=marker):
                self.assertIn(marker, license_text)

    def test_readme_publishes_the_canonical_license(self) -> None:
        self.assertEqual(check_readme_license(ROOT), [])

    def test_readme_license_rejects_the_previous_gplv3_contradiction(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            (root / "README.md").write_text(
                "# Proyecto\n\n## Licencia\n\nGPLv3.\n",
                encoding="utf-8",
            )
            violations = check_readme_license(root)

        self.assertEqual(len(violations), 1)
        self.assertEqual(violations[0].code, "inconsistent-readme-license")

    def test_user_facing_license_surfaces_are_consistent(self) -> None:
        self.assertEqual(check_published_license_consistency(ROOT), [])

    def test_user_facing_license_gate_rejects_obsolete_help(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            help_path = root / "help.txt"
            help_path.write_text(
                "Software libre bajo GNU GPL v3.\n",
                encoding="utf-8",
            )

            violations = check_published_license_consistency(root, ("help.txt",))

        self.assertEqual(len(violations), 1)
        self.assertEqual(violations[0].code, "inconsistent-published-license")

    def test_git_inventory_does_not_include_untracked_files(self) -> None:
        with tempfile.TemporaryDirectory() as temporary_directory:
            repository = Path(temporary_directory)
            subprocess.run(
                ["git", "init", "-q", str(repository)],
                check=True,
            )
            tracked = repository / "tracked.py"
            untracked = repository / "untracked.py"
            tracked.write_text("# tracked\n", encoding="utf-8")
            untracked.write_text("# untracked\n", encoding="utf-8")
            subprocess.run(
                ["git", "-C", str(repository), "add", "tracked.py"],
                check=True,
            )

            self.assertEqual(list_tracked_files(repository), ("tracked.py",))


if __name__ == "__main__":
    unittest.main()
