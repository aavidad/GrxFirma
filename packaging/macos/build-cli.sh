#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/packaging/macos/reproducible-build.sh"
grxfirma_initialize_reproducible_build "${ROOT_DIR}"
if [[ -f "${ROOT_DIR}/VERSION.txt" ]]; then
  VERSION="$(tr -d '\r\n' < "${ROOT_DIR}/VERSION.txt")"
else
  VERSION="$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")"
fi
ARCH="${GOARCH:-$(go env GOARCH)}"
OUT_DIR="${ROOT_DIR}/release/macos-cli"
STAGE_DIR="${OUT_DIR}/GrxFirma-cli-macos-${ARCH}"
TAR_PATH="${OUT_DIR}/GrxFirma-cli-macos-${ARCH}.tar.gz"

usage() {
  cat <<'EOF'
Uso:
  packaging/macos/build-cli.sh

Variables relevantes:
  GOARCH=amd64|arm64
  CC / CXX              toolchain Darwin cruzada si no se ejecuta en macOS
EOF
}

is_macos_host() {
  [[ "$(uname -s)" == "Darwin" ]]
}

ensure_supported_arch() {
  case "${ARCH}" in
    amd64|arm64)
      return 0
      ;;
    *)
      echo "error: arquitectura macOS no soportada: ${ARCH}" >&2
      echo "       Usa GOARCH=amd64 o GOARCH=arm64." >&2
      exit 1
      ;;
  esac
}

ensure_macos_toolchain() {
  if is_macos_host; then
    return 0
  fi
  if [[ -n "${CC:-}" && -n "${CXX:-}" ]]; then
    return 0
  fi
  echo "error: el build macOS requiere macOS real o una toolchain Darwin cruzada configurada en CC/CXX." >&2
  echo "       En Linux, exporta una toolchain tipo osxcross antes de ejecutar este script." >&2
  exit 1
}

for arg in "$@"; do
  case "$arg" in
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: argumento no soportado: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

ensure_supported_arch
ensure_macos_toolchain

rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR"
rm -f "${STAGE_DIR}/grxfirma" "${STAGE_DIR}/README_CLI_MACOS.md" "${STAGE_DIR}/VERSION.txt"

echo "Compilando CLI de macOS (${ARCH})..."
CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma" \
  ./cmd/grxfirma

cp "${ROOT_DIR}/packaging/macos/README_CLI_MACOS.md" "${STAGE_DIR}/README_CLI_MACOS.md"
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"

mkdir -p "$OUT_DIR"
rm -f "$TAR_PATH"
grxfirma_reproducible_tar "${STAGE_DIR}" "${TAR_PATH}"
echo "Paquete generado en: $TAR_PATH"
