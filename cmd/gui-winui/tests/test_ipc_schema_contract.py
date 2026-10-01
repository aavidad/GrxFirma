# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[3]
SCHEMA = ROOT / "docs" / "schemas" / "desktop-ipc-v1.schema.json"


class DesktopIpcSchemaContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.definitions = json.loads(
            SCHEMA.read_text(encoding="utf-8")
        )["$defs"]

    def test_versioned_request_requires_complete_correlation_and_params(self) -> None:
        condition = next(
            item
            for item in self.definitions["request"]["allOf"]
            if item.get("if", {}).get("required") == ["protocol"]
        )

        self.assertEqual(
            {"requestId", "traceId", "params"},
            set(condition["then"]["required"]),
        )
        self.assertEqual(
            {"type": "null"},
            condition["then"]["properties"]["params"]["not"],
        )

    def test_response_metadata_and_non_correlatable_exception_are_closed(self) -> None:
        conditions = self.definitions["response"]["allOf"]
        serialized = json.dumps(conditions, sort_keys=True)

        self.assertIn('"const": "success"', serialized)
        self.assertIn('"const": "partial"', serialized)
        self.assertIn('"const": "failure"', serialized)
        self.assertIn('"const": ""', serialized)

        protocol_condition = next(
            item
            for item in conditions
            if item.get("if", {}).get("required") == ["protocol"]
        )
        branches = protocol_condition["then"]["anyOf"]
        self.assertIn(
            {"required": ["requestId", "traceId"]},
            branches,
        )
        non_correlatable = next(branch for branch in branches if "not" in branch)
        self.assertEqual(
            ["admission", "protocol"],
            non_correlatable["properties"]["phase"]["enum"],
        )


if __name__ == "__main__":
    unittest.main()
