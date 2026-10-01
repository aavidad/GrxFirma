#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Shared by the Linux packagers. This file is sourced, not executed.

grxfirma_resolve_source_date_epoch() {
  local root_dir="$1"
  local epoch="${SOURCE_DATE_EPOCH:-}"

  if [[ -z "${epoch}" ]] && command -v git >/dev/null 2>&1; then
    epoch="$(git -C "${root_dir}" log -1 --format=%ct 2>/dev/null || true)"
  fi
  if [[ -z "${epoch}" ]]; then
    epoch=0
  fi
  if [[ ! "${epoch}" =~ ^[0-9]+$ ]]; then
    echo "error: SOURCE_DATE_EPOCH debe ser un entero no negativo: ${epoch}" >&2
    return 1
  fi
  printf '%s\n' "${epoch}"
}

grxfirma_initialize_reproducible_build() {
  local root_dir="$1"
  SOURCE_DATE_EPOCH="$(grxfirma_resolve_source_date_epoch "${root_dir}")"
  export SOURCE_DATE_EPOCH
  export TZ=UTC
  export ZERO_AR_DATE=1
}

grxfirma_go_build() {
  local version="$1"
  local platform_ldflags="$2"
  shift 2

  local ldflags="-buildid="
  if [[ -n "${platform_ldflags}" ]]; then
    ldflags+=" ${platform_ldflags}"
  fi
  ldflags+=" -X main.version=${version} -s -w"

  env GOFLAGS= go build \
    -mod=readonly \
    -pgo=off \
    -trimpath \
    -buildvcs=false \
    -ldflags "${ldflags}" \
    "$@"
}

grxfirma_build_pkcs11_worker() {
  local version="$1" output="$2" metadata
  # Solo el auxiliar carga CGo; no habilita PKCS#11 en los procesos principales.
  CGO_ENABLED=1 GOOS=linux grxfirma_go_build "${version}" "" \
    -tags production -o "${output}" ./cmd/grxfirma-pkcs11-worker || return 1
  metadata="$(go version -m "${output}")" || return 1
  if ! grep -Eq '^[[:space:]]*build[[:space:]]+CGO_ENABLED=1$' <<<"${metadata}" ||
      ! grep -Eq '^[[:space:]]*path[[:space:]]+grxfirma/cmd/grxfirma-pkcs11-worker$' <<<"${metadata}"; then
    echo "error: el auxiliar PKCS#11 no contiene el backend CGo Linux esperado" >&2
    return 1
  fi
}

grxfirma_reproducible_tar() {
  local source_dir="$1"
  local output_path="$2"
  local helper_dir
  helper_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  python3 "${helper_dir}/reproducible-tar.py" \
    --source "${source_dir}" \
    --output "${output_path}" \
    --mtime "${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH no inicializado}"
}

grxfirma_normalize_output_mtime() {
  python3 - "${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH no inicializado}" "$@" <<'PY'
import os
import sys
from pathlib import Path

mtime = int(sys.argv[1])
for raw_path in sys.argv[2:]:
    path = Path(raw_path)
    if not path.is_file():
        raise SystemExit(f"artifact does not exist: {path}")
    os.utime(path, (mtime, mtime))
PY
}
