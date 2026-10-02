#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Bats invokes fixture functions indirectly.
# shellcheck disable=SC2317,SC2329

setup() {
  export FIXTURE_ROOT="${BATS_TEST_TMPDIR}/runtime-dependencies"
  export TEST_BIN="${FIXTURE_ROOT}/bin"
  export PACKAGE_ROOT="${FIXTURE_ROOT}/package"
  export QML_ROOT="${FIXTURE_ROOT}/qt6/qml"
  export QML_MANIFEST="${FIXTURE_ROOT}/runtime-dependencies.qml"
  export WORK_DIR="${FIXTURE_ROOT}/work"
  export DPKG_SHLIBDEPS_ARGS_LOG="${FIXTURE_ROOT}/dpkg-shlibdeps.args"
  mkdir -p "${TEST_BIN}" "${PACKAGE_ROOT}/usr/bin" "${QML_ROOT}"
  # shellcheck disable=SC1091
  source "${BATS_TEST_DIRNAME}/../runtime-dependencies.sh"
}

write_fake_elf_tools() {
  cat > "${TEST_BIN}/readelf" <<'EOF'
#!/usr/bin/env bash
[[ "$1" == "-h" && -f "$2" ]]
EOF
cat > "${TEST_BIN}/dpkg-shlibdeps" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--help" ]]; then
  if [[ "${FAKE_DPKG_SHLIBDEPS_PACKAGE_OPTION:-0}" == "1" ]]; then
    printf '%s\n' '  --package=<package>'
  fi
  exit 0
fi
[[ -f debian/control ]]
[[ "$*" == *"debian/grxfirma/usr/bin/gui"* ]]
printf '%s\n' "$*" > "${DPKG_SHLIBDEPS_ARGS_LOG:?}"
printf '%s\n' 'shlibs:Depends=libc6 (>= 2.34), libqt6quick6 (>= 6.2.0), libqt6widgets6 (>= 6.2.0)'
EOF
  cat > "${TEST_BIN}/dpkg-query" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${*: -1}" in
  */QtQuick/Controls/qmldir)
    printf 'qml6-module-qtquick-controls:amd64: %s\n' "${*: -1}"
    ;;
  */QtQuick/qmldir)
    printf 'qml6-module-qtquick:amd64: %s\n' "${*: -1}"
    ;;
  *)
    exit 1
    ;;
esac
EOF
  chmod 755 "${TEST_BIN}/readelf" "${TEST_BIN}/dpkg-shlibdeps" "${TEST_BIN}/dpkg-query"
  export PATH="${TEST_BIN}:${PATH}"
}

@test "QML imports are normalized sorted and local imports are ignored" {
  mkdir -p "${FIXTURE_ROOT}/qml/nested"
  cat > "${FIXTURE_ROOT}/qml/main.qml" <<'EOF'
import QtQuick 2.15
import QtQuick.Controls 2.15
import "components"
EOF
  cat > "${FIXTURE_ROOT}/qml/nested/Dialog.qml" <<'EOF'
  import QtQuick.Controls 2.15
import Qt.labs.settings 1.1 as Settings // a packaged module with an alias
EOF

  grxfirma_collect_qml_imports "${FIXTURE_ROOT}/qml" "${QML_MANIFEST}"

  run cat "${QML_MANIFEST}"
  [ "$status" -eq 0 ]
  [ "$output" = $'Qt.labs.settings\nQtQuick\nQtQuick.Controls' ]
}

