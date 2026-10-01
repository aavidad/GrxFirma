#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export TEST_HOME="${BATS_TEST_TMPDIR}/home"
  export SUITE="${BATS_TEST_TMPDIR}/suite"
  mkdir -p \
    "${TEST_HOME}" \
    "${SUITE}/extensions" \
    "${SUITE}/GrxFirma AfirmaURI.app/Contents/MacOS" \
    "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS" \
    "${SUITE}/GrxFirma Desktop Qt.app/Contents/Resources"
  cp "${BATS_TEST_DIRNAME}/../install-suite.sh" "${SUITE}/install-suite.sh"
  cp "${BATS_TEST_DIRNAME}/../install-nativehost.sh" "${SUITE}/install-nativehost.sh"
  cp "${BATS_TEST_DIRNAME}/../install-afirmauri.sh" "${SUITE}/install-afirmauri.sh"
  cp "${BATS_TEST_DIRNAME}/../install-desktop-qml.sh" "${SUITE}/install-desktop-qml.sh"
  cp "${BATS_TEST_DIRNAME}/../uninstall-suite.sh" "${SUITE}/uninstall-suite.sh"
  printf 'cli' > "${SUITE}/grxfirma"
  printf 'native-host' > "${SUITE}/grxfirma-nativehost"
  chmod 755 "${SUITE}/grxfirma" "${SUITE}/grxfirma-nativehost"
  printf '<plist version="1.0"><dict/></plist>\n' \
    > "${SUITE}/GrxFirma AfirmaURI.app/Contents/Info.plist"
  printf '#!/usr/bin/env bash\nexit 0\n' \
    > "${SUITE}/GrxFirma AfirmaURI.app/Contents/MacOS/grxfirma-afirmauri"
  chmod 755 "${SUITE}/GrxFirma AfirmaURI.app/Contents/MacOS/grxfirma-afirmauri"
  printf '<plist version="1.0"><dict/></plist>\n' \
    > "${SUITE}/GrxFirma Desktop Qt.app/Contents/Info.plist"
  printf '#!/usr/bin/env bash\nexit 0\n' \
    > "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml"
  printf '#!/usr/bin/env bash\nexit 0\n' \
    > "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui"
  printf '#!/usr/bin/env bash\nexit 0\n' \
    > "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma"
  printf '0.1.0\n' \
    > "${SUITE}/GrxFirma Desktop Qt.app/Contents/Resources/VERSION.txt"
  chmod 755 \
    "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml" \
    "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui" \
    "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma"
}

@test "preflight rejects an incomplete suite without installing the CLI" {
  rm "${SUITE}/grxfirma-nativehost"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"paquete macOS incompleto"* ]]
  [ ! -e "${TEST_HOME}/Library/Application Support/GrxFirma/CLI/grxfirma" ]
}

@test "installs the complete suite and replaces the app safely" {
  local old_app="${TEST_HOME}/Applications/GrxFirma AfirmaURI.app"
  mkdir -p "${old_app}"
  printf 'old' > "${old_app}/old-marker"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"

  [ "$status" -eq 0 ]
  [ -x "${TEST_HOME}/Library/Application Support/GrxFirma/CLI/grxfirma" ]
  [ -x "${TEST_HOME}/Library/Application Support/GrxFirma/NativeHost/grxfirma-nativehost" ]
  [ -f "${old_app}/Contents/Info.plist" ]
  [ ! -e "${old_app}/old-marker" ]
  [ -x "${TEST_HOME}/Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml" ]
  [ -x "${TEST_HOME}/Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui" ]
  [ -x "${TEST_HOME}/Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma" ]
  [ -f "${TEST_HOME}/Library/Application Support/Google/Chrome/NativeMessagingHosts/com.grxfirma.native.json" ]
}

@test "rejects an incomplete desktop app before installing the CLI" {
  rm "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"paquete macOS incompleto"* ]]
  [ ! -e "${TEST_HOME}/Library/Application Support/GrxFirma/CLI/grxfirma" ]
}

@test "rejects Desktop Qt without the IPC bootstrap before installing the CLI" {
  rm "${SUITE}/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"grxfirma-gui"* ]]
  [ ! -e "${TEST_HOME}/Library/Application Support/GrxFirma/CLI/grxfirma" ]
}

@test "desktop installer replaces an old app without following a destination symlink" {
  local external="${BATS_TEST_TMPDIR}/external-app"
  local destination="${TEST_HOME}/Applications/GrxFirma Desktop Qt.app"
  mkdir -p "${TEST_HOME}/Applications" "${external}"
  printf 'keep\n' > "${external}/marker"
  ln -s "${external}" "${destination}"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-desktop-qml.sh"

  [ "$status" -eq 0 ]
  [ ! -L "${destination}" ]
  [ -x "${destination}/Contents/MacOS/grxfirma-gui-qml" ]
  [ -x "${destination}/Contents/MacOS/grxfirma-gui" ]
  [ "$(cat "${external}/marker")" = "keep" ]
}

