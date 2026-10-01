#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
export PYTHONDONTWRITEBYTECODE=1

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
IOS_DIR="$ROOT_DIR/mobile/ios"

for command in ruby python3 shellcheck diff; do
    command -v "$command" >/dev/null 2>&1 || { printf 'ERROR: falta %s\n' "$command" >&2; exit 1; }
done
ruby -c "$SCRIPT_DIR/generate-xcode-project.rb" >/dev/null

while IFS= read -r script; do
    bash -n "$script"
    shellcheck -x "$script"
done < <(find "$SCRIPT_DIR" "$ROOT_DIR/packaging/mobile/ios" -type f -name '*.sh' -print | sort)

python3 "$SCRIPT_DIR/validate_project.py" --ios-root "$IOS_DIR"
python3 -m unittest discover -s "$SCRIPT_DIR/tests" -p 'test_*.py' -v

TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-ios-project.XXXXXX")
cleanup() { rm -rf -- "$TEMP_DIR"; }
trap cleanup EXIT HUP INT TERM
ruby "$SCRIPT_DIR/generate-xcode-project.rb" --output "$TEMP_DIR/GrxFirma.xcodeproj" >/dev/null
diff -ru "$IOS_DIR/GrxFirma.xcodeproj" "$TEMP_DIR/GrxFirma.xcodeproj"

if [[ $(uname -s) == Darwin ]] && command -v xcodebuild >/dev/null 2>&1; then
    xcodebuild -project "$IOS_DIR/GrxFirma.xcodeproj" -scheme GrxFirma -list
    if [[ ${RUN_XCODE_TESTS:-0} == 1 ]]; then
        xcodebuild test \
            -project "$IOS_DIR/GrxFirma.xcodeproj" \
            -scheme GrxFirma \
            -configuration Debug \
            -destination 'platform=iOS Simulator,name=iPhone 16' \
            CODE_SIGNING_ALLOWED=NO
    fi
else
    printf '%s\n' "INFO: validación Xcode omitida; requiere macOS con Xcode"
fi
