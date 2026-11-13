# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# G-01: un PE ajeno sin firma solo se firma si su huella procede del build.

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot '..' 'windows-signing-provenance.ps1')

function Assert-Rejects {
    param([scriptblock]$Action, [string]$Message)
    try { & $Action } catch { return }
    throw $Message
}

$root = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-provenance-" + [guid]::NewGuid().ToString('N'))
try {
    $qt = Join-Path $root 'qt/bin'
    $stage = Join-Path $root 'stage'
    $winui = Join-Path $stage 'desktop-winui/app'
    New-Item -ItemType Directory -Force -Path $qt, (Join-Path $stage 'desktop-qt'), $winui | Out-Null

    [System.IO.File]::WriteAllBytes((Join-Path $qt 'Qt6Core.dll'), [byte[]](1, 2, 3))
    $manifest = Join-Path $root 'qt-provenance.sha256'
    Write-WindowsQtProvenanceManifest -QtRoot (Join-Path $root 'qt') -OutputPath $manifest
    $provenance = Get-WindowsQtProvenance -ManifestPath $manifest

    # Copia legítima de Qt: se admite.
    Copy-Item (Join-Path $qt 'Qt6Core.dll') (Join-Path $stage 'desktop-qt/Qt6Core.dll')
    Assert-WindowsUnsignedVendorProvenance -FilePath (Join-Path $stage 'desktop-qt/Qt6Core.dll') -StageDirectory $stage -QtProvenance $provenance

    # DLL de Qt alterada (p. ej. llegada desde una caché envenenada): se rechaza.
    [System.IO.File]::WriteAllBytes((Join-Path $stage 'desktop-qt/Qt6Gui.dll'), [byte[]](9, 9, 9))
    Assert-Rejects { Assert-WindowsUnsignedVendorProvenance -FilePath (Join-Path $stage 'desktop-qt/Qt6Gui.dll') -StageDirectory $stage -QtProvenance $provenance } `
        "Se aceptó una DLL que no está en el inventario de Qt."

    # Misma huella con otro nombre: se rechaza.
    Copy-Item (Join-Path $qt 'Qt6Core.dll') (Join-Path $stage 'desktop-qt/evil.dll')
    Assert-Rejects { Assert-WindowsUnsignedVendorProvenance -FilePath (Join-Path $stage 'desktop-qt/evil.dll') -StageDirectory $stage -QtProvenance $provenance } `
        "Se aceptó una huella de Qt con otro nombre."

    # Ensamblado WinUI del propio build, listado en PUBLISH-MANIFEST: se admite.
    $appDll = Join-Path $winui 'GrxFirma.App.dll'
    [System.IO.File]::WriteAllBytes($appDll, [byte[]](4, 5, 6))
    $appHash = (Get-FileHash -LiteralPath $appDll -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath (Join-Path $stage 'desktop-winui/PUBLISH-MANIFEST.sha256') -Value "$appHash *app/GrxFirma.App.dll"
    Assert-WindowsUnsignedVendorProvenance -FilePath $appDll -StageDirectory $stage -QtProvenance $provenance

    # Ensamblado WinUI modificado tras el inventario: se rechaza.
    [System.IO.File]::WriteAllBytes($appDll, [byte[]](7, 7, 7))
    Assert-Rejects { Assert-WindowsUnsignedVendorProvenance -FilePath $appDll -StageDirectory $stage -QtProvenance $provenance } `
        "Se aceptó un ensamblado WinUI que no coincide con su inventario."

    Assert-Rejects { Get-WindowsQtProvenance -ManifestPath (Join-Path $root 'no-existe.sha256') } `
        "Se aceptó un inventario de Qt ausente."

    Write-Host "Procedencia de firma Windows: OK"
} finally {
    Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue
}
