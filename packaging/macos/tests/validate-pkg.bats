#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export MOCK_BIN="${BATS_TEST_TMPDIR}/bin"
  export PKGUTIL_LISTING_FILE="${BATS_TEST_TMPDIR}/payload.txt"
  export PKGUTIL_SIGNATURE_MARKER="${BATS_TEST_TMPDIR}/signature.checked"
  export TEST_PKG="${BATS_TEST_TMPDIR}/GrxFirma.pkg"
  export VALIDATOR="${BATS_TEST_DIRNAME}/../validate-pkg.sh"
  mkdir -p "${MOCK_BIN}"
  printf 'package-fixture' > "${TEST_PKG}"
  cat > "${MOCK_BIN}/pkgutil" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  --payload-files)
    cat "${PKGUTIL_LISTING_FILE:?}"
    ;;
  --check-signature)
    printf 'checked' > "${PKGUTIL_SIGNATURE_MARKER:?}"
    ;;
  *)
    echo "pkgutil simulado: argumento no soportado: ${1:-}" >&2
    exit 2
    ;;
esac
EOF
  chmod 755 "${MOCK_BIN}/pkgutil"
  export PATH="${MOCK_BIN}:${PATH}"
}

write_complete_listing() {
  local prefix="${1:-}"
  cat > "${PKGUTIL_LISTING_FILE}" <<EOF
${prefix}Applications/GrxFirma AfirmaURI.app/Contents/Info.plist
${prefix}Applications/GrxFirma AfirmaURI.app/Contents/MacOS/grxfirma-afirmauri
${prefix}Applications/GrxFirma Desktop Qt.app/Contents/Info.plist
${prefix}Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml
${prefix}Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui
${prefix}Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma
${prefix}Applications/GrxFirma Desktop Qt.app/Contents/Resources/VERSION.txt
${prefix}Library/Application Support/GrxFirma/grxfirma
${prefix}Library/Application Support/GrxFirma/grxfirma-nativehost
${prefix}Library/Application Support/GrxFirma/install-nativehost.sh
${prefix}Library/Application Support/GrxFirma/uninstall-suite.sh
${prefix}Library/Application Support/GrxFirma/register-user.sh
${prefix}Library/Application Support/GrxFirma/extensions/grxfirma-extension-firefox.metadata.json
${prefix}Library/LaunchAgents/es.dipgra.grxfirma.register-user.plist
${prefix}usr/local/bin/grxfirma
EOF
}

@test "acepta la salida de pkgutil con prefijo punto y barra" {
  write_complete_listing "./"

  run bash "${VALIDATOR}" "${TEST_PKG}"

  [ "$status" -eq 0 ]
}

@test "acepta la salida de pkgutil sin prefijo" {
  write_complete_listing

  run bash "${VALIDATOR}" "${TEST_PKG}"

  [ "$status" -eq 0 ]
}

@test "rechaza un payload al que le falta un binario" {
  write_complete_listing
  sed -i '/grxfirma-nativehost/d' "${PKGUTIL_LISTING_FILE}"

  run bash "${VALIDATOR}" "${TEST_PKG}"

  [ "$status" -ne 0 ]
  [[ "$output" == *"grxfirma-nativehost"* ]]
}

@test "rechaza un payload Desktop Qt sin el bootstrap IPC" {
  write_complete_listing
  sed -i '/MacOS\/grxfirma-gui$/d' "${PKGUTIL_LISTING_FILE}"

  run bash "${VALIDATOR}" "${TEST_PKG}"

  [ "$status" -ne 0 ]
  [[ "$output" == *"MacOS/grxfirma-gui"* ]]
}

@test "rechaza un payload Desktop Qt sin version interna" {
  write_complete_listing
  sed -i '/Contents\/Resources\/VERSION.txt$/d' "${PKGUTIL_LISTING_FILE}"

  run bash "${VALIDATOR}" "${TEST_PKG}"

  [ "$status" -ne 0 ]
  [[ "$output" == *"Contents/Resources/VERSION.txt"* ]]
}

@test "comprueba la firma cuando se declara identidad de instalador" {
  write_complete_listing

  run env MACOS_INSTALLER_IDENTITY="Developer ID Installer: Test" \
    bash "${VALIDATOR}" "${TEST_PKG}"

  [ "$status" -eq 0 ]
  [ -f "${PKGUTIL_SIGNATURE_MARKER}" ]
}
