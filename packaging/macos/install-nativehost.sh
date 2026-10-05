#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
umask 077

BASE_DIR="${HOME}/Library/Application Support/GrxFirma/NativeHost"
EXT_DIR="${HOME}/Library/Application Support/GrxFirma/Extensions"
MANIFEST_DIR_CHROME="${HOME}/Library/Application Support/Google/Chrome/NativeMessagingHosts"
MANIFEST_DIR_CHROMIUM="${HOME}/Library/Application Support/Chromium/NativeMessagingHosts"
MANIFEST_DIR_EDGE="${HOME}/Library/Application Support/Microsoft Edge/NativeMessagingHosts"
MANIFEST_DIR_BRAVE="${HOME}/Library/Application Support/BraveSoftware/Brave-Browser/NativeMessagingHosts"
MANIFEST_DIR_VIVALDI="${HOME}/Library/Application Support/Vivaldi/NativeMessagingHosts"
MANIFEST_DIR_OPERA="${HOME}/Library/Application Support/com.operasoftware.Opera/NativeMessagingHosts"
MANIFEST_DIR_FIREFOX="${HOME}/Library/Application Support/Mozilla/NativeMessagingHosts"

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_BIN="${SELF_DIR}/grxfirma-nativehost"
DST_BIN="${BASE_DIR}/grxfirma-nativehost"
FIREFOX_XPI="${SELF_DIR}/extensions/grxfirma-extension-firefox.xpi"
FIREFOX_METADATA="${SELF_DIR}/extensions/grxfirma-extension-firefox.metadata.json"
CHROMIUM_ZIP="${SELF_DIR}/extensions/grxfirma-extension-chromium.zip"
CHROMIUM_CRX="${SELF_DIR}/extensions/grxfirma-extension-chromium.crx"
CHROMIUM_ID_FILE="${SELF_DIR}/extensions/grxfirma-extension-chromium.id"
CHROMIUM_VERSION_FILE="${SELF_DIR}/extensions/grxfirma-extension-chromium.version"
EXTERNAL_DIR_CHROME="${HOME}/Library/Application Support/Google/Chrome/External Extensions"
EXTERNAL_DIR_CHROMIUM="${HOME}/Library/Application Support/Chromium/External Extensions"
EXTERNAL_DIR_EDGE="${HOME}/Library/Application Support/Microsoft Edge/External Extensions"
EXTERNAL_DIR_BRAVE="${HOME}/Library/Application Support/BraveSoftware/Brave-Browser/External Extensions"
EXTERNAL_DIR_VIVALDI="${HOME}/Library/Application Support/Vivaldi/External Extensions"
EXTERNAL_DIR_OPERA="${HOME}/Library/Application Support/com.operasoftware.Opera/External Extensions"

if [[ ! -f "$SRC_BIN" || -L "$SRC_BIN" ]]; then
  echo "error: no se encuentra grxfirma-nativehost junto al instalador" >&2
  exit 1
fi

CHROMIUM_PACKAGE_ID=""
if [[ -f "$CHROMIUM_ID_FILE" ]]; then
  CHROMIUM_PACKAGE_ID="$(tr -d '\r\n' < "$CHROMIUM_ID_FILE")"
fi

CHROMIUM_IDS=("pkefjandjcgdmhoonmhnllikibobijgg")
add_chromium_id() {
  local id="$1"
  [[ -n "${id}" ]] || return 0
  if [[ ! "${id}" =~ ^[a-p]{32}$ ]]; then
    echo "error: ID de extension Chromium no valido: ${id}" >&2
    exit 1
  fi
  local existing
  for existing in "${CHROMIUM_IDS[@]}"; do
    [[ "${existing}" != "${id}" ]] || return 0
  done
  CHROMIUM_IDS+=("${id}")
}

add_chromium_id "${CHROMIUM_PACKAGE_ID}"
add_chromium_id "${GRXFIRMA_CHROMIUM_EXTENSION_ID:-}"
add_chromium_id "${GRXFIRMA_EDGE_EXTENSION_ID:-}"

