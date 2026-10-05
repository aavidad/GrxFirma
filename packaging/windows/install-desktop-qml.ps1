# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\DesktopQML",
    [string]$PackageDir = "",
    [string]$LauncherPath = "",
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
$baseDir = [System.IO.Path]::GetFullPath($PackageDir)
Assert-NoGrxFirmaReparsePoint -Path $baseDir

$exeSource = Join-Path $baseDir "grxfirma-gui-qml.exe"
$ipcBackendSource = Join-Path $baseDir "grxfirma-gui.exe"
$backendSource = Join-Path $baseDir "grxfirma.exe"
$requiredQtRuntime = @(
    "Qt6Core.dll",
    "Qt6Gui.dll",
    "Qt6Qml.dll",
    "Qt6Quick.dll",
    "platforms/qwindows.dll"
)
$requiredQmlPlugins = @(
    "qmlplugin.dll",
    "qtquick2plugin.dll",
    "qtquickcontrols2plugin.dll",
    "qquicklayoutsplugin.dll",
    "qtquickdialogsplugin.dll",
    "qmlsettingsplugin.dll"
)
$requiredMinGwRuntime = @(
    "libgcc_s_seh-1.dll",
    "libstdc++-6.dll",
    "libwinpthread-1.dll"
)

function Get-DesktopPackageFileNames {
    return @(
        Get-ChildItem -LiteralPath $baseDir -File -Recurse -ErrorAction SilentlyContinue |
            ForEach-Object { $_.Name }
    )
}

function Assert-DesktopPackageRuntime {
    $missing = @()
    foreach ($relativePath in $requiredQtRuntime) {
        if (-not (Test-Path -LiteralPath (Join-Path $baseDir $relativePath) -PathType Leaf)) {
            $missing += $relativePath
        }
    }

    $fileNames = @(Get-DesktopPackageFileNames)
    $rootFileNames = @(
        Get-ChildItem -LiteralPath $baseDir -File -ErrorAction SilentlyContinue |
            ForEach-Object { $_.Name }
    )
    foreach ($pluginName in $requiredQmlPlugins) {
        if ($fileNames -notcontains $pluginName) {
            $missing += "*/$pluginName"
        }
    }

    $redistributableCandidates = @(
        Get-ChildItem -LiteralPath $baseDir -File -Filter "vc_redist.*.exe" -ErrorAction SilentlyContinue
    )
    $msvcRedistributable = @(
        $redistributableCandidates |
            Where-Object { $_.Name -ieq "vc_redist.x64.exe" }
    )
    if ($redistributableCandidates.Count -gt 0 -and
        ($redistributableCandidates.Count -ne 1 -or $msvcRedistributable.Count -ne 1)) {
        throw "El paquete debe incluir como maximo un unico vc_redist.x64.exe."
    }
    $hasMinGwRuntime = $true
    foreach ($dll in $requiredMinGwRuntime) {
        if ($rootFileNames -notcontains $dll) {
            $hasMinGwRuntime = $false
            break
        }
    }
    if ($msvcRedistributable.Count -eq 0 -and (-not $hasMinGwRuntime)) {
        $missing += "vc_redist.x64.exe o runtime MinGW"
    }

    if ($missing.Count -gt 0) {
        throw ("El paquete Qt/QML esta incompleto. Faltan:`n  - " + ($missing -join "`n  - "))
    }
    Assert-MsvcRedistributableSignature -Installers $msvcRedistributable
    return $msvcRedistributable
}

function Assert-MsvcRedistributableSignature {
    param(
        [System.IO.FileInfo[]]$Installers
    )

    if ($Installers.Count -gt 0) {
        $securityModule = Join-Path `
            $PSHOME `
            "Modules\Microsoft.PowerShell.Security\Microsoft.PowerShell.Security.psd1"
        if (-not (Test-Path -LiteralPath $securityModule -PathType Leaf)) {
            throw "No se encuentra el modulo de seguridad de PowerShell en su ruta de confianza."
        }
        Import-Module -Name $securityModule -ErrorAction Stop
    }
    foreach ($installer in $Installers) {
        $signature = Get-AuthenticodeSignature -LiteralPath $installer.FullName
        if ([string]$signature.Status -ne "Valid" -or
            $null -eq $signature.SignerCertificate -or
            $signature.SignerCertificate.Subject -notmatch '(^|,\s*)O=Microsoft Corporation(,|$)') {
            throw "El redistribuible $($installer.Name) no tiene una firma Authenticode valida de Microsoft."
        }
    }
}

