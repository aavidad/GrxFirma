#!/usr/bin/env bats
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
# shellcheck disable=SC2016 # las órdenes van entre comillas simples para que bash -c reciba $1 sin expandir.

# Verificación del instalador sin red ni dependencias adicionales.
setup() {
  export I18N_RUNTIME="${BATS_TEST_DIRNAME}/../installer-i18n.sh"
}

@test "los once catálogos tienen las mismas claves y no queda prosa visible en el código" {
  run python3 "${BATS_TEST_DIRNAME}/test_installer_i18n.py"
  [ "$status" -eq 0 ]
}

@test "se resuelve el idioma del sistema, valenciano, sustitución y reserva inglesa" {
  run bash -c 'source "$1"; unset GRXFIRMA_INSTALLER_LANG; LC_ALL=ca_ES.UTF-8@valencia grx_installer_language' _ "$I18N_RUNTIME"
  [ "$status" -eq 0 ]
  [ "$output" = va ]
  run env -u GRXFIRMA_INSTALLER_LANG LC_ALL=C bash -c 'source "$1"; grx_message UNKNOWN_VERSION' _ "$I18N_RUNTIME"
  [ "$output" = unknown ]
  run env GRXFIRMA_INSTALLER_LANG=de bash -c 'source "$1"; grx_message UNKNOWN_VERSION' _ "$I18N_RUNTIME"
  [ "$output" = unbekannt ]
}

@test "los argumentos no se evalúan ni sustituyen marcadores introducidos por el usuario" {
  local dangerous='$(touch /tmp/no-ejecutar-lote25) @2@ %s \n'
  run env GRXFIRMA_INSTALLER_LANG=en bash -c 'set -eu; source "$1"; grx_message UPDATING "$2" final' _ "$I18N_RUNTIME" "$dangerous"
  [ "$status" -eq 0 ]
  [ "$output" = "Updating GrxFirma: ${dangerous} -> final..." ]
}

@test "sin terminal no se pregunta ni cambia la elección de idioma" {
  run env GRXFIRMA_INSTALLER_LANG=fr bash -c 'source "$1"; grx_select_installer_language < /dev/null; printf "%s" "$GRXFIRMA_INSTALLER_LANG"' _ "$I18N_RUNTIME"
  [ "$status" -eq 0 ]
  [ "$output" = fr ]
}

@test "preinst autónomo conserva los once idiomas sin catálogos instalados" {
  local script="${BATS_TEST_TMPDIR}/preinst"
  printf '#!/bin/bash\nset -eu\ngrx_message UNKNOWN_VERSION\n' > "$script"
  run bash -c 'source "$1"; grx_prepend_maintainer_i18n "$2"' _ "$I18N_RUNTIME" "$script"
  [ "$status" -eq 0 ]
  for locale in es en ca va gl eu fr de it pt zh; do
    run env GRXFIRMA_INSTALLER_LANG="$locale" bash "$script"
    [ "$status" -eq 0 ]
    [ "$output" != UNKNOWN_VERSION ]
  done
  run env GRXFIRMA_INSTALLER_LANG=zh bash "$script"
  [ "$output" = 未知 ]
}

@test "los cuatro scripts Debian reales tienen sintaxis válida y preinst/prerm son autónomos" {
  python3 - "${BATS_TEST_DIRNAME}/../build-suite.sh" "${BATS_TEST_TMPDIR}" <<'PY'
from pathlib import Path
import re
import sys
source = Path(sys.argv[1]).read_text()
for name in ('preinst', 'postinst', 'prerm', 'postrm'):
    match = re.search(r'DEBIAN/' + name + r'" <<\x27(\w+)\x27\n(.*?)\n\1\n', source, re.S)
    assert match, name
    (Path(sys.argv[2]) / name).write_text(match[2] + '\n')
PY
  for name in preinst prerm; do
    run bash -c 'source "$1"; grx_prepend_maintainer_i18n "$2"' _ "$I18N_RUNTIME" "${BATS_TEST_TMPDIR}/${name}"
    [ "$status" -eq 0 ]
    # Una acción ajena a install/upgrade/remove solo comprueba que el script
    # se puede cargar; no cierra procesos ni toca certificados del equipo.
    run env GRXFIRMA_INSTALLER_LANG=zh bash "${BATS_TEST_TMPDIR}/${name}" comprobar
    [ "$status" -eq 0 ]
  done
  for name in preinst postinst prerm postrm; do
    run bash -n "${BATS_TEST_TMPDIR}/${name}"
    [ "$status" -eq 0 ]
  done
}
