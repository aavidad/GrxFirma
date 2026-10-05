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
OUT_DIR="${ROOT_DIR}/release/macos-suite"
STAGE_DIR="${OUT_DIR}/GrxFirma-suite-macos-${ARCH}"
APP_DIR="${STAGE_DIR}/GrxFirma AfirmaURI.app"
BIN_DIR="${APP_DIR}/Contents/MacOS"
RES_DIR="${APP_DIR}/Contents/Resources"
DESKTOP_APP_DIR="${STAGE_DIR}/GrxFirma Desktop Qt.app"
DESKTOP_BUILD_DIR="${OUT_DIR}/build-desktop-${ARCH}"
DESKTOP_PROJECT="${ROOT_DIR}/cmd/gui-qml/grxfirma_qt.pro"
TAR_PATH="${OUT_DIR}/GrxFirma-suite-macos-${ARCH}.tar.gz"
PKG_PATH="${OUT_DIR}/GrxFirma-suite-macos-${ARCH}.pkg"
DO_PKG=0
DEFAULT_EXTENSION_KEY="${HOME}/.local/share/grxfirma/build-keys/chromium-extension.pem"

usage() {
  cat <<'EOF'
Uso:
  packaging/macos/build-suite.sh [--pkg]

Variables relevantes:
  GOARCH=amd64|arm64
  CC / CXX              toolchain Darwin cruzada si no se ejecuta en macOS
  MACOS_CODESIGN_IDENTITY
  MACOS_INSTALLER_IDENTITY
  MACOS_NOTARY_PROFILE
  QMAKE=/ruta/a/qmake6
  GRXFIRMA_BUILD_CHROMIUM_CRX=1 genera el CRX local no reproducible
  GRXFIRMA_BUILD_SAFARI=1 genera proyecto Xcode Safari Web Extension si se ejecuta en macOS
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
  echo "error: la suite macOS requiere macOS real o una toolchain Darwin cruzada configurada en CC/CXX." >&2
  echo "       En Linux, exporta una toolchain tipo osxcross antes de ejecutar este script." >&2
  exit 1
}

ensure_macos_qt_bundle_tools() {
  if ! is_macos_host; then
    echo "error: la suite final con GUI Qt requiere macOS real." >&2
    echo "       macdeployqt no esta soportado como cross-build completo desde este entorno." >&2
    exit 1
  fi
  if ! command -v macdeployqt >/dev/null 2>&1; then
    echo "error: macdeployqt no esta disponible. Instala Qt 6 para macOS." >&2
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
  echo "error: se requiere Qt 6 (qmake6) para compilar la GUI macOS." >&2
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

ensure_codesign_ready_if_enabled() {
  if [[ -z "${MACOS_CODESIGN_IDENTITY:-}" ]]; then
    return 0
  fi
  if ! is_macos_host; then
    echo "error: MACOS_CODESIGN_IDENTITY requiere ejecutarse en macOS real." >&2
    echo "       El cross-build puede generar el binario, pero no firmarlo desde este entorno." >&2
    exit 1
  fi
  if ! command -v codesign >/dev/null 2>&1; then
    echo "error: codesign no está disponible en este entorno macOS." >&2
    exit 1
  fi
}

ensure_pkg_ready_if_requested() {
  if [[ "$DO_PKG" != "1" ]]; then
    return 0
  fi
  if ! is_macos_host; then
    echo "error: --pkg requiere ejecutarse en macOS real." >&2
    exit 1
  fi
  if ! command -v pkgbuild >/dev/null 2>&1; then
    echo "error: pkgbuild no está disponible." >&2
    exit 1
  fi
}

ensure_notary_ready_if_enabled() {
  if [[ -z "${MACOS_NOTARY_PROFILE:-}" ]]; then
    return 0
  fi
  if [[ "$DO_PKG" != "1" ]]; then
    echo "error: MACOS_NOTARY_PROFILE solo aplica cuando se construye el PKG con --pkg." >&2
    exit 1
  fi
  if ! is_macos_host; then
    echo "error: MACOS_NOTARY_PROFILE requiere ejecutarse en macOS real." >&2
    exit 1
  fi
  if ! command -v xcrun >/dev/null 2>&1; then
    echo "error: xcrun no está disponible para notarytool/stapler." >&2
    exit 1
  fi
}

ensure_safari_ready_if_requested() {
  case "${GRXFIRMA_BUILD_SAFARI:-0}" in
    0)
      return 0
      ;;
    1)
      ;;
    *)
      echo "error: GRXFIRMA_BUILD_SAFARI debe valer 0 o 1." >&2
      exit 1
      ;;
  esac
  if ! is_macos_host; then
    echo "error: GRXFIRMA_BUILD_SAFARI=1 requiere macOS real con Xcode." >&2
    exit 1
  fi
  if ! command -v xcrun >/dev/null 2>&1 || ! command -v xcodebuild >/dev/null 2>&1; then
    echo "error: GRXFIRMA_BUILD_SAFARI=1 requiere Xcode y sus Command Line Tools." >&2
    exit 1
  fi
}

