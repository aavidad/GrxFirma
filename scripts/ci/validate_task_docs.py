#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Validate consistency between the task backlog and canonical task cards."""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_BACKLOG = ROOT / "tasks" / "BACKLOG.md"
DEFAULT_TASKS_DIR = ROOT / "tasks"

ALLOWED_STATUSES = frozenset(
    {"LIBRE", "ASIGNADA", "EN_PROGRESO", "HECHA", "BLOQUEADA"}
)
CANONICAL_ID_RE = re.compile(r"^T[0-9]{3}$")
FRONTMATTER_FIELD_RE = re.compile(
    r"^(?P<key>[A-Za-z_][A-Za-z0-9_-]*):(?P<value>.*)$"
)
MARKDOWN_TABLE_SEPARATOR_RE = re.compile(r"^:?-{3,}:?$")


@dataclass(frozen=True)
class BacklogEntry:
    task_id: str
    status: str
    line: int


@dataclass(frozen=True)
class TaskCard:
    task_id: str
    status: str | None
    path: Path


def _plain_cell(value: str) -> str:
    """Remove the lightweight Markdown used around backlog identifiers/statuses."""
    return value.strip().strip("*_`").strip()


def _table_cells(line: str) -> list[str] | None:
    stripped = line.strip()
    if not stripped.startswith("|") or not stripped.endswith("|"):
        return None
    return [_plain_cell(cell) for cell in stripped[1:-1].split("|")]


def parse_backlog(path: Path) -> list[BacklogEntry]:
    """Read canonical task rows from Markdown tables headed by Tarea/Estado."""
    entries: list[BacklogEntry] = []
    task_column: int | None = None
    status_column: int | None = None

    for line_number, line in enumerate(
        path.read_text(encoding="utf-8").splitlines(), start=1
    ):
        cells = _table_cells(line)
        if cells is None:
            task_column = None
            status_column = None
            continue

        normalized_headers = [cell.casefold() for cell in cells]
        if "tarea" in normalized_headers and "estado" in normalized_headers:
            task_column = normalized_headers.index("tarea")
            status_column = normalized_headers.index("estado")
            continue

        if task_column is None or status_column is None:
            continue
        if max(task_column, status_column) >= len(cells):
            continue
        if all(
            MARKDOWN_TABLE_SEPARATOR_RE.fullmatch(cell.replace(" ", ""))
            for cell in cells
        ):
            continue

        task_id = cells[task_column]
        if CANONICAL_ID_RE.fullmatch(task_id):
            entries.append(
                BacklogEntry(
                    task_id=task_id,
                    status=cells[status_column],
                    line=line_number,
                )
            )

    return entries


def parse_frontmatter(path: Path) -> dict[str, str] | None:
    """Parse the scalar fields needed from a Markdown YAML frontmatter block."""
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0].strip() != "---":
        return None

    closing_line: int | None = None
    for index, line in enumerate(lines[1:], start=1):
        if line.strip() == "---":
            closing_line = index
            break
    if closing_line is None:
        return None

    fields: dict[str, str] = {}
    for line in lines[1:closing_line]:
        match = FRONTMATTER_FIELD_RE.match(line)
        if not match:
            continue
        key = match.group("key")
        value = match.group("value").strip()
        if (
            len(value) >= 2
            and value[0] == value[-1]
            and value[0] in {"'", '"'}
        ):
            value = value[1:-1]
        fields[key] = value
    return fields


def parse_task_cards(tasks_dir: Path) -> list[TaskCard]:
    """Return canonical cards, ignoring subtask and other non-canonical IDs."""
    cards: list[TaskCard] = []
    for path in sorted(tasks_dir.glob("T*.md")):
        frontmatter = parse_frontmatter(path)
        if frontmatter is None:
            continue
        task_id = frontmatter.get("id", "")
        if not CANONICAL_ID_RE.fullmatch(task_id):
            # This intentionally excludes subtask IDs such as T100-PADES-MIN.
            continue
        cards.append(
            TaskCard(
                task_id=task_id,
                status=frontmatter.get("status"),
                path=path,
            )
        )
    return cards


def _allowed_statuses_message() -> str:
    return ", ".join(sorted(ALLOWED_STATUSES))


def validate_task_docs(backlog_path: Path, tasks_dir: Path) -> list[str]:
    """Return all documentation consistency violations in deterministic order."""
    backlog_entries = parse_backlog(backlog_path)
    cards = parse_task_cards(tasks_dir)
    violations: list[str] = []

    backlog_by_id: dict[str, list[BacklogEntry]] = {}
    for entry in backlog_entries:
        backlog_by_id.setdefault(entry.task_id, []).append(entry)
    cards_by_id: dict[str, list[TaskCard]] = {}
    for card in cards:
        cards_by_id.setdefault(card.task_id, []).append(card)

    for task_id, entries in sorted(backlog_by_id.items()):
        if len(entries) > 1:
            lines = ", ".join(str(entry.line) for entry in entries)
            violations.append(
                f"{backlog_path}: duplicate backlog task ID {task_id} "
                f"(lines {lines})"
            )

    for task_id, matching_cards in sorted(cards_by_id.items()):
        if len(matching_cards) > 1:
            paths = ", ".join(str(card.path) for card in matching_cards)
            violations.append(
                f"duplicate canonical task ID {task_id} in task cards: {paths}"
            )

    allowed = _allowed_statuses_message()
    for entry in backlog_entries:
        if entry.status not in ALLOWED_STATUSES:
            violations.append(
                f"{backlog_path}:{entry.line}: status {entry.status!r} for "
                f"{entry.task_id} is not allowed (expected one of: {allowed})"
            )

    for card in cards:
        if card.status not in ALLOWED_STATUSES:
            violations.append(
                f"{card.path}: status {card.status!r} for {card.task_id} is "
                f"not allowed (expected one of: {allowed})"
            )

    for task_id, entries in sorted(backlog_by_id.items()):
        matching_cards = cards_by_id.get(task_id, [])
        if not matching_cards:
            violations.append(
                f"{task_id}: canonical task card is missing from {tasks_dir}"
            )
            continue
        if len(entries) != 1 or len(matching_cards) != 1:
            continue

        backlog_status = entries[0].status
        card = matching_cards[0]
        if (
            backlog_status in ALLOWED_STATUSES
            and card.status in ALLOWED_STATUSES
            and backlog_status != card.status
        ):
            violations.append(
                f"{task_id}: status mismatch: BACKLOG={backlog_status}, "
                f"frontmatter={card.status} ({card.path})"
            )

    return violations


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Validate canonical task IDs and statuses between BACKLOG.md and "
            "task-card frontmatter."
        )
    )
    parser.add_argument("--backlog", type=Path, default=DEFAULT_BACKLOG)
    parser.add_argument("--tasks-dir", type=Path, default=DEFAULT_TASKS_DIR)
    args = parser.parse_args(argv)

    try:
        violations = validate_task_docs(args.backlog, args.tasks_dir)
    except OSError as error:
        print(f"Task documentation validation failed: {error}", file=sys.stderr)
        return 1

    if violations:
        print("Task documentation validation failed:", file=sys.stderr)
        for violation in violations:
            print(f"  - {violation}", file=sys.stderr)
        return 1

    canonical_count = len(parse_backlog(args.backlog))
    print(
        f"Task documentation validation passed: "
        f"{canonical_count} canonical backlog tasks."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
