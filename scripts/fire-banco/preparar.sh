#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2
#
# Prepara desde cero el banco de pruebas FIRe local:
#   - descarga Maven y Tomcat (sumas SHA-512 fijadas) en el directorio del banco;
#   - clona FIRe en el commit fijado y compila fire-signature y fire-client-java;
#   - genera una PKI SINTÉTICA (CA, servidor, aplicación cliente y firmante);
#   - configura FIRe sin base de datos, solo con el proveedor «local»;
#   - configura Tomcat escuchando únicamente en 127.0.0.1.
#
# No instala nada en el sistema ni usa certificados reales. Todo queda en
# $FIRE_BANCO_DIR (por defecto /tmp/fire-banco), fuera del repositorio.
set -euo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 source=scripts/fire-banco/versiones.env
source "$AQUI/versiones.env"

BANCO="${FIRE_BANCO_DIR:-/tmp/fire-banco}"
mkdir -p "$BANCO"
BANCO="$(cd "$BANCO" && pwd -P)"
# El directorio vive en /tmp: debe ser nuestro y privado (claves y contraseñas de prueba).
[[ -O "$BANCO" && ! -L "${FIRE_BANCO_DIR:-/tmp/fire-banco}" ]] || { echo "$BANCO no pertenece a este usuario o es un enlace" >&2; exit 2; }
chmod 700 "$BANCO"
case "$BANCO" in
  "$(cd "$AQUI/../.." && pwd)"*) echo "El banco no puede estar dentro del repositorio: $BANCO" >&2; exit 2 ;;
esac

log() { printf '[fire-banco] %s\n' "$*"; }

# --- JDK -------------------------------------------------------------------
detectar_java() {
  if [[ -n "${FIRE_BANCO_JAVA_HOME:-}" ]]; then echo "$FIRE_BANCO_JAVA_HOME"; return; fi
  for v in 11 17 21; do
    for d in /usr/lib/jvm/java-$v-openjdk-amd64 /usr/lib/jvm/java-$v-openjdk /usr/lib/jvm/temurin-$v-jdk-amd64; do
      [[ -x "$d/bin/javac" ]] && { echo "$d"; return; }
    done
  done
  echo ""
}
JAVA_HOME="$(detectar_java)"
if [[ -z "$JAVA_HOME" ]]; then
  echo "Hace falta un JDK 11, 17 o 21 (con javac). Indique FIRE_BANCO_JAVA_HOME." >&2
  exit 2
fi
export JAVA_HOME
log "JDK: $JAVA_HOME"
for herramienta in git curl openssl sha512sum tar; do
  command -v "$herramienta" >/dev/null || { echo "Falta la herramienta: $herramienta" >&2; exit 2; }
done

# --- Descargas verificadas ---------------------------------------------------
descargar() { # url sha512 destino
  local url="$1" sum="$2" dest="$3"
  if [[ ! -f "$dest" ]] || ! echo "$sum  $dest" | sha512sum -c --status; then
    log "Descargando $(basename "$dest")"
    curl -fsSL --retry 3 -o "$dest.part" "$url"
    mv "$dest.part" "$dest"
  fi
  echo "$sum  $dest" | sha512sum -c --status || { echo "Suma SHA-512 incorrecta: $dest" >&2; exit 3; }
}
mkdir -p "$BANCO/tools"
descargar "$MAVEN_URL" "$MAVEN_SHA512" "$BANCO/tools/apache-maven-$MAVEN_VERSION-bin.tar.gz"
descargar "$TOMCAT_URL" "$TOMCAT_SHA512" "$BANCO/tools/apache-tomcat-$TOMCAT_VERSION.tar.gz"
[[ -d "$BANCO/tools/apache-maven-$MAVEN_VERSION" ]] || tar -xzf "$BANCO/tools/apache-maven-$MAVEN_VERSION-bin.tar.gz" -C "$BANCO/tools"
[[ -d "$BANCO/tools/apache-tomcat-$TOMCAT_VERSION" ]] || tar -xzf "$BANCO/tools/apache-tomcat-$TOMCAT_VERSION.tar.gz" -C "$BANCO/tools"
MVN="$BANCO/tools/apache-maven-$MAVEN_VERSION/bin/mvn"
CATALINA_HOME="$BANCO/tools/apache-tomcat-$TOMCAT_VERSION"