resolve_pack_browser() {
  local cmd
  for cmd in "Google Chrome" "Chromium" "Brave Browser" "Microsoft Edge"; do
    if [[ -x "/Applications/${cmd}.app/Contents/MacOS/${cmd}" ]]; then
      printf '%s\n' "/Applications/${cmd}.app/Contents/MacOS/${cmd}"
      return 0
    fi
  done
  return 1
}

ensure_chromium_extension_key() {
  if [[ -n "${GRXFIRMA_CHROMIUM_EXTENSION_KEY:-}" && -f "${GRXFIRMA_CHROMIUM_EXTENSION_KEY}" ]]; then
    printf '%s\n' "${GRXFIRMA_CHROMIUM_EXTENSION_KEY}"
    return 0
  fi

  mkdir -p "$(dirname "${DEFAULT_EXTENSION_KEY}")"
  chmod 700 "$(dirname "${DEFAULT_EXTENSION_KEY}")" 2>/dev/null || true
  if [[ ! -f "${DEFAULT_EXTENSION_KEY}" ]]; then
    umask 077
    openssl genrsa -out "${DEFAULT_EXTENSION_KEY}" 2048 >/dev/null 2>&1
  fi
  if [[ -f "${DEFAULT_EXTENSION_KEY}" ]]; then
    chmod 600 "${DEFAULT_EXTENSION_KEY}" 2>/dev/null || true
    printf '%s\n' "${DEFAULT_EXTENSION_KEY}"
    return 0
  fi
  return 1
}

compute_extension_id() {
  local pem="$1"
  python3 - "$pem" <<'PY'
import hashlib
import subprocess
import sys

pem = sys.argv[1]
pub_der = subprocess.check_output(["openssl", "pkey", "-in", pem, "-pubout", "-outform", "DER"])
hex_digest = hashlib.sha256(pub_der).hexdigest()[:32]
print(hex_digest.translate(str.maketrans("0123456789abcdef", "abcdefghijklmnop")))
PY
}

extract_extension_version() {
  local zip_path="$1"
  python3 - "$zip_path" <<'PY'
import json
import sys
import zipfile

with zipfile.ZipFile(sys.argv[1]) as zf:
    for name in zf.namelist():
        if name.endswith("manifest.json"):
            print(json.loads(zf.read(name)).get("version", "1.0.0"))
            break
    else:
        print("1.0.0")
PY
}

