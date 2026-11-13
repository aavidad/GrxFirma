#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# G-07: los binarios Linux salen como PIE y con RELRO completo (BIND_NOW).

setup() {
  command -v go >/dev/null 2>&1 || skip "go no disponible"
  command -v readelf >/dev/null 2>&1 || skip "readelf no disponible"
  command -v cc >/dev/null 2>&1 || skip "compilador C no disponible"
  export LC_ALL=C
  export SOURCE_DATE_EPOCH=1700000000
  export GOPROXY=off
  unset GOOS GOARCH
  # shellcheck disable=SC1091
  source "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
  MOD="${BATS_TEST_TMPDIR}/mod"
  mkdir -p "${MOD}/purego" "${MOD}/withcgo"
  printf 'module hardening\n\ngo 1.22\n' > "${MOD}/go.mod"
  printf 'package main\n\nfunc main() { println("ok") }\n' > "${MOD}/purego/main.go"
  printf 'package main\n\n// int one(void) { return 1; }\nimport "C"\n\nfunc main() { println(C.one()) }\n' \
    > "${MOD}/withcgo/main.go"
}

assert_hardened() {
  local binary="$1"
  readelf -h "${binary}" | grep -Eq 'Type:[[:space:]]+DYN'
  readelf -lW "${binary}" | grep -q 'GNU_RELRO'
  readelf -d "${binary}" | grep -Eq '\(FLAGS_1\).*NOW'
}

@test "binario Go puro: PIE y RELRO completo" {
  cd "${MOD}"
  grxfirma_go_build test "" -o "${BATS_TEST_TMPDIR}/purego" ./purego
  assert_hardened "${BATS_TEST_TMPDIR}/purego"
}

@test "binario con CGo: PIE y RELRO completo" {
  cd "${MOD}"
  CGO_ENABLED=1 grxfirma_go_build test "" -o "${BATS_TEST_TMPDIR}/withcgo" ./withcgo
  assert_hardened "${BATS_TEST_TMPDIR}/withcgo"
}

@test "la compilación endurecida es reproducible" {
  cd "${MOD}"
  grxfirma_go_build test "" -o "${BATS_TEST_TMPDIR}/one" ./withcgo
  grxfirma_go_build test "" -o "${BATS_TEST_TMPDIR}/two" ./withcgo
  cmp "${BATS_TEST_TMPDIR}/one" "${BATS_TEST_TMPDIR}/two"
}
