#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
#
# Arranca el Tomcat del banco FIRe (solo 127.0.0.1) y espera a que responda.
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 source=scripts/fire-banco/versiones.env
source "$AQUI/versiones.env"
BANCO="${FIRE_BANCO_DIR:-/tmp/fire-banco}"
TB="$BANCO/tomcat"
[[ -f "$TB/conf/server.xml" ]] || { echo "Banco sin preparar: ejecute preparar.sh" >&2; exit 2; }
if [[ -f "$TB/tomcat.pid" ]] && kill -0 "$(cat "$TB/tomcat.pid")" 2>/dev/null; then
  echo "[fire-banco] Tomcat ya estaba en marcha (pid $(cat "$TB/tomcat.pid"))"
else
  rm -f "$TB/tomcat.pid"
  CATALINA_BASE="$TB" "$BANCO/tools/apache-tomcat-$TOMCAT_VERSION/bin/catalina.sh" start >/dev/null
fi
# Se espera a que FIRe esté desplegado, no solo a que Tomcat acepte conexiones.
URL="https://127.0.0.1:$FIRE_PUERTO_HTTPS/fire-signature/public/js/autoscript.js"
for _ in $(seq 1 90); do
  codigo="$(curl -s -o /dev/null -w '%{http_code}' --cacert "$BANCO/pki/ca.pem" "$URL" || true)"
  if [[ "$codigo" == "200" ]]; then
    echo "[fire-banco] FIRe desplegado en https://127.0.0.1:$FIRE_PUERTO_HTTPS/fire-signature"
    exit 0
  fi
  sleep 1
done
echo "FIRe no respondió a tiempo; revise $TB/logs" >&2
exit 1
