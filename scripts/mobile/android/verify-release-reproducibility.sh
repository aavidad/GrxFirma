#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
PROJECT_DIR="$ROOT_DIR/mobile/android"
WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-android-repro.XXXXXX")

cleanup() {
    rm -rf -- "$WORK_DIR"
}
trap cleanup EXIT HUP INT TERM

required=(
    GRXFIRMA_ANDROID_CORE_AAR
    GRXFIRMA_ANDROID_CORE_SHA256
    GRXFIRMA_ANDROID_KEYSTORE
    GRXFIRMA_ANDROID_KEYSTORE_PASSWORD
    GRXFIRMA_ANDROID_KEY_ALIAS
    GRXFIRMA_ANDROID_KEY_PASSWORD
    GRXFIRMA_ANDROID_SIGNING_CERT_SHA256
    GRXFIRMA_ANDROID_SOURCE_COMMIT
)
for name in "${required[@]}"; do
    if [[ -z "${!name:-}" ]]; then
        printf 'ERROR: falta %s para comprobar reproducibilidad Android\n' "$name" >&2
        exit 1
    fi
done

export SOURCE_DATE_EPOCH
SOURCE_DATE_EPOCH=$(git -C "$ROOT_DIR" show -s --format=%ct HEAD)
export TZ=UTC

build_and_copy() {
    local destination="$1"
    (
        cd "$PROJECT_DIR"
        ./gradlew --no-daemon --no-configuration-cache clean \
            assembleProductionRelease \
            bundleProductionRelease
    )
    install -m 0600 \
        "$PROJECT_DIR/app/build/outputs/apk/production/release/app-production-release.apk" \
        "$destination.apk"
    install -m 0600 \
        "$PROJECT_DIR/app/build/outputs/bundle/productionRelease/app-production-release.aab" \
        "$destination.aab"
}

build_and_copy "$WORK_DIR/first"
build_and_copy "$WORK_DIR/second"

for extension in apk aab; do
    if ! cmp -s "$WORK_DIR/first.$extension" "$WORK_DIR/second.$extension"; then
        printf 'ERROR: el %s Android no es reproducible con la misma clave QA\n' \
            "$extension" >&2
        sha256sum "$WORK_DIR/first.$extension" "$WORK_DIR/second.$extension" >&2
        exit 1
    fi
done

sha256sum "$WORK_DIR/first.apk" "$WORK_DIR/first.aab"
printf '%s\n' \
    "Release Android reproducible con sourceCommit y clave QA fijados."
