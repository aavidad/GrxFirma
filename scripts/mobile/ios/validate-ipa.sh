#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
IPA=${1:?uso: validate-ipa.sh <aplicacion.ipa>}

if [[ $(uname -s) != Darwin ]]; then
    printf '%s\n' "ERROR: la validación criptográfica del IPA requiere macOS" >&2
    exit 1
fi
for command in python3 ditto; do
    command -v "$command" >/dev/null 2>&1 || { printf 'ERROR: falta %s\n' "$command" >&2; exit 1; }
done
[[ -f "$IPA" ]] || { printf 'ERROR: IPA ausente: %s\n' "$IPA" >&2; exit 1; }

WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-ios-ipa.XXXXXX")
cleanup() { rm -rf -- "$WORK_DIR"; }
trap cleanup EXIT HUP INT TERM

python3 - "$IPA" <<'PY'
import pathlib
import stat
import sys
import zipfile

ipa = pathlib.Path(sys.argv[1])
with zipfile.ZipFile(ipa) as archive:
    entries = archive.infolist()
    if not entries or len(entries) > 10_000:
        raise SystemExit("ERROR: el IPA tiene un número de entradas no permitido")
    if sum(entry.file_size for entry in entries) > 1024 * 1024 * 1024:
        raise SystemExit("ERROR: el IPA descomprimido supera 1 GiB")
    apps = set()
    for entry in entries:
        name = entry.filename
        path = pathlib.PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts or "\\" in name:
            raise SystemExit(f"ERROR: ruta insegura en IPA: {name}")
        if entry.flag_bits & 1:
            raise SystemExit("ERROR: el IPA contiene entradas cifradas")
        mode = (entry.external_attr >> 16) & 0o170000
        if mode == stat.S_IFLNK:
            raise SystemExit(f"ERROR: enlace simbólico no admitido en IPA: {name}")
        if mode not in {0, stat.S_IFREG, stat.S_IFDIR}:
            raise SystemExit(f"ERROR: tipo de entrada no admitido en IPA: {name}")
        if len(path.parts) >= 2 and path.parts[0] == "Payload" and path.parts[1].endswith(".app"):
            apps.add(path.parts[1])
    if len(apps) != 1:
        raise SystemExit("ERROR: el IPA debe contener exactamente una app bajo Payload")
PY

EXTRACTED="$WORK_DIR/extracted"
SYNTHETIC_ARCHIVE="$WORK_DIR/Exported.xcarchive"
mkdir -p "$EXTRACTED" "$SYNTHETIC_ARCHIVE/Products/Applications"
ditto -x -k "$IPA" "$EXTRACTED"

APP=''
APP_COUNT=0
for candidate in "$EXTRACTED/Payload/"*.app; do
    if [[ -d "$candidate" ]]; then APP=$candidate; APP_COUNT=$((APP_COUNT + 1)); fi
done
[[ $APP_COUNT -eq 1 ]] || { printf '%s\n' "ERROR: Payload no contiene exactamente una app" >&2; exit 1; }
mv -- "$APP" "$SYNTHETIC_ARCHIVE/Products/Applications/"

"$SCRIPT_DIR/validate-archive.sh" "$SYNTHETIC_ARCHIVE"
printf 'IPA iOS válido tras exportación: %s\n' "$IPA"
