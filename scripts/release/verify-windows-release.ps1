# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactDirectory,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedThumbprint,

    [string]$SignToolPath,

    [switch]$WriteManifest,

    [switch]$RequireEvidence,

    [switch]$RequireDualGui
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if (-not $IsWindows) {
    throw "La verificacion Authenticode oficial requiere Windows."
}

$authenticodeHelpers = Join-Path $PSScriptRoot 'windows-authenticode.ps1'
if (-not (Test-Path -LiteralPath $authenticodeHelpers -PathType Leaf)) {
    throw "Falta windows-authenticode.ps1."
}
. $authenticodeHelpers
$suiteLayoutHelpers = Join-Path $PSScriptRoot 'windows-suite-layout.ps1'
if (-not (Test-Path -LiteralPath $suiteLayoutHelpers -PathType Leaf)) {
    throw "Falta windows-suite-layout.ps1."
}
. $suiteLayoutHelpers
if ([string]::IsNullOrWhiteSpace($SignToolPath)) {
    $SignToolPath = Resolve-WindowsSignTool
} else {
    $SignToolPath = (Resolve-Path -LiteralPath $SignToolPath).Path
}

$expected = ($ExpectedThumbprint -replace '\s', '').ToUpperInvariant()
if ($expected -notmatch '^[0-9A-F]{40}$') {
    throw "ExpectedThumbprint debe ser una huella SHA-1 de 40 hexadecimales."
}
if (-not (Test-Path -LiteralPath $ArtifactDirectory -PathType Container)) {
    throw "No existe el directorio de artefactos: $ArtifactDirectory"
}
$ArtifactDirectory = (Resolve-Path -LiteralPath $ArtifactDirectory).Path

function Get-FileSha256 {
    param([Parameter(Mandatory = $true)][string]$Path)
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Get-VerifiedAuthenticodeRecord {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][bool]$RequireProjectSigner,
        [Parameter(Mandatory = $true)][string]$LogicalPath
    )

    $expectedSigner = if ($RequireProjectSigner) { $expected } else { $null }
    $signature = Assert-WindowsAuthenticodeFile `
        -Path $Path `
        -SignToolPath $SignToolPath `
        -ExpectedThumbprint $expectedSigner `
        -RequireSha256Rfc3161:$RequireProjectSigner

    $signerThumbprint = $signature.SignerCertificate.Thumbprint.ToUpperInvariant()
    $algorithms = Get-WindowsAuthenticodeAlgorithms -Path $Path

    return [ordered]@{
        path = $LogicalPath.Replace('\', '/')
        sha256 = Get-FileSha256 -Path $Path
        status = $signature.Status.ToString()
        signer_subject = $signature.SignerCertificate.Subject
        signer_thumbprint = $signerThumbprint
        timestamp_subject = $signature.TimeStamperCertificate.Subject
        timestamp_thumbprint = $signature.TimeStamperCertificate.Thumbprint.ToUpperInvariant()
        file_digest_algorithm = $algorithms.FileDigestAlgorithm
        timestamp_protocol = $algorithms.TimestampProtocol
        timestamp_digest_algorithm = $algorithms.TimestampDigestAlgorithm
    }
}

$version = (Get-Content -LiteralPath (Join-Path (Split-Path -Parent (Split-Path -Parent $PSScriptRoot)) "VERSION.txt") -Raw).Trim()
$expectedNames = @(
    "GrxFirma-$version-desktop-qml-windows-amd64.zip",
    "GrxFirma-$version-desktop-qml-windows-amd64-setup.exe",
    "GrxFirma-$version-windows-amd64.zip",
    "GrxFirma-$version-windows-amd64-setup.exe"
)
foreach ($name in $expectedNames) {
    $path = Join-Path $ArtifactDirectory $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Falta el artefacto Windows oficial: $name"
    }
}

$unexpectedContainers = @(
    Get-ChildItem -LiteralPath $ArtifactDirectory -File |
        Where-Object { $_.Extension -in @('.zip', '.exe') -and $_.Name -notin $expectedNames }
)
if ($unexpectedContainers.Count -gt 0) {
    throw "Artefactos ejecutables Windows inesperados: $($unexpectedContainers.Name -join ', ')"
}

