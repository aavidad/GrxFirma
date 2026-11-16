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
# El portal de documentos y gvfs montan FUSE en $WORK_DIR/run/doc y
# $WORK_DIR/run/gvfs, y al cerrarse el bus pueden tardar en desmontar: se
# desmontan aquí y se reintenta el borrado para no dejar restos en /tmp.
unmount_work_dir() {
    local mount_point
    [ -r /proc/self/mounts ] || return 0
    while read -r _ mount_point _; do
        case "$mount_point" in
            "$WORK_DIR"/*)
                fusermount3 -uz "$mount_point" 2>/dev/null ||
                    fusermount -uz "$mount_point" 2>/dev/null ||
                    umount -l "$mount_point" 2>/dev/null || true
                ;;
        esac
    done </proc/self/mounts
}
cleanup() {
    local attempt
    for attempt in 1 2 3 4 5; do
        unmount_work_dir
        if rm -rf -- "$WORK_DIR" 2>/dev/null; then
            return 0
        fi
        sleep "$attempt"
    done
    printf 'AVISO: no se pudo borrar %s\n' "$WORK_DIR" >&2
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$WORK_DIR/build" "$WORK_DIR/home" "$WORK_DIR/config" "$WORK_DIR/cache" \
    "$WORK_DIR/data"
# Carpeta de ejecución propia (0700): sin ella, gnome-keyring y el bus de
# accesibilidad buscan sus sockets en el XDG_RUNTIME_DIR real del usuario y
# pueden hablar con su llavero.
mkdir -m 700 "$WORK_DIR/run"
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
# El entorno aislado se fija antes de dbus-run-session para que también lo
# hereden los servicios que arranca el bus (llavero, bus de accesibilidad).
cd "$WORK_DIR"
env -u WAYLAND_DISPLAY -u DBUS_SESSION_BUS_ADDRESS \
    HOME="$WORK_DIR/home" \
    XDG_CONFIG_HOME="$WORK_DIR/config" \
    XDG_CACHE_HOME="$WORK_DIR/cache" \
    XDG_DATA_HOME="$WORK_DIR/data" \
    XDG_RUNTIME_DIR="$WORK_DIR/run" \
    dbus-run-session -- "$XVFB_RUN" -a -s '-screen 0 1440x900x24 -nolisten tcp' \
    env -u WAYLAND_DISPLAY \
    QT_QPA_PLATFORM=xcb \
    QT_LINUX_ACCESSIBILITY_ALWAYS_ON=1 \
    QT_ACCESSIBILITY=1 \
    QT_QUICK_BACKEND=software \
    GTK_USE_PORTAL=0 \
    NO_AT_BRIDGE=0 \
    "$PYTHON" "$ROOT_DIR/scripts/accessibility/audit_atspi.py" \
    --app "$APP_BINARY"