validate_external_update() {
  local extension_id="$1"
  local update_url="$2"
  [[ -n "${update_url}" ]] || return 0
  if [[ ! "${extension_id}" =~ ^[a-p]{32}$ ]]; then
    echo "error: falta un ID valido para registrar el canal de actualizacion" >&2
    exit 1
  fi
  case "${update_url}" in
    https://*) ;;
    *)
      echo "error: external_update_url debe usar HTTPS: ${update_url}" >&2
      exit 1
      ;;
  esac
  if [[ "${update_url}" == *\"* || "${update_url}" == *\\* || "${update_url}" == *' '* ||
        "${update_url}" == *$'\t'* || "${update_url}" == *$'\r'* || "${update_url}" == *$'\n'* ]]; then
    echo "error: external_update_url contiene caracteres no permitidos" >&2
    exit 1
  fi
}

CHROMIUM_STORE_ID="${GRXFIRMA_CHROMIUM_EXTENSION_ID:-${CHROMIUM_PACKAGE_ID}}"
EDGE_STORE_ID="${GRXFIRMA_EDGE_EXTENSION_ID:-${CHROMIUM_STORE_ID}}"
CHROMIUM_UPDATE_URL="${GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL:-}"
EDGE_UPDATE_URL="${GRXFIRMA_EDGE_EXTENSION_UPDATE_URL:-${CHROMIUM_UPDATE_URL}}"
validate_external_update "${CHROMIUM_STORE_ID}" "${CHROMIUM_UPDATE_URL}"
validate_external_update "${EDGE_STORE_ID}" "${EDGE_UPDATE_URL}"

mkdir -p "$BASE_DIR" "$EXT_DIR/firefox" "$EXT_DIR/chromium" "$MANIFEST_DIR_CHROME" "$MANIFEST_DIR_CHROMIUM" "$MANIFEST_DIR_EDGE" "$MANIFEST_DIR_BRAVE" "$MANIFEST_DIR_VIVALDI" "$MANIFEST_DIR_OPERA" "$MANIFEST_DIR_FIREFOX"
install -m 755 "$SRC_BIN" "$DST_BIN"

CHROME_ORIGINS=()
for chromium_id in "${CHROMIUM_IDS[@]}"; do
  CHROME_ORIGINS+=("chrome-extension://${chromium_id}/")
done

json_quote() {
  local LC_ALL=C
  local value="$1"
  local escaped=""
  local char code index
  for ((index = 0; index < ${#value}; index++)); do
    char="${value:index:1}"
    case "${char}" in
      '"') escaped+="\\\"" ;;
      \\) escaped+="\\\\" ;;
      $'\b') escaped+="\\b" ;;
      $'\f') escaped+="\\f" ;;
      $'\n') escaped+="\\n" ;;
      $'\r') escaped+="\\r" ;;
      $'\t') escaped+="\\t" ;;
      *)
        printf -v code '%d' "'${char}"
        if ((code < 32)); then
          printf -v char '\\u%04x' "${code}"
        fi
        escaped+="${char}"
        ;;
    esac
  done
  printf '"%s"' "${escaped}"
}

write_json_string_array() {
  local value
  local separator=""
  printf '['
  for value in "$@"; do
    printf '%s' "${separator}"
    json_quote "${value}"
    separator=', '
  done
  printf ']'
}

write_chrome_manifest() {
  local target="$1"
  local name="$2"
  shift 2
  {
    printf '{\n'
    printf '  "allowed_origins": '
    write_json_string_array "$@"
    printf ',\n  "description": "GrxFirma Native Messaging Host",\n'
    printf '  "name": '
    json_quote "${name}"
    printf ',\n  "path": '
    json_quote "${DST_BIN}"
    printf ',\n  "type": "stdio"\n}\n'
  } > "${target}"
}

write_firefox_manifest() {
  local target="$1"
  local name="$2"
  shift 2
  {
    printf '{\n'
    printf '  "allowed_extensions": '
    write_json_string_array "$@"
    printf ',\n  "description": "GrxFirma Native Messaging Host",\n'
    printf '  "name": '
    json_quote "${name}"
    printf ',\n  "path": '
    json_quote "${DST_BIN}"
    printf ',\n  "type": "stdio"\n}\n'
  } > "${target}"
}

for dir in "$MANIFEST_DIR_CHROME" "$MANIFEST_DIR_CHROMIUM" "$MANIFEST_DIR_EDGE" "$MANIFEST_DIR_BRAVE" "$MANIFEST_DIR_VIVALDI" "$MANIFEST_DIR_OPERA"; do
  write_chrome_manifest "$dir/com.grxfirma.native.json" "com.grxfirma.native" "${CHROME_ORIGINS[@]}"
  write_chrome_manifest "$dir/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${CHROME_ORIGINS[@]}"
done

