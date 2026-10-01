#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

OUT_DIR="${1:-/tmp/grxfirma-pades-cierre}"
REPORT_PATH="${OUT_DIR}/reporte.txt"
V1_FIXTURE="test/regression/fixtures/v1/samples/2_signed.pdf"

mkdir -p "${OUT_DIR}"

{
  echo "GrxFirma - cierre local PAdES"
  echo "fecha: $(date -Iseconds)"
  echo
  echo "1. Generando muestras V2"
} > "${REPORT_PATH}"

{
  GOCACHE=/tmp/grxfirma-gocache go run ./scripts/generar_muestras_pades.go -out "${OUT_DIR}"
  echo
  echo "2. Validando muestras V2 con qpdf/pdfsig"
  bash scripts/validar_pades_manual.sh \
    "${OUT_DIR}/pades-etsi.pdf" \
    "${OUT_DIR}/pades-adobe.pdf" \
    "${OUT_DIR}/pades-adobe-t.pdf"
  echo
  echo "3. Validando fixture PAdES firmado con certificado de pruebas"
  if [[ -f "${V1_FIXTURE}" ]]; then
    echo "==> ${V1_FIXTURE}"
    qpdf --check "${V1_FIXTURE}"
    pdfsig "${V1_FIXTURE}"
  else
    echo "OMITIDO: falta ${V1_FIXTURE}; firmar el 2.pdf sintético con el certificado FNMT de pruebas."
  fi
  echo
  echo "4. Resultado"
  echo "reporte: ${REPORT_PATH}"
} >> "${REPORT_PATH}" 2>&1

cat "${REPORT_PATH}"
