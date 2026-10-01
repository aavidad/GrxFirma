#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Enforce baseline safety and reliability rules for GitHub Actions workflows."""

from __future__ import annotations

import re
import sys
import textwrap
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_DIR = ROOT / ".github" / "workflows"
USES_RE = re.compile(r"^\s*uses:\s*([^\s#]+)")
FULL_SHA_RE = re.compile(r"^[0-9a-fA-F]{40}$")
JOB_RE = re.compile(r"^  ([A-Za-z0-9_-]+):(?:\s*#.*)?$")
CHECKOUT_RE = re.compile(r"^actions/checkout@")
PYTHON_HEREDOC_RE = re.compile(
    r"^(?P<indent>\s*)python3\s+-\s+<<'(?P<tag>[A-Za-z_][A-Za-z0-9_]*)'\s*$"
)


def step_lines(lines: list[str], uses_index: int) -> list[str]:
    """Return the remainder of the step containing a uses declaration."""
    uses_indent = len(lines[uses_index]) - len(lines[uses_index].lstrip())
    step_indent = max(0, uses_indent - 2)
    block = [lines[uses_index]]
    for line in lines[uses_index + 1 :]:
        stripped = line.strip()
        if stripped:
            indent = len(line) - len(line.lstrip())
            if indent <= step_indent:
                break
        block.append(line)
    return block


def workflow_jobs(lines: list[str]) -> list[tuple[str, list[str]]]:
    try:
        jobs_line = next(index for index, line in enumerate(lines) if line == "jobs:")
    except StopIteration:
        return []

    jobs: list[tuple[str, list[str]]] = []
    current_name: str | None = None
    current_lines: list[str] = []
    for line in lines[jobs_line + 1 :]:
        match = JOB_RE.match(line)
        if match:
            if current_name is not None:
                jobs.append((current_name, current_lines))
            current_name = match.group(1)
            current_lines = [line]
        elif current_name is not None:
            current_lines.append(line)
    if current_name is not None:
        jobs.append((current_name, current_lines))
    return jobs


def validate_embedded_python(
    relative: Path, lines: list[str], violations: list[str]
) -> int:
    checked = 0
    index = 0
    while index < len(lines):
        match = PYTHON_HEREDOC_RE.match(lines[index])
        if not match:
            index += 1
            continue

        start_line = index + 1
        terminator = f"{match.group('indent')}{match.group('tag')}"
        script_lines: list[str] = []
        index += 1
        while index < len(lines) and lines[index] != terminator:
            script_lines.append(lines[index])
            index += 1
        if index == len(lines):
            violations.append(f"{relative}:{start_line}: unterminated Python heredoc")
            break

        source = textwrap.dedent("\n".join(script_lines)) + "\n"
        try:
            compile(source, f"{relative}:{start_line}", "exec")
        except SyntaxError as error:
            detail = error.msg
            if error.lineno is not None:
                detail = f"line {start_line + error.lineno}: {detail}"
            violations.append(f"{relative}:{start_line}: invalid embedded Python: {detail}")
        checked += 1
        index += 1
    return checked


def main() -> int:
    workflows = sorted((*WORKFLOW_DIR.glob("*.yml"), *WORKFLOW_DIR.glob("*.yaml")))
    if not workflows:
        print(f"No workflows found in {WORKFLOW_DIR}", file=sys.stderr)
        return 1

    violations: list[str] = []
    pinned_actions = 0
    checked_jobs = 0
    checked_python_blocks = 0

    for workflow in workflows:
        relative = workflow.relative_to(ROOT)
        lines = workflow.read_text(encoding="utf-8").splitlines()
        text = "\n".join(lines)

        if re.search(r"(?m)^\s*pull_request_target\s*:", text):
            violations.append(f"{relative}: pull_request_target is not allowed")
        if workflow.name == "compatibility.yml" and "secrets." in text:
            violations.append(f"{relative}: compatibility smoke jobs must not require secrets")

        checked_python_blocks += validate_embedded_python(relative, lines, violations)

        try:
            jobs_index = lines.index("jobs:")
        except ValueError:
            violations.append(f"{relative}: missing jobs mapping")
            continue

        top_level_permissions = any(line == "permissions:" for line in lines[:jobs_index])

        for line_number, line in enumerate(lines, start=1):
            match = USES_RE.match(line)
            if not match:
                continue
            reference = match.group(1)
            if reference.startswith("./"):
                continue
            if reference.startswith("docker://"):
                if "@sha256:" not in reference:
                    violations.append(
                        f"{relative}:{line_number}: Docker action is not pinned by digest: {reference}"
                    )
                continue
            if "@" not in reference:
                violations.append(f"{relative}:{line_number}: action has no revision: {reference}")
                continue
            revision = reference.rsplit("@", 1)[1]
            if not FULL_SHA_RE.fullmatch(revision):
                violations.append(
                    f"{relative}:{line_number}: action must use a full commit SHA: {reference}"
                )
                continue
            pinned_actions += 1

            if CHECKOUT_RE.match(reference):
                checkout_step = "\n".join(step_lines(lines, line_number - 1))
                if not re.search(r"(?m)^\s+persist-credentials:\s*false\s*$", checkout_step):
                    violations.append(
                        f"{relative}:{line_number}: checkout must set persist-credentials: false"
                    )

        for job_name, job_lines in workflow_jobs(lines):
            if not any(re.match(r"^\s{4}runs-on\s*:", line) for line in job_lines):
                continue
            checked_jobs += 1
            if not any(re.match(r"^\s{4}timeout-minutes\s*:", line) for line in job_lines):
                violations.append(f"{relative}: job {job_name!r} has no timeout-minutes")
            if not top_level_permissions and not any(
                re.match(r"^\s{4}permissions\s*:", line) for line in job_lines
            ):
                violations.append(f"{relative}: job {job_name!r} has no explicit permissions")

    if violations:
        print("Workflow audit failed:", file=sys.stderr)
        for violation in violations:
            print(f"  - {violation}", file=sys.stderr)
        return 1

    print(
        f"Workflow audit passed: {len(workflows)} workflows, "
        f"{checked_jobs} jobs, {pinned_actions} pinned actions, "
        f"{checked_python_blocks} embedded Python blocks."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
