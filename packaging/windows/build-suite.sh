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
OUT_DIR="${ROOT_DIR}/release/windows-suite"
STAGE_DIR="${OUT_DIR}/GrxFirma-${VERSION}-windows-${ARCH}"
ZIP_PATH="${OUT_DIR}/GrxFirma-${VERSION}-windows-${ARCH}.zip"
NSIS_OUT="${OUT_DIR}/GrxFirma-${VERSION}-windows-${ARCH}-setup.exe"
DESKTOP_QML_STAGE="${ROOT_DIR}/release/windows-desktop-qml/GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}"
DESKTOP_WINUI_STAGE="${ROOT_DIR}/release/windows-desktop-winui/GrxFirma-${VERSION}-desktop-winui-windows-amd64"
HELP_SOURCE_DIR="${ROOT_DIR}/cmd/gui-qml/help"
DO_NSIS=0
REQUIRE_QT_STAGE=0
REQUIRE_WINUI_STAGE=0
QT_PAYLOAD_INTEGRATED=0
WINUI_PAYLOAD_INTEGRATED=0
DEFAULT_EXTENSION_KEY="${HOME}/.local/share/grxfirma/build-keys/chromium-extension.pem"

usage() {
  cat <<'EOF'
Uso:
  packaging/windows/build-suite.sh [--with-winui] [--with-qt] [--nsis]

Variables relevantes:
  GOARCH=amd64
  GRXFIRMA_BUILD_CHROMIUM_CRX=1 genera el CRX local no reproducible
  GRXFIRMA_CHROMIUM_EXTENSION_KEY=/ruta/a/chromium-extension.pem

Firma Authenticode (opcional; sin estas variables el build no firma):
  WINDOWS_SIGNCODE_PFX=/ruta/al/certificado.pfx
  WINDOWS_SIGNCODE_PASS_FILE=/ruta/al/fichero-con-password   (0600; evita argv)
  WINDOWS_SIGNCODE_TS_URL=http://timestamp.digicert.com      (RFC 3161; por defecto DigiCert)
EOF
}

# --- Firma Authenticode opcional (osslsigncode) -----------------------------
# Mismo patrón que packaging/macos (codesign_if_enabled): si no hay certificado
# configurado, el build sale sin firmar y Windows SmartScreen avisará al
# usuario. Con WINDOWS_SIGNCODE_PFX definido, todos los .exe y el instalador
# NSIS se firman y sellan con RFC 3161. La password se lee de fichero
# (-readpass) para que nunca aparezca en argv (misma política que el resto de secretos).
WINDOWS_SIGNCODE_TS_URL="${WINDOWS_SIGNCODE_TS_URL:-http://timestamp.digicert.com}"

authenticode_enabled() {
  [[ -n "${WINDOWS_SIGNCODE_PFX:-}" ]]
}

ensure_authenticode_ready_if_enabled() {
  authenticode_enabled || return 0
  if ! command -v osslsigncode >/dev/null 2>&1; then
    echo "error: WINDOWS_SIGNCODE_PFX está definido pero osslsigncode no está en PATH." >&2
    echo "       En Debian/Ubuntu: apt-get install osslsigncode" >&2
    exit 1
  fi
  if [[ ! -f "${WINDOWS_SIGNCODE_PFX}" ]]; then
    echo "error: no existe el certificado Authenticode: ${WINDOWS_SIGNCODE_PFX}" >&2
    exit 1
  fi
  if [[ -z "${WINDOWS_SIGNCODE_PASS_FILE:-}" || ! -f "${WINDOWS_SIGNCODE_PASS_FILE}" ]]; then
    echo "error: WINDOWS_SIGNCODE_PASS_FILE debe apuntar a un fichero legible con la password del PFX." >&2
    exit 1
  fi
}

