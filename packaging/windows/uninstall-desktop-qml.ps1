# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\DesktopQML"
)

$ErrorActionPreference = "Stop"

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $baseDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al desinstalador."
}
. $pathSafety

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "DesktopQML"
$programsDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
$startMenuDir = Join-Path $programsDir "GrxFirma"
$legacyStartMenuDir = Join-Path $programsDir "Diputación de Granada"
$shortcutPaths = @(
    (Join-Path $startMenuDir "GrxFirma Desktop Qt.lnk"),
    (Join-Path $startMenuDir "GrxFirma - Qt.lnk"),
    (Join-Path $legacyStartMenuDir "GrxFirma Desktop Qt.lnk"),
    (Join-Path $legacyStartMenuDir "GrxFirma - Qt.lnk")
)

function Stop-InstalledDesktopProcesses {
    $expectedPaths = @(
        [System.IO.Path]::GetFullPath((Join-Path $InstallDir "grxfirma-gui-qml.exe")),
        [System.IO.Path]::GetFullPath((Join-Path $InstallDir "grxfirma-gui.exe")),
        [System.IO.Path]::GetFullPath((Join-Path $InstallDir "grxfirma.exe"))
    )
    foreach ($process in Get-Process -Name "grxfirma-gui-qml", "grxfirma-gui", "grxfirma" -ErrorAction SilentlyContinue) {
        $processPath = Get-GrxFirmaProcessPath -Process $process
        if (-not $processPath) {
            continue
        }
        if ($expectedPaths -contains $processPath) {
            Stop-Process -Id $process.Id -Force -ErrorAction Stop
        }
    }
}
Stop-InstalledDesktopProcesses

foreach ($shortcutPath in $shortcutPaths) {
    if (Test-Path -LiteralPath $shortcutPath -PathType Leaf) {
        Remove-Item -LiteralPath $shortcutPath -Force
    }
}
foreach ($menuDir in @($startMenuDir, $legacyStartMenuDir)) {
    if (Test-Path -LiteralPath $menuDir -PathType Container) {
        $remaining = @(Get-ChildItem -LiteralPath $menuDir -Force -ErrorAction SilentlyContinue)
        if ($remaining.Count -eq 0) {
            Remove-Item -LiteralPath $menuDir -Force
        }
    }
}
Remove-GrxFirmaInstallDirectory `
    -Path $InstallDir `
    -Component "DesktopQML" `
    -LegacyPayload "grxfirma-gui-qml.exe"

Write-Host "Desktop Qt/QML desinstalado de: $InstallDir"
