# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [string]$ProjectPath = "",
    [string]$OutputDirectory = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

$RepositoryRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
& python (Join-Path $RepositoryRoot "scripts/comprobar-novedades.py")
if ($LASTEXITCODE -ne 0) {
    throw "Compilación detenida: complete la sección de la versión actual en docs/NOVEDADES.md."
}
. (Join-Path $PSScriptRoot "reproducible-build.ps1")
$SourceDateEpoch = Initialize-GrxFirmaReproducibleBuild -RootDirectory $RepositoryRoot

$Script:ExpectedDotNetSdk = "10.0.302"
$Script:ExpectedWindowsAppSdk = "2.3.1"
$Script:RuntimeIdentifier = "win-x64"
$Script:Architecture = "amd64"
$Script:StageName = "GrxFirma-$((Get-Content -LiteralPath (Join-Path $RepositoryRoot 'VERSION.txt') -Raw).Trim())-desktop-winui-windows-amd64"

function Assert-GrxFirmaPathWithinRoot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [switch]$AllowRoot
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path).TrimEnd('\', '/')
    $fullRoot = [System.IO.Path]::GetFullPath($Root).TrimEnd('\', '/')
    if ($AllowRoot -and [string]::Equals(
        $fullPath,
        $fullRoot,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        return $fullPath
    }
    $rootPrefix = $fullRoot + [System.IO.Path]::DirectorySeparatorChar
    if (-not $fullPath.StartsWith(
        $rootPrefix,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        throw "La ruta queda fuera del directorio autorizado: $fullPath"
    }
    return $fullPath
}

function Assert-GrxFirmaNoReparsePoints {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $root = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    $items = @($root)
    if ($root.PSIsContainer) {
        $items += @(Get-ChildItem -LiteralPath $root.FullName -Force -Recurse)
    }
    foreach ($item in $items) {
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "No se permiten enlaces ni reparse points: $($item.FullName)"
        }
    }
}

function Resolve-GrxFirmaWinUiProject {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [string]$Candidate
    )

    $sourceRoot = Join-Path $Root "cmd/gui-winui"
    if (-not (Test-Path -LiteralPath $sourceRoot -PathType Container)) {
        throw (
            "No existe cmd/gui-winui. Espera a que se cree el proyecto WinUI " +
            "antes de ejecutar este empaquetador."
        )
    }
    Assert-GrxFirmaNoReparsePoints -Path $sourceRoot

    if ([string]::IsNullOrWhiteSpace($Candidate)) {
        $preferredProject = Join-Path `
            $sourceRoot `
            "src/GrxFirma.WinUI/GrxFirma.WinUI.csproj"
        if (Test-Path -LiteralPath $preferredProject -PathType Leaf) {
            $project = Get-Item -LiteralPath $preferredProject -Force
        } else {
            $projects = @(
                Get-ChildItem -LiteralPath $sourceRoot -File -Recurse -Filter "*.csproj" |
                    Where-Object {
                        $_.FullName -notmatch '[\\/](?:tests?|samples?)[\\/]'
                    }
            )
            if ($projects.Count -ne 1) {
                throw (
                    "No existe el proyecto de aplicacion esperado y se encontraron " +
                    "$($projects.Count) candidatos fuera de tests. Usa -ProjectPath."
                )
            }
            $project = $projects[0]
        }
    } else {
        $candidatePath = if ([System.IO.Path]::IsPathRooted($Candidate)) {
            $Candidate
        } else {
            Join-Path $Root $Candidate
        }
        $project = Get-Item -LiteralPath $candidatePath -Force -ErrorAction Stop
    }

    if ($project.PSIsContainer -or $project.Extension -ine ".csproj") {
        throw "El proyecto WinUI debe ser un fichero .csproj: $($project.FullName)"
    }
    [void](Assert-GrxFirmaPathWithinRoot -Path $project.FullName -Root $sourceRoot)
    if (($project.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "El proyecto WinUI no puede ser un enlace: $($project.FullName)"
    }
    return $project.FullName
}

function Assert-GrxFirmaPinnedDotNet {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [Parameter(Mandatory = $true)]
        [string]$ExpectedVersion
    )

    $globalJsonPath = Join-Path $Root "global.json"
    if (-not (Test-Path -LiteralPath $globalJsonPath -PathType Leaf)) {
        throw "Falta global.json; no se permite una toolchain .NET sin fijar."
    }
    $globalJson = Get-Content -LiteralPath $globalJsonPath -Raw | ConvertFrom-Json
    if ($globalJson.sdk.version -ne $ExpectedVersion) {
        throw "global.json debe fijar .NET SDK $ExpectedVersion."
    }
    if ($globalJson.sdk.rollForward -ne "disable") {
        throw "global.json debe usar rollForward=disable para el build WinUI."
    }
    if ($globalJson.sdk.allowPrerelease -ne $false) {
        throw "global.json no debe permitir SDK .NET prerelease."
    }

    $dotnet = Get-Command dotnet -CommandType Application -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -eq $dotnet) {
        throw "No se encontro dotnet. Instala Microsoft.DotNet.SDK.10."
    }
    $actualVersion = (& $dotnet.Source --version 2>$null).Trim()
    if ($LASTEXITCODE -ne 0 -or $actualVersion -ne $ExpectedVersion) {
        throw (
            "SDK .NET inesperado. Se requiere $ExpectedVersion y dotnet resolvio " +
            "'$actualVersion'."
        )
    }
    return $dotnet.Source
}

