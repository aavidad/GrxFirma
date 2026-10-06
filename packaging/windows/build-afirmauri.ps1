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
Assert-GrxFirmaWindowsFyneToolchain -Architecture $Arquitectura
$Salida = Join-Path $Raiz "release/windows-afirmauri"
$Escenario = Join-Path $Salida "GrxFirma-$Version-afirmauri-windows-$Arquitectura"
$Zip = Join-Path $Salida "GrxFirma-$Version-afirmauri-windows-$Arquitectura.zip"
$Setup = Join-Path $Salida "GrxFirma-$Version-afirmauri-windows-$Arquitectura-setup.exe"
$AfirmaUriExe = Join-Path $Escenario "grxfirma-afirmauri.exe"
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

Write-Host "Compilando handler afirma:// de Windows ($Arquitectura)..."
$env:GOOS = "windows"
$env:GOARCH = $Arquitectura
$cgoAnteriorDefinido = Test-Path Env:CGO_ENABLED
$cgoAnterior = $env:CGO_ENABLED
try {
    # El handler de navegador debe conservar selector y confirmacion graficos.
    # Compilarlo sin fyne_gui activa aprobacion headless y no es un artefacto
    # distribuible.
    $env:CGO_ENABLED = "1"
    Invoke-GrxFirmaGoBuild `
        -Version $Version `
        -WorkingDirectory $Raiz `
        -LinkerFlags @("-H=windowsgui") `
        -BuildArguments @(
            "-tags",
            "production,fyne_gui",
            "-o",
            $AfirmaUriExe,
            "./cmd/grxfirmauri"
        )
} finally {
    if ($cgoAnteriorDefinido) {
        $env:CGO_ENABLED = $cgoAnterior
    } else {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion de AfirmaURI fallo con codigo $LASTEXITCODE"
}
Assert-GrxFirmaWindowsFyneArtifact `
    -Path $AfirmaUriExe `
    -Architecture $Arquitectura

Copy-Item (Join-Path $Raiz "packaging/windows/README_AFIRMAURI_WINDOWS.md") (Join-Path $Escenario "README_AFIRMAURI_WINDOWS.md") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/grxfirma-grx.ico") (Join-Path $Escenario "grxfirma-grx.ico") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-afirmauri.ps1") (Join-Path $Escenario "install-afirmauri.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-afirmauri.ps1") (Join-Path $Escenario "uninstall-afirmauri.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/afirmauri-registration.ps1") (Join-Path $Escenario "afirmauri-registration.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-path-safety.ps1") (Join-Path $Escenario "install-path-safety.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/invoke-uninstall-silent.ps1") (Join-Path $Escenario "invoke-uninstall-silent.ps1") -Force
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
        (Join-Path $Raiz "packaging/windows/grxfirma-afirmauri.nsi")
    if ($LASTEXITCODE -ne 0) {
        throw "La construccion NSIS de AfirmaURI fallo con codigo $LASTEXITCODE"
    }
    Set-GrxFirmaFileTimestamp -Path $Setup -SourceDateEpoch $SourceDateEpoch
    Write-Host "Instalador NSIS generado en: $Setup"
}
