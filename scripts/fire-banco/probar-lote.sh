#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

#
# Prueba de punta a punta del banco FIRe. Uso: probar-lote.sh [mixto|parcial|detener|todos]
#
#   1. el portal (fire-client-java oficial, con certificado de aplicación
#      sintético y TLS mutuo) crea un lote y pide la firma;
#   2. Chrome sin interfaz abre la página de FIRe, pulsa «Firmar» y el
#      autoscript.js de FIRe lanza afirma://batch (lote JSON trifásico);
#   3. el manejador afirma:// de GrxFirma firma con el P12 sintético contra
#      preSignBatchService/postSignBatchService de FIRe y deja el resultado en
#      su servicio de almacenamiento;
#   4. FIRe redirige al portal, que recupera cada firma;
#   5. se verifica cada firma con GrxFirma (integridad y cadena hasta la CA
#      sintética), OpenSSL (CAdES) y pdfsig (PAdES) si está instalado.
#
# Escenarios:
#   mixto    CAdES + XAdES + PAdES; todo debe firmarse.
#   parcial  un «PDF» que no lo es, sin stoponerror: FIRe debe marcar solo ese
#            documento (PRESIGN_ERROR) y el resto debe firmarse.
#   detener  el mismo documento roto con stoponerror: FIRe debe abortar el lote
#            y llevar al portal a su URL de error.
#
# Requisitos: banco preparado y arrancado (preparar.sh, arrancar.sh), Go,
# Python 3 con Playwright y Google Chrome del sistema.
set -euo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAIZ="$(cd "$AQUI/../.." && pwd)"
# shellcheck disable=SC1091 source=scripts/fire-banco/versiones.env
source "$AQUI/versiones.env"
BANCO="${FIRE_BANCO_DIR:-/tmp/fire-banco}"
[[ -f "$BANCO/portal-config.properties" ]] || { echo "Banco sin preparar: ejecute preparar.sh" >&2; exit 2; }
ESCENARIO="${1:-todos}"
case "$ESCENARIO" in mixto|parcial|detener|todos) ;; *) echo "Escenario desconocido: $ESCENARIO" >&2; exit 2 ;; esac

log() { printf '[fire-banco] %s\n' "$*"; }
falla() { printf '[fire-banco] FALLO: %s\n' "$*" >&2; exit 1; }

JAVA_HOME="$(sed -n 's/^JAVA_HOME="\(.*\)"$/\1/p' "$BANCO/tomcat/bin/setenv.sh")"
JAVA="$JAVA_HOME/bin/java"
OK_URL="https://portal.banco.invalid/ok"
ERROR_URL="https://portal.banco.invalid/error"

curl -fs -o /dev/null --cacert "$BANCO/pki/ca.pem" \
  "https://127.0.0.1:$FIRE_PUERTO_HTTPS/fire-signature/public/js/autoscript.js" \
  || falla "FIRe no responde; ejecute arrancar.sh"

# --- Compilar GrxFirma desde este árbol -----------------------------------------
mkdir -p "$BANCO/bin"
log "Compilando GrxFirma (CLI y manejador afirma:// de prueba)"
(cd "$RAIZ" && GOFLAGS=-buildvcs=false go build -o "$BANCO/bin/grxfirma" ./cmd/grxfirma)
(cd "$RAIZ" && GOFLAGS=-buildvcs=false go test -c -tags firebanco \
    -o "$BANCO/bin/grxfirmauri-banco.test" ./cmd/grxfirmauri)

# --- Perfil aislado de GrxFirma ---------------------------------------------------
# HOME propio: no toca la configuración ni los almacenes del usuario.
GH="$BANCO/grx-home"
rm -rf "$GH"
mkdir -p "$GH/tmp" "$GH/.config/grxfirma/pkcs12"
chmod -R 700 "$GH"
cp "$BANCO/pki/firmante.p12" "$GH/.config/grxfirma/pkcs12/"
printf '{\n  "https://127.0.0.1:%s": "allowed"\n}\n' "$FIRE_PUERTO_HTTPS" > "$GH/.config/grxfirma/trusted-domains.json"
chmod 600 "$GH/.config/grxfirma/trusted-domains.json" "$GH/.config/grxfirma/pkcs12/firmante.p12"

