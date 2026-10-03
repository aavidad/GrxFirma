# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$BaseInstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma",
    [switch]$Silent
)

$ErrorActionPreference = "Stop"

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $baseDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al desinstalador."
}
. $pathSafety

$BaseInstallDir = Resolve-GrxFirmaBaseInstallPath -Path $BaseInstallDir
$runKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
if (Test-Path -Path $runKey) {
    $run = Get-Item -Path $runKey -ErrorAction Stop
    if ($run.GetValueNames() -contains "GrxFirma") {
        Remove-ItemProperty -Path $runKey -Name "GrxFirma" -ErrorAction Stop
    }
}
$cliDir = Join-Path $BaseInstallDir "CLI"
$nativeDir = Join-Path $BaseInstallDir "NativeHost"
$afirmaDir = Join-Path $BaseInstallDir "AfirmaURI"
$launcherDir = Join-Path $BaseInstallDir "DesktopLauncher"
$desktopQtDir = Join-Path $BaseInstallDir "DesktopQML"
$desktopWinUiDir = Join-Path $BaseInstallDir "DesktopWinUI"

$nativeUninstaller = Join-Path $baseDir "uninstall-nativehost.ps1"
$afirmaUninstaller = Join-Path $baseDir "uninstall-afirmauri.ps1"
$desktopQtUninstaller = Join-Path $baseDir "uninstall-desktop-qml.ps1"
$desktopWinUiUninstaller = Join-Path $baseDir "uninstall-desktop-winui.ps1"
foreach ($required in @($nativeUninstaller, $afirmaUninstaller)) {
    if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
        throw "Falta un desinstalador requerido del paquete: $required"
    }
}
if ((Test-GrxFirmaSuiteOwnershipMarker -Path $desktopQtDir -Component "DesktopQML") -and
    (-not (Test-Path -LiteralPath $desktopQtUninstaller -PathType Leaf))) {
    throw "Falta el desinstalador Desktop Qt/QML: $desktopQtUninstaller"
}
if ((Test-GrxFirmaSuiteOwnershipMarker -Path $desktopWinUiDir -Component "DesktopWinUI") -and
    (-not (Test-Path -LiteralPath $desktopWinUiUninstaller -PathType Leaf))) {
    throw "Falta el desinstalador Desktop WinUI: $desktopWinUiUninstaller"
}

& $nativeUninstaller -InstallDir $nativeDir
& $afirmaUninstaller -InstallDir $afirmaDir -Silent:$Silent
if (Test-GrxFirmaSuiteOwnershipMarker -Path $desktopQtDir -Component "DesktopQML") {
    & $desktopQtUninstaller -InstallDir $desktopQtDir
}
if (Test-GrxFirmaSuiteOwnershipMarker -Path $desktopWinUiDir -Component "DesktopWinUI") {
    & $desktopWinUiUninstaller -InstallDir $desktopWinUiDir
}
if (Test-GrxFirmaSuiteOwnershipMarker -Path $launcherDir -Component "DesktopLauncher") {
    $expectedLauncher = [System.IO.Path]::GetFullPath(
        (Join-Path $launcherDir "grxfirma-gui.exe")
    )
    foreach ($process in Get-Process -Name "grxfirma-gui" -ErrorAction SilentlyContinue) {
        $processPath = Get-GrxFirmaProcessPath -Process $process
        if (-not $processPath) {
            continue
        }
        if ([string]::Equals(
            $processPath,
            $expectedLauncher,
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
            Stop-Process -Id $process.Id -Force -ErrorAction Stop
        }
    }
    Remove-GrxFirmaInstallDirectory `
        -Path $launcherDir `
        -Component "DesktopLauncher" `
        -LegacyPayload "grxfirma-gui.exe"
}

Remove-GrxFirmaInstallDirectory `
    -Path $cliDir `
    -Component "CLI" `
    -LegacyPayload "grxfirma.exe"

if (Test-Path -LiteralPath $BaseInstallDir) {
    $remaining = @(Get-ChildItem -LiteralPath $BaseInstallDir -Force -ErrorAction Stop)
    if ($remaining.Count -eq 0) {
        Remove-Item -LiteralPath $BaseInstallDir -Force
    }
}

Write-Host "Suite desinstalada de: $BaseInstallDir"
