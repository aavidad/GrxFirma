#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
python3 "${ROOT_DIR}/scripts/comprobar-novedades.py"
source "${ROOT_DIR}/packaging/linux/reproducible-build.sh"
source "${ROOT_DIR}/packaging/linux/runtime-dependencies.sh"
grxfirma_initialize_reproducible_build "${ROOT_DIR}"
if [[ -f "${ROOT_DIR}/VERSION.txt" ]]; then
  RAW_VERSION="$(tr -d '\r\n' < "${ROOT_DIR}/VERSION.txt")"
else
  RAW_VERSION="$(git -C "${ROOT_DIR}" describe --tags --always --dirty 2>/dev/null || echo "dev")"
fi
ARCH="${GOARCH:-$(go env GOARCH)}"
OUT_DIR="${ROOT_DIR}/release/linux-suite"
STAGE_DIR="${OUT_DIR}/GrxFirma-${RAW_VERSION}-linux-${ARCH}"
TAR_PATH="${OUT_DIR}/GrxFirma-${RAW_VERSION}-linux-${ARCH}.tar.gz"
QT_BUILD_DIR="${OUT_DIR}/qt-build-${ARCH}"
QML_RUNTIME_MANIFEST="${STAGE_DIR}/runtime-dependencies.qml"
QML_RUNTIME_ROOT=""
RUNTIME_DEPENDS=""
DO_DEB=0
DEFAULT_EXTENSION_KEY="${HOME}/.local/share/grxfirma/build-keys/chromium-extension.pem"
REQUIRED_RUNTIME_DEPENDS=(libnss3-tools libsecret-tools)
SOURCE_COMMIT=""
SOURCE_TREE_STATE="unknown"

usage() {
  cat <<'EOF'
Uso:
  packaging/linux/build-suite.sh [--deb]

Variables relevantes:
  GOARCH=amd64|arm64|arm|386
  QMAKE=/ruta/a/qmake6
  GRXFIRMA_BUILD_CHROMIUM_CRX=1 genera el CRX local no reproducible
  GRXFIRMA_CHROMIUM_EXTENSION_KEY=/ruta/a/chromium-extension.pem
EOF
}

ensure_supported_arch() {
  case "${ARCH}" in
    amd64|arm64|arm|386)
      return 0
      ;;
    *)
      echo "error: arquitectura Linux no soportada: ${ARCH}" >&2
      echo "       Usa GOARCH=amd64, arm64, arm o 386." >&2
      exit 1
      ;;
  esac
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
  return 1
}

resolve_pack_browser() {
  local cmd
  for cmd in google-chrome google-chrome-stable chromium chromium-browser brave-browser microsoft-edge microsoft-edge-stable; do
    if command -v "${cmd}" >/dev/null 2>&1; then
      printf '%s\n' "${cmd}"
      return 0
    fi
  done
  return 1
}

ensure_chromium_extension_key() {
  if [[ -n "${GRXFIRMA_CHROMIUM_EXTENSION_KEY:-}" ]]; then
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
trans = str.maketrans("0123456789abcdef", "abcdefghijklmnop")
print(hex_digest.translate(trans))
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
            data = json.loads(zf.read(name))
            print(data.get("version", "1.0.0"))
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

  local tmpdir ext_root ext_dir crx pem version ext_id
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' RETURN
  unzip -oq "${zip_path}" -d "${tmpdir}/ext"
  ext_root="${tmpdir}/ext"
  ext_dir="${ext_root}"
  if [[ -d "${ext_root}/chromium" ]]; then
    ext_dir="${ext_root}/chromium"
  fi

  local pack_args=("--pack-extension=${ext_dir}")
  local key_path=""
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

  if [[ -n "${key_path}" ]]; then
    echo "CRX Chromium generado con clave estable local: ${key_path}" >&2
  else
    echo "Aviso: CRX Chromium generado con clave efímera; el ID cambiará en cada build." >&2
  fi
}

validate_tar_artifact() {
  local tar_path="$1"
  local stage_name
  stage_name="$(basename "${STAGE_DIR}")"
  local expect_help=0
  if [[ -d "${ROOT_DIR}/cmd/gui-qml/help" ]]; then
    expect_help=1
  fi
  python3 - "$tar_path" "$stage_name" "$expect_help" <<'PY'
import sys
import tarfile

tar_path = sys.argv[1]
stage_name = sys.argv[2]
expect_help = sys.argv[3] == "1"

required = {
    f"{stage_name}/grxfirma",
    f"{stage_name}/grxfirma-gui",
    f"{stage_name}/grxfirma-desktop",
    f"{stage_name}/grxfirma-afirmauri",
    f"{stage_name}/grxfirma-nativehost",
    f"{stage_name}/grxfirma-pkcs11-worker",
    f"{stage_name}/README_LINUX_SUITE.md",
    f"{stage_name}/install-suite.sh",
    f"{stage_name}/configure-browsers.sh",
    f"{stage_name}/check-runtime-dependencies.sh",
    f"{stage_name}/runtime-dependencies.qml",
    f"{stage_name}/VERSION.txt",
    f"{stage_name}/help/NOVEDADES.md",
    f"{stage_name}/BUILDINFO",
    f"{stage_name}/icons/hicolor/scalable/apps/grxfirma.svg",
    f"{stage_name}/icons/hicolor/48x48/apps/grxfirma.png",
    f"{stage_name}/icons/hicolor/128x128/apps/grxfirma.png",
    f"{stage_name}/icons/hicolor/256x256/apps/grxfirma.png",
    f"{stage_name}/extensions/grxfirma-extension-chromium.zip",
    f"{stage_name}/extensions/grxfirma-extension-firefox.xpi",
    f"{stage_name}/extensions/grxfirma-extension-firefox.metadata.json",
}
if expect_help:
    required.add(f"{stage_name}/help/")

with tarfile.open(tar_path, "r:gz") as tf:
    names = set(tf.getnames())

missing = []
for entry in sorted(required):
    if entry.endswith("/"):
        if not any(name.startswith(entry) for name in names):
            missing.append(entry)
    elif entry not in names:
        missing.append(entry)
if missing:
    raise SystemExit("tar artifact incomplete:\n  - " + "\n  - ".join(missing))
PY
}

