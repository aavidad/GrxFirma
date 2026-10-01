#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Gate explícito de la frontera Linux experimental. No activa la capacidad ni
# instala herramientas/certificados. Una omisión por entorno NO es un PASS.
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_dir}"
if [[ "$(go env GOOS)" != linux || "$(go env CGO_ENABLED)" != 1 ]]; then
  echo "error: el gate de aislamiento requiere Linux y CGo real" >&2
  exit 1
fi
case "$(go env GOARCH)" in
  amd64|arm64) ;;
  *) echo "error: arquitectura sin política seccomp de producción" >&2; exit 1 ;;
esac
GRXFIRMA_REQUIRE_PKCS11_SANDBOX=1 go test -race -count=1 -timeout=5m \
  ./internal/testsupport/pkcs11sandbox \
  ./internal/adapters/outbound/desktop/pkcs11worker \
  ./internal/adapters/outbound/desktop/isolatedtokenstore \
  ./cmd/grxfirma-pkcs11-worker
echo "Frontera de aislamiento PKCS#11: gate requerido PASS; no acredita hardware ni firma."
