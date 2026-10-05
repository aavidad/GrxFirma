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
OUT_DIR="${ROOT_DIR}/release/windows-nativehost"
STAGE_DIR="${OUT_DIR}/GrxFirma-${VERSION}-nativehost-windows-${ARCH}"
ZIP_PATH="${OUT_DIR}/GrxFirma-${VERSION}-nativehost-windows-${ARCH}.zip"
DEFAULT_EXTENSION_KEY="${HOME}/.local/share/grxfirma/build-keys/chromium-extension.pem"

usage() {
  cat <<'EOF'
Uso:
  packaging/windows/build-nativehost.sh

Variables relevantes:
  GOARCH=amd64
  GRXFIRMA_BUILD_CHROMIUM_CRX=1 genera el CRX local no reproducible
  GRXFIRMA_CHROMIUM_EXTENSION_KEY=/ruta/a/chromium-extension.pem
EOF
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

validate_zip_artifact() {
  local zip_path="$1"
  local stage_name
  stage_name="$(basename "$STAGE_DIR")"
  python3 - "$zip_path" "$stage_name" <<'PY'
import sys
import zipfile

zip_path = sys.argv[1]
stage_name = sys.argv[2]
required = {
    f"{stage_name}/grxfirma-nativehost.exe",
    f"{stage_name}/README_NATIVEHOST_WINDOWS.md",
    f"{stage_name}/install-nativehost.ps1",
    f"{stage_name}/uninstall-nativehost.ps1",
    f"{stage_name}/install-path-safety.ps1",
    f"{stage_name}/VERSION.txt",
    f"{stage_name}/extensions/grxfirma-extension-chromium.zip",
    f"{stage_name}/extensions/grxfirma-extension-firefox.xpi",
    f"{stage_name}/extensions/grxfirma-extension-firefox.metadata.json",
}

with zipfile.ZipFile(zip_path) as zf:
    names = set(zf.namelist())

missing = sorted(required - names)
if missing:
    raise SystemExit("zip artifact incomplete:\n  - " + "\n  - ".join(missing))
PY
}

write_sha256sums() {
  local checksum_path="${OUT_DIR}/SHA256SUMS.txt"
  (
    cd "${OUT_DIR}"
    find . -maxdepth 1 -type f -name "GrxFirma-${VERSION}-nativehost-windows-${ARCH}.zip" -print0 | sort -z | xargs -0 sha256sum > "${checksum_path}"
  )
  if [[ ! -f "${checksum_path}" || ! -s "${checksum_path}" ]]; then
    echo "error: no se pudo generar SHA256SUMS.txt para NativeHost Windows." >&2
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
  python3 - "${OUT_DIR}" "$(basename "${ZIP_PATH}")" <<'PY'
import hashlib
import sys
from pathlib import Path

out_dir = Path(sys.argv[1])
zip_name = sys.argv[2]
lines = [
    "# Windows NativeHost Artifacts",
    "",
    "| File | Size (bytes) | SHA-256 |",
    "| --- | ---: | --- |",
]
path = out_dir / zip_name
if path.is_file():
    lines.append(f"| `{zip_name}` | {path.stat().st_size} | `{hashlib.sha256(path.read_bytes()).hexdigest()}` |")
(out_dir / "ARTIFACTS.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
  if [[ ! -f "${report_path}" || ! -s "${report_path}" ]]; then
    echo "error: no se pudo generar ARTIFACTS.md para NativeHost Windows." >&2
    exit 1
  fi
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

rm -rf "${STAGE_DIR}"
mkdir -p "$STAGE_DIR"

echo "Compilando NativeHost de Windows (${ARCH})..."
GOOS=windows GOARCH="$ARCH" grxfirma_go_build "${VERSION}" "" \
  -tags production \
  -o "${STAGE_DIR}/grxfirma-nativehost.exe" \
  ./cmd/nativehost

cp "${ROOT_DIR}/packaging/windows/README_NATIVEHOST_WINDOWS.md" "${STAGE_DIR}/README_NATIVEHOST_WINDOWS.md"
cp "${ROOT_DIR}/packaging/windows/install-nativehost.ps1" "${STAGE_DIR}/install-nativehost.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-nativehost.ps1" "${STAGE_DIR}/uninstall-nativehost.ps1"
cp "${ROOT_DIR}/packaging/windows/install-path-safety.ps1" "${STAGE_DIR}/install-path-safety.ps1"
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
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"

mkdir -p "$OUT_DIR"
rm -f "$ZIP_PATH"
grxfirma_reproducible_zip "${STAGE_DIR}" "${ZIP_PATH}"
validate_zip_artifact "$ZIP_PATH"
write_sha256sums
verify_sha256sums
write_artifacts_report
echo "ZIP generado en: $ZIP_PATH"
