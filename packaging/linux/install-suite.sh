#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Las variables SC2016 se escriben literalmente en env.sh para evaluarlas al usarlo.
# shellcheck disable=SC2016
set -euo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

USERBIN="${HOME}/.local/bin"
USERAPP="${HOME}/.local/share/applications"
USERLIBDIR="${HOME}/.local/lib/grxfirma/bin"
USERCFGDIR="${HOME}/.config/grxfirma/pkcs12"
USERMANDIR="${HOME}/.local/share/man/man1"
USERMAN7DIR="${HOME}/.local/share/man/man7"
CONFIG_DIR="${HOME}/.local/lib/grxfirma/config"
ENV_FILE="${CONFIG_DIR}/env.sh"
BASE_DIR="${HOME}/.local/lib/grxfirma"
USERGUIQMLDIR="${BASE_DIR}/gui-qml"
PACKAGE_VERSION_FILE="${SELF_DIR}/VERSION.txt"
PACKAGE_BUILD_INFO_FILE="${SELF_DIR}/BUILDINFO"
INSTALLED_VERSION_FILE="${BASE_DIR}/VERSION.txt"
INSTALLED_BUILD_INFO_FILE="${BASE_DIR}/BUILDINFO"
USEREXTDIR="${BASE_DIR}/extensions"

NM_CHROME="${HOME}/.config/google-chrome/NativeMessagingHosts"
NM_CHROMIUM="${HOME}/.config/chromium/NativeMessagingHosts"
NM_EDGE="${HOME}/.config/microsoft-edge/NativeMessagingHosts"
NM_BRAVE="${HOME}/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts"
NM_VIVALDI="${HOME}/.config/vivaldi/NativeMessagingHosts"
NM_OPERA="${HOME}/.config/opera/NativeMessagingHosts"
NM_FIREFOX="${HOME}/.mozilla/native-messaging-hosts"

FIREFOX_XPI="${SELF_DIR}/extensions/grxfirma-extension-firefox.xpi"
FIREFOX_METADATA="${SELF_DIR}/extensions/grxfirma-extension-firefox.metadata.json"
CHROMIUM_ZIP="${SELF_DIR}/extensions/grxfirma-extension-chromium.zip"
CHROMIUM_CRX="${SELF_DIR}/extensions/grxfirma-extension-chromium.crx"
CHROMIUM_CRX_ID_FILE="${SELF_DIR}/extensions/grxfirma-extension-chromium.id"

CHROME_EXT_ID="${CHROME_EXT_ID:-pkefjandjcgdmhoonmhnllikibobijgg}"
FIREFOX_EXT_ID="${FIREFOX_EXT_ID:-grxfirma@aavidad.github.io}"
EXTRA_CHROME_EXT_ID=""
if [[ -f "${CHROMIUM_CRX_ID_FILE}" ]]; then
  EXTRA_CHROME_EXT_ID="$(tr -d '\r\n' < "${CHROMIUM_CRX_ID_FILE}")"
fi

for bin in grxfirma grxfirma-gui grxfirma-desktop grxfirma-afirmauri grxfirma-nativehost grxfirma-pkcs11-worker; do
  if [[ ! -f "${SELF_DIR}/${bin}" ]]; then
    echo "error: no se encuentra ${bin} junto al instalador" >&2
    exit 1
  fi
done

if [[ ! -x "${SELF_DIR}/check-runtime-dependencies.sh" ]]; then
  echo "error: no se encuentra el verificador de dependencias runtime junto al instalador" >&2
  exit 1
fi
"${SELF_DIR}/check-runtime-dependencies.sh" --check-bundle "${SELF_DIR}"

package_version="desconocida"
if [[ -f "${PACKAGE_VERSION_FILE}" ]]; then
  package_version="$(tr -d '\r\n' < "${PACKAGE_VERSION_FILE}")"
fi

installed_version=""
if [[ -f "${INSTALLED_VERSION_FILE}" ]]; then
  installed_version="$(tr -d '\r\n' < "${INSTALLED_VERSION_FILE}")"
fi

if [[ -z "${installed_version}" ]]; then
  echo "Instalando GrxFirma ${package_version}..."
elif [[ "${installed_version}" == "${package_version}" ]]; then
  echo "Reinstalando GrxFirma ${package_version}..."
else
  echo "Actualizando GrxFirma: ${installed_version} -> ${package_version}..."
fi

mkdir -p "${USERBIN}" "${USERAPP}" "${USERLIBDIR}" "${USERCFGDIR}" "${USERMANDIR}" "${USERMAN7DIR}" "${CONFIG_DIR}" "${USEREXTDIR}/firefox" "${USEREXTDIR}/chromium"
chmod 700 "${CONFIG_DIR}"

