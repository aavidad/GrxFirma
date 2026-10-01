#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Qt 6.7.3 conserva AGL en QtGui.prl aunque el proyecto sustituya
# QMAKE_LIBS_OPENGL. Xcode 26 ya no incluye ese framework. El parche queda
# limitado a la versión y al formato conocidos; cualquier variante inesperada
# falla de forma cerrada en lugar de modificar metadatos de Qt a ciegas.
grxfirma_prepare_qmake_macos() {
  local qmake_cmd="$1"
  local qt_version qt_libs qt_gui_prl

  qt_version="$("${qmake_cmd}" -query QT_VERSION)"
  qt_libs="$("${qmake_cmd}" -query QT_INSTALL_LIBS)"
  qt_gui_prl="${qt_libs}/QtGui.framework/Versions/A/Resources/QtGui.prl"
  if [[ ! -f "${qt_gui_prl}" ]]; then
    qt_gui_prl="${qt_libs}/QtGui.framework/Resources/QtGui.prl"
  fi

  if [[ ! -f "${qt_gui_prl}" ]]; then
    if [[ "${qt_version}" == "6.7.3" ]]; then
      echo "error: Qt 6.7.3 no expone el QtGui.prl esperado: ${qt_libs}" >&2
      return 1
    fi
    return 0
  fi

  python3 - "${qt_version}" "${qt_gui_prl}" <<'PY'
import os
from pathlib import Path
import sys

version, raw_path = sys.argv[1:]
path = Path(raw_path)
source = path.read_text(encoding="utf-8")
agl_count = source.count("-framework AGL")

if version == "6.7.3":
    if agl_count not in (0, 2):
        raise SystemExit(
            f"QtGui.prl 6.7.3 inesperado: contiene {agl_count} referencias AGL"
        )
    updated = source.replace(" -framework AGL", "").replace(
        ";-framework AGL", ""
    )
    if "-framework AGL" in updated:
        raise SystemExit("QtGui.prl 6.7.3 conserva una referencia AGL desconocida")
    if updated != source:
        mode = path.stat().st_mode
        path.write_text(updated, encoding="utf-8")
        os.chmod(path, mode)
elif agl_count:
    raise SystemExit(
        f"Qt {version} aún declara AGL; se requiere un parche compatible explícito"
    )
PY
}

# Algunos kits universales de Qt 6.7.3 vuelven a incorporar AGL al Makefile
# aunque QtGui.prl ya esté saneado. Se corrige únicamente el token autónomo
# generado por esa versión; una versión distinta continúa fallando de forma
# cerrada para evitar alterar a ciegas las órdenes de enlace.
grxfirma_prepare_qmake_makefile_macos() {
  local qmake_cmd="$1"
  local makefile="$2"
  local qt_version

  if [[ ! -f "${makefile}" ]]; then
    echo "error: qmake no ha generado ${makefile}" >&2
    return 1
  fi
  qt_version="$("${qmake_cmd}" -query QT_VERSION)"

  python3 - "${qt_version}" "${makefile}" <<'PY'
import os
from pathlib import Path
import re
import sys

version, raw_path = sys.argv[1:]
path = Path(raw_path)
source = path.read_text(encoding="utf-8")
pattern = re.compile(r"(?<!\S)-framework[ \t]+AGL(?=\s|$)")
agl_count = len(pattern.findall(source))

if version == "6.7.3":
    updated = pattern.sub("", source)
    if pattern.search(updated):
        raise SystemExit("el Makefile de Qt 6.7.3 conserva una referencia AGL")
    if updated != source:
        mode = path.stat().st_mode
        path.write_text(updated, encoding="utf-8")
        os.chmod(path, mode)
elif agl_count:
    raise SystemExit(
        f"Qt {version} genera AGL; se requiere un parche compatible explícito"
    )
PY
}

grxfirma_assert_qmake_makefile_without_agl() {
  local makefile="$1"
  if [[ ! -f "${makefile}" ]]; then
    echo "error: qmake no ha generado ${makefile}" >&2
    return 1
  fi
  if grep -Eq -- '-framework[[:space:]]+AGL' "${makefile}"; then
    echo "error: qmake ha conservado el framework AGL retirado por Xcode" >&2
    return 1
  fi
}
