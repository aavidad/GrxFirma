#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../../.." && pwd)"
  TMP_ROOT="$(mktemp -d)"
}

teardown() {
  rm -rf "${TMP_ROOT}"
  if [[ -n "${GENERATED_FIXTURE_DIR:-}" ]]; then
    rm -rf "${GENERATED_FIXTURE_DIR}"
  fi
}

set_valid_release_environment() {
  local encoded
  encoded="$(printf 'test-material' | base64 | tr -d '\n')"
  export WINDOWS_SIGNING_PFX_BASE64="${encoded}"
  export WINDOWS_SIGNING_PFX_PASSWORD="test-password"
  export WINDOWS_SIGNING_CERT_THUMBPRINT="0123456789ABCDEF0123456789ABCDEF01234567"
  export ANDROID_SIGNING_KEYSTORE_BASE64="${encoded}"
  export GRXFIRMA_ANDROID_KEYSTORE_PASSWORD="test-password"
  export GRXFIRMA_ANDROID_KEY_ALIAS="release"
  export GRXFIRMA_ANDROID_KEY_PASSWORD="test-password"
  GRXFIRMA_ANDROID_SIGNING_CERT_SHA256="$(printf 'A%.0s' {1..64})"
  export GRXFIRMA_ANDROID_SIGNING_CERT_SHA256
  export ANDROID_QA_KEYSTORE_BASE64="${encoded}"
  export ANDROID_QA_KEYSTORE_PASSWORD="test-password"
  export ANDROID_QA_KEY_ALIAS="qa"
  export ANDROID_QA_KEY_PASSWORD="test-password"
  ANDROID_QA_SIGNING_CERT_SHA256="$(printf 'B%.0s' {1..64})"
  export ANDROID_QA_SIGNING_CERT_SHA256
  export WEB_EXT_API_KEY="user:test"
  export WEB_EXT_API_SECRET="test-secret"
  export MACOS_APPLICATION_CERT_P12_BASE64="${encoded}"
  export MACOS_APPLICATION_CERT_PASSWORD="test-password"
  export MACOS_INSTALLER_CERT_P12_BASE64="${encoded}"
  export MACOS_INSTALLER_CERT_PASSWORD="test-password"
  export MACOS_CODESIGN_IDENTITY="Developer ID Application: Example (ABCDE12345)"
  export MACOS_INSTALLER_IDENTITY="Developer ID Installer: Example (ABCDE12345)"
  export MACOS_TEAM_ID="ABCDE12345"
  export MACOS_NOTARY_KEY_P8_BASE64="${encoded}"
  export MACOS_NOTARY_KEY_ID="FGHIJ67890"
  export MACOS_NOTARY_ISSUER_ID="01234567-89ab-cdef-0123-456789abcdef"
  export RELEASE_GPG_PRIVATE_KEY_BASE64="${encoded}"
  export RELEASE_GPG_PASSPHRASE="test-password"
  export RELEASE_GPG_FINGERPRINT="0123456789ABCDEF0123456789ABCDEF01234567"
}

@test "official release preflight rejects an unpinned Android certificate" {
  set_valid_release_environment
  export GRXFIRMA_ANDROID_SIGNING_CERT_SHA256="not-a-fingerprint"
  run bash "${REPO_ROOT}/scripts/release/check-official-release-env.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"64 hexadecimales"* ]]
}

@test "official release preflight rejects missing credentials" {
  run env -i PATH="${PATH}" bash "${REPO_ROOT}/scripts/release/check-official-release-env.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"faltan credenciales"* ]]
}

@test "local official release entrypoint is disabled" {
  run bash "${REPO_ROOT}/scripts/release/build-official-release.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"publicacion oficial local esta deshabilitada"* ]]
}

@test "official release preflight accepts complete well-formed bindings" {
  set_valid_release_environment
  run bash "${REPO_ROOT}/scripts/release/check-official-release-env.sh"
  [ "$status" -eq 0 ]
}

@test "official release preflight rejects an unpinned Windows certificate" {
  set_valid_release_environment
  export WINDOWS_SIGNING_CERT_THUMBPRINT="not-a-thumbprint"
  run bash "${REPO_ROOT}/scripts/release/check-official-release-env.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"40 hexadecimales"* ]]
}

