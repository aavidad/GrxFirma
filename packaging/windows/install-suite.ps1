# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$BaseInstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma",
    [switch]$CoreOnly,
    [switch]$SilentInstall,
    [switch]$RestoreTray
)

$ErrorActionPreference = "Stop"

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $baseDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 en el paquete."
}
. $pathSafety

$BaseInstallDir = Resolve-GrxFirmaBaseInstallPath -Path $BaseInstallDir
$restartKey = "HKCU:\Software\GrxFirma"
$restartName = "InstallerRestartFrontend"
$launcherDir = Join-Path $BaseInstallDir "DesktopLauncher"
$desktopQtDir = Join-Path $BaseInstallDir "DesktopQML"
$desktopWinUiDir = Join-Path $BaseInstallDir "DesktopWinUI"
function Test-GrxFirmaStartupEnabled {
    $runKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey("Software\Microsoft\Windows\CurrentVersion\Run")
    try {
        $registered = if ($runKey) { $runKey.GetValue("GrxFirma") } else { $null }
        $expectedRun = '"' + (Join-Path $launcherDir "grxfirma-gui.exe") + '" --frontend=winui --start-hidden'
        return [string]::Equals($registered, $expectedRun, [System.StringComparison]::OrdinalIgnoreCase)
    } finally {
        if ($runKey) { $runKey.Dispose() }
    }
}
if ($RestoreTray) {
    $restartFrontend = (Get-ItemProperty -Path $restartKey -Name $restartName -ErrorAction SilentlyContinue).$restartName
    Remove-ItemProperty -Path $restartKey -Name $restartName -ErrorAction SilentlyContinue
    if ($restartFrontend -eq "qt" -and
        -not (Test-Path -LiteralPath (Join-Path $desktopQtDir "grxfirma-gui-qml.exe") -PathType Leaf) -and
        (Test-GrxFirmaStartupEnabled)) {
        $restartFrontend = "winui"
    }
    if ($restartFrontend -in @("qt", "winui")) {
        $launcherDir = Resolve-GrxFirmaInstallPath -Path $launcherDir -Component "DesktopLauncher"
        $frontendDir = if ($restartFrontend -eq "qt") {
            Resolve-GrxFirmaInstallPath -Path $desktopQtDir -Component "DesktopQML"
        } else {
            Resolve-GrxFirmaInstallPath -Path $desktopWinUiDir -Component "DesktopWinUI"
        }
        $frontendExe = if ($restartFrontend -eq "qt") { "grxfirma-gui-qml.exe" } else { "grxfirma-winui.exe" }
        $launcher = Join-Path $launcherDir "grxfirma-gui.exe"
        $frontendPath = Join-Path $frontendDir $frontendExe
        if ((Test-GrxFirmaInstallMarker -Path $launcherDir -Component "DesktopLauncher") -and
            (Test-Path -LiteralPath $launcher -PathType Leaf) -and
            (Test-Path -LiteralPath $frontendPath -PathType Leaf)) {
            Start-Process -FilePath $launcher -ArgumentList "--frontend=$restartFrontend --start-hidden --ui-binary=`"$frontendPath`""
        }
    }
    return
}
$cliDir = Join-Path $BaseInstallDir "CLI"
$nativeDir = Join-Path $BaseInstallDir "NativeHost"
$afirmaDir = Join-Path $BaseInstallDir "AfirmaURI"

# Capturar el estado antes de que el instalador detenga los procesos antiguos.
Remove-ItemProperty -Path $restartKey -Name $restartName -ErrorAction SilentlyContinue
$expectedQt = [System.IO.Path]::GetFullPath((Join-Path $desktopQtDir "grxfirma-gui-qml.exe"))
$expectedWinUi = [System.IO.Path]::GetFullPath((Join-Path $desktopWinUiDir "grxfirma-winui.exe"))
$currentSession = [System.Diagnostics.Process]::GetCurrentProcess().SessionId
$restartFrontend = ""
foreach ($process in Get-Process -Name "grxfirma-gui-qml", "grxfirma-winui" -ErrorAction SilentlyContinue) {
    if ($process.SessionId -ne $currentSession) { continue }
    $exe = Get-GrxFirmaProcessPath -Process $process
    if (-not $exe) { continue }
    if ([string]::Equals($exe, $expectedQt, [System.StringComparison]::OrdinalIgnoreCase)) {
        $restartFrontend = "qt"
        break
    }
    if ([string]::Equals($exe, $expectedWinUi, [System.StringComparison]::OrdinalIgnoreCase)) {
        $restartFrontend = "winui"
    }
}
if (-not $restartFrontend) {
    if (Test-GrxFirmaStartupEnabled) { $restartFrontend = "winui" }
}
if ($restartFrontend) {
    New-Item -Path $restartKey -Force | Out-Null
    New-ItemProperty -Path $restartKey -Name $restartName -PropertyType String -Value $restartFrontend -Force | Out-Null
}

$cliSource = Join-Path $baseDir "grxfirma.exe"
$launcherSource = Join-Path $baseDir "grxfirma-gui.exe"
$nativeInstaller = Join-Path $baseDir "install-nativehost.ps1"
$afirmaInstaller = Join-Path $baseDir "install-afirmauri.ps1"
$desktopQtInstaller = Join-Path $baseDir "install-desktop-qml.ps1"
$desktopWinUiInstaller = Join-Path $baseDir "install-desktop-winui.ps1"
$desktopQtPackage = Join-Path $baseDir "desktop-qt"
$desktopWinUiPackage = Join-Path $baseDir "desktop-winui"

foreach ($required in @(
    $cliSource,
    $launcherSource,
    $nativeInstaller,
    $afirmaInstaller,
    $desktopQtInstaller,
    $desktopWinUiInstaller,
    $pathSafety
)) {
    if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
        throw "Falta un componente requerido del paquete: $required"
    }
}

# En el flujo NSIS, -CoreOnly se ejecuta antes de extraer las interfaces.
# Ignora tambien directorios de staging vacios que pueda dejar un intento
# interrumpido: cada seccion GUI valida despues su propio paquete completo.
$hasDesktopQt = (-not $CoreOnly) -and
    (Test-Path -LiteralPath $desktopQtPackage -PathType Container)
$hasDesktopWinUi = (-not $CoreOnly) -and
    (Test-Path -LiteralPath $desktopWinUiPackage -PathType Container)

Stop-GrxFirmaInstalledProcesses -Path $cliDir -Component "CLI"
Stop-GrxFirmaInstalledProcesses -Path $launcherDir -Component "DesktopLauncher"

$cliDir = Initialize-GrxFirmaInstallDirectory `
    -Path $cliDir `
    -Component "CLI" `
    -LegacyPayload "grxfirma.exe"
Copy-Item -LiteralPath $cliSource -Destination (Join-Path $cliDir "grxfirma.exe") -Force
$launcherDir = Clear-GrxFirmaInstallDirectory `
    -Path $launcherDir `
    -Component "DesktopLauncher" `
    -LegacyPayload "grxfirma-gui.exe"
Copy-Item `
    -LiteralPath $launcherSource `
    -Destination (Join-Path $launcherDir "grxfirma-gui.exe") `
    -Force
Set-GrxFirmaSuiteOwnershipMarker -Path $launcherDir -Component "DesktopLauncher"
$installedLauncher = Join-Path $launcherDir "grxfirma-gui.exe"
if ($hasDesktopQt) {
    & $desktopQtInstaller `
        -InstallDir $desktopQtDir `
        -PackageDir $desktopQtPackage `
        -LauncherPath $installedLauncher `
        -ValidateOnly
}
if ($hasDesktopWinUi) {
    & $desktopWinUiInstaller `
        -InstallDir $desktopWinUiDir `
        -PackageDir $desktopWinUiPackage `
        -LauncherPath $installedLauncher `
        -ValidateOnly
}

& $nativeInstaller -InstallDir $nativeDir
& $afirmaInstaller -InstallDir $afirmaDir -SilentInstall:$SilentInstall
if (-not $CoreOnly -and $hasDesktopQt) {
    & $desktopQtInstaller `
        -InstallDir $desktopQtDir `
        -PackageDir $desktopQtPackage `
        -LauncherPath $installedLauncher `
        -ManagedBySuite
}
if (-not $CoreOnly -and $hasDesktopWinUi) {
    & $desktopWinUiInstaller `
        -InstallDir $desktopWinUiDir `
        -PackageDir $desktopWinUiPackage `
        -LauncherPath $installedLauncher `
        -ManagedBySuite
}

Write-Host "Suite instalada en: $BaseInstallDir"
