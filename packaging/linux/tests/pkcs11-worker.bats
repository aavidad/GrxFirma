#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Bats carga helpers relativos e invoca dobles indirectamente; los contratos
# buscan variables de shell literales, no valores del entorno del laboratorio.
# shellcheck disable=SC1091,SC2329,SC2016

setup() {
  source "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
  source "${BATS_TEST_DIRNAME}/../runtime-dependencies.sh"
}

@test "worker forces Linux CGo without changing parent build flags" {
  grxfirma_go_build() {
    [[ "${CGO_ENABLED}" == 1 && "${GOOS}" == linux ]]
    [[ "$*" == *"-tags production -o candidate ./cmd/grxfirma-pkcs11-worker" ]]
  }
  go() {
    [[ "$*" == 'version -m candidate' ]]
    printf '\tpath\tgrxfirma/cmd/grxfirma-pkcs11-worker\n\tbuild\tCGO_ENABLED=1\n'
  }
  CGO_ENABLED=0
  GOOS=unrelated
  grxfirma_build_pkcs11_worker dev candidate
  [ "${CGO_ENABLED}" = 0 ]
  [ "${GOOS}" = unrelated ]
}

@test "worker refuses a stub even if build command succeeds" {
  grxfirma_go_build() { return 0; }
  go() { printf '\tpath\tgrxfirma/cmd/grxfirma-pkcs11-worker\n\tbuild\tCGO_ENABLED=0\n'; }
  run grxfirma_build_pkcs11_worker dev candidate
  [ "$status" -ne 0 ]
  [[ "$output" == *"no contiene el backend CGo"* ]]
}

@test "worker refuses wrong command identity and compiler failure" {
  grxfirma_go_build() { return 0; }
  go() { printf '\tpath\tgrxfirma/cmd/nativehost\n\tbuild\tCGO_ENABLED=1\n'; }
  run grxfirma_build_pkcs11_worker dev candidate
  [ "$status" -ne 0 ]
  grxfirma_go_build() { return 7; }
  run grxfirma_build_pkcs11_worker dev candidate
  [ "$status" -ne 0 ]
}

@test "worker participates in bundle runtime dependency rejection" {
  local bundle="${BATS_TEST_TMPDIR}/bundle"
  mkdir -p "${bundle}"
  touch "${bundle}/grxfirma-pkcs11-worker"
  ldd() { printf 'libc.so.6 => not found\n'; }
  run grxfirma_check_bundle_runtime "${bundle}"
  [ "$status" -ne 0 ]
  [[ "$output" == *"faltan bibliotecas para grxfirma-pkcs11-worker"* ]]
}

@test "worker package paths and fail-closed installer inventory agree" {
  local base="${BATS_TEST_DIRNAME}/.."
  run grep -F 'f"{stage_name}/grxfirma-pkcs11-worker"' "${base}/build-suite.sh"
  [ "$status" -eq 0 ]
  run grep -F '"./usr/lib/grxfirma/bin/grxfirma-pkcs11-worker"' "${base}/build-suite.sh"
  [ "$status" -eq 0 ]
  run grep -F '"${USERLIBDIR}/grxfirma-pkcs11-worker"' "${base}/install-suite.sh"
  [ "$status" -eq 0 ]
  run grep -E '^for bin in .* grxfirma-pkcs11-worker; do$' "${base}/install-suite.sh"
  [ "$status" -eq 0 ]
  run grep -F 'pkcs11Capability=disabled' "${base}/build-suite.sh"
  [ "$status" -eq 0 ]
}

@test "sandbox runtime stays optional for the disabled production capability" {
  local script="${BATS_TEST_DIRNAME}/../build-suite.sh"
  run grep -F 'Suggests: bubblewrap (>= 0.11.1), pinentry-qt | pinentry-gnome3 | pinentry-gtk2' "${script}"
  [ "$status" -eq 0 ]
  run grep -E '^Depends:.*bubblewrap|^Recommends:.*bubblewrap' "${script}"
  [ "$status" -ne 0 ]
  run grep -F 'pkcs11WorkerSandbox=bwrap-fd-mounts+seccomp-tsync' "${script}"
  [ "$status" -eq 0 ]
}
