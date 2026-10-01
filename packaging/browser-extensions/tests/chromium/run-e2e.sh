#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_PYTHON="${CHROMIUM_E2E_SYSTEM_PYTHON:-python3}"

playwright_is_usable() {
  local python_bin="$1"
  "${python_bin}" - <<'PY' >/dev/null 2>&1
import importlib.metadata
from pathlib import Path
import subprocess

from playwright._impl._driver import compute_driver_executable

node, cli = compute_driver_executable()
if not Path(node).is_file() or not Path(cli).is_file():
    raise SystemExit(1)
if importlib.metadata.version("playwright").split("+", 1)[0] != "1.55.0":
    raise SystemExit(1)
driver_version = subprocess.run(
    [node, cli, "--version"],
    check=False,
    stdout=subprocess.PIPE,
    stderr=subprocess.STDOUT,
    text=True,
    timeout=5,
)
if driver_version.returncode != 0 or not driver_version.stdout.startswith(
    "Version 1.55.0"
):
    raise SystemExit(1)
PY
}

if [[ -n "${CHROMIUM_E2E_PYTHON:-}" ]]; then
  PYTHON_BIN="${CHROMIUM_E2E_PYTHON}"
  if ! playwright_is_usable "${PYTHON_BIN}"; then
    echo "CHROMIUM_E2E_PYTHON no contiene Playwright 1.55.0 completo" >&2
    exit 78
  fi
elif playwright_is_usable "${DEFAULT_PYTHON}"; then
  PYTHON_BIN="${DEFAULT_PYTHON}"
else
  PYTHON_TAG="$("${DEFAULT_PYTHON}" -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')"
  CACHE_ROOT="${XDG_CACHE_HOME:-${HOME}/.cache}"
  CACHED_PYTHON="${CHROMIUM_E2E_VENV:-${CACHE_ROOT}/grxfirma/chromium-e2e/playwright-1.55.0-py${PYTHON_TAG}}/bin/python"
  if playwright_is_usable "${CACHED_PYTHON}"; then
    PYTHON_BIN="${CACHED_PYTHON}"
  else
    cat >&2 <<EOF
Playwright Python no tiene un driver 1.55.0 utilizable.
Instala la dependencia aislada con:
  ${SCRIPT_DIR}/install-playwright.sh

Tambien puedes fijar un entorno compatible mediante CHROMIUM_E2E_PYTHON.
EOF
    exit 78
  fi
fi

exec "${PYTHON_BIN}" "${SCRIPT_DIR}/native_messaging_e2e.py" "$@"
