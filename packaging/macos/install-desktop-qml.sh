#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
umask 077

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${HOME:-}" || "${HOME}" != /* || "${HOME}" == "/" || -L "${HOME}" ]]; then
  echo "error: HOME debe ser una ruta absoluta de usuario y no simbolica" >&2
  exit 1
fi
SRC_APP="${SELF_DIR}/GrxFirma Desktop Qt.app"
DST_APP="${HOME}/Applications/GrxFirma Desktop Qt.app"

ensure_no_symlink_ancestors() {
  local target="$1"
  local current
  current="$(dirname "${target}")"
  while [[ "${current}" != "${HOME}" ]]; do
    if [[ -L "${current}" ]]; then
      echo "error: la ruta de instalacion contiene un enlace simbolico: ${current}" >&2
      return 1
    fi
    current="$(dirname "${current}")"
    if [[ "${current}" != "${HOME}" && "${current}" != "${HOME}/"* ]]; then
      echo "error: ruta de instalacion fuera de HOME: ${target}" >&2
      return 1
    fi
  done
}

if [[ ! -d "$SRC_APP" || -L "$SRC_APP" ]]; then
  echo "error: no se encuentra GrxFirma Desktop Qt.app junto al instalador" >&2
  exit 1
fi
if [[ -L "${SRC_APP}/Contents" ||
      -L "${SRC_APP}/Contents/MacOS" ||
      -L "${SRC_APP}/Contents/Resources" ||
      ! -f "${SRC_APP}/Contents/Info.plist" ||
      -L "${SRC_APP}/Contents/Info.plist" ||
      ! -f "${SRC_APP}/Contents/MacOS/grxfirma-gui-qml" ||
      -L "${SRC_APP}/Contents/MacOS/grxfirma-gui-qml" ||
      ! -x "${SRC_APP}/Contents/MacOS/grxfirma-gui-qml" ||
      ! -f "${SRC_APP}/Contents/MacOS/grxfirma-gui" ||
      -L "${SRC_APP}/Contents/MacOS/grxfirma-gui" ||
      ! -x "${SRC_APP}/Contents/MacOS/grxfirma-gui" ||
      ! -f "${SRC_APP}/Contents/MacOS/grxfirma" ||
      -L "${SRC_APP}/Contents/MacOS/grxfirma" ||
      ! -x "${SRC_APP}/Contents/MacOS/grxfirma" ||
      ! -f "${SRC_APP}/Contents/Resources/VERSION.txt" ||
      -L "${SRC_APP}/Contents/Resources/VERSION.txt" ]]; then
  echo "error: el paquete GrxFirma Desktop Qt.app esta incompleto" >&2
  exit 1
fi

ensure_no_symlink_ancestors "${DST_APP}"
mkdir -p "${HOME}/Applications"
stage_dir="$(mktemp -d "${HOME}/Applications/.grxfirma-desktop-install.XXXXXX")"
cleanup() {
  rm -rf "${stage_dir}"
}
trap cleanup EXIT

new_app="${stage_dir}/GrxFirma Desktop Qt.app"
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
  echo "error: no se pudo sustituir la aplicacion Desktop Qt/QML" >&2
  exit 1
fi
rm -rf "$previous_app"

echo "Desktop Qt/QML instalado en: $DST_APP"
