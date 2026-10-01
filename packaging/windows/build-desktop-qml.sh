#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/packaging/windows/reproducible-build.sh"
grxfirma_initialize_reproducible_build "${ROOT_DIR}"
if [[ -f "${ROOT_DIR}/VERSION.txt" ]]; then
  VERSION="$(tr -d '\r\n' < "${ROOT_DIR}/VERSION.txt")"
else
  VERSION="$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")"
fi
ARCH="${GOARCH:-amd64}"
OUT_DIR="${ROOT_DIR}/release/windows-desktop-qml"
STAGE_DIR="${OUT_DIR}/GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}"
BUILD_DIR="${OUT_DIR}/build-${ARCH}"
ZIP_PATH="${OUT_DIR}/GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}.zip"
NSIS_OUT="${OUT_DIR}/GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}-setup.exe"
PROJECT_QML="${ROOT_DIR}/cmd/gui-qml/grxfirma_qt.pro"
HELP_SOURCE_DIR="${ROOT_DIR}/cmd/gui-qml/help"
DO_NSIS=0

usage() {
  cat <<'EOF'
Uso:
  packaging/windows/build-desktop-qml.sh [--nsis]

Variables relevantes:
  GOARCH=amd64
  QMAKE=/ruta/al/qmake6-del-kit-windows
  WINDEPLOYQT=/ruta/al/windeployqt-del-kit-windows
  MAKE=/ruta/a/mingw32-make

Notas:
  - Este script requiere una toolchain Qt para Windows, no la Qt nativa de Linux.
  - Si el qmake apuntado genera binarios Linux, el script fallará con diagnóstico.
EOF
}

ensure_supported_arch() {
  case "${ARCH}" in
    amd64)
      return 0
      ;;
    *)
      echo "error: arquitectura Windows no soportada por este script: ${ARCH}" >&2
      exit 1
      ;;
  esac
}

ensure_nsis_if_requested() {
  if [[ "${DO_NSIS}" != "1" ]]; then
    return 0
  fi
  if ! command -v makensis >/dev/null 2>&1; then
    echo "error: --nsis requiere makensis en PATH." >&2
    exit 1
  fi
}

resolve_qmake() {
  if [[ -n "${QMAKE:-}" ]]; then
    printf '%s\n' "${QMAKE}"
    return 0
  fi
  local candidate
  for candidate in \
    x86_64-w64-mingw32-qmake6 \
    x86_64-w64-mingw32-qmake \
    qmake6 \
    qmake
  do
    if command -v "${candidate}" >/dev/null 2>&1; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  echo "error: no se ha encontrado qmake para Windows. Define QMAKE con el qmake6 del kit MinGW/Windows." >&2
  exit 1
}

resolve_windeployqt() {
  if [[ -n "${WINDEPLOYQT:-}" ]]; then
    printf '%s\n' "${WINDEPLOYQT}"
    return 0
  fi
  local candidate
  for candidate in \
    x86_64-w64-mingw32-windeployqt6 \
    x86_64-w64-mingw32-windeployqt \
    windeployqt6 \
    windeployqt
  do
    if command -v "${candidate}" >/dev/null 2>&1; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  return 1
}

resolve_make() {
  if [[ -n "${MAKE:-}" ]]; then
    printf '%s\n' "${MAKE}"
    return 0
  fi
  local candidate
  for candidate in mingw32-make make; do
    if command -v "${candidate}" >/dev/null 2>&1; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  echo "error: no se ha encontrado una utilidad make válida. Define MAKE con mingw32-make." >&2
  exit 1
}

validate_qmake_for_windows() {
  local qmake_cmd="$1"
  local version spec
  version="$("${qmake_cmd}" -query QT_VERSION 2>/dev/null || true)"
  if [[ -z "${version}" || "${version}" != 6.* ]]; then
    echo "error: se requiere Qt6 para compilar el frontend Qt/QML de Windows." >&2
    exit 1
  fi
  spec="$("${qmake_cmd}" -query QMAKE_XSPEC 2>/dev/null || true)"
  if [[ "${spec}" != *win32* && "${spec}" != *mingw* ]]; then
    echo "error: el qmake detectado no apunta a un kit Windows/MinGW: ${spec:-desconocido}" >&2
    echo "       Usa QMAKE con el qmake6 del kit Windows." >&2
    exit 1
  fi
}

