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
    "${BATS_TEST_DIRNAME}/../configure-browsers.sh"
}

@test "does not install an unsigned Firefox artifact" {
  write_metadata false

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -e "${TEST_PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "installs a signed Firefox artifact with matching hash" {
  write_metadata true

  run configure_browsers

  [ "$status" -eq 0 ]
  [ -f "${TEST_PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "rejects a signed Firefox artifact with mismatched hash" {
  write_metadata true
  printf 'tampered' >> "${TEST_XPI}"

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -e "${TEST_PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "los manifiestos nativos tienen modo 0644 con umask restrictiva" {
  write_metadata false
  umask 077

  run configure_browsers

  [ "$status" -eq 0 ]
  [ "$(stat -c %a "${TEST_HOME}/.config/google-chrome/NativeMessagingHosts/com.dipgra.grxfirma.json")" = 644 ]
  [ "$(stat -c %a "${TEST_HOME}/.mozilla/native-messaging-hosts/com.dipgra.grxfirma.json")" = 644 ]
}

@test "reemplaza un enlace de manifiesto sin modificar su destino" {
  write_metadata false
  local target="${TEST_HOME}/.config/google-chrome/NativeMessagingHosts/com.dipgra.grxfirma.json"
  local sentinel="${BATS_TEST_TMPDIR}/ajeno"
  mkdir -p "$(dirname "${target}")"
  printf 'intacto\n' > "${sentinel}"
  ln -s "${sentinel}" "${target}"

  run configure_browsers

  [ "$status" -eq 0 ]
  [ ! -L "${target}" ]
  [ "$(cat "${sentinel}")" = intacto ]
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
