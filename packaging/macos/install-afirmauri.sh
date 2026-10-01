#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
umask 077

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_APP="${SELF_DIR}/GrxFirma AfirmaURI.app"
DST_APP="${HOME}/Applications/GrxFirma AfirmaURI.app"

if [[ ! -d "$SRC_APP" || -L "$SRC_APP" ]]; then
  echo "error: no se encuentra GrxFirma AfirmaURI.app junto al instalador" >&2
  exit 1
fi
if [[ ! -f "${SRC_APP}/Contents/Info.plist" ||
      -L "${SRC_APP}/Contents/Info.plist" ||
      ! -f "${SRC_APP}/Contents/MacOS/grxfirma-afirmauri" ||
      -L "${SRC_APP}/Contents/MacOS/grxfirma-afirmauri" ||
      ! -x "${SRC_APP}/Contents/MacOS/grxfirma-afirmauri" ]]; then
  echo "error: el paquete GrxFirma AfirmaURI.app esta incompleto" >&2
  exit 1
fi

mkdir -p "${HOME}/Applications"
stage_dir="$(mktemp -d "${HOME}/Applications/.grxfirma-install.XXXXXX")"
cleanup() {
  rm -rf "${stage_dir}"
}
trap cleanup EXIT

new_app="${stage_dir}/GrxFirma AfirmaURI.app"
previous_app="${stage_dir}/previous.app"
if command -v ditto >/dev/null 2>&1; then
  ditto "$SRC_APP" "$new_app"
else
  cp -R "$SRC_APP" "$new_app"
fi

had_previous=0
if [[ -e "$DST_APP" || -L "$DST_APP" ]]; then
  mv "$DST_APP" "$previous_app"
  had_previous=1
fi
if ! mv "$new_app" "$DST_APP"; then
  if [[ "$had_previous" == "1" ]]; then
    mv "$previous_app" "$DST_APP"
  fi
  echo "error: no se pudo sustituir el handler afirma://" >&2
  exit 1
fi
rm -rf "$previous_app"

LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
if [[ -x "$LSREGISTER" ]]; then
  "$LSREGISTER" -f "$DST_APP" >/dev/null 2>&1 || true
fi

echo "Handler afirma:// instalado en: $DST_APP"