validate_zip_artifact() {
  local zip_path="$1"
  local stage_name
  stage_name="$(basename "${STAGE_DIR}")"
  local expect_help=0
  [[ -d "${HELP_SOURCE_DIR}" ]] && expect_help=1
  python3 - "${zip_path}" "${stage_name}" "${expect_help}" <<'PY'
import sys
import zipfile

zip_path = sys.argv[1]
stage_name = sys.argv[2]
expect_help = sys.argv[3] == "1"

required = {
    f"{stage_name}/grxfirma-gui-qml.exe",
    f"{stage_name}/grxfirma-gui.exe",
    f"{stage_name}/grxfirma.exe",
    f"{stage_name}/install-desktop-qml.ps1",
    f"{stage_name}/uninstall-desktop-qml.ps1",
    f"{stage_name}/install-path-safety.ps1",
    f"{stage_name}/invoke-uninstall-silent.ps1",
    f"{stage_name}/README_DESKTOP_QML_WINDOWS.md",
    f"{stage_name}/qml/",
    f"{stage_name}/assets/",
    f"{stage_name}/VERSION.txt",
}
if expect_help:
    required.add(f"{stage_name}/help/")

with zipfile.ZipFile(zip_path) as zf:
    names = set(zf.namelist())

missing = []
for entry in sorted(required):
    if entry.endswith("/"):
        if not any(name.startswith(entry) for name in names):
            missing.append(entry)
    elif entry not in names:
        missing.append(entry)

if missing:
    raise SystemExit("zip artifact incomplete:\n  - " + "\n  - ".join(missing))
PY
}

validate_nsis_output() {
  local path="$1"
  if [[ ! -f "${path}" || ! -s "${path}" ]]; then
    echo "error: el instalador NSIS no se ha generado correctamente: ${path}" >&2
    exit 1
  fi
}

