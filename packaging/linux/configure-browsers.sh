#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

TARGET_HOME="${GRXFIRMA_TARGET_HOME:-${HOME}}"
DESKTOP_ID="${GRXFIRMA_DESKTOP_ID:-grxfirma.desktop}"
BROWSER_BRIDGE="${GRXFIRMA_BROWSER_BRIDGE:-/usr/lib/grxfirma/bin/browser-bridge.sh}"
FIREFOX_XPI="${GRXFIRMA_FIREFOX_XPI:-/usr/lib/grxfirma/extensions/dipgra-extension-firefox.xpi}"
FIREFOX_METADATA="${GRXFIRMA_FIREFOX_METADATA:-/usr/lib/grxfirma/extensions/dipgra-extension-firefox.metadata.json}"
FIREFOX_EXT_ID="${FIREFOX_EXT_ID:-extension@dipgra.es}"
CHROME_EXT_ID="${CHROME_EXT_ID:-pkefjandjcgdmhoonmhnllikibobijgg}"
EXTRA_CHROME_EXT_ID="${EXTRA_CHROME_EXT_ID:-}"

log() {
  printf 'grxfirma browsers: %s\n' "$*"
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
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

# Uso: afirma_handlers_py set|restore <copia> <desktop_id> <mimeapps>...
afirma_handlers_py() {
  python3 - "$@" <<'PY'
import configparser
import json
import os
import sys

mode, backup_path, desktop_id, *paths = sys.argv[1:]
schemes = ("x-scheme-handler/afirma", "x-scheme-handler/afirmav2")
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
                if entry[scheme]:
                    previous_handlers.add(entry[scheme])
            backup[path] = entry
        if not parser.has_section(section):
            parser.add_section(section)
        for scheme in schemes:
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
  local files=() previous
  mapfile -t files < <(mimeapps_files)
  if ! have_cmd python3; then
    local file
    for file in "${files[@]}"; do
      mkdir -p "$(dirname "${file}")"
      {
        printf '\n[Default Applications]\n'
        printf 'x-scheme-handler/afirma=%s\n' "${DESKTOP_ID}"
        printf 'x-scheme-handler/afirmav2=%s\n' "${DESKTOP_ID}"
      } >> "${file}"
    done
    log "Aviso: GrxFirma abrirá ahora los enlaces afirma://. Sin python3 no se ha podido guardar qué aplicación los abría antes (por ejemplo AutoFirma)."
    return 0
  fi
  previous="$(afirma_handlers_py set "${HANDLER_BACKUP}" "${DESKTOP_ID}" "${files[@]}")"
  if [[ -n "${previous}" ]]; then
    log "Aviso: los enlaces afirma:// los abría ${previous//$'\n'/, }. Desde ahora los abre GrxFirma. Al desinstalar GrxFirma se devolverán a esa aplicación."
  fi
}

restore_afirma_scheme_handlers() {
  local files=()
  mapfile -t files < <(mimeapps_files)
  if have_cmd python3; then
    afirma_handlers_py restore "${HANDLER_BACKUP}" "${DESKTOP_ID}" "${files[@]}"
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
  log "Manejador afirma:// devuelto a la aplicación anterior"
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
    write_chrome_manifest "${TARGET_HOME}/.config/google-chrome/NativeMessagingHosts/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/google-chrome/NativeMessagingHosts/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/\",\"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/\""
    log "Chrome detectado/configurado"
  fi

  if have_cmd chromium || have_cmd chromium-browser || is_snap_installed chromium || is_flatpak_installed org.chromium.Chromium; then
    write_chrome_manifest "${TARGET_HOME}/.config/chromium/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/chromium/NativeMessagingHosts/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/chromium/NativeMessagingHosts/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/\",\"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/\""
    if is_snap_installed chromium || is_flatpak_installed org.chromium.Chromium; then
      log "Chromium confinado detectado: Native Messaging requiere un portal o despliegue permitido por el paquete"
    fi
    log "Chromium detectado/configurado"
  fi

  if have_cmd microsoft-edge || have_cmd microsoft-edge-stable || [[ -d /opt/microsoft/msedge ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/\",\"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/\""
    log "Edge detectado/configurado"
  fi

  if have_cmd brave-browser || is_flatpak_installed com.brave.Browser || [[ -d /opt/brave.com/brave ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/\",\"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/\""
    if is_flatpak_installed com.brave.Browser; then
      log "Brave Flatpak detectado: Native Messaging requiere un portal o despliegue permitido por el paquete"
    fi
    log "Brave detectado/configurado"
  fi

  if have_cmd vivaldi || have_cmd vivaldi-stable || [[ -d /opt/vivaldi ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/\",\"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/\""
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/\",\"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/\""
    log "Vivaldi detectado/configurado"
  fi
  if is_snap_installed vivaldi; then
    log "Vivaldi Snap detectado: esa distribucion no admite Native Messaging"
  fi

  if have_cmd opera || have_cmd opera-stable || is_flatpak_installed com.opera.Opera || [[ -d /usr/lib/x86_64-linux-gnu/opera || -d /opt/opera ]]; then
    # Opera documenta el fallback de Chrome para Native Messaging. Conservamos
    # además sus perfiles habituales para builds Chromium que los consulten.
    write_chrome_manifest "${TARGET_HOME}/.config/opera/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/opera/NativeMessagingHosts/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/opera/NativeMessagingHosts/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/\",\"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/\""
    if is_flatpak_installed com.opera.Opera; then
      log "Opera Flatpak detectado: el host externo depende de los permisos de su paquete"
    fi
    log "Opera detectado/configurado mediante manifiestos Chromium"
  fi
}

configure_firefox_profile() {
  local profile="$1"
  [[ -d "${profile}" ]] || return 0

  mkdir -p "${profile}/extensions"
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
  log "Perfil Firefox configurado: ${profile}"
}

install_firefox_integration() {
  local has_firefox=1
  if have_cmd firefox || have_cmd firefox-esr || is_snap_installed firefox || is_flatpak_installed org.mozilla.firefox || [[ -d "${TARGET_HOME}/.mozilla/firefox" || -d "${TARGET_HOME}/snap/firefox/common/.mozilla/firefox" || -d "${TARGET_HOME}/.var/app/org.mozilla.firefox/.mozilla/firefox" ]]; then
    has_firefox=0
  fi
  [[ "${has_firefox}" == "0" ]] || return 0

  if [[ -f "${FIREFOX_XPI}" ]] && ! firefox_xpi_is_signed; then
    log "XPI Firefox sin firma AMO: se configura el protocolo, pero no se instala la extension estable"
  fi

  local nm_dirs=(
    "${TARGET_HOME}/.mozilla/native-messaging-hosts"
  )
  # Firefox confinado usa el portal WebExtensions para leer el manifiesto del
  # host. No se escriben copias modificables dentro del sandbox.

  local dir
  for dir in "${nm_dirs[@]}"; do
    write_firefox_manifest "${dir}/com.grxfirma.native.json" "com.grxfirma.native" "\"${FIREFOX_EXT_ID}\""
    write_firefox_manifest "${dir}/com.dipgra.grxfirma.json" "com.dipgra.grxfirma" "\"${FIREFOX_EXT_ID}\""
    write_firefox_manifest "${dir}/com.dipgra.portafirmas.json" "com.dipgra.portafirmas" "\"portafirmas@dipgra.es\""
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
  log "Firefox detectado/configurado"
}

register_afirma_scheme() {
  sanitize_afirma_scheme_handlers
  if have_cmd update-desktop-database; then
    update-desktop-database "${TARGET_HOME}/.local/share/applications" >/dev/null 2>&1 || true
  fi
  if have_cmd xdg-mime; then
    xdg-mime default "${DESKTOP_ID}" x-scheme-handler/afirma || true
  fi
}

case "${1:-}" in
  "")
    install_chromium_family_manifests
    install_firefox_integration
    register_afirma_scheme
    ;;
  --restore-afirma-handler)
    restore_afirma_scheme_handlers
    ;;
  *)
    echo "uso: configure-browsers.sh [--restore-afirma-handler]" >&2
    exit 2
    ;;
esac
