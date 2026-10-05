#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
#
# Detiene el Tomcat del banco FIRe.
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 source=scripts/fire-banco/versiones.env
source "$AQUI/versiones.env"
BANCO="${FIRE_BANCO_DIR:-/tmp/fire-banco}"
TB="$BANCO/tomcat"
if [[ -f "$TB/tomcat.pid" ]] && kill -0 "$(cat "$TB/tomcat.pid")" 2>/dev/null; then
  CATALINA_BASE="$TB" "$BANCO/tools/apache-tomcat-$TOMCAT_VERSION/bin/catalina.sh" stop 20 -force >/dev/null 2>&1 || true
fi
rm -f "$TB/tomcat.pid"
# Respaldo: un Tomcat del banco sin fichero pid (por ejemplo, si se borró el directorio).
for pid in $(pgrep -u "$(id -u)" -f -- "-Dcatalina.base=$TB( |$)" || true); do
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 20); do kill -0 "$pid" 2>/dev/null || break; sleep 0.5; done
  kill -9 "$pid" 2>/dev/null || true
done
echo "[fire-banco] Tomcat detenido"