write_sha256sums() {
  local checksum_path="${OUT_DIR}/SHA256SUMS.txt"
  (
    cd "${OUT_DIR}"
    find . -maxdepth 1 -type f \( \
      -name "GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}.zip" -o \
      -name "GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}-setup.exe" \
    \) -print0 | sort -z | xargs -0 sha256sum > "${checksum_path}"
  )
  if [[ ! -f "${checksum_path}" || ! -s "${checksum_path}" ]]; then
    echo "error: no se pudo generar SHA256SUMS.txt para Desktop Qt/QML Windows." >&2
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
  python3 - "${OUT_DIR}" "$(basename "${ZIP_PATH}")" "$(basename "${NSIS_OUT}")" <<'PY'
import hashlib
import sys
from pathlib import Path

out_dir = Path(sys.argv[1])
candidates = [sys.argv[2], sys.argv[3]]
lines = [
    "# Windows Desktop Qt/QML Artifacts",
    "",
    "| File | Size (bytes) | SHA-256 |",
    "| --- | ---: | --- |",
]
for name in candidates:
    path = out_dir / name
    if not path.is_file():
        continue
    lines.append(f"| `{name}` | {path.stat().st_size} | `{hashlib.sha256(path.read_bytes()).hexdigest()}` |")
(out_dir / "ARTIFACTS.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
  if [[ ! -f "${report_path}" || ! -s "${report_path}" ]]; then
    echo "error: no se pudo generar ARTIFACTS.md para Desktop Qt/QML Windows." >&2
    exit 1
  fi
}

deploy_qt_runtime() {
  local qmake_cmd="$1"
  local deployqt_cmd="$2"

  if [[ -n "${deployqt_cmd}" ]]; then
    echo "Desplegando runtime Qt para Windows con ${deployqt_cmd}..."
    "${deployqt_cmd}" --qmldir "${ROOT_DIR}/cmd/gui-qml/qml" "${STAGE_DIR}/grxfirma-gui-qml.exe"
    return 0
  fi

  local qt_prefix
  qt_prefix="$("${qmake_cmd}" -query QT_INSTALL_PREFIX 2>/dev/null || true)"
  if [[ -z "${qt_prefix}" || ! -d "${qt_prefix}/bin" ]]; then
    echo "error: no se ha encontrado windeployqt ni un prefijo Qt usable para copiar runtime." >&2
    echo "       Define WINDEPLOYQT o usa un qmake de MXE/MinGW con QT_INSTALL_PREFIX válido." >&2
    exit 1
  fi

  echo "Desplegando runtime Qt para Windows copiando desde: ${qt_prefix}"
  find "${qt_prefix}/bin" -maxdepth 1 -type f -name 'Qt6*.dll' -exec cp {} "${STAGE_DIR}/" \;
  for dir in platforms imageformats styles networkinformation tls iconengines generic; do
    if [[ -d "${qt_prefix}/plugins/${dir}" ]]; then
      rm -rf "${STAGE_DIR:?}/${dir}"
      mkdir -p "${STAGE_DIR}/${dir}"
      find "${qt_prefix}/plugins/${dir}" -type f -name '*.dll' -exec cp {} "${STAGE_DIR}/${dir}/" \;
    fi
  done
  if [[ -d "${qt_prefix}/qml" ]]; then
    rm -rf "${STAGE_DIR:?}/qt-qml"
    mkdir -p "${STAGE_DIR}/qt-qml"
    cp -R "${qt_prefix}/qml/QtCore" "${STAGE_DIR}/qt-qml/" 2>/dev/null || true
    cp -R "${qt_prefix}/qml/QtNetwork" "${STAGE_DIR}/qt-qml/" 2>/dev/null || true
    cp -R "${qt_prefix}/qml/QtQml" "${STAGE_DIR}/qt-qml/" 2>/dev/null || true
    cp -R "${qt_prefix}/qml/QtQuick" "${STAGE_DIR}/qt-qml/" 2>/dev/null || true
    if [[ -d "${qt_prefix}/qml/Qt/labs" ]]; then
      mkdir -p "${STAGE_DIR}/qt-qml/Qt"
      cp -R "${qt_prefix}/qml/Qt/labs" "${STAGE_DIR}/qt-qml/Qt/"
    fi
  fi
  cat > "${STAGE_DIR}/qt.conf" <<'EOF'
[Paths]
Plugins = .
Qml2Imports = qt-qml
EOF
}

deploy_mingw_runtime() {
  local qmake_cmd="$1"
  local qt_prefix runtime_dir dll

  qt_prefix="$("${qmake_cmd}" -query QT_INSTALL_PREFIX 2>/dev/null || true)"
  runtime_dir=""
  if [[ -n "${qt_prefix}" ]]; then
    runtime_dir="$(cd "${qt_prefix}/.." 2>/dev/null && pwd)/bin"
  fi
  if [[ ! -d "${runtime_dir}" ]]; then
    runtime_dir="$(dirname "$(command -v x86_64-w64-mingw32-g++ 2>/dev/null || true)")"
  fi

  for dll in \
    libgcc_s_seh-1.dll \
    libstdc++-6.dll \
    libwinpthread-1.dll \
    libbrotlicommon.dll \
    libbrotlidec.dll \
    libbrotlienc.dll
  do
    if [[ -f "${runtime_dir}/${dll}" ]]; then
      cp "${runtime_dir}/${dll}" "${STAGE_DIR}/${dll}"
      continue
    fi
    echo "error: no se ha encontrado la dependencia MinGW requerida: ${dll}" >&2
    echo "       Buscada en: ${runtime_dir}" >&2
    exit 1
  done

  python3 - "${STAGE_DIR}" "${runtime_dir}" <<'PY'
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

stage = Path(sys.argv[1])
runtime = Path(sys.argv[2])
system = {
    "advapi32.dll", "authz.dll", "bcrypt.dll", "comctl32.dll", "comdlg32.dll",
    "crypt32.dll", "d3d11.dll", "d3d12.dll", "dnsapi.dll", "dwmapi.dll",
    "dwrite.dll", "dxgi.dll", "gdi32.dll", "imm32.dll", "iphlpapi.dll",
    "kernel32.dll", "mpr.dll", "msvcrt.dll", "ncrypt.dll", "netapi32.dll",
    "ntdll.dll", "ole32.dll", "oleaut32.dll", "rpcrt4.dll", "secur32.dll",
    "setupapi.dll", "shell32.dll", "shlwapi.dll", "user32.dll", "userenv.dll",
    "uxtheme.dll", "version.dll", "winhttp.dll", "winmm.dll", "winspool.drv",
    "ws2_32.dll", "wtsapi32.dll",
}

runtime_by_name = {p.name.lower(): p for p in runtime.glob("*.dll")}

def imported_dlls(path: Path) -> set[str]:
    try:
        out = subprocess.check_output(
            ["x86_64-w64-mingw32-objdump", "-p", str(path)],
            text=True,
            errors="ignore",
        )
    except (OSError, subprocess.CalledProcessError):
        return set()
    return {m.group(1) for m in re.finditer(r"DLL Name: (.+)", out)}

for _ in range(20):
    present = {p.name.lower() for p in stage.glob("*.dll")}
    to_copy = []
    for path in list(stage.glob("*.exe")) + list(stage.glob("*.dll")):
        for dll in imported_dlls(path):
            key = dll.lower()
            if key in present or key in system or key.startswith(("api-ms-", "ext-ms-")):
                continue
            source = runtime_by_name.get(key)
            if source is not None:
                to_copy.append(source)
    if not to_copy:
        break
    for source in sorted(set(to_copy)):
        shutil.copy2(source, stage / source.name)
else:
    raise SystemExit("error: no convergió la resolución de DLL MinGW/Qt")

present = {p.name.lower() for p in stage.glob("*.dll")}
missing = set()
for path in list(stage.glob("*.exe")) + list(stage.glob("*.dll")):
    for dll in imported_dlls(path):
        key = dll.lower()
        if key not in present and key not in system and not key.startswith(("api-ms-", "ext-ms-")):
            missing.add(dll)
if missing:
    raise SystemExit("error: faltan dependencias DLL no sistema: " + ", ".join(sorted(missing)))
PY
}

assert_qt_runtime_stage() {
  local relative plugin
  local missing=()
  for relative in \
    Qt6Core.dll \
    Qt6Gui.dll \
    Qt6Qml.dll \
    Qt6Quick.dll \
    platforms/qwindows.dll
  do
    [[ -f "${STAGE_DIR}/${relative}" ]] || missing+=("${relative}")
  done
  for plugin in \
    qmlplugin.dll \
    qtquick2plugin.dll \
    qtquickcontrols2plugin.dll \
    qquicklayoutsplugin.dll \
    qtquickdialogsplugin.dll \
    qmlsettingsplugin.dll
  do
    find "${STAGE_DIR}" -type f -iname "${plugin}" -print -quit | grep -q . ||
      missing+=("*/${plugin}")
  done
  for relative in libgcc_s_seh-1.dll libstdc++-6.dll libwinpthread-1.dll; do
    [[ -f "${STAGE_DIR}/${relative}" ]] || missing+=("${relative}")
  done
  if [[ "${#missing[@]}" -gt 0 ]]; then
    printf 'error: el runtime Qt/QML desplegado está incompleto. Faltan:\n' >&2
    printf '  - %s\n' "${missing[@]}" >&2
    exit 1
  fi
}

for arg in "$@"; do
  case "${arg}" in
    --nsis)
      DO_NSIS=1
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
ensure_nsis_if_requested

