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
  printf 'test-xpi' > "${INSTALLER}/extensions/grxfirma-extension-firefox.xpi"
  printf 'chromium-zip' > "${INSTALLER}/extensions/grxfirma-extension-chromium.zip"
  : > "${PROFILE}/prefs.js"
}

write_firefox_metadata() {
  local signed="$1"
  local hash
  hash="$(shasum -a 256 "${INSTALLER}/extensions/grxfirma-extension-firefox.xpi" | awk '{print $1}')"
  printf '{"extension_id":"grxfirma@aavidad.github.io","signed":%s,"xpi_sha256":"%s"}\n' "${signed}" "${hash}" \
    > "${INSTALLER}/extensions/grxfirma-extension-firefox.metadata.json"
}

run_installer() {
  HOME="${TEST_HOME}" "${INSTALLER}/install-nativehost.sh"
}

@test "does not install an unsigned Firefox extension" {
  write_firefox_metadata false

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${PROFILE}/extensions/grxfirma@aavidad.github.io.xpi" ]
}

@test "installs a signed Firefox extension with matching hash" {
  write_firefox_metadata true

  run run_installer

  [ "$status" -eq 0 ]
  [ -f "${PROFILE}/extensions/grxfirma@aavidad.github.io.xpi" ]
}

@test "rejects a Firefox extension modified after metadata generation" {
  write_firefox_metadata true
  printf 'tampered' >> "${INSTALLER}/extensions/grxfirma-extension-firefox.xpi"

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${PROFILE}/extensions/grxfirma@aavidad.github.io.xpi" ]
}

@test "rejects ambiguous Firefox metadata with duplicate keys" {
  local hash
  hash="$(shasum -a 256 "${INSTALLER}/extensions/grxfirma-extension-firefox.xpi" | awk '{print $1}')"
  printf '{"extension_id":"grxfirma@aavidad.github.io","signed":false,"signed":true,"xpi_sha256":"%s"}\n' "${hash}" \
    > "${INSTALLER}/extensions/grxfirma-extension-firefox.metadata.json"

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${PROFILE}/extensions/grxfirma@aavidad.github.io.xpi" ]
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

@test "retira los hosts y la extension de versiones anteriores propios" {
  local support="${TEST_HOME}/Library/Application Support"
  local host_bin="${support}/GrxFirma/NativeHost/grxfirma-nativehost"
  local chrome_dir="${support}/Google/Chrome/NativeMessagingHosts"
  local edge_dir="${support}/Microsoft Edge/NativeMessagingHosts"
  local firefox_dir="${support}/Mozilla/NativeMessagingHosts"
  local ext_dir="${support}/GrxFirma/Extensions"
  mkdir -p "${chrome_dir}" "${edge_dir}" "${firefox_dir}" "${ext_dir}/firefox" "${ext_dir}/chromium" "${PROFILE}/extensions"
  printf '{\n  "name": "com.dipgra.grxfirma",\n  "path": "%s",\n  "type": "stdio"\n}\n' "${host_bin}" \
    > "${chrome_dir}/com.dipgra.grxfirma.json"
  printf '{\n  "name": "com.dipgra.portafirmas",\n  "path": "%s",\n  "type": "stdio"\n}\n' "${host_bin}" \
    > "${firefox_dir}/com.dipgra.portafirmas.json"
  printf '{\n  "name": "com.dipgra.portafirmas",\n  "path": "/opt/otro/host",\n  "type": "stdio"\n}\n' \
    > "${edge_dir}/com.dipgra.portafirmas.json"
  printf 'xpi-anterior' > "${ext_dir}/firefox/dipgra-extension-firefox.xpi"
  printf 'zip-anterior' > "${ext_dir}/chromium/dipgra-extension-chromium.zip"
  cp "${ext_dir}/firefox/dipgra-extension-firefox.xpi" "${PROFILE}/extensions/extension@dipgra.es.xpi"
  write_firefox_metadata false

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${chrome_dir}/com.dipgra.grxfirma.json" ]
  [ ! -e "${firefox_dir}/com.dipgra.portafirmas.json" ]
  [ -f "${edge_dir}/com.dipgra.portafirmas.json" ]
  [ ! -e "${PROFILE}/extensions/extension@dipgra.es.xpi" ]
  [ ! -e "${ext_dir}/firefox/dipgra-extension-firefox.xpi" ]
  [ ! -e "${ext_dir}/chromium/dipgra-extension-chromium.zip" ]
  [ -f "${chrome_dir}/io.github.aavidad.grxfirma.json" ]
}

@test "conserva una extension anterior que no coincide con la instalada por GrxFirma" {
  local ext_dir="${TEST_HOME}/Library/Application Support/GrxFirma/Extensions"
  mkdir -p "${ext_dir}/firefox" "${PROFILE}/extensions"
  printf 'xpi-anterior' > "${ext_dir}/firefox/dipgra-extension-firefox.xpi"
  printf 'xpi-del-usuario' > "${PROFILE}/extensions/extension@dipgra.es.xpi"
  write_firefox_metadata false

  run run_installer

  [ "$status" -eq 0 ]
  [ -f "${PROFILE}/extensions/extension@dipgra.es.xpi" ]
}

@test "no registra el host de portafirmas y retira solo el que apunta a GrxFirma" {
  local support="${TEST_HOME}/Library/Application Support"
  local host_bin="${support}/GrxFirma/NativeHost/grxfirma-nativehost"
  local chrome_dir="${support}/Google/Chrome/NativeMessagingHosts"
  local edge_dir="${support}/Microsoft Edge/NativeMessagingHosts"
  local firefox_dir="${support}/Mozilla/NativeMessagingHosts"
  mkdir -p "${chrome_dir}" "${edge_dir}" "${firefox_dir}"
  printf '{\n  "name": "io.github.aavidad.portafirmas",\n  "path": "%s",\n  "type": "stdio"\n}\n' "${host_bin}" \
    > "${chrome_dir}/io.github.aavidad.portafirmas.json"
  printf '{\n  "name": "io.github.aavidad.portafirmas",\n  "path": "%s",\n  "type": "stdio"\n}\n' "${host_bin}" \
    > "${firefox_dir}/io.github.aavidad.portafirmas.json"
  printf '{\n  "name": "io.github.aavidad.portafirmas",\n  "path": "/opt/portafirmas/host",\n  "type": "stdio"\n}\n' \
    > "${edge_dir}/io.github.aavidad.portafirmas.json"
  write_firefox_metadata false

  run run_installer

  [ "$status" -eq 0 ]
  [ ! -e "${chrome_dir}/io.github.aavidad.portafirmas.json" ]
  [ ! -e "${firefox_dir}/io.github.aavidad.portafirmas.json" ]
  [ -f "${edge_dir}/io.github.aavidad.portafirmas.json" ]
  [ -f "${chrome_dir}/io.github.aavidad.grxfirma.json" ]
  run grep -Rq 'portafirmas@dipgra.es\|ipkpimgjhkjibkbhfdhggjldlaetbcoa' "${chrome_dir}" "${firefox_dir}"
  [ "$status" -eq 1 ]
}
