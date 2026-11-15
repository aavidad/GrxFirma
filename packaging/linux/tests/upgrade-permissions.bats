#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
# shellcheck disable=SC2016 # las comillas simples comparan el texto literal del postinst, con sus ${...} sin expandir.

# Al actualizar, dpkg no corrige los permisos de las carpetas que ya existían.
# El postinst quita la escritura de grupo y de otros en las carpetas propias.
setup() {
  BUILD_SUITE="${BATS_TEST_DIRNAME}/../build-suite.sh"
  POSTINST="$(sed -n "/DEBIAN\/postinst\" <<'EOF'/,/^EOF$/p" "${BUILD_SUITE}")"
}

@test "el postinst quita la escritura de grupo en /usr/lib/grxfirma al actualizar" {
  [[ "${POSTINST}" == *'for grx_dir in /usr/lib/grxfirma /usr/share/doc/grxfirma; do'* ]]
  [[ "${POSTINST}" == *'-perm /022 -exec chmod go-w {} +'* ]]
  [[ "${POSTINST}" == *'! -L "${grx_dir}"'* ]]
}

@test "el postinst corrige solo las carpetas de mensajería nativa y extensiones, sin recursión" {
  for d in /etc/opt/chrome/native-messaging-hosts /usr/lib/mozilla/native-messaging-hosts /usr/share/chromium/extensions; do
    [[ "${POSTINST}" == *"${d}"* ]]
  done
  [[ "${POSTINST}" == *'chmod go-w "${grx_dir}" || true'* ]]
  [[ "${POSTINST}" == *'"$(stat -c %u "${grx_dir}")" == 0'* ]]
  for d in /etc/opt/chrome /etc/opt/brave.com /usr/lib/mozilla /usr/share/google-chrome; do
    [[ "${POSTINST}" == *"  ${d} "* ]]
  done
  [[ "${POSTINST}" != *'chmod -R'* ]]
}

@test "la corrección funciona sobre un árbol con escritura de grupo" {
  root="${BATS_TEST_TMPDIR}/raiz"
  mkdir -p "${root}/usr/lib/grxfirma/gui-qml"
  touch "${root}/usr/lib/grxfirma/gui-qml/a"
  chmod 0775 "${root}/usr/lib/grxfirma" "${root}/usr/lib/grxfirma/gui-qml"
  chmod 0664 "${root}/usr/lib/grxfirma/gui-qml/a"
  snippet="$(printf '%s\n' "${POSTINST}" | sed -n '/^for grx_dir in \/usr\/lib\/grxfirma/,/^done$/p' | sed "s# /usr/# ${root}/usr/#g")"
  run bash -c "${snippet}"
  [ "$status" -eq 0 ]
  [ "$(stat -c %a "${root}/usr/lib/grxfirma")" = 755 ]
  [ "$(stat -c %a "${root}/usr/lib/grxfirma/gui-qml")" = 755 ]
  [ "$(stat -c %a "${root}/usr/lib/grxfirma/gui-qml/a")" = 644 ]
}