install -m 755 "${SELF_DIR}/grxfirma" "${USERBIN}/grxfirma"
install -m 755 "${SELF_DIR}/grxfirma-gui" "${USERBIN}/grxfirma-gui"
install -m 755 "${SELF_DIR}/grxfirma-desktop" "${USERBIN}/grxfirma-desktop"
if [[ -f "${SELF_DIR}/grxfirma-gui-qml" ]]; then
  install -m 755 "${SELF_DIR}/grxfirma-gui-qml" "${USERBIN}/grxfirma-gui-qml"
else
  rm -f "${USERBIN}/grxfirma-gui-qml"
fi
if [[ -d "${SELF_DIR}/qml" || -d "${SELF_DIR}/assets" || -d "${SELF_DIR}/help" ]]; then
  rm -rf "${USERGUIQMLDIR}"
  mkdir -p "${USERGUIQMLDIR}"
  if [[ -d "${SELF_DIR}/qml" ]]; then
    cp -a "${SELF_DIR}/qml" "${USERGUIQMLDIR}/"
  fi
  if [[ -d "${SELF_DIR}/assets" ]]; then
    cp -a "${SELF_DIR}/assets" "${USERGUIQMLDIR}/"
  fi
  if [[ -d "${SELF_DIR}/help" ]]; then
    cp -a "${SELF_DIR}/help" "${USERGUIQMLDIR}/"
  fi
else
  rm -rf "${USERGUIQMLDIR}"
fi
install -m 755 "${SELF_DIR}/grxfirma-afirmauri" "${USERBIN}/grxfirma-afirmauri"
install -m 755 "${SELF_DIR}/grxfirma-nativehost" "${USERLIBDIR}/grxfirma-nativehost"
install -m 755 "${SELF_DIR}/grxfirma-pkcs11-worker" "${USERLIBDIR}/grxfirma-pkcs11-worker"

if [[ ! -f "${ENV_FILE}" ]]; then
  cat > "${ENV_FILE}" <<'EOF'
#!/usr/bin/env bash
export GRXFIRMA_CERTS_DIR="${GRXFIRMA_CERTS_DIR:-$HOME/.config/grxfirma/certs}"
export GRXFIRMA_PKCS12_DIR="${GRXFIRMA_PKCS12_DIR:-$HOME/.config/grxfirma/pkcs12}"
export GRXFIRMA_PKCS12_PASSWORD="${GRXFIRMA_PKCS12_PASSWORD:-}"
EOF
  chmod 600 "${ENV_FILE}"
fi
chmod 600 "${ENV_FILE}"

if grep -q '^export GRXFIRMA_PKCS12_PASSWORD=' "${ENV_FILE}"; then
  sed -i 's|^export GRXFIRMA_PKCS12_PASSWORD=.*|export GRXFIRMA_PKCS12_PASSWORD="${GRXFIRMA_PKCS12_PASSWORD:-}"|' "${ENV_FILE}"
else
  printf 'export GRXFIRMA_PKCS12_PASSWORD="${GRXFIRMA_PKCS12_PASSWORD:-}"\n' >> "${ENV_FILE}"
fi

sed -i \
  '/^export GRXFIRMA_DEBUG_ENABLED=/d; /^export GRXFIRMA_DEBUG_LOG_FILE=/d' \
  "${ENV_FILE}"

cat > "${USERLIBDIR}/browser-bridge.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
source "${ENV_FILE}" 2>/dev/null || true
exec "${USERLIBDIR}/grxfirma-nativehost" "\$@"
EOF
chmod 755 "${USERLIBDIR}/browser-bridge.sh"

cat > "${USERLIBDIR}/afirmauri-handler.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
source "${ENV_FILE}" 2>/dev/null || true
uri="\${1:-}"
if [[ -z "\${uri}" ]]; then
  exec "${USERLIBDIR}/desktop-launcher.sh"
fi
export GRXFIRMA_PROTOCOL_UI=1
exec "${USERBIN}/grxfirma-afirmauri" "\${uri}"
EOF
chmod 755 "${USERLIBDIR}/afirmauri-handler.sh"

cat > "${USERLIBDIR}/desktop-launcher.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
source "${ENV_FILE}" 2>/dev/null || true
if [[ -x "${USERBIN}/grxfirma-gui-qml" ]]; then
  if [[ -x "${USERBIN}/grxfirma-gui" ]]; then
    exec "${USERBIN}/grxfirma-gui" "\$@"
  fi
  exec "${USERBIN}/grxfirma-gui-qml" "\$@"