authenticode_sign_if_enabled() {
  local target="$1"
  authenticode_enabled || return 0
  [[ -f "$target" ]] || return 0
  local tmp="${target}.authenticode-tmp"
  osslsigncode sign \
    -pkcs12 "${WINDOWS_SIGNCODE_PFX}" \
    -readpass "${WINDOWS_SIGNCODE_PASS_FILE}" \
    -n "GrxFirma" \
    -i "https://github.com/aavidad/GrxFirma" \
    -h sha256 \
    -ts "${WINDOWS_SIGNCODE_TS_URL}" \
    -in "$target" \
    -out "$tmp" >/dev/null
  mv "$tmp" "$target"
  osslsigncode verify -in "$target" >/dev/null 2>&1 || {
    echo "error: la verificación Authenticode de $(basename "$target") falló tras firmar." >&2
    exit 1
  }
  echo "Firmado (Authenticode): $(basename "$target")"
}

authenticode_sign_stage_binaries() {
  authenticode_enabled || return 0
  local binary
  while IFS= read -r -d '' binary; do
    authenticode_sign_if_enabled "$binary"
  done < <(
    find "${STAGE_DIR}" -type f \
      \( -iname '*.exe' -o -iname '*.dll' \) \
      ! -iname 'vc_redist.*.exe' \
      ! -path "${STAGE_DIR}/desktop-qt/grxfirma-gui.exe" \
      ! -path "${STAGE_DIR}/desktop-winui/app/grxfirma-gui.exe" \
      -print0 |
      sort -z
  )
}

synchronize_shared_backend_copies() {
  local canonical="${STAGE_DIR}/grxfirma-gui.exe"
  if [[ "$QT_PAYLOAD_INTEGRATED" == "1" ]]; then
    cp "${canonical}" "${STAGE_DIR}/desktop-qt/grxfirma-gui.exe"
  fi
  if [[ "$WINUI_PAYLOAD_INTEGRATED" == "1" ]]; then
    local winui_stage="${STAGE_DIR}/desktop-winui"
    cp "${canonical}" "${winui_stage}/app/grxfirma-gui.exe"
    python3 - "${winui_stage}" <<'PY'
import hashlib
import pathlib
import re
import sys

stage = pathlib.Path(sys.argv[1])
manifest = stage / "PUBLISH-MANIFEST.sha256"
declared = set()
for line in manifest.read_text(encoding="utf-8-sig").splitlines():
    match = re.fullmatch(r"([0-9a-fA-F]{64}) \*(.+)", line)
    if match is None:
        raise SystemExit("manifiesto WinUI no válido")
    relative = match.group(2).replace("\\", "/")
    path = pathlib.PurePosixPath(relative)
    if path.is_absolute() or ".." in path.parts:
        raise SystemExit(f"ruta no permitida en manifiesto WinUI: {relative}")
    declared.add(path.as_posix())
actual = {
    path.relative_to(stage).as_posix()
    for path in stage.rglob("*")
    if path.is_file() and path != manifest
}
if "app/grxfirma-gui.exe" not in declared:
    raise SystemExit("el manifiesto WinUI no inventaría el backend compartido")
if actual != declared:
    raise SystemExit(
        "el payload WinUI cambió su inventario durante la firma "
        f"(ausentes={sorted(declared - actual)}, extra={sorted(actual - declared)})"
    )
updated = []
for relative in sorted(actual):
    digest = hashlib.sha256((stage / relative).read_bytes()).hexdigest()
    updated.append(f"{digest} *{relative}")
with manifest.open("w", encoding="utf-8", newline="\n") as stream:
    stream.write("\n".join(updated) + "\n")
PY
    ensure_winui_stage_complete "${winui_stage}"
  fi
}

ensure_supported_arch() {
  case "${ARCH}" in
    amd64)
      return 0
      ;;
    *)
      echo "error: arquitectura Windows no soportada por este script: ${ARCH}" >&2
      echo "       La ruta shell actual está cerrada para GOARCH=amd64." >&2
      exit 1
      ;;
  esac
}

