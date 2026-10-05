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
$Salida = Join-Path $Raiz "release/windows-nativehost"
$Escenario = Join-Path $Salida "GrxFirma-$Version-nativehost-windows-$Arquitectura"
$Zip = Join-Path $Salida "GrxFirma-$Version-nativehost-windows-$Arquitectura.zip"
$DefaultExtensionKey = Join-Path $env:USERPROFILE ".local\share\grxfirma\build-keys\chromium-extension.pem"

function Get-PackBrowser {
    $candidates = @(
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
        "$env:ProgramFiles\BraveSoftware\Brave-Browser\Application\brave.exe",
        "${env:ProgramFiles(x86)}\BraveSoftware\Brave-Browser\Application\brave.exe"
    )
    foreach ($candidate in $candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path $candidate)) {
            return $candidate
        }
    }
    return $null
}

function Get-OrCreateChromiumExtensionKey {
    if (-not [string]::IsNullOrWhiteSpace($env:GRXFIRMA_CHROMIUM_EXTENSION_KEY) -and (Test-Path $env:GRXFIRMA_CHROMIUM_EXTENSION_KEY)) {
        return $env:GRXFIRMA_CHROMIUM_EXTENSION_KEY
    }
    $keyDir = Split-Path -Parent $DefaultExtensionKey
    New-Item -ItemType Directory -Force -Path $keyDir | Out-Null
    if (-not (Test-Path $DefaultExtensionKey)) {
        & openssl genrsa -out $DefaultExtensionKey 2048 | Out-Null
    }
    if (Test-Path $DefaultExtensionKey) {
        return $DefaultExtensionKey
    }
    return $null
}

