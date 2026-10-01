#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export MOCK_BIN="${BATS_TEST_TMPDIR}/bin"
  export SIGN_LOG="${BATS_TEST_TMPDIR}/codesign.log"
  export SIGN_ARGS_LOG="${BATS_TEST_TMPDIR}/codesign-args.log"
  export APP="${BATS_TEST_TMPDIR}/GrxFirma Desktop Qt.app"
  export SIGNER="${BATS_TEST_DIRNAME}/../codesign-bundle.sh"
  mkdir -p \
    "${MOCK_BIN}" \
    "${APP}/Contents/MacOS" \
    "${APP}/Contents/Resources" \
    "${APP}/Contents/Frameworks/QtCore.framework/Versions/A" \
    "${APP}/Contents/PlugIns/platforms/libqcocoa.plugin/Contents/MacOS"
  : > "${APP}/Contents/MacOS/grxfirma-gui-qml"
  : > "${APP}/Contents/MacOS/grxfirma-gui"
  : > "${APP}/Contents/MacOS/grxfirma"
  : > "${APP}/Contents/Frameworks/QtCore.framework/Versions/A/QtCore"
  : > "${APP}/Contents/PlugIns/platforms/libqcocoa.plugin/Contents/MacOS/libqcocoa"
  : > "${APP}/Contents/Resources/not-code"
  chmod 755 \
    "${APP}/Contents/MacOS/grxfirma-gui-qml" \
    "${APP}/Contents/MacOS/grxfirma-gui" \
    "${APP}/Contents/MacOS/grxfirma"

  cat > "${MOCK_BIN}/file" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${*: -1}" in
  */Contents/MacOS/*|*/Versions/A/*)
    echo "Mach-O 64-bit executable"
    ;;
  *)
    echo "ASCII text"
    ;;
esac
EOF
  cat > "${MOCK_BIN}/codesign" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ " $* " == *" --sign "* ]]; then
  printf '%s\n' "${*: -1}" >> "${SIGN_LOG:?}"
  printf '%s\n' "$*" >> "${SIGN_ARGS_LOG:?}"
fi
EOF
  chmod 755 "${MOCK_BIN}/file" "${MOCK_BIN}/codesign"
  export PATH="${MOCK_BIN}:${PATH}"
}

line_number() {
  local target="$1"
  grep -Fxn -- "${target}" "${SIGN_LOG}" | head -n 1 | cut -d: -f1
}

@test "firma Mach-O y bundles Qt desde el nivel mas profundo" {
  run env MACOS_CODESIGN_IDENTITY="Developer ID Application: Test" \
    bash "${SIGNER}" "${APP}"

  [ "$status" -eq 0 ]
  [ "$(grep -Fxc "${APP}/Contents/MacOS/grxfirma-gui-qml" "${SIGN_LOG}")" -eq 1 ]
  [ "$(grep -Fxc "${APP}/Contents/MacOS/grxfirma-gui" "${SIGN_LOG}")" -eq 1 ]
  [ "$(grep -Fxc "${APP}/Contents/MacOS/grxfirma" "${SIGN_LOG}")" -eq 1 ]
  [ "$(grep -Fxc "${APP}/Contents/Frameworks/QtCore.framework/Versions/A/QtCore" "${SIGN_LOG}")" -eq 1 ]
  [ "$(grep -Fxc "${APP}/Contents/Frameworks/QtCore.framework" "${SIGN_LOG}")" -eq 1 ]
  [ "$(grep -Fxc "${APP}/Contents/PlugIns/platforms/libqcocoa.plugin/Contents/MacOS/libqcocoa" "${SIGN_LOG}")" -eq 1 ]
  [ "$(grep -Fxc "${APP}/Contents/PlugIns/platforms/libqcocoa.plugin" "${SIGN_LOG}")" -eq 1 ]
  [ "$(tail -n 1 "${SIGN_LOG}")" = "${APP}" ]

  [ "$(line_number "${APP}/Contents/Frameworks/QtCore.framework/Versions/A/QtCore")" \
    -lt "$(line_number "${APP}/Contents/Frameworks/QtCore.framework")" ]
  [ "$(line_number "${APP}/Contents/PlugIns/platforms/libqcocoa.plugin/Contents/MacOS/libqcocoa")" \
    -lt "$(line_number "${APP}/Contents/PlugIns/platforms/libqcocoa.plugin")" ]
  [ "$(line_number "${APP}/Contents/Frameworks/QtCore.framework")" \
    -lt "$(line_number "${APP}")" ]
  [ "$(line_number "${APP}/Contents/PlugIns/platforms/libqcocoa.plugin")" \
    -lt "$(line_number "${APP}")" ]
}

@test "la firma ad hoc no solicita timestamp ni hardened runtime" {
  run env MACOS_CODESIGN_IDENTITY="-" bash "${SIGNER}" "${APP}"

  [ "$status" -eq 0 ]
  run grep -F -- "--timestamp" "${SIGN_ARGS_LOG}"
  [ "$status" -eq 1 ]
  run grep -F -- "--options runtime" "${SIGN_ARGS_LOG}"
  [ "$status" -eq 1 ]
  run grep -F -- "--sign -" "${SIGN_ARGS_LOG}"
  [ "$status" -eq 0 ]
}

@test "rechaza Desktop Qt sin el bootstrap IPC antes de firmar" {
  rm "${APP}/Contents/MacOS/grxfirma-gui"

  run env MACOS_CODESIGN_IDENTITY="Developer ID Application: Test" \
    bash "${SIGNER}" "${APP}"

  [ "$status" -ne 0 ]
  [[ "$output" == *"grxfirma-gui"* ]]
  [ ! -s "${SIGN_LOG}" ]
}

@test "rechaza un bundle de entrada simbolico" {
  local external="${BATS_TEST_TMPDIR}/external.app"
  mkdir -p "${external}"
  ln -s "${external}" "${BATS_TEST_TMPDIR}/linked.app"

  run env MACOS_CODESIGN_IDENTITY="Developer ID Application: Test" \
    bash "${SIGNER}" "${BATS_TEST_TMPDIR}/linked.app"

  [ "$status" -ne 0 ]
  [[ "$output" == *"enlace simbolico"* ]]
  [ ! -s "${SIGN_LOG}" ]
}