validate_deb_artifact() {
  local deb_path="$1"
  if [[ ! -f "${deb_path}" || ! -s "${deb_path}" ]]; then
    echo "error: el paquete .deb no se ha generado correctamente: ${deb_path}" >&2
    exit 1
  fi
  local pkg_name pkg_arch pkg_depends
  pkg_name="$(dpkg-deb -f "${deb_path}" Package)"
  pkg_arch="$(dpkg-deb -f "${deb_path}" Architecture)"
  pkg_depends="$(dpkg-deb -f "${deb_path}" Depends)"
  if [[ "${pkg_name}" != "grxfirma" ]]; then
    echo "error: paquete .deb inesperado, Package=${pkg_name}" >&2
    exit 1
  fi
  if [[ "${pkg_arch}" != "$(deb_arch "${ARCH}")" ]]; then
    echo "error: arquitectura .deb inesperada, Architecture=${pkg_arch}" >&2
    exit 1
  fi
  if [[ -z "${pkg_depends}" || "${pkg_depends}" != "${RUNTIME_DEPENDS}" ]]; then
    echo "error: Depends del .deb no coincide con el análisis runtime" >&2
    echo "       esperado: ${RUNTIME_DEPENDS}" >&2
    echo "       obtenido: ${pkg_depends}" >&2
    exit 1
  fi
  local required_dependency
  for required_dependency in "${REQUIRED_RUNTIME_DEPENDS[@]}"; do
    if [[ ", ${pkg_depends}," != *", ${required_dependency},"* ]]; then
      echo "error: Depends del .deb no contiene ${required_dependency}" >&2
      exit 1
    fi
  done
  local listing
  listing="$(dpkg-deb -c "${deb_path}")"
  local required=(
    "./usr/bin/grxfirma"
    "./usr/bin/grxfirma-gui"
    "./usr/bin/grxfirma-desktop"
    "./usr/bin/grxfirma-afirmauri"
    "./usr/lib/grxfirma/bin/grxfirma-nativehost"
    "./usr/lib/grxfirma/bin/grxfirma-pkcs11-worker"
    "./usr/lib/grxfirma/extensions/grxfirma-extension-chromium.zip"
    "./usr/lib/grxfirma/extensions/grxfirma-extension-firefox.metadata.json"
    "./usr/share/doc/grxfirma/BUILDINFO"
    "./usr/share/doc/grxfirma/README_LINUX_SUITE.md"
    "./usr/share/icons/hicolor/scalable/apps/grxfirma.svg"
    "./usr/share/icons/hicolor/48x48/apps/grxfirma.png"
    "./usr/share/icons/hicolor/128x128/apps/grxfirma.png"
    "./usr/share/icons/hicolor/256x256/apps/grxfirma.png"
    "./usr/lib/grxfirma/gui-qml/help/NOVEDADES.md"
    "./usr/lib/grxfirma/gui-qml/VERSION.txt"
  )
  local missing=()
  for entry in "${required[@]}"; do
    grep -Fq "${entry}" <<<"${listing}" || missing+=("${entry}")
  done
  if [[ "${#missing[@]}" -gt 0 ]]; then
    printf 'error: paquete .deb incompleto. Faltan:\n' >&2
    printf '  - %s\n' "${missing[@]}" >&2
    exit 1
  fi
  if [[ -d "${ROOT_DIR}/cmd/gui-qml/help" ]]; then
    if ! grep -Fq "./usr/lib/grxfirma/gui-qml/help/" <<<"${listing}"; then
      echo "error: paquete .deb incompleto. Falta ./usr/lib/grxfirma/gui-qml/help/" >&2
      exit 1
    fi
  fi
  local build_info
  build_info="$(dpkg-deb --fsys-tarfile "${deb_path}" |
    tar -xOf - ./usr/share/doc/grxfirma/BUILDINFO)"
  if ! grep -Fxq "sourceCommit=${SOURCE_COMMIT}" <<<"${build_info}" ||
      ! grep -Fxq "sourceTreeState=${SOURCE_TREE_STATE}" <<<"${build_info}"; then
    echo "error: BUILDINFO del .deb no identifica el árbol compilado" >&2
    exit 1
  fi
}

resolve_source_identity() {
  SOURCE_COMMIT="$(git -C "${ROOT_DIR}" rev-parse --verify 'HEAD^{commit}' 2>/dev/null || true)"
  if [[ -z "${SOURCE_COMMIT}" ]]; then
    SOURCE_COMMIT="unknown"
    SOURCE_TREE_STATE="unknown"
    return 0
  fi

  SOURCE_COMMIT="${SOURCE_COMMIT,,}"
  if git -C "${ROOT_DIR}" diff --quiet --ignore-submodules HEAD -- &&
      git -C "${ROOT_DIR}" diff --cached --quiet --ignore-submodules HEAD -- &&
      [[ -z "$(git -C "${ROOT_DIR}" ls-files --others --exclude-standard | head -n 1)" ]]; then
    SOURCE_TREE_STATE="clean"
  else
    SOURCE_TREE_STATE="dirty"
  fi
}