build_chromium_extension_assets() {
  local zip_path="$1"
  local out_dir="$2"
  [[ -f "${zip_path}" ]] || return 0

  case "${GRXFIRMA_BUILD_CHROMIUM_CRX:-0}" in
    0) return 0 ;;
    1) ;;
    *)
      echo "error: GRXFIRMA_BUILD_CHROMIUM_CRX debe valer 0 o 1." >&2
      return 1
      ;;
  esac

  local pack_browser
  pack_browser="$(resolve_pack_browser || true)"
  if [[ -z "${pack_browser}" ]]; then
    echo "Aviso: no hay navegador Chromium disponible para empaquetar la extensión local." >&2
    return 0
  fi

  local tmpdir ext_root ext_dir crx pem version ext_id key_path
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN
  unzip -oq "${zip_path}" -d "${tmpdir}/ext"
  ext_root="${tmpdir}/ext"
  ext_dir="${ext_root}"
  if [[ -d "${ext_root}/chromium" ]]; then
    ext_dir="${ext_root}/chromium"
  fi

  local pack_args=("--pack-extension=${ext_dir}")
  key_path="$(ensure_chromium_extension_key || true)"
  if [[ -n "${key_path}" && -f "${key_path}" ]]; then
    pack_args+=("--pack-extension-key=${key_path}")
  fi
  "${pack_browser}" "${pack_args[@]}" >/dev/null 2>&1

  crx=""
  for candidate in \
    "${ext_root}/$(basename "${ext_dir}").crx" \
    "${ext_root}.crx" \
    "${ext_dir}.crx"; do
    if [[ -f "${candidate}" ]]; then
      crx="${candidate}"
      break
    fi
  done
  pem=""
  for candidate in \
    "${ext_root}/$(basename "${ext_dir}").pem" \
    "${ext_root}.pem" \
    "${ext_dir}.pem"; do
    if [[ -f "${candidate}" ]]; then
      pem="${candidate}"
      break
    fi
  done
  if [[ -n "${key_path}" && -f "${key_path}" ]]; then
    pem="${key_path}"
  fi
  if [[ -z "${crx}" || ! -f "${crx}" || -z "${pem}" || ! -f "${pem}" ]]; then
    echo "Aviso: no se pudo generar el CRX Chromium empaquetado." >&2
    return 0
  fi

  version="$(extract_extension_version "${zip_path}")"
  ext_id="$(compute_extension_id "${pem}")"

  cp "${crx}" "${out_dir}/grxfirma-extension-chromium.crx"
  printf '%s\n' "${ext_id}" > "${out_dir}/grxfirma-extension-chromium.id"
  printf '%s\n' "${version}" > "${out_dir}/grxfirma-extension-chromium.version"
}

codesign_if_enabled() {
  local target="$1"
  if [[ -z "${MACOS_CODESIGN_IDENTITY:-}" ]]; then
    return 0
  fi
  codesign --force --timestamp --options runtime --sign "${MACOS_CODESIGN_IDENTITY}" "$target"
  codesign --verify --strict --verbose=2 "$target"
}

notarize_if_enabled() {
  local target="$1"
  if [[ -z "${MACOS_NOTARY_PROFILE:-}" ]]; then
    return 0
  fi
  xcrun notarytool submit "$target" --keychain-profile "${MACOS_NOTARY_PROFILE}" --wait
  xcrun stapler staple "$target"
}

validate_tar_artifact() {
  local tar_path="$1"
  local stage_name
  stage_name="$(basename "${STAGE_DIR}")"
  python3 - "${tar_path}" "${stage_name}" "${GRXFIRMA_BUILD_SAFARI:-0}" <<'PY'
import pathlib
import sys
import tarfile

tar_path, stage_name = sys.argv[1:3]
expect_safari = sys.argv[3] == "1"
required = {
    f"{stage_name}/grxfirma",
    f"{stage_name}/grxfirma-nativehost",
    f"{stage_name}/GrxFirma AfirmaURI.app/Contents/Info.plist",
    f"{stage_name}/GrxFirma AfirmaURI.app/Contents/MacOS/grxfirma-afirmauri",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Info.plist",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Resources/qml/main.qml",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Resources/assets/grxfirma.ico",
    f"{stage_name}/GrxFirma Desktop Qt.app/Contents/Resources/VERSION.txt",
    f"{stage_name}/README_MACOS_SUITE.md",
    f"{stage_name}/README_DESKTOP_QML_MACOS.md",
    f"{stage_name}/SAFARI_BROWSER_EXTENSION.md",
    f"{stage_name}/install-suite.sh",
    f"{stage_name}/install-nativehost.sh",
    f"{stage_name}/install-afirmauri.sh",
    f"{stage_name}/install-desktop-qml.sh",
    f"{stage_name}/uninstall-suite.sh",
    f"{stage_name}/VERSION.txt",
    f"{stage_name}/extensions/grxfirma-extension-chromium.zip",
    f"{stage_name}/extensions/grxfirma-extension-firefox.xpi",
    f"{stage_name}/extensions/grxfirma-extension-firefox.metadata.json",
}
with tarfile.open(tar_path, "r:gz") as archive:
    names = set(archive.getnames())
for name in names:
    path = pathlib.PurePosixPath(name)
    if path.is_absolute() or ".." in path.parts or (name != stage_name and not name.startswith(stage_name + "/")):
        raise SystemExit(f"unsafe tar entry: {name}")
missing = sorted(required - names)
if expect_safari and not any(name.endswith(".xcodeproj/project.pbxproj") for name in names):
    missing.append(f"{stage_name}/safari/*.xcodeproj/project.pbxproj")
if missing:
    raise SystemExit("tar artifact incomplete:\n  - " + "\n  - ".join(missing))
PY
}

