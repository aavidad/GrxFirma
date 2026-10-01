# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [ValidateSet("0", "1")]
    [string]$KeepQt = "0",
    [ValidateSet("0", "1")]
    [string]$KeepWinUi = "0",
    [string]$BaseInstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma"
)

$ErrorActionPreference = "Stop"

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $baseDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al mantenedor."
}
. $pathSafety

$BaseInstallDir = Resolve-GrxFirmaBaseInstallPath -Path $BaseInstallDir
$plans = @(
    [pscustomobject]@{
        Keep = $KeepQt
        Component = "DesktopQML"
        InstallDir = Join-Path $BaseInstallDir "DesktopQML"
        Uninstaller = Join-Path $baseDir "uninstall-desktop-qml.ps1"
    },
    [pscustomobject]@{
        Keep = $KeepWinUi
        Component = "DesktopWinUI"
        InstallDir = Join-Path $BaseInstallDir "DesktopWinUI"
        Uninstaller = Join-Path $baseDir "uninstall-desktop-winui.ps1"
    }
)

foreach ($plan in $plans) {
    if ($plan.Keep -eq "1" -or
        (-not (Test-GrxFirmaSuiteOwnershipMarker `
            -Path $plan.InstallDir `
            -Component $plan.Component))) {
        continue
    }
    if (-not (Test-Path -LiteralPath $plan.Uninstaller -PathType Leaf)) {
        throw "Falta el desinstalador del componente $($plan.Component): $($plan.Uninstaller)"
    }
    & $plan.Uninstaller -InstallDir $plan.InstallDir
}