function Assert-GrxFirmaLockedRestore {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Project
    )

    $projectDirectory = Split-Path -Parent $Project
    $lockFiles = @(
        Get-ChildItem -LiteralPath $projectDirectory -File -Filter "packages.lock.json"
    )
    if ($lockFiles.Count -ne 1) {
        throw (
            "El proyecto WinUI debe versionar exactamente un packages.lock.json junto " +
            "al .csproj para permitir dotnet restore --locked-mode."
        )
    }
    Assert-GrxFirmaNoReparsePoints -Path $lockFiles[0].FullName
}

function Get-GrxFirmaProjectBuildDirectories {
    param(
        [Parameter(Mandatory = $true)]
        [string]$SourceRoot
    )

    if (-not (Test-Path -LiteralPath $SourceRoot -PathType Container)) {
        return @()
    }
    return @(
        Get-ChildItem -LiteralPath $SourceRoot -Directory -Force -Recurse |
            Where-Object { $_.Name -in @("bin", "obj") } |
            ForEach-Object { $_.FullName }
    )
}

function Get-GrxFirmaPeMachine {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $stream = [System.IO.File]::Open(
        $Path,
        [System.IO.FileMode]::Open,
        [System.IO.FileAccess]::Read,
        [System.IO.FileShare]::Read
    )
    try {
        $reader = [System.IO.BinaryReader]::new($stream)
        try {
            if ($stream.Length -lt 64 -or $reader.ReadUInt16() -ne 0x5A4D) {
                throw "El fichero no contiene una cabecera PE valida: $Path"
            }
            $stream.Position = 0x3C
            $peOffset = $reader.ReadUInt32()
            if ($peOffset -gt ($stream.Length - 6)) {
                throw "Offset PE fuera de rango: $Path"
            }
            $stream.Position = $peOffset
            if ($reader.ReadUInt32() -ne 0x00004550) {
                throw "Firma PE invalida: $Path"
            }
            return $reader.ReadUInt16()
        } finally {
            $reader.Dispose()
        }
    } finally {
        $stream.Dispose()
    }
}

