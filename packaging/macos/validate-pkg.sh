#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

if [[ "$#" -ne 1 ]]; then
  echo "uso: validate-pkg.sh PAQUETE.pkg" >&2
  exit 2
fi

pkg_path="$1"
if [[ ! -f "${pkg_path}" || ! -s "${pkg_path}" ]]; then
  echo "error: el PKG no existe o esta vacio: ${pkg_path}" >&2
  exit 1
fi
if ! command -v pkgutil >/dev/null 2>&1; then
  echo "error: pkgutil no esta disponible para validar ${pkg_path}" >&2
  exit 1
fi

# pkgutil puede devolver las rutas con o sin el prefijo './' segun la version
# de macOS. Se normaliza la representacion, no el contenido del paquete.
listing="$(
  pkgutil --payload-files "${pkg_path}" |
    sed -e 's#^\./##' -e 's#/$##' -e '/^\.$/d'
)"

required=(
  "Applications/GrxFirma AfirmaURI.app/Contents/Info.plist"
  "Applications/GrxFirma AfirmaURI.app/Contents/MacOS/grxfirma-afirmauri"
  "Applications/GrxFirma Desktop Qt.app/Contents/Info.plist"
  "Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml"
  "Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui"
  "Applications/GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma"
  "Applications/GrxFirma Desktop Qt.app/Contents/Resources/VERSION.txt"
  "Library/Application Support/GrxFirma/grxfirma"
  "Library/Application Support/GrxFirma/grxfirma-nativehost"
  "Library/Application Support/GrxFirma/install-nativehost.sh"
  "Library/Application Support/GrxFirma/uninstall-suite.sh"
  "Library/Application Support/GrxFirma/register-user.sh"
  "Library/Application Support/GrxFirma/extensions/grxfirma-extension-firefox.metadata.json"
  "Library/LaunchAgents/io.github.aavidad.grxfirma.register-user.plist"
  "usr/local/bin/grxfirma"
)

for entry in "${required[@]}"; do
  if ! grep -Fqx -- "${entry}" <<<"${listing}"; then
    echo "error: PKG incompleto, falta ${entry}" >&2
    exit 1
  fi
done

# G-05: un bundle reubicable puede acabar instalado (como root) sobre otra copia
# con el mismo identificador en lugar de en /Applications.
expand_dir="$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-validate-pkg.XXXXXX")"
trap 'rm -rf "${expand_dir}"' EXIT
pkgutil --expand "${pkg_path}" "${expand_dir}/pkg"
package_info="${expand_dir}/pkg/PackageInfo"
if [[ ! -f "${package_info}" ]]; then
  echo "error: el PKG no contiene PackageInfo: ${pkg_path}" >&2
  exit 1
fi
if grep -q '<relocate' "${package_info}"; then
  echo "error: el PKG declara bundles reubicables (<relocate>); deben instalarse siempre en /Applications" >&2
  exit 1
fi

if [[ -n "${MACOS_INSTALLER_IDENTITY:-}" ]]; then
  pkgutil --check-signature "${pkg_path}" >/dev/null
fi

echo "PKG macOS validado: ${pkg_path}"
