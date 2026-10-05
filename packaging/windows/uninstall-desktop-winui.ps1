# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\DesktopWinUI"
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $scriptDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al desinstalador."
}
. $pathSafety

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "DesktopWinUI"
# La preferencia de arranque pertenece solo a este usuario y a esta interfaz.
$runKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
if (Test-Path -Path $runKey) {
    $run = Get-Item -Path $runKey -ErrorAction Stop
    if ($run.GetValueNames() -contains "GrxFirma") {
        Remove-ItemProperty -Path $runKey -Name "GrxFirma" -ErrorAction Stop
    }
}
$programsDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
$startMenuDir = Join-Path $programsDir "GrxFirma"
$shortcutPath = Join-Path $startMenuDir "GrxFirma - Windows nativo.lnk"
$legacyStartMenuDir = Join-Path $programsDir "Diputación de Granada"
$legacyShortcutPath = Join-Path $legacyStartMenuDir "GrxFirma - Windows nativo.lnk"

$expectedFrontend = [System.IO.Path]::GetFullPath(
    (Join-Path $InstallDir "grxfirma-winui.exe")
)
foreach ($process in Get-Process -Name "grxfirma-winui" -ErrorAction SilentlyContinue) {
    $processPath = Get-GrxFirmaProcessPath -Process $process
    if (-not $processPath) {
        continue
    }
    if ([string]::Equals(
        $processPath,
        $expectedFrontend,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        Stop-Process -Id $process.Id -Force -ErrorAction Stop
    }
}

if (Test-Path -LiteralPath $shortcutPath -PathType Leaf) {
    Remove-Item -LiteralPath $shortcutPath -Force
}
if (Test-Path -LiteralPath $legacyShortcutPath -PathType Leaf) {
    Remove-Item -LiteralPath $legacyShortcutPath -Force
}
if (Test-Path -LiteralPath $startMenuDir -PathType Container) {
    $remaining = @(Get-ChildItem -LiteralPath $startMenuDir -Force -ErrorAction SilentlyContinue)
    if ($remaining.Count -eq 0) {
        Remove-Item -LiteralPath $startMenuDir -Force
    }
}
if (Test-Path -LiteralPath $legacyStartMenuDir -PathType Container) {
    $legacyRemaining = @(
        Get-ChildItem -LiteralPath $legacyStartMenuDir -Force -ErrorAction SilentlyContinue
    )
    if ($legacyRemaining.Count -eq 0) {
        Remove-Item -LiteralPath $legacyStartMenuDir -Force
    }
}
Remove-GrxFirmaInstallDirectory `
    -Path $InstallDir `
    -Component "DesktopWinUI" `
    -LegacyPayload "grxfirma-winui.exe"

Write-Host "Desktop WinUI desinstalado de: $InstallDir"
