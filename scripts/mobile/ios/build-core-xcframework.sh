#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
IOS_DIR="$ROOT_DIR/mobile/ios"
OUTPUT=${1:-"$IOS_DIR/Frameworks/Mobilebind.xcframework"}
CHECKSUM="${OUTPUT}.sha256"
GO_BIN=${GO_BIN:-go}
GOMOBILE_VERSION=${GOMOBILE_VERSION:-v0.0.0-20260217195705-b56b3793a9c4}
IOS_MIN_VERSION=${IOS_MIN_VERSION:-16.0}

if [[ $(uname -s) != Darwin ]]; then
    printf '%s\n' "ERROR: gomobile para iOS requiere macOS y Xcode completo" >&2
    exit 1
fi
for command in git xcodebuild xcrun python3 ditto "$GO_BIN"; do
    if ! command -v "$command" >/dev/null 2>&1; then
        printf 'ERROR: herramienta obligatoria ausente: %s\n' "$command" >&2
        exit 1
    fi
done
XCODE_MAJOR=$(xcodebuild -version | awk 'NR == 1 { split($2, version, "."); print version[1] }')
if [[ ! "$XCODE_MAJOR" =~ ^[0-9]+$ || "$XCODE_MAJOR" -lt 16 ]]; then
    printf '%s\n' "ERROR: el XCFramework requiere Xcode 16 o posterior" >&2
    exit 1
fi
if [[ "${ALLOW_DIRTY_CORE:-0}" != 1 ]] &&
    [[ -n $(git -C "$ROOT_DIR" status --porcelain -- go.mod go.sum mobilebind internal third_party) ]]; then
    printf '%s\n' "ERROR: el núcleo Go tiene cambios sin commit" >&2
    exit 1
fi

case "$OUTPUT" in
    "$IOS_DIR"/*) ;;
    *) printf '%s\n' "ERROR: la salida debe permanecer bajo mobile/ios" >&2; exit 1 ;;
esac

WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-ios-core.XXXXXX")
cleanup() {
    rm -rf -- "$WORK_DIR"
}
trap cleanup EXIT HUP INT TERM
mkdir -p "$WORK_DIR/source" "$WORK_DIR/bin" "$WORK_DIR/output" "$(dirname -- "$OUTPUT")"
git -C "$ROOT_DIR" archive --format=tar HEAD | tar -xf - -C "$WORK_DIR/source"

(
    cd "$WORK_DIR/source"
    "$GO_BIN" get "golang.org/x/mobile/bind@$GOMOBILE_VERSION"
    GOBIN="$WORK_DIR/bin" "$GO_BIN" install "golang.org/x/mobile/cmd/gomobile@$GOMOBILE_VERSION"
    GOBIN="$WORK_DIR/bin" "$GO_BIN" install "golang.org/x/mobile/cmd/gobind@$GOMOBILE_VERSION"
    PATH="$WORK_DIR/bin:$PATH" "$WORK_DIR/bin/gomobile" init
    if ! PATH="$WORK_DIR/bin:$PATH" "$WORK_DIR/bin/gomobile" bind \
        -target=ios \
        -iosversion="$IOS_MIN_VERSION" \
        -trimpath \
        -o "$WORK_DIR/output/Mobilebind.xcframework" \
        ./mobilebind; then
        cat >&2 <<'EOF'
ERROR: mobilebind no puede producir el XCFramework iOS operativo.
La fachada Go debe ser bindable y proporcionar NewIOSFacade(appSupportDir,
appGroupDir, keychainAccessGroup), MobileContractJSON, ClearSession y los
servicios críticos reales. No se genera un framework vacío ni una fachada nil.
EOF
        exit 1
    fi
)

python3 "$SCRIPT_DIR/validate_core_xcframework.py" \
    --xcframework "$WORK_DIR/output/Mobilebind.xcframework" \
    --write-sha256 "$WORK_DIR/output/Mobilebind.xcframework.sha256"

STAGED="${OUTPUT}.new"
rm -rf -- "$STAGED"
ditto "$WORK_DIR/output/Mobilebind.xcframework" "$STAGED"
rm -rf -- "$OUTPUT"
mv -- "$STAGED" "$OUTPUT"
install -m 0600 "$WORK_DIR/output/Mobilebind.xcframework.sha256" "$CHECKSUM"
python3 "$SCRIPT_DIR/validate_core_xcframework.py" \
    --xcframework "$OUTPUT" \
    --sha256-file "$CHECKSUM"