validate_pkg_artifact() {
  local pkg_path="$1"
  bash "${ROOT_DIR}/packaging/macos/validate-pkg.sh" "${pkg_path}"
}

artifact_sha256() {
  local path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${path}" | awk '{print $1}'
  else
    shasum -a 256 "${path}" | awk '{print $1}'
  fi
}

write_artifact_manifests() {
  local checksum_path="${OUT_DIR}/SHA256SUMS.txt"
  local report_path="${OUT_DIR}/ARTIFACTS.md"
  local artifacts=("${TAR_PATH}")
  [[ -f "${PKG_PATH}" ]] && artifacts+=("${PKG_PATH}")
  : > "${checksum_path}"
  {
    printf '# macOS Suite Artifacts\n\n'
    printf '| File | Size (bytes) | SHA-256 |\n'
    printf '| --- | ---: | --- |\n'
  } > "${report_path}"
  local path hash size
  for path in "${artifacts[@]}"; do
    hash="$(artifact_sha256 "${path}")"
    size="$(wc -c < "${path}" | tr -d '[:space:]')"
    printf '%s  %s\n' "${hash}" "$(basename "${path}")" >> "${checksum_path}"
    printf '| %s | %s | %s |\n' "$(basename "${path}")" "${size}" "${hash}" >> "${report_path}"
  done
}

verify_artifact_manifests() {
  local expected name actual
  while read -r expected name; do
    [[ -n "${expected}" && -n "${name}" ]] || continue
    actual="$(artifact_sha256 "${OUT_DIR}/${name}")"
    if [[ "${expected}" != "${actual}" ]]; then
      echo "error: checksum inesperado para ${name}" >&2
      exit 1
    fi
  done < "${OUT_DIR}/SHA256SUMS.txt"
}

for arg in "$@"; do
  case "$arg" in
    --pkg)
      DO_PKG=1
      ;;
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
ensure_safari_ready_if_requested
ensure_macos_toolchain
ensure_macos_qt_bundle_tools
ensure_codesign_ready_if_enabled
ensure_pkg_ready_if_requested
ensure_notary_ready_if_enabled

mkdir -p "$OUT_DIR"
rm -rf "$STAGE_DIR" "$DESKTOP_BUILD_DIR"
rm -f "$TAR_PATH" "$PKG_PATH" "${OUT_DIR}/SHA256SUMS.txt" "${OUT_DIR}/ARTIFACTS.md"
mkdir -p "$BIN_DIR" "$RES_DIR" "$DESKTOP_BUILD_DIR"

echo "Compilando suite de macOS (${ARCH})..."
CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma" \
  ./cmd/grxfirma

CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma-nativehost" \
  ./cmd/nativehost

CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production,fyne_gui \
  -o "${BIN_DIR}/grxfirma-afirmauri" \
  ./cmd/grxfirmauri

CGO_ENABLED=1 GOOS=darwin GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${DESKTOP_BUILD_DIR}/grxfirma-gui" \
  ./cmd/grxfirma-gui

echo "Compilando GUI Qt/QML de macOS (${ARCH})..."
QMAKE_CMD="$(resolve_qmake)"
grxfirma_prepare_qmake_macos "${QMAKE_CMD}"
pushd "${DESKTOP_BUILD_DIR}" >/dev/null
"${QMAKE_CMD}" "${DESKTOP_PROJECT}" "QMAKE_APPLE_DEVICE_ARCHS=${QT_ARCH}"
grxfirma_prepare_qmake_makefile_macos \
  "${QMAKE_CMD}" \
  "${DESKTOP_BUILD_DIR}/Makefile"