function Assert-GrxFirmaStageSecrets {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory
    )

    $forbiddenName = '(?i)(^|[._-])(id_rsa|id_dsa|id_ecdsa|id_ed25519|credentials?|secrets?)([._-]|$)'
    $forbiddenExtension = @(
        ".env", ".key", ".pem", ".pfx", ".p12", ".jks", ".keystore", ".snk",
        ".user", ".suo"
    )
    $textExtensions = @(
        "", ".bat", ".cmd", ".config", ".ini", ".json", ".log", ".md", ".ps1",
        ".props", ".targets", ".txt", ".xml", ".yaml", ".yml"
    )
    $contentMarkers = @(
        "CLIENT_RANDOM ",
        "CLIENT_HANDSHAKE_TRAFFIC_SECRET ",
        "SERVER_HANDSHAKE_TRAFFIC_SECRET ",
        "CLIENT_TRAFFIC_SECRET_0 ",
        "SERVER_TRAFFIC_SECRET_0 ",
        "EXPORTER_SECRET ",
        ("-----BEGIN " + "PRIVATE KEY-----"),
        ("-----BEGIN ENCRYPTED " + "PRIVATE KEY-----"),
        ("-----BEGIN RSA " + "PRIVATE KEY-----"),
        ("-----BEGIN EC " + "PRIVATE KEY-----"),
        ("-----BEGIN OPENSSH " + "PRIVATE KEY-----")
    )

    foreach ($file in Get-ChildItem -LiteralPath $StageDirectory -File -Force -Recurse) {
        if ($file.Name -ieq ".env" -or
            $file.Name.StartsWith(".env.", [System.StringComparison]::OrdinalIgnoreCase) -or
            $file.Name -match $forbiddenName -or
            $forbiddenExtension -contains $file.Extension.ToLowerInvariant()) {
            throw "El stage contiene un fichero potencialmente sensible: $($file.FullName)"
        }
        if ($file.Length -gt 1048576 -or
            $textExtensions -notcontains $file.Extension.ToLowerInvariant()) {
            continue
        }
        $bytes = [System.IO.File]::ReadAllBytes($file.FullName)
        $text = [System.Text.Encoding]::ASCII.GetString($bytes)
        foreach ($marker in $contentMarkers) {
            if ($text.Contains($marker)) {
                throw "El stage contiene material secreto o un key log TLS: $($file.FullName)"
            }
        }
    }
}

