#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
umask 077

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${HOME:-}" || "${HOME}" != /* || "${HOME}" == "/" || -L "${HOME}" ]]; then
  echo "error: HOME debe ser una ruta absoluta de usuario y no simbolica" >&2
  exit 1
fi
BASE_DIR="${HOME}/Library/Application Support/GrxFirma"
AFIRMA_APP="${HOME}/Applications/GrxFirma AfirmaURI.app"
DESKTOP_APP="${HOME}/Applications/GrxFirma Desktop Qt.app"

required=(
  "${SELF_DIR}/grxfirma"
  "${SELF_DIR}/grxfirma-nativehost"
  "${SELF_DIR}/install-nativehost.sh"
  "${SELF_DIR}/install-afirmauri.sh"
  "${SELF_DIR}/install-desktop-qml.sh"
  "${SELF_DIR}/uninstall-suite.sh"
  "${SELF_DIR}/GrxFirma AfirmaURI.app/Contents/Info.plist"
  "${SELF_DIR}/GrxFirma AfirmaURI.app/Contents/MacOS/grxfirma-afirmauri"
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/Info.plist"
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml"
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui"
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma"
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/Resources/VERSION.txt"
)
for path in "${required[@]}"; do
  if [[ ! -e "${path}" || -L "${path}" ]]; then
    echo "error: paquete macOS incompleto, falta ${path}" >&2
    exit 1
  fi
done
for executable in \
  "${SELF_DIR}/grxfirma" \
  "${SELF_DIR}/grxfirma-nativehost" \
  "${SELF_DIR}/install-nativehost.sh" \
  "${SELF_DIR}/install-afirmauri.sh" \
  "${SELF_DIR}/install-desktop-qml.sh" \
  "${SELF_DIR}/uninstall-suite.sh" \
  "${SELF_DIR}/GrxFirma AfirmaURI.app/Contents/MacOS/grxfirma-afirmauri" \
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml" \
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui" \
  "${SELF_DIR}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma"; do
  if [[ ! -f "${executable}" || ! -x "${executable}" || -L "${executable}" ]]; then
    echo "error: paquete macOS incompleto, ejecutable no valido: ${executable}" >&2
    exit 1
  fi
done

declare -a transaction_targets=()
declare -a transaction_backups=()
declare -a transaction_existed=()
declare -a missing_parents=()
transaction_active=0
transaction_dir=""

is_safe_user_target() {
  local target="$1"
  [[ "${target}" == "${HOME}/"* && "${target}" != "${HOME}" ]]
}

add_unique_value() {
  local array_name="$1"
  local value="$2"
  local existing
  case "${array_name}" in
    transaction_targets)
      for existing in "${transaction_targets[@]}"; do
        [[ "${existing}" != "${value}" ]] || return 0
      done
      transaction_targets+=("${value}")
      ;;
    missing_parents)
      for existing in "${missing_parents[@]}"; do
        [[ "${existing}" != "${value}" ]] || return 0
      done
      missing_parents+=("${value}")
      ;;
    *)
      echo "error: array transaccional no soportado: ${array_name}" >&2
      return 1
      ;;
  esac
}

ensure_no_symlink_ancestors() {
  local target="$1"
  local current="${target}"
  while [[ "${current}" != "${HOME}" ]]; do
    if [[ -L "${current}" ]]; then
      echo "error: la ruta de instalacion contiene un enlace simbolico: ${current}" >&2
      return 1
    fi
    current="$(dirname "${current}")"
    is_safe_user_target "${current}" || [[ "${current}" == "${HOME}" ]] || {
      echo "error: ruta de instalacion fuera de HOME: ${target}" >&2
      return 1
    }
  done
}

record_missing_parents() {
  local current
  current="$(dirname "$1")"
  while [[ "${current}" != "${HOME}" ]]; do
    if [[ ! -e "${current}" && ! -L "${current}" ]]; then
      add_unique_value missing_parents "${current}"
    fi
    current="$(dirname "${current}")"
  done
}

add_transaction_target() {
  local target="$1"
  is_safe_user_target "${target}" || {
    echo "error: objetivo transaccional inseguro: ${target}" >&2
    return 1
  }
  ensure_no_symlink_ancestors "${target}"
  add_unique_value transaction_targets "${target}"
  record_missing_parents "${target}"
}

copy_preserving_path() {
  local source="$1"
  local destination="$2"
  if [[ -L "${source}" ]]; then
    ln -s "$(readlink "${source}")" "${destination}"
  elif [[ -d "${source}" ]]; then
    if command -v ditto >/dev/null 2>&1; then
      ditto "${source}" "${destination}"
    else
      cp -a "${source}" "${destination}"
    fi
  else
    cp -p "${source}" "${destination}"
  fi
}

snapshot_transaction() {
  transaction_dir="$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-suite-install.XXXXXX")"
  chmod 700 "${transaction_dir}"
  local index target backup
  for index in "${!transaction_targets[@]}"; do
    target="${transaction_targets[index]}"
    backup="${transaction_dir}/${index}"
    transaction_backups[index]="${backup}"
    if [[ -e "${target}" || -L "${target}" ]]; then
      copy_preserving_path "${target}" "${backup}"
      transaction_existed[index]=1
    else
      transaction_existed[index]=0
    fi
  done
  transaction_active=1
}

