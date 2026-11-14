#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export TEST_HOME="${BATS_TEST_TMPDIR}/home"
  export TEST_BIN="${BATS_TEST_TMPDIR}/bin"
  export TEST_XPI="${BATS_TEST_TMPDIR}/firefox.xpi"
  export TEST_METADATA="${BATS_TEST_TMPDIR}/firefox.metadata.json"
  export TEST_PROFILE="${TEST_HOME}/.mozilla/firefox/test.default"
  mkdir -p "${TEST_BIN}" "${TEST_PROFILE}"
  : > "${TEST_PROFILE}/prefs.js"
  printf 'test-xpi' > "${TEST_XPI}"
  for command_name in firefox xdg-mime update-desktop-database snap flatpak; do
    printf '#!/usr/bin/env bash\nexit 0\n' > "${TEST_BIN}/${command_name}"
    chmod 755 "${TEST_BIN}/${command_name}"
  done
  printf '#!/usr/bin/env bash\nexit 1\n' > "${TEST_BIN}/snap"
  printf '#!/usr/bin/env bash\nexit 1\n' > "${TEST_BIN}/flatpak"
  export PATH="${TEST_BIN}:${PATH}"
}

write_metadata() {
  local signed="$1"
  local hash
  hash="$(sha256sum "${TEST_XPI}" | awk '{print $1}')"
  printf '{"signed":%s,"xpi_sha256":"%s"}\n' "${signed}" "${hash}" > "${TEST_METADATA}"
}

configure_browsers() {
  HOME="${TEST_HOME}" \
    GRXFIRMA_TARGET_HOME="${TEST_HOME}" \
    GRXFIRMA_BROWSER_BRIDGE=/bin/true \
    GRXFIRMA_FIREFOX_XPI="${TEST_XPI}" \
    GRXFIRMA_FIREFOX_METADATA="${TEST_METADATA}" \
    GRXFIRMA_SYSTEM_APP_DIRS="${BATS_TEST_TMPDIR}/usr/share/applications" \
    "${BATS_TEST_DIRNAME}/../configure-browsers.sh"
}

@test "does not install an unsigned Firefox artifact" {
  write_metadata false

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -e "${TEST_PROFILE}/extensions/grxfirma@aavidad.github.io.xpi" ]
}

@test "installs a signed Firefox artifact with matching hash" {
  write_metadata true

  run configure_browsers

  [ "$status" -eq 0 ]
  [ -f "${TEST_PROFILE}/extensions/grxfirma@aavidad.github.io.xpi" ]
}

@test "rejects a signed Firefox artifact with mismatched hash" {
  write_metadata true
  printf 'tampered' >> "${TEST_XPI}"

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -e "${TEST_PROFILE}/extensions/grxfirma@aavidad.github.io.xpi" ]
}

@test "los manifiestos nativos tienen modo 0644 con umask restrictiva" {
  write_metadata false
  umask 077

  run configure_browsers

  [ "$status" -eq 0 ]
  [ "$(stat -c %a "${TEST_HOME}/.config/google-chrome/NativeMessagingHosts/io.github.aavidad.grxfirma.json")" = 644 ]
  [ "$(stat -c %a "${TEST_HOME}/.mozilla/native-messaging-hosts/io.github.aavidad.grxfirma.json")" = 644 ]
}

@test "reemplaza un enlace de manifiesto sin modificar su destino" {
  write_metadata false
  local target="${TEST_HOME}/.config/google-chrome/NativeMessagingHosts/io.github.aavidad.grxfirma.json"
  local sentinel="${BATS_TEST_TMPDIR}/ajeno"
  mkdir -p "$(dirname "${target}")"
  printf 'intacto\n' > "${sentinel}"
  ln -s "${sentinel}" "${target}"

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -L "${target}" ]
  [ "$(cat "${sentinel}")" = intacto ]
}

@test "retira los hosts y la extension de versiones anteriores y conserva los ajenos" {
  write_metadata false
  local chrome_dir="${TEST_HOME}/.config/google-chrome/NativeMessagingHosts"
  local edge_dir="${TEST_HOME}/.config/microsoft-edge/NativeMessagingHosts"
  local firefox_dir="${TEST_HOME}/.mozilla/native-messaging-hosts"
  mkdir -p "${chrome_dir}" "${edge_dir}" "${firefox_dir}" "${TEST_PROFILE}/extensions"
  printf '{"name":"com.dipgra.grxfirma","path":"/usr/lib/grxfirma/bin/browser-bridge.sh"}\n' \
    > "${chrome_dir}/com.dipgra.grxfirma.json"
  printf '{"name":"com.dipgra.portafirmas","path":"%s/.local/lib/grxfirma/bin/browser-bridge.sh"}\n' "${TEST_HOME}" \
    > "${firefox_dir}/com.dipgra.portafirmas.json"
  printf '{"name":"com.dipgra.portafirmas","path":"/opt/otro-producto/host"}\n' \
    > "${edge_dir}/com.dipgra.portafirmas.json"
  printf 'xpi-anterior' > "${TEST_PROFILE}/extensions/extension@dipgra.es.xpi"

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -e "${chrome_dir}/com.dipgra.grxfirma.json" ]
  [ ! -e "${firefox_dir}/com.dipgra.portafirmas.json" ]
  [ -f "${edge_dir}/com.dipgra.portafirmas.json" ]
  [ ! -e "${TEST_PROFILE}/extensions/extension@dipgra.es.xpi" ]
  [ -f "${firefox_dir}/io.github.aavidad.grxfirma.json" ]
}