function Assert-GrxFirmaWinUiPublication {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory,
        [string]$ExpectedWindowsAppSdk = "2.3.1"
    )

    $stage = Get-Item -LiteralPath $StageDirectory -Force -ErrorAction Stop
    if (-not $stage.PSIsContainer) {
        throw "El stage WinUI no es un directorio: $StageDirectory"
    }
    Assert-GrxFirmaNoReparsePoints -Path $stage.FullName
    Assert-GrxFirmaStageSecrets -StageDirectory $stage.FullName

    $appDirectory = Join-Path $stage.FullName "app"
    if (-not (Test-Path -LiteralPath $appDirectory -PathType Container)) {
        throw "El stage WinUI debe contener el directorio app."
    }
    foreach ($required in @(
        "README_DESKTOP_WINUI_WINDOWS.md",
        "VERSION.txt"
    )) {
        if (-not (Test-Path -LiteralPath (Join-Path $stage.FullName $required) -PathType Leaf)) {
            throw "Falta $required en el stage WinUI."
        }
    }
    $backend = Join-Path $appDirectory "grxfirma-gui.exe"
    if (-not (Test-Path -LiteralPath $backend -PathType Leaf)) {
        throw "Falta el backend IPC grxfirma-gui.exe en app/."
    }
    $userGuide = Join-Path $appDirectory "help/guia-usuario.txt"
    if (-not (Test-Path -LiteralPath $userGuide -PathType Leaf)) {
        throw "Falta la guia local app/help/guia-usuario.txt en la publicacion WinUI."
    }
    if ((Get-Item -LiteralPath $userGuide -Force).Length -lt 256) {
        throw "La guia local app/help/guia-usuario.txt esta vacia o incompleta."
    }
    $releaseNotes = Join-Path $appDirectory "help/NOVEDADES.md"
    if (-not (Test-Path -LiteralPath $releaseNotes -PathType Leaf) -or
        (Get-Item -LiteralPath $releaseNotes -Force).Length -gt 65536) {
        throw "Falta app/help/NOVEDADES.md o supera 64 KiB en la publicacion WinUI."
    }
    $runtimeConfigs = @(
        Get-ChildItem -LiteralPath $appDirectory -File -Filter "*.runtimeconfig.json"
    )
    if ($runtimeConfigs.Count -ne 1) {
        throw (
            "La publicacion debe contener exactamente un runtimeconfig de la app WinUI; " +
            "se encontraron $($runtimeConfigs.Count)."
        )
    }
    $applicationName = $runtimeConfigs[0].Name.Substring(
        0,
        $runtimeConfigs[0].Name.Length - ".runtimeconfig.json".Length
    )
    $frontend = Join-Path $appDirectory "$applicationName.exe"
    $managedEntryPoint = Join-Path $appDirectory "$applicationName.dll"
    $depsPath = Join-Path $appDirectory "$applicationName.deps.json"
    foreach ($requiredPath in @($frontend, $managedEntryPoint, $depsPath)) {
        if (-not (Test-Path -LiteralPath $requiredPath -PathType Leaf)) {
            throw "Publicacion WinUI incompleta; falta $requiredPath"
        }
    }
    $applicationResources = Join-Path $appDirectory "$applicationName.pri"
    if (-not (Test-Path -LiteralPath $applicationResources -PathType Leaf)) {
        throw (
            "Falta app/$applicationName.pri; WinUI no puede cargar los recursos " +
            "XAML de una publicacion unpackaged sin este indice."
        )
    }
    if ((Get-Item -LiteralPath $applicationResources -Force).Length -eq 0) {
        throw "app/$applicationName.pri esta vacio."
    }

    $runtimeConfig = Get-Content -LiteralPath $runtimeConfigs[0].FullName -Raw |
        ConvertFrom-Json
    $runtimePropertyNames = @($runtimeConfig.runtimeOptions.PSObject.Properties.Name)
    if ($runtimePropertyNames -contains "framework" -or
        $runtimePropertyNames -contains "frameworks") {
        throw "La publicacion WinUI es framework-dependent, no self-contained."
    }

    foreach ($runtimeFile in @(
        "coreclr.dll",
        "hostfxr.dll",
        "hostpolicy.dll",
        "Microsoft.UI.Xaml.dll",
        "Microsoft.WindowsAppRuntime.dll"
    )) {
        if (-not (Test-Path -LiteralPath (Join-Path $appDirectory $runtimeFile) -PathType Leaf)) {
            throw "La publicacion self-contained no incluye $runtimeFile."
        }
    }

    $deps = Get-Content -LiteralPath $depsPath -Raw | ConvertFrom-Json
    $runtimeTargetName = [string]$deps.runtimeTarget.name
    if (-not $runtimeTargetName.EndsWith(
        "/win-x64",
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        throw "RID inesperado en deps.json: $runtimeTargetName"
    }
    $libraries = @($deps.libraries.PSObject.Properties.Name)
    if ($libraries -notcontains "Microsoft.WindowsAppSDK/$ExpectedWindowsAppSdk") {
        throw (
            "La publicacion no contiene Microsoft.WindowsAppSDK/$ExpectedWindowsAppSdk " +
            "fijado."
        )
    }

    foreach ($pePath in @($frontend, $backend)) {
        $machine = Get-GrxFirmaPeMachine -Path $pePath
        if ($machine -ne 0x8664) {
            throw "El ejecutable no es PE x64 (AMD64): $pePath (machine=0x$('{0:X4}' -f $machine))"
        }
    }

    $packagedArtifacts = @(
        Get-ChildItem -LiteralPath $stage.FullName -File -Recurse |
            Where-Object { $_.Extension -in @(".appx", ".msix", ".msixbundle") }
    )
    if ($packagedArtifacts.Count -gt 0) {
        throw "El stage unpackaged contiene un paquete MSIX/AppX inesperado."
    }
}

