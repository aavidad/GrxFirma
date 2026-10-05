#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASE_DIR="${ROOT_DIR}/release"
INSTALLERS_DIR="${RELEASE_DIR}/installers"

if [[ "${ALLOW_DIRTY_RELEASE:-0}" != "1" ]]; then
  if ! git -C "${ROOT_DIR}" diff --quiet --ignore-submodules -- || ! git -C "${ROOT_DIR}" diff --cached --quiet --ignore-submodules --; then
    echo "error: el arbol git no esta limpio. Usa ALLOW_DIRTY_RELEASE=1 solo si sabes lo que haces." >&2
    exit 1
  fi
fi

rm -rf "${INSTALLERS_DIR}"
mkdir -p "${INSTALLERS_DIR}/linux" "${INSTALLERS_DIR}/windows" "${INSTALLERS_DIR}/macos"

copy_if_exists() {
  local pattern="$1"
  local target_dir="$2"
  shopt -s nullglob
  for file in ${pattern}; do
    cp -f "${file}" "${target_dir}/"
  done
  shopt -u nullglob
  return 0
}

copy_if_exists "${RELEASE_DIR}/linux-suite/*.tar.gz" "${INSTALLERS_DIR}/linux"
copy_if_exists "${RELEASE_DIR}/linux-suite/*.deb" "${INSTALLERS_DIR}/linux"

copy_if_exists "${RELEASE_DIR}/windows-suite/*.zip" "${INSTALLERS_DIR}/windows"
copy_if_exists "${RELEASE_DIR}/windows-suite/*-setup.exe" "${INSTALLERS_DIR}/windows"
copy_if_exists "${RELEASE_DIR}/windows-nativehost/*.zip" "${INSTALLERS_DIR}/windows"
copy_if_exists "${RELEASE_DIR}/windows-afirmauri/*-setup.exe" "${INSTALLERS_DIR}/windows"
copy_if_exists "${RELEASE_DIR}/windows-cli/*-setup.exe" "${INSTALLERS_DIR}/windows"

copy_if_exists "${RELEASE_DIR}/macos-suite/*.tar.gz" "${INSTALLERS_DIR}/macos"
copy_if_exists "${RELEASE_DIR}/macos-suite/*.pkg" "${INSTALLERS_DIR}/macos"

# Verifica que ningun artefacto sensible se haya colado en los paquetes
# finales antes de sellarlos con checksums.
"${ROOT_DIR}/scripts/release/check-secrets.sh" "${INSTALLERS_DIR}"

(
  cd "${INSTALLERS_DIR}"
  find . -type f ! -name 'SHA256SUMS.txt' -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS.txt
)

echo "Candidatos tecnicos NO OFICIALES agrupados en: ${INSTALLERS_DIR}"
echo "Checksums sin firma oficial en: ${INSTALLERS_DIR}/SHA256SUMS.txt"
