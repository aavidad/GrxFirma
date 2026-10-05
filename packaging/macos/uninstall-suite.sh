#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
umask 077

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODE="user"

usage() {
  cat <<'EOF'
Uso:
  uninstall-suite.sh
  sudo uninstall-suite.sh --system

Sin opciones elimina la instalacion y los registros del usuario actual.
--system elimina el payload instalado por el PKG; debe ejecutarse como root.
EOF
}

case "${1:-}" in
  "")
    ;;
  --system)
    MODE="system"
    ;;
  --help|-h)
    usage
    exit 0
    ;;
  *)
    echo "error: argumento no soportado: ${1}" >&2
    usage >&2
    exit 2
    ;;
esac
if [[ "$#" -gt 1 ]]; then
  echo "error: demasiados argumentos" >&2
  exit 2
fi

remove_exact_tree() {
  local target="$1"
  if [[ -d "${target}" && ! -L "${target}" ]]; then
    rm -rf -- "${target}"
  else
    rm -f -- "${target}"
  fi
}

has_symlink_between() {
  local current="$1"
  local boundary="$2"
  while [[ "${current}" != "${boundary}" ]]; do
    [[ -L "${current}" ]] && return 0
    current="$(dirname "${current}")"
    [[ "${current}" == "${boundary}" || "${current}" == "${boundary}/"* ]] || return 0
  done
  return 1
}

remove_user_tree() {
  local target="$1"
  if [[ "${target}" != "${HOME}/"* || "${target}" == "${HOME}" ]]; then
    echo "error: se rechazo un objetivo fuera de HOME: ${target}" >&2
    return 1
  fi
  if has_symlink_between "$(dirname "${target}")" "${HOME}"; then
    echo "Aviso: no se elimina una ruta con ancestros simbolicos: ${target}" >&2
    return 0
  fi
  remove_exact_tree "${target}"
}

remove_manifest_if_owned() {
  local target="$1"
  local expected_binary="$2"
  if [[ ! -e "${target}" && ! -L "${target}" ]]; then
    return 0
  fi
  if has_symlink_between "$(dirname "${target}")" "${HOME}"; then
    echo "Aviso: no se elimina un manifiesto bajo un directorio simbolico: ${target}" >&2
    return 0
  fi
  if [[ -f "${target}" ]] && grep -Fq -- "${expected_binary}" "${target}"; then
    rm -f -- "${target}"
  else
    echo "Aviso: se conserva un manifiesto modificado o ajeno: ${target}" >&2
  fi
}

add_extension_id() {
  local candidate="$1"
  local existing
  [[ "${candidate}" =~ ^[a-p]{32}$ ]] || return 0
  for existing in "${extension_ids[@]}"; do
    [[ "${existing}" != "${candidate}" ]] || return 0
  done
  extension_ids+=("${candidate}")
}