grxfirma_assert_qmake_makefile_without_agl "${DESKTOP_BUILD_DIR}/Makefile"
make
popd >/dev/null

BUILT_DESKTOP_APP="${DESKTOP_BUILD_DIR}/grxfirma-gui-qml.app"
if [[ ! -d "${BUILT_DESKTOP_APP}" ]]; then
  echo "error: qmake no ha generado grxfirma-gui-qml.app" >&2
  exit 1
fi
cp -R "${BUILT_DESKTOP_APP}" "${DESKTOP_APP_DIR}"
mkdir -p "${DESKTOP_APP_DIR}/Contents/MacOS"
install -m 755 \
  "${DESKTOP_BUILD_DIR}/grxfirma-gui" \
  "${DESKTOP_APP_DIR}/Contents/MacOS/grxfirma-gui"
install -m 755 \
  "${STAGE_DIR}/grxfirma" \
  "${DESKTOP_APP_DIR}/Contents/MacOS/grxfirma"
mkdir -p "${DESKTOP_APP_DIR}/Contents/Resources"
cp -R \
  "${ROOT_DIR}/cmd/gui-qml/qml" \
  "${DESKTOP_APP_DIR}/Contents/Resources/qml"
cp -R \
  "${ROOT_DIR}/cmd/gui-qml/assets" \
  "${DESKTOP_APP_DIR}/Contents/Resources/assets"
cp \
  "${ROOT_DIR}/assets/branding/grxfirma.ico" \
  "${DESKTOP_APP_DIR}/Contents/Resources/assets/grxfirma.ico"
printf '%s\n' \
  "${VERSION}" \
  > "${DESKTOP_APP_DIR}/Contents/Resources/VERSION.txt"

desktop_plist="${DESKTOP_APP_DIR}/Contents/Info.plist"
if [[ ! -f "${desktop_plist}" ]]; then
  echo "error: la aplicacion Qt no contiene Info.plist" >&2
  exit 1
fi
set_plist_string "${desktop_plist}" "CFBundleIdentifier" "io.github.aavidad.grxfirma.desktop"
set_plist_string "${desktop_plist}" "CFBundleShortVersionString" "${VERSION}"
set_plist_string "${desktop_plist}" "CFBundleVersion" "${VERSION}"
set_plist_string "${desktop_plist}" "LSMinimumSystemVersion" "11.0"
plutil -lint "${desktop_plist}" >/dev/null

macdeployqt \
  "${DESKTOP_APP_DIR}" \
  -qmldir="${ROOT_DIR}/cmd/gui-qml/qml" \
  -codesign=-
MACOS_CODESIGN_IDENTITY=- \
  bash \
    "${ROOT_DIR}/packaging/macos/codesign-bundle.sh" \
    "${DESKTOP_APP_DIR}"
codesign --verify --deep --strict --verbose=2 "${DESKTOP_APP_DIR}"

cat > "${APP_DIR}/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>GrxFirma AfirmaURI</string>
  <key>CFBundleDisplayName</key><string>GrxFirma AfirmaURI</string>
  <key>CFBundleIdentifier</key><string>io.github.aavidad.grxfirma.afirmauri</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>grxfirma-afirmauri</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>CFBundleURLTypes</key>
  <array>
    <dict>
      <key>CFBundleURLName</key><string>io.github.aavidad.grxfirma.afirma</string>
      <key>CFBundleURLSchemes</key>
      <array><string>afirma</string></array>
    </dict>
  </array>
</dict>
</plist>
EOF

if command -v plutil >/dev/null 2>&1; then
  plutil -lint "${APP_DIR}/Contents/Info.plist" >/dev/null
fi

