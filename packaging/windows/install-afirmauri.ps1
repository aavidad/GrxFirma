# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\AfirmaURI",
    [switch]$SilentInstall
)

$ErrorActionPreference = "Stop"

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $baseDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al instalador."
}
. $pathSafety
$registrationHelpers = Join-Path $baseDir "afirmauri-registration.ps1"
if (-not (Test-Path -LiteralPath $registrationHelpers -PathType Leaf)) {
    throw "No se encuentra afirmauri-registration.ps1 junto al instalador."
}
. $registrationHelpers

function Restore-AfirmaUriInstallSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstallDir,
        [Parameter(Mandatory = $true)]
        [string]$BackupPayload,
        [Parameter(Mandatory = $true)]
        [bool]$PreviousInstallExisted
    )

    if (Test-Path -LiteralPath $InstallDir) {
        Assert-NoGrxFirmaReparsePoint -Path $InstallDir
        Remove-Item -LiteralPath $InstallDir -Recurse -Force -ErrorAction Stop
    }
    if ($PreviousInstallExisted) {
        if (-not (Test-Path -LiteralPath $BackupPayload -PathType Container)) {
            throw "La instantanea AfirmaURI no esta disponible: $BackupPayload"
        }
        Move-Item -LiteralPath $BackupPayload -Destination $InstallDir -ErrorAction Stop
    }
}

$exeSource = Join-Path $baseDir "grxfirma-afirmauri.exe"
$iconSource = Join-Path $baseDir "grxfirma-diputacion.ico"
if (-not (Test-Path -LiteralPath $exeSource -PathType Leaf)) {
    throw "No se encuentra grxfirma-afirmauri.exe junto al instalador."
}
if (-not (Test-Path -LiteralPath $iconSource -PathType Leaf)) {
    throw "No se encuentra el icono de GrxFirma junto al instalador."
}

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "AfirmaURI"
$previousInstallExisted = Test-Path -LiteralPath $InstallDir -PathType Container
if ($previousInstallExisted -and
    (-not (Test-GrxFirmaInstallMarker -Path $InstallDir -Component "AfirmaURI")) -and
    (-not (Test-Path -LiteralPath (Join-Path $InstallDir "grxfirma-afirmauri.exe") -PathType Leaf))) {
    throw "La ruta contiene datos que no pertenecen a GrxFirma y no se modificara: $InstallDir"
}

Stop-GrxFirmaInstalledProcesses -Path $InstallDir -Component "AfirmaURI"

