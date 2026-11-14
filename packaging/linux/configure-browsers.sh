#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

# Catálogos de datos compartidos con el instalador.
# shellcheck source=packaging/linux/installer-i18n.sh
source "$(dirname "${BASH_SOURCE[0]}")/installer-i18n.sh"

TARGET_HOME="${GRXFIRMA_TARGET_HOME:-${HOME}}"
DESKTOP_ID="${GRXFIRMA_DESKTOP_ID:-grxfirma.desktop}"
BROWSER_BRIDGE="${GRXFIRMA_BROWSER_BRIDGE:-/usr/lib/grxfirma/bin/browser-bridge.sh}"
FIREFOX_XPI="${GRXFIRMA_FIREFOX_XPI:-/usr/lib/grxfirma/extensions/grxfirma-extension-firefox.xpi}"
FIREFOX_METADATA="${GRXFIRMA_FIREFOX_METADATA:-/usr/lib/grxfirma/extensions/grxfirma-extension-firefox.metadata.json}"
FIREFOX_EXT_ID="${FIREFOX_EXT_ID:-grxfirma@aavidad.github.io}"
CHROME_EXT_ID="${CHROME_EXT_ID:-pkefjandjcgdmhoonmhnllikibobijgg}"
EXTRA_CHROME_EXT_ID="${EXTRA_CHROME_EXT_ID:-}"
SYSTEM_APP_DIRS="${GRXFIRMA_SYSTEM_APP_DIRS:-/usr/local/share/applications:/usr/share/applications}"

log() {
  printf 'grxfirma browsers: %s\n' "$*"
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

# Lanzadores que abrían afirma:// antes de GrxFirma según la copia guardada
# al instalar (HANDLER_BACKUP), uno por línea. Solo se devuelven nombres de
# .desktop válidos; el contenido de cada uno se comprueba aparte.
afirma_saved_handler_ids() {
  [[ -f "${HANDLER_BACKUP}" && ! -L "${HANDLER_BACKUP}" ]] || return 0
  have_cmd python3 || return 0
  python3 - "${HANDLER_BACKUP}" "${DESKTOP_ID}" <<'PY' 2>/dev/null || true
import json
import re
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        backup = json.load(fh)
except (OSError, ValueError):
    sys.exit(0)
if not isinstance(backup, dict):
    sys.exit(0)
for path in sorted(backup):
    entry = backup[path]
    value = entry.get("x-scheme-handler/afirma") if isinstance(entry, dict) else None
    if not isinstance(value, str):
        continue
    for desktop_id in value.split(";"):
        desktop_id = desktop_id.strip()
        if re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*\.desktop", desktop_id) and desktop_id != sys.argv[2]:
            print(desktop_id)
PY
}

# Verdadero si el .desktop abre afirma:// con un programa que se llama
# autofirma (como los paquetes oficiales de AutoFirma).
afirma_desktop_is_autofirma() {
  local file="$1" exec_line program
  [[ -f "${file}" && ! -L "${file}" ]] || return 1
  grep -Eq '^MimeType=(.*;)?x-scheme-handler/afirma(;|$)' "${file}" || return 1
  exec_line="$(sed -n 's/^Exec=//p' "${file}" | head -n 1)"
  program="${exec_line%% *}"
  program="${program//\"/}"
  program="${program##*/}"
  [[ "${program,,}" == autofirma* ]]
}

# Verdadero si AutoFirma está instalado: los nombres de los paquetes
# oficiales o el lanzador que abría afirma:// antes de GrxFirma, si es
# AutoFirma (su nombre depende del paquete).
afirma_autofirma_installed() {
  local dir id
  local -a dirs saved
  IFS=: read -r -a dirs <<< "${SYSTEM_APP_DIRS}"
  for dir in "${TARGET_HOME}/.local/share/applications" "${dirs[@]}"; do
    if [[ -f "${dir}/afirma.desktop" || -f "${dir}/autofirma.desktop" ]]; then
      return 0
    fi
  done
  mapfile -t saved < <(afirma_saved_handler_ids)
  for id in "${saved[@]}"; do
    for dir in "${TARGET_HOME}/.local/share/applications" "${dirs[@]}"; do
      if afirma_desktop_is_autofirma "${dir}/${id}"; then
        return 0
      fi
    done
  done
  return 1
}

# Programa para firmar en las webs (lote 15): grxfirma, autofirma o
# preguntar. «preguntar» deja afirma:// a GrxFirma, que pregunta en cada firma.
AFIRMA_PREFERENCE_FILE="${TARGET_HOME}/.config/grxfirma/afirma-protocol-handler"
POLICY_FILE="${GRXFIRMA_POLICY_FILE:-/etc/grxfirma/policy.json}"
# Programa pedido al instalar (install-suite.sh --afirma-handler=); vacío si no
# se pidió ninguno.
REQUESTED_AFIRMA_HANDLER="${GRXFIRMA_AFIRMA_HANDLER:-}"
AFIRMA_HANDLER=""
AFIRMA_HANDLER_SOURCE=""

afirma_valid_choice() {
  case "$1" in
    grxfirma | autofirma | preguntar) return 0 ;;
  esac
  return 1
}