cp "${ROOT_DIR}/packaging/macos/README_MACOS_SUITE.md" "${STAGE_DIR}/README_MACOS_SUITE.md"
cp "${ROOT_DIR}/packaging/browser-extensions/SAFARI.md" "${STAGE_DIR}/SAFARI_BROWSER_EXTENSION.md"
cp "${ROOT_DIR}/packaging/macos/install-suite.sh" "${STAGE_DIR}/install-suite.sh"
cp "${ROOT_DIR}/packaging/macos/install-nativehost.sh" "${STAGE_DIR}/install-nativehost.sh"
cp "${ROOT_DIR}/packaging/macos/install-afirmauri.sh" "${STAGE_DIR}/install-afirmauri.sh"
cp "${ROOT_DIR}/packaging/macos/install-desktop-qml.sh" "${STAGE_DIR}/install-desktop-qml.sh"
cp "${ROOT_DIR}/packaging/macos/uninstall-suite.sh" "${STAGE_DIR}/uninstall-suite.sh"
cp "${ROOT_DIR}/packaging/macos/README_DESKTOP_QML_MACOS.md" "${STAGE_DIR}/README_DESKTOP_QML_MACOS.md"
mkdir -p "${STAGE_DIR}/extensions"
bash "${ROOT_DIR}/packaging/browser-extensions/build.sh"
for extension_asset in \
  grxfirma-extension-chromium.zip \
  grxfirma-extension-firefox.xpi \
  grxfirma-extension-firefox.metadata.json; do
  extension_source="${ROOT_DIR}/packaging/browser-extensions/${extension_asset}"
  if [[ ! -f "${extension_source}" ]]; then
    echo "error: falta el artefacto de navegador aprobado: ${extension_source}" >&2
    exit 1
  fi
  cp "${extension_source}" "${STAGE_DIR}/extensions/"
done
build_chromium_extension_assets "${ROOT_DIR}/packaging/browser-extensions/grxfirma-extension-chromium.zip" "${STAGE_DIR}/extensions"
if [[ "${GRXFIRMA_BUILD_SAFARI:-0}" == "1" ]]; then
  SAFARI_PROJECT_DIR="${STAGE_DIR}/safari" \
    bash "${ROOT_DIR}/packaging/browser-extensions/build-safari.sh"
fi
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"
chmod 755 \
  "${STAGE_DIR}/install-suite.sh" \
  "${STAGE_DIR}/install-nativehost.sh" \
  "${STAGE_DIR}/install-afirmauri.sh" \
  "${STAGE_DIR}/install-desktop-qml.sh" \
  "${STAGE_DIR}/uninstall-suite.sh"

codesign_if_enabled "${STAGE_DIR}/grxfirma"
codesign_if_enabled "${STAGE_DIR}/grxfirma-nativehost"
if [[ -n "${MACOS_CODESIGN_IDENTITY:-}" ]]; then
  bash "${ROOT_DIR}/packaging/macos/codesign-bundle.sh" "${APP_DIR}"
  bash "${ROOT_DIR}/packaging/macos/codesign-bundle.sh" "${DESKTOP_APP_DIR}"
fi

mkdir -p "$OUT_DIR"
grxfirma_reproducible_tar "${STAGE_DIR}" "${TAR_PATH}"
validate_tar_artifact "${TAR_PATH}"
echo "Paquete generado en: $TAR_PATH"

if [[ "$DO_PKG" == "1" ]]; then
  local_root="${OUT_DIR}/pkgroot"
  pkg_scripts_dir="${OUT_DIR}/pkg-scripts"
  rm -rf "${local_root}"
  rm -rf "${pkg_scripts_dir}"
  mkdir -p \
    "${local_root}/Applications" \
    "${local_root}/Library/Application Support/GrxFirma" \
    "${local_root}/Library/LaunchAgents" \
    "${local_root}/usr/local/bin" \
    "${pkg_scripts_dir}"
  cp -R "${APP_DIR}" "${local_root}/Applications/GrxFirma AfirmaURI.app"
  cp -R "${DESKTOP_APP_DIR}" "${local_root}/Applications/GrxFirma Desktop Qt.app"
  cp "${STAGE_DIR}/grxfirma" "${local_root}/Library/Application Support/GrxFirma/grxfirma"
  cp "${STAGE_DIR}/grxfirma-nativehost" "${local_root}/Library/Application Support/GrxFirma/grxfirma-nativehost"
  cp -R "${STAGE_DIR}/extensions" "${local_root}/Library/Application Support/GrxFirma/extensions"
  install -m 755 "${STAGE_DIR}/install-nativehost.sh" "${local_root}/Library/Application Support/GrxFirma/install-nativehost.sh"
  install -m 755 "${STAGE_DIR}/uninstall-suite.sh" "${local_root}/Library/Application Support/GrxFirma/uninstall-suite.sh"
  install -m 644 "${STAGE_DIR}/README_MACOS_SUITE.md" "${local_root}/Library/Application Support/GrxFirma/README_MACOS_SUITE.md"
  if [[ -d "${STAGE_DIR}/safari" ]]; then
    cp -R "${STAGE_DIR}/safari" "${local_root}/Library/Application Support/GrxFirma/safari"
  fi

  cat > "${local_root}/Library/Application Support/GrxFirma/register-user.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
