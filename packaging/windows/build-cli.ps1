# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

$Raiz = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
. (Join-Path $PSScriptRoot "reproducible-build.ps1")
$SourceDateEpoch = Initialize-GrxFirmaReproducibleBuild -RootDirectory $Raiz
$versionFile = Join-Path $Raiz "VERSION.txt"
if (Test-Path $versionFile) {
    $Version = (Get-Content -Path $versionFile -Raw).Trim()
} else {
    $Version = (git -C $Raiz describe --tags --always --dirty 2>$null)
    if ([string]::IsNullOrWhiteSpace($Version)) {
        $Version = "dev"
    }
}

$Arquitectura = if ($env:GOARCH) { $env:GOARCH } else { "amd64" }
$Salida = Join-Path $Raiz "release/windows-cli"
$Escenario = Join-Path $Salida "GrxFirma-$Version-cli-windows-$Arquitectura"
$Zip = Join-Path $Salida "GrxFirma-$Version-cli-windows-$Arquitectura.zip"
$Setup = Join-Path $Salida "GrxFirma-$Version-cli-windows-$Arquitectura-setup.exe"
$MakeNsis = if ($args -contains "--nsis") {
    Resolve-GrxFirmaMakeNsis
} else {
    $null
}

if (Test-Path $Escenario) {
    Remove-Item $Escenario -Recurse -Force
}
if (Test-Path $Setup) {
    Remove-Item $Setup -Force
}
New-Item -ItemType Directory -Force -Path $Escenario | Out-Null

Write-Host "Compilando CLI de Windows ($Arquitectura)..."
$env:GOOS = "windows"
$env:GOARCH = $Arquitectura
Invoke-GrxFirmaGoBuild `
    -Version $Version `
    -WorkingDirectory $Raiz `
    -BuildArguments @("-tags", "production", "-o", (Join-Path $Escenario "grxfirma.exe"), "./cmd/grxfirma")
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion de la CLI fallo con codigo $LASTEXITCODE"
}

Copy-Item (Join-Path $Raiz "packaging/windows/README_CLI_WINDOWS.md") (Join-Path $Escenario "README_CLI_WINDOWS.md") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/grxfirma.ico") (Join-Path $Escenario "grxfirma.ico") -Force
Set-Content -Path (Join-Path $Escenario "VERSION.txt") -Value $Version -Encoding UTF8

if (Test-Path $Zip) {
    Remove-Item $Zip -Force
}
New-GrxFirmaReproducibleZip `
    -SourceDirectory $Escenario `
    -DestinationPath $Zip `
    -SourceDateEpoch $SourceDateEpoch
Write-Host "ZIP generado en: $Zip"

if ($args -contains "--nsis") {
    Write-Host "Compilando instalador NSIS..."
    & $MakeNsis `
        /DVERSION=$Version `
        /DARCH=$Arquitectura `
        /DSTAGE_DIR=$Escenario `
        /DOUT_FILE=$Setup `
        (Join-Path $Raiz "packaging/windows/grxfirma-cli.nsi")
    if ($LASTEXITCODE -ne 0) {
        throw "La construccion NSIS de la CLI fallo con codigo $LASTEXITCODE"
    }
    Set-GrxFirmaFileTimestamp -Path $Setup -SourceDateEpoch $SourceDateEpoch
    Write-Host "Instalador NSIS generado en: $Setup"
}
