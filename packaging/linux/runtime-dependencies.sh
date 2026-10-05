#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

# Helpers shared by the Debian packager and the per-user bundle. The Debian
# dependency generator intentionally delegates ELF resolution to
# dpkg-shlibdeps and maps QML modules to the package that owns their qmldir.
# This avoids maintaining distribution-specific Qt package names here.

grxfirma_collect_qml_imports() {
  local source_dir="$1"
  local output_path="$2"

  if [[ ! -d "${source_dir}" ]]; then
    : > "${output_path}"
    return 0
  fi

  python3 - "${source_dir}" "${output_path}" <<'PY'
import re
import sys
from pathlib import Path

source_dir = Path(sys.argv[1])
output_path = Path(sys.argv[2])
pattern = re.compile(
    r"^\s*import\s+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)"
    r"(?:\s+[0-9][0-9.]*)?(?:\s+as\s+[A-Za-z_][A-Za-z0-9_]*)?"
    r"\s*(?://.*)?$"
)
modules = set()
for qml_path in source_dir.rglob("*.qml"):
    for line in qml_path.read_text(encoding="utf-8").splitlines():
        match = pattern.match(line)
        if match:
            modules.add(match.group(1))
output_path.write_text(
    "".join(f"{module}\n" for module in sorted(modules)),
    encoding="utf-8",
)
PY
}

grxfirma_qml_root_for_qmake() {
  local qmake_cmd="$1"
  local qml_root
  qml_root="$("${qmake_cmd}" -query QT_INSTALL_QML 2>/dev/null || true)"
  if [[ -z "${qml_root}" || ! -d "${qml_root}" ]]; then
    echo "error: ${qmake_cmd} no proporciona un QT_INSTALL_QML válido" >&2
    return 1
  fi
  printf '%s\n' "${qml_root}"
}

grxfirma_debian_package_owning_path() {
  local path="$1"
  local query_output owner
  if ! query_output="$(dpkg-query -S "${path}" 2>/dev/null)"; then
    echo "error: ningún paquete Debian instalado declara ${path}" >&2
    return 1
  fi
  owner="${query_output%%$'\n'*}"
  owner="${owner%%: /*}"
  owner="${owner%%:*}"
  if [[ -z "${owner}" || ! "${owner}" =~ ^[a-z0-9][a-z0-9+.-]+$ ]]; then
    echo "error: propietario Debian no válido para ${path}: ${owner}" >&2
    return 1
  fi
  printf '%s\n' "${owner}"
}

grxfirma_qml_debian_packages() {
  local manifest_path="$1"
  local qml_root="$2"
  local module module_path qmldir

  [[ -f "${manifest_path}" ]] || {
    echo "error: no existe el manifiesto QML ${manifest_path}" >&2
    return 1
  }
  [[ -d "${qml_root}" ]] || {
    echo "error: no existe el directorio de módulos QML ${qml_root}" >&2
    return 1
  }

  while IFS= read -r module || [[ -n "${module}" ]]; do
    [[ -n "${module}" ]] || continue
    if [[ ! "${module}" =~ ^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$ ]]; then
      echo "error: import QML no válido en ${manifest_path}: ${module}" >&2
      return 1
    fi
    module_path="${module//./\/}"
    qmldir="${qml_root}/${module_path}/qmldir"
    if [[ ! -f "${qmldir}" ]]; then
      echo "error: el entorno de build no contiene el módulo QML ${module} (${qmldir})" >&2
      return 1
    fi
    grxfirma_debian_package_owning_path "${qmldir}"
  done < "${manifest_path}" | LC_ALL=C sort -u
}

