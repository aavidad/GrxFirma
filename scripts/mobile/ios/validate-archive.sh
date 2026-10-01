#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ARCHIVE=${1:?uso: validate-archive.sh <archivo.xcarchive>}
if [[ $(uname -s) != Darwin ]]; then
    printf '%s\n' "ERROR: la validación criptográfica del archivo requiere macOS" >&2
    exit 1
fi
for command in codesign security plutil python3; do
    command -v "$command" >/dev/null 2>&1 || { printf 'ERROR: falta %s\n' "$command" >&2; exit 1; }
done
[[ -d "$ARCHIVE" ]] || { printf 'ERROR: archivo ausente: %s\n' "$ARCHIVE" >&2; exit 1; }

APP=''
APP_COUNT=0
for candidate in "$ARCHIVE/Products/Applications/"*.app; do
    if [[ -d "$candidate" ]]; then APP=$candidate; APP_COUNT=$((APP_COUNT + 1)); fi
done
[[ $APP_COUNT -eq 1 ]] || { printf '%s\n' "ERROR: el xcarchive debe contener exactamente una app" >&2; exit 1; }
SHARE=''
SHARE_COUNT=0
for candidate in "$APP/PlugIns/"*.appex; do
    if [[ -d "$candidate" ]]; then SHARE=$candidate; SHARE_COUNT=$((SHARE_COUNT + 1)); fi
done
[[ $SHARE_COUNT -eq 1 ]] || { printf '%s\n' "ERROR: la app debe contener exactamente una extensión Share" >&2; exit 1; }

codesign --verify --deep --strict --verbose=2 "$APP"
codesign --verify --strict --verbose=2 "$SHARE"

WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-ios-archive.XXXXXX")
cleanup() { rm -rf -- "$WORK_DIR"; }
trap cleanup EXIT HUP INT TERM
codesign -d --entitlements :- "$APP" >"$WORK_DIR/app-entitlements.plist" 2>/dev/null
codesign -d --entitlements :- "$SHARE" >"$WORK_DIR/share-entitlements.plist" 2>/dev/null
security cms -D -i "$APP/embedded.mobileprovision" >"$WORK_DIR/app-profile.plist"
security cms -D -i "$SHARE/embedded.mobileprovision" >"$WORK_DIR/share-profile.plist"
plutil -lint \
    "$APP/Info.plist" \
    "$SHARE/Info.plist" \
    "$WORK_DIR/app-entitlements.plist" \
    "$WORK_DIR/share-entitlements.plist" \
    "$WORK_DIR/app-profile.plist" \
    "$WORK_DIR/share-profile.plist" >/dev/null

APP_ID=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP/Info.plist")
SHARE_ID=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$SHARE/Info.plist")
TEAM_ID=$(/usr/libexec/PlistBuddy -c 'Print :TeamIdentifier:0' "$WORK_DIR/app-profile.plist")
SHARE_TEAM_ID=$(/usr/libexec/PlistBuddy -c 'Print :TeamIdentifier:0' "$WORK_DIR/share-profile.plist")
APP_PREFIX=$(/usr/libexec/PlistBuddy -c 'Print :ApplicationIdentifierPrefix:0' "$WORK_DIR/app-profile.plist")
SHARE_PREFIX=$(/usr/libexec/PlistBuddy -c 'Print :ApplicationIdentifierPrefix:0' "$WORK_DIR/share-profile.plist")
PROFILE_APP_ID=$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:application-identifier' "$WORK_DIR/app-profile.plist")
PROFILE_SHARE_ID=$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:application-identifier' "$WORK_DIR/share-profile.plist")

[[ "$SHARE_ID" == "$APP_ID.share" ]] || { printf '%s\n' "ERROR: Bundle ID Share incoherente" >&2; exit 1; }
[[ "$SHARE_TEAM_ID" == "$TEAM_ID" ]] || { printf '%s\n' "ERROR: app y Share usan Team ID distintos" >&2; exit 1; }
[[ "$SHARE_PREFIX" == "$APP_PREFIX" ]] || { printf '%s\n' "ERROR: app y Share usan prefijos de aplicación distintos" >&2; exit 1; }
[[ "$PROFILE_APP_ID" == "$APP_PREFIX.$APP_ID" ]] || { printf '%s\n' "ERROR: perfil app no coincide con prefijo/Bundle" >&2; exit 1; }
[[ "$PROFILE_SHARE_ID" == "$APP_PREFIX.$SHARE_ID" ]] || { printf '%s\n' "ERROR: perfil Share no coincide con prefijo/Bundle" >&2; exit 1; }

for entitlements in "$WORK_DIR/app-entitlements.plist" "$WORK_DIR/share-entitlements.plist"; do
    if /usr/libexec/PlistBuddy -c 'Print :get-task-allow' "$entitlements" 2>/dev/null | grep -qx true; then
        printf '%s\n' "ERROR: el archivo de distribución contiene get-task-allow" >&2
        exit 1
    fi