# Lanzador: lo que haría el manejador de protocolo del sistema con la URL afirma://.
LANZADOR="$BANCO/lanzador.sh"
cat > "$LANZADOR" <<EOF
#!/usr/bin/env bash
export HOME="$GH" TMPDIR="$GH/tmp" XDG_CONFIG_HOME="$GH/.config" XDG_RUNTIME_DIR="$GH/tmp"
export GRXFIRMA_PKCS12_DIR="$GH/.config/grxfirma/pkcs12"
export GRXFIRMA_PKCS12_PASSWORD="\$(cat "$BANCO/pki/clave-firmante.txt")"
# La CA sintética del banco firma el certificado TLS de FIRe en 127.0.0.1.
export SSL_CERT_FILE="$BANCO/pki/ca.pem"
export FIRE_BANCO_URI="\$1"
exec "$BANCO/bin/grxfirmauri-banco.test" -test.run '^TestBancoFIRe_ProcesaURIReal\$' -test.v -test.count=1
EOF
chmod 700 "$LANZADOR"

# --- Documentos de prueba (sintéticos) -------------------------------------------
D="$BANCO/prueba/documentos"
rm -rf "$BANCO/prueba"; mkdir -p "$D"
printf 'Texto sintético del banco FIRe para CAdES.\n' > "$D/doc.txt"
printf '<?xml version="1.0" encoding="UTF-8"?>\n<documento><asunto>Prueba sintética del banco FIRe</asunto></documento>\n' > "$D/doc.xml"
cp "$RAIZ/testdata/dictamen-v2/01_una_firma/original.pdf" "$D/doc.pdf"

CP="$BANCO/fire/fire-client-java/target/fire-client-2.4.jar:$(cat "$BANCO/fire-client.classpath")"
APP_ID="$(cat "$BANCO/fire-app-id.txt")"
portal() { "$JAVA" "${PORTAL_OPCIONES[@]}" -cp "$CP" "$AQUI/PortalFire.java" "$@" 2> >(grep -v '^SLF4J' >&2); }

verificar_grx() { # fichero [original]
  local extra=()
  [[ -n "${2:-}" ]] && extra=(-documento-original "$2")
  HOME="$GH" SSL_CERT_FILE="$BANCO/pki/ca.pem" "$BANCO/bin/grxfirma" -modo-cli -operacion verificar \
    -entrada "$1" "${extra[@]}" -salida-json 2>/dev/null \
  | python3 -c '
import json, sys
d = json.load(sys.stdin)
r = d["resultado"]
i, c = r["integridad"]["estado"], r["confianza"]["estado"]
print("  GrxFirma %s: integridad=%s confianza=%s (%s)" % (r.get("formato"), i, c, d.get("motivo") or "sin incidencias"))
sys.exit(0 if i == "valid" and c == "valid" else 1)'
}

verificar_firmas() { # directorio de firmas; verifica las que existan
  local F="$1"
  if [[ -f "$F/cades.firma" ]]; then
    verificar_grx "$F/cades.firma" "$D/doc.txt" || falla "CAdES no verifica"
    openssl cms -verify -inform DER -in "$F/cades.firma" -CAfile "$BANCO/pki/ca.pem" \
      -purpose any -out "$F/cades-contenido.txt" 2>/dev/null || falla "OpenSSL no verifica la CAdES"
    cmp -s "$F/cades-contenido.txt" "$D/doc.txt" || falla "la CAdES no contiene el documento original"
    echo "  OpenSSL: CAdES válida y con el documento original"
  fi
  if [[ -f "$F/xades.firma" ]]; then
    verificar_grx "$F/xades.firma" || falla "XAdES no verifica"
  fi
  if [[ -f "$F/pades.firma" ]]; then
    verificar_grx "$F/pades.firma" || falla "PAdES no verifica"
    if command -v pdfsig >/dev/null; then
      pdfsig "$F/pades.firma" > "$F/pdfsig.txt" 2>&1 || true
      grep -q 'Signature is Valid' "$F/pdfsig.txt" || falla "pdfsig no valida la PAdES"
      grep -q 'Total document signed' "$F/pdfsig.txt" || falla "la PAdES no cubre el documento completo"
      echo "  pdfsig: PAdES válida y cubre el documento completo"
    fi
  fi
}

