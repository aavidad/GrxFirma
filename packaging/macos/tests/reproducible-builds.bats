#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export FIXTURE_ROOT="${BATS_TEST_TMPDIR}/macos-repro"
  export STAGE_DIR="${FIXTURE_ROOT}/GrxFirma.app"
  export ARCHIVE_ONE="${FIXTURE_ROOT}/one.tar.gz"
  export ARCHIVE_TWO="${FIXTURE_ROOT}/two.tar.gz"
  export SOURCE_DATE_EPOCH=1700000000
  mkdir -p "${STAGE_DIR}/Contents/MacOS"
  printf 'binary-fixture\n' > "${STAGE_DIR}/Contents/MacOS/grxfirma"
  chmod 755 "${STAGE_DIR}/Contents/MacOS/grxfirma"
  # shellcheck disable=SC1091
  source "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
  grxfirma_initialize_reproducible_build "${BATS_TEST_DIRNAME}/../../.."
}

@test "macOS tar.gz is stable and hides host ownership" {
  grxfirma_reproducible_tar "${STAGE_DIR}" "${ARCHIVE_ONE}"
  touch -t 202501020304 "${STAGE_DIR}/Contents/MacOS/grxfirma"
  grxfirma_reproducible_tar "${STAGE_DIR}" "${ARCHIVE_TWO}"

  run cmp "${ARCHIVE_ONE}" "${ARCHIVE_TWO}"
  [ "$status" -eq 0 ]

  run python3 - "${ARCHIVE_ONE}" "${SOURCE_DATE_EPOCH}" <<'PY'
import sys
import tarfile

archive_path, raw_epoch = sys.argv[1:]
with tarfile.open(archive_path, "r:gz") as archive:
    members = archive.getmembers()
assert all(member.uid == 0 and member.gid == 0 for member in members)
assert all(member.uname == "" and member.gname == "" for member in members)
assert all(member.mtime == int(raw_epoch) for member in members)
PY
  [ "$status" -eq 0 ]
}

@test "PKG staging timestamps include directories and symlinks" {
  ln -s "MacOS/grxfirma" "${STAGE_DIR}/Contents/current"
  grxfirma_normalize_tree_mtime "${STAGE_DIR}"

  run python3 - "${STAGE_DIR}" "${SOURCE_DATE_EPOCH}" <<'PY'
import sys
from pathlib import Path

root = Path(sys.argv[1])
epoch = int(sys.argv[2])
for path in [root, *root.rglob("*")]:
    assert int(path.lstat().st_mtime) == epoch, path
PY
  [ "$status" -eq 0 ]
}

@test "signed package file metadata can be normalized without rewriting it" {
  local package="${FIXTURE_ROOT}/GrxFirma.pkg"
  printf 'signed-package-fixture\n' > "${package}"
  local before
  before="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "${package}")"
  grxfirma_normalize_output_mtime "${package}"

  run python3 - "${package}" "${SOURCE_DATE_EPOCH}" <<'PY'
import os
import sys

assert int(os.stat(sys.argv[1]).st_mtime) == int(sys.argv[2])
PY
  [ "$status" -eq 0 ]
  [ "$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "${package}")" = "${before}" ]
}

@test "all macOS Go builds and tarballs use hardened helpers" {
  for script in "${BATS_TEST_DIRNAME}"/../build-*.sh; do
    run grep -Eq '(^|[[:space:]])go[[:space:]]+build' "${script}"
    [ "$status" -ne 0 ]
    run grep -F 'grxfirma_go_build' "${script}"
    [ "$status" -eq 0 ]
    run grep -Eq 'tar[[:space:]].*-[^[:space:]]*[cz]' "${script}"
    [ "$status" -ne 0 ]
    run grep -F 'grxfirma_reproducible_tar' "${script}"
    [ "$status" -eq 0 ]
  done

  for flag in -mod=readonly -pgo=off -trimpath -buildvcs=false -buildid=; do
    run grep -F -- "${flag}" "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
    [ "$status" -eq 0 ]
  done


  run grep -F 'GRXFIRMA_BUILD_CHROMIUM_CRX:-0' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -eq 0 ]
  run grep -F 'dipgra-extension-*' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -ne 0 ]
  run grep -F 'dipgra-extension-firefox.metadata.json' "${BATS_TEST_DIRNAME}/../build-suite.sh"
  [ "$status" -eq 0 ]
}
