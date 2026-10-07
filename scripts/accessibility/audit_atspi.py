#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Audit the live Qt accessibility tree through AT-SPI.

Recorre cada sección de la navegación activando sus botones por AT-SPI y,
en cada una, exige que los controles visibles tengan un nombre propio (el
marcador de un campo de texto no cuenta como nombre), que los botones no se
anuncien solo con un símbolo y que puedan recibir el foco. También comprueba
que, nada más arrancar, el teclado llega a un control (el tabulador no debe
quedarse sin efecto hasta pulsar con el ratón).
"""

from __future__ import annotations

import argparse
import os
import shutil
import signal
import subprocess
import time
import unicodedata
from collections.abc import Iterator

import pyatspi


REQUIRED_NAVIGATION = {"FIRMAR", "VERIFICAR", "CIFRAR", "CONFIGURACION"}
# Secciones que se recorren si están visibles; «ACERCA DE» abre un diálogo.
OPTIONAL_NAVIGATION = ("FACTURAE", "ENI", "DOCUMENTOS ENI", "EXPERTO")
BUTTON_ROLES = {"button", "push button", "toggle button"}
TEXT_ROLES = {"password text", "text", "entry"}
NAMED_ROLES = BUTTON_ROLES | {"check box", "combo box", "radio button", "slider", "spin button"}
INTERACTIVE_ROLES = NAMED_ROLES | TEXT_ROLES


def normalized(value: str | None) -> str:
    decomposed = unicodedata.normalize("NFKD", (value or "").strip().upper())
    return "".join(char for char in decomposed if not unicodedata.combining(char))


def has_words(value: str) -> bool:
    """El nombre dice algo: contiene al menos una letra o una cifra."""
    return any(unicodedata.category(char)[0] in {"L", "N"} for char in value)


def walk(node: object, *, maximum: int = 20000) -> Iterator[object]:
    pending = [node]
    visited = 0
    while pending:
        current = pending.pop()
        visited += 1
        if visited > maximum:
            raise RuntimeError(f"el arbol AT-SPI supera {maximum} nodos")
        yield current
        children = []
        try:
            for index in range(current.childCount):
                child = current.getChildAtIndex(index)
                if child is not None:
                    children.append(child)
        except Exception:  # noqa: BLE001 - nodos que desaparecen al recorrer
            continue
        pending.extend(reversed(children))


def has_state(node: object, state: object) -> bool:
    try:
        return node.getState().contains(state)
    except Exception:  # noqa: BLE001
        return False


def showing(node: object) -> bool:
    return has_state(node, pyatspi.STATE_VISIBLE) and has_state(node, pyatspi.STATE_SHOWING)


def find_application(timeout: float) -> object:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        desktop = pyatspi.Registry.getDesktop(0)
        candidates = []
        for index in range(desktop.childCount):
            app = desktop.getChildAtIndex(index)
            if app is not None and "grxfirma" in (app.name or "").lower() and app.childCount > 0:
                candidates.append(app)
        if candidates:
            return max(candidates, key=lambda item: item.childCount)
        time.sleep(0.25)
    raise RuntimeError("GrxFirma no aparecio en el bus AT-SPI")


def focused_control(application: object) -> object | None:
    for node in walk(application):
        if has_state(node, pyatspi.STATE_FOCUSED) and node.getRoleName() in INTERACTIVE_ROLES:
            return node
    return None


def describe(node: object) -> str:
    return f"{node.getRoleName()}={(node.name or '').strip()!r}"


def audit_section(application: object, section: str) -> tuple[list[str], set[str], int]:
    failures: list[str] = []
    visible_buttons: set[str] = set()
    visible = [node for node in walk(application) if showing(node)]
    for node in visible:
        role = node.getRoleName()
        name = (node.name or "").strip()
        description = (node.description or "").strip()
        if role in BUTTON_ROLES and name:
            visible_buttons.add(normalized(name))
        if role not in INTERACTIVE_ROLES or not has_state(node, pyatspi.STATE_ENABLED):
            continue
        parent = getattr(node, "parent", None)
        embedded_combo_text = (
            role in TEXT_ROLES and parent is not None and parent.getRoleName() == "combo box"
        )
        if embedded_combo_text:
            continue
        if not name:
            # El marcador (descripción) no sustituye a una etiqueta.
            detail = f" (solo marcador {description!r})" if description else ""
            failures.append(f"{section}: {role} sin nombre accesible{detail}")
        elif role in NAMED_ROLES and not has_words(name):
            failures.append(f"{section}: {role} anunciado solo con el símbolo {name!r}")
        if not has_state(node, pyatspi.STATE_FOCUSABLE):
            failures.append(f"{section}: control habilitado sin foco: {describe(node)}")
    return failures, visible_buttons, len(visible)


def find_navigation_button(application: object, label: str) -> object | None:
    wanted = normalized(label)
    for node in walk(application):
        if node.getRoleName() in BUTTON_ROLES and normalized(node.name) == wanted and showing(node):
            return node
    return None


def press(node: object) -> None:
    action = node.queryAction()
    for index in range(action.nActions):
        if action.getName(index) in {"press", "click", "activate", "Press"}:
            action.doAction(index)
            return
    action.doAction(0)


def keyboard_reaches_a_control(application: object, settle: float) -> str:
    """Tras arrancar, el foco del teclado debe estar ya en un control y el
    tabulador debe moverlo a otro control (sin pulsar antes con el ratón)."""
    xdotool = shutil.which("xdotool")
    if xdotool:
        windows = subprocess.run(
            [xdotool, "search", "--name", "^GrxFirma$"],
            capture_output=True, text=True, timeout=10, check=False,
        ).stdout.split()
        for window in windows:
            try:
                subprocess.run([xdotool, "windowactivate", window],
                               capture_output=True, timeout=5, check=False)
            except subprocess.TimeoutExpired:
                pass
        time.sleep(settle)
    initial = focused_control(application)
    if initial is None:
        raise RuntimeError("al arrancar ningun control tiene el foco del teclado")
    if not xdotool:
        return f"foco inicial en {describe(initial)} (sin xdotool no se pulsa Tab)"
    subprocess.run([xdotool, "key", "Tab"], capture_output=True, timeout=10, check=False)
    time.sleep(settle)
    after = focused_control(application)
    if after is None:
        raise RuntimeError("tras pulsar Tab ningun control tiene el foco")
    return f"foco inicial en {describe(initial)}; Tab lo lleva a {describe(after)}"


def audit(application: object, settle: float) -> None:
    keyboard_failures = []
    try:
        keyboard = keyboard_reaches_a_control(application, settle)
    except RuntimeError as error:
        keyboard = "teclado sin control enfocado"
        keyboard_failures.append(str(error))

    failures, buttons, visible_count = audit_section(application, "inicio")
    failures = keyboard_failures + failures
    missing_navigation = sorted(REQUIRED_NAVIGATION - buttons)
    if missing_navigation:
        failures.append("navegacion ausente: " + ", ".join(missing_navigation))

    visited = []
    for label in sorted(REQUIRED_NAVIGATION) + list(OPTIONAL_NAVIGATION):
        button = find_navigation_button(application, label)
        if button is None:
            continue
        try:
            press(button)
        except Exception as error:  # noqa: BLE001
            failures.append(f"{label}: el boton de navegacion no se activa por AT-SPI ({error})")
            continue
        time.sleep(settle)
        section_failures, _, count = audit_section(application, label)
        failures.extend(section_failures)
        visible_count += count
        visited.append(label)

    if failures:
        unique = list(dict.fromkeys(failures))
        raise RuntimeError("; ".join(unique))

    print(
        "AT-SPI audit passed: "
        f"{len(visited)} sections ({', '.join(visited)}), "
        f"{visible_count} visible nodes checked; {keyboard}."
    )


def start_window_manager() -> subprocess.Popen | None:
    """Con un gestor de ventanas la ventana se activa y recibe el teclado.
    Solo si está instalado: en CI puede no haberlo."""
    metacity = shutil.which("metacity")
    if not metacity or not os.environ.get("DISPLAY"):
        return None
    process = subprocess.Popen(
        [metacity, "--replace"],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    time.sleep(1.5)
    return process


def stop(process: subprocess.Popen | None) -> None:
    if process is None:
        return
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


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--app", required=True)
    parser.add_argument("--timeout", type=float, default=15.0)
    parser.add_argument("--settle", type=float, default=1.5,
                        help="segundos de espera tras cada cambio de sección")
    args = parser.parse_args()

    app = os.path.abspath(args.app)
    if not os.path.isfile(app) or not os.access(app, os.X_OK):
        raise SystemExit(f"binario no ejecutable: {app}")

    window_manager = start_window_manager()
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
        time.sleep(args.settle)
        audit(application, args.settle)
    finally:
        stop(process)
        stop(window_manager)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