$records = [System.Collections.Generic.List[object]]::new()
$containers = [System.Collections.Generic.List[object]]::new()

foreach ($setupName in $expectedNames | Where-Object { $_ -like '*-setup.exe' }) {
    $setupPath = Join-Path $ArtifactDirectory $setupName
    $records.Add((Get-VerifiedAuthenticodeRecord `
        -Path $setupPath `
        -RequireProjectSigner $true `
        -LogicalPath $setupName))
    $containers.Add([ordered]@{
        path = $setupName
        sha256 = Get-FileSha256 -Path $setupPath
        type = "authenticode-executable"
    })
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
foreach ($zipName in $expectedNames | Where-Object { $_ -like '*.zip' }) {
    $zipPath = Join-Path $ArtifactDirectory $zipName
    $archive = [System.IO.Compression.ZipFile]::OpenRead($zipPath)
    try {
        foreach ($entry in $archive.Entries) {
            $parts = $entry.FullName.Replace('\', '/').Split('/', [System.StringSplitOptions]::RemoveEmptyEntries)
            if ($entry.FullName.StartsWith('/') -or $entry.FullName -match '^[A-Za-z]:' -or $parts -contains '..') {
                throw "Entrada ZIP insegura en ${zipName}: $($entry.FullName)"
            }
        }
    } finally {
        $archive.Dispose()
    }

    $extractDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-verify-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Force -Path $extractDirectory | Out-Null
    try {
        Expand-Archive -LiteralPath $zipPath -DestinationPath $extractDirectory -Force
        if ($zipName -like "GrxFirma-$version-windows-*") {
            $stageRoots = @(
                Get-ChildItem -LiteralPath $extractDirectory -Directory -Force
            )
            if ($stageRoots.Count -ne 1) {
                throw "El ZIP Suite debe contener una unica raiz de etapa."
            }
            $componentDefines = @(Get-WindowsSuiteComponentDefines `
                -StageDirectory $stageRoots[0].FullName `
                -DefinePrefix "/D")
            if ($RequireDualGui -and (
                $componentDefines -notcontains "/DHAS_WINUI=1" -or
                $componentDefines -notcontains "/DHAS_QT=1"
            )) {
                throw "La Suite Windows oficial debe contener WinUI y Qt completos."
            }
            Assert-WindowsSuiteSharedBackend `
                -StageDirectory $stageRoots[0].FullName
        }
        $portableExecutables = @(
            Get-ChildItem -LiteralPath $extractDirectory -Recurse -File |
                Where-Object { $_.Extension -in @('.exe', '.dll') }
        )
        if ($portableExecutables.Count -eq 0) {
            throw "El ZIP no contiene binarios PE: $zipName"
        }
        $projectExecutableCount = 0
        $libraryCount = 0
        foreach ($portableExecutable in $portableExecutables | Sort-Object FullName) {
            $relative = [System.IO.Path]::GetRelativePath($extractDirectory, $portableExecutable.FullName)
            $isProjectExecutable = $portableExecutable.Name -match '^grxfirma(?:-[A-Za-z0-9-]+)?\.exe$'
            if ($isProjectExecutable) {
                $projectExecutableCount++
            }
            if ($portableExecutable.Extension -eq '.dll') {
                $libraryCount++
            }
            $records.Add((Get-VerifiedAuthenticodeRecord `
                -Path $portableExecutable.FullName `
                -RequireProjectSigner $isProjectExecutable `
                -LogicalPath ("$zipName/$relative")))
        }
        if ($projectExecutableCount -eq 0) {
            throw "El ZIP no contiene ejecutables propios de GrxFirma: $zipName"
        }
        if ($libraryCount -eq 0) {
            throw "El ZIP Qt no contiene bibliotecas DLL: $zipName"
        }
    } finally {
        Remove-Item -LiteralPath $extractDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }

    $containers.Add([ordered]@{
        path = $zipName
        sha256 = Get-FileSha256 -Path $zipPath
        type = "zip-with-authenticode-payloads"
    })
}

$manifest = [ordered]@{
    schema_version = 2
    expected_signer_thumbprint = $expected
    containers = @($containers | Sort-Object path)
    signatures = @($records | Sort-Object path)
}
if ($WriteManifest) {
    $manifestPath = Join-Path $ArtifactDirectory "WINDOWS-SIGNATURES.json"
    $manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $manifestPath -Encoding UTF8
}

if ($RequireEvidence) {
    $manifestPath = Join-Path $ArtifactDirectory "WINDOWS-SIGNATURES.json"
    $checksumPath = Join-Path $ArtifactDirectory "SHA256SUMS-windows.txt"
    foreach ($path in @($manifestPath, $checksumPath)) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Falta evidencia Windows obligatoria: $path"
        }
    }

    $stored = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ($stored.schema_version -ne 2 -or $stored.expected_signer_thumbprint -ne $expected) {
        throw "WINDOWS-SIGNATURES.json tiene version o firmante inesperados."
    }
    $storedContainers = @{}
    foreach ($item in $stored.containers) {
        $storedContainers[$item.path] = $item.sha256
    }
    foreach ($item in $manifest.containers) {
        if ($storedContainers[$item.path] -ne $item.sha256) {
            throw "Evidencia de contenedor Windows incoherente: $($item.path)"
        }
    }
    if ($storedContainers.Count -ne $manifest.containers.Count) {
        throw "WINDOWS-SIGNATURES.json contiene contenedores inesperados."
    }
    $storedSignatures = @{}
    foreach ($item in $stored.signatures) {
        $storedSignatures[$item.path] = $item
    }
    foreach ($item in $manifest.signatures) {
        $storedItem = $storedSignatures[$item.path]
        if ($null -eq $storedItem -or
            $storedItem.sha256 -ne $item.sha256 -or
            $storedItem.signer_thumbprint -ne $item.signer_thumbprint -or
            $storedItem.timestamp_thumbprint -ne $item.timestamp_thumbprint -or
            $storedItem.file_digest_algorithm -ne $item.file_digest_algorithm -or
            $storedItem.timestamp_protocol -ne $item.timestamp_protocol -or
            $storedItem.timestamp_digest_algorithm -ne
                $item.timestamp_digest_algorithm) {
            throw "Evidencia Authenticode incoherente: $($item.path)"
        }
    }
    if ($storedSignatures.Count -ne $manifest.signatures.Count) {
        throw "WINDOWS-SIGNATURES.json contiene firmas inesperadas."
    }

    $expectedChecksums = @($expectedNames + @('WINDOWS-SIGNATURES.json'))
    $seenChecksums = @{}
    foreach ($line in Get-Content -LiteralPath $checksumPath) {
        if ($line -notmatch '^([0-9a-fA-F]{64})  ([A-Za-z0-9][A-Za-z0-9._+-]*)$') {
            throw "Linea de checksum Windows no valida: $line"
        }
        $fileName = $Matches[2]
        if ($seenChecksums.ContainsKey($fileName)) {
            throw "Checksum Windows duplicado: $fileName"
        }
        $filePath = Join-Path $ArtifactDirectory $fileName
        if (-not (Test-Path -LiteralPath $filePath -PathType Leaf)) {
            throw "Checksum Windows referencia un fichero ausente: $fileName"
        }
        $actualHash = Get-FileSha256 -Path $filePath
        if ($actualHash -ne $Matches[1].ToLowerInvariant()) {
            throw "Checksum Windows incorrecto: $fileName"
        }
        $seenChecksums[$fileName] = $true
    }
    if (@($seenChecksums.Keys).Count -ne $expectedChecksums.Count -or
        @($expectedChecksums | Where-Object { -not $seenChecksums.ContainsKey($_) }).Count -gt 0) {
        throw "SHA256SUMS-windows.txt no cubre exactamente los artefactos Windows oficiales."
    }
}

Write-Output "Authenticode, timestamps y payloads ZIP Windows verificados."
