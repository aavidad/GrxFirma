#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
IOS_DIR="$ROOT_DIR/mobile/ios"
SIGNING_XCCONFIG=${SIGNING_XCCONFIG:-"$IOS_DIR/Config/Signing.local.xcconfig"}
EXPORT_OPTIONS_PLIST=${EXPORT_OPTIONS_PLIST:-"$SCRIPT_DIR/ExportOptions.plist"}
OUTPUT_DIR=${1:-"$ROOT_DIR/release/ios"}

if [[ $(uname -s) != Darwin ]]; then
    printf '%s\n' "ERROR: el archivo iOS requiere macOS y Xcode" >&2
    exit 1
fi
for command in xcodebuild xcrun codesign security python3 ditto unzip shasum; do
    command -v "$command" >/dev/null 2>&1 || { printf 'ERROR: falta %s\n' "$command" >&2; exit 1; }
done
[[ -f "$EXPORT_OPTIONS_PLIST" ]] || {
    printf 'ERROR: cree un ExportOptions.plist privado desde %s\n' "$SCRIPT_DIR/ExportOptions.example.plist" >&2
    exit 1
}
if grep -q '__.*_REQUIRED__' "$EXPORT_OPTIONS_PLIST"; then
    printf '%s\n' "ERROR: ExportOptions.plist todavía contiene placeholders" >&2
    exit 1
fi

"$ROOT_DIR/scripts/mobile/ios/check-release-inputs.sh" --xcconfig "$SIGNING_XCCONFIG"
mkdir -p "$OUTPUT_DIR"
ARCHIVE="$OUTPUT_DIR/GrxFirma.xcarchive"
EXPORT_DIR="$OUTPUT_DIR/export"
rm -rf -- "$ARCHIVE" "$EXPORT_DIR"

xcodebuild archive \
    -project "$IOS_DIR/GrxFirma.xcodeproj" \
    -scheme GrxFirma \
    -configuration Release \
    -destination 'generic/platform=iOS' \
    -archivePath "$ARCHIVE" \
    -xcconfig "$SIGNING_XCCONFIG"
"$ROOT_DIR/scripts/mobile/ios/validate-archive.sh" "$ARCHIVE"

xcodebuild -exportArchive \
    -archivePath "$ARCHIVE" \
    -exportPath "$EXPORT_DIR" \
    -exportOptionsPlist "$EXPORT_OPTIONS_PLIST"

IPA=''
IPA_COUNT=0
for candidate in "$EXPORT_DIR/"*.ipa; do
    if [[ -f "$candidate" ]]; then IPA=$candidate; IPA_COUNT=$((IPA_COUNT + 1)); fi
done
[[ $IPA_COUNT -eq 1 ]] || { printf '%s\n' "ERROR: la exportación no produjo exactamente un IPA" >&2; exit 1; }
unzip -tq "$IPA" >/dev/null
"$ROOT_DIR/scripts/mobile/ios/validate-ipa.sh" "$IPA"
shasum -a 256 "$IPA" | tee "$OUTPUT_DIR/SHA256SUMS"
printf 'IPA listo para revisión y TestFlight: %s\n' "$IPA"