fi
exec "${USERBIN}/grxfirma-desktop" "\$@"
EOF
chmod 755 "${USERLIBDIR}/desktop-launcher.sh"

install -m 755 "${SELF_DIR}/configure-browsers.sh" "${USERLIBDIR}/configure-browsers.sh"

printf '%s\n' "${package_version}" > "${INSTALLED_VERSION_FILE}"
if [[ -f "${PACKAGE_BUILD_INFO_FILE}" ]]; then
  install -m 644 "${PACKAGE_BUILD_INFO_FILE}" "${INSTALLED_BUILD_INFO_FILE}"
else
  rm -f "${INSTALLED_BUILD_INFO_FILE}"
fi

sed 's|^Exec=.*|Exec='"${USERLIBDIR//\//\\/}"'/afirmauri-handler.sh %u|' "${SELF_DIR}/grxfirma.desktop" > "${USERAPP}/grxfirma.desktop"
sed 's|^Exec=.*|Exec='"${USERLIBDIR//\//\\/}"'/desktop-launcher.sh|' "${SELF_DIR}/grxfirma-manual.desktop" > "${USERAPP}/grxfirma-manual.desktop"
for icon_size in 48 128 256; do
  install -D -m 644 "${SELF_DIR}/icons/hicolor/${icon_size}x${icon_size}/apps/grxfirma.png" \
    "${HOME}/.local/share/icons/hicolor/${icon_size}x${icon_size}/apps/grxfirma.png"
done
install -D -m 644 "${SELF_DIR}/icons/hicolor/scalable/apps/grxfirma.svg" \
  "${HOME}/.local/share/icons/hicolor/scalable/apps/grxfirma.svg"
rm -f \
  "${USERAPP}/grxfirma-debug.desktop" \
  "${USERAPP}/grxfirma-manual-debug.desktop"