@test "desktop installer rejects a symbolic Applications ancestor" {
  local external="${BATS_TEST_TMPDIR}/external-applications"
  mkdir -p "${external}"
  printf 'keep\n' > "${external}/marker"
  ln -s "${external}" "${TEST_HOME}/Applications"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-desktop-qml.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"enlace simbolico"* ]]
  [ "$(cat "${external}/marker")" = "keep" ]
  [ ! -e "${external}/GrxFirma Desktop Qt.app" ]
}

@test "restaura CLI nativehost manifiestos y aplicaciones si falla el ultimo paso" {
  local base="${TEST_HOME}/Library/Application Support/GrxFirma"
  local native_manifests="${TEST_HOME}/Library/Application Support/Google/Chrome/NativeMessagingHosts"
  local afirma_app="${TEST_HOME}/Applications/GrxFirma AfirmaURI.app"
  local desktop_app="${TEST_HOME}/Applications/GrxFirma Desktop Qt.app"
  local mock_bin="${BATS_TEST_TMPDIR}/mock-bin"
  mkdir -p \
    "${base}/CLI" \
    "${base}/NativeHost" \
    "${native_manifests}" \
    "${afirma_app}" \
    "${desktop_app}" \
    "${mock_bin}"
  printf 'old-cli\n' > "${base}/CLI/grxfirma"
  printf 'old-nativehost\n' > "${base}/NativeHost/grxfirma-nativehost"
  printf 'old-manifest\n' > "${native_manifests}/com.grxfirma.native.json"
  printf 'unrelated\n' > "${native_manifests}/keep.json"
  printf 'old-afirma\n' > "${afirma_app}/old-marker"
  printf 'old-desktop\n' > "${desktop_app}/old-marker"

  cat > "${mock_bin}/ditto" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "${SUITE:?}/GrxFirma Desktop Qt.app" ]]; then
  echo "fallo simulado al copiar Desktop Qt" >&2
  exit 73
fi
cp -a "${1:?}" "${2:?}"
EOF
  chmod 755 "${mock_bin}/ditto"

  run env \
    HOME="${TEST_HOME}" \
    SUITE="${SUITE}" \
    PATH="${mock_bin}:${PATH}" \
    bash "${SUITE}/install-suite.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"restaurando el estado anterior"* ]]
  [ "$(cat "${base}/CLI/grxfirma")" = "old-cli" ]
  [ "$(cat "${base}/NativeHost/grxfirma-nativehost")" = "old-nativehost" ]
  [ "$(cat "${native_manifests}/com.grxfirma.native.json")" = "old-manifest" ]
  [ "$(cat "${native_manifests}/keep.json")" = "unrelated" ]
  [ "$(cat "${afirma_app}/old-marker")" = "old-afirma" ]
  [ "$(cat "${desktop_app}/old-marker")" = "old-desktop" ]
}

@test "restaura el estado anterior si la instalacion recibe SIGTERM" {
  local base="${TEST_HOME}/Library/Application Support/GrxFirma"
  mkdir -p "${base}/CLI"
  printf 'old-cli\n' > "${base}/CLI/grxfirma"
  # $PPID debe quedar literal en el script fixture generado.
  # shellcheck disable=SC2016
  printf '#!/usr/bin/env bash\nkill -TERM "$PPID"\n' \
    > "${SUITE}/install-desktop-qml.sh"
  chmod 755 "${SUITE}/install-desktop-qml.sh"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"

  [ "$status" -eq 143 ]
  [[ "$output" == *"restaurando el estado anterior"* ]]
  [ "$(cat "${base}/CLI/grxfirma")" = "old-cli" ]
  [ ! -e "${TEST_HOME}/Applications/GrxFirma AfirmaURI.app" ]
  [ ! -e "${TEST_HOME}/Applications/GrxFirma Desktop Qt.app" ]
}

@test "rechaza un directorio de manifiestos simbolico antes de modificar el usuario" {
  local external="${BATS_TEST_TMPDIR}/external-manifests"
  local manifest_parent="${TEST_HOME}/Library/Application Support/Google/Chrome"
  mkdir -p "${external}" "${manifest_parent}"
  printf 'keep\n' > "${external}/marker"
  ln -s "${external}" "${manifest_parent}/NativeMessagingHosts"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"enlace simbolico"* ]]
  [ "$(cat "${external}/marker")" = "keep" ]
  [ ! -e "${TEST_HOME}/Library/Application Support/GrxFirma/CLI/grxfirma" ]
}