$backupLeaf = "grxfirma-afirmauri-backup-$([guid]::NewGuid().ToString('N'))"
$backupRoot = Join-Path ([System.IO.Path]::GetTempPath()) $backupLeaf
$backupParent = [System.IO.Path]::GetFullPath(
    [System.IO.Path]::GetTempPath()
).TrimEnd('\', '/')
if (-not [string]::Equals(
        [System.IO.Path]::GetFullPath(
            (Split-Path -Parent $backupRoot)
        ).TrimEnd('\', '/'),
        $backupParent,
        [System.StringComparison]::OrdinalIgnoreCase
    ) -or
    (Split-Path -Leaf $backupRoot) -notmatch
        '^grxfirma-afirmauri-backup-[0-9a-f]{32}$') {
    throw "Ruta de instantanea AfirmaURI inesperada: $backupRoot"
}
New-Item -ItemType Directory -Path $backupRoot -ErrorAction Stop | Out-Null
$backupPayload = Join-Path $backupRoot "payload"
try {
    if ($previousInstallExisted) {
        Copy-Item `
            -LiteralPath $InstallDir `
            -Destination $backupPayload `
            -Recurse `
            -Force `
            -ErrorAction Stop
        Assert-NoGrxFirmaReparsePoint -Path $backupPayload
    }
} catch {
    Remove-Item -LiteralPath $backupRoot -Recurse -Force -ErrorAction SilentlyContinue
    throw
}

$filesystemMutationStarted = $false
$preserveBackupForRecovery = $false
try {
    $filesystemMutationStarted = $true
    $InstallDir = Initialize-GrxFirmaInstallDirectory `
        -Path $InstallDir `
        -Component "AfirmaURI" `
        -LegacyPayload "grxfirma-afirmauri.exe"
    $exeTarget = Join-Path $InstallDir "grxfirma-afirmauri.exe"
    $iconTarget = Join-Path $InstallDir "grxfirma-diputacion.ico"
    Copy-Item -LiteralPath $exeSource -Destination $exeTarget -Force
    Copy-Item -LiteralPath $iconSource -Destination $iconTarget -Force

    $handler = Join-Path $InstallDir "afirmauri-handler.cmd"
    $handlerContent = @"
@echo off
"$exeTarget" %*
"@
    Set-Content -LiteralPath $handler -Value $handlerContent -Encoding ASCII
    $legacyDebugHandler = Join-Path $InstallDir "afirmauri-handler-debug.cmd"
    if (Test-Path -LiteralPath $legacyDebugHandler -PathType Leaf) {
        Remove-Item -LiteralPath $legacyDebugHandler -Force
    }

    $protocolKey = "Software\Classes\afirma"
    $protocolSnapshot = Join-Path $InstallDir "afirma-protocol-snapshot.json"
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $exeTarget `
        -IconPath $iconTarget `
        -SnapshotPath $protocolSnapshot

    Write-Host "Handler afirma:// instalado en: $InstallDir"
    Write-Host "Protocolo afirma:// registrado para el usuario actual"
    Write-Host "Lanzador normal: $exeTarget"
} catch {
    $installError = $_.Exception
    if ($filesystemMutationStarted) {
        try {
            Restore-AfirmaUriInstallSnapshot `
                -InstallDir $InstallDir `
                -BackupPayload $backupPayload `
                -PreviousInstallExisted $previousInstallExisted
        } catch {
            $preserveBackupForRecovery = $true
            throw "Fallo instalando AfirmaURI: $($installError.Message). Rollback de ficheros incompleto: $($_.Exception.Message). Instantanea conservada en $backupPayload"
        }
    }
    throw $installError
} finally {
    if (-not $preserveBackupForRecovery -and
        (Test-Path -LiteralPath $backupRoot)) {
        Remove-Item `
            -LiteralPath $backupRoot `
            -Recurse `
            -Force `
            -ErrorAction SilentlyContinue
    }
}

# La CA del WebSocket se solicita en el primer uso si /S evita los avisos de
# Windows o si la instalación interactiva no consigue completar este paso.
function Invoke-GrxFirmaLocalTlsTrust {
    param(
        [string]$InstallDir,
        [switch]$SilentInstall
    )

    if ($SilentInstall) {
        Write-Host "Instalación silenciosa: la confianza TLS local se solicitará en el primer uso."
        return
    }

    $trustProcess = $null
    try {
        $trustProcess = Start-Process `
            -FilePath (Join-Path $InstallDir "grxfirma-afirmauri.exe") `
            -ArgumentList @("--install-local-tls-trust") `
            -WindowStyle Hidden `
            -PassThru
        if (-not $trustProcess.WaitForExit(300000)) {
            try {
                $trustProcess.Kill()
            } catch {
                Write-Warning "No se pudo detener el proceso de confianza TLS local: $($_.Exception.Message)"
            }
            Write-Warning "La confirmación del certificado local superó los 5 minutos. La instalación continúa; se volverá a solicitar en el primer uso."
        } elseif ($trustProcess.ExitCode -ne 0) {
            Write-Warning "No se instaló el certificado local de GrxFirma (código $($trustProcess.ExitCode)). Se volverá a solicitar la primera vez que un portal use la aplicación."
        }
    } catch {
        Write-Warning "No se pudo preparar la confianza TLS local: $($_.Exception.Message). Se volverá a solicitar en el primer uso."
    } finally {
        if ($null -ne $trustProcess) {
            $trustProcess.Dispose()
        }
    }
}

Invoke-GrxFirmaLocalTlsTrust -InstallDir $InstallDir -SilentInstall:$SilentInstall