# --- Fuentes en los commits fijados ---------------------------------------------
obtener() { # directorio repo commit
  local dir="$1" repo="$2" commit="$3"
  if [[ ! -d "$dir/.git" ]]; then
    git init -q "$dir"
    git -C "$dir" remote add origin "$repo"
  fi
  if [[ "$(git -C "$dir" rev-parse HEAD 2>/dev/null || true)" != "$commit" ]]; then
    log "Obteniendo $(basename "$dir") $commit"
    git -C "$dir" fetch -q --depth 1 origin "$commit"
    git -C "$dir" checkout -q --detach "$commit"
  fi
  [[ "$(git -C "$dir" rev-parse HEAD)" == "$commit" ]] || { echo "$dir no está en el commit fijado" >&2; exit 3; }
  [[ -z "$(git -C "$dir" status --porcelain)" ]] || { echo "$dir tiene cambios locales; bórrelo y repita" >&2; exit 3; }
}
obtener "$BANCO/clienteafirma" "$CLIENTEAFIRMA_REPO" "$CLIENTEAFIRMA_COMMIT"
obtener "$BANCO/fire" "$FIRE_REPO" "$FIRE_COMMIT"

# --- Compilación ---------------------------------------------------------------
WAR="$BANCO/fire/fire-signature/target/fire-signature-2.4.war"
CLIENT_JAR="$BANCO/fire/fire-client-java/target/fire-client-2.4.jar"
if [[ ! -f "$WAR" || ! -f "$CLIENT_JAR" || "${FIRE_BANCO_RECOMPILAR:-0}" == 1 ]]; then
  # Primero el cliente @firma desde la fuente: se instala en el repositorio Maven
  # local del banco y FIRe lo usa en lugar del binario publicado.
  log "Compilando clienteafirma ($CLIENTEAFIRMA_MODULOS y dependencias)"
  (cd "$BANCO/clienteafirma" && "$MVN" -B -q -Dmaven.repo.local="$BANCO/m2" \
      -pl "$CLIENTEAFIRMA_MODULOS" -am install -DskipTests) > "$BANCO/compilacion-clienteafirma.log" 2>&1 \
    || { tail -40 "$BANCO/compilacion-clienteafirma.log" >&2; exit 4; }
  # fire-client-java declara Java 1.6, que los JDK actuales ya no admiten: se compila a 1.8.
  log "Compilando fire-signature y fire-client-java (Maven, sin pruebas)"
  (cd "$BANCO/fire" && "$MVN" -B -q -Dmaven.repo.local="$BANCO/m2" -Pmain,services \
      -Djdk.version=1.8 -pl fire-signature,fire-client-java -am package -DskipTests) > "$BANCO/compilacion.log" 2>&1 \
    || { tail -40 "$BANCO/compilacion.log" >&2; exit 4; }
fi
(cd "$BANCO/fire/fire-client-java" && "$MVN" -B -q -Dmaven.repo.local="$BANCO/m2" \
    dependency:build-classpath -Dmdep.outputFile="$BANCO/fire-client.classpath" -Dmdep.includeScope=runtime) \
  >> "$BANCO/compilacion.log" 2>&1 || { tail -40 "$BANCO/compilacion.log" >&2; exit 4; }
log "WAR: $(sha256sum "$WAR" | cut -c1-16)…"

