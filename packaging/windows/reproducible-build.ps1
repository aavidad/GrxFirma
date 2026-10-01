# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

function Get-GrxFirmaSourceDateEpoch {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RootDirectory
    )

    $epoch = $env:SOURCE_DATE_EPOCH
    if ([string]::IsNullOrWhiteSpace($epoch) -and (Get-Command git -ErrorAction SilentlyContinue)) {
        $epoch = (& git -C $RootDirectory log -1 --format=%ct 2>$null)
    }
    if ([string]::IsNullOrWhiteSpace($epoch)) {
        $epoch = "0"
    }
    if ($epoch -notmatch '^\d+$') {
        throw "SOURCE_DATE_EPOCH debe ser un entero no negativo: $epoch"
    }
    return [long]$epoch
}

function Initialize-GrxFirmaReproducibleBuild {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RootDirectory
    )

    $epoch = Get-GrxFirmaSourceDateEpoch -RootDirectory $RootDirectory
    $env:SOURCE_DATE_EPOCH = $epoch.ToString([System.Globalization.CultureInfo]::InvariantCulture)
    $env:TZ = "UTC"
    $env:ZERO_AR_DATE = "1"
    return $epoch
}

function Resolve-GrxFirmaMakeNsis {
    $explicit = $env:MAKENSIS
    if (-not [string]::IsNullOrWhiteSpace($explicit)) {
        if (Test-Path -LiteralPath $explicit -PathType Leaf) {
            return (Get-Item -LiteralPath $explicit -Force).FullName
        }
        $explicitCommand = Get-Command `
            $explicit `
            -CommandType Application `
            -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($null -ne $explicitCommand) {
            return $explicitCommand.Source
        }
        throw "MAKENSIS no apunta a un ejecutable valido: $explicit"
    }

    $pathCommand = Get-Command `
        "makensis" `
        -CommandType Application `
        -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -ne $pathCommand) {
        return $pathCommand.Source
    }

    $candidates = @()
    foreach ($programFilesRoot in @(${env:ProgramFiles(x86)}, $env:ProgramFiles)) {
        if (-not [string]::IsNullOrWhiteSpace($programFilesRoot)) {
            $candidates += Join-Path $programFilesRoot "NSIS\makensis.exe"
        }
    }
    foreach ($candidate in $candidates | Select-Object -Unique) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return (Get-Item -LiteralPath $candidate -Force).FullName
        }
    }

    throw (
        "No se encontro makensis. Instala NSIS, añade makensis.exe a PATH " +
        "o define MAKENSIS con su ruta completa."
    )
}

function Invoke-GrxFirmaGoBuild {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Version,
        [Parameter(Mandatory = $true)]
        [string]$WorkingDirectory,
        [string[]]$LinkerFlags = @(),
        [Parameter(Mandatory = $true)]
        [string[]]$BuildArguments
    )

    $workingDirectoryItem = Get-Item `
        -LiteralPath $WorkingDirectory `
        -Force `
        -ErrorAction Stop
    if (-not $workingDirectoryItem.PSIsContainer) {
        throw "El directorio de compilacion Go no es valido: $WorkingDirectory"
    }

    $ldflags = (@("-buildid=") + $LinkerFlags + @("-X", "main.version=$Version", "-s", "-w")) -join " "
    $hadGoFlags = Test-Path Env:GOFLAGS
    $previousGoFlags = $env:GOFLAGS
    Push-Location -LiteralPath $workingDirectoryItem.FullName
    try {
        $env:GOFLAGS = ""
        & go build `
            -mod=readonly `
            -pgo=off `
            -trimpath `
            -buildvcs=false `
            -ldflags $ldflags `
            @BuildArguments
        if ($LASTEXITCODE -ne 0) {
            throw "La compilacion Go fallo con codigo $LASTEXITCODE"
        }
    } finally {
        Pop-Location
        if ($hadGoFlags) {
            $env:GOFLAGS = $previousGoFlags
        } else {
            Remove-Item Env:GOFLAGS -ErrorAction SilentlyContinue
        }
    }
}

function Assert-GrxFirmaWindowsFyneToolchain {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Architecture,
        [string]$CompilerCommand = $env:CC
    )

    if ($Architecture -ne "amd64") {
        throw "La GUI afirma:// con CGO solo esta soportada para GOARCH=amd64."
    }
    if ([string]::IsNullOrWhiteSpace($CompilerCommand)) {
        $CompilerCommand = "gcc"
    }
    $compiler = Get-Command $CompilerCommand -CommandType Application -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -eq $compiler) {
        throw (
            "No se encontro el compilador MinGW-w64 amd64 requerido: " +
            "$CompilerCommand. Instala MinGW-w64 o define CC con la ruta de gcc.exe."
        )
    }

    $targetOutput = @(& $compiler.Source -dumpmachine 2>&1)
    $compilerExitCode = $LASTEXITCODE
    $target = ($targetOutput -join "`n").Trim()
    if ($compilerExitCode -ne 0) {
        throw "No se pudo consultar el target del compilador CGO $($compiler.Source): $target"
    }
    if ($target -ne "x86_64-w64-mingw32") {
        throw "El compilador CGO no es MinGW-w64 amd64: $($compiler.Source) ($target)"
    }
}

