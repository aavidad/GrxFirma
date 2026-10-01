#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export TEST_HOME="${BATS_TEST_TMPDIR}/home"
  export INSTALLER="${BATS_TEST_TMPDIR}/installer"
  export PROFILE="${TEST_HOME}/Library/Application Support/Firefox/Profiles/test.default"
  mkdir -p "${INSTALLER}/extensions" "${PROFILE}"
  cp "${BATS_TEST_DIRNAME}/../install-nativehost.sh" "${INSTALLER}/install-nativehost.sh"
  printf 'native-host' > "${INSTALLER}/grxfirma-nativehost"
  printf 'test-xpi' > "${INSTALLER}/extensions/dipgra-extension-firefox.xpi"
  printf 'chromium-zip' > "${INSTALLER}/extensions/dipgra-extension-chromium.zip"
  : > "${PROFILE}/prefs.js"
}

write_firefox_metadata() {
  local signed="$1"
  local hash
  hash="$(shasum -a 256 "${INSTALLER}/extensions/dipgra-extension-firefox.xpi" | awk '{print $1}')"
  printf '{"extension_id":"extension@dipgra.es","signed":%s,"xpi_sha256":"%s"}\n' "${signed}" "${hash}" \
    > "${INSTALLER}/extensions/dipgra-extension-firefox.metadata.json"
}

run_installer() {
  HOME="${TEST_HOME}" "${INSTALLER}/install-nativehost.sh"
}

@test "does not install an unsigned Firefox extension" {
  write_firefox_metadata false

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "installs a signed Firefox extension with matching hash" {
  write_firefox_metadata true

  run run_installer

  [ "$status" -eq 0 ]
  [ -f "${PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "rejects a Firefox extension modified after metadata generation" {
  write_firefox_metadata true
  printf 'tampered' >> "${INSTALLER}/extensions/dipgra-extension-firefox.xpi"

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "rejects ambiguous Firefox metadata with duplicate keys" {
  local hash
  hash="$(shasum -a 256 "${INSTALLER}/extensions/dipgra-extension-firefox.xpi" | awk '{print $1}')"
  printf '{"extension_id":"extension@dipgra.es","signed":false,"signed":true,"xpi_sha256":"%s"}\n' "${hash}" \
    > "${INSTALLER}/extensions/dipgra-extension-firefox.metadata.json"

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "writes valid manifests when the home path needs JSON escaping" {
  local quoted_home="${BATS_TEST_TMPDIR}/home-\"quoted\\slash-á"

  run env HOME="${quoted_home}" "${INSTALLER}/install-nativehost.sh"

  [ "$status" -eq 0 ]
  python3 - "${quoted_home}" <<'PY'
import json
import pathlib
import sys

home = pathlib.Path(sys.argv[1])
manifest = json.loads(
    (
        home
        / "Library/Application Support/Google/Chrome/NativeMessagingHosts/com.grxfirma.native.json"
    ).read_text(encoding="utf-8")
)
assert manifest["path"] == str(
    home / "Library/Application Support/GrxFirma/NativeHost/grxfirma-nativehost"
)
PY
}

@test "registers only HTTPS Chromium update channels and allowed origins" {
  local chrome_id="abcdefghijklmnopabcdefghijklmnop"
  local edge_id="ponmlkjihgfedcbaponmlkjihgfedcba"

  run env \
    HOME="${TEST_HOME}" \
    GRXFIRMA_CHROMIUM_EXTENSION_ID="${chrome_id}" \
    GRXFIRMA_EDGE_EXTENSION_ID="${edge_id}" \
    GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL="https://updates.example.test/chrome.xml" \
    GRXFIRMA_EDGE_EXTENSION_UPDATE_URL="https://updates.example.test/edge.xml" \
    "${INSTALLER}/install-nativehost.sh"

  [ "$status" -eq 0 ]
  python3 - "${TEST_HOME}" "${chrome_id}" "${edge_id}" <<'PY'
import json
import pathlib
import sys

home = pathlib.Path(sys.argv[1])
chrome_id, edge_id = sys.argv[2:]
manifest = json.loads((home / "Library/Application Support/Google/Chrome/NativeMessagingHosts/com.grxfirma.native.json").read_text())
assert f"chrome-extension://{chrome_id}/" in manifest["allowed_origins"]
assert f"chrome-extension://{edge_id}/" in manifest["allowed_origins"]
chrome = json.loads((home / f"Library/Application Support/Google/Chrome/External Extensions/{chrome_id}.json").read_text())
edge = json.loads((home / f"Library/Application Support/Microsoft Edge/External Extensions/{edge_id}.json").read_text())
assert chrome == {"external_update_url": "https://updates.example.test/chrome.xml"}
assert edge == {"external_update_url": "https://updates.example.test/edge.xml"}
PY
}

@test "rejects non-HTTPS Chromium update channels" {
  run env \
    HOME="${TEST_HOME}" \
    GRXFIRMA_CHROMIUM_EXTENSION_ID="abcdefghijklmnopabcdefghijklmnop" \
    GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL="http://updates.example.test/chrome.xml" \
    "${INSTALLER}/install-nativehost.sh"

  [ "$status" -ne 0 ]
  [[ "$output" == *"debe usar HTTPS"* ]]
  [ ! -e "${TEST_HOME}/Library/Application Support/GrxFirma/NativeHost/grxfirma-nativehost" ]
  [ ! -e "${TEST_HOME}/Library/Application Support/Google/Chrome/NativeMessagingHosts/com.grxfirma.native.json" ]
}

@test "removes an obsolete Chromium external update registration" {
  local chrome_id="abcdefghijklmnopabcdefghijklmnop"

  run env \
    HOME="${TEST_HOME}" \
    GRXFIRMA_CHROMIUM_EXTENSION_ID="${chrome_id}" \
    GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL="https://updates.example.test/chrome.xml" \
    "${INSTALLER}/install-nativehost.sh"
  [ "$status" -eq 0 ]
  [ -f "${TEST_HOME}/Library/Application Support/Google/Chrome/External Extensions/${chrome_id}.json" ]

  run env \
    HOME="${TEST_HOME}" \
    GRXFIRMA_CHROMIUM_EXTENSION_ID="${chrome_id}" \
    "${INSTALLER}/install-nativehost.sh"

  [ "$status" -eq 0 ]
  [ ! -e "${TEST_HOME}/Library/Application Support/Google/Chrome/External Extensions/${chrome_id}.json" ]
}