# Valor de manejador_afirma en la política de máquina: el valor, nada si no lo
# fija o «!» si el fichero existe pero no se puede leer o no se entiende.
afirma_policy_value() {
  [[ -e "${POLICY_FILE}" ]] || return 0
  if ! have_cmd python3; then
    printf '!'
    return 0
  fi
  python3 - "${POLICY_FILE}" <<'PY' 2>/dev/null || printf '!'
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as fh:
        data = json.load(fh)
except (OSError, ValueError):
    print("!")
    sys.exit(0)
if not isinstance(data, dict):
    print("!")
    sys.exit(0)
value = data.get("manejador_afirma")
if value is None:
    sys.exit(0)
value = value.strip().lower() if isinstance(value, str) else ""
print(value if value in ("grxfirma", "autofirma", "preguntar") else "!")
PY
}

# Lo que eligió la persona en Configuración, o nada.
afirma_preference_value() {
  local value
  [[ -f "${AFIRMA_PREFERENCE_FILE}" && ! -L "${AFIRMA_PREFERENCE_FILE}" ]] || return 0
  value="$(head -c 32 "${AFIRMA_PREFERENCE_FILE}" | tr -d '[:space:]')"
  if afirma_valid_choice "${value}"; then
    printf '%s' "${value}"
  fi
  return 0
}

# Decide qué programa atiende afirma:// al instalar: la política; si no, el
# que pide el instalador; si no, el que eligió la persona; si no, GrxFirma,
# como hasta ahora. AutoFirma solo si sigue instalado. Deja el resultado en
# AFIRMA_HANDLER y AFIRMA_HANDLER_SOURCE (politica, parametro, preferencia o
# defecto).
afirma_resolve_handler() {
  local policy preference
  policy="$(afirma_policy_value)"
  if [[ "${policy}" == "!" ]]; then
    log "$(grx_message MSG_001 "${POLICY_FILE}")"
    policy=""
  fi
  preference="$(afirma_preference_value)"
  if [[ -n "${policy}" ]]; then
    AFIRMA_HANDLER="${policy}"
    AFIRMA_HANDLER_SOURCE="politica"
  elif [[ -n "${REQUESTED_AFIRMA_HANDLER}" ]]; then
    AFIRMA_HANDLER="${REQUESTED_AFIRMA_HANDLER}"
    AFIRMA_HANDLER_SOURCE="parametro"
  elif [[ -n "${preference}" ]]; then
    AFIRMA_HANDLER="${preference}"
    AFIRMA_HANDLER_SOURCE="preferencia"
  else
    AFIRMA_HANDLER="grxfirma"
    AFIRMA_HANDLER_SOURCE="defecto"
  fi
  if [[ "${AFIRMA_HANDLER}" != grxfirma ]] && ! afirma_autofirma_installed; then
    if [[ "${AFIRMA_HANDLER_SOURCE}" == politica || "${AFIRMA_HANDLER_SOURCE}" == parametro ]]; then
      log "$(grx_message MSG_002)"
    fi
    AFIRMA_HANDLER="grxfirma"
  fi
}

