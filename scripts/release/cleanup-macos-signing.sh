#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  exit 0
fi

runner_temp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
keychain_path="${MACOS_RELEASE_KEYCHAIN:-${runner_temp}/grxfirma-release.keychain-db}"
original_default_file="${runner_temp}/grxfirma-original-default-keychain.txt"
original_search_file="${runner_temp}/grxfirma-original-keychain-search.txt"

if [[ -s "${original_search_file}" ]]; then
  search_keychains=()
  while IFS= read -r existing; do
    [[ -n "${existing}" ]] && search_keychains+=("${existing}")
  done < "${original_search_file}"
  if (( ${#search_keychains[@]} > 0 )); then
    security list-keychains -d user -s "${search_keychains[@]}" || true
  fi
fi
if [[ -s "${original_default_file}" ]]; then
  original_default="$(head -n 1 "${original_default_file}")"
  if [[ -n "${original_default}" ]]; then
    security default-keychain -d user -s "${original_default}" || true
  fi
fi

security delete-keychain "${keychain_path}" >/dev/null 2>&1 || true
rm -f \
  "${runner_temp}"/grxfirma-application.p12 \
  "${runner_temp}"/grxfirma-installer.p12 \
  "${original_default_file}" \
  "${original_search_file}"
if [[ -n "${MACOS_NOTARY_KEY_ID:-}" ]]; then
  rm -f "${runner_temp}/AuthKey_${MACOS_NOTARY_KEY_ID}.p8"
fi

echo "Credenciales efimeras de firma macOS eliminadas."