write_build_info() {
  {
    printf 'formatVersion=1\n'
    printf 'version=%s\n' "${RAW_VERSION}"
    printf 'sourceCommit=%s\n' "${SOURCE_COMMIT}"
    printf 'sourceTreeState=%s\n' "${SOURCE_TREE_STATE}"
    printf 'sourceDateEpoch=%s\n' "${SOURCE_DATE_EPOCH}"
    printf 'architecture=%s\n' "${ARCH}"
    printf 'pkcs11Capability=disabled\n'
    printf 'pkcs11WorkerCGO=1\n'
    printf 'pkcs11WorkerSandbox=bwrap-fd-mounts+seccomp-tsync\n'
    printf 'pkcs11WorkerSandboxBaseline=bubblewrap-0.11.1\n'
    printf 'pkcs11WorkerSHA256=%s\n' "$(sha256sum "${STAGE_DIR}/grxfirma-pkcs11-worker" | cut -d ' ' -f 1)"
  } > "${STAGE_DIR}/BUILDINFO"
}

write_sha256sums() {
  local checksum_path="${OUT_DIR}/SHA256SUMS.txt"
  local filenames=("$(basename "${TAR_PATH}")")
  if [[ "${DO_DEB}" == "1" ]]; then
    filenames+=("$(basename "${DEB_PATH}")")
  fi
  (
    cd "${OUT_DIR}"
    printf './%s\0' "${filenames[@]}" | sort -z | xargs -0 sha256sum > "${checksum_path}"
  )
  if [[ ! -f "${checksum_path}" || ! -s "${checksum_path}" ]]; then
    echo "error: no se pudo generar SHA256SUMS.txt para la suite Linux." >&2
    exit 1
  fi
}

verify_sha256sums() {
  (
    cd "${OUT_DIR}"
    sha256sum -c "SHA256SUMS.txt"
  )
}

write_artifacts_report() {
  local report_path="${OUT_DIR}/ARTIFACTS.md"
  python3 - "${OUT_DIR}" "$(basename "${TAR_PATH}")" "$(basename "${DEB_PATH:-}")" <<'PY'
import hashlib
import sys
from pathlib import Path

out_dir = Path(sys.argv[1])
candidates = [sys.argv[2], sys.argv[3]]
lines = [
    "# Linux Suite Artifacts",
    "",
    "| File | Size (bytes) | SHA-256 |",
    "| --- | ---: | --- |",
]
for name in candidates:
    if not name:
        continue
    path = out_dir / name
    if not path.is_file():
        continue
    lines.append(f"| `{name}` | {path.stat().st_size} | `{hashlib.sha256(path.read_bytes()).hexdigest()}` |")
(out_dir / "ARTIFACTS.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
  if [[ ! -f "${report_path}" || ! -s "${report_path}" ]]; then
    echo "error: no se pudo generar ARTIFACTS.md para la suite Linux." >&2
    exit 1
  fi
}

for arg in "$@"; do
  case "${arg}" in
    --deb)
      DO_DEB=1
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: argumento no soportado: ${arg}" >&2
      usage >&2
      exit 1
      ;;
  esac
done

ensure_supported_arch
resolve_source_identity

deb_arch() {
  case "$1" in
    amd64) echo "amd64" ;;
    arm64) echo "arm64" ;;
    arm) echo "armhf" ;;
    386) echo "i386" ;;
    *)
      echo "$1"
      ;;
  esac
}

mkdir -p "${STAGE_DIR}/man"
mkdir -p "${OUT_DIR}"
rm -f "${OUT_DIR}/GrxFirma-${RAW_VERSION}-linux-${ARCH}.tar.gz"
rm -f \
  "${STAGE_DIR}/grxfirma" \
  "${STAGE_DIR}/grxfirma-gui" \
  "${STAGE_DIR}/grxfirma-desktop" \
  "${STAGE_DIR}/grxfirma-gui-qml" \
  "${STAGE_DIR}/grxfirma-afirmauri" \
  "${STAGE_DIR}/grxfirma-nativehost" \
  "${STAGE_DIR}/grxfirma-pkcs11-worker" \
  "${STAGE_DIR}/README_LINUX_SUITE.md" \
  "${STAGE_DIR}/install-suite.sh" \
  "${STAGE_DIR}/configure-browsers.sh" \
  "${STAGE_DIR}/check-runtime-dependencies.sh" \
  "${STAGE_DIR}/runtime-dependencies.qml" \
  "${STAGE_DIR}/VERSION.txt" \
  "${STAGE_DIR}/BUILDINFO"
rm -f "${STAGE_DIR}"/grxfirma*.desktop
rm -f "${STAGE_DIR}/man/"*
rm -rf "${STAGE_DIR}/extensions"
rm -rf "${STAGE_DIR}/qml" "${STAGE_DIR}/assets" "${STAGE_DIR}/help"

