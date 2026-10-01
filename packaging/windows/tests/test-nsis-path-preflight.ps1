# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

$windowsDir = Split-Path -Parent $PSScriptRoot
$preflight = Join-Path $windowsDir "nsis-path-preflight.ps1"
$suiteBuilder = Join-Path $windowsDir "build-suite.ps1"
. $preflight

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Assert-Throws {
    param(
        [scriptblock]$Action,
        [string]$Message
    )

    try {
        & $Action
    } catch {
        return
    }
    throw $Message
}

$temporaryRoot = Join-Path (
    [System.IO.Path]::GetTempPath()
) ("grxfirma-nsis-path-" + [guid]::NewGuid().ToString("N"))
try {
    $repository = Join-Path $temporaryRoot "repository"
    $stage = Join-Path $repository "release/windows-suite/stage"
    $deepDirectory = Join-Path $stage (
        ("a" * 70) + [System.IO.Path]::DirectorySeparatorChar + ("b" * 70)
    )
    New-Item -ItemType Directory -Force -Path $deepDirectory | Out-Null
    $deepFile = Join-Path $deepDirectory "payload.dll"
    Set-Content -LiteralPath $deepFile -Value "fixture" -Encoding ASCII
    $scriptPath = Join-Path $repository "packaging/windows/suite.nsi"
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $scriptPath) |
        Out-Null
    Set-Content -LiteralPath $scriptPath -Value @'
Unicode True
!ifndef STAGE_DIR
  !error "Falta STAGE_DIR"
!endif
!ifndef OUT_FILE
  !error "Falta OUT_FILE"
!endif
OutFile "${OUT_FILE}"
Section
  SetOutPath "$TEMP\GrxFirmaNsisPathFixture"
  File /r "${STAGE_DIR}\*.*"
SectionEnd
'@ -Encoding ASCII
    $outputPath = Join-Path $repository "release/windows-suite/setup.exe"

    $metrics = Get-GrxFirmaNsisPathMetrics `
        -StageDirectory $stage `
        -OutputPath $outputPath `
        -ScriptPath $scriptPath
    Assert-True `
        -Condition $metrics.ExceedsLegacyBudget `
        -Message "El preflight no detecto una ruta de payload deliberadamente larga."
    Assert-True `
        -Condition ($metrics.LongestPath -eq [System.IO.Path]::GetFullPath($deepFile)) `
        -Message "El preflight no identifica la ruta mas larga real del payload."
    Assert-True `
        -Condition ($metrics.LegacyBudget -eq 240) `
        -Message "El presupuesto preventivo NSIS cambio sin actualizar el contrato."

    $aliasRoot = Join-Path $temporaryRoot "r"
    $mappedStage = ConvertTo-GrxFirmaNsisAliasPath `
        -Path $stage `
        -RepositoryRoot $repository `
        -AliasRoot $aliasRoot
    Assert-True `
        -Condition ($mappedStage -eq (
            Join-Path $aliasRoot "release/windows-suite/stage"
        )) `
        -Message "La conversion a alias no conserva la ruta relativa de la stage."
    Assert-Throws `
        -Action {
            ConvertTo-GrxFirmaNsisAliasPath `
                -Path (Split-Path -Parent $repository) `
                -RepositoryRoot $repository `
                -AliasRoot $aliasRoot
        } `
        -Message "El alias NSIS acepto una ruta fuera del repositorio."

    $parsedTarget = Get-GrxFirmaSubstTargetFromOutput `
        -Lines @(
            "Y:\: => C:\otro",
            "Z:\: => C:\ruta larga\repositorio"
        ) `
        -Drive "Z:"
    Assert-True `
        -Condition ($parsedTarget -eq "C:\ruta larga\repositorio") `
        -Message "No se puede verificar de forma cerrada el propietario del alias SUBST."

    $context = $null
    try {
        $context = New-GrxFirmaNsisPathContext `
            -RepositoryRoot $repository `
            -StageDirectory $stage `
            -OutputPath $outputPath `
            -ScriptPath $scriptPath
        $runningOnWindows = [System.Environment]::OSVersion.Platform -eq
            [System.PlatformID]::Win32NT
        Assert-True `
            -Condition ([bool]$context.UsesSubst -eq $runningOnWindows) `
            -Message "El preflight no limita el alias SUBST a Windows con rutas largas."
        if ($runningOnWindows) {
            Assert-True `
                -Condition (-not $context.EffectiveMetrics.ExceedsLegacyBudget) `
                -Message "El alias SUBST no redujo la stage por debajo del presupuesto NSIS."
        }

        $makensisCommand = Get-Command "makensis" -ErrorAction SilentlyContinue
        $makensisPath = if ($null -ne $makensisCommand) {
            $makensisCommand.Source
        } else {
            $null
        }
        if ([string]::IsNullOrWhiteSpace($makensisPath) -and $runningOnWindows) {
            $knownMakensis = Join-Path ${env:ProgramFiles(x86)} "NSIS\makensis.exe"
            if (Test-Path -LiteralPath $knownMakensis -PathType Leaf) {
                $makensisPath = $knownMakensis
            }
        }
        if (-not [string]::IsNullOrWhiteSpace($makensisPath)) {
            $makensisLog = Join-Path $temporaryRoot "makensis.log"
            & $makensisPath `
                "-DSTAGE_DIR=$($context.StageDirectory)" `
                "-DOUT_FILE=$($context.OutputPath)" `
                $context.ScriptPath *> $makensisLog
            if ($LASTEXITCODE -ne 0 -or
                (-not (Test-Path -LiteralPath $outputPath -PathType Leaf))) {
                $makensisDetails = (
                    Get-Content -LiteralPath $makensisLog -Tail 10 -ErrorAction SilentlyContinue
                ) -join [Environment]::NewLine
                throw (
                    "makensis no pudo consumir el payload mediante la ruta prevalidada. " +
                    $makensisDetails
                )
            }
        }
    } finally {
        if ($null -ne $context) {
            Remove-GrxFirmaNsisPathContext -Context $context
        }
    }

    $builderContent = Get-Content -LiteralPath $suiteBuilder -Raw
    foreach ($fragment in @(
        "nsis-path-preflight.ps1",
        "New-GrxFirmaNsisPathContext",
        "Remove-GrxFirmaNsisPathContext",
        '/DSTAGE_DIR=$($nsisPathContext.StageDirectory)',
        '/DOUT_FILE=$($nsisPathContext.OutputPath)',
        '$nsisArguments += $nsisPathContext.ScriptPath'
    )) {
        if (-not $builderContent.Contains($fragment)) {
            throw "build-suite.ps1 no aplica el contrato de rutas NSIS: $fragment"
        }
    }
    $newContextOffset = $builderContent.LastIndexOf(
        "New-GrxFirmaNsisPathContext",
        [System.StringComparison]::Ordinal
    )
    $makensisOffset = $builderContent.LastIndexOf(
        '& $MakeNsis',
        [System.StringComparison]::Ordinal
    )
    $removeContextOffset = $builderContent.LastIndexOf(
        "Remove-GrxFirmaNsisPathContext",
        [System.StringComparison]::Ordinal
    )
    if ($newContextOffset -lt 0 -or
        $makensisOffset -le $newContextOffset -or
        $removeContextOffset -le $makensisOffset) {
        throw "El alias NSIS no se crea y retira alrededor de makensis."
    }
} finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}

Write-Output "Windows NSIS long-path preflight contracts passed."
