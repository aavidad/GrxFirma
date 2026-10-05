# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\DesktopWinUI",
    [string]$PackageDir = "",
    [string]$LauncherPath = "$env:LOCALAPPDATA\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe",
    [switch]$ValidateOnly,
    [switch]$ManagedBySuite
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $scriptDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al instalador."
}
. $pathSafety

if ([string]::IsNullOrWhiteSpace($PackageDir)) {
    $PackageDir = $scriptDir
}
$PackageDir = [System.IO.Path]::GetFullPath($PackageDir)
Assert-NoGrxFirmaReparsePoint -Path $PackageDir

$appSource = Join-Path $PackageDir "app"
$frontendSource = Join-Path $appSource "grxfirma-winui.exe"
$packagedBackend = Join-Path $appSource "grxfirma-gui.exe"
$manifestPath = Join-Path $PackageDir "PUBLISH-MANIFEST.sha256"
foreach ($required in @(
    $appSource,
    $frontendSource,
    $packagedBackend,
    $manifestPath,
    (Join-Path $appSource "coreclr.dll"),
    (Join-Path $appSource "hostfxr.dll"),
    (Join-Path $appSource "Microsoft.UI.Xaml.dll"),
    (Join-Path $appSource "Microsoft.WindowsAppRuntime.dll")
)) {
    if (-not (Test-Path -LiteralPath $required)) {
        throw "El paquete WinUI esta incompleto: $required"
    }
}

$manifestEntries = @{}
foreach ($line in Get-Content -LiteralPath $manifestPath -ErrorAction Stop) {
    if ($line -notmatch '^([0-9a-fA-F]{64}) \*(.+)$') {
        throw "PUBLISH-MANIFEST.sha256 contiene una linea no valida."
    }
    $relativePath = $Matches[2].Replace('\', '/')
    if ([System.IO.Path]::IsPathRooted($relativePath) -or
        @($relativePath -split '[/\\]') -contains "..") {
        throw "El manifiesto WinUI contiene una ruta no permitida: $relativePath"
    }
    $manifestEntries[$relativePath] = $Matches[1].ToLowerInvariant()
}
foreach ($file in Get-ChildItem -LiteralPath $PackageDir -File -Force -Recurse) {
    if ($file.FullName -eq $manifestPath) {
        continue
    }
    $relativePath = $file.FullName.Substring($PackageDir.Length).TrimStart('\', '/').Replace('\', '/')
    if (-not $manifestEntries.ContainsKey($relativePath)) {
        throw "El paquete WinUI contiene un fichero no inventariado: $relativePath"
    }
    $actualHash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $manifestEntries[$relativePath]) {
        throw "Checksum inesperado en el paquete WinUI: $relativePath"
    }
}
if ($manifestEntries.Count -ne (@(
    Get-ChildItem -LiteralPath $PackageDir -File -Force -Recurse |
        Where-Object { $_.FullName -ne $manifestPath }
)).Count) {
    throw "El manifiesto WinUI referencia ficheros ausentes."
}

$LauncherPath = [System.IO.Path]::GetFullPath($LauncherPath)
$expectedLauncherDir = Resolve-GrxFirmaInstallPath `
    -Path (Split-Path -Parent $LauncherPath) `
    -Component "DesktopLauncher"
if (-not [string]::Equals(
    $LauncherPath,
    (Join-Path $expectedLauncherDir "grxfirma-gui.exe"),
    [System.StringComparison]::OrdinalIgnoreCase
)) {
    throw "Ruta del lanzador compartido no permitida: $LauncherPath"
}
if (-not (Test-Path -LiteralPath $LauncherPath -PathType Leaf)) {
    throw "No se encuentra el lanzador compartido de escritorio: $LauncherPath"
}
if ((Get-FileHash -LiteralPath $LauncherPath -Algorithm SHA256).Hash -ne
    (Get-FileHash -LiteralPath $packagedBackend -Algorithm SHA256).Hash) {
    throw "El backend WinUI no coincide con el lanzador compartido instalado."
}

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "DesktopWinUI"
if ($ValidateOnly) {
    Write-Host "Paquete Desktop WinUI validado para: $InstallDir"
    return
}

Stop-GrxFirmaInstalledProcesses -Path $InstallDir -Component "DesktopWinUI"

$InstallDir = Clear-GrxFirmaInstallDirectory `
    -Path $InstallDir `
    -Component "DesktopWinUI" `
    -LegacyPayload "grxfirma-winui.exe"
if ($ManagedBySuite) {
    # Se marca antes de copiar para que un fallo parcial siga siendo
    # recuperable mediante el desinstalador de la suite.
    Set-GrxFirmaSuiteOwnershipMarker -Path $InstallDir -Component "DesktopWinUI"
}
foreach ($item in Get-ChildItem -LiteralPath $appSource -Force) {
    if ($item.Name -ieq "grxfirma-gui.exe") {
        continue
    }
    Copy-Item `
        -LiteralPath $item.FullName `
        -Destination (Join-Path $InstallDir $item.Name) `
        -Recurse `
        -Force
}
$programsDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
$startMenuDir = Join-Path $programsDir "Diputación de Granada"
New-Item -ItemType Directory -Force -Path $startMenuDir | Out-Null
$legacyShortcut = Join-Path $programsDir "GrxFirma\GrxFirma - Windows nativo.lnk"
if (Test-Path -LiteralPath $legacyShortcut -PathType Leaf) {
    Remove-Item -LiteralPath $legacyShortcut -Force
}
Remove-Item -LiteralPath (Join-Path $startMenuDir "GrxFirma - Windows nativo.lnk") -Force -ErrorAction SilentlyContinue
$shortcutPath = Join-Path $startMenuDir "GrxFirma - Windows nativo.lnk"
$wshell = New-Object -ComObject WScript.Shell
$shortcut = $wshell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $LauncherPath
$shortcut.Arguments = '--frontend=winui --ui-binary="' +
    (Join-Path $InstallDir "grxfirma-winui.exe") + '"'
$shortcut.WorkingDirectory = $InstallDir
$shortcut.Description = "GrxFirma con interfaz nativa de Windows"
$shortcut.IconLocation = (Join-Path $InstallDir "Assets\grxfirma.ico") + ",0"
$shortcut.Save()

Write-Host "Desktop WinUI instalado en: $InstallDir"
Write-Host "Acceso directo creado en: $shortcutPath"