function Assert-GrxFirmaWindowsFyneArtifact {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [string]$Architecture = "amd64",
        [string]$GoCommand = "go"
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "No existe el handler afirma:// que se debe validar: $Path"
    }
    $LASTEXITCODE = 0
    $metadataLines = @(& $GoCommand version -m $Path 2>&1)
    $goExitCode = $LASTEXITCODE
    if ($goExitCode -ne 0) {
        throw "No se pudieron leer los metadatos Go de $Path`: $($metadataLines -join ' ')"
    }

    $settings = @{}
    foreach ($line in $metadataLines) {
        if ($line -match '^\s*build\s+(\S+)=(.*)\s*$') {
            $settings[$matches[1]] = $matches[2].Trim().Trim('"')
        }
    }
    $tags = @($settings["-tags"] -split "," | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    if ($tags -notcontains "fyne_gui") {
        throw "$Path no contiene el build tag obligatorio fyne_gui."
    }
    if ($tags -notcontains "production") {
        throw "$Path no contiene el build tag obligatorio production."
    }
    if ($settings["CGO_ENABLED"] -ne "1") {
        throw "$Path no declara CGO_ENABLED=1."
    }
    if ($settings["GOOS"] -ne "windows" -or $settings["GOARCH"] -ne $Architecture) {
        throw (
            "Metadatos de plataforma inesperados en $Path`: " +
            "GOOS=$($settings['GOOS']) GOARCH=$($settings['GOARCH'])"
        )
    }
}

