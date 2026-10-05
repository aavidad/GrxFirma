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
PACKAGE=io.github.aavidad.grxfirma.debug
TEST_PACKAGE=io.github.aavidad.grxfirma.debug.test
RUNNER=androidx.test.runner.AndroidJUnitRunner
TEST_CLASS=${GRXFIRMA_ANDROID_TEST_CLASS:-io.github.aavidad.grxfirma.android.CoreProductionSigningTest}
PDF_ARTIFACT=${GRXFIRMA_ANDROID_PDF_ARTIFACT:-firma-android-qa-pades.pdf}
PASSWORD=valor-prueba
SUBJECT="GrxFirma Android QA synthetic"
OUTPUT_DIR=${1:-"$ROOT_DIR/build/android-signing-smoke"}

if [[ -z "$SDK_ROOT" || ! -x "$SDK_ROOT/platform-tools/adb" ]]; then
    printf '%s\n' "ERROR: adb no está disponible en ANDROID_HOME/ANDROID_SDK_ROOT" >&2
    exit 1
fi
if [[ -z "${GRXFIRMA_ANDROID_CORE_AAR:-}" ||
      -z "${GRXFIRMA_ANDROID_CORE_SHA256:-}" ]]; then
    printf '%s\n' "ERROR: defina el AAR productivo y su SHA-256 aprobado" >&2
    exit 1
fi
for command in openssl python3; do
    if ! command -v "$command" >/dev/null 2>&1; then
        printf 'ERROR: falta la herramienta requerida: %s\n' "$command" >&2
        exit 1
    fi
done

ADB="$SDK_ROOT/platform-tools/adb"
if ! "$ADB" devices | awk 'NR > 1 && $2 == "device" { found=1 } END { exit !found }'; then
    printf '%s\n' "ERROR: no hay un emulador o dispositivo Android listo" >&2
    exit 1
fi

WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-android-signing.XXXXXX")
cleanup() {
    "$ADB" shell rm -f \
        /data/local/tmp/grxfirma-android-qa.p12 \
        /data/local/tmp/grxfirma-android-qa.pdf >/dev/null 2>&1 || true
    "$ADB" shell run-as "$PACKAGE" rm -rf files/qa-input >/dev/null 2>&1 || true
    rm -rf -- "$WORK_DIR"
}
trap cleanup EXIT HUP INT TERM

openssl req \
    -x509 \
    -newkey rsa:2048 \
    -sha256 \
    -nodes \
    -days 2 \
    -subj "/C=ES/O=GrxFirma QA/CN=$SUBJECT" \
    -addext "basicConstraints=critical,CA:FALSE" \
    -addext "keyUsage=critical,digitalSignature" \
    -addext "extendedKeyUsage=clientAuth,emailProtection" \
    -keyout "$WORK_DIR/key.pem" \
    -out "$WORK_DIR/certificate.pem" >/dev/null 2>&1
openssl pkcs12 \
    -export \
    -inkey "$WORK_DIR/key.pem" \
    -in "$WORK_DIR/certificate.pem" \
    -name "$SUBJECT" \
    -passout "pass:$PASSWORD" \
    -out "$WORK_DIR/grxfirma-android-qa.p12"

install -m 0600 \
    "$ROOT_DIR/test/prueba1.pdf" \
    "$WORK_DIR/grxfirma-android-qa.pdf"
printf '%s\n' "GrxFirma Android QA" >"$WORK_DIR/android-qa.txt"

(
    cd "$PROJECT_DIR"
    ./gradlew --no-daemon --no-configuration-cache \
        assembleProductionDebug \
        assembleProductionDebugAndroidTest
)

APP_APK="$PROJECT_DIR/app/build/outputs/apk/production/debug/app-production-debug.apk"
TEST_APK="$PROJECT_DIR/app/build/outputs/apk/androidTest/production/debug/app-production-debug-androidTest.apk"
if [[ ! -f "$APP_APK" || ! -f "$TEST_APK" ]]; then
    printf '%s\n' "ERROR: no se generaron los APK de producción y prueba" >&2
    exit 1
fi

"$ADB" install -r -t "$APP_APK" >/dev/null
"$ADB" install -r -t "$TEST_APK" >/dev/null
"$ADB" push "$WORK_DIR/grxfirma-android-qa.p12" \
    /data/local/tmp/grxfirma-android-qa.p12 >/dev/null
"$ADB" push "$WORK_DIR/grxfirma-android-qa.pdf" \
    /data/local/tmp/grxfirma-android-qa.pdf >/dev/null
"$ADB" shell run-as "$PACKAGE" mkdir -p files/qa-input files/qa-output
"$ADB" shell run-as "$PACKAGE" cp \
    /data/local/tmp/grxfirma-android-qa.p12 \
    files/qa-input/grxfirma-android-qa.p12
"$ADB" shell run-as "$PACKAGE" cp \
    /data/local/tmp/grxfirma-android-qa.pdf \
    files/qa-input/grxfirma-android-qa.pdf

instrumentation_output=$(
    "$ADB" shell am instrument \
        -w \
        -r \
        -e class "$TEST_CLASS" \
        "$TEST_PACKAGE/$RUNNER"
)
printf '%s\n' "$instrumentation_output"
if ! grep -Fq "OK (1 test)" <<<"$instrumentation_output"; then
    printf '%s\n' "ERROR: la campaña de firma productiva Android no terminó correctamente" >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR"
artifacts=("$PDF_ARTIFACT")
if [[ "${GRXFIRMA_ANDROID_TEST_ONLY_PDF:-0}" != "1" ]]; then
    artifacts+=(firma-android-qa-cades.p7s firma-android-qa-xades.xml)
fi
for artifact in "${artifacts[@]}"; do
    "$ADB" exec-out run-as "$PACKAGE" cat "files/qa-output/$artifact" \
        >"$OUTPUT_DIR/$artifact"
    if [[ ! -s "$OUTPUT_DIR/$artifact" ]]; then
        printf 'ERROR: evidencia Android vacía: %s\n' "$artifact" >&2
        exit 1
    fi
done

if [[ "${GRXFIRMA_ANDROID_TEST_ONLY_PDF:-0}" != "1" ]] && ! openssl cms \
    -verify \
    -binary \
    -inform DER \
    -in "$OUTPUT_DIR/firma-android-qa-cades.p7s" \
    -content "$WORK_DIR/android-qa.txt" \
    -noverify \
    -out /dev/null >/dev/null 2>&1; then
    printf '%s\n' "ERROR: OpenSSL no pudo verificar la firma CAdES separada" >&2
    exit 1
fi
if command -v pdfsig >/dev/null 2>&1; then
    pdfsig_report=$(pdfsig "$OUTPUT_DIR/$PDF_ARTIFACT" 2>&1 || true)
    if ! grep -Fq "Signature Validation: Signature is Valid." <<<"$pdfsig_report"; then
        printf '%s\n' "ERROR: pdfsig no confirmó la integridad PAdES" >&2
        exit 1
    fi
fi

"$ADB" shell run-as "$PACKAGE" rm -rf files/qa-output
sha256sum "$OUTPUT_DIR"/*
printf 'Firmas Android verificadas y extraídas en %s\n' "$OUTPUT_DIR"
