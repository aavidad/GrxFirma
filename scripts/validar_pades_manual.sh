#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

if [[ $# -lt 1 ]]; then
  cat <<'EOF'
Uso:
  bash scripts/validar_pades_manual.sh <pdf-firmado> [<pdf-firmado>...]

Valida cada PDF con:
  - pdfsig
  - qpdf --check

El script no decide la confianza del certificado. Solo ayuda a comprobar:
  - que el PDF está bien formado
  - que la firma se detecta
  - que pdfsig la considera válida estructuralmente
EOF
  exit 1
fi

if ! command -v pdfsig >/dev/null 2>&1; then
  echo "ERROR: no se encontró 'pdfsig' en PATH" >&2
  exit 2
fi

if ! command -v qpdf >/dev/null 2>&1; then
  echo "ERROR: no se encontró 'qpdf' en PATH" >&2
  exit 2
fi

status=0

for pdf in "$@"; do
  echo "==> ${pdf}"

  if [[ ! -f "${pdf}" ]]; then
    echo "ERROR: no existe el fichero" >&2
    status=1
    echo
    continue
  fi

  echo "-- qpdf --check"
  if ! qpdf --check "${pdf}"; then
    echo "ERROR: qpdf detectó problemas estructurales" >&2
    status=1
  fi

  echo "-- pdfsig"
  pdfsig_output="$(pdfsig "${pdf}" 2>&1 || true)"
  echo "${pdfsig_output}"

  if ! grep -q "Signature #1:" <<<"${pdfsig_output}"; then
    echo "ERROR: pdfsig no detectó ninguna firma" >&2
    status=1
  fi

  if ! grep -q "Signature Validation: Signature is Valid." <<<"${pdfsig_output}"; then
    echo "ERROR: pdfsig no validó la firma como válida" >&2
    status=1
  fi

  if grep -q "Signature Type: ETSI.CAdES.detached" <<<"${pdfsig_output}"; then
    echo "INFO: subfiltro ETSI detectado"
  elif grep -q "Signature Type: adbe.pkcs7.detached" <<<"${pdfsig_output}"; then
    echo "INFO: subfiltro Adobe detectado"
  else
    echo "WARN: pdfsig no mostró un tipo de firma esperado"
  fi

  echo
done

exit "${status}"