function Write-GrxFirmaPublishManifest {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory
    )

    $manifestPath = Join-Path $StageDirectory "PUBLISH-MANIFEST.sha256"
    $stagePrefix = [System.IO.Path]::GetFullPath($StageDirectory).TrimEnd('\', '/') +
        [System.IO.Path]::DirectorySeparatorChar
    $lines = @()
    foreach ($file in @(
        Get-ChildItem -LiteralPath $StageDirectory -File -Force -Recurse |
            Where-Object { $_.FullName -ne $manifestPath } |
            Sort-Object FullName
    )) {
        $fullPath = [System.IO.Path]::GetFullPath($file.FullName)
        if (-not $fullPath.StartsWith(
            $stagePrefix,
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
            throw "El manifiesto intento inventariar un artefacto fuera del stage: $fullPath"
        }
        $relativePath = $fullPath.Substring($stagePrefix.Length).Replace('\', '/')
        if ([System.IO.Path]::IsPathRooted($relativePath) -or
            $relativePath.Split('/') -contains "..") {
            throw "Ruta no portable en el manifiesto WinUI: $relativePath"
        }
        $hash = (Get-FileHash -LiteralPath $fullPath -Algorithm SHA256).
            Hash.ToLowerInvariant()
        $lines += ("{0} *{1}" -f $hash, $relativePath)
    }
    if ($lines.Count -eq 0) {
        throw "No hay ficheros que inventariar en el stage WinUI."
    }
    [System.IO.File]::WriteAllLines(
        $manifestPath,
        $lines,
        [System.Text.UTF8Encoding]::new($false)
    )
}

function Assert-GrxFirmaPublishManifest {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory
    )

    $stage = Get-Item -LiteralPath $StageDirectory -Force -ErrorAction Stop
    if (-not $stage.PSIsContainer) {
        throw "El stage del manifiesto no es un directorio: $StageDirectory"
    }
    $manifestPath = Join-Path $stage.FullName "PUBLISH-MANIFEST.sha256"
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "Falta PUBLISH-MANIFEST.sha256 en el stage WinUI."
    }

    $stagePrefix = $stage.FullName.TrimEnd('\', '/') +
        [System.IO.Path]::DirectorySeparatorChar
    $expectedPaths = @{}
    foreach ($line in Get-Content -LiteralPath $manifestPath) {
        if ($line -notmatch '^([0-9a-f]{64}) \*(.+)$') {
            throw "Linea no valida en PUBLISH-MANIFEST.sha256: $line"
        }
        $expectedHash = $matches[1]
        $relativePath = $matches[2]
        if ([System.IO.Path]::IsPathRooted($relativePath) -or
            $relativePath.Replace('\', '/').Split('/') -contains "..") {
            throw "Ruta no segura en PUBLISH-MANIFEST.sha256: $relativePath"
        }
        $portablePath = $relativePath.Replace('\', '/')
        if ($expectedPaths.ContainsKey($portablePath)) {
            throw "Ruta duplicada en PUBLISH-MANIFEST.sha256: $portablePath"
        }
        $path = [System.IO.Path]::GetFullPath(
            (Join-Path $stage.FullName $relativePath)
        )
        if (-not $path.StartsWith(
            $stagePrefix,
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
            throw "El manifiesto referencia un artefacto fuera del stage: $relativePath"
        }
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "El manifiesto referencia un fichero inexistente: $relativePath"
        }
        $item = Get-Item -LiteralPath $path -Force
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "El manifiesto referencia un reparse point: $relativePath"
        }
        $actualHash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).
            Hash.ToLowerInvariant()
        if ($actualHash -ne $expectedHash) {
            throw "Checksum inesperado en el stage WinUI: $relativePath"
        }
        $expectedPaths[$portablePath] = $true
    }
    if ($expectedPaths.Count -eq 0) {
        throw "PUBLISH-MANIFEST.sha256 esta vacio."
    }

    $actualPaths = @(
        Get-ChildItem -LiteralPath $stage.FullName -File -Force -Recurse |
            Where-Object { $_.FullName -ne $manifestPath } |
            ForEach-Object {
                $_.FullName.Substring($stagePrefix.Length).Replace('\', '/')
            }
    )
    foreach ($actualPath in $actualPaths) {
        if (-not $expectedPaths.ContainsKey($actualPath)) {
            throw "Fichero no inventariado en el stage WinUI: $actualPath"
        }
    }
    if ($actualPaths.Count -ne $expectedPaths.Count) {
        throw "El numero de ficheros no coincide con PUBLISH-MANIFEST.sha256."
    }
}

function Write-GrxFirmaOuterArtifactMetadata {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Directory,
        [Parameter(Mandatory = $true)]
        [string]$ZipPath
    )

    $zip = Get-Item -LiteralPath $ZipPath -ErrorAction Stop
    $hash = (Get-FileHash -LiteralPath $zip.FullName -Algorithm SHA256).
        Hash.ToLowerInvariant()
    [System.IO.File]::WriteAllText(
        (Join-Path $Directory "SHA256SUMS.txt"),
        ("{0} *{1}`n" -f $hash, $zip.Name),
        [System.Text.Encoding]::ASCII
    )
    $report = @(
        "# Windows Desktop WinUI Artifacts",
        "",
        "| File | Size (bytes) | SHA-256 |",
        "| --- | ---: | --- |",
        ("| `{0}` | {1} | `{2}` |" -f $zip.Name, $zip.Length, $hash),
        ""
    ) -join "`n"
    [System.IO.File]::WriteAllText(
        (Join-Path $Directory "ARTIFACTS.md"),
        $report,
        [System.Text.UTF8Encoding]::new($false)
    )
}

function Suspend-GrxFirmaSensitiveEnvironment {
    $saved = @{}
    foreach ($entry in Get-ChildItem Env:) {
        if ($entry.Name -match '(?i)(password|passwd|secret|token|keylog|private[_-]?key|credential|pfx)') {
            $saved[$entry.Name] = $entry.Value
            Remove-Item -LiteralPath ("Env:" + $entry.Name) -ErrorAction SilentlyContinue
        }
    }
    return $saved
}

function Restore-GrxFirmaEnvironment {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$Values
    )

    foreach ($name in $Values.Keys) {
        Set-Item -LiteralPath ("Env:" + $name) -Value $Values[$name]
    }
}

