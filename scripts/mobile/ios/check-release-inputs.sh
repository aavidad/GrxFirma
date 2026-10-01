#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
IOS_DIR="$ROOT_DIR/mobile/ios"
FRAMEWORK="$IOS_DIR/Frameworks/Mobilebind.xcframework"
CHECKSUM="${FRAMEWORK}.sha256"
BUILD_PHASE=0
XC_CONFIG=${SIGNING_XCCONFIG:-"$IOS_DIR/Config/Signing.local.xcconfig"}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --build-phase) BUILD_PHASE=1; shift ;;
        --xcconfig) XC_CONFIG=${2:?falta ruta para --xcconfig}; shift 2 ;;
        *) printf 'ERROR: argumento desconocido: %s\n' "$1" >&2; exit 2 ;;
    esac
done

if [[ $(uname -s) != Darwin ]]; then
    printf '%s\n' "ERROR: el gate Release iOS solo puede ejecutarse en macOS" >&2
    exit 1
fi
for command in python3 security git xcodebuild; do
    command -v "$command" >/dev/null 2>&1 || {
        printf 'ERROR: herramienta obligatoria ausente: %s\n' "$command" >&2
        exit 1
    }
done
XCODE_MAJOR=$(xcodebuild -version | awk 'NR == 1 { split($2, version, "."); print version[1] }')
if [[ ! "$XCODE_MAJOR" =~ ^[0-9]+$ || "$XCODE_MAJOR" -lt 16 ]]; then
    printf '%s\n' "ERROR: Release requiere Xcode 16 o posterior" >&2
    exit 1
fi
if [[ ! -d "$FRAMEWORK" || ! -f "$CHECKSUM" ]]; then
    printf '%s\n' "ERROR: falta Mobilebind.xcframework o su checksum aprobado" >&2
    exit 1
fi
python3 "$SCRIPT_DIR/validate_core_xcframework.py" \
    --xcframework "$FRAMEWORK" \
    --sha256-file "$CHECKSUM"

if [[ $BUILD_PHASE -eq 0 ]] &&
    [[ -n $(git -C "$ROOT_DIR" status --porcelain -- mobile/ios scripts/mobile/ios packaging/mobile/ios) ]]; then
    printf '%s\n' "ERROR: el producto iOS contiene cambios sin commit" >&2
    exit 1
fi

if [[ $BUILD_PHASE -eq 0 ]]; then
    if [[ ! -f "$XC_CONFIG" ]]; then
        printf 'ERROR: falta la configuración privada de firma: %s\n' "$XC_CONFIG" >&2
        exit 1
    fi
    SETTINGS=$(mktemp "${TMPDIR:-/tmp}/grxfirma-ios-settings.XXXXXX")
    cleanup() { rm -f -- "$SETTINGS"; }
    trap cleanup EXIT HUP INT TERM
    xcodebuild -project "$IOS_DIR/GrxFirma.xcodeproj" \
        -target GrxFirma \
        -configuration Release \
        -sdk iphoneos \
        -xcconfig "$XC_CONFIG" \
        -showBuildSettings >"$SETTINGS"
fi

setting() {
    local name=$1
    if [[ $BUILD_PHASE -eq 1 ]]; then
        printf '%s' "${!name:-}"
    else
        awk -v key="$name" '$1 == key && $2 == "=" { $1=""; $2=""; sub(/^[[:space:]]+/, ""); print; exit }' "$SETTINGS"
    fi
}

require_setting() {
    local name=$1
    local value
    local unresolved="\$("
    value=$(setting "$name")
    if [[ -z "$value" || "$value" == *"$unresolved"* || "$value" == *'__'* ]]; then
        printf 'ERROR: valor Release ausente o placeholder: %s\n' "$name" >&2
        exit 1
    fi
    printf '%s' "$value"
}

TEAM_ID=$(require_setting DEVELOPMENT_TEAM)
APP_ID=$(require_setting PRODUCT_BUNDLE_IDENTIFIER)
APP_GROUP=$(require_setting GRXFIRMA_APP_GROUP)
KEYCHAIN_GROUP=$(require_setting GRXFIRMA_KEYCHAIN_GROUP)
SHARE_ID=$(require_setting GRXFIRMA_SHARE_BUNDLE_IDENTIFIER)
require_setting PROVISIONING_PROFILE_SPECIFIER >/dev/null
require_setting GRXFIRMA_SHARE_PROVISIONING_PROFILE >/dev/null
CORE_MODE=$(require_setting GRXFIRMA_CORE_MODE)
IDENTITY=$(require_setting CODE_SIGN_IDENTITY)
SIGN_STYLE=$(require_setting CODE_SIGN_STYLE)
SWIFT_CONDITIONS=$(require_setting SWIFT_ACTIVE_COMPILATION_CONDITIONS)
OBJC_DEFINITIONS=$(require_setting GCC_PREPROCESSOR_DEFINITIONS)

[[ "$TEAM_ID" =~ ^[A-Z0-9]{10}$ ]] || { printf '%s\n' "ERROR: DEVELOPMENT_TEAM inválido" >&2; exit 1; }
[[ "$APP_ID" =~ ^[A-Za-z0-9.-]+$ ]] || { printf '%s\n' "ERROR: Bundle ID de app inválido" >&2; exit 1; }
[[ "$SHARE_ID" == "$APP_ID.share" ]] || { printf '%s\n' "ERROR: el Bundle ID Share debe ser <app>.share" >&2; exit 1; }
[[ "$APP_GROUP" == "group.$APP_ID" ]] || { printf '%s\n' "ERROR: el App Group debe ser group.<bundle-id>" >&2; exit 1; }
[[ "$KEYCHAIN_GROUP" == "$APP_ID.keys" ]] || { printf '%s\n' "ERROR: el Keychain Group debe ser <bundle-id>.keys" >&2; exit 1; }
[[ "$CORE_MODE" == production ]] || { printf '%s\n' "ERROR: Release no usa el núcleo production" >&2; exit 1; }
[[ "$SIGN_STYLE" == Manual ]] || { printf '%s\n' "ERROR: Release no exige firma manual" >&2; exit 1; }
[[ "$IDENTITY" == *"Apple Distribution"* ]] || { printf '%s\n' "ERROR: Release no usa Apple Distribution" >&2; exit 1; }
[[ "$SWIFT_CONDITIONS" == *PRODUCTION_CORE* ]] || { printf '%s\n' "ERROR: Release no activa PRODUCTION_CORE" >&2; exit 1; }
[[ "$OBJC_DEFINITIONS" == *GRXFIRMA_PRODUCTION_CORE=1* ]] || {
    printf '%s\n' "ERROR: Release no activa GRXFIRMA_PRODUCTION_CORE" >&2
    exit 1
}

if [[ $BUILD_PHASE -eq 0 ]] &&
    ! security find-identity -v -p codesigning | grep -F "Apple Distribution" >/dev/null; then
    printf '%s\n' "ERROR: no hay una identidad Apple Distribution utilizable en el llavero" >&2
    exit 1
fi
printf 'Gate Release iOS correcto para %s (Team %s)\n' "$APP_ID" "$TEAM_ID"