echo "Compilando suite de Linux (${ARCH})..."
grxfirma_go_build "${RAW_VERSION}" "" -tags production -o "${STAGE_DIR}/grxfirma" ./cmd/grxfirma
grxfirma_go_build "${RAW_VERSION}" "" -tags production -o "${STAGE_DIR}/grxfirma-gui" ./cmd/grxfirma-gui
grxfirma_go_build "${RAW_VERSION}" "" -tags production,fyne_gui -o "${STAGE_DIR}/grxfirma-desktop" ./cmd/grxfirma
grxfirma_go_build "${RAW_VERSION}" "" -tags production,fyne_gui -o "${STAGE_DIR}/grxfirma-afirmauri" ./cmd/grxfirmauri
grxfirma_go_build "${RAW_VERSION}" "" -tags production -o "${STAGE_DIR}/grxfirma-nativehost" ./cmd/nativehost
grxfirma_build_pkcs11_worker "${RAW_VERSION}" "${STAGE_DIR}/grxfirma-pkcs11-worker"
if qmake_cmd="$(resolve_qmake)"; then
  echo "Compilando frontend Qt/QML..."
  if [[ -L "${QT_BUILD_DIR}" ]]; then
    echo "error: el directorio de build Qt no puede ser un enlace simbólico: ${QT_BUILD_DIR}" >&2
    exit 1
  fi
  if [[ -d "${QT_BUILD_DIR}" ]]; then
    find "${QT_BUILD_DIR}" -depth -delete
  fi
  mkdir -p "${QT_BUILD_DIR}"
  "${qmake_cmd}" \
    -o "${QT_BUILD_DIR}/Makefile" \
    "${ROOT_DIR}/cmd/gui-qml/grxfirma_qt.pro"
  make -C "${QT_BUILD_DIR}" -j"$(nproc)"
  install -m 755 "${QT_BUILD_DIR}/grxfirma-gui-qml" "${STAGE_DIR}/grxfirma-gui-qml"
  find "${QT_BUILD_DIR}" -depth -delete
  rm -rf "${STAGE_DIR}/qml" "${STAGE_DIR}/assets" "${STAGE_DIR}/help"
  mkdir -p "${STAGE_DIR}"
  command cp -R "${ROOT_DIR}/cmd/gui-qml/qml" "${STAGE_DIR}/qml"
  command cp -R "${ROOT_DIR}/cmd/gui-qml/assets" "${STAGE_DIR}/assets"
  if [[ -d "${ROOT_DIR}/cmd/gui-qml/help" ]]; then
    command cp -R "${ROOT_DIR}/cmd/gui-qml/help" "${STAGE_DIR}/help"
  fi
  QML_RUNTIME_ROOT="$(grxfirma_qml_root_for_qmake "${qmake_cmd}")"
  grxfirma_collect_qml_imports "${ROOT_DIR}/cmd/gui-qml/qml" "${QML_RUNTIME_MANIFEST}"
else
  echo "Aviso: se requiere Qt6/qmake6 para generar el frontend Qt/QML; la suite saldrá sin Qt." >&2
  : > "${QML_RUNTIME_MANIFEST}"
fi

mkdir -p "${STAGE_DIR}/help"
cp "${ROOT_DIR}/docs/NOVEDADES.md" "${STAGE_DIR}/help/NOVEDADES.md"
# La interfaz Qt lee su versión de VERSION.txt (Acerca de, avisos de actualización).
cp "${ROOT_DIR}/VERSION.txt" "${STAGE_DIR}/VERSION.txt"

cp "${ROOT_DIR}/packaging/linux/README_LINUX_SUITE.md" "${STAGE_DIR}/README_LINUX_SUITE.md"
cp "${ROOT_DIR}/packaging/linux/install-suite.sh" "${STAGE_DIR}/install-suite.sh"
cp "${ROOT_DIR}/packaging/linux/configure-browsers.sh" "${STAGE_DIR}/configure-browsers.sh"
cp "${ROOT_DIR}/packaging/linux/runtime-dependencies.sh" "${STAGE_DIR}/check-runtime-dependencies.sh"
mkdir -p "${STAGE_DIR}/extensions"
python3 "${ROOT_DIR}/packaging/browser-extensions/build.py" --output-dir "${STAGE_DIR}/extensions"
python3 "${ROOT_DIR}/packaging/browser-extensions/verify_package.py" "${STAGE_DIR}/extensions"
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
write_build_info
chmod 755 "${STAGE_DIR}/install-suite.sh"
chmod 755 "${STAGE_DIR}/configure-browsers.sh"
chmod 755 "${STAGE_DIR}/check-runtime-dependencies.sh"
"${STAGE_DIR}/check-runtime-dependencies.sh" --check-bundle "${STAGE_DIR}"

rm -f "${TAR_PATH}"
grxfirma_reproducible_tar "${STAGE_DIR}" "${TAR_PATH}"
validate_tar_artifact "${TAR_PATH}"
echo "Bundle generado en: ${TAR_PATH}"

if [[ "${DO_DEB}" != "1" ]]; then
  write_sha256sums
  verify_sha256sums
  write_artifacts_report
  exit 0
fi

if ! command -v dpkg-deb >/dev/null 2>&1; then
  echo "error: dpkg-deb no está disponible; no se puede generar el .deb" >&2
  exit 1
fi

PKG_VERSION="${RAW_VERSION}"
if [[ ! "${PKG_VERSION}" =~ ^[0-9][0-9A-Za-z.+~-]*$ ]]; then
  echo "error: VERSION.txt no contiene una versión Debian válida: ${PKG_VERSION}" >&2
  exit 1
fi
PKG_ARCH="$(deb_arch "${ARCH}")"
PKG_ROOT="${OUT_DIR}/debroot"
DEB_PATH="${OUT_DIR}/grxfirma_${PKG_VERSION}_${PKG_ARCH}.deb"

