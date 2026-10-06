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
$iconTarget = Join-Path $InstallDir "grxfirma-grx.ico"
$protocolSnapshot = Join-Path $InstallDir "afirma-protocol-snapshot.json"

$plan = @(Get-AfirmaProtocolRegistrationPlan `
    -ProtocolKey $protocolKey `
    -ExecutablePath $exeTarget `
    -IconPath $iconTarget)
$currentSet = New-AfirmaProtocolOwnerSet -Plan $plan
# Valores que escribieron versiones anteriores de esta misma instalación
# (por ejemplo, los iconos grxfirma-diputacion.ico y grxfirma.ico). La orden debe seguir
# apuntando a este ejecutable.
$legacySets = @(Get-AfirmaProtocolLegacyOwnerSets `
    -ProtocolKey $protocolKey `
    -ExecutablePath $exeTarget `
    -IconPath $iconTarget)
$acceptedSets = @($currentSet) + $legacySets

# Se valida la instantanea antes de detener procesos o retirar la CA local,
# para no dejar la desinstalacion a medias si no pertenece a esta instalacion.
$state = $null
if (Test-Path -LiteralPath $protocolSnapshot -PathType Leaf) {
    $state = Read-AfirmaProtocolSnapshot `
        -Path $protocolSnapshot `
        -ProtocolKey $protocolKey `
        -Plan $plan `
        -AcceptedLegacyOwnerSets $legacySets
}

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

$restored = $false
if ($null -ne $state) {
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues @($state.OwnerValues) `
        -Snapshots @($state.Snapshots)
    if (-not $restored) {
        # La instantanea y el registro pueden discrepar solo en el icono si
        # una actualizacion se interrumpio; ambos valores son de GrxFirma.
        foreach ($acceptedSet in $acceptedSets) {
            $restored = Restore-AfirmaProtocolRegistration `
                -OwnerValues @($acceptedSet.OwnerValues) `
                -Snapshots @($state.Snapshots)
            if ($restored) {
                break
            }
        }
    }
} else {
    $currentValues = @($plan | ForEach-Object {
        Get-AfirmaRegistryValueSnapshot `
            -Path ([string]$_.Path) `
            -Name ([string]$_.Name)
    })
    $ownedSet = Find-AfirmaMatchingOwnerSet `
        -Values $currentValues `
        -OwnerSets $acceptedSets
    if ($null -ne $ownedSet) {
        $legacySnapshots = @($plan | ForEach-Object {
            New-AfirmaAbsentValueSnapshot -Plan $_
        })
        $restored = Restore-AfirmaProtocolRegistration `
            -OwnerValues @($ownedSet.OwnerValues) `
            -Snapshots $legacySnapshots
    }
}

if (-not $restored) {
    Write-Warning "Se conserva el protocolo afirma:// porque ya no pertenece a esta instalacion."
}

Remove-GrxFirmaInstallDirectory `
    -Path $InstallDir `
    -Component "AfirmaURI" `
    -LegacyPayload "grxfirma-afirmauri.exe"

Write-Host "Handler afirma:// desinstalado de: $InstallDir"
