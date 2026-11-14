#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
PROJECT_DIR="$ROOT_DIR/mobile/android"

# No se genera ni se usa ninguna clave. Gradle también rechaza credenciales
# externas en modo F-Droid, antes de configurar las tareas de compilación.
for name in GRXFIRMA_ANDROID_KEYSTORE GRXFIRMA_ANDROID_KEYSTORE_PASSWORD \
    GRXFIRMA_ANDROID_KEY_ALIAS GRXFIRMA_ANDROID_KEY_PASSWORD; do
    if [[ -v "$name" ]]; then
        printf 'ERROR: retire %s para comprobar el APK sin firma\n' "$name" >&2
        exit 1
    fi
done

export GRXFIRMA_FDROID_BUILD=1 TZ=UTC
export GRXFIRMA_ANDROID_SOURCE_COMMIT SOURCE_DATE_EPOCH
GRXFIRMA_ANDROID_SOURCE_COMMIT=$(git -C "$ROOT_DIR" rev-parse HEAD)
SOURCE_DATE_EPOCH=$(git -C "$ROOT_DIR" show -s --format=%ct HEAD)

# Las dos compilaciones enlazan el mismo AAR, que debe estar validado y fijado
# por SHA-256. verifyProductionCore vuelve a comprobarlo en cada pasada.
OUTPUT_DIR=${1:-$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-unsigned-repro.XXXXXX")}
mkdir -p -- "$OUTPUT_DIR"
OUTPUT_DIR=$(CDPATH='' cd -- "$OUTPUT_DIR" && pwd)
for name in first.apk second.apk SHA256SUMS.txt; do
    if [[ -e "$OUTPUT_DIR/$name" ]]; then
        printf 'ERROR: ya existe una evidencia: %s/%s\n' "$OUTPUT_DIR" "$name" >&2
        exit 1
    fi
done
printf 'Evidencias de reproducibilidad: %s\n' "$OUTPUT_DIR"

for name in first second; do
    (
        cd "$PROJECT_DIR"
        ./gradlew --no-daemon --no-configuration-cache --no-build-cache \
            clean assembleProductionRelease
    )
    install -m 0644 \
        "$PROJECT_DIR/app/build/outputs/apk/production/release/app-production-release-unsigned.apk" \
        "$OUTPUT_DIR/$name.apk"
done

(
    cd "$OUTPUT_DIR"
    sha256sum first.apk second.apk > SHA256SUMS.txt
    cat SHA256SUMS.txt
    if ! cmp -s first.apk second.apk; then
        printf '%s\n' "ERROR: los dos APK sin firma son distintos; se conservan para analizarlos" >&2
        exit 1
    fi
)
printf '%s\n' "Los dos APK de producción sin firma son idénticos byte a byte."