write_firefox_manifest "$MANIFEST_DIR_FIREFOX/com.grxfirma.native.json" "com.grxfirma.native" "grxfirma@aavidad.github.io"
write_firefox_manifest "$MANIFEST_DIR_FIREFOX/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "grxfirma@aavidad.github.io"

# Las versiones anteriores registraban los hosts com.dipgra.* y la extension
# Firefox extension@dipgra.es, y algunas tambien el host de la extension
# «portafirmas», que no forma parte de GrxFirma. Se retiran solo los
# manifiestos que apuntan a este host y las copias del XPI identicas a la que
# instalo GrxFirma; los de otros productos no se tocan.
legacy_host_path="$(json_quote "${DST_BIN}")"
for dir in "$MANIFEST_DIR_CHROME" "$MANIFEST_DIR_CHROMIUM" "$MANIFEST_DIR_EDGE" "$MANIFEST_DIR_BRAVE" "$MANIFEST_DIR_VIVALDI" "$MANIFEST_DIR_OPERA" "$MANIFEST_DIR_FIREFOX"; do
  for legacy_name in com.dipgra.grxfirma com.dipgra.portafirmas io.github.aavidad.portafirmas; do
    legacy_manifest="${dir}/${legacy_name}.json"
    [[ -f "${legacy_manifest}" && ! -L "${legacy_manifest}" ]] || continue
    if grep -Fq "\"path\": ${legacy_host_path}" "${legacy_manifest}"; then
      rm -f -- "${legacy_manifest}"
    fi
  done