@test "instalador y desinstalador rechazan un HOME simbolico" {
  local real_home="${BATS_TEST_TMPDIR}/real-home"
  local linked_home="${BATS_TEST_TMPDIR}/linked-home"
  mkdir -p "${real_home}"
  ln -s "${real_home}" "${linked_home}"

  run env HOME="${linked_home}" bash "${SUITE}/install-suite.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no simbolica"* ]]

  run env HOME="${linked_home}" bash "${SUITE}/install-desktop-qml.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no simbolica"* ]]

  run env HOME="${linked_home}" bash "${SUITE}/uninstall-suite.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no simbolica"* ]]
  [ ! -e "${real_home}/Library" ]
}

@test "el preflight system rechaza ancestros y objetivos simbolicos" {
  local fake_root="${BATS_TEST_TMPDIR}/fake-system"
  local external="${BATS_TEST_TMPDIR}/external-system"
  mkdir -p "${fake_root}" "${external}/Application Support"
  ln -s "${external}" "${fake_root}/Library"

  # Las variables pertenecen al shell hijo de bash -c.
  # shellcheck disable=SC2016
  run env GRXFIRMA_UNINSTALL_LIBRARY_ONLY=1 \
    UNINSTALLER="${SUITE}/uninstall-suite.sh" \
    TARGET="${fake_root}/Library/Application Support/GrxFirma" \
    bash -c 'source "${UNINSTALLER}"; validate_system_target "${TARGET}" 0'

  [ "$status" -ne 0 ]
  [[ "$output" == *"ancestro simbolico"* ]]

  rm "${fake_root}/Library"
  mkdir -p "${fake_root}/Library/Application Support"
  ln -s "${external}" "${fake_root}/Library/Application Support/GrxFirma"
  # Las variables pertenecen al shell hijo de bash -c.
  # shellcheck disable=SC2016
  run env GRXFIRMA_UNINSTALL_LIBRARY_ONLY=1 \
    UNINSTALLER="${SUITE}/uninstall-suite.sh" \
    TARGET="${fake_root}/Library/Application Support/GrxFirma" \
    bash -c 'source "${UNINSTALLER}"; validate_system_target "${TARGET}" 0'

  [ "$status" -ne 0 ]
  [[ "$output" == *"inesperadamente simbolico"* ]]
  [ -d "${external}" ]
}

@test "el desinstalador elimina solo la suite y es idempotente" {
  local base="${TEST_HOME}/Library/Application Support/GrxFirma"
  local native_manifests="${TEST_HOME}/Library/Application Support/Google/Chrome/NativeMessagingHosts"
  local external="${BATS_TEST_TMPDIR}/external-app"
  local desktop_link="${TEST_HOME}/Applications/GrxFirma Desktop Qt.app"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"
  [ "$status" -eq 0 ]
  printf 'unrelated\n' > "${native_manifests}/keep.json"

  rm -rf "${desktop_link}"
  mkdir -p "${external}"
  printf 'keep\n' > "${external}/marker"
  ln -s "${external}" "${desktop_link}"

  run env HOME="${TEST_HOME}" bash "${base}/uninstall-suite.sh"

  [ "$status" -eq 0 ]
  [ ! -e "${base}/CLI" ]
  [ ! -e "${base}/NativeHost" ]
  [ ! -e "${TEST_HOME}/Applications/GrxFirma AfirmaURI.app" ]
  [ ! -e "${desktop_link}" ]
  [ "$(cat "${external}/marker")" = "keep" ]
  [ ! -e "${native_manifests}/com.grxfirma.native.json" ]
  [ "$(cat "${native_manifests}/keep.json")" = "unrelated" ]

  run env HOME="${TEST_HOME}" bash "${SUITE}/uninstall-suite.sh"
  [ "$status" -eq 0 ]
  [ "$(cat "${native_manifests}/keep.json")" = "unrelated" ]
}

@test "el desinstalador limpia el bundle Desktop Qt y su bootstrap IPC" {
  local base="${TEST_HOME}/Library/Application Support/GrxFirma"
  local desktop_app="${TEST_HOME}/Applications/GrxFirma Desktop Qt.app"
  local ipc_bootstrap="${desktop_app}/Contents/MacOS/grxfirma-gui"

  run env HOME="${TEST_HOME}" bash "${SUITE}/install-suite.sh"
  [ "$status" -eq 0 ]
  [ -x "${ipc_bootstrap}" ]

  run env HOME="${TEST_HOME}" bash "${base}/uninstall-suite.sh"
  [ "$status" -eq 0 ]
  [ ! -e "${desktop_app}" ]
  [ ! -e "${ipc_bootstrap}" ]
}

@test "Safari build request fails before compilation outside macOS" {
  run env GRXFIRMA_BUILD_SAFARI=1 bash "${BATS_TEST_DIRNAME}/../build-suite.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"requiere macOS real con Xcode"* ]]
  [[ "$output" != *"Compilando suite"* ]]
}
