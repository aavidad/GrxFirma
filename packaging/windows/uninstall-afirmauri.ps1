# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\AfirmaURI",
    [switch]$Silent
)

$ErrorActionPreference = "Stop"

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $baseDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al desinstalador."
}
. $pathSafety
$registrationHelpers = Join-Path $baseDir "afirmauri-registration.ps1"
if (-not (Test-Path -LiteralPath $registrationHelpers -PathType Leaf)) {
    throw "No se encuentra afirmauri-registration.ps1 junto al desinstalador."
}
. $registrationHelpers

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "AfirmaURI"
$protocolKey = "Software\Classes\afirma"
$exeTarget = Join-Path $InstallDir "grxfirma-afirmauri.exe"
$iconTarget = Join-Path $InstallDir "grxfirma-diputacion.ico"
$protocolSnapshot = Join-Path $InstallDir "afirma-protocol-snapshot.json"

Stop-GrxFirmaInstalledProcesses -Path $InstallDir -Component "AfirmaURI"
if (Test-Path -LiteralPath $exeTarget -PathType Leaf) {
    $cleanupArguments = @("--remove-local-tls-trust")
    if ($Silent) {
        $cleanupArguments += "--no-prompt"
    }
    $cleanupProcess = $null
    try {
        $cleanupProcess = Start-Process `
            -FilePath $exeTarget `
            -ArgumentList $cleanupArguments `
            -WindowStyle Hidden `
            -PassThru `
            -ErrorAction Stop
        if (-not $cleanupProcess.WaitForExit(120000)) {
            $cleanupProcess.Kill()
            if (-not $cleanupProcess.WaitForExit(10000)) {
                throw "La limpieza TLS local no terminó tras detener el proceso."
            }
            throw "La limpieza TLS local superó los 120 segundos. Se conserva la instalacion para reintentar."
        }
        if ($cleanupProcess.ExitCode -ne 0) {
            throw "La limpieza TLS local devolvió $($cleanupProcess.ExitCode). Se conserva la instalacion para reintentar."
        }
        if ($Silent) {
            Write-Warning "CA local inerte: clave privada eliminada; retirada de ROOT pendiente de sesión interactiva. Marcador conservado en los datos TLS de GrxFirma."
        }
    } finally {
        if ($null -ne $cleanupProcess) {
            $cleanupProcess.Dispose()
        }
    }
}

$plan = @(Get-AfirmaProtocolRegistrationPlan `
    -ProtocolKey $protocolKey `
    -ExecutablePath $exeTarget `
    -IconPath $iconTarget)
$ownerValues = @($plan | ForEach-Object {
    New-AfirmaOwnedValueSnapshot -Plan $_
})

$restored = $false
if (Test-Path -LiteralPath $protocolSnapshot -PathType Leaf) {
    $state = Read-AfirmaProtocolSnapshot `
        -Path $protocolSnapshot `
        -ProtocolKey $protocolKey `
        -Plan $plan
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues @($state.OwnerValues) `
        -Snapshots @($state.Snapshots)
} elseif (Test-AfirmaProtocolValuesMatch -Expected $ownerValues) {
    $legacySnapshots = @($plan | ForEach-Object {
        New-AfirmaAbsentValueSnapshot -Plan $_
    })
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues $ownerValues `
        -Snapshots $legacySnapshots
}

if (-not $restored) {
    Write-Warning "Se conserva el protocolo afirma:// porque ya no pertenece a esta instalacion."
}

Remove-GrxFirmaInstallDirectory `
    -Path $InstallDir `
    -Component "AfirmaURI" `
    -LegacyPayload "grxfirma-afirmauri.exe"

Write-Host "Handler afirma:// desinstalado de: $InstallDir"
