# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ConfigurationPath,

    [Parameter(Mandatory = $true)]
    [string]$FilePath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if (-not ($PSVersionTable.PSEdition -eq 'Desktop' -or $IsWindows -eq $true)) {
    throw "La firma Authenticode requiere Windows."
}

$helpers = Join-Path $PSScriptRoot 'windows-authenticode.ps1'
if (-not (Test-Path -LiteralPath $helpers -PathType Leaf)) {
    throw "Falta windows-authenticode.ps1."
}
. $helpers

$configurationFile = Get-Item -LiteralPath $ConfigurationPath -ErrorAction Stop
if ($configurationFile.Length -gt 65536) {
    throw "La configuracion Authenticode supera el limite permitido."
}
try {
    $configuration = Get-Content `
        -LiteralPath $configurationFile.FullName `
        -Raw `
        -ErrorAction Stop |
        ConvertFrom-Json -ErrorAction Stop
} catch {
    throw "La configuracion Authenticode no es JSON valido: $($_.Exception.Message)"
}
if ($configuration.schema_version -ne 1) {
    throw "Version de configuracion Authenticode no valida."
}

$resolvedFile = (Resolve-Path -LiteralPath $FilePath).Path
$resolvedSignTool = (Resolve-Path `
    -LiteralPath ([string]$configuration.sign_tool_path)).Path
$thumbprint = Get-NormalizedWindowsSigningThumbprint `
    -Thumbprint ([string]$configuration.expected_thumbprint)
$timestampUrl = Get-NormalizedRfc3161TimestampUrl `
    -TimestampUrl ([string]$configuration.timestamp_url)

Invoke-WindowsAuthenticodeSign `
    -Path $resolvedFile `
    -SignToolPath $resolvedSignTool `
    -ExpectedThumbprint $thumbprint `
    -TimestampUrl $timestampUrl |
    Out-Null

$evidencePath = [System.IO.Path]::GetFullPath(
    [string]$configuration.evidence_path
)
if ([string]::Equals(
        $resolvedFile,
        $evidencePath,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
    throw "La evidencia del uninstaller no puede sobrescribir el original."
}
$evidenceParent = Split-Path -Parent $evidencePath
if (-not (Test-Path -LiteralPath $evidenceParent -PathType Container)) {
    throw "No existe el directorio seguro para la evidencia Authenticode."
}
Copy-Item `
    -LiteralPath $resolvedFile `
    -Destination $evidencePath `
    -Force `
    -ErrorAction Stop
Assert-WindowsAuthenticodeFile `
    -Path $evidencePath `
    -SignToolPath $resolvedSignTool `
    -ExpectedThumbprint $thumbprint `
    -RequireSha256Rfc3161 |
    Out-Null