ensure_nsis_if_requested() {
  if [[ "$DO_NSIS" != "1" ]]; then
    return 0
  fi
  if ! command -v makensis >/dev/null 2>&1; then
    echo "error: --nsis requiere makensis en PATH." >&2
    exit 1
  fi
}

reset_stage() {
  case "${STAGE_DIR}" in
    "${OUT_DIR}"/GrxFirma-*-windows-*)
      ;;
    *)
      echo "error: ruta de staging inesperada; no se eliminara: ${STAGE_DIR}" >&2
      exit 1
      ;;
  esac
  rm -rf -- "${STAGE_DIR}"
  mkdir -p -- "${STAGE_DIR}"
}

ensure_qt_stage_complete() {
  local missing=()
  local redist_candidates=()
  local required_files=(
    "grxfirma-gui-qml.exe"
    "grxfirma-gui.exe"
    "grxfirma.exe"
    "install-desktop-qml.ps1"
    "README_DESKTOP_QML_WINDOWS.md"
  )
  local required_dirs=(
    "qml"
    "assets"
  )
  if [[ -d "${HELP_SOURCE_DIR}" ]]; then
    required_dirs+=("help")
  fi
  for file in "${required_files[@]}"; do
    [[ -f "${DESKTOP_QML_STAGE}/${file}" ]] || missing+=("${DESKTOP_QML_STAGE}/${file}")
  done
  for dir in "${required_dirs[@]}"; do
    [[ -d "${DESKTOP_QML_STAGE}/${dir}" ]] || missing+=("${DESKTOP_QML_STAGE}/${dir}")
  done
  for file in Qt6Core.dll Qt6Gui.dll Qt6Qml.dll Qt6Quick.dll platforms/qwindows.dll; do
    [[ -f "${DESKTOP_QML_STAGE}/${file}" ]] || missing+=("${DESKTOP_QML_STAGE}/${file}")
  done
  for file in qmlplugin.dll qtquick2plugin.dll qtquickcontrols2plugin.dll qquicklayoutsplugin.dll qtquickdialogsplugin.dll qmlsettingsplugin.dll; do
    find "${DESKTOP_QML_STAGE}" -type f -iname "${file}" -print -quit | grep -q . ||
      missing+=("${DESKTOP_QML_STAGE}/**/${file}")
  done
  mapfile -t redist_candidates < <(
    find "${DESKTOP_QML_STAGE}" -maxdepth 1 -type f -iname 'vc_redist.*.exe' -printf '%f\n'
  )
  if [[ "${#redist_candidates[@]}" -gt 0 ]] &&
      { [[ "${#redist_candidates[@]}" -ne 1 ]] ||
        [[ "${redist_candidates[0],,}" != "vc_redist.x64.exe" ]]; }; then
    missing+=("${DESKTOP_QML_STAGE}/unico-vc_redist.x64.exe")
  fi
  if [[ ! -f "${DESKTOP_QML_STAGE}/vc_redist.x64.exe" ]] &&
      { [[ ! -f "${DESKTOP_QML_STAGE}/libgcc_s_seh-1.dll" ]] ||
        [[ ! -f "${DESKTOP_QML_STAGE}/libstdc++-6.dll" ]] ||
        [[ ! -f "${DESKTOP_QML_STAGE}/libwinpthread-1.dll" ]]; }; then
    missing+=("${DESKTOP_QML_STAGE}/(vc_redist.x64.exe o runtime MinGW)")
  fi
  if [[ "${#missing[@]}" -gt 0 ]]; then
    printf 'la stage Qt/QML de Windows está incompleta. Faltan:\n' >&2
    printf '  - %s\n' "${missing[@]}" >&2
    return 1
  fi
}

