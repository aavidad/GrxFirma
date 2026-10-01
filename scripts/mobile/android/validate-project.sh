#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
PROJECT_DIR="$ROOT_DIR/mobile/android"

if [[ -n "${JAVA_HOME:-}" ]]; then
    export PATH="$JAVA_HOME/bin:$PATH"
fi

python3 "$SCRIPT_DIR/validate_project.py"
python3 -m unittest discover -s "$SCRIPT_DIR/tests" -p 'test_*.py'

(
    cd "$PROJECT_DIR"
    ./gradlew --no-daemon \
        testVerificationDebugUnitTest \
        lintVerificationDebug \
        assembleVerificationDebug \
        assembleVerificationDebugAndroidTest \
        lintVerificationRelease \
        assembleVerificationRelease
)

if [[ "${RUN_ANDROID_DEVICE_TESTS:-0}" == "1" ]]; then
    "$SCRIPT_DIR/test-device.sh"
fi

printf '%s\n' "Validación Android completada"