@test "official release preflight accepts SignPath without a PFX on an official tag" {
  set_valid_release_environment
  run env -u WINDOWS_SIGNING_PFX_BASE64 -u WINDOWS_SIGNING_PFX_PASSWORD \
    SIGNPATH_API_TOKEN="test-token" \
    SIGNPATH_ORGANIZATION_ID="00000000-0000-0000-0000-000000000000" \
    GITHUB_REF_NAME="v1.2.3" \
    bash "${REPO_ROOT}/scripts/release/check-official-release-env.sh"
  [ "$status" -eq 0 ]
  [[ "$output" == *"firma Windows: signpath"* ]]
}

@test "official release preflight rejects SignPath alone on a test tag" {
  set_valid_release_environment
  run env -u WINDOWS_SIGNING_PFX_BASE64 -u WINDOWS_SIGNING_PFX_PASSWORD \
    SIGNPATH_API_TOKEN="test-token" \
    SIGNPATH_ORGANIZATION_ID="00000000-0000-0000-0000-000000000000" \
    GITHUB_REF_NAME="v1.2.3-rc.1" \
    bash "${REPO_ROOT}/scripts/release/check-official-release-env.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"no usa SignPath"* ]]
}

@test "Windows signing selector prefers SignPath on official tags and PFX on test tags" {
  local both=(
    WINDOWS_SIGNING_PFX_BASE64="dGVzdA=="
    WINDOWS_SIGNING_PFX_PASSWORD="test-password"
    SIGNPATH_API_TOKEN="test-token"
    SIGNPATH_ORGANIZATION_ID="test-org"
  )
  run env "${both[@]}" \
    bash "${REPO_ROOT}/scripts/release/select-windows-signing-mode.sh" v1.2.3
  [ "$status" -eq 0 ]
  [[ "$output" == *"signpath" ]]
  run env "${both[@]}" \
    bash "${REPO_ROOT}/scripts/release/select-windows-signing-mode.sh" v1.2.3-rc.1
  [ "$status" -eq 0 ]
  [ "$output" = "pfx" ]
}

@test "Windows signing selector rejects incomplete or missing credentials" {
  run env -i PATH="${PATH}" SIGNPATH_API_TOKEN="test-token" \
    bash "${REPO_ROOT}/scripts/release/select-windows-signing-mode.sh" v1.2.3
  [ "$status" -ne 0 ]
  [[ "$output" == *"SIGNPATH_ORGANIZATION_ID"* ]]
  run env -i PATH="${PATH}" WINDOWS_SIGNING_PFX_PASSWORD="test-password" \
    bash "${REPO_ROOT}/scripts/release/select-windows-signing-mode.sh" v1.2.3
  [ "$status" -ne 0 ]
  [[ "$output" == *"WINDOWS_SIGNING_PFX_BASE64"* ]]
  run env -i PATH="${PATH}" \
    bash "${REPO_ROOT}/scripts/release/select-windows-signing-mode.sh" v1.2.3
  [ "$status" -ne 0 ]
  [[ "$output" == *"falta una vía de firma Windows"* ]]
}

@test "isolated Firefox preflight rejects missing signing credentials" {
  run env \
    -u WEB_EXT_API_SECRET \
    WEB_EXT_API_KEY="user:test" \
    bash "${REPO_ROOT}/scripts/release/check-firefox-signing-env.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"WEB_EXT_API_SECRET"* ]]
}

@test "isolated Firefox preflight accepts complete signing credentials" {
  run env \
    WEB_EXT_API_KEY="user:test" \
    WEB_EXT_API_SECRET="test-secret" \
    bash "${REPO_ROOT}/scripts/release/check-firefox-signing-env.sh"
  [ "$status" -eq 0 ]
}

@test "official tag must match strict SemVer and VERSION.txt" {
  printf '1.2.3-rc.1\n' > "${TMP_ROOT}/VERSION.txt"
  run python3 "${REPO_ROOT}/scripts/release/validate-official-tag.py" \
    v1.2.3-rc.1 --version-file "${TMP_ROOT}/VERSION.txt"
  [ "$status" -eq 0 ]

  run python3 "${REPO_ROOT}/scripts/release/validate-official-tag.py" \
    v1.bad.3 --version-file "${TMP_ROOT}/VERSION.txt"
  [ "$status" -ne 0 ]
}

@test "secret scanner allows defensive TLS keylog literals" {
  mkdir -p "${TMP_ROOT}/source"
  printf 'markers = @(\"CLIENT_RANDOM \", \"EXPORTER_SECRET \")\n' \
    > "${TMP_ROOT}/source/validator.ps1"
  printf 'fixture = \"CLIENT_RANDOM deadbeef test-only\"\n' \
    > "${TMP_ROOT}/source/validator_test.ps1"

  run bash "${REPO_ROOT}/scripts/release/check-secrets.sh" "${TMP_ROOT}/source"
  [ "$status" -eq 0 ]
}