QMAKE_CMD="$(resolve_qmake)"
WINDEPLOYQT_CMD="$(resolve_windeployqt || true)"
MAKE_CMD="$(resolve_make)"
validate_qmake_for_windows "${QMAKE_CMD}"

case "${ARCH}" in
  amd64)
    export CC="${CC:-x86_64-w64-mingw32-gcc}"
    export CXX="${CXX:-x86_64-w64-mingw32-g++}"
    ;;
esac

rm -rf "${STAGE_DIR}" "${BUILD_DIR}"
mkdir -p "${STAGE_DIR}" "${BUILD_DIR}"

echo "Compilando backend Go para Windows (${ARCH})..."
GOOS=windows GOARCH="${ARCH}" CGO_ENABLED=0 grxfirma_go_build "${VERSION}" "-H=windowsgui" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma-gui.exe" \
  ./cmd/grxfirma-gui

GOOS=windows GOARCH="${ARCH}" CGO_ENABLED=0 grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma.exe" \
  ./cmd/grxfirma

echo "Compilando frontend Qt/QML para Windows..."
(
  cd "${BUILD_DIR}"
  "${QMAKE_CMD}" "${PROJECT_QML}"
  "${MAKE_CMD}"
)

QML_EXE="${BUILD_DIR}/grxfirma-gui-qml.exe"
if [[ ! -f "${QML_EXE}" && -f "${BUILD_DIR}/release/grxfirma-gui-qml.exe" ]]; then
  QML_EXE="${BUILD_DIR}/release/grxfirma-gui-qml.exe"
