#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Verify Developer ID signatures, hardened runtime and notarization evidence.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Uso: verify-macos-release.sh DIRECTORIO_DE_ARTEFACTOS" >&2
  exit 2
fi
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: la verificacion oficial macOS requiere un runner macOS." >&2
  exit 1
fi

: "${MACOS_CODESIGN_IDENTITY:?falta MACOS_CODESIGN_IDENTITY}"
: "${MACOS_INSTALLER_IDENTITY:?falta MACOS_INSTALLER_IDENTITY}"
: "${MACOS_TEAM_ID:?falta MACOS_TEAM_ID}"

artifact_dir="$1"
shopt -s nullglob
packages=("${artifact_dir}"/*.pkg)
shopt -u nullglob
if (( ${#packages[@]} != 1 )); then
  echo "error: se esperaba exactamente un PKG oficial y hay ${#packages[@]}." >&2
  exit 1
fi
python3 - "${artifact_dir}/MACOS-NOTARIZATION.json" <<'PY'
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
if not path.is_file():
    raise SystemExit(f"missing notarization evidence: {path}")
payload = json.loads(path.read_text(encoding="utf-8"))
if (
    payload.get("status") != "Accepted"
    or not payload.get("id")
    or not payload.get("package")
    or len(payload.get("sha256", "")) != 64
):
    raise SystemExit(f"invalid notarization evidence: {payload}")
PY

verify_code_signature() {
  local target="$1"
  local details
  codesign --verify --strict --verbose=4 "${target}"
  details="$(codesign --display --verbose=4 "${target}" 2>&1)"
  grep -Fq "Authority=${MACOS_CODESIGN_IDENTITY}" <<<"${details}" || {
    echo "error: identidad Developer ID inesperada en ${target}." >&2
    exit 1
  }
  grep -Fq "TeamIdentifier=${MACOS_TEAM_ID}" <<<"${details}" || {
    echo "error: Team ID inesperado en ${target}." >&2
    exit 1
  }
  grep -Eq 'flags=.*\(.*runtime.*\)' <<<"${details}" || {
    echo "error: Hardened Runtime no esta activo en ${target}." >&2
    exit 1
  }
  grep -Fq 'Timestamp=' <<<"${details}" || {
    echo "error: falta timestamp seguro en ${target}." >&2
    exit 1
  }
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

verify_bundle_tree() {
  local bundle="$1"
  local target
  while IFS= read -r -d '' target; do
    [[ ! -L "${target}" ]] || continue
    if [[ -f "${target}" ]]; then
      if file -b "${target}" | grep -q '^Mach-O'; then
        verify_code_signature "${target}"
      fi
      continue
    fi
    if [[ -d "${target}" ]] && is_nested_code_bundle "${target}"; then
      verify_code_signature "${target}"
    fi
  done < <(find "${bundle}/Contents" -depth \( -type f -o -type d \) -print0)
  verify_code_signature "${bundle}"
  codesign --verify --deep --strict --verbose=4 "${bundle}"
}

for package in "${packages[@]}"; do
  python3 - "${artifact_dir}/MACOS-NOTARIZATION.json" "${package}" <<'PY'
import hashlib
import json
import pathlib
import sys

evidence_path, package = map(pathlib.Path, sys.argv[1:])
evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
digest = hashlib.sha256()
with package.open("rb") as stream:
    for chunk in iter(lambda: stream.read(1024 * 1024), b""):
        digest.update(chunk)
actual = digest.hexdigest()
if (
    evidence.get("package") != package.name
    or evidence.get("sha256") != actual
    or evidence.get("size_bytes") != package.stat().st_size
):
    raise SystemExit("notarization evidence does not match the stapled PKG")
PY
  signature_report="$(pkgutil --check-signature "${package}")"
  grep -Fq "${MACOS_INSTALLER_IDENTITY}" <<<"${signature_report}" || {
    echo "error: el PKG no esta firmado por MACOS_INSTALLER_IDENTITY." >&2
    exit 1
  }
  spctl --assess --type install --verbose=4 "${package}"
  xcrun stapler validate "${package}"

  expanded="$(mktemp -d)"
  trap 'rm -rf "${expanded:-}"' EXIT
  pkgutil --expand-full "${package}" "${expanded}/pkg"

  app="$(find "${expanded}/pkg" -type d -name 'GrxFirma AfirmaURI.app' -print -quit)"
  [[ -n "${app}" ]] || { echo "error: el PKG no contiene la aplicacion AfirmaURI." >&2; exit 1; }
  verify_bundle_tree "${app}"
  spctl --assess --type execute --verbose=4 "${app}"

  desktop_app="$(find "${expanded}/pkg" -type d -name 'GrxFirma Desktop Qt.app' -print -quit)"
  [[ -n "${desktop_app}" ]] || { echo "error: el PKG no contiene la GUI Desktop Qt." >&2; exit 1; }
  verify_bundle_tree "${desktop_app}"
  spctl --assess --type execute --verbose=4 "${desktop_app}"

  desktop_gui="${desktop_app}/Contents/MacOS/grxfirma-gui-qml"
  desktop_bootstrap="${desktop_app}/Contents/MacOS/grxfirma-gui"
  desktop_backend="${desktop_app}/Contents/MacOS/grxfirma"
  for desktop_executable in \
    "${desktop_gui}" \
    "${desktop_bootstrap}" \
    "${desktop_backend}"; do
    if [[ ! -f "${desktop_executable}" ||
          -L "${desktop_executable}" ||
          ! -x "${desktop_executable}" ]]; then
      echo "error: ejecutable Desktop Qt ausente o no valido: ${desktop_executable}." >&2
      exit 1
    fi
    verify_code_signature "${desktop_executable}"
  done
  cli="$(find "${expanded}/pkg" -type f -path '*/Library/Application Support/GrxFirma/grxfirma' -print -quit)"
  nativehost="$(find "${expanded}/pkg" -type f -path '*/Library/Application Support/GrxFirma/grxfirma-nativehost' -print -quit)"
  afirmauri="${app}/Contents/MacOS/grxfirma-afirmauri"
  [[ -n "${cli}" ]] || { echo "error: el PKG no contiene grxfirma." >&2; exit 1; }
  [[ -n "${nativehost}" ]] || { echo "error: el PKG no contiene grxfirma-nativehost." >&2; exit 1; }
  [[ -f "${afirmauri}" ]] || { echo "error: el PKG no contiene grxfirma-afirmauri." >&2; exit 1; }
  verify_code_signature "${cli}"
  verify_code_signature "${nativehost}"
  # afirmauri y los tres ejecutables Desktop quedan incluidos además en el
  # recorrido completo del bundle; la comprobación explícita evita que un
  # cambio futuro del inventario pase inadvertido.

  rm -rf "${expanded}"
  trap - EXIT
done

echo "Firma Developer ID, Hardened Runtime, Gatekeeper y notarizacion macOS: OK."
