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
OUT_DIR="${ROOT_DIR}/release/windows-afirmauri"
STAGE_DIR="${OUT_DIR}/GrxFirma-${VERSION}-afirmauri-windows-${ARCH}"
ZIP_PATH="${OUT_DIR}/GrxFirma-${VERSION}-afirmauri-windows-${ARCH}.zip"
NSIS_OUT="${OUT_DIR}/GrxFirma-${VERSION}-afirmauri-windows-${ARCH}-setup.exe"
DO_NSIS=0

usage() {
  cat <<'EOF'
Uso:
  packaging/windows/build-afirmauri.sh [--nsis]

Variables relevantes:
  GOARCH=amd64
EOF
}

ensure_supported_arch() {
  case "${ARCH}" in
    amd64)
      export CC="${CC:-x86_64-w64-mingw32-gcc}"
      export CXX="${CXX:-x86_64-w64-mingw32-g++}"
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
    f"{stage_name}/grxfirma-afirmauri.exe",
    f"{stage_name}/grxfirma-grx.ico",
    f"{stage_name}/README_AFIRMAURI_WINDOWS.md",
    f"{stage_name}/install-afirmauri.ps1",
    f"{stage_name}/uninstall-afirmauri.ps1",
    f"{stage_name}/afirmauri-registration.ps1",
    f"{stage_name}/install-path-safety.ps1",
    f"{stage_name}/invoke-uninstall-silent.ps1",
    f"{stage_name}/VERSION.txt",
}

with zipfile.ZipFile(zip_path) as zf:
    names = set(zf.namelist())

missing = sorted(required - names)
if missing:
    raise SystemExit("zip artifact incomplete:\n  - " + "\n  - ".join(missing))
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
      -name "GrxFirma-${VERSION}-afirmauri-windows-${ARCH}.zip" -o \
      -name "GrxFirma-${VERSION}-afirmauri-windows-${ARCH}-setup.exe" \
    \) -print0 | sort -z | xargs -0 sha256sum > "${checksum_path}"
  )
  if [[ ! -f "${checksum_path}" || ! -s "${checksum_path}" ]]; then
    echo "error: no se pudo generar SHA256SUMS.txt para AfirmaURI Windows." >&2
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
    "# Windows AfirmaURI Artifacts",
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
    echo "error: no se pudo generar ARTIFACTS.md para AfirmaURI Windows." >&2
    exit 1
  fi
}

for arg in "$@"; do
  case "$arg" in
    --nsis)
      DO_NSIS=1
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

rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR"
rm -f "${STAGE_DIR}/grxfirma-afirmauri.exe" \
      "${STAGE_DIR}/README_AFIRMAURI_WINDOWS.md" \
      "${STAGE_DIR}/install-afirmauri.ps1" \
      "${STAGE_DIR}/uninstall-afirmauri.ps1" \
      "${STAGE_DIR}/afirmauri-registration.ps1" \
      "${STAGE_DIR}/VERSION.txt"

echo "Compilando handler afirma:// de Windows (${ARCH}, GUI interactiva)..."
GOOS=windows GOARCH="$ARCH" CGO_ENABLED=1 grxfirma_go_build "${VERSION}" "-H=windowsgui" \
  -tags production,fyne_gui \
  -o "${STAGE_DIR}/grxfirma-afirmauri.exe" \
  ./cmd/grxfirmauri
grxfirma_assert_windows_fyne_artifact \
  "${STAGE_DIR}/grxfirma-afirmauri.exe" \
  "${ARCH}"

cp "${ROOT_DIR}/packaging/windows/README_AFIRMAURI_WINDOWS.md" "${STAGE_DIR}/README_AFIRMAURI_WINDOWS.md"
cp "${ROOT_DIR}/packaging/windows/grxfirma-grx.ico" "${STAGE_DIR}/grxfirma-grx.ico"
cp "${ROOT_DIR}/packaging/windows/install-afirmauri.ps1" "${STAGE_DIR}/install-afirmauri.ps1"
cp "${ROOT_DIR}/packaging/windows/uninstall-afirmauri.ps1" "${STAGE_DIR}/uninstall-afirmauri.ps1"
cp "${ROOT_DIR}/packaging/windows/afirmauri-registration.ps1" "${STAGE_DIR}/afirmauri-registration.ps1"
cp "${ROOT_DIR}/packaging/windows/install-path-safety.ps1" "${STAGE_DIR}/install-path-safety.ps1"
cp "${ROOT_DIR}/packaging/windows/invoke-uninstall-silent.ps1" "${STAGE_DIR}/invoke-uninstall-silent.ps1"
printf '%s\n' "${VERSION}" > "${STAGE_DIR}/VERSION.txt"

mkdir -p "$OUT_DIR"
rm -f "$ZIP_PATH"
grxfirma_reproducible_zip "${STAGE_DIR}" "${ZIP_PATH}"
validate_zip_artifact "$ZIP_PATH"
echo "ZIP generado en: $ZIP_PATH"

if [[ "$DO_NSIS" == "1" ]]; then
  makensis \
    -DVERSION="${VERSION}" \
    -DARCH="${ARCH}" \
    -DSTAGE_DIR="${STAGE_DIR}" \
    -DOUT_FILE="${NSIS_OUT}" \
    "${ROOT_DIR}/packaging/windows/grxfirma-afirmauri.nsi"
  validate_nsis_output "${NSIS_OUT}"
  grxfirma_normalize_output_mtime "${NSIS_OUT}"
  echo "Instalador NSIS generado en: $NSIS_OUT"
fi

write_sha256sums
verify_sha256sums
write_artifacts_report
