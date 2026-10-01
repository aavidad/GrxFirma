#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
for component in grxfirmauri nativehost; do
  (
    cd "${ROOT_DIR}/cmd/${component}"
    x86_64-w64-mingw32-windres -O coff \
      -o "${component}_windows_amd64.syso" \
      "${component}_windows_amd64.rc"
  )
done
