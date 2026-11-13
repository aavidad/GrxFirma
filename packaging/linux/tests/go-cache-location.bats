#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# G-02: ninguna caché de Go en una ruta predecible de un directorio compartido.

setup() {
  ROOT_DIR="$(cd "${BATS_TEST_DIRNAME}/../../.." && pwd)"
}

@test "ningún script ni documento fija GOCACHE o GOMODCACHE en /tmp" {
  run git -C "${ROOT_DIR}" grep -nE '(GOCACHE|GOMODCACHE)[^[:space:]]*=[^[:space:]]*/tmp/' -- \
    packaging scripts Makefile README.md docs ':!packaging/linux/tests/go-cache-location.bats'
  [ "${status}" -ne 0 ]
  [ -z "${output}" ]
}

@test "install-user.sh no lleva la caché de Go a /tmp/grxfirma-*" {
  local fake_bin="${BATS_TEST_TMPDIR}/bin"
  local record="${BATS_TEST_TMPDIR}/go-env"
  mkdir -p "${fake_bin}" "${BATS_TEST_TMPDIR}/home"
  cat > "${fake_bin}/go" <<EOF
#!/usr/bin/env bash
printf 'GOCACHE=%s\nGOMODCACHE=%s\n' "\${GOCACHE:-}" "\${GOMODCACHE:-}" > "${record}"
exit 1
EOF
  chmod 755 "${fake_bin}/go"

  run env -u GOCACHE -u GOMODCACHE HOME="${BATS_TEST_TMPDIR}/home" \
    PATH="${fake_bin}:/usr/bin:/bin" \
    bash "${ROOT_DIR}/packaging/linux/install-user.sh"
  [ "${status}" -ne 0 ]
  [ -f "${record}" ]
  run grep -c '/tmp/grxfirma-' "${record}"
  [ "${output}" = "0" ]
}
