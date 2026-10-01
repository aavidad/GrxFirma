#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
PROJECT_DIR="$ROOT_DIR/mobile/android"
SDK_ROOT=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
PACKAGE=es.dipgra.grxfirma.debug
TEST_PACKAGE=es.dipgra.grxfirma.debug.test
RUNNER=androidx.test.runner.AndroidJUnitRunner
TEST_CLASS=es.dipgra.grxfirma.android.CoreOfficialTestCertificateSigningTest
EXPECTED_P12_SHA256=6e0cad97b78be2918ed54a64a0dd4f3f6e4c16e01b405ef0836fb91b77a3ffb4
P12=${GRXFIRMA_ANDROID_OFFICIAL_TEST_P12:-}
PASSWORD_VALUE=${GRXFIRMA_ANDROID_OFFICIAL_TEST_PASSWORD:-}
PASSWORD_SOURCE=${GRXFIRMA_ANDROID_OFFICIAL_TEST_PASSWORD_FILE:-}
OUTPUT_DIR=${1:-"$ROOT_DIR/build/android-official-signing-smoke"}
PASSWORD_FILE=

if [[ -z "$SDK_ROOT" || ! -x "$SDK_ROOT/platform-tools/adb" ]]; then
    printf '%s\n' "ERROR: adb no está disponible en ANDROID_HOME/ANDROID_SDK_ROOT" >&2
    exit 1
fi
if [[ -z "${GRXFIRMA_ANDROID_CORE_AAR:-}" ||
      -z "${GRXFIRMA_ANDROID_CORE_SHA256:-}" ]]; then
    printf '%s\n' "ERROR: defina el AAR productivo y su SHA-256 aprobado" >&2
    exit 1
fi
if [[ -z "$P12" || ! -f "$P12" ]]; then
    printf '%s\n' \
        "ERROR: GRXFIRMA_ANDROID_OFFICIAL_TEST_P12 debe apuntar al P12 QA oficial" >&2
    exit 1
fi
if [[ -n "$PASSWORD_VALUE" && -n "$PASSWORD_SOURCE" ]]; then
    printf '%s\n' \
        "ERROR: use contraseña QA por entorno o por fichero, no ambos" >&2
    exit 1
fi
if [[ -z "$PASSWORD_VALUE" && -z "$PASSWORD_SOURCE" ]]; then
    printf '%s\n' \
        "ERROR: defina la contraseña QA por entorno o mediante un fichero privado" >&2
    exit 1
