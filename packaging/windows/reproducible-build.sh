#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Shared by the shell-based Windows cross-packagers. This file is sourced.

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

grxfirma_assert_windows_fyne_toolchain() {
  local arch="$1"
  local compiler="${CC:-}"
  local target

  if [[ "${arch}" != "amd64" ]]; then
    echo "error: la GUI afirma:// con CGO solo esta soportada para GOARCH=amd64." >&2
    return 1
  fi
  if [[ -z "${compiler}" ]]; then
    compiler="x86_64-w64-mingw32-gcc"
  fi
  if ! command -v "${compiler}" >/dev/null 2>&1; then
    echo "error: no se encontro el compilador MinGW-w64 amd64 requerido: ${compiler}" >&2
    echo "       Instala x86_64-w64-mingw32-gcc o define CC con su ruta." >&2
    return 1
  fi
  if ! target="$("${compiler}" -dumpmachine 2>/dev/null)"; then
    echo "error: no se pudo consultar el target del compilador CGO: ${compiler}" >&2
    return 1
  fi
  target="${target//$'\r'/}"
  target="${target//$'\n'/}"
  if [[ "${target}" != "x86_64-w64-mingw32" ]]; then
    echo "error: el compilador CGO no es MinGW-w64 amd64: ${compiler} (${target:-target desconocido})" >&2
    return 1
  fi
}

grxfirma_assert_windows_fyne_artifact() {
  local artifact="$1"
  local arch="${2:-amd64}"
  local go_command="${GRXFIRMA_GO_COMMAND:-go}"
  local metadata
  local tags
  local cgo
  local goos
  local goarch

  if [[ ! -f "${artifact}" ]]; then
    echo "error: no existe el handler afirma:// que se debe validar: ${artifact}" >&2
    return 1
  fi
  if ! metadata="$("${go_command}" version -m "${artifact}" 2>&1)"; then
    echo "error: no se pudieron leer los metadatos Go de ${artifact}" >&2
    printf '%s\n' "${metadata}" >&2
    return 1
  fi

  tags="$(printf '%s\n' "${metadata}" | awk '$1 == "build" && $2 ~ /^-tags=/ {sub(/^-tags=/, "", $2); print $2; exit}')"
  tags="${tags#\"}"
  tags="${tags%\"}"
  cgo="$(printf '%s\n' "${metadata}" | awk '$1 == "build" && $2 ~ /^CGO_ENABLED=/ {sub(/^CGO_ENABLED=/, "", $2); print $2; exit}')"
  goos="$(printf '%s\n' "${metadata}" | awk '$1 == "build" && $2 ~ /^GOOS=/ {sub(/^GOOS=/, "", $2); print $2; exit}')"
  goarch="$(printf '%s\n' "${metadata}" | awk '$1 == "build" && $2 ~ /^GOARCH=/ {sub(/^GOARCH=/, "", $2); print $2; exit}')"

  case ",${tags}," in
    *,fyne_gui,*)
      ;;
    *)
      echo "error: ${artifact} no contiene el build tag obligatorio fyne_gui." >&2
      return 1
      ;;
  esac
  case ",${tags}," in
    *,production,*)
      ;;
    *)
      echo "error: ${artifact} no contiene el build tag obligatorio production." >&2
      return 1
      ;;
  esac
  if [[ "${cgo}" != "1" ]]; then
    echo "error: ${artifact} no declara CGO_ENABLED=1." >&2
    return 1
  fi
  if [[ "${goos}" != "windows" || "${goarch}" != "${arch}" ]]; then
    echo "error: metadatos de plataforma inesperados en ${artifact}: GOOS=${goos:-?} GOARCH=${goarch:-?}" >&2
    return 1
  fi
}

grxfirma_reproducible_zip() {
  local source_dir="$1"
  local output_path="$2"
  local helper_dir
  helper_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  python3 "${helper_dir}/reproducible-zip.py" \
    --source "${source_dir}" \
    --output "${output_path}" \
    --mtime "${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH no inicializado}"
}

grxfirma_normalize_output_mtime() {
  python3 - "${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH no inicializado}" "$@" <<'PY'
import os
import sys
from pathlib import Path

mtime = max(int(sys.argv[1]), 315532800)
for raw_path in sys.argv[2:]:
    path = Path(raw_path)
    if not path.is_file():
        raise SystemExit(f"artifact does not exist: {path}")
    os.utime(path, (mtime, mtime))
PY
}
