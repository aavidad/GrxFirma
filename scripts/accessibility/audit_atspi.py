#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Audit the live Qt accessibility tree through AT-SPI."""

from __future__ import annotations

import argparse
import os
import signal
import subprocess
import time
import unicodedata
from collections.abc import Iterator

import pyatspi


REQUIRED_NAVIGATION = {"FIRMAR", "VERIFICAR", "CIFRAR", "CONFIGURACION"}
BUTTON_ROLES = {"button", "push button"}
TEXT_ROLES = {"password text", "text"}
INTERACTIVE_ROLES = BUTTON_ROLES | {"check box", "combo box"} | TEXT_ROLES


def normalized(value: str | None) -> str:
    decomposed = unicodedata.normalize("NFKD", (value or "").strip().upper())
    return "".join(char for char in decomposed if not unicodedata.combining(char))


def walk(node: object, *, maximum: int = 5000) -> Iterator[object]:
    pending = [node]
    visited = 0
    while pending:
        current = pending.pop()
        visited += 1
        if visited > maximum:
            raise RuntimeError(f"el arbol AT-SPI supera {maximum} nodos")
        yield current
        children = []
        for index in range(current.childCount):
            children.append(current.getChildAtIndex(index))
        pending.extend(reversed(children))


def has_state(node: object, state: object) -> bool:
    return node.getState().contains(state)


def find_application(timeout: float) -> object:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        desktop = pyatspi.Registry.getDesktop(0)
        candidates = []
        for index in range(desktop.childCount):
            app = desktop.getChildAtIndex(index)
            if "grxfirma" in (app.name or "").lower() and app.childCount > 0:
                candidates.append(app)
        if candidates:
            return max(candidates, key=lambda item: item.childCount)
        time.sleep(0.25)
    raise RuntimeError("GrxFirma no aparecio en el bus AT-SPI")


def audit(application: object) -> None:
    nodes = list(walk(application))
    visible = [
        node
        for node in nodes
        if has_state(node, pyatspi.STATE_VISIBLE)
        and has_state(node, pyatspi.STATE_SHOWING)
    ]

    unnamed = []
    not_focusable = []
    visible_buttons = set()
    for node in visible:
        role = node.getRoleName()
        name = (node.name or "").strip()
        description = (node.description or "").strip()
        enabled = has_state(node, pyatspi.STATE_ENABLED)
        if role in BUTTON_ROLES and name:
            visible_buttons.add(normalized(name))
        if role not in INTERACTIVE_ROLES or not enabled:
            continue
        if role in BUTTON_ROLES | {"check box", "combo box"} and not name:
            unnamed.append(role)
        parent = getattr(node, "parent", None)
        embedded_combo_text = (
            role in TEXT_ROLES
            and parent is not None
            and parent.getRoleName() == "combo box"
        )
        if (
            role in TEXT_ROLES
            and not embedded_combo_text
            and not name
            and not description
        ):
            unnamed.append(role)
        if not has_state(node, pyatspi.STATE_FOCUSABLE):
            not_focusable.append((role, name))

    missing_navigation = sorted(REQUIRED_NAVIGATION - visible_buttons)
    failures = []
    if missing_navigation:
        failures.append("navegacion ausente: " + ", ".join(missing_navigation))
    if unnamed:
        failures.append(
            "controles visibles sin nombre accesible: " + ", ".join(sorted(unnamed))
        )
    if not_focusable:
        failures.append(
            "controles habilitados sin foco: "
            + ", ".join(f"{role}={name!r}" for role, name in not_focusable)
        )
    if failures:
        raise RuntimeError("; ".join(failures))

    print(
        "AT-SPI audit passed: "
        f"{len(nodes)} nodes, {len(visible)} visible nodes, "
        f"{len(visible_buttons)} named visible buttons."
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--app", required=True)
    parser.add_argument("--timeout", type=float, default=15.0)
    args = parser.parse_args()

    app = os.path.abspath(args.app)
    if not os.path.isfile(app) or not os.access(app, os.X_OK):
        raise SystemExit(f"binario no ejecutable: {app}")

    # Start AT-SPI before Qt so the application registers its accessible tree.
    pyatspi.Registry.getDesktop(0)
    process = subprocess.Popen(
        [app],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    try:
        application = find_application(args.timeout)
        audit(application)
    finally:
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait(timeout=5)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
