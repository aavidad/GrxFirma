#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Official releases require independent OS runners and protected credentials.
set -euo pipefail

cat >&2 <<'EOF'
error: la publicacion oficial local esta deshabilitada.

Un release oficial requiere simultaneamente Authenticode Windows, Developer ID
y notarizacion macOS, firma OpenPGP, verificadores independientes y el entorno
protegido de GitHub `official-release`. Ningun host local aislado satisface ese
contrato.

Para construir candidatos tecnicos no oficiales usa los scripts de packaging y
despues scripts/release/assemble_installers.sh. Para publicar, crea y empuja un
tag SemVer v* que coincida con VERSION.txt.
EOF
exit 1