$winUiProject = Resolve-GrxFirmaWinUiProject `
    -Root $RepositoryRoot `
    -Candidate $ProjectPath
Assert-GrxFirmaLockedRestore -Project $winUiProject
$dotnetCommand = Assert-GrxFirmaPinnedDotNet `
    -Root $RepositoryRoot `
    -ExpectedVersion $Script:ExpectedDotNetSdk

$winUiSourceRoot = Join-Path $RepositoryRoot "cmd/gui-winui"
$preexistingBuildDirectories = @(Get-GrxFirmaProjectBuildDirectories -SourceRoot $winUiSourceRoot)
if ($preexistingBuildDirectories.Count -gt 0) {
    throw (
        "Hay directorios bin/obj fuera del staging WinUI. Limpialos antes del build:`n  - " +
        ($preexistingBuildDirectories -join "`n  - ")
    )
}

$releaseRoot = Join-Path $RepositoryRoot "release"
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $releaseRoot "windows-desktop-winui"
} elseif (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $RepositoryRoot $OutputDirectory
}
$OutputDirectory = Assert-GrxFirmaPathWithinRoot `
    -Path $OutputDirectory `
    -Root $releaseRoot

$stageDirectory = Join-Path $OutputDirectory $Script:StageName
$zipPath = Join-Path $OutputDirectory "$($Script:StageName).zip"
$temporaryRoot = [System.IO.Path]::GetTempPath()
$workDirectory = Join-Path $temporaryRoot "grxfirma-winui-amd64"
$candidateStage = Join-Path $workDirectory $Script:StageName
$publishDirectory = Join-Path $workDirectory "publish"
$buildArtifactsDirectory = Join-Path $workDirectory "artifacts"
$nugetDirectory = Join-Path $workDirectory "nuget-packages"
$dotnetHome = Join-Path $workDirectory "dotnet-home"
$nugetHttpCache = Join-Path $workDirectory "nuget-http-cache"
$goCache = Join-Path $workDirectory "go-cache"

New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
if (Test-Path -LiteralPath $workDirectory) {
    Assert-GrxFirmaNoReparsePoints -Path $workDirectory
    throw (
        "Ya existe el workspace WinUI exclusivo: $workDirectory. " +
        "Puede haber otro build activo o una limpieza pendiente."
    )
}
foreach ($oldArtifact in @(
    $stageDirectory,
    $zipPath,
    (Join-Path $OutputDirectory "SHA256SUMS.txt"),
    (Join-Path $OutputDirectory "ARTIFACTS.md")
)) {
    if (-not (Test-Path -LiteralPath $oldArtifact)) {
        continue
    }
    [void](Assert-GrxFirmaPathWithinRoot -Path $oldArtifact -Root $OutputDirectory)
    Assert-GrxFirmaNoReparsePoints -Path $oldArtifact
    Remove-Item -LiteralPath $oldArtifact -Recurse -Force
}