_grxfirma_generate_debian_depends() {
  local package_root="$1"
  local package_name="$2"
  local qml_manifest="$3"
  local qml_root="$4"
  local work_dir="$5"
  local relative_path shlibs_output shlibs_depends dependency qml_packages
  local -a elf_binaries=()
  local -a dependencies=()
  local -a shlibdeps_options=(-O)

  for required_command in dpkg-query dpkg-shlibdeps find readelf; do
    if ! command -v "${required_command}" >/dev/null 2>&1; then
      echo "error: falta ${required_command}; no se pueden calcular Depends de Debian" >&2
      return 1
    fi
  done
  [[ -d "${package_root}" ]] || {
    echo "error: no existe la raíz del paquete ${package_root}" >&2
    return 1
  }
  if [[ ! "${package_name}" =~ ^[a-z0-9][a-z0-9+.-]+$ ]]; then
    echo "error: nombre de paquete Debian no válido: ${package_name}" >&2
    return 1
  fi

  mkdir -p "${work_dir}/debian"
  cat > "${work_dir}/debian/control" <<EOF
Source: ${package_name}
Section: utils
Priority: optional
Maintainer: Alberto Avidad Fernández <avidad@dipgra.es>
Standards-Version: 4.6.2

Package: ${package_name}
Architecture: any
Description: dependency analysis workspace
EOF
  ln -s "${package_root}" "${work_dir}/debian/${package_name}"

  while IFS= read -r -d '' candidate; do
    if readelf -h "${candidate}" >/dev/null 2>&1; then
      relative_path="${candidate#"${package_root}/"}"
      elf_binaries+=("${work_dir}/debian/${package_name}/${relative_path}")
    fi
  done < <(find "${package_root}" -type f -print0 | LC_ALL=C sort -z)

  if [[ "${#elf_binaries[@]}" -eq 0 ]]; then
    echo "error: no se encontraron ejecutables ELF en ${package_root}" >&2
    return 1
  fi

  # --package se incorporó a dpkg-shlibdeps después de las versiones aún
  # presentes en algunas distribuciones soportadas. Con -O y el control
  # temporal no es imprescindible, pero se aprovecha cuando está disponible.
  if DPKG_COLORS=never LC_ALL=C dpkg-shlibdeps --help 2>&1 |
      grep -Fq -- "--package="; then
    shlibdeps_options=(--package="${package_name}" -O)
  fi

  shlibs_output="$(
    cd "${work_dir}"
    DPKG_COLORS=never LC_ALL=C dpkg-shlibdeps \
      "${shlibdeps_options[@]}" \
      "${elf_binaries[@]}"
  )"
  if [[ "${shlibs_output}" != shlibs:Depends=* ]]; then
    echo "error: salida inesperada de dpkg-shlibdeps: ${shlibs_output}" >&2
    return 1
  fi
  shlibs_depends="${shlibs_output#shlibs:Depends=}"
  IFS=',' read -r -a dependencies <<< "${shlibs_depends}"

  if [[ -s "${qml_manifest}" ]]; then
    qml_packages="$(grxfirma_qml_debian_packages "${qml_manifest}" "${qml_root}")" || return 1
    while IFS= read -r dependency || [[ -n "${dependency}" ]]; do
      [[ -n "${dependency}" ]] && dependencies+=("${dependency}")
    done <<< "${qml_packages}"
  fi

  printf '%s\n' "${dependencies[@]}" |
    sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//' |
    awk 'NF && !seen[$0]++' |
    LC_ALL=C sort |
    awk 'BEGIN { separator="" } { printf "%s%s", separator, $0; separator=", " } END { print "" }'
}

grxfirma_generate_debian_depends() {
  local package_root="$1"
  local package_name="$2"
  local qml_manifest="$3"
  local qml_root="$4"
  local work_parent="$5"
  local canonical_parent dependency_work_dir output status

  if [[ -z "${work_parent}" || "${work_parent}" == "/" || -L "${work_parent}" ]]; then
    echo "error: directorio temporal de dependencias no válido: ${work_parent}" >&2
    return 1
  fi
  mkdir -p "${work_parent}"
  canonical_parent="$(cd "${work_parent}" && pwd -P)"
  dependency_work_dir="$(mktemp -d "${canonical_parent}/.grxfirma-shlibdeps.XXXXXX")"
  case "${dependency_work_dir}" in
    "${canonical_parent}"/.grxfirma-shlibdeps.*) ;;
    *)
      echo "error: mktemp devolvió una ruta inesperada: ${dependency_work_dir}" >&2
      return 1
      ;;
  esac

  status=0
  output="$(
    _grxfirma_generate_debian_depends \
      "${package_root}" \
      "${package_name}" \
      "${qml_manifest}" \
      "${qml_root}" \
      "${dependency_work_dir}"
  )" || status=$?

  # The directory was created by mktemp under the validated parent. find
  # removes its contents without following symlinks that a tool may have made.
  find "${dependency_work_dir}" -depth -delete

  if [[ "${status}" -ne 0 ]]; then
    return "${status}"
  fi
  printf '%s\n' "${output}"
}

grxfirma_merge_debian_depends() {
  local generated="$1"
  shift

  {
    printf '%s\n' "${generated}" | tr ',' '\n'
    printf '%s\n' "$@"
  } |
    sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//' |
    awk 'NF && !seen[$0]++' |
    LC_ALL=C sort |
    awk 'BEGIN { separator="" } { printf "%s%s", separator, $0; separator=", " } END { print "" }'
}