@test "no registra el host de portafirmas y retira solo el que apunta a GrxFirma" {
  write_metadata false
  local chrome_dir="${TEST_HOME}/.config/google-chrome/NativeMessagingHosts"
  local edge_dir="${TEST_HOME}/.config/microsoft-edge/NativeMessagingHosts"
  local firefox_dir="${TEST_HOME}/.mozilla/native-messaging-hosts"
  mkdir -p "${chrome_dir}" "${edge_dir}" "${firefox_dir}"
  printf '{"name":"io.github.aavidad.portafirmas","path":"/usr/lib/grxfirma/bin/browser-bridge.sh"}\n' \
    > "${chrome_dir}/io.github.aavidad.portafirmas.json"
  printf '{"name":"io.github.aavidad.portafirmas","path":"%s/.local/lib/grxfirma/bin/browser-bridge.sh"}\n' "${TEST_HOME}" \
    > "${firefox_dir}/io.github.aavidad.portafirmas.json"
  printf '{"name":"io.github.aavidad.portafirmas","path":"/opt/portafirmas/host"}\n' \
    > "${edge_dir}/io.github.aavidad.portafirmas.json"

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -e "${chrome_dir}/io.github.aavidad.portafirmas.json" ]
  [ ! -e "${firefox_dir}/io.github.aavidad.portafirmas.json" ]
  [ -f "${edge_dir}/io.github.aavidad.portafirmas.json" ]
  [ -f "${chrome_dir}/io.github.aavidad.grxfirma.json" ]
  run grep -Rq 'portafirmas@dipgra.es\|ipkpimgjhkjibkbhfdhggjldlaetbcoa' "${chrome_dir}" "${firefox_dir}"
  [ "$status" -eq 1 ]
}

@test "registra afirma:// para GrxFirma por defecto" {
  write_metadata false

  run configure_browsers

  [ "$status" -eq 0 ]
  grep -qx 'x-scheme-handler/afirma=grxfirma.desktop' "${TEST_HOME}/.config/mimeapps.list"
  grep -qx 'x-scheme-handler/afirmav2=grxfirma.desktop' "${TEST_HOME}/.config/mimeapps.list"
}

@test "respeta la elección de AutoFirma al actualizar si sigue instalado" {
  write_metadata false
  mkdir -p "${TEST_HOME}/.config/grxfirma" "${TEST_HOME}/.local/share/applications"
  printf 'autofirma\n' > "${TEST_HOME}/.config/grxfirma/afirma-protocol-handler"
  printf '[Desktop Entry]\nExec=/usr/bin/autofirma %%u\nMimeType=x-scheme-handler/afirma;\n' \
    > "${TEST_HOME}/.local/share/applications/afirma.desktop"
  printf '[Default Applications]\nx-scheme-handler/afirma=afirma.desktop\n' > "${TEST_HOME}/.config/mimeapps.list"

  run configure_browsers

  [ "$status" -eq 0 ]
  grep -qx 'x-scheme-handler/afirma=afirma.desktop' "${TEST_HOME}/.config/mimeapps.list"
  grep -qx 'x-scheme-handler/afirmav2=grxfirma.desktop' "${TEST_HOME}/.config/mimeapps.list"

  rm "${TEST_HOME}/.local/share/applications/afirma.desktop"
  run configure_browsers

  [ "$status" -eq 0 ]
  grep -qx 'x-scheme-handler/afirma=grxfirma.desktop' "${TEST_HOME}/.config/mimeapps.list"
}

# G-11: avisar de que se sustituye a AutoFirma y devolver el manejador al desinstalar.
restore_browsers() {
  HOME="${TEST_HOME}" \
    GRXFIRMA_TARGET_HOME="${TEST_HOME}" \
    "${BATS_TEST_DIRNAME}/../configure-browsers.sh" --restore-afirma-handler
}

mime_value() {
  sed -n "s|^$2=||p" "$1"
}