done
DATA_PROTECTION=$(/usr/libexec/PlistBuddy -c 'Print :com.apple.developer.default-data-protection' "$WORK_DIR/app-entitlements.plist")
SHARE_DATA_PROTECTION=$(/usr/libexec/PlistBuddy -c 'Print :com.apple.developer.default-data-protection' "$WORK_DIR/share-entitlements.plist")
[[ "$DATA_PROTECTION" == NSFileProtectionComplete ]] || { printf '%s\n' "ERROR: falta protección completa" >&2; exit 1; }
[[ "$SHARE_DATA_PROTECTION" == NSFileProtectionComplete ]] || { printf '%s\n' "ERROR: Share no tiene protección completa" >&2; exit 1; }
APP_GROUP=$(/usr/libexec/PlistBuddy -c 'Print :com.apple.security.application-groups:0' "$WORK_DIR/app-entitlements.plist")
SHARE_GROUP=$(/usr/libexec/PlistBuddy -c 'Print :com.apple.security.application-groups:0' "$WORK_DIR/share-entitlements.plist")
[[ "$APP_GROUP" == "$SHARE_GROUP" && "$APP_GROUP" == "group.$APP_ID" ]] || {
    printf '%s\n' "ERROR: App Group firmado incoherente" >&2
    exit 1
}
SIGNED_APP_ID=$(/usr/libexec/PlistBuddy -c 'Print :application-identifier' "$WORK_DIR/app-entitlements.plist")
SIGNED_SHARE_ID=$(/usr/libexec/PlistBuddy -c 'Print :application-identifier' "$WORK_DIR/share-entitlements.plist")
SIGNED_APP_TEAM=$(/usr/libexec/PlistBuddy -c 'Print :com.apple.developer.team-identifier' "$WORK_DIR/app-entitlements.plist")
SIGNED_SHARE_TEAM=$(/usr/libexec/PlistBuddy -c 'Print :com.apple.developer.team-identifier' "$WORK_DIR/share-entitlements.plist")
[[ "$SIGNED_APP_TEAM" == "$TEAM_ID" && "$SIGNED_SHARE_TEAM" == "$TEAM_ID" ]] || { printf '%s\n' "ERROR: Team ID firmado incoherente" >&2; exit 1; }
[[ "$SIGNED_APP_ID" == "$APP_PREFIX.$APP_ID" ]] || { printf '%s\n' "ERROR: application-identifier de app incoherente" >&2; exit 1; }
[[ "$SIGNED_SHARE_ID" == "$APP_PREFIX.$SHARE_ID" ]] || { printf '%s\n' "ERROR: application-identifier de Share incoherente" >&2; exit 1; }
KEYCHAIN_GROUP=$(/usr/libexec/PlistBuddy -c 'Print :keychain-access-groups:0' "$WORK_DIR/app-entitlements.plist")
[[ "$KEYCHAIN_GROUP" == "$APP_PREFIX.$APP_ID.keys" ]] || { printf '%s\n' "ERROR: Keychain Group firmado incoherente" >&2; exit 1; }
PROFILE_APP_GROUP=$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:com.apple.security.application-groups:0' "$WORK_DIR/app-profile.plist")
PROFILE_SHARE_GROUP=$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:com.apple.security.application-groups:0' "$WORK_DIR/share-profile.plist")
[[ "$PROFILE_APP_GROUP" == "$APP_GROUP" && "$PROFILE_SHARE_GROUP" == "$APP_GROUP" ]] || {
    printf '%s\n' "ERROR: los perfiles no autorizan el App Group firmado" >&2
    exit 1
}

for key in NSAllowsArbitraryLoads NSAllowsArbitraryLoadsForMedia NSAllowsArbitraryLoadsInWebContent NSAllowsLocalNetworking; do
    value=$(/usr/libexec/PlistBuddy -c "Print :NSAppTransportSecurity:$key" "$APP/Info.plist")
    [[ "$value" == false ]] || { printf 'ERROR: ATS relajado en archivo: %s\n' "$key" >&2; exit 1; }
done
if /usr/libexec/PlistBuddy -c 'Print :NSAppTransportSecurity:NSExceptionDomains' "$APP/Info.plist" >/dev/null 2>&1; then
    printf '%s\n' "ERROR: el archivo contiene excepciones ATS" >&2
    exit 1
fi
[[ -f "$APP/PrivacyInfo.xcprivacy" && -f "$SHARE/PrivacyInfo.xcprivacy" ]] || {
    printf '%s\n' "ERROR: falta PrivacyInfo.xcprivacy en app o Share" >&2
    exit 1
}
plutil -lint "$APP/PrivacyInfo.xcprivacy" "$SHARE/PrivacyInfo.xcprivacy" >/dev/null

EXPIRATION=$(python3 - "$WORK_DIR/app-profile.plist" "$WORK_DIR/share-profile.plist" <<'PY'
import datetime
import plistlib
import sys

now = datetime.datetime.now(datetime.timezone.utc)
expirations = []
for path in sys.argv[1:]:
    with open(path, "rb") as handle:
        expiration = plistlib.load(handle).get("ExpirationDate")
    if not isinstance(expiration, datetime.datetime):
        raise SystemExit(f"ERROR: el perfil no declara ExpirationDate: {path}")
    if expiration.tzinfo is None:
        expiration = expiration.replace(tzinfo=datetime.timezone.utc)
    if expiration <= now:
        raise SystemExit(f"ERROR: perfil caducado: {path}")
    expirations.append(expiration)
print(min(expirations).isoformat())
PY
)
printf 'xcarchive iOS válido: app=%s share=%s team=%s perfil_hasta=%s\n' "$APP_ID" "$SHARE_ID" "$TEAM_ID" "$EXPIRATION"
