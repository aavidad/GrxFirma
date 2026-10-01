# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

$SelfDir = Split-Path -Parent $MyInvocation.MyCommand.Path
& python (Join-Path $SelfDir "build.py")
if ($LASTEXITCODE -ne 0) {
    throw "La construccion de extensiones fallo con codigo $LASTEXITCODE"
}