function Build-ChromiumExtensionAsset {
    param(
        [string]$ZipSource,
        [string]$OutputDir
    )

    if (-not (Test-Path $ZipSource)) {
        return
    }
    $buildCrx = if ([string]::IsNullOrWhiteSpace($env:GRXFIRMA_BUILD_CHROMIUM_CRX)) {
        "0"
    } else {
        $env:GRXFIRMA_BUILD_CHROMIUM_CRX
    }
    if ($buildCrx -eq "0") {
        return
    }
    if ($buildCrx -ne "1") {
        throw "GRXFIRMA_BUILD_CHROMIUM_CRX debe valer 0 o 1."
    }

    $packBrowser = Get-PackBrowser
    if ([string]::IsNullOrWhiteSpace($packBrowser)) {
        Write-Warning "No hay navegador Chromium disponible para empaquetar la extension."
        return
    }

    $tmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-ext-" + [System.Guid]::NewGuid().ToString("N"))
    $extractDir = Join-Path $tmpRoot "ext"
    New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
    Expand-Archive -Path $ZipSource -DestinationPath $extractDir -Force

    $extensionDir = $extractDir
    if (Test-Path (Join-Path $extractDir "chromium")) {
        $extensionDir = Join-Path $extractDir "chromium"
    }

    $keyPath = Get-OrCreateChromiumExtensionKey
    $packArgs = @("--pack-extension=$extensionDir")
    if (-not [string]::IsNullOrWhiteSpace($keyPath)) {
        $packArgs += "--pack-extension-key=$keyPath"
    }
    & $packBrowser $packArgs | Out-Null

    $crxPath = @(
        (Join-Path $extractDir ((Split-Path $extensionDir -Leaf) + ".crx")),
        ($extractDir + ".crx"),
        ($extensionDir + ".crx")
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1
    $pemPath = @(
        (Join-Path $extractDir ((Split-Path $extensionDir -Leaf) + ".pem")),
        ($extractDir + ".pem"),
        ($extensionDir + ".pem")
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1
    if (-not [string]::IsNullOrWhiteSpace($keyPath) -and (Test-Path $keyPath)) {
        $pemPath = $keyPath
    }
    if ([string]::IsNullOrWhiteSpace($crxPath) -or (-not (Test-Path $crxPath)) -or
        [string]::IsNullOrWhiteSpace($pemPath) -or (-not (Test-Path $pemPath))) {
        Write-Warning "No se pudo generar el CRX Chromium empaquetado."
        return
    }

    $extensionIdScript = @'
import hashlib
import subprocess
import sys
pub_der = subprocess.check_output(["openssl", "pkey", "-in", sys.argv[1], "-pubout", "-outform", "DER"])
hex_digest = hashlib.sha256(pub_der).hexdigest()[:32]
print(hex_digest.translate(str.maketrans("0123456789abcdef", "abcdefghijklmnop")))
'@
    $extensionId = (& python -c $extensionIdScript $pemPath).Trim()
    $versionScript = @'
import json
import sys
import zipfile
with zipfile.ZipFile(sys.argv[1]) as zf:
    for name in zf.namelist():
        if name.endswith("manifest.json"):
            print(json.loads(zf.read(name)).get("version", "1.0.0"))
            break
    else:
        print("1.0.0")
'@
    $version = (& python -c $versionScript $ZipSource).Trim()

    Copy-Item $crxPath (Join-Path $OutputDir "grxfirma-extension-chromium.crx") -Force
    Set-Content -Path (Join-Path $OutputDir "grxfirma-extension-chromium.id") -Value $extensionId -Encoding ASCII
    Set-Content -Path (Join-Path $OutputDir "grxfirma-extension-chromium.version") -Value $version -Encoding ASCII
    Remove-Item $tmpRoot -Recurse -Force
}

if (Test-Path $Escenario) {
    Remove-Item $Escenario -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $Escenario | Out-Null

Write-Output "Compilando NativeHost de Windows ($Arquitectura)..."
$env:GOOS = "windows"
$env:GOARCH = $Arquitectura
Invoke-GrxFirmaGoBuild `
    -Version $Version `
    -WorkingDirectory $Raiz `
    -BuildArguments @("-tags", "production", "-o", (Join-Path $Escenario "grxfirma-nativehost.exe"), "./cmd/nativehost")
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion del NativeHost fallo con codigo $LASTEXITCODE"
}

Copy-Item (Join-Path $Raiz "packaging/windows/README_NATIVEHOST_WINDOWS.md") (Join-Path $Escenario "README_NATIVEHOST_WINDOWS.md") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-nativehost.ps1") (Join-Path $Escenario "install-nativehost.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-nativehost.ps1") (Join-Path $Escenario "uninstall-nativehost.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-path-safety.ps1") (Join-Path $Escenario "install-path-safety.ps1") -Force
New-Item -ItemType Directory -Force -Path (Join-Path $Escenario "extensions") | Out-Null
& (Join-Path $Raiz "packaging/browser-extensions/build.ps1")
foreach ($extensionAsset in @(
    "grxfirma-extension-chromium.zip",
    "grxfirma-extension-firefox.xpi",
    "grxfirma-extension-firefox.metadata.json"
)) {
    $extensionSource = Join-Path $Raiz "packaging/browser-extensions/$extensionAsset"
    if (-not (Test-Path -LiteralPath $extensionSource -PathType Leaf)) {
        throw "Falta el artefacto de navegador aprobado: $extensionSource"
    }
    Copy-Item -LiteralPath $extensionSource -Destination (Join-Path $Escenario "extensions") -Force
}
Build-ChromiumExtensionAsset -ZipSource (Join-Path $Raiz "packaging/browser-extensions/grxfirma-extension-chromium.zip") -OutputDir (Join-Path $Escenario "extensions")
Set-Content -Path (Join-Path $Escenario "VERSION.txt") -Value $Version -Encoding UTF8

if (Test-Path $Zip) {
    Remove-Item $Zip -Force
}
New-GrxFirmaReproducibleZip `
    -SourceDirectory $Escenario `
    -DestinationPath $Zip `
    -SourceDateEpoch $SourceDateEpoch
$validateZipScript = @'
import sys
import zipfile

zip_path = sys.argv[1]
stage_name = sys.argv[2]
required = {
    f"{stage_name}/grxfirma-nativehost.exe",
    f"{stage_name}/README_NATIVEHOST_WINDOWS.md",
    f"{stage_name}/install-nativehost.ps1",
    f"{stage_name}/uninstall-nativehost.ps1",
    f"{stage_name}/install-path-safety.ps1",
    f"{stage_name}/VERSION.txt",
    f"{stage_name}/extensions/grxfirma-extension-chromium.zip",
    f"{stage_name}/extensions/grxfirma-extension-firefox.xpi",
    f"{stage_name}/extensions/grxfirma-extension-firefox.metadata.json",
}
with zipfile.ZipFile(zip_path) as archive:
    names = set(archive.namelist())
missing = sorted(required - names)
if missing:
    raise SystemExit("zip artifact incomplete:\n  - " + "\n  - ".join(missing))
'@
& python -c $validateZipScript $Zip (Split-Path $Escenario -Leaf)
if ($LASTEXITCODE -ne 0) {
    throw "La validacion del ZIP NativeHost fallo con codigo $LASTEXITCODE"
}
Write-Output "ZIP generado en: $Zip"