ensure_winui_stage_complete() {
  local stage_dir="${1:-${DESKTOP_WINUI_STAGE}}"
  local required=(
    "README_DESKTOP_WINUI_WINDOWS.md"
    "VERSION.txt"
    "PUBLISH-MANIFEST.sha256"
    "app/grxfirma-winui.exe"
    "app/grxfirma-gui.exe"
    "app/coreclr.dll"
    "app/hostfxr.dll"
    "app/Microsoft.UI.Xaml.dll"
    "app/Microsoft.WindowsAppRuntime.dll"
  )
  local missing=()
  local file
  for file in "${required[@]}"; do
    [[ -f "${stage_dir}/${file}" ]] ||
      missing+=("${stage_dir}/${file}")
  done
  if [[ "${#missing[@]}" -gt 0 ]]; then
    printf 'la stage WinUI de Windows está incompleta. Faltan:\n' >&2
    printf '  - %s\n' "${missing[@]}" >&2
    return 1
  fi

  python3 - "${stage_dir}" <<'PY'
import hashlib
import pathlib
import re
import sys

stage = pathlib.Path(sys.argv[1]).resolve()
manifest = stage / "PUBLISH-MANIFEST.sha256"
entries = {}
for raw_line in manifest.read_text(encoding="utf-8-sig").splitlines():
    match = re.fullmatch(r"([0-9a-fA-F]{64}) \*(.+)", raw_line)
    if match is None:
        raise SystemExit("manifiesto WinUI no válido")
    relative = pathlib.PurePosixPath(match.group(2).replace("\\", "/"))
    if relative.is_absolute() or ".." in relative.parts:
        raise SystemExit(f"ruta no permitida en manifiesto WinUI: {relative}")
    entries[relative.as_posix()] = match.group(1).lower()

actual = {}
for path in stage.rglob("*"):
    if not path.is_file() or path == manifest:
        continue
    relative = path.relative_to(stage).as_posix()
    actual[relative] = hashlib.sha256(path.read_bytes()).hexdigest()
if actual != entries:
    missing = sorted(set(entries) - set(actual))
    extra = sorted(set(actual) - set(entries))
    changed = sorted(name for name in set(actual) & set(entries)
                     if actual[name] != entries[name])
    raise SystemExit(
        "manifiesto WinUI no coincide "
        f"(ausentes={missing}, extra={extra}, alterados={changed})"
    )
PY
}