fi
if [[ ! -f "${QML_EXE}" ]]; then
  echo "error: no se ha generado grxfirma-gui-qml.exe en ${BUILD_DIR}" >&2
  exit 1
fi
cp "${QML_EXE}" "${STAGE_DIR}/grxfirma-gui-qml.exe"

cp -R "${ROOT_DIR}/cmd/gui-qml/qml" "${STAGE_DIR}/qml"
cp -R "${ROOT_DIR}/cmd/gui-qml/assets" "${STAGE_DIR}/assets"
cp "${ROOT_DIR}/packaging/windows/grxfirma-diputacion.ico" "${STAGE_DIR}/assets/grxfirma-diputacion.ico"
if [[ -d "${HELP_SOURCE_DIR}" ]]; then
  cp -R "${HELP_SOURCE_DIR}" "${STAGE_DIR}/help"
fi
cp "${ROOT_DIR}/packaging/windows/install-desktop-qml.ps1" "${STAGE_DIR}/install-desktop-qml.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-desktop-qml.ps1" "${STAGE_DIR}/uninstall-desktop-qml.ps1"
cp "${ROOT_DIR}/packaging/windows/install-path-safety.ps1" "${STAGE_DIR}/install-path-safety.ps1"
cp "${ROOT_DIR}/packaging/windows/invoke-uninstall-silent.ps1" "${STAGE_DIR}/invoke-uninstall-silent.ps1"
cp "${ROOT_DIR}/packaging/windows/README_DESKTOP_QML_WINDOWS.md" "${STAGE_DIR}/README_DESKTOP_QML_WINDOWS.md"
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"

deploy_qt_runtime "${QMAKE_CMD}" "${WINDEPLOYQT_CMD}"
deploy_mingw_runtime "${QMAKE_CMD}"
assert_qt_runtime_stage

mkdir -p "${OUT_DIR}"
rm -f "${ZIP_PATH}"
grxfirma_reproducible_zip "${STAGE_DIR}" "${ZIP_PATH}"
validate_zip_artifact "${ZIP_PATH}"
echo "ZIP generado en: ${ZIP_PATH}"

if [[ "${DO_NSIS}" == "1" ]]; then
  makensis \
    -DVERSION="${VERSION}" \
    -DARCH="${ARCH}" \
    -DSTAGE_DIR="${STAGE_DIR}" \
    -DOUT_FILE="${NSIS_OUT}" \
    "${ROOT_DIR}/packaging/windows/grxfirma-desktop-qml.nsi"
  validate_nsis_output "${NSIS_OUT}"
  grxfirma_normalize_output_mtime "${NSIS_OUT}"
  echo "Instalador NSIS generado en: ${NSIS_OUT}"
fi

write_sha256sums
verify_sha256sums
write_artifacts_report