@test "secret scanner rejects a complete TLS keylog entry" {
  mkdir -p "${TMP_ROOT}/artifacts"
  printf 'CLIENT_RANDOM %064d %096d\n' 0 0 \
    > "${TMP_ROOT}/artifacts/tls-debug.txt"

  run bash "${REPO_ROOT}/scripts/release/check-secrets.sh" "${TMP_ROOT}/artifacts"
  [ "$status" -ne 0 ]
  [[ "$output" == *"formato SSLKEYLOGFILE"* ]]
}

@test "secret scanner rejects sensitive filenames in one traversal" {
  mkdir -p "${TMP_ROOT}/source/config"
  printf 'not-a-real-secret\n' > "${TMP_ROOT}/source/config/.env"
  printf 'not-a-real-key\n' > "${TMP_ROOT}/source/id_rsa"

  run bash "${REPO_ROOT}/scripts/release/check-secrets.sh" "${TMP_ROOT}/source"
  [ "$status" -ne 0 ]
  [[ "$output" == *"${TMP_ROOT}/source/config/.env"* ]]
  [[ "$output" == *"${TMP_ROOT}/source/id_rsa"* ]]
}

@test "repository secret scanner ignores generated files but still checks their names" {
  GENERATED_FIXTURE_DIR="${REPO_ROOT}/mobile/android/app/build/secret-scanner-fixture"
  mkdir -p "${GENERATED_FIXTURE_DIR}"
  local generated="${GENERATED_FIXTURE_DIR}/generated.txt"
  local named="${GENERATED_FIXTURE_DIR}/id_rsa"
  printf 'CLIENT_RANDOM %064d %064d\n' 0 0 > "${generated}"

  run bash "${REPO_ROOT}/scripts/release/check-secrets.sh"
  [ "$status" -eq 0 ]

  printf 'not-a-real-key\n' > "${named}"
  run bash "${REPO_ROOT}/scripts/release/check-secrets.sh"
  [ "$status" -ne 0 ]
  [[ "$output" == *"${named}"* ]]
}

@test "OpenPGP checksum verification detects artifact tampering" {
  export GNUPGHOME="${TMP_ROOT}/keygen"
  mkdir -m 700 "${GNUPGHOME}"
  gpg --batch --quiet --pinentry-mode loopback --passphrase test-passphrase \
    --quick-generate-key "GrxFirma Release Test <release-test@example.invalid>" rsa2048 sign 0
  fingerprint="$(gpg --batch --with-colons --fingerprint | awk -F: '$1 == "fpr" { print $10; exit }')"
  secret_key="$(
    printf 'test-passphrase' |
      gpg --batch --quiet --pinentry-mode loopback --passphrase-fd 0 --export-secret-keys |
      base64 | tr -d '\n'
  )"
  unset GNUPGHOME

  mkdir -p "${TMP_ROOT}/artifacts"
  printf 'signed payload\n' > "${TMP_ROOT}/artifacts/payload.bin"
  export RELEASE_GPG_PRIVATE_KEY_BASE64="${secret_key}"
  export RELEASE_GPG_PASSPHRASE="test-passphrase"
  export RELEASE_GPG_FINGERPRINT="${fingerprint}"

  run bash "${REPO_ROOT}/scripts/release/sign-checksums.sh" \
    "${TMP_ROOT}/artifacts" --export-public-key "${TMP_ROOT}/artifacts/RELEASE-SIGNING-KEY.asc"
  [ "$status" -eq 0 ]
  run bash "${REPO_ROOT}/scripts/release/verify-signed-checksums.sh" \
    "${TMP_ROOT}/artifacts" \
    "${TMP_ROOT}/artifacts/SHA256SUMS.txt" \
    "${TMP_ROOT}/artifacts/SHA256SUMS.txt.asc" \
    "${TMP_ROOT}/artifacts/RELEASE-SIGNING-KEY.asc"
  [ "$status" -eq 0 ]

  printf 'tampered\n' >> "${TMP_ROOT}/artifacts/payload.bin"
  run bash "${REPO_ROOT}/scripts/release/verify-signed-checksums.sh" \
    "${TMP_ROOT}/artifacts" \
    "${TMP_ROOT}/artifacts/SHA256SUMS.txt" \
    "${TMP_ROOT}/artifacts/SHA256SUMS.txt.asc" \
    "${TMP_ROOT}/artifacts/RELEASE-SIGNING-KEY.asc"
  [ "$status" -ne 0 ]
}
