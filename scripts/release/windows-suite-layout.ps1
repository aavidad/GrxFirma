# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Assert-WindowsReleaseStageSafe {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory
    )

    $stage = Get-Item -LiteralPath $StageDirectory -Force -ErrorAction Stop
    if (-not $stage.PSIsContainer) {
        throw "La etapa Windows no es un directorio: $StageDirectory"
    }
    if (($stage.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "La etapa Windows no puede ser un punto de reanálisis."
    }
    $nestedReparsePoint = Get-ChildItem `
        -LiteralPath $stage.FullName `
        -Force `
        -Recurse `
        -ErrorAction Stop |
        Where-Object {
            ($_.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
        } |
        Select-Object -First 1
    if ($null -ne $nestedReparsePoint) {
        throw "La etapa Windows contiene un punto de reanálisis: $($nestedReparsePoint.FullName)"
    }
    return $stage
}

function Get-WindowsSuiteComponentDefines {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory,
        [ValidateSet("/D", "-D")]
        [string]$DefinePrefix = "/D"
    )

    $stage = Assert-WindowsReleaseStageSafe `
        -StageDirectory $StageDirectory

    $defines = @()
    $winUiDirectory = Join-Path $stage.FullName "desktop-winui"
    if (Test-Path -LiteralPath $winUiDirectory -PathType Container) {
        foreach ($relativePath in @(
            "PUBLISH-MANIFEST.sha256",
            "app/grxfirma-winui.exe",
            "app/grxfirma-gui.exe"
        )) {
            if (-not (Test-Path `
                -LiteralPath (Join-Path $winUiDirectory $relativePath) `
                -PathType Leaf)) {
                throw "La etapa Suite declara WinUI pero falta $relativePath"
            }
        }
        $defines += "${DefinePrefix}HAS_WINUI=1"
    }

    $qtDirectory = Join-Path $stage.FullName "desktop-qt"
    if (Test-Path -LiteralPath $qtDirectory -PathType Container) {
        foreach ($relativePath in @(
            "grxfirma-gui-qml.exe",
            "grxfirma-gui.exe"
        )) {
            if (-not (Test-Path `
                -LiteralPath (Join-Path $qtDirectory $relativePath) `
                -PathType Leaf)) {
                throw "La etapa Suite declara Qt pero falta $relativePath"
            }
        }
        $defines += "${DefinePrefix}HAS_QT=1"
    }
    return $defines
}

function Get-WindowsReleaseSigningTargets {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory,
        [switch]$Suite
    )

    $stage = Assert-WindowsReleaseStageSafe `
        -StageDirectory $StageDirectory
    $excludedPaths = [System.Collections.Generic.HashSet[string]]::new(
        [System.StringComparer]::OrdinalIgnoreCase
    )
    if ($Suite) {
        foreach ($relativePath in @(
            "desktop-qt/grxfirma-gui.exe",
            "desktop-winui/app/grxfirma-gui.exe"
        )) {
            $candidate = Join-Path $stage.FullName $relativePath
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                [void]$excludedPaths.Add(
                    [System.IO.Path]::GetFullPath($candidate)
                )
            }
        }
    }

    return @(
        Get-ChildItem -LiteralPath $stage.FullName -Recurse -File |
            Where-Object {
                $_.Extension -in @('.exe', '.dll') -and
                    $_.Name -notlike 'vc_redist.*.exe' -and
                    (-not $excludedPaths.Contains($_.FullName))
            } |
            Sort-Object FullName
    )
}

function Get-WindowsWinUiManifestInventory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WinUiStageDirectory
    )

    $stage = Assert-WindowsReleaseStageSafe `
        -StageDirectory $WinUiStageDirectory

    $manifestPath = Join-Path $stage.FullName "PUBLISH-MANIFEST.sha256"
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "Falta PUBLISH-MANIFEST.sha256 en la etapa WinUI."
    }

    $declared = @{}
    foreach ($line in Get-Content -LiteralPath $manifestPath -ErrorAction Stop) {
        if ($line -notmatch '^([0-9a-fA-F]{64}) \*(.+)$') {
            throw "PUBLISH-MANIFEST.sha256 contiene una línea no válida."
        }
        $relativePath = $Matches[2].Replace('\', '/')
        if ([System.IO.Path]::IsPathRooted($relativePath) -or
            @($relativePath -split '[/\\]') -contains "..") {
            throw "Ruta no permitida en PUBLISH-MANIFEST.sha256: $relativePath"
        }
        if ($declared.ContainsKey($relativePath)) {
            throw "Ruta duplicada en PUBLISH-MANIFEST.sha256: $relativePath"
        }
        $declared[$relativePath] = $Matches[1].ToLowerInvariant()
    }

    $actual = @{}
    foreach ($file in Get-ChildItem `
        -LiteralPath $stage.FullName `
        -File `
        -Force `
        -Recurse `
        -ErrorAction Stop) {
        if ($file.FullName -eq $manifestPath) {
            continue
        }
        $relativePath = [System.IO.Path]::GetRelativePath(
            $stage.FullName,
            $file.FullName
        ).Replace('\', '/')
        if ($actual.ContainsKey($relativePath)) {
            throw "Ruta física duplicada en la etapa WinUI: $relativePath"
        }
        $actual[$relativePath] = $file
    }
    foreach ($relativePath in $declared.Keys) {
        if (-not $actual.ContainsKey($relativePath)) {
            throw "El manifiesto WinUI referencia un fichero ausente: $relativePath"
        }
    }
    foreach ($relativePath in $actual.Keys) {
        if (-not $declared.ContainsKey($relativePath)) {
            throw "Fichero WinUI no inventariado: $relativePath"
        }
    }

    return [pscustomobject]@{
        StageDirectory = $stage.FullName
        ManifestPath = $manifestPath
        Declared = $declared
        Actual = $actual
    }
}