@test "avisa al sustituir AutoFirma y lo devuelve al desinstalar (G-11)" {
  write_metadata false
  local mimeapps="${TEST_HOME}/.config/mimeapps.list"
  mkdir -p "$(dirname "${mimeapps}")"
  printf '[Default Applications]\nx-scheme-handler/afirma=autofirma.desktop\ntext/plain=gedit.desktop\n' > "${mimeapps}"

  run configure_browsers
  [ "$status" -eq 0 ]
  [[ "${output}" == *"autofirma.desktop"* ]]
  [[ "${output}" == *"afirma://"* ]]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = grxfirma.desktop ]
  grep -q 'GrxFirma:' "${TEST_PROFILE}/user.js"

  # Reinstalar no debe perder el manejador original.
  run configure_browsers
  [ "$status" -eq 0 ]

  run restore_browsers
  [ "$status" -eq 0 ]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = autofirma.desktop ]
  [ -z "$(mime_value "${mimeapps}" x-scheme-handler/afirmav2)" ]
  [ "$(mime_value "${mimeapps}" text/plain)" = gedit.desktop ]
  run grep -c -e 'GrxFirma:' -e 'protocol-handler.*afirma' "${TEST_PROFILE}/user.js"
  [ "${output}" = 0 ]
  [ ! -e "${TEST_HOME}/.local/lib/grxfirma/config/afirma-handler-anterior.json" ]
}

@test "no pisa un manejador que el usuario cambió después de instalar (G-11)" {
  write_metadata false
  local mimeapps="${TEST_HOME}/.config/mimeapps.list"
  mkdir -p "$(dirname "${mimeapps}")"
  printf '[Default Applications]\nx-scheme-handler/afirma=autofirma.desktop\n' > "${mimeapps}"

  run configure_browsers
  [ "$status" -eq 0 ]
  sed -i 's|^x-scheme-handler/afirma=.*|x-scheme-handler/afirma=otra.desktop|' "${mimeapps}"

  run restore_browsers
  [ "$status" -eq 0 ]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = otra.desktop ]
}

@test "el prerm del .deb devuelve el manejador afirma:// antes de borrar ficheros (G-11)" {
  local prerm
  prerm="$(sed -n '/DEBIAN\/prerm" <<.EOF./,/^EOF$/p' "${BATS_TEST_DIRNAME}/../build-suite.sh")"
  [[ "${prerm}" == *"configure-browsers.sh --restore-afirma-handler"* ]]
}

@test "con AutoFirma elegido no avisa de sustitución y el desinstalado no lo toca (G-11 y selector)" {
  write_metadata false
  local mimeapps="${TEST_HOME}/.config/mimeapps.list"
  mkdir -p "${TEST_HOME}/.config/grxfirma" "${TEST_HOME}/.local/share/applications"
  printf 'autofirma\n' > "${TEST_HOME}/.config/grxfirma/afirma-protocol-handler"
  printf '[Desktop Entry]\nExec=/usr/bin/autofirma %%u\nMimeType=x-scheme-handler/afirma;\n' \
    > "${TEST_HOME}/.local/share/applications/afirma.desktop"
  printf '[Default Applications]\nx-scheme-handler/afirma=afirma.desktop\n' > "${mimeapps}"

  run configure_browsers
  [ "$status" -eq 0 ]
  [[ "${output}" != *"Desde ahora los abre GrxFirma"* ]]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = afirma.desktop ]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirmav2)" = grxfirma.desktop ]

  run restore_browsers
  [ "$status" -eq 0 ]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = afirma.desktop ]
  [ -z "$(mime_value "${mimeapps}" x-scheme-handler/afirmav2)" ]
}

@test "respeta AutoFirma al actualizar aunque su lanzador tenga otro nombre (queja 14/11/2026)" {
  write_metadata false
  local mimeapps="${TEST_HOME}/.config/mimeapps.list"
  local apps="${BATS_TEST_TMPDIR}/usr/share/applications"
  mkdir -p "$(dirname "${mimeapps}")" "${apps}" "${TEST_HOME}/.config/grxfirma"
  printf '[Desktop Entry]\nExec=/usr/bin/AutoFirma %%u\nMimeType=x-scheme-handler/afirma;\n' > "${apps}/AutoFirma.desktop"
  printf '[Default Applications]\nx-scheme-handler/afirma=AutoFirma.desktop\n' > "${mimeapps}"

  # Primera instalación: GrxFirma toma afirma:// y guarda AutoFirma.desktop.
  run configure_browsers
  [ "$status" -eq 0 ]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = grxfirma.desktop ]

  # La persona elige AutoFirma en Configuración y luego se actualiza GrxFirma.
  sed -i 's|^x-scheme-handler/afirma=.*|x-scheme-handler/afirma=AutoFirma.desktop|' "${mimeapps}"
  printf 'autofirma\n' > "${TEST_HOME}/.config/grxfirma/afirma-protocol-handler"
  run configure_browsers
  [ "$status" -eq 0 ]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = AutoFirma.desktop ]

  # Un lanzador guardado que no es AutoFirma no cuenta.
  printf '[Desktop Entry]\nExec=/usr/bin/otro %%u\nMimeType=x-scheme-handler/afirma;\n' > "${apps}/AutoFirma.desktop"
  run configure_browsers
  [ "$status" -eq 0 ]
  [ "$(mime_value "${mimeapps}" x-scheme-handler/afirma)" = grxfirma.desktop ]
}
