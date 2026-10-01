#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Create a bounded, redacted diagnostic from a CI build log."""

from __future__ import annotations

import argparse
import re
from pathlib import Path

from summarize_go_test_json import annotation_escape, redact


ERROR_RE = re.compile(
    r"(?i)(?:\berror\b|\bfatal\b|\bfailed\b|undefined symbols?|"
    r"unsupported architecture|no such file|not found|"
    r"(?:^|\s)(?:make|g?make)(?:\[\d+\])?:\s+\*\*\*|"
    r"(?:^|\s)ld:)"
)
PHASE_RE = re.compile(
    r"(?i)^(?:Compilando .+|Paquete generado .+|Instalador PKG generado .+)$"
)


def sanitize_lines(lines: list[str]) -> list[str]:
    return [clean for line in lines if (clean := redact(line))]


def diagnostic(lines: list[str], *, max_lines: int = 8, max_chars: int = 1200) -> str:
    clean = sanitize_lines(lines)
    phase = next((line for line in reversed(clean) if PHASE_RE.match(line)), "")
    errors = [line for line in clean if ERROR_RE.search(line)]
    selected = errors[-max_lines:] if errors else clean[-min(max_lines, 5) :]
    if phase and phase not in selected:
        selected.insert(0, f"phase: {phase}")
    message = " | ".join(selected) or "build failed without diagnostic output"
    if len(message) > max_chars:
        message = message[: max(0, max_chars - 1)].rstrip() + "…"
    return message


def write_sanitized_tail(
    destination: Path, lines: list[str], *, max_lines: int = 400
) -> None:
    clean = sanitize_lines(lines)
    destination.write_text(
        "\n".join(clean[-max_lines:]) + ("\n" if clean else ""),
        encoding="utf-8",
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("log", type=Path)
    parser.add_argument("--title", default="macOS bundle build failure")
    parser.add_argument("--sanitized-output", type=Path)
    parser.add_argument("--max-lines", type=int, default=8)
    parser.add_argument("--max-chars", type=int, default=1200)
    parser.add_argument("--max-artifact-lines", type=int, default=400)
    args = parser.parse_args()

    try:
        lines = args.log.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        lines = []

    if args.sanitized_output is not None:
        write_sanitized_tail(
            args.sanitized_output,
            lines,
            max_lines=max(1, args.max_artifact_lines),
        )

    title = annotation_escape(redact(args.title), property_value=True)
    message = diagnostic(
        lines,
        max_lines=max(1, args.max_lines),
        max_chars=max(120, args.max_chars),
    )
    print(f"::error title={title}::{annotation_escape(message)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
