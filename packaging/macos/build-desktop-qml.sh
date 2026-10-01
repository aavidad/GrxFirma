#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/packaging/macos/reproducible-build.sh"
source "${ROOT_DIR}/packaging/macos/qmake-compat.sh"
grxfirma_initialize_reproducible_build "${ROOT_DIR}"
if [[ -f "${ROOT_DIR}/VERSION.txt" ]]; then
  VERSION="$(tr -d '\r\n' < "${ROOT_DIR}/VERSION.txt")"
else
  VERSION="$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")"
fi
ARCH="${GOARCH:-$(go env GOARCH)}"
QT_ARCH=""
OUT_DIR="${ROOT_DIR}/release/macos-desktop-qml"
STAGE_DIR="${OUT_DIR}/GrxFirma-desktop-qml-macos-${ARCH}"
BUILD_DIR="${OUT_DIR}/build-${ARCH}"
APP_DIR="${STAGE_DIR}/GrxFirma Desktop Qt.app"
APP_BIN_DIR="${APP_DIR}/Contents/MacOS"
APP_RES_DIR="${APP_DIR}/Contents/Resources"
TAR_PATH="${OUT_DIR}/GrxFirma-desktop-qml-macos-${ARCH}.tar.gz"
PROJECT_QML="${ROOT_DIR}/cmd/gui-qml/grxfirma_qt.pro"

usage() {
  cat <<'EOF'
Uso:
  packaging/macos/build-desktop-qml.sh

Variables relevantes:
  GOARCH=amd64|arm64
  QMAKE=/ruta/a/qmake6
  CC / CXX              toolchain Darwin cruzada solo para el backend Go

Notas:
  - el empaquetado final del .app requiere macOS real y macdeployqt;
  - esta ruta no está soportada como cross-build completo desde Linux.
EOF
}

is_macos_host() {
  [[ "$(uname -s)" == "Darwin" ]]
}

ensure_supported_arch() {
  case "${ARCH}" in
    amd64)
      QT_ARCH="x86_64"
      return 0
      ;;
    arm64)
      QT_ARCH="arm64"
      return 0
      ;;
    *)
      echo "error: arquitectura macOS no soportada: ${ARCH}" >&2
      echo "       Usa GOARCH=amd64 o GOARCH=arm64." >&2
      exit 1
      ;;
  esac
}

