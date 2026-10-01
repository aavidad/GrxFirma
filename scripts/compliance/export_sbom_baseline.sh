#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="$ROOT_DIR/docs/generated"

mkdir -p "$OUT_DIR"

echo "Exportando baseline de módulos Go en: $OUT_DIR"

(
  cd "$ROOT_DIR"
  go list -m all > "$OUT_DIR/go-modules.txt"
  go list -m -json all > "$OUT_DIR/go-modules.json"
)

cat <<EOF
Baseline exportada:
  - $OUT_DIR/go-modules.txt
  - $OUT_DIR/go-modules.json

Nota:
  Esto es una baseline reproducible de módulos Go.
  No sustituye una SBOM formal en formato CycloneDX o SPDX.
EOF