install -m 644 "${SELF_DIR}/man/grxfirma.1" "${USERMANDIR}/grxfirma.1"
for manpage in "${SELF_DIR}"/man/*.7; do
  install -m 644 "${manpage}" "${USERMAN7DIR}/$(basename "${manpage}")"
done

mkdir -p "${NM_CHROME}" "${NM_CHROMIUM}" "${NM_EDGE}" "${NM_BRAVE}" "${NM_VIVALDI}" "${NM_OPERA}" "${NM_FIREFOX}"

write_chrome_manifest() {
  local target="$1"
  local name="$2"
  local origins="$3"
  local tmp
  tmp="$(mktemp "${target}.tmp.XXXXXXXX")"
  cat > "${tmp}" <<EOF
{
  "name": "${name}",
  "description": "GrxFirma Native Messaging Host",
  "path": "${USERLIBDIR}/browser-bridge.sh",
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
  local tmp
  tmp="$(mktemp "${target}.tmp.XXXXXXXX")"
  cat > "${tmp}" <<EOF
{
  "name": "${name}",
  "description": "GrxFirma Native Messaging Host",
  "path": "${USERLIBDIR}/browser-bridge.sh",
  "type": "stdio",
  "allowed_extensions": [${extensions}]
}
EOF
  chmod 0644 "${tmp}"
  mv -f -- "${tmp}" "${target}"
}

build_primary_chrome_origins() {
  local origins=("\"chrome-extension://${CHROME_EXT_ID}/\"")
  if [[ -n "${EXTRA_CHROME_EXT_ID}" && "${EXTRA_CHROME_EXT_ID}" != "${CHROME_EXT_ID}" ]]; then
    origins+=("\"chrome-extension://${EXTRA_CHROME_EXT_ID}/\"")
  fi
  local joined=""
  local item
  for item in "${origins[@]}"; do
    if [[ -n "${joined}" ]]; then
      joined+=","
    fi
    joined+="${item}"
  done
  printf '%s' "${joined}"
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

PRIMARY_CHROME_ORIGINS="$(build_primary_chrome_origins)"

for dir in "${NM_CHROME}" "${NM_CHROMIUM}" "${NM_EDGE}" "${NM_BRAVE}" "${NM_VIVALDI}" "${NM_OPERA}"; do
  write_chrome_manifest "${dir}/com.grxfirma.native.json" "com.grxfirma.native" "${PRIMARY_CHROME_ORIGINS}"
  write_chrome_manifest "${dir}/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "${PRIMARY_CHROME_ORIGINS}"
done

write_firefox_manifest "${NM_FIREFOX}/com.grxfirma.native.json" "com.grxfirma.native" "\"${FIREFOX_EXT_ID}\""
write_firefox_manifest "${NM_FIREFOX}/io.github.aavidad.grxfirma.json" "io.github.aavidad.grxfirma" "\"${FIREFOX_EXT_ID}\""

# Paquetes de extension con el nombre de versiones anteriores.
rm -f "${USEREXTDIR}/firefox/dipgra-extension-firefox.xpi" \
  "${USEREXTDIR}/firefox/dipgra-extension-firefox.metadata.json" \
  "${USEREXTDIR}/chromium/dipgra-extension-chromium.zip" \
  "${USEREXTDIR}/chromium/dipgra-extension-chromium.crx" \
  "${USEREXTDIR}/chromium/dipgra-extension-chromium.id"

if [[ -f "${FIREFOX_XPI}" ]]; then
  install -m 644 "${FIREFOX_XPI}" "${USEREXTDIR}/firefox/grxfirma-extension-firefox.xpi"
  if [[ -f "${FIREFOX_METADATA}" ]]; then
    install -m 644 "${FIREFOX_METADATA}" "${USEREXTDIR}/firefox/grxfirma-extension-firefox.metadata.json"
  else
    rm -f "${USEREXTDIR}/firefox/grxfirma-extension-firefox.metadata.json"
  fi
  if firefox_xpi_is_signed; then
    for firefox_root in \
      "${HOME}/.mozilla/firefox" \
      "${HOME}/snap/firefox/common/.mozilla/firefox" \
      "${HOME}/.var/app/org.mozilla.firefox/.mozilla/firefox"
    do
      [[ -d "${firefox_root}" ]] || continue
      while IFS= read -r -d '' profile; do
        mkdir -p "${profile}/extensions"
        install -m 644 "${FIREFOX_XPI}" "${profile}/extensions/${FIREFOX_EXT_ID}.xpi"
      done < <(find "${firefox_root}" -mindepth 1 -maxdepth 1 -type d -exec test -f "{}/prefs.js" ';' -print0)
    done
  else
    echo "Aviso: el XPI Firefox es de desarrollo y no se instala en perfiles estables." >&2
  fi
fi

if [[ -f "${CHROMIUM_ZIP}" ]]; then
  install -m 644 "${CHROMIUM_ZIP}" "${USEREXTDIR}/chromium/grxfirma-extension-chromium.zip"
fi
if [[ -f "${CHROMIUM_CRX}" ]]; then
  install -m 644 "${CHROMIUM_CRX}" "${USEREXTDIR}/chromium/grxfirma-extension-chromium.crx"
fi
if [[ -f "${CHROMIUM_CRX_ID_FILE}" ]]; then
  install -m 644 "${CHROMIUM_CRX_ID_FILE}" "${USEREXTDIR}/chromium/grxfirma-extension-chromium.id"
fi

GRXFIRMA_TARGET_HOME="${HOME}" \
GRXFIRMA_DESKTOP_ID="grxfirma.desktop" \
GRXFIRMA_BROWSER_BRIDGE="${USERLIBDIR}/browser-bridge.sh" \
GRXFIRMA_FIREFOX_XPI="${USEREXTDIR}/firefox/grxfirma-extension-firefox.xpi" \
GRXFIRMA_FIREFOX_METADATA="${USEREXTDIR}/firefox/grxfirma-extension-firefox.metadata.json" \
CHROME_EXT_ID="${CHROME_EXT_ID}" \
EXTRA_CHROME_EXT_ID="${EXTRA_CHROME_EXT_ID}" \
FIREFOX_EXT_ID="${FIREFOX_EXT_ID}" \
  "${USERLIBDIR}/configure-browsers.sh" || true

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "${USERAPP}" || true
fi
# configure-browsers.sh ya registra afirma:// respetando si se eligió AutoFirma.

echo "Suite instalada en:"
echo "  CLI:          ${USERBIN}/grxfirma"
echo "  Desktop IPC:  ${USERBIN}/grxfirma-gui"
echo "  Desktop:      ${USERBIN}/grxfirma-desktop"
if [[ -x "${USERBIN}/grxfirma-gui-qml" ]]; then
  echo "  Desktop Qt:   ${USERBIN}/grxfirma-gui-qml"
  if [[ -d "${USERGUIQMLDIR}" ]]; then
    echo "  Qt QML/Assets:${USERGUIQMLDIR}"
  fi
fi
echo "  AfirmaURI:    ${USERBIN}/grxfirma-afirmauri"
echo "  NativeHost:   ${USERLIBDIR}/grxfirma-nativehost"
echo "  PKCS11 worker: ${USERLIBDIR}/grxfirma-pkcs11-worker (capacidad desactivada)"
echo "  Extensiones:  ${USEREXTDIR}"
echo "  Config:       ${ENV_FILE}"
echo "  Certificados: ${USERCFGDIR}"