uninstall_user() {
  if [[ -z "${HOME:-}" || "${HOME}" != /* || "${HOME}" == "/" || -L "${HOME}" ]]; then
    echo "error: HOME debe ser una ruta absoluta de usuario y no simbolica" >&2
    return 1
  fi

  local base="${HOME}/Library/Application Support/GrxFirma"
  local native_binary="${base}/NativeHost/grxfirma-nativehost"
  local afirma_app="${HOME}/Applications/GrxFirma AfirmaURI.app"
  local desktop_app="${HOME}/Applications/GrxFirma Desktop Qt.app"
  local lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
  local manifest_dir manifest_name external_dir extension_id

  if [[ -x "${lsregister}" ]]; then
    "${lsregister}" -u "${afirma_app}" >/dev/null 2>&1 || true
  fi

  manifest_dirs=(
    "${HOME}/Library/Application Support/Google/Chrome/NativeMessagingHosts"
    "${HOME}/Library/Application Support/Chromium/NativeMessagingHosts"
    "${HOME}/Library/Application Support/Microsoft Edge/NativeMessagingHosts"
    "${HOME}/Library/Application Support/BraveSoftware/Brave-Browser/NativeMessagingHosts"
    "${HOME}/Library/Application Support/Vivaldi/NativeMessagingHosts"
    "${HOME}/Library/Application Support/com.operasoftware.Opera/NativeMessagingHosts"
    "${HOME}/Library/Application Support/Mozilla/NativeMessagingHosts"
  )
  manifest_names=(
    "com.grxfirma.native.json"
    "io.github.aavidad.grxfirma.json"
    "io.github.aavidad.portafirmas.json"
    # Nombres de versiones anteriores.
    "com.dipgra.grxfirma.json"
    "com.dipgra.portafirmas.json"
  )
  for manifest_dir in "${manifest_dirs[@]}"; do
    for manifest_name in "${manifest_names[@]}"; do
      remove_manifest_if_owned "${manifest_dir}/${manifest_name}" "${native_binary}"
    done
  done

  extension_ids=("pkefjandjcgdmhoonmhnllikibobijgg")
  for id_file in \
    "${SELF_DIR}/extensions/grxfirma-extension-chromium.id" \
    "${base}/Extensions/chromium/grxfirma-extension-chromium.id" \
    "${base}/Extensions/chromium/dipgra-extension-chromium.id"; do
    if [[ -f "${id_file}" && ! -L "${id_file}" ]]; then
      add_extension_id "$(tr -d '\r\n' < "${id_file}")"
    fi
  done
  add_extension_id "${GRXFIRMA_CHROMIUM_EXTENSION_ID:-}"
  add_extension_id "${GRXFIRMA_EDGE_EXTENSION_ID:-}"

  external_dirs=(
    "${HOME}/Library/Application Support/Google/Chrome/External Extensions"
    "${HOME}/Library/Application Support/Chromium/External Extensions"
    "${HOME}/Library/Application Support/Microsoft Edge/External Extensions"
    "${HOME}/Library/Application Support/BraveSoftware/Brave-Browser/External Extensions"
    "${HOME}/Library/Application Support/Vivaldi/External Extensions"
    "${HOME}/Library/Application Support/com.operasoftware.Opera/External Extensions"
  )
  for external_dir in "${external_dirs[@]}"; do
    if has_symlink_between "${external_dir}" "${HOME}"; then
      echo "Aviso: no se limpia un directorio de extensiones simbolico: ${external_dir}" >&2
      continue
    fi
    for extension_id in "${extension_ids[@]}"; do
      rm -f -- "${external_dir}/${extension_id}.json"
    done
  done

  remove_firefox_extension_if_owned "${base}"
  remove_user_tree "${afirma_app}"
  remove_user_tree "${desktop_app}"
  remove_user_tree "${base}/CLI"
  remove_user_tree "${base}/NativeHost"
  remove_user_tree "${base}/Extensions"
  remove_user_tree "${base}/uninstall-suite.sh"
  rmdir -- "${base}" >/dev/null 2>&1 || true
  rmdir -- "${HOME}/Applications" >/dev/null 2>&1 || true

  echo "Suite de usuario desinstalada."
}

sha256_file() {
  local target="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "${target}" | awk '{print $1}'
  else
    sha256sum "${target}" | awk '{print $1}'
  fi
}

remove_firefox_extension_if_owned() {
  local base="$1"
  local candidate target profile xpi_name hash known owned
  local -a expected_hashes=()
  for candidate in \
    "${SELF_DIR}/extensions/grxfirma-extension-firefox.xpi" \
    "${base}/Extensions/firefox/grxfirma-extension-firefox.xpi" \
    "${base}/Extensions/firefox/dipgra-extension-firefox.xpi"; do
    if [[ -f "${candidate}" && ! -L "${candidate}" ]]; then
      expected_hashes+=("$(sha256_file "${candidate}")")
    fi
  done
  [[ "${#expected_hashes[@]}" -gt 0 ]] || return 0

  local profiles="${HOME}/Library/Application Support/Firefox/Profiles"
  [[ -d "${profiles}" && ! -L "${profiles}" ]] || return 0
  for profile in "${profiles}"/*; do
    [[ -d "${profile}" && ! -L "${profile}" ]] || continue
    # extension@dipgra.es es el ID de versiones anteriores.
    for xpi_name in "grxfirma@aavidad.github.io.xpi" "extension@dipgra.es.xpi"; do
      target="${profile}/extensions/${xpi_name}"
      [[ -f "${target}" && ! -L "${target}" ]] || continue
      if has_symlink_between "$(dirname "${target}")" "${HOME}"; then
        echo "Aviso: no se elimina una extension Firefox bajo una ruta simbolica: ${target}" >&2
        continue
      fi
      hash="$(sha256_file "${target}")"
      owned=0
      for known in "${expected_hashes[@]}"; do
        [[ "${hash}" != "${known}" ]] || owned=1
      done
      if [[ "${owned}" == "1" ]]; then
        rm -f -- "${target}"
        rmdir -- "$(dirname "${target}")" >/dev/null 2>&1 || true
      else
        echo "Aviso: se conserva una extension Firefox modificada: ${target}" >&2
      fi
    done
  done
}

validate_system_target() {
  local target="$1"
  local allow_final_symlink="$2"
  local current
  [[ "${target}" == /* && "${target}" != "/" ]] || {
    echo "error: objetivo de sistema inseguro: ${target}" >&2
    return 1
  }
  current="$(dirname "${target}")"
  while [[ "${current}" != "/" ]]; do
    if [[ -L "${current}" ]]; then
      echo "error: ancestro simbolico en objetivo de sistema: ${current}" >&2
      return 1
    fi
    current="$(dirname "${current}")"
  done
  if [[ "${allow_final_symlink}" != "1" && -L "${target}" ]]; then
    echo "error: objetivo de sistema inesperadamente simbolico: ${target}" >&2
    return 1
  fi
}

uninstall_system() {
  if [[ "${EUID}" -ne 0 ]]; then
    echo "error: --system requiere privilegios de root" >&2
    return 1
  fi

  local support="/Library/Application Support/GrxFirma"
  local cli_link="/usr/local/bin/grxfirma"
  local cli_target="/Library/Application Support/GrxFirma/grxfirma"
  local afirma_app="/Applications/GrxFirma AfirmaURI.app"
  local desktop_app="/Applications/GrxFirma Desktop Qt.app"
  local launch_agent="/Library/LaunchAgents/es.dipgra.grxfirma.register-user.plist"
  local lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
  local target

  # Preflight completo antes de cualquier borrado. El unico objetivo que puede
  # ser simbolico es el enlace CLI esperado; ninguno de sus ancestros puede
  # serlo. De este modo un /Applications o /Library redirigido no convierte la
  # desinstalacion con root en un borrado fuera del volumen previsto.
  system_targets=(
    "${support}"
    "${cli_link}"
    "${afirma_app}"
    "${desktop_app}"
    "${launch_agent}"
  )
  for target in "${system_targets[@]}"; do
    if [[ "${target}" == "${cli_link}" ]]; then
      validate_system_target "${target}" 1
    else
      validate_system_target "${target}" 0
    fi
  done

  if [[ -x "${lsregister}" ]]; then
    "${lsregister}" -u "${afirma_app}" >/dev/null 2>&1 || true
  fi
  if [[ -L "${cli_link}" ]]; then
    if [[ "$(readlink "${cli_link}")" == "${cli_target}" ]]; then
      rm -f -- "${cli_link}"
    else
      echo "Aviso: se conserva un enlace CLI que apunta a otro producto: ${cli_link}" >&2
    fi
  elif [[ -e "${cli_link}" ]]; then
    echo "Aviso: se conserva una CLI no simbolica: ${cli_link}" >&2
  fi

  remove_exact_tree "${afirma_app}"
  remove_exact_tree "${desktop_app}"
  rm -f -- "${launch_agent}"
  remove_exact_tree "${support}"
  echo "Payload PKG de GrxFirma desinstalado."
}

if [[ "${GRXFIRMA_UNINSTALL_LIBRARY_ONLY:-0}" == "1" ]]; then
  if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
    return 0
  fi
  exit 0
fi

if [[ "${MODE}" == "system" ]]; then
  uninstall_system
else
  uninstall_user
fi
