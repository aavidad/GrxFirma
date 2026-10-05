#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# The assertions compare literal shell source, including dollar signs.
# shellcheck disable=SC2016

setup() {
  export FIXTURE_ROOT="${BATS_TEST_TMPDIR}/linux-repro"
  export STAGE_DIR="${FIXTURE_ROOT}/stage"
  export ARCHIVE_ONE="${FIXTURE_ROOT}/one.tar.gz"
  export ARCHIVE_TWO="${FIXTURE_ROOT}/two.tar.gz"
  export SOURCE_DATE_EPOCH=1700000000
  mkdir -p "${STAGE_DIR}/nested"
  printf 'alpha\n' > "${STAGE_DIR}/a.txt"
  printf '#!/usr/bin/env bash\nexit 0\n' > "${STAGE_DIR}/nested/tool"
  chmod 755 "${STAGE_DIR}/nested/tool"
  # shellcheck disable=SC1091
  source "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
  grxfirma_initialize_reproducible_build "${BATS_TEST_DIRNAME}/../../.."
}

@test "tar.gz is stable across source timestamp changes" {
  grxfirma_reproducible_tar "${STAGE_DIR}" "${ARCHIVE_ONE}"
  touch -t 202501020304 "${STAGE_DIR}/a.txt" "${STAGE_DIR}/nested/tool"
  grxfirma_reproducible_tar "${STAGE_DIR}" "${ARCHIVE_TWO}"

  run cmp "${ARCHIVE_ONE}" "${ARCHIVE_TWO}"
  [ "$status" -eq 0 ]
}

@test "tar.gz has ordered private metadata and normalized modes" {
  grxfirma_reproducible_tar "${STAGE_DIR}" "${ARCHIVE_ONE}"

  run python3 - "${ARCHIVE_ONE}" "${SOURCE_DATE_EPOCH}" <<'PY'
import sys
import tarfile

archive_path, raw_epoch = sys.argv[1:]
epoch = int(raw_epoch)
with tarfile.open(archive_path, "r:gz") as archive:
    members = archive.getmembers()
names = [member.name for member in members]
assert names == sorted(names, key=lambda name: name.encode("utf-8"))
assert all(member.uid == 0 and member.gid == 0 for member in members)
assert all(member.uname == "" and member.gname == "" for member in members)
assert all(member.mtime == epoch for member in members)
assert next(member for member in members if member.name.endswith("/tool")).mode == 0o755
assert next(member for member in members if member.name.endswith("/a.txt")).mode == 0o644
PY
  [ "$status" -eq 0 ]
}

@test "standalone package metadata uses SOURCE_DATE_EPOCH" {
  local artifact="${FIXTURE_ROOT}/package.deb"
  printf 'package\n' > "${artifact}"
  grxfirma_normalize_output_mtime "${artifact}"

  run python3 - "${artifact}" "${SOURCE_DATE_EPOCH}" <<'PY'
import os
import sys

assert int(os.stat(sys.argv[1]).st_mtime) == int(sys.argv[2])
PY
  [ "$status" -eq 0 ]
}

@test "Debian uses VERSION.txt without an epoch or migration metadata" {
  local script="${BATS_TEST_DIRNAME}/../build-suite.sh"
  run grep -F 'PKG_VERSION="${RAW_VERSION}"' "${script}"
  [ "$status" -eq 0 ]
  run grep -F 'Version: ${PKG_VERSION}' "${script}"
  [ "$status" -eq 0 ]
  run grep -E '^(Replaces|Conflicts):' "${script}"
  [ "$status" -ne 0 ]
}

@test "distributed Linux filenames include GrxFirma and the version" {
  local script="${BATS_TEST_DIRNAME}/../build-suite.sh"
  run grep -F 'STAGE_DIR="${OUT_DIR}/GrxFirma-${RAW_VERSION}-linux-${ARCH}"' "${script}"
  [ "$status" -eq 0 ]
  run grep -F 'TAR_PATH="${OUT_DIR}/GrxFirma-${RAW_VERSION}-linux-${ARCH}.tar.gz"' "${script}"
  [ "$status" -eq 0 ]
  run grep -F 'DEB_PATH="${OUT_DIR}/grxfirma_${PKG_VERSION}_${PKG_ARCH}.deb"' "${script}"
  [ "$status" -eq 0 ]
  run grep -F 'Package: grxfirma' "${script}"
  [ "$status" -eq 0 ]
}

@test "all Linux Go builds use the hardened helper" {
  for script in \
    "${BATS_TEST_DIRNAME}/../build-suite.sh" \
    "${BATS_TEST_DIRNAME}/../install-user.sh"; do
    run grep -Eq '(^|[[:space:]])go[[:space:]]+build' "${script}"
    [ "$status" -ne 0 ]
    run grep -F 'grxfirma_go_build' "${script}"
    [ "$status" -eq 0 ]
  done

  for flag in -mod=readonly -pgo=off -trimpath -buildvcs=false -buildid=; do
    run grep -F -- "${flag}" "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
    [ "$status" -eq 0 ]
  done

  run grep -F 'GRXFIRMA_BUILD_CHROMIUM_CRX:-0' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -eq 0 ]
  run grep -F 'grxfirma-extension-*' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -ne 0 ]
  run grep -F 'grxfirma-extension-firefox.metadata.json' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -eq 0 ]

  for script in \
    "${BATS_TEST_DIRNAME}/../build-suite.sh" \
    "${BATS_TEST_DIRNAME}/../install-user.sh"; do
    # Patrón literal: se comprueba que el script conserva la expansión para su
    # propia ejecución y no que Bats la expanda durante esta prueba.
    # shellcheck disable=SC2016
    run grep -F -- '-o "${' "${script}"
    [ "$status" -eq 0 ]
    # shellcheck disable=SC2016
    run grep -F 'make -C "${' "${script}"
    [ "$status" -eq 0 ]
  done
}

@test "Linux package records source identity and ships the manual Chromium artifact" {
  local script="${BATS_TEST_DIRNAME}/../build-suite.sh"

  run grep -F 'sourceCommit=%s' "${script}"
  [ "$status" -eq 0 ]
  run grep -F '"${STAGE_DIR}/BUILDINFO" "${PKG_ROOT}/usr/share/doc/grxfirma/BUILDINFO"' "${script}"
  [ "$status" -eq 0 ]
  run grep -F '"${STAGE_DIR}/extensions/grxfirma-extension-chromium.zip" "${PKG_ROOT}/usr/lib/grxfirma/extensions/grxfirma-extension-chromium.zip"' "${script}"
  [ "$status" -eq 0 ]
}

@test "protocol handler is hidden and an invocation without URI opens the desktop" {
  run grep -Fx 'NoDisplay=true' "${BATS_TEST_DIRNAME}/../../../grxfirma.desktop"
  [ "$status" -eq 0 ]
  run grep -F 'if [[ -z "${uri}" ]]; then' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -eq 0 ]
  run grep -F 'exec /usr/lib/grxfirma/bin/desktop-launcher.sh' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -eq 0 ]
}

@test "Debian native host wrapper forwards browser arguments" {
  local script="${BATS_TEST_DIRNAME}/../build-suite.sh"
  run grep -F 'exec /usr/lib/grxfirma/bin/grxfirma-nativehost "$@"' "${script}"
  [ "$status" -eq 0 ]
  run grep -F 'chmod 0644 "${target}"' "${script}"
  [ "$status" -eq 0 ]
}
