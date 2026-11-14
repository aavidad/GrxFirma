#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Catálogos de datos UTF-8; no se ejecuta su contenido ni se necesita Python.
GRX_I18N_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/i18n"

grx_installer_language() {
  local locale="${GRXFIRMA_INSTALLER_LANG:-${LC_ALL:-${LC_MESSAGES:-${LANGUAGE:-${LANG:-en}}}}}"
  locale="${locale%%:*}"
  locale="${locale,,}"
  case "${locale}" in
    ca*valencia*|va*) printf '%s' va ;;
    es*|en*|ca*|gl*|eu*|fr*|de*|it*|pt*|zh*) printf '%s' "${locale:0:2}" ;;
    *) printf '%s' en ;;
  esac
}

grx_message_in() {
  local locale="$1" key="$2" line value="" found=0 part arg index=1
  shift 2
  # En preinst/prerm se incrustan datos porque preinst se ejecuta antes de
  # instalar el paquete y tampoco debe depender de una versión anterior.
  while IFS=$'\t' read -r line part; do
    if [[ "${line}" == "${key}" ]]; then value="${part}"; found=1; break; fi
  done < <(if declare -F grx_embedded_catalog >/dev/null; then
    grx_embedded_catalog "${locale}"
  else
    cat "${GRX_I18N_DIR}/${locale}.tsv"
  fi)
  [[ "${found}" == 1 ]] || { printf '%s\n' "${key}"; return 0; }
  value="${value//\\n/$'\n'}"
  # Sustituye cada marcador una sola vez; los argumentos nunca se evalúan y
  # no pueden inyectar nuevos marcadores ni secuencias de printf.
  local rendered="" rest="${value}" token
  while [[ "${rest}" =~ @([0-9]+)@ ]]; do
    token="${BASH_REMATCH[0]}"; index="${BASH_REMATCH[1]}"
    rendered+="${rest%%"${token}"*}"
    arg="${!index-}"
    rendered+="${arg}"
    rest="${rest#*"${token}"}"
  done
  printf '%s\n' "${rendered}${rest}"
}

grx_message() {
  grx_message_in "$(grx_installer_language)" "$@"
}

grx_select_installer_language() {
  [[ -t 0 && -t 1 ]] || return 0
  local choice locale
  local -a locales=(es en ca va gl eu fr de it pt zh)
  locale="$(grx_installer_language)"
  grx_message LANGUAGE_CHOICES
  read -r -p "$(grx_message LANGUAGE_PROMPT "${locale}")" choice || return 0
  if [[ "${choice}" =~ ^([1-9]|10|11)$ ]]; then locale="${locales[choice-1]}"; fi
  export GRXFIRMA_INSTALLER_LANG="${locale}"
}

# Convierte el script de mantenimiento en un fichero autónomo. Los textos se
# leen de los mismos datos que la instalación normal; no dependen del equipo
# donde dpkg instala o retira el paquete.
grx_prepend_maintainer_i18n() {
  local target="$1" locale temp
  temp="$(mktemp "${target}.XXXXXX")"
  {
    # Se escribe $1 literalmente para evaluarlo al ejecutar el mantenimiento.
    # shellcheck disable=SC2016
    printf '#!/usr/bin/env bash\ngrx_embedded_catalog() {\n  case "$1" in\n'
    for locale in es en ca va gl eu fr de it pt zh; do
      printf '    %s) cat <<\x27GRX_CATALOG_DATA\x27\n' "${locale}"
      cat "${GRX_I18N_DIR}/${locale}.tsv"
      printf 'GRX_CATALOG_DATA\n      ;;\n'
    done
    printf '  esac\n}\n'
    # No incluye esta función de empaquetado en los scripts generados.
    sed '/^# Convierte el script de mantenimiento/,$d' "${GRX_I18N_DIR}/../installer-i18n.sh"
    cat "${target}"
  } > "${temp}"
  mv -- "${temp}" "${target}"
}
