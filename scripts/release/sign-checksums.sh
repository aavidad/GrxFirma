#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Generate SHA-256 checksums and an armored detached OpenPGP signature.
set -euo pipefail

usage() {
  cat <<'EOF'
Uso:
  sign-checksums.sh DIRECTORIO [--checksum-name NOMBRE] [--export-public-key RUTA]

Entorno obligatorio:
  RELEASE_GPG_PRIVATE_KEY_BASE64
  RELEASE_GPG_PASSPHRASE
  RELEASE_GPG_FINGERPRINT
EOF
}

if [[ $# -lt 1 ]]; then
  usage >&2
  exit 2
fi

target_dir="$1"
shift
checksum_name="SHA256SUMS.txt"
public_key_path=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --checksum-name)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      checksum_name="$2"
      shift 2
      ;;
    --export-public-key)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      public_key_path="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: argumento no soportado: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "${target_dir}" ]]; then
  echo "error: no existe el directorio de artefactos: ${target_dir}" >&2
  exit 1
fi
if [[ "${checksum_name}" == */* || -z "${checksum_name}" ]]; then
  echo "error: --checksum-name debe ser un nombre de fichero, no una ruta." >&2
  exit 2
fi

: "${RELEASE_GPG_PRIVATE_KEY_BASE64:?falta RELEASE_GPG_PRIVATE_KEY_BASE64}"
: "${RELEASE_GPG_PASSPHRASE:?falta RELEASE_GPG_PASSPHRASE}"
: "${RELEASE_GPG_FINGERPRINT:?falta RELEASE_GPG_FINGERPRINT}"

expected_fingerprint="$(tr -d '[:space:]' <<<"${RELEASE_GPG_FINGERPRINT}" | tr '[:lower:]' '[:upper:]')"
if [[ ! "${expected_fingerprint}" =~ ^([0-9A-F]{40}|[0-9A-F]{64})$ ]]; then
  echo "error: RELEASE_GPG_FINGERPRINT no es una huella OpenPGP valida." >&2
  exit 1
fi

work_dir="$(mktemp -d)"
export GNUPGHOME="${work_dir}/gnupg"
mkdir -m 700 "${GNUPGHOME}"
cleanup() {
  rm -rf "${work_dir}"
}
trap cleanup EXIT

key_path="${work_dir}/release-key.gpg"
printf '%s' "${RELEASE_GPG_PRIVATE_KEY_BASE64}" | base64 --decode > "${key_path}"
chmod 600 "${key_path}"
gpg --batch --quiet --import "${key_path}"

actual_fingerprint="$(
  gpg --batch --with-colons --list-secret-keys "${expected_fingerprint}" |
    awk -F: '$1 == "sec" { seen = 1; next } seen && $1 == "fpr" { print toupper($10); exit }'
)"
if [[ "${actual_fingerprint}" != "${expected_fingerprint}" ]]; then
  echo "error: la clave OpenPGP importada no coincide con RELEASE_GPG_FINGERPRINT." >&2
  exit 1
fi

checksum_path="${target_dir}/${checksum_name}"
signature_path="${checksum_path}.asc"
checksum_temp="${work_dir}/checksums.txt"
rm -f "${checksum_path}" "${signature_path}"

if [[ -n "${public_key_path}" ]]; then
  mkdir -p "$(dirname "${public_key_path}")"
  gpg --batch --armor --export "${expected_fingerprint}" > "${public_key_path}"
  [[ -s "${public_key_path}" ]] || { echo "error: no se pudo exportar la clave publica." >&2; exit 1; }
fi

(
  cd "${target_dir}"
  find . -type f \
    ! -name "${checksum_name}" \
    ! -name "${checksum_name}.asc" \
    -print0 |
    sort -z |
    xargs -0 sha256sum
) > "${checksum_temp}"
mv "${checksum_temp}" "${checksum_path}"
if [[ ! -s "${checksum_path}" ]]; then
  echo "error: no hay artefactos que firmar en ${target_dir}." >&2
  exit 1
fi

printf '%s' "${RELEASE_GPG_PASSPHRASE}" |
  gpg --batch --yes --quiet --pinentry-mode loopback --passphrase-fd 0 \
    --local-user "${expected_fingerprint}" --digest-algo SHA512 \
    --armor --detach-sign --output "${signature_path}" "${checksum_path}"

gpg --batch --verify "${signature_path}" "${checksum_path}" >/dev/null 2>&1
(
  cd "${target_dir}"
  sha256sum -c "${checksum_name}"
)
echo "Checksums firmados: ${checksum_path}"