root="/Library/Application Support/GrxFirma"
"${root}/install-nativehost.sh"
lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
if [[ -x "${lsregister}" ]]; then
  "${lsregister}" -f "/Applications/GrxFirma AfirmaURI.app" >/dev/null 2>&1 || true
fi
EOF
  chmod 755 "${local_root}/Library/Application Support/GrxFirma/register-user.sh"

  cat > "${local_root}/Library/LaunchAgents/io.github.aavidad.grxfirma.register-user.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>io.github.aavidad.grxfirma.register-user</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>/Library/Application Support/GrxFirma/register-user.sh</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>ProcessType</key><string>Background</string>
  <key>LimitLoadToSessionType</key><string>Aqua</string>
</dict>
</plist>
EOF
  if command -v plutil >/dev/null 2>&1; then
    plutil -lint "${local_root}/Library/LaunchAgents/io.github.aavidad.grxfirma.register-user.plist" >/dev/null
  fi
  ln -s "/Library/Application Support/GrxFirma/grxfirma" "${local_root}/usr/local/bin/grxfirma"

  cat > "${pkg_scripts_dir}/postinstall" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
register_script="/Library/Application Support/GrxFirma/register-user.sh"
# Las versiones anteriores instalaban el agente y el recibo con el
# identificador es.dipgra.grxfirma; el nuevo agente los sustituye.
legacy_agent="/Library/LaunchAgents/es.dipgra.grxfirma.register-user.plist"
if [[ -f "${legacy_agent}" && ! -L "${legacy_agent}" ]]; then
  rm -f -- "${legacy_agent}"
fi
/usr/sbin/pkgutil --forget es.dipgra.grxfirma.suite >/dev/null 2>&1 || true
console_user="$(stat -f '%Su' /dev/console 2>/dev/null || true)"
if [[ -n "${console_user}" && "${console_user}" != "root" && "${console_user}" != "loginwindow" ]]; then
  console_home="$(dscl . -read "/Users/${console_user}" NFSHomeDirectory 2>/dev/null | awk '{print $2}')"
  if [[ -n "${console_home}" && -d "${console_home}" ]]; then
    /usr/bin/sudo -u "${console_user}" /usr/bin/env \
      HOME="${console_home}" USER="${console_user}" LOGNAME="${console_user}" \
      /bin/bash "${register_script}"
  fi
fi
exit 0
EOF
  chmod 755 "${pkg_scripts_dir}/postinstall"
  bash -n "${local_root}/Library/Application Support/GrxFirma/register-user.sh" "${pkg_scripts_dir}/postinstall"
  grxfirma_normalize_tree_mtime "${local_root}" "${pkg_scripts_dir}"

  pkg_args=(
    --root "${local_root}"
    --scripts "${pkg_scripts_dir}"
    --ownership recommended
    --identifier "io.github.aavidad.grxfirma.suite"
    --version "${VERSION}"
    --install-location "/"
    "${PKG_PATH}"
  )
  if [[ -n "${MACOS_INSTALLER_IDENTITY:-}" ]]; then
    pkg_args=(--sign "${MACOS_INSTALLER_IDENTITY}" "${pkg_args[@]}")
  fi
  pkgbuild "${pkg_args[@]}"
  notarize_if_enabled "${PKG_PATH}"
  validate_pkg_artifact "${PKG_PATH}"
  grxfirma_normalize_output_mtime "${PKG_PATH}"
  rm -rf "${local_root}" "${pkg_scripts_dir}"
  echo "Instalador PKG generado en: $PKG_PATH"
fi

write_artifact_manifests
verify_artifact_manifests
