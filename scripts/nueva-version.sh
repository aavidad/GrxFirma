#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
export LC_ALL=C

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="${ROOT_DIR}/VERSION.txt"
NOTES_FILE="${ROOT_DIR}/docs/NOVEDADES.md"

if [[ ( $# -eq 1 && ( "$1" == "--help" || "$1" == "-h" ) ) || $# -gt 2 ]]; then
  echo "Uso: scripts/nueva-version.sh [nueva-versión-SemVer] | --renumerar nueva-versión-SemVer" >&2
  echo "--renumerar exige la versión exacta como argumento y permite una bajada excepcional de numeración." >&2
  if [[ $# -gt 2 ]]; then exit 2; fi
  exit 0
fi
if [[ $# -eq 2 && "$1" != "--renumerar" ]] || [[ $# -eq 1 && "$1" == "--renumerar" ]]; then
  echo "error: use --renumerar con una versión SemVer explícita" >&2
  exit 2
fi
if [[ ! -f "${VERSION_FILE}" || ! -f "${NOTES_FILE}" ||
      ! -r "${VERSION_FILE}" || ! -r "${NOTES_FILE}" ||
      -L "${VERSION_FILE}" || -L "${NOTES_FILE}" ]]; then
  echo "error: faltan VERSION.txt o docs/NOVEDADES.md, o son enlaces simbólicos" >&2
  exit 1
fi

parse_semver() {
  local value="$1" identifier
  if [[ ! "${value}" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)(-([0-9A-Za-z.-]+))?(\+([0-9A-Za-z.-]+))?$ ]]; then
    return 1
  fi
  SEMVER_MAJOR="${BASH_REMATCH[1]}"
  SEMVER_MINOR="${BASH_REMATCH[2]}"
  SEMVER_PATCH="${BASH_REMATCH[3]}"
  SEMVER_PRE="${BASH_REMATCH[5]}"
  SEMVER_BUILD="${BASH_REMATCH[7]}"
  for identifier in "${SEMVER_MAJOR}" "${SEMVER_MINOR}" "${SEMVER_PATCH}"; do
    [[ "${identifier}" == 0 || "${identifier}" != 0* ]] || return 1
  done
  if [[ -n "${SEMVER_PRE}" ]]; then
    local -a parts
    IFS=. read -ra parts <<< "${SEMVER_PRE}"
    for identifier in "${parts[@]}"; do
      [[ -n "${identifier}" ]] || return 1
      [[ ! "${identifier}" =~ ^[0-9]+$ || "${identifier}" == 0 || "${identifier}" != 0* ]] || return 1
    done
    [[ "${SEMVER_PRE}" != .* && "${SEMVER_PRE}" != *..* && "${SEMVER_PRE}" != *. ]] || return 1
  fi
  if [[ -n "${SEMVER_BUILD}" ]]; then
    [[ "${SEMVER_BUILD}" != .* && "${SEMVER_BUILD}" != *..* && "${SEMVER_BUILD}" != *. ]] || return 1
  fi
}

compare_number() {
  CMP=0
  if (( ${#1} > ${#2} )); then CMP=1
  elif (( ${#1} < ${#2} )); then CMP=-1
  elif [[ "$1" > "$2" ]]; then CMP=1
  elif [[ "$1" < "$2" ]]; then CMP=-1
  fi
}

compare_pre() {
  local left="$1" right="$2" index lhs rhs
  CMP=0
  if [[ -z "${left}" && -n "${right}" ]]; then CMP=1; return; fi
  if [[ -n "${left}" && -z "${right}" ]]; then CMP=-1; return; fi
  local -a left_parts right_parts
  IFS=. read -ra left_parts <<< "${left}"
  IFS=. read -ra right_parts <<< "${right}"
  for (( index=0; index<${#left_parts[@]} && index<${#right_parts[@]}; index++ )); do
    lhs="${left_parts[index]}"
    rhs="${right_parts[index]}"
    if [[ "${lhs}" =~ ^[0-9]+$ && "${rhs}" =~ ^[0-9]+$ ]]; then
      compare_number "${lhs}" "${rhs}"
    elif [[ "${lhs}" =~ ^[0-9]+$ ]]; then CMP=-1
    elif [[ "${rhs}" =~ ^[0-9]+$ ]]; then CMP=1
    elif [[ "${lhs}" > "${rhs}" ]]; then CMP=1
    elif [[ "${lhs}" < "${rhs}" ]]; then CMP=-1
    else CMP=0
    fi
    (( CMP == 0 )) || return 0
  done
  if (( ${#left_parts[@]} > ${#right_parts[@]} )); then CMP=1
  elif (( ${#left_parts[@]} < ${#right_parts[@]} )); then CMP=-1
  fi
}

increment_decimal() {
  local digits="$1" index digit carry=1 result=""
  for (( index=${#digits}-1; index>=0; index-- )); do
    digit="${digits:index:1}"
    if (( carry )); then
      if [[ "${digit}" == 9 ]]; then digit=0
      else digit="$((digit + 1))"; carry=0
      fi
    fi
    result="${digit}${result}"
  done
  if (( carry )); then result="1${result}"; fi
  printf '%s' "${result}"
}

CURRENT="$(<"${VERSION_FILE}")"
if ! parse_semver "${CURRENT}"; then
  echo "error: VERSION.txt no contiene una versión SemVer válida" >&2
  exit 1
fi
old_major="${SEMVER_MAJOR}" old_minor="${SEMVER_MINOR}"
old_patch="${SEMVER_PATCH}" old_pre="${SEMVER_PRE}"
if [[ $# -eq 2 ]]; then
  NEW_VERSION="$2"
elif [[ $# -eq 1 ]]; then
  NEW_VERSION="$1"
else
  NEW_VERSION="${old_major}.${old_minor}.$(increment_decimal "${old_patch}")"
fi
if ! parse_semver "${NEW_VERSION}"; then
  echo "error: la nueva versión no es SemVer válida: ${NEW_VERSION}" >&2
  exit 1
fi
compare_number "${SEMVER_MAJOR}" "${old_major}"
if (( CMP == 0 )); then compare_number "${SEMVER_MINOR}" "${old_minor}"; fi
if (( CMP == 0 )); then compare_number "${SEMVER_PATCH}" "${old_patch}"; fi
if (( CMP == 0 )); then compare_pre "${SEMVER_PRE}" "${old_pre}"; fi
if (( CMP <= 0 )) && [[ "${1:-}" != "--renumerar" ]]; then
  echo "error: la versión ${NEW_VERSION} debe ser mayor que ${CURRENT}" >&2
  exit 1
fi
if [[ "${NEW_VERSION}" == "${CURRENT}" ]]; then
  echo "error: la nueva versión debe ser distinta de ${CURRENT}" >&2
  exit 1
fi
if awk -v heading="## ${NEW_VERSION} — " 'index($0, heading) == 1 { found=1 } END { exit !found }' "${NOTES_FILE}"; then
  echo "error: docs/NOVEDADES.md ya contiene una sección para ${NEW_VERSION}" >&2
  exit 1
fi

tmp_notes="$(mktemp "${ROOT_DIR}/docs/.NOVEDADES.XXXXXX")"
tmp_version="$(mktemp "${ROOT_DIR}/.VERSION.XXXXXX")"
trap 'rm -f "${tmp_notes}" "${tmp_version}"' EXIT
awk -v version="${NEW_VERSION}" -v today="$(date +%F)" '
  /^## / && !inserted {
    print "## " version " — " today "\n"
    print "- Pendiente: describir aquí los cambios visibles para los usuarios.\n"
    inserted=1
  }
  { print }
  END {
    if (!inserted) {
      print "\n## " version " — " today "\n"
      print "- Pendiente: describir aquí los cambios visibles para los usuarios."
    }
  }
' "${NOTES_FILE}" > "${tmp_notes}"
printf '%s\n' "${NEW_VERSION}" > "${tmp_version}"
chmod 644 "${tmp_notes}" "${tmp_version}"
mv -f "${tmp_notes}" "${NOTES_FILE}"
mv -f "${tmp_version}" "${VERSION_FILE}"
echo "Versión preparada: ${CURRENT} → ${NEW_VERSION}. Complete docs/NOVEDADES.md antes de compilar."