# --- PKI sintética ---------------------------------------------------------------
PKI="$BANCO/pki"
if [[ ! -f "$PKI/listo" ]]; then
  log "Generando PKI sintética (válida 30 días)"
  rm -rf "$PKI"; mkdir -p "$PKI"; chmod 700 "$PKI"
  # Contraseñas aleatorias de un solo uso; solo viven en el directorio del banco.
  openssl rand -hex 16 > "$PKI/clave-almacenes.txt"
  openssl rand -hex 16 > "$PKI/clave-firmante.txt"
  chmod 600 "$PKI"/*.txt
  ALM="$(cat "$PKI/clave-almacenes.txt")"
  FIR="$(cat "$PKI/clave-firmante.txt")"
  ( cd "$PKI"
    openssl req -x509 -newkey rsa:3072 -nodes -keyout ca.key -out ca.pem -days 30 -sha256 \
      -subj "/C=ES/O=Banco de pruebas FIRe (SINTETICO)/CN=CA sintetica banco FIRe" \
      -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" 2>/dev/null
    emitir() { # nombre sujeto extensiones
      openssl req -newkey rsa:2048 -nodes -keyout "$1.key" -out "$1.csr" -subj "$2" 2>/dev/null
      printf '%b\n' "$3" > "$1.ext"
      openssl x509 -req -in "$1.csr" -CA ca.pem -CAkey ca.key -CAcreateserial -out "$1.pem" \
        -days 30 -sha256 -extfile "$1.ext" 2>/dev/null
      rm -f "$1.csr" "$1.ext"
    }
    emitir servidor "/C=ES/O=Banco de pruebas FIRe (SINTETICO)/CN=127.0.0.1" \
      "basicConstraints=CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.1,DNS:localhost"
    emitir aplicacion "/C=ES/O=Banco de pruebas FIRe (SINTETICO)/CN=Aplicacion portal de pruebas" \
      "basicConstraints=CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth"
    emitir firmante "/C=ES/O=Banco de pruebas FIRe (SINTETICO)/serialNumber=IDCES-99999999R/GN=PRUEBA/SN=SINTETICO/CN=PRUEBA SINTETICO FIRMANTE - 99999999R" \
      "basicConstraints=CA:FALSE\nkeyUsage=critical,digitalSignature,nonRepudiation"
    openssl pkcs12 -export -in servidor.pem -inkey servidor.key -certfile ca.pem -name servidor \
      -out servidor.p12 -passout "pass:$ALM"
    openssl pkcs12 -export -in aplicacion.pem -inkey aplicacion.key -certfile ca.pem -name aplicacion \
      -out aplicacion.p12 -passout "pass:$ALM"
    openssl pkcs12 -export -in firmante.pem -inkey firmante.key -certfile ca.pem -name firmante \
      -out firmante.p12 -passout "pass:$FIR"
    "$JAVA_HOME/bin/keytool" -importcert -noprompt -alias ca -file ca.pem -keystore confianza.p12 \
      -storetype PKCS12 -storepass "$ALM" >/dev/null
    rm -f ca.srl
    chmod 600 ./*.key ./*.p12
  )
  touch "$PKI/listo"
fi
ALM="$(cat "$PKI/clave-almacenes.txt")"

# --- Configuración de FIRe (sin BD, solo proveedor local) ------------------------
CONF="$BANCO/fire-config"
mkdir -p "$CONF" "$BANCO/run/temp" "$BANCO/run/logs" "$BANCO/run/auditoria" "$BANCO/run/estadisticas"
APP_CERT_B64="$(openssl x509 -in "$PKI/aplicacion.pem" -outform DER | base64 -w0)"
FIRE_APP_ID="B4NC0F1RE0000001"
cat > "$CONF/fire_config.properties" <<EOF
# Generado por scripts/fire-banco/preparar.sh. Banco local con PKI sintética.
default.appId=$FIRE_APP_ID
default.certificate=$APP_CERT_B64
security.checkCertificate=true
security.checkApplication=true
validator.class=
params.maxSize=8388608
request.maxSize=12582912
batch.maxDocuments=10
temp.dir=$BANCO/run/temp
temp.fire.timeout=600
skipCertSelection=false
providers=local
local.verification.key=$(openssl rand -hex 16)
docmanager.default=es.gob.fire.server.services.document.DefaultFIReDocumentManager
alarms.notifier=
pages.title=Banco de pruebas FIRe (local)
logs.dir=$BANCO/run/logs
logs.level.fire=INFO
logs.level.afirma=INFO
logs.level=INFO
statistics.policy=0
statistics.dir=$BANCO/run/estadisticas
# FIRe 2.4 falla (NullPointerException en AuditSignatureRecorder) si no hay audit.dir,
# aunque la auditoría esté desactivada.
audit.policy=0
audit.dir=$BANCO/run/auditoria
legacy.services.enabled=false
EOF
# Ficheros auxiliares que FIRe espera encontrar junto a la configuración.
cp "$BANCO/fire/fire-signature/src/main/resources/alarms_config.properties" "$CONF/alarms_config.properties"
echo "$FIRE_APP_ID" > "$BANCO/fire-app-id.txt"

# --- Tomcat (CATALINA_BASE propio, solo 127.0.0.1) --------------------------------
TB="$BANCO/tomcat"
# Si quedó un Tomcat del banco en marcha, se detiene antes de rehacer su directorio.
FIRE_BANCO_DIR="$BANCO" "$AQUI/parar.sh" >/dev/null
rm -rf "$TB"; mkdir -p "$TB"/{conf,logs,temp,webapps,work,bin}
cp "$CATALINA_HOME/conf/web.xml" "$CATALINA_HOME/conf/logging.properties" "$CATALINA_HOME/conf/context.xml" "$TB/conf/"
cat > "$TB/conf/server.xml" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<Server port="$FIRE_PUERTO_APAGADO" address="127.0.0.1" shutdown="$(openssl rand -hex 12)">
  <Listener className="org.apache.catalina.startup.VersionLoggerListener" />
  <Service name="Catalina">
    <Connector port="$FIRE_PUERTO_HTTPS" address="127.0.0.1"
               protocol="org.apache.coyote.http11.Http11NioProtocol"
               SSLEnabled="true" scheme="https" secure="true" maxPostSize="16777216">
      <SSLHostConfig certificateVerification="optional"
                     truststoreFile="$PKI/confianza.p12" truststoreType="PKCS12"
                     truststorePassword="$ALM" protocols="TLSv1.2+TLSv1.3">
        <Certificate certificateKeystoreFile="$PKI/servidor.p12" certificateKeystoreType="PKCS12"
                     certificateKeystorePassword="$ALM" type="RSA" />
      </SSLHostConfig>
    </Connector>
    <Engine name="Catalina" defaultHost="localhost">
      <Host name="localhost" appBase="webapps" unpackWARs="true" autoDeploy="false" />
    </Engine>
  </Service>
</Server>
EOF
chmod 600 "$TB/conf/server.xml"
cp "$WAR" "$TB/webapps/fire-signature.war"
cat > "$TB/bin/setenv.sh" <<EOF
JAVA_HOME="$JAVA_HOME"
CATALINA_PID="$TB/tomcat.pid"
CATALINA_OPTS="-Dfire.config.path=$CONF -Djava.awt.headless=true -Djava.net.preferIPv4Stack=true"
EOF

# --- Configuración del cliente FIRe que hace de portal --------------------------
cat > "$BANCO/portal-config.properties" <<EOF
fireUrl=https://127.0.0.1:$FIRE_PUERTO_HTTPS/fire-signature/fireService
javax.net.ssl.keyStore=$PKI/aplicacion.p12
javax.net.ssl.keyStorePassword=$ALM
javax.net.ssl.keyStoreType=PKCS12
javax.net.ssl.certAlias=aplicacion
javax.net.ssl.trustStore=$PKI/confianza.p12
javax.net.ssl.trustStorePassword=$ALM
javax.net.ssl.trustStoreType=PKCS12
verify.hostnames=true
EOF
chmod 600 "$BANCO/portal-config.properties"

log "Banco preparado en $BANCO. Arranque con scripts/fire-banco/arrancar.sh"
