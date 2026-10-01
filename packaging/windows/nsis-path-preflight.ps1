# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$script:GrxFirmaNsisLegacyPathBudget = 240

function Get-GrxFirmaNsisPathMetrics {
    param(
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory,
        [Parameter(Mandatory = $true)]
        [string]$OutputPath,
        [Parameter(Mandatory = $true)]
        [string]$ScriptPath
    )

    $stage = [System.IO.Path]::GetFullPath($StageDirectory)
    $candidates = @(
        $stage,
        [System.IO.Path]::GetFullPath($OutputPath),
        [System.IO.Path]::GetFullPath($ScriptPath)
    )
    if (Test-Path -LiteralPath $stage -PathType Container) {
        $candidates += @(
            Get-ChildItem -LiteralPath $stage -Force -Recurse -ErrorAction Stop |
                ForEach-Object { [System.IO.Path]::GetFullPath($_.FullName) }
        )
    }

    $longest = @(
        $candidates |
            Sort-Object { $_.Length } -Descending |
            Select-Object -First 1
    )
    if ($longest.Count -ne 1) {
        throw "No se pudo calcular la ruta mas larga del payload NSIS."
    }
    $length = $longest[0].Length
    return [pscustomobject]@{
        LongestPath = $longest[0]
        LongestLength = $length
        LegacyBudget = $script:GrxFirmaNsisLegacyPathBudget
        ExceedsLegacyBudget = $length -ge $script:GrxFirmaNsisLegacyPathBudget
    }
}

function ConvertTo-GrxFirmaNsisAliasPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory = $true)]
        [string]$AliasRoot
    )

    $root = [System.IO.Path]::GetFullPath($RepositoryRoot).TrimEnd('\', '/')
    $candidate = [System.IO.Path]::GetFullPath($Path)
    $comparison = if (
        [System.Environment]::OSVersion.Platform -eq
            [System.PlatformID]::Win32NT
    ) {
        [System.StringComparison]::OrdinalIgnoreCase
    } else {
        [System.StringComparison]::Ordinal
    }
    if ($candidate.Equals($root, $comparison)) {
        return [System.IO.Path]::GetFullPath($AliasRoot)
    }

    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if (-not $candidate.StartsWith($prefix, $comparison)) {
        throw "La ruta NSIS queda fuera del repositorio y no puede recibir un alias seguro: $candidate"
    }
    $relative = $candidate.Substring($prefix.Length)
    return [System.IO.Path]::GetFullPath((Join-Path $AliasRoot $relative))
}

function Get-GrxFirmaSubstTargetFromOutput {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Lines,
        [Parameter(Mandatory = $true)]
        [ValidatePattern('^[A-Z]:$')]
        [string]$Drive
    )

    $drivePrefix = [regex]::Escape($Drive + "\")
    $pattern = "^\s*$drivePrefix`:\s*=>\s*(.+?)\s*$"
    foreach ($line in $Lines) {
        if ([string]$line -match $pattern) {
            return $Matches[1]
        }
    }
    return $null
}

function Get-GrxFirmaSubstTarget {
    param(
        [Parameter(Mandatory = $true)]
        [string]$SubstPath,
        [Parameter(Mandatory = $true)]
        [ValidatePattern('^[A-Z]:$')]
        [string]$Drive
    )

    $output = @(& $SubstPath 2>$null)
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo consultar el inventario de alias SUBST."
    }
    return Get-GrxFirmaSubstTargetFromOutput -Lines $output -Drive $Drive
}

