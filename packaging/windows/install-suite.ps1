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

# Codigos de salida que interpreta el instalador NSIS:
#   20  la comprobacion previa fallo y no se ha cambiado nada;
#   21  una fase fallo y se ha restaurado la version anterior;
#   otro  fallo inesperado o restauracion incompleta.
$exitPreflightRejected = 20
$exitRolledBack = 21

# Comprobacion previa: el protocolo afirma:// debe pertenecer a esta
# instalacion (o a ninguna) antes de detener procesos o copiar nada. Asi un
# conflicto no deja la version anterior a medias ni sin bandeja.
try {
    & $afirmaInstaller -InstallDir $afirmaDir -ValidateOnly
} catch {
    Remove-ItemProperty -Path $restartKey -Name $restartName -ErrorAction SilentlyContinue
    Write-Host "No se ha cambiado nada: $($_.Exception.Message)"
    exit $exitPreflightRejected
}

function New-GrxFirmaSuiteBackupRoot {
    $tempRoot = [System.IO.Path]::GetFullPath(
        [System.IO.Path]::GetTempPath()
    ).TrimEnd('\', '/')
    $leaf = "grxfirma-suite-backup-$([guid]::NewGuid().ToString('N'))"
    $root = Join-Path $tempRoot $leaf
    if (-not [string]::Equals(
            [System.IO.Path]::GetFullPath((Split-Path -Parent $root)).TrimEnd('\', '/'),
            $tempRoot,
            [System.StringComparison]::OrdinalIgnoreCase
        ) -or
        (Split-Path -Leaf $root) -notmatch '^grxfirma-suite-backup-[0-9a-f]{32}$') {
        throw "Ruta de copia de seguridad de la suite inesperada: $root"
    }
    New-Item -ItemType Directory -Path $root -ErrorAction Stop | Out-Null
    return $root
}

function Backup-GrxFirmaSuiteComponent {
    param(
        [Parameter(Mandatory = $true)] [string]$Path,
        [Parameter(Mandatory = $true)] [string]$Component,
        [Parameter(Mandatory = $true)] [string]$BackupRoot
    )

    $actual = Resolve-GrxFirmaInstallPath -Path $Path -Component $Component
    if (-not (Test-Path -LiteralPath $actual -PathType Container)) {
        return $false
    }
    $target = Join-Path $BackupRoot $Component
    Copy-Item -LiteralPath $actual -Destination $target -Recurse -Force -ErrorAction Stop
    Assert-NoGrxFirmaReparsePoint -Path $target
    return $true
}

function Restore-GrxFirmaSuiteComponent {
    param(
        [Parameter(Mandatory = $true)] [string]$Path,
        [Parameter(Mandatory = $true)] [string]$Component,
        [Parameter(Mandatory = $true)] [string]$LegacyPayload,
        [Parameter(Mandatory = $true)] [string]$BackupRoot,
        [Parameter(Mandatory = $true)] [bool]$Existed
    )

    $actual = Resolve-GrxFirmaInstallPath -Path $Path -Component $Component
    Stop-GrxFirmaInstalledProcesses -Path $actual -Component $Component
    if (Test-Path -LiteralPath $actual) {
        Remove-GrxFirmaInstallDirectory `
            -Path $actual `
            -Component $Component `
            -LegacyPayload $LegacyPayload
    }
    if ($Existed) {
        $source = Join-Path $BackupRoot $Component
        if (-not (Test-Path -LiteralPath $source -PathType Container)) {
            throw "Falta la copia de seguridad de ${Component}: $source"
        }
        Move-Item -LiteralPath $source -Destination $actual -ErrorAction Stop
    }
}

# Copia de los componentes que esta fase reemplaza directamente. AfirmaURI y
# NativeHost hacen su propia vuelta atras; NativeHost va el ultimo porque
# retira registros de nombres anteriores que no se pueden recuperar.
$backupRoot = New-GrxFirmaSuiteBackupRoot
$preserveBackup = $false
try {
    $cliExisted = Backup-GrxFirmaSuiteComponent -Path $cliDir -Component "CLI" -BackupRoot $backupRoot
    $launcherExisted = Backup-GrxFirmaSuiteComponent -Path $launcherDir -Component "DesktopLauncher" -BackupRoot $backupRoot
} catch {
    Remove-Item -LiteralPath $backupRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-ItemProperty -Path $restartKey -Name $restartName -ErrorAction SilentlyContinue
    Write-Host "No se ha cambiado nada: no se pudo copiar la version anterior: $($_.Exception.Message)"
    exit $exitPreflightRejected
}

try {
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

    & $afirmaInstaller -InstallDir $afirmaDir -SilentInstall:$SilentInstall
    & $nativeInstaller -InstallDir $nativeDir
} catch {
    $installError = $_.Exception
    $rollbackErrors = @()
    foreach ($component in @(
        @{ Path = $launcherDir; Name = "DesktopLauncher"; Payload = "grxfirma-gui.exe"; Existed = $launcherExisted },
        @{ Path = $cliDir; Name = "CLI"; Payload = "grxfirma.exe"; Existed = $cliExisted }
    )) {
        try {
            Restore-GrxFirmaSuiteComponent `
                -Path $component.Path `
                -Component $component.Name `
                -LegacyPayload $component.Payload `
                -BackupRoot $backupRoot `
                -Existed $component.Existed
        } catch {
            $rollbackErrors += "$($component.Name): $($_.Exception.Message)"
        }
    }
    if ($rollbackErrors.Count -gt 0) {
        $preserveBackup = $true
        throw "Fallo instalando la suite: $($installError.Message). Restauracion incompleta: $($rollbackErrors -join '; '). Copia conservada en $backupRoot"
    }
    Write-Host "Se ha restaurado la version anterior: $($installError.Message)"
    exit $exitRolledBack
} finally {
    if (-not $preserveBackup -and (Test-Path -LiteralPath $backupRoot)) {
        Remove-Item -LiteralPath $backupRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}

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

# Las versiones anteriores dejaron accesos en el escritorio con otros nombres.
# Solo se retiran si apuntan a un programa instalado por GrxFirma o AutoFirmaV2.
function Remove-GrxFirmaLegacyDesktopShortcuts {
    param([string]$BaseInstallDir)
    $roots = @(
        $BaseInstallDir,
        (Join-Path $env:LOCALAPPDATA "GrxFirma"),
        (Join-Path $env:LOCALAPPDATA "AutoFirmaV2"),
        (Join-Path $env:LOCALAPPDATA "Programs\AutoFirmaV2")
    ) | ForEach-Object { [System.IO.Path]::GetFullPath($_).TrimEnd('\') + '\' }
    $names = @("GrxFirma Diputación.lnk", "AutoFirma Diputación.lnk", "AutoFirmaV2.lnk")
    $desktop = [Environment]::GetFolderPath("Desktop")
    if ([string]::IsNullOrWhiteSpace($desktop)) { return }
    $shell = New-Object -ComObject WScript.Shell
    foreach ($name in $names) {
        $link = Join-Path $desktop $name
        if (-not (Test-Path -LiteralPath $link -PathType Leaf)) { continue }
        try {
            $target = $shell.CreateShortcut($link).TargetPath
            if ([string]::IsNullOrWhiteSpace($target)) { continue }
            $full = [System.IO.Path]::GetFullPath($target)
            $owned = $roots | Where-Object { $full.StartsWith($_, [System.StringComparison]::OrdinalIgnoreCase) }
            if ($owned) {
                Remove-Item -LiteralPath $link -Force
                Write-Host "Acceso antiguo retirado del escritorio: $name"
            }
        } catch {
            Write-Host "No se pudo revisar el acceso antiguo $name del escritorio: $($_.Exception.Message)"
        }
    }
}

if (-not $CoreOnly) {
    Remove-GrxFirmaLegacyDesktopShortcuts -BaseInstallDir $BaseInstallDir
}

Write-Host "Suite instalada en: $BaseInstallDir"
