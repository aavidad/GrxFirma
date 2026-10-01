#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
PROJECT_DIR="$ROOT_DIR/mobile/android"
SDK_ROOT=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}

if [[ -z "$SDK_ROOT" || ! -x "$SDK_ROOT/platform-tools/adb" ]]; then
    printf '%s\n' "ERROR: adb no está disponible en ANDROID_HOME/ANDROID_SDK_ROOT" >&2
    exit 1
fi

if ! "$SDK_ROOT/platform-tools/adb" devices | awk 'NR > 1 && $2 == "device" { found=1 } END { exit !found }'; then
    printf '%s\n' "ERROR: no hay un emulador o dispositivo Android listo" >&2
    exit 1
fi

(
    cd "$PROJECT_DIR"
    ./gradlew --no-daemon connectedVerificationDebugAndroidTest
)
