#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

setup() {
  export FIXTURE_ROOT="${BATS_TEST_TMPDIR}/windows-repro"
  export STAGE_DIR="${FIXTURE_ROOT}/GrxFirma"
  export ARCHIVE_ONE="${FIXTURE_ROOT}/one.zip"
  export ARCHIVE_TWO="${FIXTURE_ROOT}/two.zip"
  export SOURCE_DATE_EPOCH=1700000000
  mkdir -p "${STAGE_DIR}/nested"
  printf 'exe-fixture\n' > "${STAGE_DIR}/grxfirma.exe"
  printf 'config\n' > "${STAGE_DIR}/nested/config.txt"
  # shellcheck disable=SC1091
  source "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
  grxfirma_initialize_reproducible_build "${BATS_TEST_DIRNAME}/../../.."
}

@test "Windows ZIP is stable across source timestamp changes" {
  grxfirma_reproducible_zip "${STAGE_DIR}" "${ARCHIVE_ONE}"
  touch -t 202501020304 "${STAGE_DIR}/grxfirma.exe" "${STAGE_DIR}/nested/config.txt"
  grxfirma_reproducible_zip "${STAGE_DIR}" "${ARCHIVE_TWO}"

  run cmp "${ARCHIVE_ONE}" "${ARCHIVE_TWO}"
  [ "$status" -eq 0 ]
}

@test "Windows ZIP has ordered entries and one normalized timestamp" {
  grxfirma_reproducible_zip "${STAGE_DIR}" "${ARCHIVE_ONE}"

  run python3 - "${ARCHIVE_ONE}" <<'PY'
import sys
import zipfile

with zipfile.ZipFile(sys.argv[1]) as archive:
    entries = archive.infolist()
names = [entry.filename for entry in entries]
assert names == sorted(names, key=lambda name: name.encode("utf-8"))
assert len({entry.date_time for entry in entries}) == 1
assert all(not entry.extra and not entry.comment for entry in entries)
PY
  [ "$status" -eq 0 ]
}

@test "NSIS output metadata uses the portable normalized epoch" {
  local setup="${FIXTURE_ROOT}/setup.exe"
  printf 'setup-fixture\n' > "${setup}"
  grxfirma_normalize_output_mtime "${setup}"

  run python3 - "${setup}" "${SOURCE_DATE_EPOCH}" <<'PY'
import os
import sys

assert int(os.stat(sys.argv[1]).st_mtime) == int(sys.argv[2])
PY
  [ "$status" -eq 0 ]
}

@test "all shell Windows Go builds and ZIPs use hardened helpers" {
  for script in "${BATS_TEST_DIRNAME}"/../build-*.sh; do
    if [[ "$(basename "${script}")" == "build-desktop-winui.sh" ]]; then
      # shellcheck disable=SC2016
      run grep -F 'exec powershell.exe "${PS_ARGS[@]}"' "${script}"
      [ "$status" -eq 0 ]
      run grep -F 'grxfirma_go_build' "${script}"
      [ "$status" -ne 0 ]
      run grep -F 'grxfirma_reproducible_zip' "${script}"
      [ "$status" -ne 0 ]
      continue
    fi

    run grep -Eq '(^|[[:space:]])go[[:space:]]+build' "${script}"
    [ "$status" -ne 0 ]
    run grep -F 'grxfirma_go_build' "${script}"
    [ "$status" -eq 0 ]
    run grep -Eq 'zip[[:space:]]+-[^[:space:]]*r' "${script}"
    [ "$status" -ne 0 ]
    run grep -F 'grxfirma_reproducible_zip' "${script}"
    [ "$status" -eq 0 ]
  done

  for flag in -mod=readonly -pgo=off -trimpath -buildvcs=false -buildid=; do
    run grep -F -- "${flag}" "${BATS_TEST_DIRNAME}/../reproducible-build.sh"
    [ "$status" -eq 0 ]
  done


  for script in \
    "${BATS_TEST_DIRNAME}/../build-nativehost.sh" \
    "${BATS_TEST_DIRNAME}/../build-suite.sh"; do
    run grep -F 'GRXFIRMA_BUILD_CHROMIUM_CRX:-0' "${script}"
    [ "$status" -eq 0 ]
    run grep -F 'grxfirma-extension-*' "${script}"
    [ "$status" -ne 0 ]
    run grep -F 'grxfirma-extension-firefox.metadata.json' "${script}"
    [ "$status" -eq 0 ]
  done
}

@test "Windows Fyne toolchain preflight accepts only MinGW-w64 amd64" {
  local compiler="${FIXTURE_ROOT}/fake-mingw-gcc"
  cat > "${compiler}" <<'SH'
#!/usr/bin/env bash
if [[ "${1:-}" == "-dumpmachine" ]]; then
  printf '%s\n' "${FAKE_COMPILER_TARGET:-x86_64-w64-mingw32}"
  exit 0
fi
exit 2
SH
  chmod +x "${compiler}"

  CC="${compiler}" run grxfirma_assert_windows_fyne_toolchain amd64
  [ "$status" -eq 0 ]

  FAKE_COMPILER_TARGET=aarch64-w64-mingw32 \
    CC="${compiler}" \
    run grxfirma_assert_windows_fyne_toolchain amd64
  [ "$status" -ne 0 ]

  CC="${compiler}" run grxfirma_assert_windows_fyne_toolchain arm64
  [ "$status" -ne 0 ]
}

@test "Windows Fyne artifact gate validates Go build metadata" {
  local fake_go="${FIXTURE_ROOT}/fake-go"
  local artifact="${FIXTURE_ROOT}/grxfirma-afirmauri.exe"
  printf 'pe-fixture\n' > "${artifact}"
  cat > "${fake_go}" <<'SH'
#!/usr/bin/env bash
if [[ "${1:-}" == "version" && "${2:-}" == "-m" ]]; then
  cat "${3}.metadata"
  exit 0
fi
exit 2
SH
  chmod +x "${fake_go}"

  cat > "${artifact}.metadata" <<'META'
	path	grxfirma/cmd/grxfirmauri
	build	-tags=other,production,fyne_gui
	build	CGO_ENABLED=1
	build	GOARCH=amd64
	build	GOOS=windows
META
  GRXFIRMA_GO_COMMAND="${fake_go}" \
    run grxfirma_assert_windows_fyne_artifact "${artifact}" amd64
  [ "$status" -eq 0 ]

  sed -i 's/other,production,fyne_gui/other,fyne_gui/' "${artifact}.metadata"
  GRXFIRMA_GO_COMMAND="${fake_go}" \
    run grxfirma_assert_windows_fyne_artifact "${artifact}" amd64
  [ "$status" -ne 0 ]

  sed -i 's/other,fyne_gui/other,production/' "${artifact}.metadata"
  GRXFIRMA_GO_COMMAND="${fake_go}" \
    run grxfirma_assert_windows_fyne_artifact "${artifact}" amd64
  [ "$status" -ne 0 ]

  sed -i \
    -e 's/-tags=other,production$/-tags=production,fyne_gui/' \
    -e 's/CGO_ENABLED=1/CGO_ENABLED=0/' \
    "${artifact}.metadata"
  GRXFIRMA_GO_COMMAND="${fake_go}" \
    run grxfirma_assert_windows_fyne_artifact "${artifact}" amd64
  [ "$status" -ne 0 ]
}