@test "Debian Depends combines canonical ELF and owning QML packages" {
  write_fake_elf_tools
  printf 'ELF fixture\n' > "${PACKAGE_ROOT}/usr/bin/gui"
  chmod 755 "${PACKAGE_ROOT}/usr/bin/gui"
  mkdir -p "${PACKAGE_ROOT}/usr/lib/grxfirma/bin"
  touch "${PACKAGE_ROOT}/usr/lib/grxfirma/bin/grxfirma-pkcs11-worker"
  mkdir -p "${QML_ROOT}/QtQuick/Controls" "${QML_ROOT}/QtQuick"
  : > "${QML_ROOT}/QtQuick/Controls/qmldir"
  : > "${QML_ROOT}/QtQuick/qmldir"
  printf '%s\n' QtQuick.Controls QtQuick > "${QML_MANIFEST}"

  run grxfirma_generate_debian_depends \
    "${PACKAGE_ROOT}" \
    grxfirma \
    "${QML_MANIFEST}" \
    "${QML_ROOT}" \
    "${WORK_DIR}"

  [ "$status" -eq 0 ]
  [ "$output" = "libc6 (>= 2.34), libqt6quick6 (>= 6.2.0), libqt6widgets6 (>= 6.2.0), qml6-module-qtquick, qml6-module-qtquick-controls" ]
  run grep -F -- "--package=" "${DPKG_SHLIBDEPS_ARGS_LOG}"
  [ "$status" -eq 1 ]
  run grep -F -- '/usr/lib/grxfirma/bin/grxfirma-pkcs11-worker' "${DPKG_SHLIBDEPS_ARGS_LOG}"
  [ "$status" -eq 0 ]
}

@test "Debian Depends merges required command-line runtimes deterministically" {
  run grxfirma_merge_debian_depends \
    "libqt6widgets6 (>= 6.2.0), libc6 (>= 2.34), libnss3-tools" \
    libnss3-tools \
    libsecret-tools

  [ "$status" -eq 0 ]
  [ "$output" = "libc6 (>= 2.34), libnss3-tools, libqt6widgets6 (>= 6.2.0), libsecret-tools" ]
}

@test "Debian Depends usa --package solo cuando dpkg-shlibdeps lo admite" {
  write_fake_elf_tools
  export FAKE_DPKG_SHLIBDEPS_PACKAGE_OPTION=1
  printf 'ELF fixture\n' > "${PACKAGE_ROOT}/usr/bin/gui"
  chmod 755 "${PACKAGE_ROOT}/usr/bin/gui"
  : > "${QML_MANIFEST}"

  run grxfirma_generate_debian_depends \
    "${PACKAGE_ROOT}" \
    grxfirma \
    "${QML_MANIFEST}" \
    "${QML_ROOT}" \
    "${WORK_DIR}"

  [ "$status" -eq 0 ]
  run grep -F -- "--package=grxfirma" \
    "${DPKG_SHLIBDEPS_ARGS_LOG}"
  [ "$status" -eq 0 ]
}

@test "Debian generation fails closed for an unowned QML module" {
  write_fake_elf_tools
  printf 'ELF fixture\n' > "${PACKAGE_ROOT}/usr/bin/gui"
  chmod 755 "${PACKAGE_ROOT}/usr/bin/gui"
  mkdir -p "${QML_ROOT}/Unknown/Module"
  : > "${QML_ROOT}/Unknown/Module/qmldir"
  printf '%s\n' Unknown.Module > "${QML_MANIFEST}"

  run grxfirma_generate_debian_depends \
    "${PACKAGE_ROOT}" \
    grxfirma \
    "${QML_MANIFEST}" \
    "${QML_ROOT}" \
    "${WORK_DIR}"

  [ "$status" -ne 0 ]
  [[ "$output" == *"ningún paquete Debian instalado declara"* ]]
}

@test "bundle preflight reports missing ELF and QML runtime dependencies" {
  printf 'ELF fixture\n' > "${FIXTURE_ROOT}/grxfirma-gui-qml"
  chmod 755 "${FIXTURE_ROOT}/grxfirma-gui-qml"
  printf '%s\n' GrxFirmaMissing.Controls > "${QML_MANIFEST}"
  cat > "${TEST_BIN}/ldd" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' 'libQt6Widgets.so.6 => not found'
EOF
  cat > "${TEST_BIN}/qtpaths6" <<EOF
#!/usr/bin/env bash
  printf '%s\n' '${QML_ROOT}'
EOF
  chmod 755 "${TEST_BIN}/ldd" "${TEST_BIN}/qtpaths6"
  # Cada @test de Bats se ejecuta en su propio subshell; el PATH es local a la
  # prueba por diseño y no debe propagarse a las siguientes.
  # shellcheck disable=SC2030,SC2031
  export PATH="${TEST_BIN}:${PATH}"

  run grxfirma_check_bundle_runtime "${FIXTURE_ROOT}"

  [ "$status" -ne 0 ]
  [[ "$output" == *"libQt6Widgets.so.6 => not found"* ]]
  [[ "$output" == *"falta el módulo QML GrxFirmaMissing.Controls"* ]]
}

