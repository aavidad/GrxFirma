#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
OUTPUT=${1:-"$ROOT_DIR/mobile/android/app/core/grxfirma.aar"}
GO_BIN=${GO_BIN:-go}
GOMOBILE_VERSION=${GOMOBILE_VERSION:-v0.0.0-20260217195705-b56b3793a9c4}
ANDROID_MIN_API=${ANDROID_MIN_API:-26}

if [[ -z "${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}" ]]; then
    printf '%s\n' "ERROR: defina ANDROID_HOME o ANDROID_SDK_ROOT" >&2
    exit 1
fi

if ! command -v "$GO_BIN" >/dev/null 2>&1; then
    printf 'ERROR: no se encuentra Go: %s\n' "$GO_BIN" >&2
    exit 1
fi

if [[ -n "${JAVA_HOME:-}" ]]; then
    if [[ ! -x "$JAVA_HOME/bin/javac" ]]; then
        printf 'ERROR: JAVA_HOME no contiene un JDK utilizable: %s\n' "$JAVA_HOME" >&2
        exit 1
    fi
    export PATH="$JAVA_HOME/bin:$PATH"
fi
if ! command -v javac >/dev/null 2>&1; then
    printf '%s\n' "ERROR: gomobile bind requiere javac (JDK 17) en PATH o JAVA_HOME" >&2
    exit 1
fi

if [[ "${ALLOW_DIRTY_CORE:-0}" != "1" ]] && \
    [[ -n "$(git -C "$ROOT_DIR" status --porcelain -- VERSION.txt go.mod go.sum mobilebind internal third_party)" ]]; then
    printf '%s\n' "ERROR: el núcleo tiene cambios sin commit; el AAR oficial se construye desde HEAD" >&2
    exit 1
fi

SOURCE_FINGERPRINT=$(
    git -C "$ROOT_DIR" ls-tree -r HEAD -- \
        VERSION.txt \
        go.mod \
        go.sum \
        mobilebind \
        internal \
        third_party |
        sha256sum |
        awk '{print $1}'
)
# La ruta del módulo local forma parte de la sección Go build info de los .so.
# Una ruta aleatoria de mktemp haría que dos AAR del mismo commit tuviesen
# huellas distintas aunque el código fuese idéntico. La huella de los blobs Go
# proporciona una ruta estable incluso tras un commit solo documental, y mkdir
# actúa además como bloqueo atómico entre dos compilaciones simultáneas del
# mismo núcleo.
WORK_DIR="${TMPDIR:-/tmp}/grxfirma-gomobile-$SOURCE_FINGERPRINT"
if ! (umask 077 && mkdir "$WORK_DIR"); then
    printf 'ERROR: ya existe una compilación Android para el núcleo %s: %s\n' \
        "$SOURCE_FINGERPRINT" "$WORK_DIR" >&2
    exit 1
fi
cleanup() {
    rm -rf -- "$WORK_DIR"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$WORK_DIR/source" "$WORK_DIR/bin" "$(dirname -- "$OUTPUT")"
git -C "$ROOT_DIR" archive --format=tar HEAD | tar -xf - -C "$WORK_DIR/source"

(
    cd "$WORK_DIR/source"
    # La copia procede de git archive y, por diseño, no contiene .git. Go 1.26
    # intenta obtener metadatos VCS al construir los .so de gomobile y aborta
    # si el repositorio de origen usa un worktree. Desactivamos únicamente ese
    # sellado no reproducible; la revisión ya queda fijada por el archive de
    # HEAD y el AAR resultante se valida y publica por SHA-256.
    export GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false -trimpath"
    # This pinned revision supports Go 1.26 and predates the requirement to
    # modify the repository go.mod with a tool directive. Changes stay in the
    # isolated source copy used for the AAR build.
    "$GO_BIN" get "golang.org/x/mobile/bind@$GOMOBILE_VERSION"
    GOBIN="$WORK_DIR/bin" "$GO_BIN" install "golang.org/x/mobile/cmd/gomobile@$GOMOBILE_VERSION"
    GOBIN="$WORK_DIR/bin" "$GO_BIN" install "golang.org/x/mobile/cmd/gobind@$GOMOBILE_VERSION"
    "$GO_BIN" test ./mobilebind

    if ! PATH="$WORK_DIR/bin:$PATH" "$WORK_DIR/bin/gomobile" bind \
        -ldflags="-X grxfirma/mobilebind.engineVersion=$(cat VERSION.txt)" \
        -target=android \
        -androidapi="$ANDROID_MIN_API" \
        -o "$WORK_DIR/grxfirma.aar" \
        ./mobilebind; then
        cat >&2 <<'EOF'
ERROR: gomobile no pudo generar el AAR Android desde el commit actual.
Revise el diagnostico anterior, el JDK, el SDK/NDK y la compatibilidad bind de
mobilebind. No se crea ni conserva un artefacto parcial.
EOF
        exit 1
    fi
)

python3 "$SCRIPT_DIR/validate_core_aar.py" \
    --aar "$WORK_DIR/grxfirma.aar"
install -m 0644 "$WORK_DIR/grxfirma.aar" "$OUTPUT"
sha256sum "$OUTPUT"
