#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

@test "los builders macOS compilan el bootstrap IPC de Desktop Qt" {
  for script in \
    "${BATS_TEST_DIRNAME}/../build-desktop-qml.sh" \
    "${BATS_TEST_DIRNAME}/../build-suite.sh"; do
    run grep -F -- "./cmd/grxfirma-gui" "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- "grxfirma-gui" "${script}"
    [ "$status" -eq 0 ]
  done
}

@test "Qt arm64 preincluye ACLE sin desactivar errores del compilador" {
  local project="${BATS_TEST_DIRNAME}/../../../cmd/gui-qml/grxfirma_qt.pro"

  run grep -F -- \
    "contains(QMAKE_APPLE_DEVICE_ARCHS, arm64)" \
    "${project}"
  [ "$status" -eq 0 ]
  run grep -F -- "QMAKE_CXXFLAGS += -include arm_acle.h" "${project}"
  [ "$status" -eq 0 ]
  run grep -E -- "-Wno-error|-Wno-implicit-function-declaration" "${project}"
  [ "$status" -eq 1 ]
}

@test "Qt macOS sustituye la biblioteca OpenGL heredada del mkspec" {
  local project="${BATS_TEST_DIRNAME}/../../../cmd/gui-qml/grxfirma_qt.pro"

  run grep -F -- "QMAKE_LIBS_OPENGL = -framework OpenGL" "${project}"
  [ "$status" -eq 0 ]
  run grep -E -- "-framework[[:space:]]+AGL|AGL\\.framework" "${project}"
  [ "$status" -eq 1 ]
}

@test "el helper elimina solo las dos referencias AGL conocidas de Qt 6.7.3" {
  local helper="${BATS_TEST_DIRNAME}/../qmake-compat.sh"
  local qt_libs="${BATS_TEST_TMPDIR}/qt/lib"
  local prl="${qt_libs}/QtGui.framework/Versions/A/Resources/QtGui.prl"
  local fake_qmake="${BATS_TEST_TMPDIR}/qmake6"
  mkdir -p "$(dirname "${prl}")"
  cat > "${prl}" <<'EOF'
QMAKE_PRL_LIBS = -framework OpenGL -framework AGL -framework AppKit
QMAKE_PRL_LIBS_FOR_CMAKE = -framework OpenGL;-framework AGL;-framework AppKit
EOF
  cat > "${fake_qmake}" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "-query QT_VERSION") printf '%s\n' "6.7.3" ;;
  "-query QT_INSTALL_LIBS") printf '%s\n' "${FAKE_QT_LIBS}" ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "${fake_qmake}"

  # La expansión de $1 y $2 debe hacerla el bash hijo, no este test.
  # shellcheck disable=SC2016
  run env FAKE_QT_LIBS="${qt_libs}" bash -c \
    'source "$1"; grxfirma_prepare_qmake_macos "$2"; grxfirma_prepare_qmake_macos "$2"' \
    _ "${helper}" "${fake_qmake}"
  [ "$status" -eq 0 ]
  run grep -F -- "-framework AGL" "${prl}"
  [ "$status" -eq 1 ]
  run grep -F -- "-framework OpenGL" "${prl}"
  [ "$status" -eq 0 ]
  run grep -F -- "-framework AppKit" "${prl}"
  [ "$status" -eq 0 ]
}

@test "el helper rechaza un QtGui.prl 6.7.3 de formato desconocido" {
  local helper="${BATS_TEST_DIRNAME}/../qmake-compat.sh"
  local qt_libs="${BATS_TEST_TMPDIR}/qt-unexpected/lib"
  local prl="${qt_libs}/QtGui.framework/Versions/A/Resources/QtGui.prl"
  local fake_qmake="${BATS_TEST_TMPDIR}/qmake6-unexpected"
  mkdir -p "$(dirname "${prl}")"
  printf '%s\n' \
    "QMAKE_PRL_LIBS = -framework OpenGL -framework AGL -framework AppKit" \
    > "${prl}"
  cat > "${fake_qmake}" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "-query QT_VERSION") printf '%s\n' "6.7.3" ;;
  "-query QT_INSTALL_LIBS") printf '%s\n' "${FAKE_QT_LIBS}" ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "${fake_qmake}"

  # La expansión de $1 y $2 debe hacerla el bash hijo, no este test.
  # shellcheck disable=SC2016
  run env FAKE_QT_LIBS="${qt_libs}" bash -c \
    'source "$1"; grxfirma_prepare_qmake_macos "$2"' \
    _ "${helper}" "${fake_qmake}"
  [ "$status" -ne 0 ]
  [[ "$output" == *"inesperado"* ]]
}

