#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export TEST_HOME="${BATS_TEST_TMPDIR}/home"
  export TEST_SUITE="${BATS_TEST_TMPDIR}/suite"
  export TEST_BIN="${BATS_TEST_TMPDIR}/bin"
  mkdir -p "${TEST_HOME}" "${TEST_SUITE}/man" "${TEST_BIN}"
  for bin in xdg-mime update-desktop-database; do
    printf '#!/usr/bin/env bash\nexit 0\n' > "${TEST_BIN}/${bin}"
    chmod 755 "${TEST_BIN}/${bin}"
  done
  export PATH="${TEST_BIN}:${PATH}"
  cp "${BATS_TEST_DIRNAME}/../install-suite.sh" "${TEST_SUITE}/install-suite.sh"
  printf '#!/usr/bin/env bash\nexit 0\n' > "${TEST_SUITE}/configure-browsers.sh"
  for bin in grxfirma grxfirma-gui grxfirma-desktop grxfirma-afirmauri grxfirma-pkcs11-worker; do
    printf '#!/usr/bin/env bash\nexit 0\n' > "${TEST_SUITE}/${bin}"
  done
  cat > "${TEST_SUITE}/grxfirma-nativehost" <<'EOF'
#!/usr/bin/env bash
printf '<%s>\n' "$@"
EOF
  printf '#!/usr/bin/env bash\nexit 0\n' > "${TEST_SUITE}/check-runtime-dependencies.sh"
  chmod 755 "${TEST_SUITE}"/*.sh "${TEST_SUITE}"/grxfirma*
  printf '0.0.1\n' > "${TEST_SUITE}/VERSION.txt"
  printf '[Desktop Entry]\nExec=grxfirma\n' > "${TEST_SUITE}/grxfirma.desktop"
  printf '[Desktop Entry]\nExec=grxfirma\n' > "${TEST_SUITE}/grxfirma-manual.desktop"
  printf 'manual\n' > "${TEST_SUITE}/man/grxfirma.1"
  printf 'manual\n' > "${TEST_SUITE}/man/grxfirma.7"
  for size in 48 128 256; do
    mkdir -p "${TEST_SUITE}/icons/hicolor/${size}x${size}/apps"
    printf 'icono' > "${TEST_SUITE}/icons/hicolor/${size}x${size}/apps/grxfirma.png"
  done
  mkdir -p "${TEST_SUITE}/icons/hicolor/scalable/apps"
  printf '<svg/>\n' > "${TEST_SUITE}/icons/hicolor/scalable/apps/grxfirma.svg"
}

@test "el puente nativo instalado reenvía todos los argumentos y conserva espacios" {
  umask 077
  run env HOME="${TEST_HOME}" "${TEST_SUITE}/install-suite.sh"
  [ "$status" -eq 0 ]

  run env HOME="${TEST_HOME}" "${TEST_HOME}/.local/lib/grxfirma/bin/browser-bridge.sh" \
    --browser chrome --extension-id 'id con espacio'
  [ "$status" -eq 0 ]
  [ "$output" = $'<--browser>\n<chrome>\n<--extension-id>\n<id con espacio>' ]

  [ "$(stat -c %a "${TEST_HOME}/.config/google-chrome/NativeMessagingHosts/io.github.aavidad.grxfirma.json")" = 644 ]
  [ "$(stat -c %a "${TEST_HOME}/.mozilla/native-messaging-hosts/io.github.aavidad.grxfirma.json")" = 644 ]
}
