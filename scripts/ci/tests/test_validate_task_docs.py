# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from scripts.ci.validate_task_docs import (
    parse_backlog,
    parse_task_cards,
    validate_task_docs,
)


BACKLOG_HEADER = """\
# Backlog

| Tarea | Descripción | Fase | Estado | Dependencias |
|---|---|:---:|:---:|---|
"""


class ValidateTaskDocsTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        self.tasks_dir = Path(self.temporary_directory.name) / "tasks"
        self.tasks_dir.mkdir()
        self.backlog = self.tasks_dir / "BACKLOG.md"

    def write_backlog(self, rows: list[str]) -> None:
        self.backlog.write_text(
            BACKLOG_HEADER + "\n".join(rows) + "\n", encoding="utf-8"
        )

    def write_card(
        self,
        filename: str,
        task_id: str,
        status: str | None,
    ) -> Path:
        status_line = "" if status is None else f"status: {status}\n"
        path = self.tasks_dir / filename
        path.write_text(
            f"---\nid: {task_id}\n{status_line}---\n\n# Task\n",
            encoding="utf-8",
        )
        return path

    def test_consistent_docs_pass_and_subtasks_are_ignored(self) -> None:
        self.write_backlog(
            [
                "| T032 | Canonical task | 7 | **HECHA** | T003 |",
                "| T100 | Preferences | 19 | **EN_PROGRESO** | T072 |",
            ]
        )
        self.write_card("T032-task.md", "T032", "HECHA")
        self.write_card("T100-task.md", "T100", "EN_PROGRESO")
        self.write_card("T100-subtask.md", "T100-PADES-MIN", "PROPUESTA")

        self.assertEqual(validate_task_docs(self.backlog, self.tasks_dir), [])
        self.assertEqual(
            [entry.status for entry in parse_backlog(self.backlog)],
            ["HECHA", "EN_PROGRESO"],
        )
        self.assertEqual(
            [card.task_id for card in parse_task_cards(self.tasks_dir)],
            ["T032", "T100"],
        )

    def test_duplicate_canonical_frontmatter_id_is_reported(self) -> None:
        self.write_backlog(["| T032 | Task | 7 | **HECHA** | T003 |"])
        self.write_card("T032-first.md", "T032", "HECHA")
        self.write_card("T999-copy.md", "T032", "HECHA")

        violations = validate_task_docs(self.backlog, self.tasks_dir)

        self.assertEqual(len(violations), 1)
        self.assertIn("duplicate canonical task ID T032", violations[0])
        self.assertIn("T032-first.md", violations[0])
        self.assertIn("T999-copy.md", violations[0])

    def test_duplicate_backlog_id_is_reported(self) -> None:
        self.write_backlog(
            [
                "| T032 | Task | 7 | **HECHA** | T003 |",
                "| T032 | Repeated task | 7 | **HECHA** | T003 |",
            ]
        )
        self.write_card("T032-task.md", "T032", "HECHA")

        violations = validate_task_docs(self.backlog, self.tasks_dir)

        self.assertEqual(len(violations), 1)
        self.assertIn("duplicate backlog task ID T032", violations[0])

    def test_missing_canonical_card_is_reported(self) -> None:
        self.write_backlog(["| T074 | Missing card | 1 | **HECHA** | T001 |"])

        violations = validate_task_docs(self.backlog, self.tasks_dir)

        self.assertEqual(len(violations), 1)
        self.assertIn("T074: canonical task card is missing", violations[0])

    def test_invalid_backlog_and_frontmatter_statuses_are_reported(self) -> None:
        self.write_backlog(
            [
                "| T032 | Bad backlog | 7 | **DONE** | T003 |",
                "| T033 | Bad card | 7 | **HECHA** | T003 |",
                "| T034 | Missing card status | 7 | **HECHA** | T003 |",
            ]
        )
        self.write_card("T032-task.md", "T032", "HECHA")
        self.write_card("T033-task.md", "T033", "DONE")
        self.write_card("T034-task.md", "T034", None)

        violations = validate_task_docs(self.backlog, self.tasks_dir)

        self.assertEqual(len(violations), 3)
        self.assertTrue(any("status 'DONE' for T032" in item for item in violations))
        self.assertTrue(any("status 'DONE' for T033" in item for item in violations))
        self.assertTrue(any("status None for T034" in item for item in violations))

    def test_status_mismatch_is_reported(self) -> None:
        self.write_backlog(["| T044 | Batch | 9 | **HECHA** | T043 |"])
        self.write_card("T044-batch.md", "T044", "LIBRE")

        violations = validate_task_docs(self.backlog, self.tasks_dir)

        self.assertEqual(len(violations), 1)
        self.assertIn(
            "T044: status mismatch: BACKLOG=HECHA, frontmatter=LIBRE",
            violations[0],
        )


if __name__ == "__main__":
    unittest.main()
