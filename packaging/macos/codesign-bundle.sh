#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

if [[ "$#" -ne 1 ]]; then
  echo "uso: codesign-bundle.sh APP.app" >&2
  exit 2
fi

: "${MACOS_CODESIGN_IDENTITY:?falta MACOS_CODESIGN_IDENTITY}"

bundle="$1"
if [[ ! -d "${bundle}" || -L "${bundle}" ]]; then
  echo "error: el bundle no existe o es un enlace simbolico: ${bundle}" >&2
  exit 1
fi
if ! command -v codesign >/dev/null 2>&1; then
  echo "error: codesign no esta disponible" >&2
  exit 1
fi
if ! command -v file >/dev/null 2>&1; then
  echo "error: file no esta disponible para identificar Mach-O" >&2
  exit 1
fi

if [[ "$(basename "${bundle}")" == "GrxFirma Desktop Qt.app" ]]; then
  for executable in \
    "${bundle}/Contents/MacOS/grxfirma-gui-qml" \
    "${bundle}/Contents/MacOS/grxfirma-gui" \
    "${bundle}/Contents/MacOS/grxfirma"; do
    if [[ ! -f "${executable}" || -L "${executable}" || ! -x "${executable}" ]]; then
      echo "error: el bundle Desktop Qt esta incompleto, ejecutable no valido: ${executable}" >&2
      exit 1
    fi
  done
fi

codesign_target() {
  local target="$1"
  if [[ "${MACOS_CODESIGN_IDENTITY}" == "-" ]]; then
    codesign \
      --force \
      --sign - \
      "${target}"
  else
    codesign \
      --force \
      --timestamp \
      --options runtime \
      --sign "${MACOS_CODESIGN_IDENTITY}" \
      "${target}"
  fi
  codesign --verify --strict --verbose=2 "${target}"
}

is_nested_code_bundle() {
  local target="$1"
  case "${target}" in
    *.app|*.appex|*.bundle|*.framework|*.plugin|*.xpc)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

# `find -depth` entrega primero los hijos. Esto es imprescindible para no
# invalidar la firma de un framework, plugin o extension al firmar despues uno
# de sus ejecutables internos.
while IFS= read -r -d '' target; do
  if [[ -L "${target}" ]]; then
    continue
  fi
  if [[ -f "${target}" ]]; then
    if file -b "${target}" | grep -q '^Mach-O'; then
      codesign_target "${target}"
    fi
    continue
  fi
  if [[ -d "${target}" ]] && is_nested_code_bundle "${target}"; then
    codesign_target "${target}"
  fi
done < <(find "${bundle}/Contents" -depth \( -type f -o -type d \) -print0)

# El contenedor principal siempre se firma al final, una vez sellado todo su
# codigo anidado.
codesign_target "${bundle}"
