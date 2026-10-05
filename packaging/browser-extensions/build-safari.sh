#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EXTENSION_SRC="${SAFARI_EXTENSION_SRC:-${ROOT_DIR}/packaging/browser-extensions/src/chromium}"
PROJECT_DIR="${SAFARI_PROJECT_DIR:-${ROOT_DIR}/release/safari-web-extension}"
APP_NAME="${SAFARI_APP_NAME:-GrxFirma Safari}"
BUNDLE_ID="${SAFARI_BUNDLE_ID:-io.github.aavidad.grxfirma.safari}"
MIN_MACOS="${SAFARI_MIN_MACOS:-12.3}"
DEFAULT_ENDPOINT="${SAFARI_DEFAULT_ENDPOINT:-https://127.0.0.1:63118}"
SKIP_XCODE_BUILD="${SAFARI_SKIP_XCODE_BUILD:-0}"
SAFARI_TEMPLATE_DIR="${ROOT_DIR}/packaging/browser-extensions/safari"
POSTPROCESSOR="${SAFARI_TEMPLATE_DIR}/postprocess.py"
SOURCE_VALIDATOR="${SAFARI_TEMPLATE_DIR}/validate_source.py"
ENVIRONMENT_RECORDER="${SAFARI_TEMPLATE_DIR}/record_environment.py"

if [[ "$(uname -s)" != "Darwin" ]]; then
  cat >&2 <<'EOF'
error: Safari Web Extension requiere macOS/Xcode para generar el proyecto local.
       El empaquetado web sin Xcode no compila ni valida el handler nativo de
       GrxFirma; ejecuta este script en un host Apple antes de distribuir.
EOF
  exit 1
fi

if [[ "${SKIP_XCODE_BUILD}" != "0" && "${SKIP_XCODE_BUILD}" != "1" ]]; then
  echo "error: SAFARI_SKIP_XCODE_BUILD solo admite 0 o 1." >&2
  exit 1
fi

if [[ ! "${BUNDLE_ID}" =~ ^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$ ]]; then
  echo "error: SAFARI_BUNDLE_ID no es un identificador Apple valido." >&2
  exit 1
fi

if ! command -v xcrun >/dev/null 2>&1; then
  echo "error: xcrun no esta disponible. Instala Xcode completo." >&2
  exit 1
fi

if ! xcrun --find safari-web-extension-converter >/dev/null 2>&1; then
  cat >&2 <<'EOF'
error: safari-web-extension-converter no esta disponible en este Xcode.
       Usa una version de Xcode con Safari Web Extension Converter o el flujo
       Safari Web Extension Packager de App Store Connect.
EOF
  exit 1
fi

if ! xcrun --find xcodebuild >/dev/null 2>&1; then
  echo "error: xcodebuild no esta disponible. Selecciona Xcode con xcode-select." >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "error: python3 es necesario para validar y endurecer el proyecto generado." >&2
  exit 1
fi

if ! command -v plutil >/dev/null 2>&1; then
  echo "error: plutil no esta disponible en este macOS." >&2
  exit 1
fi

if [[ ! -f "${EXTENSION_SRC}/manifest.json" ]]; then
  echo "error: no se encuentra manifest.json en ${EXTENSION_SRC}" >&2
  exit 1
fi
if [[ -n "$(find "${EXTENSION_SRC}" -type l -print -quit)" ]]; then
  echo "error: la fuente Safari no puede contener enlaces simbolicos." >&2
  exit 1
fi

for required_file in \
  "${POSTPROCESSOR}" \
  "${SOURCE_VALIDATOR}" \
  "${ENVIRONMENT_RECORDER}" \
  "${SAFARI_TEMPLATE_DIR}/SafariWebExtensionHandler.swift.in" \
  "${SAFARI_TEMPLATE_DIR}/ViewController.swift.in" \
  "${SAFARI_TEMPLATE_DIR}/SafariNativeOnly.js.in"; do
  if [[ ! -f "${required_file}" ]]; then
    echo "error: falta el componente Safari ${required_file}" >&2
    exit 1
  fi
done

python3 "${SOURCE_VALIDATOR}" \
  --manifest "${EXTENSION_SRC}/manifest.json" \
  --minimum-macos "${MIN_MACOS}"

PROJECT_PARENT="$(dirname "${PROJECT_DIR}")"
PROJECT_BASENAME="$(basename "${PROJECT_DIR}")"
if [[ -z "${PROJECT_BASENAME}" || "${PROJECT_BASENAME}" == "." || "${PROJECT_BASENAME}" == ".." ]]; then
  echo "error: SAFARI_PROJECT_DIR no identifica un directorio de salida valido." >&2
  exit 1
