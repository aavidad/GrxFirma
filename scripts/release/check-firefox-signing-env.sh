#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Fail-closed preflight scoped to the isolated AMO signing job.
set -euo pipefail

missing=()
for name in WEB_EXT_API_KEY WEB_EXT_API_SECRET; do
  if [[ -z "${!name:-}" ]]; then
    missing+=("${name}")
  fi
done

if (( ${#missing[@]} > 0 )); then
  printf 'error: faltan credenciales obligatorias para firmar Firefox:\n' >&2
  printf '  - %s\n' "${missing[@]}" >&2
  exit 1
fi

echo "Preflight aislado de firma Firefox: OK."
