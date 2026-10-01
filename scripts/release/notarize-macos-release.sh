#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Submit the signed PKG using credentials held in the explicit ephemeral keychain.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Uso: notarize-macos-release.sh DIRECTORIO_DE_ARTEFACTOS" >&2
  exit 2
fi
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: la notarizacion oficial requiere macOS." >&2
  exit 1
fi
: "${MACOS_RELEASE_KEYCHAIN:?falta MACOS_RELEASE_KEYCHAIN}"
: "${MACOS_RELEASE_NOTARY_PROFILE:?falta MACOS_RELEASE_NOTARY_PROFILE}"

artifact_dir="$1"
shopt -s nullglob
packages=("${artifact_dir}"/*.pkg)
shopt -u nullglob
if (( ${#packages[@]} != 1 )); then
  echo "error: se esperaba exactamente un PKG para notarizar y hay ${#packages[@]}." >&2
  exit 1
fi

response_tmp="$(mktemp)"
trap 'rm -f "${response_tmp}"' EXIT
xcrun notarytool submit "${packages[0]}" \
  --keychain-profile "${MACOS_RELEASE_NOTARY_PROFILE}" \
  --keychain "${MACOS_RELEASE_KEYCHAIN}" \
  --wait \
  --output-format json > "${response_tmp}"

python3 - "${response_tmp}" <<'PY'
import json
import pathlib
import sys

source = pathlib.Path(sys.argv[1])
payload = json.loads(source.read_text(encoding="utf-8"))
if payload.get("status") != "Accepted":
    raise SystemExit(f"Apple notary service did not accept the PKG: {payload}")
if not payload.get("id"):
    raise SystemExit("Apple notary response does not contain a submission id")
PY

xcrun stapler staple "${packages[0]}"
xcrun stapler validate "${packages[0]}"
python3 - "${response_tmp}" "${packages[0]}" "${artifact_dir}/MACOS-NOTARIZATION.json" <<'PY'
import hashlib
import json
import pathlib
import sys

response_path, package, destination = map(pathlib.Path, sys.argv[1:])
response = json.loads(response_path.read_text(encoding="utf-8"))
digest = hashlib.sha256()
with package.open("rb") as stream:
    for chunk in iter(lambda: stream.read(1024 * 1024), b""):
        digest.update(chunk)
evidence = {
    "id": response["id"],
    "package": package.name,
    "sha256": digest.hexdigest(),
    "size_bytes": package.stat().st_size,
    "status": response["status"],
}
destination.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
echo "PKG aceptado por Apple y ticket notarial grapado."
