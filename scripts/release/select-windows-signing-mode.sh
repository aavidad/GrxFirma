#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Elige la vía de firma Authenticode de la release oficial según los secretos
# presentes e imprime "signpath" o "pfx". Falla si no hay ninguna completa o si
# alguna está a medias. No imprime nunca el valor de los secretos.
#
#   signpath: SIGNPATH_API_TOKEN y SIGNPATH_ORGANIZATION_ID. Solo en tags sin
#             sufijo de prueba (vX.Y.Z); los tags vX.Y.Z-algo no la usan.
#   pfx:      WINDOWS_SIGNING_PFX_BASE64 y WINDOWS_SIGNING_PFX_PASSWORD.
#
# Si ambas están completas, un tag sin sufijo usa SignPath y uno de prueba usa
# el PFX. La huella WINDOWS_SIGNING_CERT_THUMBPRINT debe ser la del
# certificado que realmente firme.
set -euo pipefail

tag="${1:-${GITHUB_REF_NAME:-}}"

complete_pair() {
  local first="$1" second="$2" label="$3"
  if [[ -n "${!first:-}" && -n "${!second:-}" ]]; then
    return 0
  fi
  if [[ -n "${!first:-}" || -n "${!second:-}" ]]; then
    printf 'error: la vía de firma Windows %s está incompleta: hacen falta %s y %s.\n' \
      "${label}" "${first}" "${second}" >&2
    exit 1
  fi
  return 1
}

signpath=false
pfx=false
if complete_pair SIGNPATH_API_TOKEN SIGNPATH_ORGANIZATION_ID SignPath; then
  signpath=true
fi
if complete_pair WINDOWS_SIGNING_PFX_BASE64 WINDOWS_SIGNING_PFX_PASSWORD PFX; then
  pfx=true
fi

test_tag=false
if [[ "${tag}" == *-* ]]; then
  test_tag=true
fi

if [[ "${signpath}" == true && "${test_tag}" == false ]]; then
  if [[ "${pfx}" == true ]]; then
    echo "aviso: hay secretos de SignPath y de PFX; este tag oficial firmará con SignPath." >&2
  fi
  echo signpath
  exit 0
fi

if [[ "${pfx}" == true ]]; then
  echo pfx
  exit 0
fi

if [[ "${signpath}" == true ]]; then
  printf 'error: el tag de prueba %s no usa SignPath y no hay PFX configurado.\n' "${tag}" >&2
else
  echo "error: falta una vía de firma Windows: SignPath (SIGNPATH_API_TOKEN y SIGNPATH_ORGANIZATION_ID) o PFX (WINDOWS_SIGNING_PFX_BASE64 y WINDOWS_SIGNING_PFX_PASSWORD)." >&2
fi
exit 1
