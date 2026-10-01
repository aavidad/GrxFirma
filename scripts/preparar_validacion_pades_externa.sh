#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

OUT_DIR="${1:-/tmp/grxfirma-pades-validacion-externa}"
MUESTRAS_DIR="${OUT_DIR}/muestras"
EVIDENCIAS_DIR="${OUT_DIR}/evidencias"
ACTAS_DIR="${OUT_DIR}/actas"
README_PATH="${OUT_DIR}/README_VALIDACION_PADES.md"
V1_FIXTURE="test/regression/fixtures/v1/samples/2_signed.pdf"

mkdir -p "${MUESTRAS_DIR}" "${EVIDENCIAS_DIR}" "${ACTAS_DIR}"

cat > "${README_PATH}" <<EOF
# Paquete de validación externa PAdES — GrxFirma

Fecha de preparación: $(date -Iseconds)

## Contenido

- \`muestras/\`
  - \`pades-etsi.pdf\`
  - \`pades-adobe.pdf\`
  - \`pades-adobe-t.pdf\`
  - \`pades-fnmt-pruebas.pdf\` (si se añadió el fixture firmado)
- \`evidencias/\`
  - \`validacion-local.txt\`
  - \`sha256sum.txt\`
- \`actas/\`: comprobaciones manuales del operador.

## Objetivo

Rematar el cierre externo de:

- Generación y verificación PAdES con muestras sintéticas.

El paquete deja preparadas muestras de prueba para
validación manual en:

- Adobe Acrobat Reader
- Okular
- Evince u otro visor relevante

## Pasos recomendados

1. Abrir cada PDF de \`muestras/\` en Acrobat y Okular.
2. Verificar:
   - que el PDF abre sin corrupción;
   - que el visor detecta la firma;
   - que el visor muestra el firmante;
   - que el visor no informa alteración inesperada;
   - que el sello temporal, si existe, se interpreta correctamente o deja un
     comportamiento claro.
3. Anotar los resultados en \`actas/comprobaciones.md\`.
4. Guardar capturas o notas en \`evidencias/\`.

## Comprobación local ya ejecutable

Antes de la validación manual externa, se puede regenerar la evidencia local:

\`\`\`bash
bash scripts/validar_pades_manual.sh muestras/pades-etsi.pdf muestras/pades-adobe.pdf muestras/pades-adobe-t.pdf
\`\`\`

## Observaciones

- La muestra FNMT es opcional hasta que se regenere a partir del PDF propio.
- Si se dispone de un PDF firmado por Acrobat, conviene añadirlo a \`muestras/\`
  y anotarlo en el acta.
EOF

GOCACHE=/tmp/grxfirma-gocache go run ./scripts/generar_muestras_pades.go -out "${MUESTRAS_DIR}" > "${EVIDENCIAS_DIR}/generacion-muestras.txt"

cat > "${ACTAS_DIR}/comprobaciones.md" <<'EOF'
# Comprobación manual de PAdES

Para cada muestra, anote visor y versión, apertura, firma detectada, identidad de
pruebas, integridad y cualquier aviso mostrado.
EOF

samples=("pades-etsi.pdf" "pades-adobe.pdf" "pades-adobe-t.pdf")
if [[ -f "${V1_FIXTURE}" ]]; then
  cp "${V1_FIXTURE}" "${MUESTRAS_DIR}/pades-fnmt-pruebas.pdf"
  samples+=("pades-fnmt-pruebas.pdf")
else
  echo "OMITIDO: falta ${V1_FIXTURE}; firmar el 2.pdf sintético con el certificado FNMT de pruebas." \
    > "${EVIDENCIAS_DIR}/fixture-fnmt-pendiente.txt"
fi

sample_paths=()
for sample in "${samples[@]}"; do sample_paths+=("${MUESTRAS_DIR}/${sample}"); done
bash scripts/validar_pades_manual.sh "${sample_paths[@]}" > "${EVIDENCIAS_DIR}/validacion-local.txt"

(
  cd "${MUESTRAS_DIR}"
  sha256sum "${samples[@]}"
) > "${EVIDENCIAS_DIR}/sha256sum.txt"

cat <<EOF
Paquete preparado en:
  ${OUT_DIR}

Artefactos principales:
  ${README_PATH}
  ${ACTAS_DIR}/comprobaciones.md
  ${EVIDENCIAS_DIR}/validacion-local.txt
  ${EVIDENCIAS_DIR}/sha256sum.txt
EOF