ensure_valid_macos_version() {
  if [[ ! "${VERSION}" =~ ^[0-9]+([.][0-9]+){0,2}$ ]]; then
    echo "error: VERSION.txt debe contener una version macOS numerica de hasta tres componentes: ${VERSION}" >&2
    exit 1
  fi
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

ensure_macos_qt_bundle_tools() {
  if ! is_macos_host; then
    echo "error: el frontend desktop Qt/QML para macOS requiere macOS real para desplegar el .app." >&2
    echo "       macdeployqt no está soportado como cross-build completo desde este entorno." >&2
    exit 1
  fi
  if ! command -v macdeployqt >/dev/null 2>&1; then
    echo "error: macdeployqt no está disponible. Instala Qt for macOS antes de ejecutar este script." >&2
    exit 1
  fi
}

resolve_qmake() {
  if [[ -n "${QMAKE:-}" ]]; then
    printf '%s\n' "${QMAKE}"
    return 0
  fi
  if command -v qmake6 >/dev/null 2>&1; then
    printf '%s\n' "qmake6"
    return 0
  fi
  if command -v qmake >/dev/null 2>&1; then
    local version
    version="$(qmake -query QT_VERSION 2>/dev/null || true)"
    if [[ "${version}" == 6.* ]]; then
      printf '%s\n' "qmake"
      return 0
    fi
  fi
  echo "error: se requiere Qt6 (qmake6) para compilar el frontend desktop QML" >&2
  exit 1
}

set_plist_string() {
  local plist="$1"
  local key="$2"
  local value="$3"
  if ! /usr/libexec/PlistBuddy -c \
    "Set :${key} ${value}" "${plist}" >/dev/null 2>&1; then
    /usr/libexec/PlistBuddy -c \
      "Add :${key} string ${value}" "${plist}"
  fi
}

validate_tar_artifact() {
  local tar_path="$1"
  local stage_name
  stage_name="$(basename "${STAGE_DIR}")"
  python3 - "${tar_path}" "${stage_name}" <<'PY'
import pathlib
import sys
import tarfile

tar_path, stage_name = sys.argv[1:3]
required = {
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Info.plist",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Resources/qml/main.qml",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Resources/assets/grxfirma-diputacion.ico",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Resources/VERSION.txt",
    f"{stage_name}/install-desktop-qml.sh",
    f"{stage_name}/README_DESKTOP_QML_MACOS.md",
    f"{stage_name}/VERSION.txt",
}
with tarfile.open(tar_path, "r:gz") as archive:
    names = set(archive.getnames())
for name in names:
    path = pathlib.PurePosixPath(name)
    if path.is_absolute() or ".." in path.parts or (
        name != stage_name and not name.startswith(stage_name + "/")
    ):
        raise SystemExit(f"unsafe tar entry: {name}")
missing = sorted(required - names)
if missing:
    raise SystemExit("tar artifact incomplete:\n  - " + "\n  - ".join(missing))
PY
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
ensure_valid_macos_version
ensure_macos_toolchain
ensure_macos_qt_bundle_tools

mkdir -p "$OUT_DIR"
rm -rf "$STAGE_DIR" "$BUILD_DIR"
rm -f "$TAR_PATH"
mkdir -p "$STAGE_DIR" "$BUILD_DIR"

echo "Compilando backend Go para macOS (${ARCH})..."
CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${BUILD_DIR}/grxfirma" \
  ./cmd/grxfirma

echo "Compilando bootstrap IPC desktop para macOS (${ARCH})..."
CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${BUILD_DIR}/grxfirma-gui" \
  ./cmd/grxfirma-gui

echo "Compilando frontend Qt/QML..."
QMAKE_CMD="$(resolve_qmake)"
grxfirma_prepare_qmake_macos "${QMAKE_CMD}"
pushd "$BUILD_DIR" >/dev/null
"$QMAKE_CMD" "$PROJECT_QML" "QMAKE_APPLE_DEVICE_ARCHS=${QT_ARCH}"
grxfirma_prepare_qmake_makefile_macos \
  "${QMAKE_CMD}" \
  "${BUILD_DIR}/Makefile"
grxfirma_assert_qmake_makefile_without_agl "${BUILD_DIR}/Makefile"
make
popd >/dev/null

BUILT_APP="${BUILD_DIR}/grxfirma-gui-qml.app"
if [[ ! -d "$BUILT_APP" ]]; then
  echo "error: no se ha generado grxfirma-gui-qml.app" >&2
  exit 1
fi

rm -rf "$APP_DIR"
cp -R "$BUILT_APP" "$APP_DIR"

mkdir -p "$APP_BIN_DIR" "$APP_RES_DIR"
install -m 755 "${BUILD_DIR}/grxfirma-gui" "${APP_BIN_DIR}/grxfirma-gui"
install -m 755 "${BUILD_DIR}/grxfirma" "${APP_BIN_DIR}/grxfirma"
cp -R "${ROOT_DIR}/cmd/gui-qml/qml" "${APP_RES_DIR}/qml"
cp -R "${ROOT_DIR}/cmd/gui-qml/assets" "${APP_RES_DIR}/assets"
cp \
  "${ROOT_DIR}/assets/branding/grxfirma-diputacion.ico" \
  "${APP_RES_DIR}/assets/grxfirma-diputacion.ico"
printf '%s\n' "${VERSION}" > "${APP_RES_DIR}/VERSION.txt"

desktop_plist="${APP_DIR}/Contents/Info.plist"
if [[ ! -f "${desktop_plist}" ]]; then
  echo "error: la aplicacion Qt no contiene Info.plist" >&2
  exit 1
fi
set_plist_string "${desktop_plist}" "CFBundleIdentifier" "es.dipgra.grxfirma.desktop"
set_plist_string "${desktop_plist}" "CFBundleShortVersionString" "${VERSION}"
set_plist_string "${desktop_plist}" "CFBundleVersion" "${VERSION}"
set_plist_string "${desktop_plist}" "LSMinimumSystemVersion" "11.0"
plutil -lint "${desktop_plist}" >/dev/null

echo "Desplegando runtime Qt..."
macdeployqt \
  "$APP_DIR" \
  -qmldir="${ROOT_DIR}/cmd/gui-qml/qml" \
  -codesign=-
MACOS_CODESIGN_IDENTITY=- \
  bash "${ROOT_DIR}/packaging/macos/codesign-bundle.sh" "$APP_DIR"
codesign --verify --deep --strict --verbose=2 "$APP_DIR"

cp "${ROOT_DIR}/packaging/macos/install-desktop-qml.sh" "${STAGE_DIR}/install-desktop-qml.sh"
cp "${ROOT_DIR}/packaging/macos/README_DESKTOP_QML_MACOS.md" "${STAGE_DIR}/README_DESKTOP_QML_MACOS.md"
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"
chmod 755 "${STAGE_DIR}/install-desktop-qml.sh"

grxfirma_reproducible_tar "${STAGE_DIR}" "${TAR_PATH}"
validate_tar_artifact "${TAR_PATH}"
echo "Paquete generado en: $TAR_PATH"
