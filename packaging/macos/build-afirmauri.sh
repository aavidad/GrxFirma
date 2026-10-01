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
OUT_DIR="${ROOT_DIR}/release/macos-afirmauri"
STAGE_DIR="${OUT_DIR}/GrxFirma-afirmauri-macos-${ARCH}"
APP_DIR="${STAGE_DIR}/GrxFirma AfirmaURI.app"
BIN_DIR="${APP_DIR}/Contents/MacOS"
RES_DIR="${APP_DIR}/Contents/Resources"
TAR_PATH="${OUT_DIR}/GrxFirma-afirmauri-macos-${ARCH}.tar.gz"

usage() {
  cat <<'EOF'
Uso:
  packaging/macos/build-afirmauri.sh

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
mkdir -p "$BIN_DIR" "$RES_DIR"

echo "Compilando handler afirma:// de macOS (${ARCH})..."
CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production,fyne_gui \
  -o "${BIN_DIR}/grxfirma-afirmauri" \
  ./cmd/grxfirmauri

cat > "${APP_DIR}/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>GrxFirma AfirmaURI</string>
  <key>CFBundleDisplayName</key><string>GrxFirma AfirmaURI</string>
  <key>CFBundleIdentifier</key><string>es.dipgra.grxfirma.afirmauri</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>grxfirma-afirmauri</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>CFBundleURLTypes</key>
  <array>
    <dict>
      <key>CFBundleURLName</key><string>es.dipgra.grxfirma.afirma</string>
      <key>CFBundleURLSchemes</key>
      <array><string>afirma</string></array>
    </dict>
  </array>
</dict>
</plist>
EOF

cp "${ROOT_DIR}/packaging/macos/README_AFIRMAURI_MACOS.md" "${STAGE_DIR}/README_AFIRMAURI_MACOS.md"
cp "${ROOT_DIR}/packaging/macos/install-afirmauri.sh" "${STAGE_DIR}/install-afirmauri.sh"
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"
chmod 755 "${STAGE_DIR}/install-afirmauri.sh"

mkdir -p "$OUT_DIR"
rm -f "$TAR_PATH"
grxfirma_reproducible_tar "${STAGE_DIR}" "${TAR_PATH}"
echo "Paquete generado en: $TAR_PATH"
