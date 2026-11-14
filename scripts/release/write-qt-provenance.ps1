# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Calcula el SHA-256 de las DLL y EXE de la instalación de Qt recién descargada
# (sin caché de Actions). finalize-windows-release.ps1 solo firma un binario
# ajeno sin firma si su huella está en este inventario (G-01).

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$QtRoot,

    [Parameter(Mandatory = $true)]
    [string]$OutputPath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot 'windows-signing-provenance.ps1')
Write-WindowsQtProvenanceManifest -QtRoot $QtRoot -OutputPath $OutputPath
