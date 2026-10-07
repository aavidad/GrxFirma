#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

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

# Si la persona eligió AutoFirma en «Firmas desde los portales» y AutoFirma
# sigue instalado, afirma:// no se reasigna a GrxFirma al actualizar.
afirma_prefers_autofirma() {
  local pref="${TARGET_HOME}/.config/grxfirma/afirma-protocol-handler"
  local dir value
  local -a dirs
  [[ -f "${pref}" && ! -L "${pref}" ]] || return 1
  value="$(head -c 32 "${pref}" | tr -d '[:space:]')"
  [[ "${value}" == "autofirma" ]] || return 1
  IFS=: read -r -a dirs <<< "${SYSTEM_APP_DIRS}"
  for dir in "${TARGET_HOME}/.local/share/applications" "${dirs[@]}"; do
    if [[ -f "${dir}/afirma.desktop" || -f "${dir}/autofirma.desktop" ]]; then
      return 0
    fi
  done
  return 1
}

afirma_schemes() {
  if ! afirma_prefers_autofirma; then
    printf '%s\n' x-scheme-handler/afirma
  fi
  printf '%s\n' x-scheme-handler/afirmav2
}

ensure_mimeapps_default() {
  local file="$1"
  local -a schemes
  mapfile -t schemes < <(afirma_schemes)
  mkdir -p "$(dirname "${file}")"
  if have_cmd python3; then
    python3 - "$file" "${DESKTOP_ID}" "${schemes[@]}" <<'PY'
import configparser
import os
import sys

path, desktop_id, schemes = sys.argv[1], sys.argv[2], sys.argv[3:]
parser = configparser.RawConfigParser(strict=False, delimiters=("=",))
parser.optionxform = str
if os.path.exists(path):
    parser.read(path, encoding="utf-8")

if not parser.has_section("Default Applications"):
    parser.add_section("Default Applications")
for scheme in schemes:
    parser.set("Default Applications", scheme, desktop_id)

with open(path, "w", encoding="utf-8") as fh:
    parser.write(fh, space_around_delimiters=False)
PY
    return 0
  fi

  {
    printf '\n[Default Applications]\n'
    local scheme
    for scheme in "${schemes[@]}"; do
      printf '%s=%s\n' "${scheme}" "${DESKTOP_ID}"
    done
  } >> "${file}"
}

sanitize_afirma_scheme_handlers() {
  ensure_mimeapps_default "${TARGET_HOME}/.config/mimeapps.list"
  ensure_mimeapps_default "${TARGET_HOME}/.local/share/applications/mimeapps.list"
  if [[ -d "${TARGET_HOME}/snap/firefox/common" ]]; then
    ensure_mimeapps_default "${TARGET_HOME}/snap/firefox/common/.config/mimeapps.list"
  fi
  if [[ -d "${TARGET_HOME}/.var/app/org.mozilla.firefox" ]]; then
    ensure_mimeapps_default "${TARGET_HOME}/.var/app/org.mozilla.firefox/config/mimeapps.list"
  fi
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
        log "Host anterior retirado: ${manifest}"
      fi
    done
  done
}

remove_legacy_firefox_extension() {
  local profile="$1"
  local legacy="${profile}/extensions/extension@dipgra.es.xpi"
  if [[ -f "${legacy}" && ! -L "${legacy}" ]]; then
    rm -f -- "${legacy}"
    log "Extension Firefox anterior retirada: ${legacy}"
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
    log "Chrome detectado/configurado"
  fi

  if have_cmd chromium || have_cmd chromium-browser || is_snap_installed chromium || is_flatpak_installed org.chromium.Chromium; then
    write_chrome_manifest "${TARGET_HOME}/.config/chromium/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/chromium/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    if is_snap_installed chromium || is_flatpak_installed org.chromium.Chromium; then
      log "Chromium confinado detectado: Native Messaging requiere un portal o despliegue permitido por el paquete"
    fi
    log "Chromium detectado/configurado"
  fi

  if have_cmd microsoft-edge || have_cmd microsoft-edge-stable || [[ -d /opt/microsoft/msedge ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/microsoft-edge/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    log "Edge detectado/configurado"
  fi

  if have_cmd brave-browser || is_flatpak_installed com.brave.Browser || [[ -d /opt/brave.com/brave ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    if is_flatpak_installed com.brave.Browser; then
      log "Brave Flatpak detectado: Native Messaging requiere un portal o despliegue permitido por el paquete"
    fi
    log "Brave detectado/configurado"
  fi

  if have_cmd vivaldi || have_cmd vivaldi-stable || [[ -d /opt/vivaldi ]]; then
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/vivaldi-snapshot/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
    log "Vivaldi detectado/configurado"
  fi
  if is_snap_installed vivaldi; then
    log "Vivaldi Snap detectado: esa distribucion no admite Native Messaging"
  fi

  if have_cmd opera || have_cmd opera-stable || is_flatpak_installed com.opera.Opera || [[ -d /usr/lib/x86_64-linux-gnu/opera || -d /opt/opera ]]; then
    # Opera documenta el fallback de Chrome para Native Messaging. Conservamos
    # además sus perfiles habituales para builds Chromium que los consulten.
    write_chrome_manifest "${TARGET_HOME}/.config/opera/NativeMessagingHosts/com.grxfirma.native.json" "com.grxfirma.native" "${origins}"
    write_chrome_manifest "${TARGET_HOME}/.config/opera/NativeMessagingHosts/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${origins}"
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
  log "Firefox detectado/configurado"
}

register_afirma_scheme() {
  sanitize_afirma_scheme_handlers
  if have_cmd update-desktop-database; then
    update-desktop-database "${TARGET_HOME}/.local/share/applications" >/dev/null 2>&1 || true
  fi
  if have_cmd xdg-mime && ! afirma_prefers_autofirma; then
    xdg-mime default "${DESKTOP_ID}" x-scheme-handler/afirma || true
  fi
}

remove_legacy_native_hosts
install_chromium_family_manifests
install_firefox_integration
register_afirma_scheme