# Guarda la elección pedida al instalar (--afirma-handler=). La política nunca
# la escribe, para que vuelva a valer si la organización la retira.
afirma_save_requested_preference() {
  local dir tmp
  [[ "${AFIRMA_HANDLER_SOURCE}" == parametro ]] || return 0
  [[ "$(afirma_preference_value)" != "${AFIRMA_HANDLER}" ]] || return 0
  dir="$(dirname "${AFIRMA_PREFERENCE_FILE}")"
  mkdir -p "${dir}"
  tmp="$(mktemp "${dir}/.afirma-protocol-handler.XXXXXXXX")"
  printf '%s\n' "${AFIRMA_HANDLER}" > "${tmp}"
  chmod 600 "${tmp}"
  mv -f -- "${tmp}" "${AFIRMA_PREFERENCE_FILE}"
}

# Lanzador de AutoFirma al que devolver afirma://: el que lo abría antes de
# GrxFirma si es AutoFirma; si no, el de los paquetes oficiales.
afirma_autofirma_desktop_id() {
  local dir id
  local -a dirs saved
  IFS=: read -r -a dirs <<< "${SYSTEM_APP_DIRS}"
  mapfile -t saved < <(afirma_saved_handler_ids)
  for id in "${saved[@]}" afirma.desktop autofirma.desktop; do
    for dir in "${TARGET_HOME}/.local/share/applications" "${dirs[@]}"; do
      if afirma_desktop_is_autofirma "${dir}/${id}"; then
        printf '%s\n' "${id}"
        return 0
      fi
    done
  done
  for id in afirma.desktop autofirma.desktop; do
    for dir in "${TARGET_HOME}/.local/share/applications" "${dirs[@]}"; do
      if [[ -f "${dir}/${id}" && ! -L "${dir}/${id}" ]]; then
        printf '%s\n' "${id}"
        return 0
      fi
    done
  done
  return 0
}

# Verdadero si afirma:// debe quedar para AutoFirma (ya resuelto).
afirma_prefers_autofirma() {
  if [[ -z "${AFIRMA_HANDLER}" ]]; then
    afirma_resolve_handler
  fi
  [[ "${AFIRMA_HANDLER}" == autofirma ]]
}

# Para la pregunta de install-suite.sh: «autofirma|política|preferencia», con
# «-» cuando no hay valor (una política ilegible cuenta como «-»). Solo lee.
afirma_detect_handler() {
  local available=0 policy preference
  if afirma_autofirma_installed; then
    available=1
  fi
  policy="$(afirma_policy_value)"
  if [[ -z "${policy}" || "${policy}" == "!" ]]; then
    policy="-"
  fi
  preference="$(afirma_preference_value)"
  printf '%s|%s|%s\n' "${available}" "${policy}" "${preference:--}"
}

afirma_schemes() {
  if ! afirma_prefers_autofirma; then
    printf '%s\n' x-scheme-handler/afirma
  fi
  printf '%s\n' x-scheme-handler/afirmav2
}

# G-11: copia del manejador afirma:// que había antes de GrxFirma (normalmente
# AutoFirma), para devolverlo al desinstalar. Solo se guarda la primera vez.
HANDLER_BACKUP="${TARGET_HOME}/.local/lib/grxfirma/config/afirma-handler-anterior.json"

mimeapps_files() {
  printf '%s\n' "${TARGET_HOME}/.config/mimeapps.list"
  printf '%s\n' "${TARGET_HOME}/.local/share/applications/mimeapps.list"
  if [[ -d "${TARGET_HOME}/snap/firefox/common" ]]; then
    printf '%s\n' "${TARGET_HOME}/snap/firefox/common/.config/mimeapps.list"
  fi
  if [[ -d "${TARGET_HOME}/.var/app/org.mozilla.firefox" ]]; then
    printf '%s\n' "${TARGET_HOME}/.var/app/org.mozilla.firefox/config/mimeapps.list"
  fi
}