fi
mkdir -p "${PROJECT_PARENT}"
PROJECT_PARENT="$(cd "${PROJECT_PARENT}" && pwd -P)"
PROJECT_DIR="${PROJECT_PARENT}/${PROJECT_BASENAME}"
if [[ -L "${PROJECT_DIR}" ]]; then
  echo "error: SAFARI_PROJECT_DIR no puede ser un enlace simbolico." >&2
  exit 1
fi
if [[ -e "${PROJECT_DIR}" && ! -d "${PROJECT_DIR}" ]]; then
  echo "error: SAFARI_PROJECT_DIR existe y no es un directorio." >&2
  exit 1
fi

STAGING_DIR="$(mktemp -d "${PROJECT_PARENT}/.grxfirma-safari-build.XXXXXX")"
BACKUP_DIR=""
DERIVED_DATA=""
cleanup() {
  if [[ -n "${DERIVED_DATA}" ]]; then
    rm -rf "${DERIVED_DATA}"
  fi
  if [[ -n "${STAGING_DIR}" ]]; then
    rm -rf "${STAGING_DIR}"
  fi
  if [[ -n "${BACKUP_DIR}" && -d "${BACKUP_DIR}" ]]; then
    if [[ ! -e "${PROJECT_DIR}" ]]; then
      mv "${BACKUP_DIR}" "${PROJECT_DIR}" || true
    else
      rm -rf "${BACKUP_DIR}"
    fi
  fi
}
trap cleanup EXIT

help_text="$(xcrun safari-web-extension-converter --help 2>&1 || true)"
for required_flag in '--macos-only' '--copy-resources' '--macos-version-minimum'; do
  if ! grep -q -- "${required_flag}" <<<"${help_text}"; then
    echo "error: safari-web-extension-converter no admite ${required_flag}. Actualiza Xcode." >&2
    exit 1
  fi
done
args=(
  "${EXTENSION_SRC}"
  "--project-location" "${STAGING_DIR}"
  "--app-name" "${APP_NAME}"
  "--bundle-identifier" "${BUNDLE_ID}"
)

append_if_supported() {
  local flag="$1"
  if grep -q -- "$flag" <<<"${help_text}"; then
    args+=("$flag")
  fi
}

args+=("--macos-only" "--copy-resources" "--macos-version-minimum" "${MIN_MACOS}")
append_if_supported "--force"
append_if_supported "--no-open"
append_if_supported "--no-prompt"

echo "Generando proyecto Safari Web Extension..."
echo "Fuente: ${EXTENSION_SRC}"
echo "Salida: ${PROJECT_DIR}"
xcrun safari-web-extension-converter "${args[@]}"

echo "Instalando puente Safari nativo y configuracion segura..."
python3 "${POSTPROCESSOR}" \
  --project-dir "${STAGING_DIR}" \
  --bundle-id "${BUNDLE_ID}" \
  --default-endpoint "${DEFAULT_ENDPOINT}" \
  --template-dir "${SAFARI_TEMPLATE_DIR}"

XCODE_VERSION="$(xcrun xcodebuild -version)"
CONVERTER_PATH="$(xcrun --find safari-web-extension-converter)"
python3 "${ENVIRONMENT_RECORDER}" \
  --source "${EXTENSION_SRC}" \
  --report "${STAGING_DIR}/.grxfirma-safari/integration-report.json" \
  --xcode-version "${XCODE_VERSION}" \
  --converter-path "${CONVERTER_PATH}" \
  --minimum-macos "${MIN_MACOS}"

projects=()
while IFS= read -r project; do
  projects+=("${project}")
done < <(find "${STAGING_DIR}" -type d -name '*.xcodeproj' -print)

if [[ "${#projects[@]}" -ne 1 ]]; then
  echo "error: se esperaba un unico proyecto Xcode y se encontraron ${#projects[@]}." >&2
  exit 1
fi
XCODE_PROJECT="${projects[0]}"

handlers=()
while IFS= read -r handler; do
  handlers+=("${handler}")
done < <(find "${STAGING_DIR}" -type f -name 'SafariWebExtensionHandler.swift' -print)
if [[ "${#handlers[@]}" -ne 1 ]] || ! grep -q 'GRXFIRMA_SAFARI_BRIDGE_V1' "${handlers[0]}"; then
  echo "error: el handler Safari seguro no quedo instalado de forma verificable." >&2
  exit 1