fi
if [[ -n "$PASSWORD_VALUE" ]] && (( ${#PASSWORD_VALUE} > 1024 )); then
    printf '%s\n' "ERROR: la contraseña QA supera el límite permitido" >&2
    exit 1
fi
if [[ -n "$PASSWORD_SOURCE" ]]; then
    if [[ ! -f "$PASSWORD_SOURCE" || -L "$PASSWORD_SOURCE" ]]; then
        printf '%s\n' "ERROR: el fichero de contraseña QA no es regular y privado" >&2
        exit 1
    fi
    if [[ "$(stat -c '%a' "$PASSWORD_SOURCE")" != "600" ]]; then
        printf '%s\n' "ERROR: el fichero de contraseña QA debe tener permisos 0600" >&2
        exit 1
    fi
fi
actual_p12_sha256=$(sha256sum "$P12" | awk '{print $1}')
if [[ "$actual_p12_sha256" != "$EXPECTED_P12_SHA256" ]]; then
    printf 'ERROR: huella inesperada para el P12 QA oficial: %s\n' "$actual_p12_sha256" >&2
    exit 1
fi

ADB="$SDK_ROOT/platform-tools/adb"
if ! "$ADB" devices | awk 'NR > 1 && $2 == "device" { found=1 } END { exit !found }'; then
    printf '%s\n' "ERROR: no hay un emulador o dispositivo Android listo" >&2
    exit 1
fi

cleanup() {
    "$ADB" shell rm -f \
        /data/local/tmp/grxfirma-fnmt-oficial.pdf >/dev/null 2>&1 || true
    "$ADB" shell run-as "$PACKAGE" rm -rf \
        files/qa-input \
        files/qa-output-official >/dev/null 2>&1 || true
    if [[ -n "$PASSWORD_FILE" && -f "$PASSWORD_FILE" ]]; then
        if command -v shred >/dev/null 2>&1; then
            shred -u -- "$PASSWORD_FILE"
        else
            rm -f -- "$PASSWORD_FILE"
        fi
    fi
}
trap cleanup EXIT HUP INT TERM

umask 077
PASSWORD_FILE=$(mktemp "${TMPDIR:-/tmp}/grxfirma-android-fnmt-password.XXXXXX")
if [[ -n "$PASSWORD_SOURCE" ]]; then
    install -m 0600 -- "$PASSWORD_SOURCE" "$PASSWORD_FILE"
else
    printf '%s' "$PASSWORD_VALUE" >"$PASSWORD_FILE"
fi
chmod 0600 "$PASSWORD_FILE"
unset PASSWORD_VALUE PASSWORD_SOURCE \
    GRXFIRMA_ANDROID_OFFICIAL_TEST_PASSWORD \
    GRXFIRMA_ANDROID_OFFICIAL_TEST_PASSWORD_FILE
password_size=$(wc -c <"$PASSWORD_FILE")
if (( password_size == 0 || password_size > 1024 )); then
    printf '%s\n' "ERROR: el secreto QA codificado tiene un tamaño no permitido" >&2
    exit 1
fi

(
    cd "$PROJECT_DIR"
    ./gradlew --no-daemon --no-configuration-cache \
        assembleProductionDebug \
        assembleProductionDebugAndroidTest
)

APP_APK="$PROJECT_DIR/app/build/outputs/apk/production/debug/app-production-debug.apk"
TEST_APK="$PROJECT_DIR/app/build/outputs/apk/androidTest/production/debug/app-production-debug-androidTest.apk"
"$ADB" install -r -t "$APP_APK" >/dev/null
"$ADB" install -r -t "$TEST_APK" >/dev/null
"$ADB" push "$ROOT_DIR/test/prueba1.pdf" \
    /data/local/tmp/grxfirma-fnmt-oficial.pdf >/dev/null
"$ADB" shell run-as "$PACKAGE" mkdir -p files/qa-input files/qa-output-official
"$ADB" shell run-as "$PACKAGE" sh -c \
    "'umask 077; cat > files/qa-input/grxfirma-fnmt-oficial.p12'" <"$P12"
"$ADB" shell run-as "$PACKAGE" cp \
    /data/local/tmp/grxfirma-fnmt-oficial.pdf \
    files/qa-input/grxfirma-fnmt-oficial.pdf
"$ADB" shell run-as "$PACKAGE" sh -c \
    "'umask 077; cat > files/qa-input/grxfirma-fnmt-oficial.password'" <"$PASSWORD_FILE"
"$ADB" shell run-as "$PACKAGE" chmod 0600 \
    files/qa-input/grxfirma-fnmt-oficial.p12 \
    files/qa-input/grxfirma-fnmt-oficial.password

instrumentation_output=$(
    "$ADB" shell am instrument \
        -w \
        -r \
        -e class "$TEST_CLASS" \
        "$TEST_PACKAGE/$RUNNER"
)
printf '%s\n' "$instrumentation_output"
if ! grep -Fq "OK (1 test)" <<<"$instrumentation_output"; then
    printf '%s\n' "ERROR: la firma Android con el certificado oficial QA falló" >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR"
for artifact in \
    firma-fnmt-oficial-pades.pdf \
    firma-fnmt-oficial-cades.p7s \
    firma-fnmt-oficial-xades.xml \
    verificacion-fnmt-oficial.txt; do
    "$ADB" exec-out run-as "$PACKAGE" cat "files/qa-output-official/$artifact" \
        >"$OUTPUT_DIR/$artifact"
    if [[ ! -s "$OUTPUT_DIR/$artifact" ]]; then
        printf 'ERROR: evidencia Android vacía: %s\n' "$artifact" >&2
        exit 1
    fi
done

printf '%s\n' "GrxFirma Android official FNMT QA" >"$OUTPUT_DIR/original-cades.txt"
openssl cms \
    -verify \
    -binary \
    -inform DER \
    -in "$OUTPUT_DIR/firma-fnmt-oficial-cades.p7s" \
    -content "$OUTPUT_DIR/original-cades.txt" \
    -noverify \
    -out /dev/null >/dev/null
if command -v pdfsig >/dev/null 2>&1; then
    pdfsig_report=$(pdfsig "$OUTPUT_DIR/firma-fnmt-oficial-pades.pdf" 2>&1 || true)
    if ! grep -Fq "Signature Validation: Signature is Valid." <<<"$pdfsig_report"; then
        printf '%s\n' "ERROR: pdfsig no confirmó la integridad PAdES oficial" >&2
        exit 1
    fi
fi
if command -v qpdf >/dev/null 2>&1; then
    qpdf --check "$OUTPUT_DIR/firma-fnmt-oficial-pades.pdf" >/dev/null
fi
if command -v go >/dev/null 2>&1 && command -v python3 >/dev/null 2>&1; then
    (
        cd "$ROOT_DIR"
        go run ./cmd/grxfirma \
            -modo-cli \
            -operacion verificar \
            -entrada "$OUTPUT_DIR/firma-fnmt-oficial-xades.xml" \
            -salida-json
    ) >"$OUTPUT_DIR/verificacion-xades-cli.json"
    python3 - "$OUTPUT_DIR/verificacion-xades-cli.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    report = json.load(source)
result = report.get("resultado", {})
integrity = result.get("integridad", {})
if result.get("formato") != "XAdES" or integrity.get("estado") != "valid":
    raise SystemExit("la verificación XAdES en proceso independiente no confirmó la integridad")
PY
else
    printf '%s\n' \
        "AVISO: no se ejecutó la verificación XAdES separada; faltan Go o Python 3" >&2
fi

sha256sum "$OUTPUT_DIR"/*
printf '%s\n' \
    "Firmas oficiales QA verificadas; integridad, vigencia y confianza se informan por separado."