# Uso: afirma_handlers_py set|restore|autofirma <copia> <desktop_id> <esquemas> <mimeapps>...
# <esquemas> son los que GrxFirma toma, separados por comas (afirma_schemes);
# en «autofirma», el lanzador de AutoFirma.
afirma_handlers_py() {
  python3 - "$@" <<'PY'
import configparser
import json
import os
import sys

mode, backup_path, desktop_id, schemes_csv, *paths = sys.argv[1:]
schemes = ("x-scheme-handler/afirma", "x-scheme-handler/afirmav2")
# Los que se asignan a GrxFirma: sin afirma si la persona eligió AutoFirma.
assigned = tuple(s for s in schemes_csv.split(",") if s in schemes)
section = "Default Applications"


def load(path):
    parser = configparser.RawConfigParser(strict=False, delimiters=("=",), interpolation=None)
    parser.optionxform = str
    if os.path.isfile(path) and not os.path.islink(path):
        parser.read(path, encoding="utf-8")
    return parser


def save(parser, path):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = f"{path}.tmp.{os.getpid()}"
    with open(tmp, "w", encoding="utf-8") as fh:
        parser.write(fh, space_around_delimiters=False)
    os.replace(tmp, path)


try:
    with open(backup_path, encoding="utf-8") as fh:
        backup = json.load(fh)
    if not isinstance(backup, dict):
        backup = {}
except (OSError, ValueError):
    backup = {}

if mode == "set":
    previous_handlers = set()
    for path in paths:
        parser = load(path)
        if path not in backup:
            entry = {}
            for scheme in schemes:
                value = parser.get(section, scheme, fallback=None)
                entry[scheme] = None if value in (None, "", desktop_id) else value
                if entry[scheme] and scheme in assigned:
                    previous_handlers.add(entry[scheme])
            backup[path] = entry
        if not parser.has_section(section):
            parser.add_section(section)
        for scheme in assigned:
            parser.set(section, scheme, desktop_id)
        save(parser, path)
    os.makedirs(os.path.dirname(backup_path), mode=0o700, exist_ok=True)
    tmp = f"{backup_path}.tmp.{os.getpid()}"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(backup, fh, ensure_ascii=False, indent=2)
    os.chmod(tmp, 0o600)
    os.replace(tmp, backup_path)
    for handler in sorted(previous_handlers):
        print(handler)
elif mode == "autofirma":
    # Devuelve afirma:// a AutoFirma solo donde lo tiene GrxFirma; lo que la
    # persona haya puesto a mano se respeta. <esquemas> es aquí el lanzador.
    autofirma_id = schemes_csv
    for path in paths:
        parser = load(path)
        if parser.get(section, schemes[0], fallback=None) != desktop_id:
            continue
        parser.set(section, schemes[0], autofirma_id)
        save(parser, path)
elif mode == "restore":
    allowed = set(paths)
    for path, entry in backup.items():
        # Solo se tocan las rutas conocidas, nunca lo que diga el fichero.
        if path not in allowed or not isinstance(entry, dict) or not os.path.isfile(path):
            continue
        parser = load(path)
        if not parser.has_section(section):
            continue
        changed = False
        for scheme in schemes:
            if parser.get(section, scheme, fallback=None) != desktop_id:
                continue  # el usuario lo cambió después: se respeta
            previous = entry.get(scheme)
            if isinstance(previous, str) and previous:
                parser.set(section, scheme, previous)
            else:
                parser.remove_option(section, scheme)
            changed = True
        if changed:
            save(parser, path)
    try:
        os.remove(backup_path)
    except FileNotFoundError:
        pass
PY
}