@test "el helper elimina AGL del Makefile generado solo para Qt 6.7.3" {
  local helper="${BATS_TEST_DIRNAME}/../qmake-compat.sh"
  local makefile="${BATS_TEST_TMPDIR}/Makefile"
  local fake_qmake="${BATS_TEST_TMPDIR}/qmake6-makefile"
  printf '%s\n' \
    "LIBS = -framework OpenGL -framework AGL -framework AppKit" \
    > "${makefile}"
  cat > "${fake_qmake}" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "-query QT_VERSION") printf '%s\n' "${FAKE_QT_VERSION}" ;;
  *) exit 1 ;;
esac
EOF
  chmod +x "${fake_qmake}"

  # La expansión de los argumentos debe hacerla el bash hijo.
  # shellcheck disable=SC2016
  run env FAKE_QT_VERSION=6.7.3 bash -c \
    'source "$1"; grxfirma_prepare_qmake_makefile_macos "$2" "$3"' \
    _ "${helper}" "${fake_qmake}" "${makefile}"
  [ "$status" -eq 0 ]
  run grep -F -- "-framework AGL" "${makefile}"
  [ "$status" -eq 1 ]
  run grep -F -- "-framework OpenGL" "${makefile}"
  [ "$status" -eq 0 ]
  run grep -F -- "-framework AppKit" "${makefile}"
  [ "$status" -eq 0 ]

  printf '%s\n' "LIBS = -framework AGL" > "${makefile}"
  # La expansión de los argumentos debe hacerla el bash hijo.
  # shellcheck disable=SC2016
  run env FAKE_QT_VERSION=6.8.0 bash -c \
    'source "$1"; grxfirma_prepare_qmake_makefile_macos "$2" "$3"' \
    _ "${helper}" "${fake_qmake}" "${makefile}"
  [ "$status" -ne 0 ]
  [[ "$output" == *"parche compatible explícito"* ]]
}

@test "ambos builders macOS preparan Qt y verifican el Makefile" {
  for script in \
    "${BATS_TEST_DIRNAME}/../build-desktop-qml.sh" \
    "${BATS_TEST_DIRNAME}/../build-suite.sh"; do
    # Se busca texto literal de los builders, no una variable de este test.
    # shellcheck disable=SC2016
    run grep -F -- 'source "${ROOT_DIR}/packaging/macos/qmake-compat.sh"' "${script}"
    [ "$status" -eq 0 ]
    # shellcheck disable=SC2016
    run grep -F -- 'grxfirma_prepare_qmake_macos "${QMAKE_CMD}"' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'grxfirma_prepare_qmake_makefile_macos' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'grxfirma_assert_qmake_makefile_without_agl' "${script}"
    [ "$status" -eq 0 ]
  done
}

@test "los inventarios TAR exigen los tres ejecutables Desktop Qt" {
  for script in \
    "${BATS_TEST_DIRNAME}/../build-desktop-qml.sh" \
    "${BATS_TEST_DIRNAME}/../build-suite.sh"; do
    run grep -F -- \
      "Contents/MacOS/grxfirma-gui-qml" \
      "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- \
      "Contents/MacOS/grxfirma-gui\"" \
      "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- \
      "Contents/MacOS/grxfirma\"" \
      "${script}"
    [ "$status" -eq 0 ]
  done
}

@test "instaladores firma y PKG exigen el bootstrap IPC" {
  for script in \
    "${BATS_TEST_DIRNAME}/../install-desktop-qml.sh" \
    "${BATS_TEST_DIRNAME}/../install-suite.sh" \
    "${BATS_TEST_DIRNAME}/../codesign-bundle.sh" \
    "${BATS_TEST_DIRNAME}/../validate-pkg.sh"; do
    run grep -F -- "Contents/MacOS/grxfirma-gui" "${script}"
    [ "$status" -eq 0 ]
  done
}

