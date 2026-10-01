#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Inspecciona y acciona una interfaz Linux mediante su árbol AT-SPI.

La herramienta evita depender de coordenadas de pantalla y funciona también
con aplicaciones Qt ejecutadas sobre Wayland. Los valores para campos de texto
se leen de la entrada estándar para no exponerlos en la línea de órdenes.
"""

from __future__ import annotations

import argparse
import sys
import time
import unicodedata
from collections.abc import Iterator

import pyatspi


INTERACTIVE_ROLES = {
    "button",
    "check box",
    "combo box",
    "entry",
    "password text",
    "push button",
    "radio button",
    "scroll bar",
    "slider",
    "text",
    "toggle button",
}


def normalized(value: str | None) -> str:
    decomposed = unicodedata.normalize("NFKD", (value or "").strip().casefold())
    return "".join(char for char in decomposed if not unicodedata.combining(char))


def walk(root: object, *, maximum: int = 10_000) -> Iterator[tuple[str, object]]:
    pending = [("0", root)]
    visited = 0
    while pending:
        path, current = pending.pop()
        visited += 1
        if visited > maximum:
            raise RuntimeError(f"el árbol AT-SPI supera {maximum} nodos")
        yield path, current
        children = []
        for index in range(current.childCount):
            children.append((f"{path}.{index}", current.getChildAtIndex(index)))
        pending.extend(reversed(children))


def has_state(node: object, state: object) -> bool:
    return node.getState().contains(state)


def find_application(pattern: str, timeout: float) -> object:
    wanted = normalized(pattern)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        desktop = pyatspi.Registry.getDesktop(0)
        candidates = []
        for index in range(desktop.childCount):
            app = desktop.getChildAtIndex(index)
            if wanted in normalized(app.name) and app.childCount > 0:
                candidates.append(app)
        if candidates:
            return max(candidates, key=lambda item: item.childCount)
        time.sleep(0.2)
    raise RuntimeError(
        f"no apareció una aplicación AT-SPI cuyo nombre contenga {pattern!r}"
    )


def node_actions(node: object) -> list[str]:
    try:
        action = node.queryAction()
    except (NotImplementedError, AttributeError):
        return []
    return [action.getName(index) for index in range(action.nActions)]


def node_extents(node: object) -> str:
    try:
        extents = node.queryComponent().getExtents(pyatspi.DESKTOP_COORDS)
    except (NotImplementedError, AttributeError):
        return "-"
    return f"{extents.x},{extents.y},{extents.width},{extents.height}"


def visible_interactive(
    application: object,
    *,
    include_offscreen: bool = False,
    include_invisible: bool = False,
) -> Iterator[tuple[str, object]]:
    for path, node in walk(application):
        role = node.getRoleName()
        if role not in INTERACTIVE_ROLES:
            continue
        if (
            not include_invisible
            and not has_state(node, pyatspi.STATE_VISIBLE)
        ):
            continue
        if not include_offscreen and not has_state(node, pyatspi.STATE_SHOWING):
            continue
        yield path, node


def matching_nodes(
    application: object,
    name: str,
    role: str | None,
    contains: bool,
    include_offscreen: bool,
    include_invisible: bool,
) -> list[tuple[str, object]]:
    wanted = normalized(name)
    matches = []
    for path, node in visible_interactive(
        application,
        include_offscreen=include_offscreen,
        include_invisible=include_invisible,
    ):
        if role and node.getRoleName() != role:
            continue
        candidate = normalized(node.name)
        if candidate == wanted or (contains and wanted in candidate):
            matches.append((path, node))
    return matches


def select_match(
    application: object,
    *,
    name: str,
    role: str | None,
    contains: bool,
    include_offscreen: bool,
    include_invisible: bool,
    index: int,
) -> tuple[str, object]:
    matches = matching_nodes(
        application,
        name,
        role,
        contains,
        include_offscreen,
        include_invisible,
    )
    if not matches:
        raise RuntimeError(f"no se encontró el control accesible {name!r}")
    if index < 0 or index >= len(matches):
        raise RuntimeError(
            f"índice {index} fuera de rango; coincidencias disponibles: {len(matches)}"
        )
    return matches[index]


def list_controls(
    application: object,
    *,
    include_offscreen: bool,
    include_invisible: bool,
) -> int:
    for path, node in visible_interactive(
        application,
        include_offscreen=include_offscreen,
        include_invisible=include_invisible,
    ):
        enabled = has_state(node, pyatspi.STATE_ENABLED)
        focusable = has_state(node, pyatspi.STATE_FOCUSABLE)
        showing = has_state(node, pyatspi.STATE_SHOWING)
        checked = has_state(node, pyatspi.STATE_CHECKED)
        print(
            "\t".join(
                (
                    path,
                    node.getRoleName(),
                    repr((node.name or "").strip()),
                    f"enabled={str(enabled).lower()}",
                    f"focusable={str(focusable).lower()}",
                    f"showing={str(showing).lower()}",
                    f"checked={str(checked).lower()}",
                    f"actions={','.join(node_actions(node)) or '-'}",
                    f"bounds={node_extents(node)}",
                )
            )
        )
    return 0


def activate(
    path: str,
    node: object,
    *,
    requested_action: str | None = None,
    repeat: int = 1,
) -> int:
    action = node.queryAction()
    preferred = ("click", "press", "activate", "toggle")
    available = {
        normalized(action.getName(index)): index for index in range(action.nActions)
    }
    if requested_action:
        wanted = normalized(requested_action)
        if wanted not in available:
            raise RuntimeError(
                f"el control {node.name!r} no ofrece la acción "
                f"{requested_action!r}: {node_actions(node)}"
            )
        for _ in range(repeat):
            if not action.doAction(available[wanted]):
                raise RuntimeError(
                    f"AT-SPI rechazó la acción {requested_action!r} "
                    f"en el nodo {path}"
                )
        print(
            f"accion\t{requested_action}\trepeticiones={repeat}\t"
            f"{path}\t{node.getRoleName()}\t{node.name!r}"
        )
        return 0
    for candidate in preferred:
        if candidate in available:
            if not action.doAction(available[candidate]):
                raise RuntimeError(
                    f"AT-SPI rechazó la acción {candidate!r} en el nodo {path}"
                )
            print(f"activado\t{path}\t{node.getRoleName()}\t{node.name!r}")
            return 0
    if action.nActions == 1 and action.doAction(0):
        print(f"activado\t{path}\t{node.getRoleName()}\t{node.name!r}")
        return 0
    raise RuntimeError(
        f"el control {node.name!r} no ofrece una acción activable: "
        f"{node_actions(node)}"
    )


def set_text(path: str, node: object) -> int:
    value = sys.stdin.read()
    if value.endswith("\n"):
        value = value[:-1]
    try:
        editable = node.queryEditableText()
        result = editable.setTextContents(value)
    except Exception as error:
        raise RuntimeError(
            f"AT-SPI no pudo escribir en el nodo {path}; "
            "el control puede bloquear la edición accesible"
        ) from error
    if result is False:
        raise RuntimeError(f"AT-SPI no pudo escribir en el nodo {path}")
    print(f"texto-establecido\t{path}\t{node.getRoleName()}\tlongitud={len(value)}")
    return 0


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser()
    result.add_argument(
        "--application",
        default="GrxFirma",
        help="fragmento del nombre de aplicación AT-SPI",
    )
    result.add_argument("--timeout", type=float, default=10.0)
    result.add_argument(
        "--include-offscreen",
        action="store_true",
        help="incluye controles visibles que quedan fuera del viewport",
    )
    result.add_argument(
        "--include-invisible",
        action="store_true",
        help="incluye controles que AT-SPI marca como no visibles",
    )
    subparsers = result.add_subparsers(dest="command", required=True)
    subparsers.add_parser("list")
    for command in ("activate", "set-text"):
        action_parser = subparsers.add_parser(command)
        action_parser.add_argument("--name", required=True)
        action_parser.add_argument("--role")
        action_parser.add_argument("--contains", action="store_true")
        action_parser.add_argument("--index", type=int, default=0)
        if command == "activate":
            action_parser.add_argument("--action")
            action_parser.add_argument("--repeat", type=int, default=1)
    return result


def main() -> int:
    args = parser().parse_args()
    application = find_application(args.application, args.timeout)
    if args.command == "list":
        return list_controls(
            application,
            include_offscreen=args.include_offscreen,
            include_invisible=args.include_invisible,
        )
    path, node = select_match(
        application,
        name=args.name,
        role=args.role,
        contains=args.contains,
        include_offscreen=args.include_offscreen,
        include_invisible=args.include_invisible,
        index=args.index,
    )
    if args.command == "activate":
        return activate(
            path,
            node,
            requested_action=args.action,
            repeat=args.repeat,
        )
    if args.command == "set-text":
        return set_text(path, node)
    raise RuntimeError(f"comando no soportado: {args.command}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as error:
        print(f"ERROR: {error}", file=sys.stderr)
        raise SystemExit(1)
