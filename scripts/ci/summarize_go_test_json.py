#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Publish bounded, redacted GitHub annotations from ``go test -json`` output."""

from __future__ import annotations

import argparse
import json
import re
from collections import defaultdict, deque
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, TextIO


ANSI_RE = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
SECRET_ASSIGNMENT_RE = re.compile(
    r"(?i)\b(password|passwd|pwd|token|secret|authorization|"
    r"api[-_]?key|client[-_]?secret|private[-_]?key)\b"
    r"(\s*[:=]\s*)([^\s,;]+)"
)
BEARER_RE = re.compile(r"(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+")
TOKEN_RE = re.compile(
    r"\b(?:github_pat_[A-Za-z0-9_]{10,}|gh[pousr]_[A-Za-z0-9]{10,})\b"
)
PEM_RE = re.compile(r"-----BEGIN [A-Z0-9 ]*(?:PRIVATE KEY|CERTIFICATE)-----")
BOILERPLATE_RE = re.compile(
    r"^(?:=== (?:RUN|PAUSE|CONT)|--- FAIL:|FAIL(?:\s|$)|exit status \d+|"
    r"\?\s+\S+\s+\[no test files\]|ok\s+\S+)"
)


@dataclass(frozen=True)
class Failure:
    package: str
    test: str
    detail: str


def redact(text: str) -> str:
    """Remove control characters and common credential forms from one log line."""
    text = ANSI_RE.sub("", text)
    text = text.replace("\x00", "")
    text = PEM_RE.sub("[REDACTED PEM]", text)
    text = BEARER_RE.sub("Bearer [REDACTED]", text)
    text = TOKEN_RE.sub("[REDACTED TOKEN]", text)
    text = SECRET_ASSIGNMENT_RE.sub(r"\1\2[REDACTED]", text)
    return " ".join(text.strip().split())


def annotation_escape(text: str, *, property_value: bool = False) -> str:
    """Escape data according to the GitHub workflow-command protocol."""
    text = text.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
    if property_value:
        text = text.replace(":", "%3A").replace(",", "%2C")
    return text


def _useful_detail(lines: Iterable[str], max_chars: int) -> str:
    useful: list[str] = []
    for raw in lines:
        line = redact(raw)
        if not line or BOILERPLATE_RE.match(line):
            continue
        if line not in useful:
            useful.append(line)
    if not useful:
        return ""
    detail = " | ".join(useful[-3:])
    if len(detail) > max_chars:
        detail = detail[: max(0, max_chars - 1)].rstrip() + "…"
    return detail


def parse_failures(stream: Iterable[str], *, max_detail_chars: int = 400) -> list[Failure]:
    """Return deterministic test/package failures from a Go JSON event stream."""
    test_output: dict[tuple[str, str], deque[str]] = defaultdict(
        lambda: deque(maxlen=12)
    )
    package_output: dict[str, deque[str]] = defaultdict(lambda: deque(maxlen=20))
    failed_tests: list[tuple[str, str]] = []
    failed_packages: list[str] = []

    for line in stream:
        try:
            event = json.loads(line)
        except (json.JSONDecodeError, TypeError):
            continue
        if not isinstance(event, dict):
            continue

        package = str(event.get("Package") or "").strip()
        test = str(event.get("Test") or "").strip()
        action = event.get("Action")
        output = event.get("Output")

        if isinstance(output, str):
            if package:
                package_output[package].append(output)
            if package and test:
                test_output[(package, test)].append(output)

        if action != "fail" or not package:
            continue
        if test:
            key = (package, test)
            if key not in failed_tests:
                failed_tests.append(key)
        elif package not in failed_packages:
            failed_packages.append(package)

    failures: list[Failure] = []
    packages_with_test_failures = {package for package, _ in failed_tests}
    for package, test in failed_tests:
        failures.append(
            Failure(
                package=package,
                test=test,
                detail=_useful_detail(
                    test_output[(package, test)], max_detail_chars
                ),
            )
        )
    for package in failed_packages:
        if package in packages_with_test_failures:
            continue
        failures.append(
            Failure(
                package=package,
                test="",
                detail=_useful_detail(package_output[package], max_detail_chars),
            )
        )
    return failures


def render_annotations(
    failures: Iterable[Failure],
    *,
    title: str,
    max_failures: int = 20,
) -> list[str]:
    """Render a bounded set of safe GitHub error annotations."""
    all_failures = list(failures)
    annotations: list[str] = []
    safe_title = annotation_escape(redact(title), property_value=True)
    for failure in all_failures[:max_failures]:
        subject = failure.package
        if failure.test:
            subject += f": {failure.test}"
        message = subject
        if failure.detail:
            message += f" — {failure.detail}"
        annotations.append(
            f"::error title={safe_title}::{annotation_escape(redact(message))}"
        )

    omitted = len(all_failures) - max_failures
    if omitted > 0:
        annotations.append(
            f"::error title={safe_title}::"
            f"{omitted} additional failure(s) omitted; inspect the streamed test log."
        )
    if not annotations:
        annotations.append(
            f"::error title={safe_title}::"
            "go test failed without a parseable JSON failure event."
        )
    return annotations


def summarize(
    source: TextIO,
    *,
    title: str,
    max_failures: int,
    max_detail_chars: int,
) -> list[str]:
    return render_annotations(
        parse_failures(source, max_detail_chars=max_detail_chars),
        title=title,
        max_failures=max_failures,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("log", type=Path)
    parser.add_argument("--title", default="Go native test failure")
    parser.add_argument("--max-failures", type=int, default=20)
    parser.add_argument("--max-detail-chars", type=int, default=400)
    args = parser.parse_args()

    try:
        with args.log.open(encoding="utf-8", errors="replace") as source:
            annotations = summarize(
                source,
                title=args.title,
                max_failures=max(1, args.max_failures),
                max_detail_chars=max(80, args.max_detail_chars),
            )
    except OSError:
        annotations = render_annotations([], title=args.title)

    for annotation in annotations:
        print(annotation)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