done
legacy_packaged_xpi="$EXT_DIR/firefox/dipgra-extension-firefox.xpi"
if [[ -f "${legacy_packaged_xpi}" && ! -L "${legacy_packaged_xpi}" ]]; then
  legacy_xpi_hash="$(shasum -a 256 "${legacy_packaged_xpi}" | awk '{print $1}')"
  for prof in "${HOME}"/Library/Application\ Support/Firefox/Profiles/*; do
    legacy_profile_xpi="${prof}/extensions/extension@dipgra.es.xpi"
    [[ -f "${legacy_profile_xpi}" && ! -L "${legacy_profile_xpi}" ]] || continue
    if [[ "$(shasum -a 256 "${legacy_profile_xpi}" | awk '{print $1}')" == "${legacy_xpi_hash}" ]]; then
      rm -f -- "${legacy_profile_xpi}"
    fi
  done
fi
rm -f "$EXT_DIR/firefox/dipgra-extension-firefox.xpi" \
  "$EXT_DIR/firefox/dipgra-extension-firefox.metadata.json" \
  "$EXT_DIR/chromium/dipgra-extension-chromium.zip" \
  "$EXT_DIR/chromium/dipgra-extension-chromium.crx" \
  "$EXT_DIR/chromium/dipgra-extension-chromium.id" \
  "$EXT_DIR/chromium/dipgra-extension-chromium.version"

firefox_xpi_is_approved() {
  [[ -f "${FIREFOX_XPI}" && -f "${FIREFOX_METADATA}" ]] || return 1
  if [[ -x /usr/bin/plutil ]]; then
    local key occurrences
    for key in extension_id signed xpi_sha256; do
      occurrences="$(
        { grep -Eo "\"${key}\"[[:space:]]*:" "${FIREFOX_METADATA}" || true; } |
          wc -l |
          tr -d '[:space:]'
      )"
      [[ "${occurrences}" == "1" ]] || return 1
    done
    /usr/bin/plutil -lint "${FIREFOX_METADATA}" >/dev/null 2>&1 || return 1
    local extension_id signed expected actual
    extension_id="$(/usr/bin/plutil -extract extension_id raw -o - "${FIREFOX_METADATA}" 2>/dev/null)" || return 1
    signed="$(/usr/bin/plutil -extract signed raw -o - "${FIREFOX_METADATA}" 2>/dev/null)" || return 1
    expected="$(/usr/bin/plutil -extract xpi_sha256 raw -o - "${FIREFOX_METADATA}" 2>/dev/null)" || return 1
    [[ "${extension_id}" == "grxfirma@aavidad.github.io" ]] || return 1
    [[ "${signed}" == "true" ]] || return 1
    [[ "${expected}" =~ ^[0-9a-fA-F]{64}$ ]] || return 1
    actual="$(shasum -a 256 "${FIREFOX_XPI}" | awk '{print $1}')"
    [[ "${actual}" == "$(printf '%s' "${expected}" | tr '[:upper:]' '[:lower:]')" ]]
    return
  fi

  # Fallback portable para las pruebas de packaging fuera de macOS.
  python3 - "${FIREFOX_XPI}" "${FIREFOX_METADATA}" <<'PY'
import hashlib
import json
import pathlib
import re
import sys

xpi, metadata = map(pathlib.Path, sys.argv[1:])
def reject_duplicate_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate metadata key: {key}")
        result[key] = value
    return result

try:
    payload = json.loads(
        metadata.read_text(encoding="utf-8"),
        object_pairs_hook=reject_duplicate_keys,
    )
except (OSError, UnicodeError, ValueError):
    raise SystemExit(1)
expected = payload.get("xpi_sha256")
if (
    payload.get("signed") is not True
    or payload.get("extension_id") != "grxfirma@aavidad.github.io"
    or not isinstance(expected, str)
    or re.fullmatch(r"[0-9a-fA-F]{64}", expected) is None
):
    raise SystemExit(1)
digest = hashlib.sha256()
with xpi.open("rb") as stream:
    for chunk in iter(lambda: stream.read(1024 * 1024), b""):
        digest.update(chunk)
raise SystemExit(0 if digest.hexdigest() == expected.lower() else 1)
PY
}

if [[ -f "$FIREFOX_XPI" ]]; then
  install -m 644 "$FIREFOX_XPI" "$EXT_DIR/firefox/grxfirma-extension-firefox.xpi"
  if [[ -f "${FIREFOX_METADATA}" ]]; then
    install -m 644 "${FIREFOX_METADATA}" "$EXT_DIR/firefox/grxfirma-extension-firefox.metadata.json"
  else
    rm -f "$EXT_DIR/firefox/grxfirma-extension-firefox.metadata.json"
  fi
  if firefox_xpi_is_approved && [[ -d "${HOME}/Library/Application Support/Firefox/Profiles" ]]; then
    for prof in "${HOME}"/Library/Application\ Support/Firefox/Profiles/*; do
      [[ -d "${prof}" && -f "${prof}/prefs.js" ]] || continue
      mkdir -p "${prof}/extensions"
      install -m 644 "$FIREFOX_XPI" "${prof}/extensions/grxfirma@aavidad.github.io.xpi"
    done
  elif ! firefox_xpi_is_approved; then
    echo "Aviso: el XPI Firefox no esta firmado/aprobado o no coincide con su SHA-256; no se instala en perfiles estables." >&2
  fi
fi

if [[ -f "$CHROMIUM_ZIP" ]]; then
  install -m 644 "$CHROMIUM_ZIP" "$EXT_DIR/chromium/grxfirma-extension-chromium.zip"
fi
if [[ -f "$CHROMIUM_CRX" && -f "$CHROMIUM_ID_FILE" && -f "$CHROMIUM_VERSION_FILE" ]]; then
  install -m 644 "$CHROMIUM_CRX" "$EXT_DIR/chromium/grxfirma-extension-chromium.crx"
  install -m 644 "$CHROMIUM_ID_FILE" "$EXT_DIR/chromium/grxfirma-extension-chromium.id"
  install -m 644 "$CHROMIUM_VERSION_FILE" "$EXT_DIR/chromium/grxfirma-extension-chromium.version"
fi

register_external_update() {
  local target_dir="$1"
  local extension_id="$2"
  local update_url="$3"
  validate_external_update "${extension_id}" "${update_url}"
  if [[ -z "${update_url}" ]]; then
    if [[ -n "${extension_id}" && -d "${target_dir}" ]]; then
      rm -f "${target_dir}/${extension_id}.json"
    fi
    return 0
  fi
  mkdir -p "${target_dir}"
  {
    printf '{\n  "external_update_url": '
    json_quote "${update_url}"
    printf '\n}\n'
  } > "${target_dir}/${extension_id}.json"
}

for ext_dir in "$EXTERNAL_DIR_CHROME" "$EXTERNAL_DIR_CHROMIUM" "$EXTERNAL_DIR_BRAVE" "$EXTERNAL_DIR_VIVALDI" "$EXTERNAL_DIR_OPERA"; do
  register_external_update "${ext_dir}" "${CHROMIUM_STORE_ID}" "${CHROMIUM_UPDATE_URL}"
done
register_external_update "$EXTERNAL_DIR_EDGE" "${EDGE_STORE_ID}" "${EDGE_UPDATE_URL}"

echo "NativeHost instalado en: $BASE_DIR"
