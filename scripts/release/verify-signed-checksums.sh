#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Verify an OpenPGP-signed checksum set using an explicitly pinned key.
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "Uso: verify-signed-checksums.sh DIRECTORIO CHECKSUM FIRMA CLAVE_PUBLICA" >&2
  exit 2
fi

target_dir="$1"
checksum_path="$2"
signature_path="$3"
public_key_path="$4"
: "${RELEASE_GPG_FINGERPRINT:?falta RELEASE_GPG_FINGERPRINT}"

for path in "${target_dir}" "${checksum_path}" "${signature_path}" "${public_key_path}"; do
  [[ -e "${path}" ]] || { echo "error: falta ${path}" >&2; exit 1; }
done

expected_fingerprint="$(tr -d '[:space:]' <<<"${RELEASE_GPG_FINGERPRINT}" | tr '[:lower:]' '[:upper:]')"
work_dir="$(mktemp -d)"
export GNUPGHOME="${work_dir}/gnupg"
mkdir -m 700 "${GNUPGHOME}"
trap 'rm -rf "${work_dir}"' EXIT

gpg --batch --quiet --import "${public_key_path}"
mapfile -t primary_fingerprints < <(
  gpg --batch --with-colons --fingerprint |
    awk -F: '$1 == "pub" { want = 1; next } want && $1 == "fpr" { print toupper($10); want = 0 }'
)
if (( ${#primary_fingerprints[@]} != 1 )) || [[ "${primary_fingerprints[0]}" != "${expected_fingerprint}" ]]; then
  echo "error: la clave publica no coincide con RELEASE_GPG_FINGERPRINT." >&2
  exit 1
fi

status="$({ gpg --batch --status-fd 1 --verify "${signature_path}" "${checksum_path}" 2>/dev/null; } || true)"
mapfile -t valid_signers < <(awk '$1 == "[GNUPG:]" && $2 == "VALIDSIG" { print toupper($3) }' <<<"${status}")
mapfile -t allowed_signers < <(
  gpg --batch --with-colons --fingerprint --fingerprint "${expected_fingerprint}" |
    awk -F: '$1 == "fpr" { print toupper($10) }'
)
if (( ${#valid_signers[@]} != 1 )) ||
  ! printf '%s\n' "${allowed_signers[@]}" | grep -Fqx "${valid_signers[0]:-INVALID}"; then
  echo "error: la firma OpenPGP de ${checksum_path} no es valida." >&2
  exit 1
fi

(
  cd "${target_dir}"
  sha256sum -c "$(realpath --relative-to="${target_dir}" "${checksum_path}")"
)
echo "Firma OpenPGP y checksums verificados: ${checksum_path}"