validate_zip_artifact() {
  local zip_path="$1"
  local stage_name
  stage_name="$(basename "$STAGE_DIR")"
  local qt_help_integrated=0
  if [[ "${QT_PAYLOAD_INTEGRATED}" == "1" && -d "${DESKTOP_QML_STAGE}/help" ]]; then
    qt_help_integrated=1
  fi
  python3 - "$zip_path" "$stage_name" "$QT_PAYLOAD_INTEGRATED" "$qt_help_integrated" "$WINUI_PAYLOAD_INTEGRATED" <<'PY'
import sys
import zipfile

zip_path = sys.argv[1]
stage_name = sys.argv[2]
qt_integrated = sys.argv[3] == "1"
qt_help_integrated = sys.argv[4] == "1"
winui_integrated = sys.argv[5] == "1"

required = {
    f"{stage_name}/grxfirma.exe",
    f"{stage_name}/grxfirma-gui.exe",
    f"{stage_name}/grxfirma-nativehost.exe",
    f"{stage_name}/grxfirma-afirmauri.exe",
    f"{stage_name}/README_WINDOWS_SUITE.md",
    f"{stage_name}/policies/GrxFirma.admx",
    f"{stage_name}/policies/es-ES/GrxFirma.adml",
    f"{stage_name}/policies/en-US/GrxFirma.adml",
    f"{stage_name}/install-suite.ps1",
    f"{stage_name}/install-nativehost.ps1",
    f"{stage_name}/install-afirmauri.ps1",
    f"{stage_name}/afirmauri-registration.ps1",
    f"{stage_name}/uninstall-suite.ps1",
    f"{stage_name}/uninstall-nativehost.ps1",
    f"{stage_name}/uninstall-afirmauri.ps1",
    f"{stage_name}/install-desktop-qml.ps1",
    f"{stage_name}/uninstall-desktop-qml.ps1",
    f"{stage_name}/install-desktop-winui.ps1",
    f"{stage_name}/uninstall-desktop-winui.ps1",
    f"{stage_name}/remove-unselected-desktop.ps1",
    f"{stage_name}/install-path-safety.ps1",
    f"{stage_name}/invoke-uninstall-silent.ps1",
    f"{stage_name}/VERSION.txt",
    f"{stage_name}/extensions/grxfirma-extension-chromium.zip",
    f"{stage_name}/extensions/grxfirma-extension-firefox.xpi",
    f"{stage_name}/extensions/grxfirma-extension-firefox.metadata.json",
}
if qt_integrated:
    required |= {
        f"{stage_name}/desktop-qt/grxfirma-gui-qml.exe",
        f"{stage_name}/desktop-qt/grxfirma-gui.exe",
        f"{stage_name}/desktop-qt/install-desktop-qml.ps1",
        f"{stage_name}/desktop-qt/README_DESKTOP_QML_WINDOWS.md",
        f"{stage_name}/desktop-qt/qml/",
        f"{stage_name}/desktop-qt/assets/",
        f"{stage_name}/desktop-qt/qt-qml/",
        f"{stage_name}/desktop-qt/platforms/",
    }
if qt_help_integrated:
    required.add(f"{stage_name}/desktop-qt/help/")
if winui_integrated:
    required |= {
        f"{stage_name}/desktop-winui/PUBLISH-MANIFEST.sha256",
        f"{stage_name}/desktop-winui/README_DESKTOP_WINUI_WINDOWS.md",
        f"{stage_name}/desktop-winui/VERSION.txt",
        f"{stage_name}/desktop-winui/app/grxfirma-winui.exe",
        f"{stage_name}/desktop-winui/app/grxfirma-gui.exe",
        f"{stage_name}/desktop-winui/app/coreclr.dll",
        f"{stage_name}/desktop-winui/app/hostfxr.dll",
        f"{stage_name}/desktop-winui/app/Microsoft.UI.Xaml.dll",
        f"{stage_name}/desktop-winui/app/Microsoft.WindowsAppRuntime.dll",
    }

with zipfile.ZipFile(zip_path) as zf:
    names = set(zf.namelist())

missing = []
for entry in required:
    if entry.endswith("/"):
        if not any(name.startswith(entry) for name in names):
            missing.append(entry)
    elif entry not in names:
        missing.append(entry)

if missing:
    raise SystemExit("zip artifact incomplete:\n  - " + "\n  - ".join(sorted(missing)))
PY
}

validate_nsis_output() {
  local path="$1"
  if [[ ! -f "$path" || ! -s "$path" ]]; then
    echo "error: el instalador NSIS no se ha generado correctamente: $path" >&2
    exit 1
  fi
}

write_sha256sums() {
  local checksum_path="${OUT_DIR}/SHA256SUMS.txt"
  (
    cd "${OUT_DIR}"
    find . -maxdepth 1 -type f \( \
      -name "GrxFirma-${VERSION}-windows-${ARCH}.zip" -o \
      -name "GrxFirma-${VERSION}-windows-${ARCH}-setup.exe" \
    \) -print0 | sort -z | xargs -0 sha256sum > "${checksum_path}"
  )
  if [[ ! -f "${checksum_path}" || ! -s "${checksum_path}" ]]; then
    echo "error: no se pudo generar SHA256SUMS.txt para la suite Windows." >&2
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
    "# Windows Suite Artifacts",
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
    echo "error: no se pudo generar ARTIFACTS.md para la suite Windows." >&2
    exit 1
  fi
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
}

case "${ARCH}" in
  amd64)
    export CC="${CC:-x86_64-w64-mingw32-gcc}"
    export CXX="${CXX:-x86_64-w64-mingw32-g++}"
    ;;
esac

