# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

#
# Empaqueta la suite Windows ya construida (release/windows-suite/...) como
# MSIX para la Microsoft Store usando makeappx (Windows SDK).
#
# Requiere haber ejecutado antes:
#   ./packaging/windows/build-desktop-qml.ps1
#   ./packaging/windows/build-suite.ps1
#
# Variables (con valores por defecto SOLO válidos para pruebas locales;
# para la Store deben ser los del Partner Center):
#   MSIX_IDENTITY_NAME       p. ej. DiputacionGranada.GrxFirma
#   MSIX_PUBLISHER           p. ej. CN=XXXXXXXX-XXXX-... (el del Partner Center)
#   MSIX_PUBLISHER_DISPLAY   p. ej. Diputación de Granada

$ErrorActionPreference = "Stop"

$Raiz = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
. (Join-Path $PSScriptRoot "reproducible-build.ps1")
$SourceDateEpoch = Initialize-GrxFirmaReproducibleBuild -RootDirectory $Raiz
$Arch = if ($env:GOARCH) { $env:GOARCH } else { "amd64" }
$OutDir = Join-Path $Raiz "release/windows-msix"
$Layout = Join-Path $OutDir "layout"
$Plantilla = Join-Path $Raiz "packaging/windows/msix/AppxManifest.xml.in"
$AssetsDir = Join-Path $Raiz "packaging/windows/msix/Assets"

$IdentityName = if ($env:MSIX_IDENTITY_NAME) { $env:MSIX_IDENTITY_NAME } else { "DiputacionGranada.GrxFirma" }
$Publisher = if ($env:MSIX_PUBLISHER) { $env:MSIX_PUBLISHER } else { "CN=Diputacion de Granada" }
$PublisherDisplay = if ($env:MSIX_PUBLISHER_DISPLAY) { $env:MSIX_PUBLISHER_DISPLAY } else { "Diputación de Granada" }

# Versión MSIX: cuatro componentes numéricos. La Store exige que el último sea 0.
$versionFile = Join-Path $Raiz "VERSION.txt"
$rawVersion = if (Test-Path $versionFile) { (Get-Content $versionFile -Raw).Trim() } else { "0.0.1" }
$rawVersion = $rawVersion.TrimStart("v", "V")
$parts = ($rawVersion -split "[.-]") | Where-Object { $_ -match '^\d+$' }
while ($parts.Count -lt 3) { $parts += "0" }
$MsixVersion = "{0}.{1}.{2}.0" -f $parts[0], $parts[1], $parts[2]
$Stage = Join-Path $Raiz "release/windows-suite/GrxFirma-$rawVersion-windows-$Arch"

function Assert-MsixStage {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDir
    )

    if (-not (Test-Path -LiteralPath $StageDir -PathType Container)) {
        throw "No existe la stage de la suite: $StageDir. Ejecuta antes build-suite.ps1."
    }
    if (-not (Test-Path -LiteralPath (Join-Path $StageDir "grxfirma-gui-qml.exe") -PathType Leaf)) {
        throw "La stage no incluye el frontend Qt (grxfirma-gui-qml.exe); la app de la Store lo necesita. Ejecuta antes build-desktop-qml.ps1."
    }
    if (-not (Test-Path -LiteralPath (Join-Path $StageDir "grxfirma-gui.exe") -PathType Leaf)) {
        throw "La stage no incluye el backend IPC (grxfirma-gui.exe); la app de la Store no puede funcionar sin el. Ejecuta de nuevo build-desktop-qml.ps1 y build-suite.ps1."
    }
}
Assert-MsixStage -StageDir $Stage

function Find-MakeAppx {
    $cmd = Get-Command makeappx -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $kits = "C:\Program Files (x86)\Windows Kits\10\bin"
    if (Test-Path $kits) {
        $candidates = Get-ChildItem $kits -Directory |
            Where-Object { $_.Name -match '^10\.' } |
            Sort-Object Name -Descending |
            ForEach-Object { Join-Path $_.FullName "x64\makeappx.exe" } |
            Where-Object { Test-Path $_ }
        if ($candidates) { return $candidates[0] }
    }
    throw "No se encontró makeappx.exe (Windows SDK)."
}

$MakeAppx = Find-MakeAppx
Write-Host "makeappx: $MakeAppx"
Write-Host "Identity: $IdentityName / $Publisher / v$MsixVersion"

# Layout: payload de la suite + Assets + manifest sustituido.
if (Test-Path $Layout) { Remove-Item $Layout -Recurse -Force }
New-Item -ItemType Directory -Path $Layout | Out-Null
Copy-Item -Path (Join-Path $Stage "*") -Destination $Layout -Recurse
# "MsixAssets" y no "Assets": la stage ya trae un directorio "assets" (QML)
# y el sistema de ficheros de Windows no distingue mayúsculas.
New-Item -ItemType Directory -Force -Path (Join-Path $Layout "MsixAssets") | Out-Null
Copy-Item -Path (Join-Path $AssetsDir "*") -Destination (Join-Path $Layout "MsixAssets")

$manifest = Get-Content $Plantilla -Raw
$manifest = $manifest.Replace("@IDENTITY_NAME@", $IdentityName)
$manifest = $manifest.Replace("@PUBLISHER@", $Publisher)
$manifest = $manifest.Replace("@PUBLISHER_DISPLAY@", $PublisherDisplay)
$manifest = $manifest.Replace("@VERSION@", $MsixVersion)
Set-Content -Path (Join-Path $Layout "AppxManifest.xml") -Value $manifest -Encoding UTF8
Set-GrxFirmaTreeTimestamp -Path $Layout -SourceDateEpoch $SourceDateEpoch

$MsixPath = Join-Path $OutDir "GrxFirma-$rawVersion-windows-$Arch.msix"
if (Test-Path $MsixPath) { Remove-Item $MsixPath -Force }

& $MakeAppx pack /d $Layout /p $MsixPath /o
if ($LASTEXITCODE -ne 0) { throw "makeappx pack falló con código $LASTEXITCODE" }
if (-not (Test-Path $MsixPath) -or (Get-Item $MsixPath).Length -eq 0) {
    throw "El MSIX no se generó correctamente: $MsixPath"
}
Set-GrxFirmaFileTimestamp -Path $MsixPath -SourceDateEpoch $SourceDateEpoch

# Checksums e inventario.
$hash = (Get-FileHash -Algorithm SHA256 -Path $MsixPath).Hash.ToLowerInvariant()
Set-Content -Path (Join-Path $OutDir "SHA256SUMS.txt") -Value ("{0} *{1}" -f $hash, (Split-Path -Leaf $MsixPath)) -Encoding ASCII
Write-Host "MSIX generado: $MsixPath"
Write-Host "SHA-256: $hash"

# El layout intermedio no forma parte del artefacto.
Remove-Item $Layout -Recurse -Force