# escenario nombre stoponerror(true|false) resultado(ok|error) fallos_esperados documentos...
escenario() {
  local nombre="$1" stop="$2" esperado="$3" fallos="$4"; shift 4
  local P="$BANCO/prueba/$nombre"
  mkdir -p "$P/firmas"
  PORTAL_OPCIONES=(-Dbanco.stopOnError="$stop" -Dbanco.fallosEsperados="$fallos")
  log "== Escenario $nombre (stoponerror=$stop): $*"

  log "Portal: creando lote en FIRe"
  local redireccion
  redireccion="$(portal crear "$BANCO/portal-config.properties" "$APP_ID" "$P/estado.properties" \
    "$OK_URL" "$ERROR_URL" "$@")"
  [[ "$redireccion" == https://127.0.0.1:$FIRE_PUERTO_HTTPS/* ]] || falla "FIRe no devolvió URL de redirección"

  log "Navegador: abriendo FIRe y firmando con certificado local"
  local rc=0
  python3 "$AQUI/navegador.py" --url "$redireccion" --ok "$OK_URL" --error "$ERROR_URL" \
    --lanzador "$LANZADOR" --registro "$P/grxfirma.log" --uri-salida "$P/afirma-uri.txt" \
    --captura "$P/final.png" --espera "${FIRE_BANCO_ESPERA:-120}" > "$P/navegador.log" 2>&1 || rc=$?
  grep -v '^\[consola\]' "$P/navegador.log" || true
  grep -q -- '--- PASS: TestBancoFIRe_ProcesaURIReal' "$P/grxfirma.log" \
    || { tail -40 "$P/grxfirma.log" >&2; falla "el manejador afirma:// no terminó bien"; }
  grep -q 'jsonbatch=true' "$P/afirma-uri.txt" || falla "FIRe no lanzó un lote JSON"
  grep -q 'batchpresignerurl=' "$P/afirma-uri.txt" || falla "la URL afirma:// no trae prefirmador de lote"

  if [[ "$esperado" == ok ]]; then
    [[ $rc -eq 0 ]] || falla "FIRe no llevó al portal a la URL de éxito (código $rc)"
    log "Portal: recuperando resultado del lote"
    portal recuperar "$BANCO/portal-config.properties" "$APP_ID" "$P/estado.properties" "$P/firmas" \
      | tee "$P/recuperacion.txt" || falla "el resultado del lote no es el esperado"
    log "Verificando firmas"
    verificar_firmas "$P/firmas"
  else
    [[ $rc -eq 1 ]] || falla "FIRe no llevó al portal a la URL de error (código $rc)"
    log "Portal: recuperando el error de la transacción"
    portal error "$BANCO/portal-config.properties" "$APP_ID" "$P/estado.properties" | tee "$P/error.txt" \
      || falla "FIRe no informó del error al portal"
  fi
  log "Escenario $nombre: correcto"
}

if [[ "$ESCENARIO" == mixto || "$ESCENARIO" == todos ]]; then
  escenario mixto false ok "" \
    "cades=CAdES=$D/doc.txt" "xades=XAdES=$D/doc.xml" "pades=PAdES=$D/doc.pdf"
fi
if [[ "$ESCENARIO" == parcial || "$ESCENARIO" == todos ]]; then
  escenario parcial false ok "roto" \
    "cades=CAdES=$D/doc.txt" "roto=PAdES=$D/doc.txt" "xades=XAdES=$D/doc.xml"
fi
if [[ "$ESCENARIO" == detener || "$ESCENARIO" == todos ]]; then
  escenario detener true error "" \
    "cades=CAdES=$D/doc.txt" "roto=PAdES=$D/doc.txt" "xades=XAdES=$D/doc.xml"
fi

log "RESULTADO: escenarios FIRe ($ESCENARIO) superados con GrxFirma como cliente de certificado local"