@test "bundle preflight accepts resolved libraries and QML modules" {
  printf 'ELF fixture\n' > "${FIXTURE_ROOT}/grxfirma-gui-qml"
  chmod 755 "${FIXTURE_ROOT}/grxfirma-gui-qml"
  printf '%s\n' QtQuick.Controls > "${QML_MANIFEST}"
  mkdir -p "${QML_ROOT}/QtQuick/Controls"
  : > "${QML_ROOT}/QtQuick/Controls/qmldir"
  cat > "${TEST_BIN}/ldd" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' 'libQt6Widgets.so.6 => /usr/lib/libQt6Widgets.so.6 (0x1234)'
EOF
  cat > "${TEST_BIN}/qtpaths6" <<EOF
#!/usr/bin/env bash
  printf '%s\n' '${QML_ROOT}'
EOF
  chmod 755 "${TEST_BIN}/ldd" "${TEST_BIN}/qtpaths6"
  # Cada @test de Bats se ejecuta en su propio subshell; el PATH es local a la
  # prueba por diseño y no debe propagarse a las siguientes.
  # shellcheck disable=SC2030,SC2031
  export PATH="${TEST_BIN}:${PATH}"

  run grxfirma_check_bundle_runtime "${FIXTURE_ROOT}"

  [ "$status" -eq 0 ]
  [[ "$output" == *"Dependencias runtime del bundle verificadas."* ]]
}

@test "bundle preflight accepts a static Go binary" {
  printf 'static ELF fixture\n' > "${FIXTURE_ROOT}/grxfirma"
  chmod 755 "${FIXTURE_ROOT}/grxfirma"
  cat > "${TEST_BIN}/ldd" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' 'not a dynamic executable' >&2
exit 1
EOF
  chmod 755 "${TEST_BIN}/ldd"
  # Cada @test de Bats se ejecuta en su propio subshell; el PATH es local a la
  # prueba por diseño y no debe propagarse a las siguientes.
  # shellcheck disable=SC2030,SC2031
  export PATH="${TEST_BIN}:${PATH}"

  run grxfirma_check_bundle_runtime "${FIXTURE_ROOT}"

  [ "$status" -eq 0 ]
  [[ "$output" == *"Dependencias runtime del bundle verificadas."* ]]
}

@test "dependency workspace rejects a broad cleanup parent" {
  run grxfirma_generate_debian_depends \
    "${PACKAGE_ROOT}" \
    grxfirma \
    "${QML_MANIFEST}" \
    "${QML_ROOT}" \
    /

  [ "$status" -ne 0 ]
  [[ "$output" == *"directorio temporal de dependencias no válido"* ]]
}

@test "QML discovery consumes large ldconfig output under pipefail" {
  qtpaths6() { printf '%s\n' "${QML_ROOT}"; }
  ldconfig() {
    awk -v root="${QML_ROOT}" 'BEGIN {
      print "libQt6Core.so.6 (libc6,x86-64) => " root "/libQt6Core.so.6"
      for (i=0; i<40000; i++) print "libqa.so.1 (libc6,x86-64) => /qa/libqa.so.1"
    }'
  }
  export -f qtpaths6 ldconfig
  run bash -e -o pipefail -c 'source "$1"; grxfirma_runtime_qml_roots' \
    _ "${BATS_TEST_DIRNAME}/../runtime-dependencies.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *"${QML_ROOT}"* ]]
}

@test "QML discovery keeps qtpaths roots when optional ldconfig fails" {
  qtpaths6() { printf '%s\n' "${QML_ROOT}"; }
  ldconfig() { return 1; }
  export -f qtpaths6 ldconfig
  run bash -e -o pipefail -c 'source "$1"; grxfirma_runtime_qml_roots' \
    _ "${BATS_TEST_DIRNAME}/../runtime-dependencies.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *"${QML_ROOT}"* ]]
}
