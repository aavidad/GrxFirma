# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"
$windows = Split-Path -Parent $PSScriptRoot

$contracts = @(
    @{ File = "build-suite.ps1"; Names = @('GrxFirma-$Version-windows-$Arquitectura.zip', 'GrxFirma-$Version-windows-$Arquitectura-setup.exe') },
    @{ File = "build-cli.ps1"; Names = @('GrxFirma-$Version-cli-windows-$Arquitectura.zip', 'GrxFirma-$Version-cli-windows-$Arquitectura-setup.exe') },
    @{ File = "build-afirmauri.ps1"; Names = @('GrxFirma-$Version-afirmauri-windows-$Arquitectura.zip', 'GrxFirma-$Version-afirmauri-windows-$Arquitectura-setup.exe') },
    @{ File = "build-desktop-qml.ps1"; Names = @('GrxFirma-$Version-desktop-qml-windows-$Arquitectura.zip', 'GrxFirma-$Version-desktop-qml-windows-$Arquitectura-setup.exe') },
    @{ File = "build-nativehost.ps1"; Names = @('GrxFirma-$Version-nativehost-windows-$Arquitectura.zip') },
    @{ File = "build-msix.ps1"; Names = @('GrxFirma-$rawVersion-windows-$Arch.msix') },
    @{ File = "grxfirma-suite.nsi"; Names = @('GrxFirma-${VERSION}-windows-${ARCH}-setup.exe') },
    @{ File = "grxfirma-cli.nsi"; Names = @('GrxFirma-${VERSION}-cli-windows-${ARCH}-setup.exe') },
    @{ File = "grxfirma-afirmauri.nsi"; Names = @('GrxFirma-${VERSION}-afirmauri-windows-${ARCH}-setup.exe') },
    @{ File = "grxfirma-desktop-qml.nsi"; Names = @('GrxFirma-${VERSION}-desktop-qml-windows-${ARCH}-setup.exe') }
)
foreach ($contract in $contracts) {
    $source = Get-Content -LiteralPath (Join-Path $windows $contract.File) -Raw -Encoding UTF8
    foreach ($name in $contract.Names) {
        if (-not $source.Contains($name)) {
            throw "$($contract.File) no genera el nombre $name"
        }
    }
}
Write-Host "Contratos de nombres distribuidos Windows: PASS"
