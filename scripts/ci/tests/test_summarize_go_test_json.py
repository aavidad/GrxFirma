# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import importlib.util
import io
import json
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "summarize_go_test_json.py"
SPEC = importlib.util.spec_from_file_location("summarize_go_test_json", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


def jsonl(*events: dict[str, object]) -> io.StringIO:
    return io.StringIO("".join(json.dumps(event) + "\n" for event in events))


class SummarizeGoTestJsonTests(unittest.TestCase):
    def test_reports_test_name_and_assertion_detail(self) -> None:
        failures = MODULE.parse_failures(
            jsonl(
                {
                    "Action": "output",
                    "Package": "example/pkg",
                    "Test": "TestContrato",
                    "Output": "contrato_test.go:42: got false, want true\n",
                },
                {
                    "Action": "fail",
                    "Package": "example/pkg",
                    "Test": "TestContrato",
                },
                {"Action": "fail", "Package": "example/pkg"},
            )
        )

        self.assertEqual(len(failures), 1)
        self.assertEqual(failures[0].package, "example/pkg")
        self.assertEqual(failures[0].test, "TestContrato")
        self.assertIn("got false, want true", failures[0].detail)

    def test_reports_package_compile_failure(self) -> None:
        failures = MODULE.parse_failures(
            jsonl(
                {
                    "Action": "output",
                    "Package": "example/broken",
                    "Output": "./broken.go:9:2: undefined: missing\n",
                },
                {"Action": "fail", "Package": "example/broken"},
            )
        )

        self.assertEqual(len(failures), 1)
        self.assertEqual(failures[0].test, "")
        self.assertIn("undefined: missing", failures[0].detail)

    def test_redacts_credentials_and_escapes_workflow_commands(self) -> None:
        failures = [
            MODULE.Failure(
                package="example/pkg",
                test="TestEscape",
                detail="token=abc123 ::error:: injected 100%",
            )
        ]

        annotation = MODULE.render_annotations(
            failures, title="Go:test,fail", max_failures=20
        )[0]

        self.assertIn("title=Go%3Atest%2Cfail", annotation)
        self.assertIn("token=[REDACTED]", annotation)
        self.assertNotIn("abc123", annotation)
        self.assertIn("100%25", annotation)

    def test_bounds_failure_count_and_ignores_malformed_json(self) -> None:
        source = io.StringIO(
            "not-json\n"
            + "".join(
                json.dumps(
                    {
                        "Action": "fail",
                        "Package": "example/pkg",
                        "Test": f"Test{index}",
                    }
                )
                + "\n"
                for index in range(3)
            )
        )

        annotations = MODULE.render_annotations(
            MODULE.parse_failures(source),
            title="Go test",
            max_failures=2,
        )

        self.assertEqual(len(annotations), 3)
        self.assertIn("1 additional failure(s) omitted", annotations[-1])

    def test_writes_only_a_bounded_redacted_build_tail(self) -> None:
        build_script = SCRIPT.parent / "summarize_build_log.py"
        build_spec = importlib.util.spec_from_file_location(
            "summarize_build_log", build_script
        )
        assert build_spec is not None and build_spec.loader is not None
        build_module = importlib.util.module_from_spec(build_spec)
        sys.modules[build_spec.name] = build_module
        build_spec.loader.exec_module(build_module)

        lines = [
            "Compilando GUI Qt/QML de macOS (arm64)...",
            "password=super-secret",
            "ld: undefined symbols for architecture arm64",
            "clang: error: linker command failed",
        ]
        message = build_module.diagnostic(lines)
        self.assertIn("phase: Compilando GUI", message)
        self.assertIn("undefined symbols", message)
        self.assertNotIn("super-secret", message)

        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "diagnostic.log"
            build_module.write_sanitized_tail(output, lines, max_lines=2)
            artifact = output.read_text(encoding="utf-8")
        self.assertNotIn("super-secret", artifact)
        self.assertEqual(len(artifact.splitlines()), 2)


if __name__ == "__main__":
    unittest.main()
