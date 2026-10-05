#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/packaging/linux/reproducible-build.sh"
source "${ROOT_DIR}/packaging/linux/runtime-dependencies.sh"
grxfirma_initialize_reproducible_build "${ROOT_DIR}"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/grxfirma-gocache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/grxfirma-gomodcache}"

build_qt="${GRXFIRMA_BUILD_QT:-0}"
for arg in "$@"; do
  case "$arg" in
    --with-qt)
      build_qt=1
      ;;
  esac
done

resolve_qmake() {
  local qmake_cmd version
  for qmake_cmd in qmake6 qmake; do
    command -v "${qmake_cmd}" >/dev/null 2>&1 || continue
    version="$("${qmake_cmd}" -query QT_VERSION 2>/dev/null || true)"
    if [[ "${version}" == 6.* ]]; then
      printf '%s\n' "${qmake_cmd}"
      return 0
    fi
  done
  return 1
}

if [[ -f "${ROOT_DIR}/VERSION.txt" ]]; then
  RAW_VERSION="$(tr -d '\r\n' < "${ROOT_DIR}/VERSION.txt")"
else
  RAW_VERSION="$(git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || echo "dev")"
fi
STAGE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-install-user.XXXXXX")"
cleanup() {
  rm -rf "${STAGE_DIR}"
}
trap cleanup EXIT

echo "==> Compilando e instalando GrxFirma para el usuario"

echo "==> Compilando binarios Go"
grxfirma_go_build "${RAW_VERSION}" "" -tags production -o "${STAGE_DIR}/grxfirma" ./cmd/grxfirma
grxfirma_go_build "${RAW_VERSION}" "" -tags production -o "${STAGE_DIR}/grxfirma-gui" ./cmd/grxfirma-gui
grxfirma_go_build "${RAW_VERSION}" "" -tags production,fyne_gui -o "${STAGE_DIR}/grxfirma-desktop" ./cmd/grxfirma
grxfirma_go_build "${RAW_VERSION}" "" -tags production,fyne_gui -o "${STAGE_DIR}/grxfirma-afirmauri" ./cmd/grxfirmauri
grxfirma_go_build "${RAW_VERSION}" "" -tags production -o "${STAGE_DIR}/grxfirma-nativehost" ./cmd/nativehost
grxfirma_build_pkcs11_worker "${RAW_VERSION}" "${STAGE_DIR}/grxfirma-pkcs11-worker"

if [[ "${build_qt}" != "0" ]]; then
  if ! qmake_cmd="$(resolve_qmake)"; then
    echo "error: se requiere Qt6/qmake6 para --with-qt" >&2
    exit 1
  fi
  echo "==> Compilando frontend Qt/QML"
  qt_build_dir="${STAGE_DIR}/.qt-build"
  mkdir -p "${qt_build_dir}"
  "${qmake_cmd}" \
    -o "${qt_build_dir}/Makefile" \
    "${ROOT_DIR}/cmd/gui-qml/grxfirma_qt.pro"
  make -C "${qt_build_dir}" -j"$(nproc)"
  install -m 755 "${qt_build_dir}/grxfirma-gui-qml" "${STAGE_DIR}/grxfirma-gui-qml"
  find "${qt_build_dir}" -depth -delete
  cp -a "${ROOT_DIR}/cmd/gui-qml/qml" "${STAGE_DIR}/qml"
  cp -a "${ROOT_DIR}/cmd/gui-qml/assets" "${STAGE_DIR}/assets"
  if [[ -d "${ROOT_DIR}/cmd/gui-qml/help" ]]; then
    cp -a "${ROOT_DIR}/cmd/gui-qml/help" "${STAGE_DIR}/help"
  fi
  mkdir -p "${STAGE_DIR}/help"
  cp "${ROOT_DIR}/docs/NOVEDADES.md" "${STAGE_DIR}/help/NOVEDADES.md"
fi

echo "==> Preparando stage local"
mkdir -p "${STAGE_DIR}/man" "${STAGE_DIR}/extensions"
cp "${ROOT_DIR}/packaging/linux/install-suite.sh" "${STAGE_DIR}/install-suite.sh"
cp "${ROOT_DIR}/packaging/linux/configure-browsers.sh" "${STAGE_DIR}/configure-browsers.sh"
cp "${ROOT_DIR}/packaging/linux/runtime-dependencies.sh" "${STAGE_DIR}/check-runtime-dependencies.sh"
cp "${ROOT_DIR}/packaging/linux/README_LINUX_SUITE.md" "${STAGE_DIR}/README_LINUX_SUITE.md"
cp "${ROOT_DIR}/packaging/linux/grxfirma.desktop" "${STAGE_DIR}/grxfirma.desktop"
cp "${ROOT_DIR}/packaging/linux/grxfirma-manual.desktop" "${STAGE_DIR}/grxfirma-manual.desktop"
for icon_size in 48 128 256; do
  icon_dir="${STAGE_DIR}/icons/hicolor/${icon_size}x${icon_size}/apps"
  mkdir -p "${icon_dir}"
  cp "${ROOT_DIR}/assets/branding/grxfirma-icono-${icon_size}.png" "${icon_dir}/grxfirma.png"
done
mkdir -p "${STAGE_DIR}/icons/hicolor/scalable/apps"
cp "${ROOT_DIR}/assets/branding/grxfirma-icono.svg" \
  "${STAGE_DIR}/icons/hicolor/scalable/apps/grxfirma.svg"
cp "${ROOT_DIR}"/packaging/linux/man/* "${STAGE_DIR}/man/"
printf '%s\n' "${RAW_VERSION}" > "${STAGE_DIR}/VERSION.txt"
chmod 755 "${STAGE_DIR}/install-suite.sh"
chmod 755 "${STAGE_DIR}/configure-browsers.sh"
chmod 755 "${STAGE_DIR}/check-runtime-dependencies.sh"

if [[ "${build_qt}" != "0" ]]; then
  grxfirma_collect_qml_imports \
    "${ROOT_DIR}/cmd/gui-qml/qml" \
    "${STAGE_DIR}/runtime-dependencies.qml"
else
  : > "${STAGE_DIR}/runtime-dependencies.qml"
fi

echo "==> Generando extensiones de navegador"
bash "${ROOT_DIR}/packaging/browser-extensions/build.sh"
cp "${ROOT_DIR}"/packaging/browser-extensions/dipgra-extension-* "${STAGE_DIR}/extensions/"

echo "==> Instalando en el usuario"
(cd "${STAGE_DIR}" && ./install-suite.sh)
