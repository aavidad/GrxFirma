# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Hook !uninstfinalize de NSIS para la firma con SignPath.
#   capture: copia el uninstaller sin firmar para enviarlo a SignPath y no lo
#            modifica.
#   inject:  exige que el uninstaller regenerado sea identico byte a byte al
#            enviado y lo sustituye por la copia firmada que devolvio SignPath.
# No maneja credenciales: SignPath firma en su HSM y aqui solo se mueven bytes.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ConfigurationPath,

    [Parameter(Mandatory = $true)]
    [string]$FilePath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Get-HookSha256 {
    param([Parameter(Mandatory = $true)][string]$Path)
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

$configurationFile = Get-Item -LiteralPath $ConfigurationPath -ErrorAction Stop
if ($configurationFile.Length -gt 65536) {
    throw "La configuracion del hook SignPath supera el limite permitido."
}
try {
    $configuration = Get-Content `
        -LiteralPath $configurationFile.FullName `
        -Raw `
        -ErrorAction Stop |
        ConvertFrom-Json -ErrorAction Stop
} catch {
    throw "La configuracion del hook SignPath no es JSON valido: $($_.Exception.Message)"
}
if ($configuration.schema_version -ne 1) {
    throw "Version de configuracion del hook SignPath no valida."
}

$resolvedFile = (Resolve-Path -LiteralPath $FilePath).Path

switch ([string]$configuration.mode) {
    'capture' {
        $capturePath = [System.IO.Path]::GetFullPath(
            [string]$configuration.capture_path
        )
        if (Test-Path -LiteralPath $capturePath) {
            throw "El uninstaller ya se habia capturado: $capturePath"
        }
        $captureParent = Split-Path -Parent $capturePath
        if (-not (Test-Path -LiteralPath $captureParent -PathType Container)) {
            throw "No existe el directorio de captura del uninstaller."
        }
        Copy-Item `
            -LiteralPath $resolvedFile `
            -Destination $capturePath `
            -ErrorAction Stop
        if ((Get-HookSha256 -Path $capturePath) -ne
            (Get-HookSha256 -Path $resolvedFile)) {
            throw "La captura del uninstaller no coincide con el original."
        }
        break
    }
    'inject' {
        $expectedUnsigned = ([string]$configuration.expected_unsigned_sha256).ToLowerInvariant()
        if ($expectedUnsigned -notmatch '^[0-9a-f]{64}$') {
            throw "expected_unsigned_sha256 debe tener 64 hexadecimales."
        }
        $signedPath = [System.IO.Path]::GetFullPath(
            [string]$configuration.signed_path
        )
        $evidencePath = [System.IO.Path]::GetFullPath(
            [string]$configuration.evidence_path
        )
        if (-not (Test-Path -LiteralPath $signedPath -PathType Leaf)) {
            throw "Falta el uninstaller firmado por SignPath: $signedPath"
        }
        foreach ($forbidden in @($signedPath, $evidencePath)) {
            if ([string]::Equals(
                    $resolvedFile,
                    $forbidden,
                    [System.StringComparison]::OrdinalIgnoreCase
                )) {
                throw "El hook no puede sobrescribir sus propias entradas."
            }
        }
        $actualUnsigned = Get-HookSha256 -Path $resolvedFile
        if ($actualUnsigned -ne $expectedUnsigned) {
            throw "El uninstaller regenerado ($actualUnsigned) no coincide con el que se envio a SignPath ($expectedUnsigned)."
        }
        Copy-Item `
            -LiteralPath $signedPath `
            -Destination $resolvedFile `
            -Force `
            -ErrorAction Stop
        Copy-Item `
            -LiteralPath $resolvedFile `
            -Destination $evidencePath `
            -Force `
            -ErrorAction Stop
        $signedHash = Get-HookSha256 -Path $signedPath
        if ((Get-HookSha256 -Path $resolvedFile) -ne $signedHash -or
            (Get-HookSha256 -Path $evidencePath) -ne $signedHash) {
            throw "La sustitucion del uninstaller firmado no es exacta."
        }
        break
    }
    default {
        throw "Modo de hook SignPath no valido: $($configuration.mode)"
    }
}