remove_user_target() {
  local target="$1"
  is_safe_user_target "${target}" || return 1
  if [[ -d "${target}" && ! -L "${target}" ]]; then
    rm -rf -- "${target}"
  else
    rm -f -- "${target}"
  fi
}

refresh_launchservices_after_rollback() {
  local lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
  [[ -x "${lsregister}" ]] || return 0
  if [[ -d "${AFIRMA_APP}" && ! -L "${AFIRMA_APP}" ]]; then
    "${lsregister}" -f "${AFIRMA_APP}" >/dev/null 2>&1 || true
  else
    "${lsregister}" -u "${AFIRMA_APP}" >/dev/null 2>&1 || true
  fi
}

rollback_transaction() {
  local rollback_failed=0
  local index target backup pass parent
  for ((index=${#transaction_targets[@]} - 1; index >= 0; index--)); do
    target="${transaction_targets[index]}"
    backup="${transaction_backups[index]}"
    if ! remove_user_target "${target}"; then
      echo "error: no se pudo limpiar durante rollback: ${target}" >&2
      rollback_failed=1
      continue
    fi
    if [[ "${transaction_existed[index]}" == "1" ]]; then
      if ! mkdir -p "$(dirname "${target}")" ||
         ! mv "${backup}" "${target}"; then
        echo "error: no se pudo restaurar durante rollback: ${target}" >&2
        rollback_failed=1
      fi
    fi
  done

  # Solo se eliminan ancestros que no existian al comenzar, y exclusivamente
  # si siguen vacios. Varias pasadas resuelven arboles compartidos.
  for ((pass=0; pass <= ${#missing_parents[@]}; pass++)); do
    for parent in "${missing_parents[@]}"; do
      rmdir -- "${parent}" >/dev/null 2>&1 || true
    done
  done
  refresh_launchservices_after_rollback
  return "${rollback_failed}"
}

finish_transaction() {
  local status="${1:-$?}"
  local rollback_status=0
  trap - EXIT INT TERM
  if [[ "${transaction_active}" == "1" && "${status}" -ne 0 ]]; then
    echo "error: la instalacion fallo; restaurando el estado anterior" >&2
    rollback_transaction || rollback_status=$?
  fi
  if [[ -n "${transaction_dir}" && -d "${transaction_dir}" ]]; then
    rm -rf -- "${transaction_dir}"
  fi
  if [[ "${rollback_status}" -ne 0 ]]; then
    echo "error: el rollback no pudo completarse por entero" >&2
    exit 1
  fi
  exit "${status}"
}
trap 'finish_transaction "$?"' EXIT
trap 'finish_transaction 130' INT
trap 'finish_transaction 143' TERM

add_transaction_target "${BASE_DIR}"
add_transaction_target "${AFIRMA_APP}"
add_transaction_target "${DESKTOP_APP}"

native_manifest_dirs=(
  "${HOME}/Library/Application Support/Google/Chrome/NativeMessagingHosts"
  "${HOME}/Library/Application Support/Chromium/NativeMessagingHosts"
  "${HOME}/Library/Application Support/Microsoft Edge/NativeMessagingHosts"
  "${HOME}/Library/Application Support/BraveSoftware/Brave-Browser/NativeMessagingHosts"
  "${HOME}/Library/Application Support/Vivaldi/NativeMessagingHosts"
  "${HOME}/Library/Application Support/com.operasoftware.Opera/NativeMessagingHosts"
  "${HOME}/Library/Application Support/Mozilla/NativeMessagingHosts"
)
external_extension_dirs=(
  "${HOME}/Library/Application Support/Google/Chrome/External Extensions"
  "${HOME}/Library/Application Support/Chromium/External Extensions"
  "${HOME}/Library/Application Support/Microsoft Edge/External Extensions"
  "${HOME}/Library/Application Support/BraveSoftware/Brave-Browser/External Extensions"
  "${HOME}/Library/Application Support/Vivaldi/External Extensions"
  "${HOME}/Library/Application Support/com.operasoftware.Opera/External Extensions"
)
for target_dir in "${native_manifest_dirs[@]}" "${external_extension_dirs[@]}"; do
  add_transaction_target "${target_dir}"
done

firefox_profiles="${HOME}/Library/Application Support/Firefox/Profiles"
if [[ -d "${firefox_profiles}" && ! -L "${firefox_profiles}" ]]; then
  for profile in "${firefox_profiles}"/*; do
    [[ -d "${profile}" && ! -L "${profile}" && -f "${profile}/prefs.js" ]] || continue
    add_transaction_target "${profile}/extensions"
  done
fi

snapshot_transaction

mkdir -p "${BASE_DIR}/CLI"
chmod 700 "${BASE_DIR}" "${BASE_DIR}/CLI"
install -m 755 "${SELF_DIR}/grxfirma" "${BASE_DIR}/CLI/grxfirma"
install -m 755 "${SELF_DIR}/uninstall-suite.sh" "${BASE_DIR}/uninstall-suite.sh"

bash "${SELF_DIR}/install-nativehost.sh"
bash "${SELF_DIR}/install-afirmauri.sh"
bash "${SELF_DIR}/install-desktop-qml.sh"

transaction_active=0
echo "Suite instalada en: ${BASE_DIR}"
