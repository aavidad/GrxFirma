#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

#
# T114 — Gate de seguridad de release.
#
# Controles no negociables previos a publicar. Falla (exit 1) si cualquiera no
# se cumple. Reutilizable como job de CI. Agrupa:
#   1. Higiene de secretos (check-secrets.sh).
#   2. Ausencia de bypass TLS genérico en el bridge QML.
#   3. Pinning TLS local presente en ambos bridges.
#   4. Manifest de extensión sin permisos de host demasiado amplios.
#   5. Ausencia de cargadores de plugins runtime o intérpretes embebidos.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fail=0

fallo() {
  echo "GATE FALLA: $*" >&2
  fail=1
}

# ---------------------------------------------------------------------------
# 1. Higiene de secretos
# ---------------------------------------------------------------------------
if ! "${ROOT_DIR}/scripts/release/check-secrets.sh" "$@"; then
  fallo "check-secrets.sh detectó artefactos sensibles."
fi

# ---------------------------------------------------------------------------
# 2. Bridge QML: prohibido ignoreSslErrors() genérico (sin argumentos), que
#    ignora TODOS los errores TLS. Solo se admite ignoreSslErrors(errors) tras
#    verificar el pin (ver control 3).
# ---------------------------------------------------------------------------
qml_bridges=(
  "${ROOT_DIR}/cmd/gui-qml/ipcbridge.cpp"
  "${ROOT_DIR}/cmd/gui-qml/backendbridge.cpp"
)
for bridge in "${qml_bridges[@]}"; do
  [[ -f "${bridge}" ]] || continue
  # ignoreSslErrors seguido de () vacío = bypass genérico.
  if grep -nE 'ignoreSslErrors\s*\(\s*\)' "${bridge}" >/dev/null 2>&1; then
    fallo "bypass TLS genérico ignoreSslErrors() en ${bridge#"${ROOT_DIR}"/}"
  fi
  # Debe existir la comprobación de pin.
  if ! grep -q 'IpcBridgePeerMatchesPinnedLocalTLSCert\|ExpectedLocalTLSPins' "${bridge}"; then
    fallo "falta la verificación de pin TLS local en ${bridge#"${ROOT_DIR}"/}"
  fi
done

# ---------------------------------------------------------------------------
# 3. Manifest de extensión Chromium: host_permissions y content_scripts no
#    pueden usar comodines universales.
# ---------------------------------------------------------------------------
manifest="${ROOT_DIR}/packaging/browser-extensions/src/chromium/manifest.json"
if [[ -f "${manifest}" ]]; then
  python3 - "${manifest}" <<'PY' || fail=1
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as fh:
    m = json.load(fh)

prohibidos = ("<all_urls>", "*://*/*", "http://*/*", "https://*/*", "*://*")
problemas = []


def revisar(lista, etiqueta):
    for patron in lista or []:
        if patron in prohibidos or patron.strip() in ("*", "<all_urls>"):
            problemas.append(f"{etiqueta}: patrón demasiado amplio {patron!r}")


revisar(m.get("host_permissions"), "host_permissions")
for cs in m.get("content_scripts", []) or []:
    revisar(cs.get("matches"), "content_scripts.matches")

# Permisos peligrosos que no debería pedir esta extensión.
peligrosos = {"<all_urls>", "tabs", "webRequest", "webRequestBlocking", "cookies", "history", "debugger"}
for p in m.get("permissions", []) or []:
    if p in peligrosos:
        problemas.append(f"permissions: permiso peligroso {p!r}")

if problemas:
    print("GATE FALLA: manifest de extensión:", file=sys.stderr)
    for pb in problemas:
        print(f"  - {pb}", file=sys.stderr)
    sys.exit(1)
PY
else
  echo "aviso: no se encontró el manifest de extensión Chromium (${manifest#"${ROOT_DIR}"/})" >&2
fi

# ---------------------------------------------------------------------------
# 5. La decisión de arquitectura de T096 prohíbe cargar código aportado por el
#    usuario dentro del proceso que maneja documentos, certificados y claves.
# ---------------------------------------------------------------------------
if ! python3 "${ROOT_DIR}/scripts/ci/check_no_runtime_plugins.py" --root "${ROOT_DIR}"; then
  fallo "se detectó una superficie de plugins runtime prohibida por ADR-004."
fi

# ---------------------------------------------------------------------------
if [[ "${fail}" -ne 0 ]]; then
  echo "check-release-gate.sh: FALLÓ. No publicar." >&2
  exit 1
fi
echo "check-release-gate.sh: todos los controles de seguridad de release OK."