@test "el builder Desktop aislado limpia una stage previa antes de compilar" {
  run grep -F -- \
    "rm -rf \"\$STAGE_DIR\" \"\$BUILD_DIR\"" \
    "${BATS_TEST_DIRNAME}/../build-desktop-qml.sh"
  [ "$status" -eq 0 ]
  run grep -F -- \
    "rm -f \"\$TAR_PATH\"" \
    "${BATS_TEST_DIRNAME}/../build-desktop-qml.sh"
  [ "$status" -eq 0 ]
}

@test "macdeployqt recibe el arbol QML y firma solo despues del ensamblado" {
  for script in \
    "${BATS_TEST_DIRNAME}/../build-desktop-qml.sh" \
    "${BATS_TEST_DIRNAME}/../build-suite.sh"; do
    run grep -F -- "-qmldir=\"\${ROOT_DIR}/cmd/gui-qml/qml\"" "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- '-codesign=-' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'MACOS_CODESIGN_IDENTITY=-' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'packaging/macos/codesign-bundle.sh' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'codesign --verify --deep --strict' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'Contents/Resources/qml' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'Contents/Resources/assets' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'assets/branding/grxfirma-diputacion.ico' "${script}"
    [ "$status" -eq 0 ]
    run grep -F -- 'Contents/MacOS/qml' "${script}"
    [ "$status" -eq 1 ]
    run grep -F -- 'Contents/MacOS/assets' "${script}"
    [ "$status" -eq 1 ]

    version_line="$(
      grep -n -F 'Contents/Resources/VERSION.txt' "${script}" |
        tail -n 1 |
        cut -d: -f1
    )"
    deploy_line="$(
      grep -n -F "macdeployqt " "${script}" |
        tail -n 1 |
        cut -d: -f1
    )"
    sign_line="$(
      grep -n -F 'MACOS_CODESIGN_IDENTITY=-' "${script}" |
        tail -n 1 |
        cut -d: -f1
    )"
    verify_line="$(
      grep -n -F 'codesign --verify --deep --strict' "${script}" |
        tail -n 1 |
        cut -d: -f1
    )"
    [ -n "${version_line}" ]
    [ -n "${deploy_line}" ]
    [ -n "${sign_line}" ]
    [ -n "${verify_line}" ]
    [ "${version_line}" -lt "${deploy_line}" ]
    [ "${deploy_line}" -lt "${sign_line}" ]
    [ "${sign_line}" -lt "${verify_line}" ]
  done
}

@test "el verificador oficial exige y firma los tres ejecutables Desktop Qt" {
  local verifier="${BATS_TEST_DIRNAME}/../../../scripts/release/verify-macos-release.sh"

  run grep -F -- \
    "desktop_bootstrap=\"\${desktop_app}/Contents/MacOS/grxfirma-gui\"" \
    "${verifier}"
  [ "$status" -eq 0 ]
  run grep -F -- "for desktop_executable in" "${verifier}"
  [ "$status" -eq 0 ]
  run grep -F -- "verify_code_signature \"\${desktop_executable}\"" "${verifier}"
  [ "$status" -eq 0 ]
}

@test "la version Desktop Qt forma parte de los inventarios e instaladores" {
  for script in \
    "${BATS_TEST_DIRNAME}/../build-desktop-qml.sh" \
    "${BATS_TEST_DIRNAME}/../build-suite.sh" \
    "${BATS_TEST_DIRNAME}/../install-desktop-qml.sh" \
    "${BATS_TEST_DIRNAME}/../install-suite.sh" \
    "${BATS_TEST_DIRNAME}/../validate-pkg.sh"; do
    run grep -F -- "Contents/Resources/VERSION.txt" "${script}"
    [ "$status" -eq 0 ]
  done
}

@test "los README macOS documentan el bootstrap IPC obligatorio" {
  for readme in \
    "${BATS_TEST_DIRNAME}/../README_DESKTOP_QML_MACOS.md" \
    "${BATS_TEST_DIRNAME}/../README_MACOS_SUITE.md"; do
    run grep -F -- 'grxfirma-gui' "${readme}"
    [ "$status" -eq 0 ]
  done
}
