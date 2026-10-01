#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
PROJECT_DIR="$ROOT_DIR/mobile/android"
VALIDATOR="$ROOT_DIR/scripts/mobile/android/validate_core_aar.py"
DIST_DIR=${DIST_DIR:-"$SCRIPT_DIR/dist"}

if [[ -n "${JAVA_HOME:-}" ]]; then
    export PATH="$JAVA_HOME/bin:$PATH"
fi
SDK_ROOT=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
if [[ -z "$SDK_ROOT" ]]; then
    printf '%s\n' "ERROR: defina ANDROID_HOME o ANDROID_SDK_ROOT" >&2
    exit 1
fi

required=(
    GRXFIRMA_ANDROID_CORE_AAR
    GRXFIRMA_ANDROID_CORE_SHA256
    GRXFIRMA_ANDROID_KEYSTORE
    GRXFIRMA_ANDROID_KEYSTORE_PASSWORD
    GRXFIRMA_ANDROID_KEY_ALIAS
    GRXFIRMA_ANDROID_KEY_PASSWORD
)
for name in "${required[@]}"; do
    if [[ -z "${!name:-}" ]]; then
        printf 'ERROR: falta %s\n' "$name" >&2
        exit 1
    fi
done

python3 "$VALIDATOR" \
    --aar "$GRXFIRMA_ANDROID_CORE_AAR" \
    --sha256 "$GRXFIRMA_ANDROID_CORE_SHA256"

(
    cd "$PROJECT_DIR"
    ./gradlew --no-daemon --no-configuration-cache \
        testProductionDebugUnitTest \
        lintProductionRelease \
        bundleProductionRelease \
        assembleProductionRelease
)

APK="$PROJECT_DIR/app/build/outputs/apk/production/release/app-production-release.apk"
AAB="$PROJECT_DIR/app/build/outputs/bundle/productionRelease/app-production-release.aab"
APKSIGNER="$SDK_ROOT/build-tools/36.0.0/apksigner"
ZIPALIGN="$SDK_ROOT/build-tools/36.0.0/zipalign"

for artifact in "$APK" "$AAB" "$APKSIGNER" "$ZIPALIGN"; do
    if [[ ! -f "$artifact" && ! -x "$artifact" ]]; then
        printf 'ERROR: artefacto o herramienta ausente: %s\n' "$artifact" >&2
        exit 1
    fi
done

"$APKSIGNER" verify --verbose --print-certs "$APK"
"$ZIPALIGN" -c -P 16 4 "$APK"
jarsigner -verify -strict "$AAB"

mkdir -p "$DIST_DIR"
VERSION=$(tr -d '\r\n' < "$ROOT_DIR/VERSION.txt")
APK_NAME="GrxFirma-${VERSION}-android.apk"
AAB_NAME="GrxFirma-${VERSION}-android.aab"
install -m 0644 "$APK" "$DIST_DIR/$APK_NAME"
install -m 0644 "$AAB" "$DIST_DIR/$AAB_NAME"
(
    cd "$DIST_DIR"
    sha256sum "$APK_NAME" "$AAB_NAME" > SHA256SUMS
)
printf 'Release Android validada en %s\n' "$DIST_DIR"