function Assert-WindowsWinUiPublishManifest {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WinUiStageDirectory
    )

    $inventory = Get-WindowsWinUiManifestInventory `
        -WinUiStageDirectory $WinUiStageDirectory
    foreach ($relativePath in $inventory.Actual.Keys) {
        $actualHash = (Get-FileHash `
            -LiteralPath $inventory.Actual[$relativePath].FullName `
            -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualHash -ne $inventory.Declared[$relativePath]) {
            throw "Checksum inesperado en la etapa WinUI: $relativePath"
        }
    }
}

function Update-WindowsWinUiPublishManifest {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WinUiStageDirectory
    )

    $inventory = Get-WindowsWinUiManifestInventory `
        -WinUiStageDirectory $WinUiStageDirectory
    $updatedLines = foreach ($relativePath in @(
        $inventory.Actual.Keys | Sort-Object
    )) {
        $hash = (Get-FileHash `
            -LiteralPath $inventory.Actual[$relativePath].FullName `
            -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash *$relativePath"
    }
    [System.IO.File]::WriteAllLines(
        $inventory.ManifestPath,
        $updatedLines,
        [System.Text.UTF8Encoding]::new($false)
    )
    Assert-WindowsWinUiPublishManifest `
        -WinUiStageDirectory $WinUiStageDirectory
}

function Assert-WindowsSuiteSharedBackend {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory
    )

    $stage = Assert-WindowsReleaseStageSafe `
        -StageDirectory $StageDirectory
    $canonical = Join-Path $stage.FullName "grxfirma-gui.exe"
    if (-not (Test-Path -LiteralPath $canonical -PathType Leaf)) {
        throw "Falta el launcher canónico de la Suite Windows."
    }
    $canonicalHash = (Get-FileHash `
        -LiteralPath $canonical `
        -Algorithm SHA256).Hash
    foreach ($relativePath in @(
        "desktop-qt/grxfirma-gui.exe",
        "desktop-winui/app/grxfirma-gui.exe"
    )) {
        $copy = Join-Path $stage.FullName $relativePath
        if ((Test-Path -LiteralPath (Split-Path -Parent $copy) -PathType Container) -and
            (-not (Test-Path -LiteralPath $copy -PathType Leaf))) {
            throw "Falta la copia de validación del launcher compartido: $relativePath"
        }
        if ((Test-Path -LiteralPath $copy -PathType Leaf) -and
            (Get-FileHash -LiteralPath $copy -Algorithm SHA256).Hash -ne
                $canonicalHash) {
            throw "La copia del launcher compartido diverge: $relativePath"
        }
    }
    $winUiStage = Join-Path $stage.FullName "desktop-winui"
    if (Test-Path -LiteralPath $winUiStage -PathType Container) {
        Assert-WindowsWinUiPublishManifest `
            -WinUiStageDirectory $winUiStage
    }
}

function Sync-WindowsSuiteSharedBackend {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory
    )

    $stage = Assert-WindowsReleaseStageSafe `
        -StageDirectory $StageDirectory
    $canonical = Join-Path $stage.FullName "grxfirma-gui.exe"
    if (-not (Test-Path -LiteralPath $canonical -PathType Leaf)) {
        throw "Falta el launcher canónico de la Suite Windows."
    }

    $qtBackend = Join-Path $stage.FullName "desktop-qt/grxfirma-gui.exe"
    if (Test-Path -LiteralPath (Split-Path -Parent $qtBackend) -PathType Container) {
        Copy-Item -LiteralPath $canonical -Destination $qtBackend -Force
    }

    $winUiStage = Join-Path $stage.FullName "desktop-winui"
    if (Test-Path -LiteralPath $winUiStage -PathType Container) {
        $winUiBackend = Join-Path $winUiStage "app/grxfirma-gui.exe"
        Copy-Item -LiteralPath $canonical -Destination $winUiBackend -Force
        Update-WindowsWinUiPublishManifest `
            -WinUiStageDirectory $winUiStage
    }
    Assert-WindowsSuiteSharedBackend -StageDirectory $stage.FullName
}