rm -rf "${PKG_ROOT}"
mkdir -p \
  "${PKG_ROOT}/DEBIAN" \
  "${PKG_ROOT}/usr/bin" \
  "${PKG_ROOT}/usr/lib/grxfirma/bin" \
  "${PKG_ROOT}/usr/lib/grxfirma/extensions" \
  "${PKG_ROOT}/usr/lib/grxfirma/gui-qml" \
  "${PKG_ROOT}/usr/share/applications" \
  "${PKG_ROOT}/usr/share/doc/grxfirma" \
  "${PKG_ROOT}/usr/share/man/man1" \
  "${PKG_ROOT}/usr/share/man/man7" \
  "${PKG_ROOT}/usr/share/google-chrome/extensions" \
  "${PKG_ROOT}/usr/share/chromium/extensions" \
  "${PKG_ROOT}/etc/opt/chrome/native-messaging-hosts" \
  "${PKG_ROOT}/etc/chromium/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/edge/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/brave.com/brave/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/vivaldi/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/opera/native-messaging-hosts" \
  "${PKG_ROOT}/usr/lib/mozilla/native-messaging-hosts"

install -m 755 "${STAGE_DIR}/grxfirma" "${PKG_ROOT}/usr/bin/grxfirma"
install -m 755 "${STAGE_DIR}/grxfirma-gui" "${PKG_ROOT}/usr/bin/grxfirma-gui"
install -m 755 "${STAGE_DIR}/grxfirma-desktop" "${PKG_ROOT}/usr/bin/grxfirma-desktop"
if [[ -f "${STAGE_DIR}/grxfirma-gui-qml" ]]; then
  install -m 755 "${STAGE_DIR}/grxfirma-gui-qml" "${PKG_ROOT}/usr/bin/grxfirma-gui-qml"
  install -m 644 "${STAGE_DIR}/VERSION.txt" "${PKG_ROOT}/usr/lib/grxfirma/gui-qml/VERSION.txt"
fi
if [[ -d "${STAGE_DIR}/qml" ]]; then
  cp -a "${STAGE_DIR}/qml" "${PKG_ROOT}/usr/lib/grxfirma/gui-qml/"
fi
if [[ -d "${STAGE_DIR}/assets" ]]; then
  cp -a "${STAGE_DIR}/assets" "${PKG_ROOT}/usr/lib/grxfirma/gui-qml/"
fi
install -m 644 "${STAGE_DIR}/VERSION.txt" "${PKG_ROOT}/usr/lib/grxfirma/gui-qml/VERSION.txt"
if [[ -d "${STAGE_DIR}/help" ]]; then
  cp -a "${STAGE_DIR}/help" "${PKG_ROOT}/usr/lib/grxfirma/gui-qml/"
fi
install -m 755 "${STAGE_DIR}/grxfirma-afirmauri" "${PKG_ROOT}/usr/bin/grxfirma-afirmauri"
install -m 755 "${STAGE_DIR}/grxfirma-nativehost" "${PKG_ROOT}/usr/lib/grxfirma/bin/grxfirma-nativehost"
install -m 755 "${STAGE_DIR}/grxfirma-pkcs11-worker" "${PKG_ROOT}/usr/lib/grxfirma/bin/grxfirma-pkcs11-worker"
install -m 644 "${STAGE_DIR}/extensions/grxfirma-extension-chromium.zip" "${PKG_ROOT}/usr/lib/grxfirma/extensions/grxfirma-extension-chromium.zip"
if [[ -f "${STAGE_DIR}/extensions/grxfirma-extension-firefox.xpi" ]]; then
  install -m 644 "${STAGE_DIR}/extensions/grxfirma-extension-firefox.xpi" "${PKG_ROOT}/usr/lib/grxfirma/extensions/grxfirma-extension-firefox.xpi"
fi
if [[ -f "${STAGE_DIR}/extensions/grxfirma-extension-firefox.metadata.json" ]]; then
  install -m 644 "${STAGE_DIR}/extensions/grxfirma-extension-firefox.metadata.json" "${PKG_ROOT}/usr/lib/grxfirma/extensions/grxfirma-extension-firefox.metadata.json"
fi
install -m 644 "${STAGE_DIR}/README_LINUX_SUITE.md" "${PKG_ROOT}/usr/share/doc/grxfirma/README_LINUX_SUITE.md"
for icon_size in 48 128 256; do
  install -D -m 644 "${STAGE_DIR}/icons/hicolor/${icon_size}x${icon_size}/apps/grxfirma.png" \
    "${PKG_ROOT}/usr/share/icons/hicolor/${icon_size}x${icon_size}/apps/grxfirma.png"
done
install -D -m 644 "${STAGE_DIR}/icons/hicolor/scalable/apps/grxfirma.svg" \
  "${PKG_ROOT}/usr/share/icons/hicolor/scalable/apps/grxfirma.svg"
