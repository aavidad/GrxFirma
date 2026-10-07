#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
QMAKE=${QMAKE:-qmake6}
PYTHON=${PYTHON:-python3}
XVFB_RUN=${XVFB_RUN:-xvfb-run}

for command in "$QMAKE" make dbus-run-session "$XVFB_RUN" Xvfb "$PYTHON"; do
    if ! command -v "$command" >/dev/null 2>&1; then
        printf 'ERROR: falta la herramienta de accesibilidad: %s\n' "$command" >&2
        exit 1
    fi
done

if ! "$PYTHON" -c 'import pyatspi' >/dev/null 2>&1; then
    printf '%s\n' 'ERROR: falta python3-pyatspi (AT-SPI).' >&2
    exit 1
fi

WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-atspi.XXXXXX")
cleanup() {
    rm -rf -- "$WORK_DIR"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$WORK_DIR/build" "$WORK_DIR/home" "$WORK_DIR/config" "$WORK_DIR/cache"
# GRXFIRMA_QT_APP permite auditar un binario ya compilado sin recompilar.
APP_BINARY=${GRXFIRMA_QT_APP:-}
if [ -z "$APP_BINARY" ]; then
    BUILD_LOG="$WORK_DIR/build.log"
    if ! "$QMAKE" "$ROOT_DIR/cmd/gui-qml/grxfirma_qt.pro" \
        -o "$WORK_DIR/build/Makefile" >"$BUILD_LOG" 2>&1; then
        cat "$BUILD_LOG" >&2
        exit 1
    fi
    if ! make --no-print-directory -C "$WORK_DIR/build" \
        -j"${JOBS:-$(getconf _NPROCESSORS_ONLN)}" >>"$BUILD_LOG" 2>&1; then
        cat "$BUILD_LOG" >&2
        exit 1
    fi
    APP_BINARY="$WORK_DIR/build/grxfirma-gui-qml"
fi

# metacity (si está instalado) activa la ventana para probar el teclado y
# xdotool pulsa Tab; sin ellos la auditoría comprueba solo el foco inicial.
for optional in metacity xdotool; do
    if ! command -v "$optional" >/dev/null 2>&1; then
        printf 'AVISO: sin %s; la prueba de teclado será parcial.\n' "$optional" >&2
    fi
done

# Fuera del árbol de fuentes: la aplicación carga cmd/gui-qml/qml/main.qml del
# directorio actual si existe, y aquí se audita el QML compilado en el binario.
cd "$WORK_DIR"
dbus-run-session -- "$XVFB_RUN" -a -s '-screen 0 1440x900x24 -nolisten tcp' \
    env -u WAYLAND_DISPLAY \
    QT_QPA_PLATFORM=xcb \
    HOME="$WORK_DIR/home" \
    XDG_CONFIG_HOME="$WORK_DIR/config" \
    XDG_CACHE_HOME="$WORK_DIR/cache" \
    QT_LINUX_ACCESSIBILITY_ALWAYS_ON=1 \
    QT_ACCESSIBILITY=1 \
    QT_QUICK_BACKEND=software \
    GTK_USE_PORTAL=0 \
    NO_AT_BRIDGE=0 \
    "$PYTHON" "$ROOT_DIR/scripts/accessibility/audit_atspi.py" \
    --app "$APP_BINARY"
