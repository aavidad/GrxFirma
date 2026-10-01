#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_BIN="${CHROMIUM_E2E_SYSTEM_PYTHON:-python3}"
PYTHON_TAG="$("${PYTHON_BIN}" -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')"
CACHE_ROOT="${XDG_CACHE_HOME:-${HOME}/.cache}"
VENV_DIR="${CHROMIUM_E2E_VENV:-${CACHE_ROOT}/grxfirma/chromium-e2e/playwright-1.55.0-py${PYTHON_TAG}}"

if [[ "${VENV_DIR}" != /* ]]; then
  echo "CHROMIUM_E2E_VENV debe ser una ruta absoluta" >&2
  exit 2
fi

mkdir -p "$(dirname -- "${VENV_DIR}")"
"${PYTHON_BIN}" -m venv "${VENV_DIR}"
"${VENV_DIR}/bin/python" -m pip install \
  --disable-pip-version-check \
  --only-binary=:all: \
  --require-hashes \
  --requirement "${SCRIPT_DIR}/requirements.txt"

"${VENV_DIR}/bin/python" - <<'PY'
import importlib.metadata
from pathlib import Path
import subprocess

from playwright._impl._driver import compute_driver_executable

version = importlib.metadata.version("playwright")
node, cli = compute_driver_executable()
if version.split("+", 1)[0] != "1.55.0":
    raise SystemExit(f"version Playwright inesperada: {version}")
for component in (node, cli):
    if not Path(component).is_file():
        raise SystemExit(f"driver Playwright incompleto: {component}")
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
    raise SystemExit(
        f"version del driver Playwright incompatible: {driver_version.stdout.strip()}"
    )
PY

printf 'Playwright E2E instalado en %s\n' "${VENV_DIR}"
printf 'Python del arnes: %s\n' "${VENV_DIR}/bin/python"
