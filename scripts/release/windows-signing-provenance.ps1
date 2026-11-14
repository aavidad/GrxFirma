# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# G-01: lista cerrada de binarios ajenos sin firma que el release puede firmar
# con la identidad Authenticode oficial. Solo se firma un PE ajeno sin firma si
# su SHA-256 procede del propio build:
#   - del inventario de Qt calculado justo después de instalar Qt sin caché
#     (GRXFIRMA_QT_PROVENANCE_MANIFEST), con el mismo nombre de fichero, o
#   - del PUBLISH-MANIFEST.sha256 de la etapa WinUI que lo contiene, con la
#     misma ruta relativa.
# Cualquier otro PE sin firma detiene la publicación.

Set-StrictMode -Version Latest

function Get-WindowsProvenanceFileHash {
    param([Parameter(Mandatory = $true)][string]$Path)
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256 -ErrorAction Stop).Hash.ToLowerInvariant()
}

function Write-WindowsQtProvenanceManifest {
    param(
        [Parameter(Mandatory = $true)][string]$QtRoot,
        [Parameter(Mandatory = $true)][string]$OutputPath
    )

    $root = Get-Item -LiteralPath $QtRoot -ErrorAction Stop
    if (-not $root.PSIsContainer) {
        throw "QtRoot no es un directorio: $QtRoot"
    }
    $lines = [System.Collections.Generic.List[string]]::new()
    foreach ($file in Get-ChildItem -LiteralPath $root.FullName -Recurse -File -ErrorAction Stop |
            Where-Object { $_.Extension -in @('.dll', '.exe') } |
            Sort-Object FullName) {
        $relative = [System.IO.Path]::GetRelativePath($root.FullName, $file.FullName).Replace('\', '/')
        $lines.Add("$(Get-WindowsProvenanceFileHash -Path $file.FullName) *$relative")
    }
    if ($lines.Count -eq 0) {
        throw "La instalación de Qt no contiene bibliotecas: $QtRoot"
    }
    [System.IO.File]::WriteAllLines($OutputPath, $lines)
}

function Read-WindowsSha256Manifest {
    param([Parameter(Mandatory = $true)][string]$Path)

    $entries = [System.Collections.Generic.List[object]]::new()
    foreach ($line in Get-Content -LiteralPath $Path -ErrorAction Stop) {
        if ($line -notmatch '^([0-9a-fA-F]{64}) \*(.+)$') {
            throw "Línea no válida en el inventario $Path"
        }
        $relative = $Matches[2].Replace('\', '/')
        if ([System.IO.Path]::IsPathRooted($relative) -or @($relative -split '/') -contains '..') {
            throw "Ruta no permitida en el inventario ${Path}: $relative"
        }
        $entries.Add([pscustomobject]@{
            Hash = $Matches[1].ToLowerInvariant()
            RelativePath = $relative
        })
    }
    return $entries
}

function Get-WindowsQtProvenance {
    param([Parameter(Mandatory = $true)][string]$ManifestPath)

    if (-not (Test-Path -LiteralPath $ManifestPath -PathType Leaf)) {
        throw "Falta el inventario de procedencia de Qt: $ManifestPath"
    }
    $byHash = @{}
    foreach ($entry in Read-WindowsSha256Manifest -Path $ManifestPath) {
        $leaf = [System.IO.Path]::GetFileName($entry.RelativePath).ToLowerInvariant()
        if (-not $byHash.ContainsKey($entry.Hash)) {
            $byHash[$entry.Hash] = [System.Collections.Generic.HashSet[string]]::new()
        }
        [void]$byHash[$entry.Hash].Add($leaf)
    }
    if ($byHash.Count -eq 0) {
        throw "El inventario de procedencia de Qt está vacío: $ManifestPath"
    }
    return $byHash
}

function Test-WindowsWinUiPublishProvenance {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string]$StageDirectory,
        [Parameter(Mandatory = $true)][string]$FileHash
    )

    $stageRoot = [System.IO.Path]::GetFullPath($StageDirectory).TrimEnd('\', '/')
    $directory = [System.IO.Path]::GetDirectoryName([System.IO.Path]::GetFullPath($FilePath))
    while ($directory -and $directory.Length -ge $stageRoot.Length) {
        $manifest = Join-Path $directory 'PUBLISH-MANIFEST.sha256'
        if (Test-Path -LiteralPath $manifest -PathType Leaf) {
            $relative = [System.IO.Path]::GetRelativePath($directory, $FilePath).Replace('\', '/')
            foreach ($entry in Read-WindowsSha256Manifest -Path $manifest) {
                if ($entry.RelativePath -ieq $relative) {
                    return $entry.Hash -eq $FileHash
                }
            }
            return $false
        }
        if ($directory.TrimEnd('\', '/') -eq $stageRoot) {
            break
        }
        $directory = [System.IO.Path]::GetDirectoryName($directory)
    }
    return $false
}

function Assert-WindowsUnsignedVendorProvenance {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string]$StageDirectory,
        [Parameter(Mandatory = $true)][hashtable]$QtProvenance
    )

    $hash = Get-WindowsProvenanceFileHash -Path $FilePath
    $leaf = [System.IO.Path]::GetFileName($FilePath).ToLowerInvariant()
    if ($QtProvenance.ContainsKey($hash) -and $QtProvenance[$hash].Contains($leaf)) {
        return
    }
    if (Test-WindowsWinUiPublishProvenance -FilePath $FilePath -StageDirectory $StageDirectory -FileHash $hash) {
        return
    }
    throw "Binario sin firma fuera de la lista cerrada; no se firma: $FilePath ($hash)"
}
