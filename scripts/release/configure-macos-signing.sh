#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Configure an ephemeral keychain for Developer ID signing and notarization.
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: configure-macos-signing.sh solo puede ejecutarse en macOS." >&2
  exit 1
fi

: "${RUNNER_TEMP:?falta RUNNER_TEMP}"
: "${GITHUB_ENV:?falta GITHUB_ENV}"
: "${MACOS_APPLICATION_CERT_P12_BASE64:?falta MACOS_APPLICATION_CERT_P12_BASE64}"
: "${MACOS_APPLICATION_CERT_PASSWORD:?falta MACOS_APPLICATION_CERT_PASSWORD}"
: "${MACOS_INSTALLER_CERT_P12_BASE64:?falta MACOS_INSTALLER_CERT_P12_BASE64}"
: "${MACOS_INSTALLER_CERT_PASSWORD:?falta MACOS_INSTALLER_CERT_PASSWORD}"
: "${MACOS_CODESIGN_IDENTITY:?falta MACOS_CODESIGN_IDENTITY}"
: "${MACOS_INSTALLER_IDENTITY:?falta MACOS_INSTALLER_IDENTITY}"
: "${MACOS_NOTARY_KEY_P8_BASE64:?falta MACOS_NOTARY_KEY_P8_BASE64}"
: "${MACOS_NOTARY_KEY_ID:?falta MACOS_NOTARY_KEY_ID}"
: "${MACOS_NOTARY_ISSUER_ID:?falta MACOS_NOTARY_ISSUER_ID}"

keychain_path="${RUNNER_TEMP}/grxfirma-release.keychain-db"
keychain_password="$(openssl rand -hex 32)"
application_p12="${RUNNER_TEMP}/grxfirma-application.p12"
installer_p12="${RUNNER_TEMP}/grxfirma-installer.p12"
notary_key="${RUNNER_TEMP}/AuthKey_${MACOS_NOTARY_KEY_ID}.p8"
notary_profile="grxfirma-release-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}"
original_default_file="${RUNNER_TEMP}/grxfirma-original-default-keychain.txt"
original_search_file="${RUNNER_TEMP}/grxfirma-original-keychain-search.txt"

security default-keychain -d user | sed 's/^[[:space:]]*"//;s/"[[:space:]]*$//' > "${original_default_file}"
security list-keychains -d user | sed 's/^[[:space:]]*"//;s/"[[:space:]]*$//' > "${original_search_file}"

python3 - "${application_p12}" "${installer_p12}" "${notary_key}" <<'PY'
import base64
import os
import pathlib
import sys

variables = (
    "MACOS_APPLICATION_CERT_P12_BASE64",
    "MACOS_INSTALLER_CERT_P12_BASE64",
    "MACOS_NOTARY_KEY_P8_BASE64",
)
for variable, destination in zip(variables, sys.argv[1:]):
    try:
        payload = base64.b64decode(os.environ[variable], validate=True)
    except Exception as exc:
        raise SystemExit(f"{variable} no contiene Base64 valido: {exc}") from exc
    if not payload:
        raise SystemExit(f"{variable} esta vacio")
    pathlib.Path(destination).write_bytes(payload)
PY
chmod 600 "${application_p12}" "${installer_p12}" "${notary_key}"

security delete-keychain "${keychain_path}" >/dev/null 2>&1 || true
security create-keychain -p "${keychain_password}" "${keychain_path}"
security set-keychain-settings -lut 21600 "${keychain_path}"
security unlock-keychain -p "${keychain_password}" "${keychain_path}"

search_keychains=("${keychain_path}")
while IFS= read -r existing; do
  [[ -n "${existing}" && "${existing}" != "${keychain_path}" ]] && search_keychains+=("${existing}")
done < "${original_search_file}"
security list-keychains -d user -s "${search_keychains[@]}"
security default-keychain -d user -s "${keychain_path}"

security import "${application_p12}" -k "${keychain_path}" \
  -P "${MACOS_APPLICATION_CERT_PASSWORD}" \
  -T /usr/bin/codesign -T /usr/bin/security
security import "${installer_p12}" -k "${keychain_path}" \
  -P "${MACOS_INSTALLER_CERT_PASSWORD}" \
  -T /usr/bin/pkgbuild -T /usr/bin/productsign -T /usr/bin/security
security set-key-partition-list -S apple-tool:,apple:,codesign: \
  -s -k "${keychain_password}" "${keychain_path}" >/dev/null

if ! security find-identity -v -p codesigning "${keychain_path}" | grep -Fq "\"${MACOS_CODESIGN_IDENTITY}\""; then
  echo "error: el P12 de aplicacion no contiene MACOS_CODESIGN_IDENTITY." >&2
  exit 1
fi
if ! security find-identity -v -p pkgSign "${keychain_path}" | grep -Fq "\"${MACOS_INSTALLER_IDENTITY}\""; then
  echo "error: el P12 de instalador no contiene MACOS_INSTALLER_IDENTITY." >&2
  exit 1
fi

xcrun notarytool store-credentials "${notary_profile}" \
  --key "${notary_key}" \
  --key-id "${MACOS_NOTARY_KEY_ID}" \
  --issuer "${MACOS_NOTARY_ISSUER_ID}" \
  --keychain "${keychain_path}"

rm -f "${application_p12}" "${installer_p12}" "${notary_key}"
printf '::add-mask::%s\n' "${keychain_password}"
{
  printf 'MACOS_RELEASE_KEYCHAIN=%s\n' "${keychain_path}"
  printf 'MACOS_RELEASE_NOTARY_PROFILE=%s\n' "${notary_profile}"
} >> "${GITHUB_ENV}"

echo "Llavero efimero de firma macOS configurado y credenciales notariales validadas."