$environmentNames = @(
    "DOTNET_CLI_TELEMETRY_OPTOUT",
    "DOTNET_NOLOGO",
    "DOTNET_MULTILEVEL_LOOKUP",
    "DOTNET_CLI_HOME",
    "NUGET_PACKAGES",
    "NUGET_HTTP_CACHE_PATH",
    "GOCACHE",
    "GOOS",
    "GOARCH",
    "CGO_ENABLED"
)
$savedEnvironment = @{}
foreach ($name in $environmentNames) {
    $item = Get-Item -LiteralPath ("Env:" + $name) -ErrorAction SilentlyContinue
    $savedEnvironment[$name] = if ($null -eq $item) { $null } else { $item.Value }
}
$sensitiveEnvironment = Suspend-GrxFirmaSensitiveEnvironment

try {
    New-Item -ItemType Directory -Path $workDirectory | Out-Null
    foreach ($directory in @(
        $candidateStage,
        (Join-Path $candidateStage "app"),
        $publishDirectory,
        $buildArtifactsDirectory,
        $nugetDirectory,
        $dotnetHome,
        $nugetHttpCache,
        $goCache
    )) {
        New-Item -ItemType Directory -Force -Path $directory | Out-Null
    }

    $env:DOTNET_CLI_TELEMETRY_OPTOUT = "1"
    $env:DOTNET_NOLOGO = "1"
    $env:DOTNET_MULTILEVEL_LOOKUP = "0"
    $env:DOTNET_CLI_HOME = $dotnetHome
    $env:NUGET_PACKAGES = $nugetDirectory
    $env:NUGET_HTTP_CACHE_PATH = $nugetHttpCache
    $env:GOCACHE = $goCache

    $commonMsBuildProperties = @(
        "-p:UseArtifactsOutput=true",
        "-p:ArtifactsPath=$buildArtifactsDirectory",
        "-p:ContinuousIntegrationBuild=true",
        "-p:Deterministic=true",
        "-p:PathMap=$RepositoryRoot=/_/"
    )

    Write-Output "Restaurando WinUI con lockfile y .NET SDK $($Script:ExpectedDotNetSdk)..."
    & $dotnetCommand restore `
        $winUiProject `
        --runtime $Script:RuntimeIdentifier `
        --locked-mode `
        --disable-parallel `
        --packages $nugetDirectory `
        @commonMsBuildProperties
    if ($LASTEXITCODE -ne 0) {
        throw "dotnet restore --locked-mode fallo con codigo $LASTEXITCODE."
    }

    Write-Output "Publicando WinUI 3 self-contained para win-x64..."
    & $dotnetCommand publish `
        $winUiProject `
        --configuration Release `
        --runtime $Script:RuntimeIdentifier `
        --self-contained true `
        --no-restore `
        --output $publishDirectory `
        "-p:WindowsPackageType=None" `
        "-p:EnableMsixTooling=true" `
        "-p:WindowsAppSDKSelfContained=true" `
        "-p:SelfContained=true" `
        "-p:PublishSingleFile=false" `
        "-p:PublishTrimmed=false" `
        "-p:PublishReadyToRun=false" `
        "-p:DebugSymbols=false" `
        "-p:DebugType=None" `
        @commonMsBuildProperties
    if ($LASTEXITCODE -ne 0) {
        throw "dotnet publish WinUI fallo con codigo $LASTEXITCODE."
    }
    Assert-GrxFirmaNoReparsePoints -Path $publishDirectory
    foreach ($publishedItem in Get-ChildItem -LiteralPath $publishDirectory -Force) {
        Copy-Item `
            -LiteralPath $publishedItem.FullName `
            -Destination (Join-Path $candidateStage "app") `
            -Recurse `
            -Force
    }

    $versionFile = Join-Path $RepositoryRoot "VERSION.txt"
    if (Test-Path -LiteralPath $versionFile -PathType Leaf) {
        $version = (Get-Content -LiteralPath $versionFile -Raw).Trim()
    } else {
        $version = (& git -C $RepositoryRoot describe --tags --always --dirty 2>$null)
        if ([string]::IsNullOrWhiteSpace($version)) {
            $version = "dev"
        }
    }

    Write-Output "Compilando backend IPC Go para Windows amd64..."
    $env:GOOS = "windows"
    $env:GOARCH = $Script:Architecture
    $env:CGO_ENABLED = "0"
    Invoke-GrxFirmaGoBuild `
        -Version $version `
        -WorkingDirectory $RepositoryRoot `
        -LinkerFlags @("-H=windowsgui") `
        -BuildArguments @(
            "-tags",
            "production",
            "-o",
            (Join-Path $candidateStage "app/grxfirma-gui.exe"),
            "./cmd/grxfirma-gui"
        )

    Copy-Item `
        -LiteralPath (Join-Path $RepositoryRoot "packaging/windows/README_DESKTOP_WINUI_WINDOWS.md") `
        -Destination (Join-Path $candidateStage "README_DESKTOP_WINUI_WINDOWS.md")
    [System.IO.File]::WriteAllText(
        (Join-Path $candidateStage "VERSION.txt"),
        "$version`n",
        [System.Text.UTF8Encoding]::new($false)
    )

    Assert-GrxFirmaWinUiPublication `
        -StageDirectory $candidateStage `
        -ExpectedWindowsAppSdk $Script:ExpectedWindowsAppSdk
    Write-GrxFirmaPublishManifest -StageDirectory $candidateStage
    Assert-GrxFirmaPublishManifest -StageDirectory $candidateStage
    Assert-GrxFirmaWinUiPublication `
        -StageDirectory $candidateStage `
        -ExpectedWindowsAppSdk $Script:ExpectedWindowsAppSdk
    Set-GrxFirmaTreeTimestamp `
        -Path $candidateStage `
        -SourceDateEpoch $SourceDateEpoch

    Move-Item -LiteralPath $candidateStage -Destination $stageDirectory
    New-GrxFirmaReproducibleZip `
        -SourceDirectory $stageDirectory `
        -DestinationPath $zipPath `
        -SourceDateEpoch $SourceDateEpoch
    Write-GrxFirmaOuterArtifactMetadata `
        -Directory $OutputDirectory `
        -ZipPath $zipPath

    Write-Output "Stage WinUI validado: $stageDirectory"
    Write-Output "ZIP WinUI generado: $zipPath"
} finally {
    foreach ($name in $environmentNames) {
        if ($null -eq $savedEnvironment[$name]) {
            Remove-Item -LiteralPath ("Env:" + $name) -ErrorAction SilentlyContinue
        } else {
            Set-Item -LiteralPath ("Env:" + $name) -Value $savedEnvironment[$name]
        }
    }
    Restore-GrxFirmaEnvironment -Values $sensitiveEnvironment

    if (Test-Path -LiteralPath $workDirectory) {
        [void](Assert-GrxFirmaPathWithinRoot -Path $workDirectory -Root $temporaryRoot)
        if ((Split-Path -Leaf $workDirectory) -ne "grxfirma-winui-amd64") {
            throw "Ruta temporal WinUI inesperada; no se eliminara: $workDirectory"
        }
        Assert-GrxFirmaNoReparsePoints -Path $workDirectory
        Remove-Item -LiteralPath $workDirectory -Recurse -Force
    }

    $newBuildDirectories = @(
        Get-GrxFirmaProjectBuildDirectories -SourceRoot $winUiSourceRoot |
            Where-Object { $_ -notin $preexistingBuildDirectories }
    )
    foreach ($directory in $newBuildDirectories) {
        [void](Assert-GrxFirmaPathWithinRoot -Path $directory -Root $winUiSourceRoot)
        Assert-GrxFirmaNoReparsePoints -Path $directory
        Remove-Item -LiteralPath $directory -Recurse -Force
    }
}