grxfirma_runtime_qml_roots() {
  local candidate core_path core_dir
  local -a candidates=()

  if [[ -n "${QML2_IMPORT_PATH:-}" ]]; then
    IFS=':' read -r -a candidates <<< "${QML2_IMPORT_PATH}"
  fi
  if [[ -n "${QML_IMPORT_PATH:-}" ]]; then
    local -a qml_import_candidates=()
    IFS=':' read -r -a qml_import_candidates <<< "${QML_IMPORT_PATH}"
    candidates+=("${qml_import_candidates[@]}")
  fi
  if command -v qtpaths6 >/dev/null 2>&1; then
    candidates+=("$(qtpaths6 --query QT_INSTALL_QML 2>/dev/null || true)")
  elif command -v qtpaths >/dev/null 2>&1; then
    candidates+=("$(qtpaths --qt-version 6 --query QT_INSTALL_QML 2>/dev/null || true)")
  fi
  if command -v ldconfig >/dev/null 2>&1; then
    # Consume the whole cache: an early awk exit can SIGPIPE ldconfig and
    # abort discovery under pipefail, even with valid qtpaths roots already
    # collected. A failed optional cache lookup must not discard those roots.
    if core_path="$(ldconfig -p 2>/dev/null | awk '$1 == "libQt6Core.so.6" && !found { path=$NF; found=1 } END { if (found) print path }')" && [[ -n "${core_path}" ]]; then
      core_dir="$(dirname "${core_path}")"
      candidates+=("${core_dir}/qt6/qml" "${core_dir}/qml")
    fi
  fi
  candidates+=("/usr/lib/qt6/qml" "/usr/lib64/qt6/qml" "/usr/share/qt6/qml")

  printf '%s\n' "${candidates[@]}" |
    awk 'NF && !seen[$0]++' |
    while IFS= read -r candidate; do
      if [[ -d "${candidate}" ]]; then
        printf '%s\n' "${candidate}"
      fi
    done
}

grxfirma_check_bundle_runtime() {
  local bundle_dir="$1"
  local qml_manifest="${bundle_dir}/runtime-dependencies.qml"
  local binary output module module_path qml_root
  local failures=0
  local qml_available=0
  local -a qml_roots=()

  if ! command -v ldd >/dev/null 2>&1; then
    echo "error: falta ldd; no se pueden validar las bibliotecas runtime del bundle" >&2
    return 1
  fi

  for binary in \
    grxfirma \
    grxfirma-gui \
    grxfirma-desktop \
    grxfirma-gui-qml \
    grxfirma-afirmauri \
    grxfirma-nativehost \
    grxfirma-pkcs11-worker
  do
    [[ -f "${bundle_dir}/${binary}" ]] || continue
    if ! output="$(LC_ALL=C ldd "${bundle_dir}/${binary}" 2>&1)"; then
      if grep -Eqi 'not a dynamic executable|statically linked' <<< "${output}"; then
        continue
      fi
      echo "error: no se pudieron resolver las bibliotecas de ${binary}:" >&2
      printf '%s\n' "${output}" >&2
      failures=1
      continue
    fi
    if grep -Eq '(^|[[:space:]])[^[:space:]]+[[:space:]]+=>[[:space:]]+not found([[:space:]]|$)' <<< "${output}"; then
      echo "error: faltan bibliotecas para ${binary}:" >&2
      grep -E '=>[[:space:]]+not found' <<< "${output}" >&2
      failures=1
    fi
  done

  if [[ -f "${bundle_dir}/grxfirma-gui-qml" && -s "${qml_manifest}" ]]; then
    mapfile -t qml_roots < <(grxfirma_runtime_qml_roots)
    while IFS= read -r module || [[ -n "${module}" ]]; do
      [[ -n "${module}" ]] || continue
      if [[ ! "${module}" =~ ^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$ ]]; then
        echo "error: import QML no válido en el bundle: ${module}" >&2
        failures=1
        continue
      fi
      module_path="${module//./\/}"
      qml_available=0
      for qml_root in "${qml_roots[@]}"; do
        if [[ -f "${qml_root}/${module_path}/qmldir" ]]; then
          qml_available=1
          break
        fi
      done
      if [[ "${qml_available}" != "1" ]]; then
        echo "error: falta el módulo QML ${module}" >&2
        failures=1
      fi
    done < "${qml_manifest}"
  fi

  if [[ "${failures}" != "0" ]]; then
    echo "error: instala las bibliotecas Qt6/QML indicadas por tu distribución o usa el paquete .deb." >&2
    return 1
  fi
  echo "Dependencias runtime del bundle verificadas."
}

runtime_dependencies_main() {
  case "${1:-}" in
    --check-bundle)
      [[ "$#" -eq 2 ]] || {
        echo "Uso: $0 --check-bundle /ruta/al/bundle" >&2
        return 2
      }
      grxfirma_check_bundle_runtime "$2"
      ;;
    *)
      echo "Uso: $0 --check-bundle /ruta/al/bundle" >&2
      return 2
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  runtime_dependencies_main "$@"
fi
