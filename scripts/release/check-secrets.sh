#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

#
# Higiene de release y secretos locales.
#
# Falla si detecta artefactos sensibles de depuracion (logs de claves TLS,
# claves privadas SSH, ficheros .env, etc.) en el arbol o en un directorio de
# empaquetado. Sirve como gate previo a release y como chequeo de CI.
#
# Uso:
#   scripts/release/check-secrets.sh [dir1 dir2 ...]
#
# Sin argumentos escanea la raiz del repositorio (excluyendo .git). Con
# argumentos escanea esos directorios (p. ej. release/installers) de forma
# exhaustiva, incluyendo el propio arbol empaquetado.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# Ruta absoluta de este propio script: contiene los marcadores de deteccion como
# literales y debe excluirse para no auto-detectarse.
SELF_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

# Directorios a ignorar siempre (historial git y cache de herramientas).
PRUNE_DIRS=(".git" ".codebase-memory" "node_modules")

# Patrones de nombre de fichero que casi siempre indican material sensible.
NAME_PATTERNS=(
  "*.ssl-key.log" "ssl-key*.log" "sslkey*.log" "sslkeylog*"
  "keylog*.txt" "*keylog*.log"
  "id_rsa" "id_dsa" "id_ecdsa" "id_ed25519"
  ".env" ".env.*"
  ".ssl-key.log"
)

# Una entrada NSS/OpenSSL SSLKEYLOGFILE real empieza por la etiqueta, el
# client_random hexadecimal de 32 bytes y el secreto hexadecimal. Exigir la
# estructura completa evita que las listas defensivas y sus fixtures de prueba
# se detecten a si mismos sin dejar pasar material de sesion utilizable.
CONTENT_MARKERS='^[[:space:]]*(CLIENT_RANDOM|CLIENT_HANDSHAKE_TRAFFIC_SECRET|SERVER_HANDSHAKE_TRAFFIC_SECRET|CLIENT_TRAFFIC_SECRET_0|SERVER_TRAFFIC_SECRET_0|EXPORTER_SECRET)[[:space:]]+[[:xdigit:]]{64}[[:space:]]+[[:xdigit:]]{32,}([[:space:]]|$)'

targets=()
scan_repository_sources=0
if [[ "$#" -gt 0 ]]; then
  targets=("$@")
else
  targets=("${ROOT_DIR}")
  scan_repository_sources=1
fi

found=0

build_prune_expr() {
  local expr=()
  local first=1
  for d in "${PRUNE_DIRS[@]}"; do
    if [[ "${first}" -eq 1 ]]; then
      first=0
    else
      expr+=("-o")
    fi
    expr+=("-name" "${d}")
  done
  printf '%s\n' "${expr[@]}"
}

mapfile -t PRUNE_EXPR < <(build_prune_expr)

build_name_expr() {
  local expr=()
  local first=1
  local pattern
  for pattern in "${NAME_PATTERNS[@]}"; do
    if [[ "${first}" -eq 1 ]]; then
      first=0
    else
      expr+=("-o")
    fi
    expr+=("-name" "${pattern}")
  done
  printf '%s\n' "${expr[@]}"
}

mapfile -t NAME_EXPR < <(build_name_expr)

for target in "${targets[@]}"; do
  [[ -e "${target}" ]] || continue

  # 1) Deteccion por nombre de fichero. Una sola travesia evita recorrer
  #    repetidamente arboles de build grandes por cada patron.
  while IFS= read -r -d '' hit; do
    echo "error: artefacto sensible por nombre: ${hit}" >&2
    found=1
  done < <(
    find "${target}" \
      \( "${PRUNE_EXPR[@]}" \) -prune -o \
      -type f \( "${NAME_EXPR[@]}" \) -print0 2>/dev/null
  )

  # 2) Deteccion por contenido (logs de claves TLS). Solo ficheros de texto
  #    pequenos para no penalizar el escaneo con binarios grandes. grep recibe
  #    lotes NUL-safe: no se lanza un proceso por cada fichero y solo devuelve
  #    nombres, nunca el contenido sensible.
  while IFS= read -r -d '' file; do
    if [[ "${scan_repository_sources}" -eq 1 ]]; then
      [[ "${ROOT_DIR}/${file}" == "${SELF_PATH}" ]] && continue
    else
      [[ "${file}" == "${SELF_PATH}" ]] && continue
    fi
    echo "error: artefacto con secretos TLS (formato SSLKEYLOGFILE): ${file}" >&2
    found=1
  done < <(
    if [[ "${scan_repository_sources}" -eq 1 ]]; then
      # En la raiz, Git proporciona exactamente el codigo versionado y los
      # ficheros nuevos no ignorados. Los directorios generados (Gradle,
      # paquetes Windows, caches...) se validan de forma exhaustiva cuando se
      # pasan expresamente al ensamblador de release.
      (
        cd "${ROOT_DIR}"
        git ls-files -co --exclude-standard -z |
          xargs -0 -r grep -IElZ "${CONTENT_MARKERS}" 2>/dev/null || true
      )
    else
      find "${target}" \
        \( "${PRUNE_EXPR[@]}" \) -prune -o \
        -type f -size -1048576c -print0 2>/dev/null |
        xargs -0 -r grep -IElZ "${CONTENT_MARKERS}" 2>/dev/null || true
    fi
  )
done

if [[ "${found}" -ne 0 ]]; then
  echo "error: check-secrets.sh ha detectado artefactos sensibles; aborta el release." >&2
  echo "       Elimina los ficheros listados (revisa SSLKEYLOGFILE en tu entorno) y reintenta." >&2
  exit 1
fi

echo "check-secrets.sh: sin artefactos sensibles."
