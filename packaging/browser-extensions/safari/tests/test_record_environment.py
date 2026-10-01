# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path


SAFARI_DIR = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "grxfirma_safari_record_environment",
    SAFARI_DIR / "record_environment.py",
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class RecordEnvironmentTests(unittest.TestCase):
    def make_inputs(self, root: Path) -> tuple[Path, Path]:
        source = root / "source"
        source.mkdir()
        (source / "manifest.json").write_text(
            '{"manifest_version":3}\n', encoding="utf-8"
        )
        nested = source / "nested"
        nested.mkdir()
        (nested / "background.js").write_text("// source\n", encoding="utf-8")
        report = root / "integration-report.json"
        report.write_text('{"schema":1,"bundle_id":"example.test"}\n', encoding="utf-8")
        return source, report

    def test_records_deterministic_source_and_tool_versions(self):
        with tempfile.TemporaryDirectory() as temporary:
            source, report = self.make_inputs(Path(temporary))
            first = MODULE.record_environment(
                source,
                report,
                "Xcode 16.4\nBuild version 16F6",
                "/Applications/Xcode.app/converter",
                "12.3",
            )
            second = MODULE.record_environment(
                source,
                report,
                "Xcode 16.4\nBuild version 16F6",
                "/Applications/Xcode.app/converter",
                "12.3",
            )

            self.assertEqual(first, second)
            environment = first["build_environment"]
            self.assertEqual(environment["minimum_macos"], "12.3")
            self.assertEqual(
                environment["xcode_version"],
                ["Xcode 16.4", "Build version 16F6"],
            )
            self.assertRegex(environment["source_tree_sha256"], r"^[0-9a-f]{64}$")
            self.assertEqual(json.loads(report.read_text()), second)

    def test_source_hash_changes_with_path_or_content(self):
        with tempfile.TemporaryDirectory() as temporary:
            source, _ = self.make_inputs(Path(temporary))
            initial = MODULE.source_tree_sha256(source)
            (source / "nested" / "background.js").write_text(
                "// changed\n", encoding="utf-8"
            )
            self.assertNotEqual(MODULE.source_tree_sha256(source), initial)

    def test_rejects_symlinked_source_entry(self):
        with tempfile.TemporaryDirectory() as temporary:
            source, _ = self.make_inputs(Path(temporary))
            outside = Path(temporary) / "outside"
            outside.write_text("outside\n", encoding="utf-8")
            (source / "link").symlink_to(outside)
            with self.assertRaises(MODULE.EnvironmentRecordError):
                MODULE.source_tree_sha256(source)

    def test_rejects_symlinked_report(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source, report = self.make_inputs(root)
            outside = root / "outside-report.json"
            outside.write_text(report.read_text(encoding="utf-8"), encoding="utf-8")
            report.unlink()
            report.symlink_to(outside)
            with self.assertRaises(MODULE.EnvironmentRecordError):
                MODULE.record_environment(source, report, "Xcode", "/converter", "12.3")


if __name__ == "__main__":
    unittest.main()