function New-GrxFirmaReproducibleZip {
    [System.Diagnostics.CodeAnalysis.SuppressMessageAttribute(
        "PSUseShouldProcessForStateChangingFunctions",
        "",
        Justification = "La funcion crea de forma atomica el artefacto solicitado por el empaquetador."
    )]
    param(
        [Parameter(Mandatory = $true)]
        [string]$SourceDirectory,
        [Parameter(Mandatory = $true)]
        [string]$DestinationPath,
        [Parameter(Mandatory = $true)]
        [long]$SourceDateEpoch
    )

    $zipMinimumEpoch = [long]315532800
    $zipMaximumEpoch = [long]4354819198
    if ($SourceDateEpoch -lt 0 -or $SourceDateEpoch -gt $zipMaximumEpoch) {
        throw "SOURCE_DATE_EPOCH esta fuera del rango ZIP portable: $SourceDateEpoch"
    }

    $source = Get-Item -LiteralPath $SourceDirectory -ErrorAction Stop
    if (-not $source.PSIsContainer) {
        throw "El origen del ZIP no es un directorio: $SourceDirectory"
    }
    $destinationDirectory = Split-Path -Parent $DestinationPath
    New-Item -ItemType Directory -Force -Path $destinationDirectory | Out-Null

    Add-Type -AssemblyName System.IO.Compression
    $zipEpoch = [Math]::Max($SourceDateEpoch, $zipMinimumEpoch)
    $fixedTimestamp = [DateTimeOffset]::FromUnixTimeSeconds($zipEpoch)
    $temporaryPath = "$DestinationPath.tmp-$([Guid]::NewGuid().ToString('N'))"
    $entryMap = @{}
    foreach ($item in Get-ChildItem -LiteralPath $source.FullName -Force -Recurse) {
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Los enlaces o reparse points no estan permitidos en el ZIP Windows: $($item.FullName)"
        }
        $relative = $item.FullName.Substring($source.Parent.FullName.Length).TrimStart('\', '/')
        $entryName = $relative.Replace('\', '/')
        if ($item.PSIsContainer) {
            $entryName = $entryName.TrimEnd('/') + '/'
        }
        if ($entryMap.ContainsKey($entryName)) {
            throw "Entrada ZIP duplicada: $entryName"
        }
        $entryMap[$entryName] = $item
    }
    $entryNames = [string[]]$entryMap.Keys
    [Array]::Sort($entryNames, [StringComparer]::Ordinal)

    try {
        $fileStream = [System.IO.File]::Open(
            $temporaryPath,
            [System.IO.FileMode]::CreateNew,
            [System.IO.FileAccess]::ReadWrite,
            [System.IO.FileShare]::None
        )
        try {
            $archive = [System.IO.Compression.ZipArchive]::new(
                $fileStream,
                [System.IO.Compression.ZipArchiveMode]::Create,
                $true
            )
            try {
                $rootEntry = $archive.CreateEntry("$($source.Name)/", [System.IO.Compression.CompressionLevel]::NoCompression)
                $rootEntry.LastWriteTime = $fixedTimestamp
                $rootEntry.ExternalAttributes = 16

                foreach ($entryName in $entryNames) {
                    $item = $entryMap[$entryName]
                    $compression = if ($item.PSIsContainer) {
                        [System.IO.Compression.CompressionLevel]::NoCompression
                    } else {
                        [System.IO.Compression.CompressionLevel]::Optimal
                    }
                    $entry = $archive.CreateEntry($entryName, $compression)
                    $entry.LastWriteTime = $fixedTimestamp
                    if ($item.PSIsContainer) {
                        $entry.ExternalAttributes = 16
                        continue
                    }
                    $inputStream = [System.IO.File]::OpenRead($item.FullName)
                    try {
                        $outputStream = $entry.Open()
                        try {
                            $inputStream.CopyTo($outputStream)
                        } finally {
                            $outputStream.Dispose()
                        }
                    } finally {
                        $inputStream.Dispose()
                    }
                }
            } finally {
                $archive.Dispose()
            }
        } finally {
            $fileStream.Dispose()
        }

        Move-Item -LiteralPath $temporaryPath -Destination $DestinationPath -Force
        [System.IO.File]::SetLastWriteTimeUtc(
            $DestinationPath,
            [DateTimeOffset]::FromUnixTimeSeconds($zipEpoch).UtcDateTime
        )
    } finally {
        if (Test-Path -LiteralPath $temporaryPath) {
            Remove-Item -LiteralPath $temporaryPath -Force
        }
    }
}

function Set-GrxFirmaTreeTimestamp {
    [System.Diagnostics.CodeAnalysis.SuppressMessageAttribute(
        "PSUseShouldProcessForStateChangingFunctions",
        "",
        Justification = "La funcion solo normaliza metadatos del arbol temporal del empaquetador."
    )]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [long]$SourceDateEpoch
    )

    $archiveMinimumEpoch = [long]315532800
    $archiveMaximumEpoch = [long]4354819198
    if ($SourceDateEpoch -lt 0 -or $SourceDateEpoch -gt $archiveMaximumEpoch) {
        throw "SOURCE_DATE_EPOCH esta fuera del rango de archivo portable: $SourceDateEpoch"
    }
    $portableEpoch = [Math]::Max($SourceDateEpoch, $archiveMinimumEpoch)
    $timestamp = [DateTimeOffset]::FromUnixTimeSeconds($portableEpoch).UtcDateTime
    $items = @(Get-ChildItem -LiteralPath $Path -Force -Recurse)
    foreach ($item in $items | Where-Object { -not $_.PSIsContainer }) {
        $item.LastWriteTimeUtc = $timestamp
    }
    foreach ($item in $items | Where-Object { $_.PSIsContainer } | Sort-Object FullName -Descending) {
        $item.LastWriteTimeUtc = $timestamp
    }
    (Get-Item -LiteralPath $Path).LastWriteTimeUtc = $timestamp
}

function Set-GrxFirmaFileTimestamp {
    [System.Diagnostics.CodeAnalysis.SuppressMessageAttribute(
        "PSUseShouldProcessForStateChangingFunctions",
        "",
        Justification = "La funcion solo normaliza el mtime del artefacto generado."
    )]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [long]$SourceDateEpoch
    )

    $item = Get-Item -LiteralPath $Path -ErrorAction Stop
    if ($item.PSIsContainer) {
        throw "El artefacto no es un fichero: $Path"
    }
    if ($SourceDateEpoch -lt 0 -or $SourceDateEpoch -gt [long]4354819198) {
        throw "SOURCE_DATE_EPOCH esta fuera del rango de archivo portable: $SourceDateEpoch"
    }
    $portableEpoch = [Math]::Max($SourceDateEpoch, [long]315532800)
    $item.LastWriteTimeUtc = [DateTimeOffset]::FromUnixTimeSeconds($portableEpoch).UtcDateTime
}