fi

view_controllers=()
while IFS= read -r view_controller; do
  view_controllers+=("${view_controller}")
done < <(find "${STAGING_DIR}" -type f -name 'ViewController.swift' -print)
if [[ "${#view_controllers[@]}" -ne 1 ]] || ! grep -q 'GRXFIRMA_SAFARI_BRIDGE_V1' "${view_controllers[0]}"; then
  echo "error: la app contenedora segura no quedo instalada de forma verificable." >&2
  exit 1
fi

background_scripts=()
while IFS= read -r background_script; do
  background_scripts+=("${background_script}")
done < <(find "${STAGING_DIR}" -type f -name 'background.js' -print)
if [[ "${#background_scripts[@]}" -ne 1 ]] || ! grep -q 'GRXFIRMA_SAFARI_NATIVE_ONLY_START' "${background_scripts[0]}"; then
  echo "error: el background Safari conserva un fallback REST no controlado." >&2
  exit 1
fi

entitlements_files=()
while IFS= read -r entitlements; do
  entitlements_files+=("${entitlements}")
done < <(find "${STAGING_DIR}" -type f -name 'GrxFirma*.entitlements' -print)
if [[ "${#entitlements_files[@]}" -ne 2 ]]; then
  echo "error: se esperaban dos ficheros de entitlements y se encontraron ${#entitlements_files[@]}." >&2
  exit 1
fi
for entitlements in "${entitlements_files[@]}"; do
  plutil -lint "${entitlements}"
done

echo "Validando estructura Xcode..."
xcrun xcodebuild -project "${XCODE_PROJECT}" -list >/dev/null

if [[ "${SKIP_XCODE_BUILD}" == "0" ]]; then
  DERIVED_DATA="$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-safari.XXXXXX")"
  echo "Compilando todos los targets sin firma..."
  xcrun xcodebuild \
    -project "${XCODE_PROJECT}" \
    -alltargets \
    -configuration Debug \
    -sdk macosx \
    SYMROOT="${DERIVED_DATA}/Build/Products" \
    OBJROOT="${DERIVED_DATA}/Build/Intermediates.noindex" \
    SHARED_PRECOMPS_DIR="${DERIVED_DATA}/Build/Intermediates.noindex/PrecompiledHeaders" \
    CLANG_MODULE_CACHE_PATH="${DERIVED_DATA}/ModuleCache.noindex" \
    CODE_SIGNING_ALLOWED=NO \
    CODE_SIGNING_REQUIRED=NO \
    build
else
  echo "Aviso: compilacion Xcode omitida por SAFARI_SKIP_XCODE_BUILD=${SKIP_XCODE_BUILD}." >&2
fi

if [[ -d "${PROJECT_DIR}" ]]; then
  BACKUP_DIR="$(mktemp -d "${PROJECT_PARENT}/.grxfirma-safari-backup.XXXXXX")"
  rmdir "${BACKUP_DIR}"
  mv "${PROJECT_DIR}" "${BACKUP_DIR}"
fi
if ! mv "${STAGING_DIR}" "${PROJECT_DIR}"; then
  if [[ -n "${BACKUP_DIR}" && -d "${BACKUP_DIR}" ]]; then
    mv "${BACKUP_DIR}" "${PROJECT_DIR}"
    BACKUP_DIR=""
  fi
  echo "error: no se pudo publicar el proyecto Safari generado." >&2
  exit 1
fi
STAGING_DIR=""
if [[ -n "${BACKUP_DIR}" ]]; then
  rm -rf "${BACKUP_DIR}"
  BACKUP_DIR=""
fi

cat <<EOF
Proyecto Safari generado en: ${PROJECT_DIR}

Integracion instalada:
- SafariWebExtensionHandler enlazado al REST HTTPS loopback de GrxFirma;
- app contenedora para guardar endpoint, Bearer y pin TLS en Keychain compartido;
- autenticacion local obligatoria en cada firma;
- fallback REST JavaScript desactivado para evitar reintentos sin credenciales;
- limites de payload, lista cerrada de acciones y errores controlados;
- targets compilados sin firma salvo que se haya definido SAFARI_SKIP_XCODE_BUILD=1.

Pendiente necesariamente de entorno Apple:
- seleccionar Team y perfiles de firma en Xcode;
- instalar confianza TLS, ejecutar la app, guardar Bearer/pin y habilitar la extension;
- validar firma y verificacion en Safari real;
- archivar, firmar y notarizar o publicar mediante App Store Connect.
EOF
