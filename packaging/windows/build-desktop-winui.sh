#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
POWERSHELL_SCRIPT="${ROOT_DIR}/packaging/windows/build-desktop-winui.ps1"

usage() {
  cat <<'EOF'
Uso:
  packaging/windows/build-desktop-winui.sh [--project RUTA] [--output RUTA]

Este lanzador ejecuta el empaquetador PowerShell en Windows. WinUI 3 necesita
el compilador XAML y el Windows SDK, por lo que Linux nativo no es un host de
compilacion soportado. Se admite Git Bash, MSYS2, Cygwin y WSL cuando pueden
invocar powershell.exe del Windows anfitrion.

No acepta credenciales, opciones de firma, NSIS ni integracion con la Suite.
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

to_windows_path() {
  local path="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "${path}"
    return
  fi
  if command -v wslpath >/dev/null 2>&1; then
    wslpath -w "${path}"
    return
  fi
  printf '%s\n' "${path}"
}

PROJECT_PATH=""
OUTPUT_DIRECTORY=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --project)
      [[ "$#" -ge 2 ]] || fail "--project requiere una ruta"
      PROJECT_PATH="$2"
      shift 2
      ;;
    --output)
      [[ "$#" -ge 2 ]] || fail "--output requiere una ruta"
      OUTPUT_DIRECTORY="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      fail "argumento no soportado: $1"
      ;;
  esac
done

[[ -f "${POWERSHELL_SCRIPT}" ]] || fail "no existe ${POWERSHELL_SCRIPT}"
[[ ! -L "${POWERSHELL_SCRIPT}" ]] || fail "el empaquetador PowerShell no puede ser un enlace"
[[ -f "${ROOT_DIR}/global.json" ]] || fail "falta global.json"
[[ ! -L "${ROOT_DIR}/global.json" ]] || fail "global.json no puede ser un enlace"

case "$(uname -s 2>/dev/null || true)" in
  MINGW*|MSYS*|CYGWIN*|Linux*)
    ;;
  *)
    fail "host no soportado para el lanzador WinUI"
    ;;
esac

if ! command -v powershell.exe >/dev/null 2>&1; then
  fail "WinUI solo se compila en Windows; no se encontro powershell.exe"
fi

PS_SCRIPT_WINDOWS="$(to_windows_path "${POWERSHELL_SCRIPT}")"
PS_ARGS=(
  -NoLogo
  -NoProfile
  -NonInteractive
  -ExecutionPolicy Bypass
  -File "${PS_SCRIPT_WINDOWS}"
)
if [[ -n "${PROJECT_PATH}" ]]; then
  PROJECT_ABSOLUTE="$(cd "$(dirname "${PROJECT_PATH}")" && pwd)/$(basename "${PROJECT_PATH}")"
  [[ -f "${PROJECT_ABSOLUTE}" ]] || fail "no existe el proyecto: ${PROJECT_PATH}"
  [[ ! -L "${PROJECT_ABSOLUTE}" ]] || fail "el proyecto no puede ser un enlace"
  PS_ARGS+=(-ProjectPath "$(to_windows_path "${PROJECT_ABSOLUTE}")")
fi
if [[ -n "${OUTPUT_DIRECTORY}" ]]; then
  mkdir -p "${OUTPUT_DIRECTORY}"
  OUTPUT_ABSOLUTE="$(cd "${OUTPUT_DIRECTORY}" && pwd)"
  [[ ! -L "${OUTPUT_ABSOLUTE}" ]] || fail "la salida no puede ser un enlace"
  PS_ARGS+=(-OutputDirectory "$(to_windows_path "${OUTPUT_ABSOLUTE}")")
fi

exec powershell.exe "${PS_ARGS[@]}"