function New-GrxFirmaNsisPathContext {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory = $true)]
        [string]$StageDirectory,
        [Parameter(Mandatory = $true)]
        [string]$OutputPath,
        [Parameter(Mandatory = $true)]
        [string]$ScriptPath
    )

    $repository = [System.IO.Path]::GetFullPath($RepositoryRoot).TrimEnd('\', '/')
    $stage = [System.IO.Path]::GetFullPath($StageDirectory)
    $output = [System.IO.Path]::GetFullPath($OutputPath)
    $script = [System.IO.Path]::GetFullPath($ScriptPath)
    $directMetrics = Get-GrxFirmaNsisPathMetrics `
        -StageDirectory $stage `
        -OutputPath $output `
        -ScriptPath $script
    $runningOnWindows = [System.Environment]::OSVersion.Platform -eq
        [System.PlatformID]::Win32NT
    if (-not $runningOnWindows -or -not $directMetrics.ExceedsLegacyBudget) {
        return [pscustomobject]@{
            UsesSubst = $false
            RepositoryRoot = $repository
            StageDirectory = $stage
            OutputPath = $output
            ScriptPath = $script
            Drive = $null
            SubstPath = $null
            DirectMetrics = $directMetrics
            EffectiveMetrics = $directMetrics
        }
    }

    if ($repository.StartsWith("\\", [System.StringComparison]::Ordinal)) {
        throw (
            "NSIS necesita acortar una ruta de $($directMetrics.LongestLength) caracteres, " +
            "pero el repositorio reside en una ruta UNC que SUBST no admite. " +
            "Use una copia local corta del repositorio."
        )
    }
    $repositoryDrive = [System.IO.Path]::GetPathRoot($repository)
    if ([string]::IsNullOrWhiteSpace($repositoryDrive)) {
        throw "No se pudo resolver el volumen local del repositorio para NSIS."
    }
    try {
        $driveInfo = [System.IO.DriveInfo]::new($repositoryDrive)
    } catch {
        throw "No se pudo verificar el volumen del repositorio para NSIS."
    }
    if ($driveInfo.DriveType -eq [System.IO.DriveType]::Network) {
        throw (
            "NSIS necesita acortar una ruta de $($directMetrics.LongestLength) caracteres, " +
            "pero el repositorio reside en una unidad de red. " +
            "Use una copia local corta del repositorio."
        )
    }

    $substPath = Join-Path $env:SystemRoot "System32\subst.exe"
    if (-not (Test-Path -LiteralPath $substPath -PathType Leaf)) {
        throw (
            "NSIS necesita un alias corto porque la ruta mas larga mide " +
            "$($directMetrics.LongestLength) caracteres, pero no se encuentra SUBST."
        )
    }

    $attemptErrors = @()
    foreach ($letterCode in ([int][char]'Z')..([int][char]'D')) {
        $letter = [char]$letterCode
        $drive = "$letter`:"
        if ($null -ne (
            Get-PSDrive -Name ([string]$letter) -PSProvider FileSystem `
                -ErrorAction SilentlyContinue
        )) {
            continue
        }

        & $substPath $drive $repository *> $null
        if ($LASTEXITCODE -ne 0) {
            $attemptErrors += $drive
            continue
        }

        try {
            $registeredTarget = Get-GrxFirmaSubstTarget `
                -SubstPath $substPath `
                -Drive $drive
            if ([string]::IsNullOrWhiteSpace($registeredTarget) -or
                -not [System.IO.Path]::GetFullPath($registeredTarget).Equals(
                    $repository,
                    [System.StringComparison]::OrdinalIgnoreCase
                )) {
                throw "SUBST no registro el alias sobre el repositorio esperado."
            }

            $aliasRoot = "$drive\"
            $mappedStage = ConvertTo-GrxFirmaNsisAliasPath `
                -Path $stage `
                -RepositoryRoot $repository `
                -AliasRoot $aliasRoot
            $mappedOutput = ConvertTo-GrxFirmaNsisAliasPath `
                -Path $output `
                -RepositoryRoot $repository `
                -AliasRoot $aliasRoot
            $mappedScript = ConvertTo-GrxFirmaNsisAliasPath `
                -Path $script `
                -RepositoryRoot $repository `
                -AliasRoot $aliasRoot
            if (-not (Test-Path -LiteralPath $mappedStage -PathType Container) -or
                -not (Test-Path -LiteralPath $mappedScript -PathType Leaf)) {
                throw "El alias corto NSIS no permite leer la stage o el script."
            }

            $effectiveMetrics = Get-GrxFirmaNsisPathMetrics `
                -StageDirectory $mappedStage `
                -OutputPath $mappedOutput `
                -ScriptPath $mappedScript
            if ($effectiveMetrics.ExceedsLegacyBudget) {
                throw (
                    "La ruta relativa del payload NSIS sigue siendo demasiado larga " +
                    "incluso con un alias de unidad: " +
                    "$($effectiveMetrics.LongestPath) " +
                    "($($effectiveMetrics.LongestLength) caracteres)."
                )
            }

            Write-Host (
                "NSIS: ruta larga detectada " +
                "($($directMetrics.LongestLength) caracteres); " +
                "se usara temporalmente el alias $drive."
            )
            return [pscustomobject]@{
                UsesSubst = $true
                RepositoryRoot = $repository
                StageDirectory = $mappedStage
                OutputPath = $mappedOutput
                ScriptPath = $mappedScript
                Drive = $drive
                SubstPath = $substPath
                DirectMetrics = $directMetrics
                EffectiveMetrics = $effectiveMetrics
            }
        } catch {
            $aliasError = $_
            $registeredTarget = Get-GrxFirmaSubstTarget `
                -SubstPath $substPath `
                -Drive $drive
            if (-not [string]::IsNullOrWhiteSpace($registeredTarget) -and
                [System.IO.Path]::GetFullPath($registeredTarget).Equals(
                    $repository,
                    [System.StringComparison]::OrdinalIgnoreCase
                )) {
                & $substPath $drive /D *> $null
            }
            throw $aliasError
        }
    }

    throw (
        "NSIS no puede abrir con seguridad una ruta de " +
        "$($directMetrics.LongestLength) caracteres y no hay una letra de unidad " +
        "libre para crear un alias SUBST temporal. Intentos fallidos: " +
        ($attemptErrors -join ", ")
    )
}

function Remove-GrxFirmaNsisPathContext {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Context
    )

    if (-not [bool]$Context.UsesSubst) {
        return
    }
    $registeredTarget = Get-GrxFirmaSubstTarget `
        -SubstPath ([string]$Context.SubstPath) `
        -Drive ([string]$Context.Drive)
    if ([string]::IsNullOrWhiteSpace($registeredTarget)) {
        if (Test-Path -LiteralPath ([string]$Context.Drive + "\")) {
            throw "El alias NSIS ya no es identificable y no se retirara automaticamente."
        }
        return
    }
    if (-not [System.IO.Path]::GetFullPath($registeredTarget).Equals(
        [string]$Context.RepositoryRoot,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        throw "El alias NSIS cambio de destino y no se retirara automaticamente."
    }

    & ([string]$Context.SubstPath) ([string]$Context.Drive) /D *> $null
    if ($LASTEXITCODE -ne 0 -or
        (Test-Path -LiteralPath ([string]$Context.Drive + "\"))) {
        throw "No se pudo retirar el alias NSIS temporal $($Context.Drive)."
    }
    Write-Host "NSIS: alias temporal $($Context.Drive) retirado."
}