sanitize_afirma_scheme_handlers() {
  local files=() schemes=() previous scheme_csv
  mapfile -t files < <(mimeapps_files)
  mapfile -t schemes < <(afirma_schemes)
  if ! have_cmd python3; then
    local file scheme
    for file in "${files[@]}"; do
      mkdir -p "$(dirname "${file}")"
      {
        printf '\n[Default Applications]\n'
        for scheme in "${schemes[@]}"; do
          printf '%s=%s\n' "${scheme}" "${DESKTOP_ID}"
        done
      } >> "${file}"
    done
    if ! afirma_prefers_autofirma; then
      log "$(grx_message MSG_003)"
    fi
    return 0
  fi
  scheme_csv="$(IFS=,; printf '%s' "${schemes[*]}")"
  previous="$(afirma_handlers_py set "${HANDLER_BACKUP}" "${DESKTOP_ID}" "${scheme_csv}" "${files[@]}")"
  if [[ -n "${previous}" ]]; then
    log "$(grx_message MSG_004 "${previous//$'\n'/, }")"
  fi
  if afirma_prefers_autofirma; then
    local autofirma_id
    autofirma_id="$(afirma_autofirma_desktop_id)"
    if [[ -n "${autofirma_id}" ]]; then
      afirma_handlers_py autofirma "${HANDLER_BACKUP}" "${DESKTOP_ID}" "${autofirma_id}" "${files[@]}"
    fi
  fi
}

restore_afirma_scheme_handlers() {
  local files=()
  mapfile -t files < <(mimeapps_files)
  if have_cmd python3; then
    afirma_handlers_py restore "${HANDLER_BACKUP}" "${DESKTOP_ID}" "" "${files[@]}"
  fi
  local root user_js tmp
  for root in \
    "${TARGET_HOME}/.mozilla/firefox" \
    "${TARGET_HOME}/snap/firefox/common/.mozilla/firefox" \
    "${TARGET_HOME}/.var/app/org.mozilla.firefox/.mozilla/firefox"; do
    [[ -d "${root}" && ! -L "${root}" ]] || continue
    while IFS= read -r -d '' user_js; do
      tmp="${user_js}.tmp.$$"
      grep -vE 'network\.protocol-handler\.(external|expose|warn-external)\.afirma|GrxFirma: permitir.*afirma://' "${user_js}" > "${tmp}" || true
      install -m 644 "${tmp}" "${user_js}"
      rm -f "${tmp}"
    done < <(find "${root}" -mindepth 2 -maxdepth 2 -name user.js -type f -print0)
  done
  if have_cmd update-desktop-database; then
    update-desktop-database "${TARGET_HOME}/.local/share/applications" >/dev/null 2>&1 || true
  fi
  log "$(grx_message MSG_005)"
}

is_snap_installed() {
  have_cmd snap && snap list "$1" >/dev/null 2>&1
}

is_flatpak_installed() {
  have_cmd flatpak && flatpak info "$1" >/dev/null 2>&1
}

write_chrome_manifest() {
  local target="$1"
  local name="$2"
  local origins="$3"
  mkdir -p "$(dirname "${target}")"
  local tmp
  tmp="$(mktemp "${target}.tmp.XXXXXXXX")"
  cat > "${tmp}" <<EOF
{
  "name": "${name}",
  "description": "GrxFirma Native Messaging Host",
  "path": "${BROWSER_BRIDGE}",
  "type": "stdio",
  "allowed_origins": [${origins}]
}
EOF
  chmod 0644 "${tmp}"
  mv -f -- "${tmp}" "${target}"
}

write_firefox_manifest() {
  local target="$1"
  local name="$2"
  local extensions="$3"
  mkdir -p "$(dirname "${target}")"
  local tmp
  tmp="$(mktemp "${target}.tmp.XXXXXXXX")"
  cat > "${tmp}" <<EOF
{
  "name": "${name}",
  "description": "GrxFirma Native Messaging Host",
  "path": "${BROWSER_BRIDGE}",
  "type": "stdio",
  "allowed_extensions": [${extensions}]
}
EOF
  chmod 0644 "${tmp}"
  mv -f -- "${tmp}" "${target}"
}

# Las versiones anteriores registraban los hosts com.dipgra.* y la extensión
# Firefox extension@dipgra.es, y algunas también el host de la extensión
# «portafirmas», que no forma parte de GrxFirma. Se retiran solo los
# manifiestos que apuntan a una instalación de GrxFirma; los de otros productos
# no se tocan.
remove_legacy_native_hosts() {
  local dir name manifest
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
    for name in com.dipgra.grxfirma com.dipgra.portafirmas io.github.aavidad.portafirmas; do
      manifest="${dir}/${name}.json"
      [[ -f "${manifest}" && ! -L "${manifest}" ]] || continue
      if grep -Eq '"path"[[:space:]]*:[[:space:]]*"[^"]*/grxfirma/[^"]*"' "${manifest}"; then
        rm -f -- "${manifest}"
        log "$(grx_message MSG_006 "${manifest}")"
      fi
    done
  done
}

remove_legacy_firefox_extension() {
  local profile="$1"
  local legacy="${profile}/extensions/extension@dipgra.es.xpi"
  if [[ -f "${legacy}" && ! -L "${legacy}" ]]; then
    rm -f -- "${legacy}"
    log "$(grx_message MSG_007 "${legacy}")"
  fi
}

primary_chrome_origins() {
  local origins="\"chrome-extension://${CHROME_EXT_ID}/\""
  if [[ -n "${EXTRA_CHROME_EXT_ID}" && "${EXTRA_CHROME_EXT_ID}" != "${CHROME_EXT_ID}" ]]; then
    origins+=",\"chrome-extension://${EXTRA_CHROME_EXT_ID}/\""
  fi
  printf '%s' "${origins}"
}

firefox_xpi_is_signed() {
  [[ -f "${FIREFOX_XPI}" && -f "${FIREFOX_METADATA}" ]] || return 1
  grep -Eq '"signed"[[:space:]]*:[[:space:]]*true' "${FIREFOX_METADATA}" || return 1
  local expected actual
  expected="$(sed -nE 's/.*"xpi_sha256"[[:space:]]*:[[:space:]]*"([0-9a-fA-F]{64})".*/\1/p' "${FIREFOX_METADATA}" | head -n 1)"
  [[ -n "${expected}" ]] || return 1
  actual="$(sha256sum "${FIREFOX_XPI}" | awk '{print $1}')"
  [[ "${actual,,}" == "${expected,,}" ]]
}

install_chromium_family_manifests() {
  local origins
  origins="$(primary_chrome_origins)"

  if have_cmd google-chrome || have_cmd google-chrome-stable || [[ -d /opt/google/chrome ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/google-chrome/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/google-chrome/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    log "$(grx_message MSG_008)"
  fi

  if have_cmd chromium || have_cmd chromium-browser || is_snap_installed chromium || is_flatpak_installed org.chromium.Chromium; then
    write_chrome_manifest "${TARGET_HOME}/.config/chromium/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/chromium/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    if is_snap_installed chromium || is_flatpak_installed org.chromium.Chromium; then
      log "$(grx_message MSG_009)"
    fi
    log "$(grx_message MSG_010)"
  fi

  if have_cmd microsoft-edge || have_cmd microsoft-edge-stable || [[ -d /opt/microsoft/msedge ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    log "$(grx_message MSG_011)"
  fi

  if have_cmd brave-browser || is_flatpak_installed com.brave.Browser || [[ -d /opt/brave.com/brave ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    if is_flatpak_installed com.brave.Browser; then
      log "$(grx_message MSG_012)"
    fi
    log "$(grx_message MSG_013)"
  fi

  if have_cmd vivaldi || have_cmd vivaldi-stable || [[ -d /opt/vivaldi ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    log "$(grx_message MSG_014)"
  fi
  if is_snap_installed vivaldi; then
    log "$(grx_message MSG_015)"
  fi

  if have_cmd opera || have_cmd opera-stable || is_flatpak_installed com.opera.Opera || [[ -d /usr/lib/x86_64-linux-gnu/opera || -d /opt/opera ]]; then
    # Opera documenta el fallback de Chrome para Native Messaging. Conservamos
    # además sus perfiles habituales para builds Chromium que los consulten.
    write_chrome_manifest "${TARGET_HOME}/.config/opera/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/opera/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    if is_flatpak_installed com.opera.Opera; then
      log "$(grx_message MSG_016)"
    fi
    log "$(grx_message MSG_017)"
  fi
}

configure_firefox_profile() {
  local profile="$1"
  [[ -d "${profile}" ]] || return 0

  mkdir -p "${profile}/extensions"
  remove_legacy_firefox_extension "${profile}"
  if [[ -f "${FIREFOX_XPI}" ]] && firefox_xpi_is_signed; then
    install -m 644 "${FIREFOX_XPI}" "${profile}/extensions/${FIREFOX_EXT_ID}.xpi" || true
  fi

  local user_js="${profile}/user.js"
  local tmp="${user_js}.tmp.$$"
  if [[ -f "${user_js}" ]]; then
    grep -vE 'network\.protocol-handler\.(external|expose|warn-external)\.afirma|GrxFirma: permitir.*afirma://' "${user_js}" > "${tmp}" || true
  else
    : > "${tmp}"
  fi
  cat >> "${tmp}" <<'EOF'
// GrxFirma: permitir delegación afirma:// al manejador externo registrado en XDG.
user_pref("network.protocol-handler.external.afirma", true);
user_pref("network.protocol-handler.expose.afirma", false);
user_pref("network.protocol-handler.warn-external.afirma", true);
EOF
  install -m 644 "${tmp}" "${user_js}"
  rm -f "${tmp}"

  local handlers_json="${profile}/handlers.json"
  if [[ -f "${handlers_json}" ]] && have_cmd python3; then
    python3 - "${handlers_json}" <<'PY' || true
import json
import os
import sys

path = sys.argv[1]
try:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    sys.exit(0)

schemes = data.setdefault("schemes", {})
entry = schemes.get("afirma")
if isinstance(entry, dict):
    entry.pop("handlers", None)
    entry["action"] = 4
else:
    schemes["afirma"] = {"action": 4}

tmp = f"{path}.tmp.{os.getpid()}"
with open(tmp, "w", encoding="utf-8") as fh:
    json.dump(data, fh, ensure_ascii=False, separators=(",", ":"))
os.replace(tmp, path)
PY
  fi
  log "$(grx_message MSG_018 "${profile}")"
}

install_firefox_integration() {
  local has_firefox=1
  if have_cmd firefox || have_cmd firefox-esr || is_snap_installed firefox || is_flatpak_installed org.mozilla.firefox || [[ -d "${TARGET_HOME}/.mozilla/firefox" || -d "${TARGET_HOME}/snap/firefox/common/.mozilla/firefox" || -d "${TARGET_HOME}/.var/app/org.mozilla.firefox/.mozilla/firefox" ]]; then
    has_firefox=0
  fi
  [[ "${has_firefox}" == "0" ]] || return 0

  if [[ -f "${FIREFOX_XPI}" ]] && ! firefox_xpi_is_signed; then
    log "$(grx_message FIREFOX_UNSIGNED)"
  fi

  local nm_dirs=(
    "${TARGET_HOME}/.mozilla/native-messaging-hosts"
  )
  # Firefox confinado usa el portal WebExtensions para leer el manifiesto del
  # host. No se escriben copias modificables dentro del sandbox.

  local dir
  for dir in "${nm_dirs[@]}"; do
    write_firefox_manifest "${dir}/com.grxfirma.native.json" "com.grxfirma.native" "\"${FIREFOX_EXT_ID}\""
    write_firefox_manifest "${dir}/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "\"${FIREFOX_EXT_ID}\""
  done

  local roots=(
    "${TARGET_HOME}/.mozilla/firefox"
    "${TARGET_HOME}/snap/firefox/common/.mozilla/firefox"
    "${TARGET_HOME}/.var/app/org.mozilla.firefox/.mozilla/firefox"
  )
  local root profile
  for root in "${roots[@]}"; do
    [[ -d "${root}" ]] || continue
    while IFS= read -r -d '' profile; do
      configure_firefox_profile "${profile}"
    done < <(find "${root}" -mindepth 1 -maxdepth 1 -type d \( -exec test -f "{}/prefs.js" ';' -o -exec test -f "{}/handlers.json" ';' \) -print0)
  done
  log "$(grx_message MSG_019)"
}

register_afirma_scheme() {
  # Se decide una vez, aquí, para que los avisos no se pierdan en un subproceso.
  afirma_resolve_handler
  sanitize_afirma_scheme_handlers
  afirma_save_requested_preference
  if have_cmd update-desktop-database; then
    update-desktop-database "${TARGET_HOME}/.local/share/applications" >/dev/null 2>&1 || true
  fi
  if have_cmd xdg-mime && ! afirma_prefers_autofirma; then
    xdg-mime default "${DESKTOP_ID}" x-scheme-handler/afirma || true
  fi
}

if [[ -n "${REQUESTED_AFIRMA_HANDLER}" ]] && ! afirma_valid_choice "${REQUESTED_AFIRMA_HANDLER}"; then
  grx_message MSG_020 "${REQUESTED_AFIRMA_HANDLER}" >&2
  exit 2
fi

case "${1:-}" in
  --detect-afirma-handler)
    afirma_detect_handler
    ;;
  "")
    remove_legacy_native_hosts
    install_chromium_family_manifests
    install_firefox_integration
    register_afirma_scheme
    ;;
  --restore-afirma-handler)
    restore_afirma_scheme_handlers
    ;;
  *)
    grx_message MSG_021 >&2
    exit 2
    ;;
esac
