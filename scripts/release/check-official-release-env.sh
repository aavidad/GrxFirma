#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Fail-closed credential preflight for tag-triggered official releases.
set -euo pipefail

required=(
  WINDOWS_SIGNING_PFX_BASE64
  WINDOWS_SIGNING_PFX_PASSWORD
  WINDOWS_SIGNING_CERT_THUMBPRINT
  ANDROID_SIGNING_KEYSTORE_BASE64
  GRXFIRMA_ANDROID_KEYSTORE_PASSWORD
  GRXFIRMA_ANDROID_KEY_ALIAS
  GRXFIRMA_ANDROID_KEY_PASSWORD
  GRXFIRMA_ANDROID_SIGNING_CERT_SHA256
  ANDROID_QA_KEYSTORE_BASE64
  ANDROID_QA_KEYSTORE_PASSWORD
  ANDROID_QA_KEY_ALIAS
  ANDROID_QA_KEY_PASSWORD
  ANDROID_QA_SIGNING_CERT_SHA256
  MACOS_APPLICATION_CERT_P12_BASE64
  MACOS_APPLICATION_CERT_PASSWORD
  MACOS_INSTALLER_CERT_P12_BASE64
  MACOS_INSTALLER_CERT_PASSWORD
  MACOS_CODESIGN_IDENTITY
  MACOS_INSTALLER_IDENTITY
  MACOS_TEAM_ID
  MACOS_NOTARY_KEY_P8_BASE64
  MACOS_NOTARY_KEY_ID
  MACOS_NOTARY_ISSUER_ID
  RELEASE_GPG_PRIVATE_KEY_BASE64
  RELEASE_GPG_PASSPHRASE
  RELEASE_GPG_FINGERPRINT
)

missing=()
for name in "${required[@]}"; do
  if [[ -z "${!name:-}" ]]; then
    missing+=("${name}")
  fi
done

if (( ${#missing[@]} > 0 )); then
  printf 'error: faltan credenciales o identificadores obligatorios para el release oficial:\n' >&2
  printf '  - %s\n' "${missing[@]}" >&2
  exit 1
fi

normalize_hex() {
  tr -d '[:space:]' <<<"$1" | tr '[:lower:]' '[:upper:]'
}

windows_thumbprint="$(normalize_hex "${WINDOWS_SIGNING_CERT_THUMBPRINT}")"
if [[ ! "${windows_thumbprint}" =~ ^[0-9A-F]{40}$ ]]; then
  echo "error: WINDOWS_SIGNING_CERT_THUMBPRINT debe ser una huella SHA-1 de 40 hexadecimales." >&2
  exit 1
fi

gpg_fingerprint="$(normalize_hex "${RELEASE_GPG_FINGERPRINT}")"
if [[ ! "${gpg_fingerprint}" =~ ^([0-9A-F]{40}|[0-9A-F]{64})$ ]]; then
  echo "error: RELEASE_GPG_FINGERPRINT debe tener 40 o 64 hexadecimales." >&2
  exit 1
fi

android_fingerprint="$(normalize_hex "${GRXFIRMA_ANDROID_SIGNING_CERT_SHA256}")"
if [[ ! "${android_fingerprint}" =~ ^[0-9A-F]{64}$ ]]; then
  echo "error: GRXFIRMA_ANDROID_SIGNING_CERT_SHA256 debe tener 64 hexadecimales." >&2
  exit 1
fi

android_qa_fingerprint="$(normalize_hex "${ANDROID_QA_SIGNING_CERT_SHA256}")"
if [[ ! "${android_qa_fingerprint}" =~ ^[0-9A-F]{64}$ ]]; then
  echo "error: ANDROID_QA_SIGNING_CERT_SHA256 debe tener 64 hexadecimales." >&2
  exit 1
fi

if [[ ! "${MACOS_TEAM_ID}" =~ ^[A-Z0-9]{10}$ ]]; then
  echo "error: MACOS_TEAM_ID debe tener los 10 caracteres del Apple Developer Team ID." >&2
  exit 1
fi

if [[ ! "${MACOS_NOTARY_KEY_ID}" =~ ^[A-Z0-9]{10}$ ]]; then
  echo "error: MACOS_NOTARY_KEY_ID debe tener 10 caracteres alfanumericos." >&2
  exit 1
fi

if [[ ! "${MACOS_NOTARY_ISSUER_ID}" =~ ^[0-9A-Fa-f-]{36}$ ]]; then
  echo "error: MACOS_NOTARY_ISSUER_ID debe ser el UUID de issuer de App Store Connect." >&2
  exit 1
fi

validate_base64() {
  local name="$1"
  if ! printf '%s' "${!name}" | base64 --decode >/dev/null 2>&1; then
    echo "error: ${name} no contiene Base64 valido." >&2
    exit 1
  fi
}

validate_base64 WINDOWS_SIGNING_PFX_BASE64
validate_base64 ANDROID_SIGNING_KEYSTORE_BASE64
validate_base64 ANDROID_QA_KEYSTORE_BASE64
validate_base64 MACOS_APPLICATION_CERT_P12_BASE64
validate_base64 MACOS_INSTALLER_CERT_P12_BASE64
validate_base64 MACOS_NOTARY_KEY_P8_BASE64
validate_base64 RELEASE_GPG_PRIVATE_KEY_BASE64

echo "Preflight de credenciales de release oficial: OK."
