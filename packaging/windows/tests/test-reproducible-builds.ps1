# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

$windowsDirectory = Split-Path -Parent $PSScriptRoot
. (Join-Path $windowsDirectory "reproducible-build.ps1")

$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-repro-" + [Guid]::NewGuid().ToString("N"))
$stageDirectory = Join-Path $temporaryRoot "GrxFirma"
$nestedDirectory = Join-Path $stageDirectory "nested"
$zipOne = Join-Path $temporaryRoot "one.zip"
$zipTwo = Join-Path $temporaryRoot "two.zip"
$epoch = [long]1700000000

function Assert-ReproBuildThrows {
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

New-Item -ItemType Directory -Force -Path $nestedDirectory | Out-Null
[System.IO.File]::WriteAllText((Join-Path $stageDirectory "grxfirma.exe"), "exe-fixture")
[System.IO.File]::WriteAllText((Join-Path $nestedDirectory "config.txt"), "config")

try {
    New-GrxFirmaReproducibleZip `
        -SourceDirectory $stageDirectory `
        -DestinationPath $zipOne `
        -SourceDateEpoch $epoch

    (Get-Item (Join-Path $stageDirectory "grxfirma.exe")).LastWriteTimeUtc = [DateTime]::UtcNow
    (Get-Item (Join-Path $nestedDirectory "config.txt")).LastWriteTimeUtc = [DateTime]::UtcNow.AddDays(-10)
    New-GrxFirmaReproducibleZip `
        -SourceDirectory $stageDirectory `
        -DestinationPath $zipTwo `
        -SourceDateEpoch $epoch

    $hashOne = (Get-FileHash -Algorithm SHA256 -LiteralPath $zipOne).Hash
    $hashTwo = (Get-FileHash -Algorithm SHA256 -LiteralPath $zipTwo).Hash
    if ($hashOne -ne $hashTwo) {
        throw "Los ZIP PowerShell no son reproducibles"
    }

    Set-GrxFirmaTreeTimestamp -Path $stageDirectory -SourceDateEpoch $epoch
    $expectedTimestamp = [DateTimeOffset]::FromUnixTimeSeconds($epoch).UtcDateTime
    foreach ($item in @(Get-Item $stageDirectory) + @(Get-ChildItem $stageDirectory -Force -Recurse)) {
        if ($item.LastWriteTimeUtc -ne $expectedTimestamp) {
            throw "Timestamp no normalizado: $($item.FullName)"
        }
    }

    $standaloneArtifact = Join-Path $temporaryRoot "setup.exe"
    [System.IO.File]::WriteAllText($standaloneArtifact, "signed-fixture")
    $artifactHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $standaloneArtifact).Hash
    Set-GrxFirmaFileTimestamp -Path $standaloneArtifact -SourceDateEpoch $epoch
    if ((Get-Item $standaloneArtifact).LastWriteTimeUtc -ne $expectedTimestamp) {
        throw "El mtime del artefacto independiente no esta normalizado"
    }
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $standaloneArtifact).Hash -ne $artifactHash) {
        throw "La normalizacion del mtime modifico los bytes del artefacto"
    }

    Add-Type -AssemblyName System.IO.Compression
    $archive = [System.IO.Compression.ZipFile]::OpenRead($zipOne)
    try {
        $names = [string[]]($archive.Entries | ForEach-Object FullName)
        $sortedNames = [string[]]$names.Clone()
        [Array]::Sort($sortedNames, [StringComparer]::Ordinal)
        if ([string]::Join("`n", $names) -cne [string]::Join("`n", $sortedNames)) {
            throw "Las entradas ZIP no estan ordenadas"
        }
        $timestamps = @($archive.Entries | ForEach-Object LastWriteTime | Select-Object -Unique)
        if ($timestamps.Count -ne 1) {
            throw "Las entradas ZIP no comparten timestamp normalizado"
        }
    } finally {
        $archive.Dispose()
    }

    $scripts = Get-ChildItem -LiteralPath $windowsDirectory -Filter "build-*.ps1"
    foreach ($script in $scripts) {
        $content = Get-Content -LiteralPath $script.FullName -Raw
        if ($content -match '(?m)^\s*(?:&\s+)?go\s+build\b') {
            throw "Invocacion go build directa en $($script.Name)"
        }
        if ($content -match '\bCompress-Archive\b') {
            throw "Compress-Archive no determinista en $($script.Name)"
        }
        if ($script.Name -ne "build-msix.ps1" -and $content -notmatch 'Invoke-GrxFirmaGoBuild') {
            throw "Falta el helper Go reproducible en $($script.Name)"
        }
        if ($script.Name -ne "build-msix.ps1") {
            $goBuildCalls = [regex]::Matches(
                $content,
                '\bInvoke-GrxFirmaGoBuild\b'
            ).Count
            $workingDirectoryArguments = [regex]::Matches(
                $content,
                '(?m)^\s+-WorkingDirectory\s+'
            ).Count
            if ($workingDirectoryArguments -ne $goBuildCalls) {
                throw (
                    "Cada compilacion Go debe fijar su directorio de trabajo " +
                    "en $($script.Name)"
                )
            }
        }
        if ($script.Name -in @("build-nativehost.ps1", "build-suite.ps1") -and
            $content -notmatch 'GRXFIRMA_BUILD_CHROMIUM_CRX') {
            throw "Falta hacer explicito el CRX no reproducible en $($script.Name)"
        }
        if ($script.Name -in @("build-nativehost.ps1", "build-suite.ps1")) {
            if ($content.Contains("dipgra-extension-*")) {
                throw "$($script.Name) empaqueta artefactos de navegador no aprobados por comodin"
            }
            foreach ($approvedExtensionAsset in @(
                "dipgra-extension-chromium.zip",
                "dipgra-extension-firefox.xpi",
                "dipgra-extension-firefox.metadata.json"
            )) {
                if (-not $content.Contains($approvedExtensionAsset)) {
                    throw "$($script.Name) no empaqueta $approvedExtensionAsset"
                }
            }
        }
    }

    $helperContent = Get-Content -LiteralPath (Join-Path $windowsDirectory "reproducible-build.ps1") -Raw
    foreach ($requiredFlag in @("-mod=readonly", "-pgo=off", "-trimpath", "-buildvcs=false", "-buildid=")) {
        if (-not $helperContent.Contains($requiredFlag)) {
            throw "Falta el flag Go reproducible $requiredFlag"
        }
    }
    foreach ($requiredWorkingDirectoryContract in @(
        "[string]`$WorkingDirectory",
        "Push-Location -LiteralPath `$workingDirectoryItem.FullName",
        "Pop-Location"
    )) {
        if (-not $helperContent.Contains($requiredWorkingDirectoryContract)) {
            throw (
                "Falta el contrato de directorio Go reproducible: " +
                $requiredWorkingDirectoryContract
            )
        }
    }

    $artifact = Join-Path $temporaryRoot "grxfirma-afirmauri.exe"
    $metadata = "$artifact.metadata"
    $buildToolStub = Join-Path $temporaryRoot "fake-build-tool.exe"
    $buildToolSource = Join-Path $temporaryRoot "fake-build-tool.go"
    [System.IO.File]::WriteAllText($artifact, "pe-fixture")
    [System.IO.File]::WriteAllText(
        $buildToolSource,
        @'
package main

import (
    "fmt"
    "os"
)

func main() {
    if len(os.Args) == 2 && os.Args[1] == "-dumpmachine" {
        target := os.Getenv("FAKE_COMPILER_TARGET")
        if target == "" {
            target = "x86_64-w64-mingw32"
        }
        fmt.Println(target)
        return
    }
    if len(os.Args) != 4 || os.Args[1] != "version" || os.Args[2] != "-m" {
        os.Exit(2)
    }
    data, err := os.ReadFile(os.Args[3] + ".metadata")
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    _, _ = os.Stdout.Write(data)
}
'@
    )
    & go build -o $buildToolStub $buildToolSource
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo construir la toolchain Go de prueba"
    }

    $fakeCompilerTargetDefined = Test-Path Env:FAKE_COMPILER_TARGET
    $previousFakeCompilerTarget = $env:FAKE_COMPILER_TARGET
    try {
        Remove-Item Env:FAKE_COMPILER_TARGET -ErrorAction SilentlyContinue
        Assert-GrxFirmaWindowsFyneToolchain `
            -Architecture "amd64" `
            -CompilerCommand $buildToolStub
        Assert-ReproBuildThrows `
            -Action {
                Assert-GrxFirmaWindowsFyneToolchain `
                    -Architecture "arm64" `
                    -CompilerCommand $buildToolStub
            } `
            -Message "El preflight Fyne acepto una arquitectura distinta de amd64"
        Assert-ReproBuildThrows `
            -Action {
                Assert-GrxFirmaWindowsFyneToolchain `
                    -Architecture "amd64" `
                    -CompilerCommand "grxfirma-missing-gcc"
            } `
            -Message "El preflight Fyne acepto un compilador inexistente"

        $env:FAKE_COMPILER_TARGET = "aarch64-w64-mingw32"
        Assert-ReproBuildThrows `
            -Action {
                Assert-GrxFirmaWindowsFyneToolchain `
                    -Architecture "amd64" `
                    -CompilerCommand $buildToolStub
            } `
            -Message "El preflight Fyne acepto un compilador que no es amd64"
    } finally {
        if ($fakeCompilerTargetDefined) {
            $env:FAKE_COMPILER_TARGET = $previousFakeCompilerTarget
        } else {
            Remove-Item Env:FAKE_COMPILER_TARGET -ErrorAction SilentlyContinue
        }
    }

    @(
        "`tpath`tgrxfirma/cmd/grxfirmauri",
        "`tbuild`t-tags=other,production,fyne_gui",
        "`tbuild`tCGO_ENABLED=1",
        "`tbuild`tGOARCH=amd64",
        "`tbuild`tGOOS=windows"
    ) | Set-Content -LiteralPath $metadata -Encoding utf8
    Assert-GrxFirmaWindowsFyneArtifact `
        -Path $artifact `
        -Architecture "amd64" `
        -GoCommand $buildToolStub

    (Get-Content -LiteralPath $metadata -Raw).Replace(
        "-tags=other,production,fyne_gui",
        "-tags=other,fyne_gui"
    ) | Set-Content -LiteralPath $metadata -Encoding utf8
    Assert-ReproBuildThrows `
        -Action {
            Assert-GrxFirmaWindowsFyneArtifact `
                -Path $artifact `
                -Architecture "amd64" `
                -GoCommand $buildToolStub
        } `
        -Message "El gate Fyne acepto un artefacto sin production"

    (Get-Content -LiteralPath $metadata -Raw).Replace(
        "-tags=other,fyne_gui",
        "-tags=other,production"
    ) | Set-Content -LiteralPath $metadata -Encoding utf8
    Assert-ReproBuildThrows `
        -Action {
            Assert-GrxFirmaWindowsFyneArtifact `
                -Path $artifact `
                -Architecture "amd64" `
                -GoCommand $buildToolStub
        } `
        -Message "El gate Fyne acepto un artefacto sin fyne_gui"

    (Get-Content -LiteralPath $metadata -Raw).Replace(
        "-tags=other,production",
        "-tags=production,fyne_gui"
    ).Replace(
        "CGO_ENABLED=1",
        "CGO_ENABLED=0"
    ) | Set-Content -LiteralPath $metadata -Encoding utf8
    Assert-ReproBuildThrows `
        -Action {
            Assert-GrxFirmaWindowsFyneArtifact `
                -Path $artifact `
                -Architecture "amd64" `
                -GoCommand $buildToolStub
        } `
        -Message "El gate Fyne acepto un artefacto sin CGO"
} finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}

Write-Output "Windows reproducible build tests passed."