for arg in "$@"; do
  case "$arg" in
    --nsis)
      DO_NSIS=1
      ;;
    --with-qt)
      REQUIRE_QT_STAGE=1
      ;;
    --with-winui)
      REQUIRE_WINUI_STAGE=1
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
grxfirma_assert_windows_fyne_toolchain "${ARCH}"
ensure_nsis_if_requested
ensure_authenticode_ready_if_enabled
if [[ "$REQUIRE_QT_STAGE" == "1" ]] &&
    { [[ ! -d "$DESKTOP_QML_STAGE" ]] || ! ensure_qt_stage_complete; }; then
  echo "error: no existe una stage Qt/QML completa requerida: ${DESKTOP_QML_STAGE}" >&2
  exit 1
fi
if [[ "$REQUIRE_WINUI_STAGE" == "1" ]] &&
    { [[ ! -d "$DESKTOP_WINUI_STAGE" ]] || ! ensure_winui_stage_complete; }; then
  echo "error: no existe una stage WinUI completa requerida: ${DESKTOP_WINUI_STAGE}" >&2
  exit 1
fi
rm -f "${NSIS_OUT}" "${ZIP_PATH}"
reset_stage

echo "Compilando suite de Windows (${ARCH})..."
GOOS=windows GOARCH="$ARCH" CGO_ENABLED=0 grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma.exe" \
  ./cmd/grxfirma

GOOS=windows GOARCH="$ARCH" CGO_ENABLED=0 grxfirma_go_build "${VERSION}" "-H=windowsgui" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma-gui.exe" \
  ./cmd/grxfirma-gui

GOOS=windows GOARCH="$ARCH" CGO_ENABLED=0 grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma-nativehost.exe" \
  ./cmd/nativehost

GOOS=windows GOARCH="$ARCH" CGO_ENABLED=1 grxfirma_go_build "${VERSION}" "-H=windowsgui" \
  -tags production,fyne_gui \
  -o "${STAGE_DIR}/grxfirma-afirmauri.exe" \
  ./cmd/grxfirmauri
grxfirma_assert_windows_fyne_artifact \
  "${STAGE_DIR}/grxfirma-afirmauri.exe" \
  "${ARCH}"

cp "${ROOT_DIR}/packaging/windows/README_WINDOWS_SUITE.md" "${STAGE_DIR}/README_WINDOWS_SUITE.md"
# Plantillas de directiva de grupo para los administradores.
for idioma_admx in es-ES en-US; do
  mkdir -p "${STAGE_DIR}/policies/${idioma_admx}"
  cp "${ROOT_DIR}/packaging/windows/admx/${idioma_admx}/GrxFirma.adml" "${STAGE_DIR}/policies/${idioma_admx}/GrxFirma.adml"
done
cp "${ROOT_DIR}/packaging/windows/admx/GrxFirma.admx" "${STAGE_DIR}/policies/GrxFirma.admx"
cp "${ROOT_DIR}/packaging/windows/grxfirma.ico" "${STAGE_DIR}/grxfirma.ico"
cp "${ROOT_DIR}/packaging/windows/install-suite.ps1" "${STAGE_DIR}/install-suite.ps1"
cp "${ROOT_DIR}/packaging/windows/install-nativehost.ps1" "${STAGE_DIR}/install-nativehost.ps1"
cp "${ROOT_DIR}/packaging/windows/install-afirmauri.ps1" "${STAGE_DIR}/install-afirmauri.ps1"
cp "${ROOT_DIR}/packaging/windows/afirmauri-registration.ps1" "${STAGE_DIR}/afirmauri-registration.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-suite.ps1" "${STAGE_DIR}/uninstall-suite.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-nativehost.ps1" "${STAGE_DIR}/uninstall-nativehost.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-afirmauri.ps1" "${STAGE_DIR}/uninstall-afirmauri.ps1"
cp "${ROOT_DIR}/packaging/windows/install-desktop-qml.ps1" "${STAGE_DIR}/install-desktop-qml.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-desktop-qml.ps1" "${STAGE_DIR}/uninstall-desktop-qml.ps1"
cp "${ROOT_DIR}/packaging/windows/install-desktop-winui.ps1" "${STAGE_DIR}/install-desktop-winui.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-desktop-winui.ps1" "${STAGE_DIR}/uninstall-desktop-winui.ps1"
cp "${ROOT_DIR}/packaging/windows/remove-unselected-desktop.ps1" "${STAGE_DIR}/remove-unselected-desktop.ps1"
cp "${ROOT_DIR}/packaging/windows/install-path-safety.ps1" "${STAGE_DIR}/install-path-safety.ps1"
cp "${ROOT_DIR}/packaging/windows/invoke-uninstall-silent.ps1" "${STAGE_DIR}/invoke-uninstall-silent.ps1"
mkdir -p "${STAGE_DIR}/extensions"
python3 "${ROOT_DIR}/packaging/browser-extensions/build.py" --output-dir "${STAGE_DIR}/extensions"
python3 "${ROOT_DIR}/packaging/browser-extensions/verify_package.py" "${STAGE_DIR}/extensions"
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"