function Install-MsvcRedistributable {
    param(
        [System.IO.FileInfo[]]$Installers
    )

    Assert-MsvcRedistributableSignature -Installers $Installers
    foreach ($installer in $Installers) {
        $process = Start-Process `
            -FilePath $installer.FullName `
            -ArgumentList @("/install", "/quiet", "/norestart") `
            -Wait `
            -PassThru `
            -ErrorAction Stop
        $exitCode = $process.ExitCode
        if ($exitCode -notin @(0, 1638, 3010)) {
            throw "La instalacion de $($installer.Name) fallo con codigo $exitCode"
        }
        if ($exitCode -eq 3010) {
            Write-Warning "El runtime MSVC solicita reiniciar Windows para completar la instalacion."
        }
    }
}

if (-not (Test-Path -LiteralPath $exeSource -PathType Leaf)) {
    throw "No se encuentra grxfirma-gui-qml.exe junto al instalador."
}
if (-not (Test-Path -LiteralPath $ipcBackendSource -PathType Leaf)) {
    throw "No se encuentra grxfirma-gui.exe junto al instalador."
}
if (-not (Test-Path -LiteralPath $backendSource -PathType Leaf)) {
    throw "No se encuentra grxfirma.exe junto al instalador."
}
$msvcRedistributables = @(Assert-DesktopPackageRuntime)

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "DesktopQML"
$useSharedLauncher = -not [string]::IsNullOrWhiteSpace($LauncherPath)
if ($useSharedLauncher) {
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
        (Get-FileHash -LiteralPath $ipcBackendSource -Algorithm SHA256).Hash) {
        throw "El backend Qt no coincide con el lanzador compartido instalado."
    }
}
if ($ValidateOnly) {
    Write-Host "Paquete Desktop Qt/QML validado para: $InstallDir"
    return
}

Stop-GrxFirmaInstalledProcesses -Path $InstallDir -Component "DesktopQML"

$InstallDir = Clear-GrxFirmaInstallDirectory `
    -Path $InstallDir `
    -Component "DesktopQML" `
    -LegacyPayload "grxfirma-gui-qml.exe"
if ($ManagedBySuite) {
    # Se marca antes de copiar para que un fallo parcial siga siendo
    # recuperable mediante el desinstalador de la suite.
    Set-GrxFirmaSuiteOwnershipMarker -Path $InstallDir -Component "DesktopQML"
}
Copy-Item $exeSource (Join-Path $InstallDir "grxfirma-gui-qml.exe") -Force
if (-not $useSharedLauncher) {
    Copy-Item $ipcBackendSource (Join-Path $InstallDir "grxfirma-gui.exe") -Force
    Copy-Item $backendSource (Join-Path $InstallDir "grxfirma.exe") -Force
}

foreach ($dir in @("qml", "assets", "help", "qt-qml")) {
    $src = Join-Path $baseDir $dir
    if (Test-Path $src) {
        Copy-Item $src (Join-Path $InstallDir $dir) -Recurse -Force
    }
}

foreach ($pattern in @("*.dll", "qt.conf")) {
    Get-ChildItem -Path $baseDir -Filter $pattern -ErrorAction SilentlyContinue | ForEach-Object {
        Copy-Item $_.FullName (Join-Path $InstallDir $_.Name) -Force
    }
}

foreach ($dir in @("platforms", "styles", "imageformats", "networkinformation", "tls", "iconengines", "generic")) {
    $src = Join-Path $baseDir $dir
    if (Test-Path $src) {
        Copy-Item $src (Join-Path $InstallDir $dir) -Recurse -Force
    }
}

foreach ($relativePath in $requiredQtRuntime) {
    if (-not (Test-Path -LiteralPath (Join-Path $InstallDir $relativePath) -PathType Leaf)) {
        throw "No se ha instalado la dependencia runtime requerida: $relativePath"
    }
}

if ($msvcRedistributables.Count -gt 0) {
    Install-MsvcRedistributable -Installers $msvcRedistributables
}
$programsDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
$startMenuDir = Join-Path $programsDir "GrxFirma"
# Versiones anteriores creaban los accesos en esta carpeta; se retiran solo los
# propios y la carpeta se elimina únicamente si queda vacía.
$legacyStartMenuDir = Join-Path $programsDir "Diputación de Granada"
New-Item -ItemType Directory -Force -Path $startMenuDir | Out-Null

$wshell = New-Object -ComObject WScript.Shell
$shortcutName = if ($useSharedLauncher) {
    "GrxFirma - Qt.lnk"
} else {
    "GrxFirma Desktop Qt.lnk"
}
foreach ($oldName in @("GrxFirma - Qt.lnk", "GrxFirma Desktop Qt.lnk")) {
    Remove-Item -LiteralPath (Join-Path $startMenuDir $oldName) -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $legacyStartMenuDir $oldName) -Force -ErrorAction SilentlyContinue
}
if (Test-Path -LiteralPath $legacyStartMenuDir -PathType Container) {
    $legacyRemaining = @(
        Get-ChildItem -LiteralPath $legacyStartMenuDir -Force -ErrorAction SilentlyContinue
    )
    if ($legacyRemaining.Count -eq 0) {
        Remove-Item -LiteralPath $legacyStartMenuDir -Force
    }
}
$shortcut = $wshell.CreateShortcut((Join-Path $startMenuDir $shortcutName))
$shortcut.TargetPath = if ($useSharedLauncher) {
    $LauncherPath
} else {
    Join-Path $InstallDir "grxfirma-gui-qml.exe"
}
if ($useSharedLauncher) {
    $shortcut.Arguments = '--frontend=qt --ui-binary="' +
        (Join-Path $InstallDir "grxfirma-gui-qml.exe") + '"'
}
$shortcut.WorkingDirectory = $InstallDir
$shortcut.Description = "GrxFirma Desktop Qt/QML"
$shortcut.IconLocation = (Join-Path $InstallDir "assets\grxfirma-diputacion.ico") + ",0"
$shortcut.Save()

Write-Host "Desktop Qt/QML instalado en: $InstallDir"
Write-Host "Acceso directo creado en: $startMenuDir"