install -m 644 "${STAGE_DIR}/BUILDINFO" "${PKG_ROOT}/usr/share/doc/grxfirma/BUILDINFO"
install -m 644 "${STAGE_DIR}/man/grxfirma.1" "${PKG_ROOT}/usr/share/man/man1/grxfirma.1"
for manpage in "${STAGE_DIR}"/man/*.7; do
  install -m 644 "${manpage}" "${PKG_ROOT}/usr/share/man/man7/$(basename "${manpage}")"
done

cat > "${PKG_ROOT}/usr/lib/grxfirma/bin/browser-bridge.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exec /usr/lib/grxfirma/bin/grxfirma-nativehost "$@"
EOF
chmod 755 "${PKG_ROOT}/usr/lib/grxfirma/bin/browser-bridge.sh"

cat > "${PKG_ROOT}/usr/lib/grxfirma/bin/afirmauri-handler.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
source "${HOME}/.local/lib/grxfirma/config/env.sh" 2>/dev/null || true
uri="${1:-}"
if [[ -z "${uri}" ]]; then
  exec /usr/lib/grxfirma/bin/desktop-launcher.sh
fi
export GRXFIRMA_PROTOCOL_UI=1
exec /usr/bin/grxfirma-afirmauri "${uri}"
EOF
chmod 755 "${PKG_ROOT}/usr/lib/grxfirma/bin/afirmauri-handler.sh"

cat > "${PKG_ROOT}/usr/lib/grxfirma/bin/desktop-launcher.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ -x /usr/bin/grxfirma-gui-qml ]]; then
  if [[ -x /usr/bin/grxfirma-gui ]]; then
    exec /usr/bin/grxfirma-gui "$@"
  fi
  exec /usr/bin/grxfirma-gui-qml "$@"
fi
exec /usr/bin/grxfirma-desktop "$@"
EOF
chmod 755 "${PKG_ROOT}/usr/lib/grxfirma/bin/desktop-launcher.sh"
install -m 755 "${STAGE_DIR}/configure-browsers.sh" "${PKG_ROOT}/usr/lib/grxfirma/bin/configure-browsers.sh"

sed 's|^Exec=.*|Exec=/usr/lib/grxfirma/bin/afirmauri-handler.sh %u|' "${STAGE_DIR}/grxfirma.desktop" > "${PKG_ROOT}/usr/share/applications/grxfirma.desktop"
sed 's|^Exec=.*|Exec=/usr/lib/grxfirma/bin/desktop-launcher.sh|' "${STAGE_DIR}/grxfirma-manual.desktop" > "${PKG_ROOT}/usr/share/applications/grxfirma-manual.desktop"

write_chrome_manifest() {
  local target="$1"
  local name="$2"
  local origins="$3"
  cat > "${target}" <<EOF
{
  "name": "${name}",
  "description": "GrxFirma Native Messaging Host",
  "path": "/usr/lib/grxfirma/bin/browser-bridge.sh",
  "type": "stdio",
  "allowed_origins": [${origins}]
}
EOF
  chmod 0644 "${target}"
}

write_firefox_manifest() {
  local target="$1"
  local name="$2"
  local extensions="$3"
  cat > "${target}" <<EOF
{
  "name": "${name}",
  "description": "GrxFirma Native Messaging Host",
  "path": "/usr/lib/grxfirma/bin/browser-bridge.sh",
  "type": "stdio",
  "allowed_extensions": [${extensions}]
}
EOF
  chmod 0644 "${target}"
}

CHROMIUM_ALLOWED_ORIGINS='"chrome-extension://pkefjandjcgdmhoonmhnllikibobijgg/"'
if [[ -f "${STAGE_DIR}/extensions/grxfirma-extension-chromium.id" ]]; then
  CHROMIUM_PACKAGE_ID="$(tr -d '\r\n' < "${STAGE_DIR}/extensions/grxfirma-extension-chromium.id")"
  if [[ -n "${CHROMIUM_PACKAGE_ID}" && "${CHROMIUM_PACKAGE_ID}" != "pkefjandjcgdmhoonmhnllikibobijgg" ]]; then
    CHROMIUM_ALLOWED_ORIGINS+=",\"chrome-extension://${CHROMIUM_PACKAGE_ID}/\""
  fi
fi

for dir in \
  "${PKG_ROOT}/etc/opt/chrome/native-messaging-hosts" \
  "${PKG_ROOT}/etc/chromium/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/edge/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/brave.com/brave/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/vivaldi/native-messaging-hosts" \
  "${PKG_ROOT}/etc/opt/opera/native-messaging-hosts"
do
  write_chrome_manifest "${dir}/com.grxfirma.native.json" "com.grxfirma.native" "${CHROMIUM_ALLOWED_ORIGINS}"
  write_chrome_manifest "${dir}/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${CHROMIUM_ALLOWED_ORIGINS}"
  write_chrome_manifest "${dir}/io.github.aavidad.portafirmas.json" "io.github.aavidad.portafirmas" '"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/","chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/"'
done

write_firefox_manifest "${PKG_ROOT}/usr/lib/mozilla/native-messaging-hosts/com.grxfirma.native.json" "com.grxfirma.native" '"grxfirma@aavidad.github.io"'
write_firefox_manifest "${PKG_ROOT}/usr/lib/mozilla/native-messaging-hosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" '"grxfirma@aavidad.github.io"'
write_firefox_manifest "${PKG_ROOT}/usr/lib/mozilla/native-messaging-hosts/io.github.aavidad.portafirmas.json" "io.github.aavidad.portafirmas" '"portafirmas@dipgra.es"'


RUNTIME_DEPENDS="$(
  grxfirma_generate_debian_depends \
    "${PKG_ROOT}" \
    "grxfirma" \
    "${QML_RUNTIME_MANIFEST}" \
    "${QML_RUNTIME_ROOT}" \
    "${OUT_DIR}"
)"
RUNTIME_DEPENDS="$(
  grxfirma_merge_debian_depends \
    "${RUNTIME_DEPENDS}" \
    "${REQUIRED_RUNTIME_DEPENDS[@]}"
)"
if [[ -z "${RUNTIME_DEPENDS}" ]]; then
  echo "error: el análisis runtime produjo un campo Depends vacío" >&2
  exit 1
fi

cat > "${PKG_ROOT}/DEBIAN/control" <<EOF
Package: grxfirma
Version: ${PKG_VERSION}
Section: utils
Priority: optional
Architecture: ${PKG_ARCH}
Maintainer: Alberto Avidad Fernández <avidad@dipgra.es>
Depends: ${RUNTIME_DEPENDS}
Recommends: zenity | kdialog | qarma, qt6-translations-l10n
Suggests: bubblewrap (>= 0.11.1), pinentry-qt | pinentry-gnome3 | pinentry-gtk2
Description: GrxFirma para Linux
 CLI, interfaz desktop, handler afirma:// y Native Messaging Host.
EOF

cat > "${PKG_ROOT}/DEBIAN/preinst" <<'PREINST_EOF'
#!/usr/bin/env bash
set -e
# Detiene todos los procesos de GrxFirma (interfaz, bandeja, REST, WebSocket,
# host nativo) de cualquier usuario antes de reemplazar o retirar ficheros. Solo
# se actúa sobre ejecutables cuya ruta real está bajo la instalación del paquete.
grxfirma_stop_processes() {
  local state_file="${1:-}" proc pid exe uid user alive pids=()
  if [[ -n "${state_file}" ]]; then
    : > "${state_file}"
    chmod 600 "${state_file}"
  fi
  for proc in /proc/[0-9]*; do
    pid="${proc#/proc/}"
    exe="$(readlink "${proc}/exe" 2>/dev/null || true)"
    exe="${exe% (deleted)}"
    case "${exe}" in
      /usr/bin/grxfirma|/usr/bin/grxfirma-*|/usr/lib/grxfirma/*) ;;
      *) continue ;;
    esac
    pids+=("${pid}")
    if [[ -n "${state_file}" && ( "${exe}" == /usr/bin/grxfirma-gui || "${exe}" == /usr/bin/grxfirma-gui-qml ) ]]; then
      uid="$(stat -c %u "${proc}" 2>/dev/null || true)"
      user="$(getent passwd "${uid}" 2>/dev/null | cut -d: -f1)"
      if [[ -n "${user}" && "${uid}" != "0" ]]; then
        echo "${user}" >> "${state_file}"
      fi
    fi
  done
  [[ "${#pids[@]}" -gt 0 ]] || return 0
  echo "GrxFirma: cerrando la aplicación y sus servicios antes de continuar..." >&2
  kill -TERM "${pids[@]}" 2>/dev/null || true
  for _ in $(seq 1 20); do
    alive=0
    for pid in "${pids[@]}"; do
      if kill -0 "${pid}" 2>/dev/null; then alive=1; fi
    done
    [[ "${alive}" -eq 0 ]] && return 0
    sleep 0.5
  done
  kill -KILL "${pids[@]}" 2>/dev/null || true
}
case "${1:-}" in
  install|upgrade)
    mkdir -p /run/grxfirma && chmod 700 /run/grxfirma
    grxfirma_stop_processes /run/grxfirma/relanzar-usuarios
    ;;
esac
PREINST_EOF
chmod 755 "${PKG_ROOT}/DEBIAN/preinst"

cat > "${PKG_ROOT}/DEBIAN/postinst" <<'EOF'
#!/usr/bin/env bash
set -e
if command -v update-desktop-database >/dev/null 2>&1; then
  if [[ -d /usr/local/share/applications ]]; then
    update-desktop-database /usr/local/share/applications || true
  fi
  update-desktop-database /usr/share/applications || true
fi
if command -v update-mime-database >/dev/null 2>&1; then
  update-mime-database /usr/share/mime || true
fi
target_user="${SUDO_USER:-}"
if [[ -n "${target_user}" && "${target_user}" != "root" ]] && command -v getent >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1; then
  target_home="$(getent passwd "${target_user}" | cut -d: -f6)"
  if [[ -n "${target_home}" && -d "${target_home}" ]]; then
    runuser -u "${target_user}" -- env \
      GRXFIRMA_TARGET_HOME="${target_home}" \
      GRXFIRMA_DESKTOP_ID="grxfirma.desktop" \
      GRXFIRMA_BROWSER_BRIDGE="/usr/lib/grxfirma/bin/browser-bridge.sh" \
      GRXFIRMA_FIREFOX_XPI="/usr/lib/grxfirma/extensions/grxfirma-extension-firefox.xpi" \
      GRXFIRMA_FIREFOX_METADATA="/usr/lib/grxfirma/extensions/grxfirma-extension-firefox.metadata.json" \
      /usr/lib/grxfirma/bin/configure-browsers.sh || true
  fi
fi
# Vuelve a abrir GrxFirma (versión nueva) a quien la tenía abierta al actualizar.
relaunch_file=/run/grxfirma/relanzar-usuarios
if [[ -f "${relaunch_file}" && ! -L "${relaunch_file}" ]] && command -v systemd-run >/dev/null 2>&1; then
  sort -u "${relaunch_file}" | while read -r relaunch_user; do
    [[ "${relaunch_user}" =~ ^[a-z_][a-z0-9_.-]*$ ]] || continue
    systemd-run --user --machine="${relaunch_user}@.host" --collect --quiet \
      /usr/bin/grxfirma-gui >/dev/null 2>&1 || true
  done
fi
rm -f "${relaunch_file}"
EOF
chmod 755 "${PKG_ROOT}/DEBIAN/postinst"

cat > "${PKG_ROOT}/DEBIAN/prerm" <<'EOF'
#!/usr/bin/env bash
set -e
# Detiene todos los procesos de GrxFirma (interfaz, bandeja, REST, WebSocket,
# host nativo) de cualquier usuario antes de reemplazar o retirar ficheros. Solo
# se actúa sobre ejecutables cuya ruta real está bajo la instalación del paquete.
grxfirma_stop_processes() {
  local state_file="${1:-}" proc pid exe uid user alive pids=()
  if [[ -n "${state_file}" ]]; then
    : > "${state_file}"
    chmod 600 "${state_file}"
  fi
  for proc in /proc/[0-9]*; do
    pid="${proc#/proc/}"
    exe="$(readlink "${proc}/exe" 2>/dev/null || true)"
    exe="${exe% (deleted)}"
    case "${exe}" in
      /usr/bin/grxfirma|/usr/bin/grxfirma-*|/usr/lib/grxfirma/*) ;;
      *) continue ;;
    esac
    pids+=("${pid}")
    if [[ -n "${state_file}" && ( "${exe}" == /usr/bin/grxfirma-gui || "${exe}" == /usr/bin/grxfirma-gui-qml ) ]]; then
      uid="$(stat -c %u "${proc}" 2>/dev/null || true)"
      user="$(getent passwd "${uid}" 2>/dev/null | cut -d: -f1)"
      if [[ -n "${user}" && "${uid}" != "0" ]]; then
        echo "${user}" >> "${state_file}"
      fi
    fi
  done
  [[ "${#pids[@]}" -gt 0 ]] || return 0
  echo "GrxFirma: cerrando la aplicación y sus servicios antes de continuar..." >&2
  kill -TERM "${pids[@]}" 2>/dev/null || true
  for _ in $(seq 1 20); do
    alive=0
    for pid in "${pids[@]}"; do
      if kill -0 "${pid}" 2>/dev/null; then alive=1; fi
    done
    [[ "${alive}" -eq 0 ]] && return 0
    sleep 0.5
  done
  kill -KILL "${pids[@]}" 2>/dev/null || true
}
[[ "${1:-}" == "remove" ]] || exit 0
grxfirma_stop_processes

# La CA y su inventario pertenecen al usuario. Durante la retirada del .deb
# el binario sigue disponible, pero dpkg no conoce a todos sus usuarios.
target_user="${SUDO_USER:-}"
if [[ -z "${target_user}" || "${target_user}" == "root" ]] ||
   ! command -v getent >/dev/null 2>&1 ||
   ! command -v runuser >/dev/null 2>&1; then
  echo 'GrxFirma: cada usuario que haya usado la aplicación debe ejecutar grxfirma-afirmauri --remove-local-tls-trust antes de retirar el paquete.' >&2
  exit 0
fi
target_uid="$(id -u "${target_user}" 2>/dev/null || true)"
target_home="$(getent passwd "${target_user}" | cut -d: -f6)"
if [[ -z "${target_uid}" || "${target_uid}" == "0" || -z "${target_home}" || ! -d "${target_home}" ]]; then
  echo "GrxFirma: no se pudo identificar el hogar de ${target_user}; compruebe esa cuenta y repita la desinstalación para poder retirar su CA." >&2
  exit 1
fi
if ! runuser -u "${target_user}" -- env -i \
  HOME="${target_home}" USER="${target_user}" LOGNAME="${target_user}" PATH=/usr/bin:/bin \
  /usr/bin/grxfirma-afirmauri --remove-local-tls-trust; then
  echo "GrxFirma: no se pudo retirar la CA gestionada de ${target_user}. Cierre Firefox y repita la desinstalación; se conserva el paquete para permitir el reintento." >&2
  exit 1
fi
EOF
chmod 755 "${PKG_ROOT}/DEBIAN/prerm"

cat > "${PKG_ROOT}/DEBIAN/postrm" <<'EOF'
#!/usr/bin/env bash
set -e
case "${1:-}" in
  remove|purge) ;;
  *) exit 0 ;;
esac
target_user="${SUDO_USER:-}"
if [[ -n "${target_user}" && "${target_user}" != "root" ]] && command -v getent >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1; then
  target_home="$(getent passwd "${target_user}" | cut -d: -f6)"
  if [[ -n "${target_home}" && -d "${target_home}" ]]; then
    runuser -u "${target_user}" -- env TARGET_HOME="${target_home}" bash <<'EOS' || true
set -e
for dir in \
  "${TARGET_HOME}/.config/google-chrome/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/chromium/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/opera/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/opera-beta/NativeMessagingHosts" \
  "${TARGET_HOME}/.config/opera-developer/NativeMessagingHosts" \
  "${TARGET_HOME}/.mozilla/native-messaging-hosts" \
  "${TARGET_HOME}/snap/firefox/common/.mozilla/native-messaging-hosts" \
  "${TARGET_HOME}/.var/app/org.mozilla.firefox/.mozilla/native-messaging-hosts"
do
  rm -f "${dir}/com.grxfirma.native.json" "${dir}/io.github.aavidad.grxfirma.json" "${dir}/io.github.aavidad.portafirmas.json"
  # Nombres de versiones anteriores.
  rm -f "${dir}/com.dipgra.grxfirma.json" "${dir}/com.dipgra.portafirmas.json"
done
for root in \
  "${TARGET_HOME}/.mozilla/firefox" \
  "${TARGET_HOME}/snap/firefox/common/.mozilla/firefox" \
  "${TARGET_HOME}/.var/app/org.mozilla.firefox/.mozilla/firefox"
do
  [[ -d "${root}" ]] || continue
  find "${root}" -mindepth 3 -maxdepth 3 -path '*/extensions/grxfirma@aavidad.github.io.xpi' -type f -delete 2>/dev/null || true
  find "${root}" -mindepth 3 -maxdepth 3 -path '*/extensions/extension@dipgra.es.xpi' -type f -delete 2>/dev/null || true
done
EOS
  fi
fi
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database /usr/share/applications || true
fi
if command -v update-mime-database >/dev/null 2>&1; then
  update-mime-database /usr/share/mime || true
fi
EOF
chmod 755 "${PKG_ROOT}/DEBIAN/postrm"

rm -f "${DEB_PATH}"
dpkg-deb --root-owner-group --build "${PKG_ROOT}" "${DEB_PATH}"
validate_deb_artifact "${DEB_PATH}"
grxfirma_normalize_output_mtime "${DEB_PATH}"
write_sha256sums
verify_sha256sums
write_artifacts_report
echo "Paquete .deb generado en: ${DEB_PATH}"