if [[ -d "$DESKTOP_QML_STAGE" ]] && ensure_qt_stage_complete; then
  echo "Integrando frontend desktop Qt/QML en un payload aislado..."
  cp -R "${DESKTOP_QML_STAGE}" "${STAGE_DIR}/desktop-qt"
  QT_PAYLOAD_INTEGRATED=1
elif [[ "$REQUIRE_QT_STAGE" == "1" ]]; then
  echo "error: no existe una stage Qt/QML completa requerida para Windows:" >&2
  echo "       ${DESKTOP_QML_STAGE}" >&2
  echo "       Construyela antes y vuelve a ejecutar con --with-qt." >&2
  exit 1
else
  echo "Aviso: no se encontró una stage Qt/QML completa; la suite saldrá sin frontend Qt." >&2
fi

if [[ -d "$DESKTOP_WINUI_STAGE" ]] && ensure_winui_stage_complete; then
  echo "Integrando frontend desktop WinUI autocontenido..."
  cp -R "${DESKTOP_WINUI_STAGE}" "${STAGE_DIR}/desktop-winui"
  WINUI_PAYLOAD_INTEGRATED=1
elif [[ "$REQUIRE_WINUI_STAGE" == "1" ]]; then
  echo "error: no existe una stage WinUI completa requerida para Windows:" >&2
  echo "       ${DESKTOP_WINUI_STAGE}" >&2
  echo "       Construyela antes y vuelve a ejecutar con --with-winui." >&2
  exit 1
else
  echo "Aviso: no se encontró una stage WinUI completa; la suite saldrá sin frontend nativo." >&2
fi

authenticode_sign_stage_binaries
synchronize_shared_backend_copies

mkdir -p "$OUT_DIR"
rm -f "$ZIP_PATH"
grxfirma_reproducible_zip "${STAGE_DIR}" "${ZIP_PATH}"
validate_zip_artifact "$ZIP_PATH"
echo "ZIP generado en: $ZIP_PATH"

if [[ "$DO_NSIS" == "1" ]]; then
  nsis_args=(
    "-DVERSION=${VERSION}"
    "-DARCH=${ARCH}"
    "-DSTAGE_DIR=${STAGE_DIR}"
    "-DOUT_FILE=${NSIS_OUT}"
  )
  if [[ "$WINUI_PAYLOAD_INTEGRATED" == "1" ]]; then
    nsis_args+=("-DHAS_WINUI=1")
  fi
  if [[ "$QT_PAYLOAD_INTEGRATED" == "1" ]]; then
    nsis_args+=("-DHAS_QT=1")
  fi
  nsis_args+=("${ROOT_DIR}/packaging/windows/grxfirma-suite.nsi")
  makensis "${nsis_args[@]}"
  validate_nsis_output "${NSIS_OUT}"
  authenticode_sign_if_enabled "${NSIS_OUT}"
  grxfirma_normalize_output_mtime "${NSIS_OUT}"
  echo "Instalador NSIS generado en: $NSIS_OUT"
fi

write_sha256sums
verify_sha256sums
write_artifacts_report
