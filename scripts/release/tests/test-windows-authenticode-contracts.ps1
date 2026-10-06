# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [switch]$RequireNsis
)

$ErrorActionPreference = "Stop"

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-Contains {
    param(
        [string]$Path,
        [string[]]$Expected
    )

    $content = Get-Content -LiteralPath $Path -Raw
    foreach ($fragment in $Expected) {
        if (-not $content.Contains($fragment)) {
            throw "Falta '$fragment' en $Path"
        }
    }
}

function Assert-Rejects {
    param(
        [scriptblock]$Action,
        [string]$Message
    )

    $rejected = $false
    try {
        & $Action
    } catch {
        $rejected = $true
    }
    if (-not $rejected) {
        throw $Message
    }
}

$repositoryRoot = Split-Path -Parent (
    Split-Path -Parent (
        Split-Path -Parent $PSScriptRoot
    )
)
$releaseScripts = Join-Path $repositoryRoot "scripts/release"
$windowsPackaging = Join-Path $repositoryRoot "packaging/windows"
$helpers = Join-Path $releaseScripts "windows-authenticode.ps1"
. $helpers

Assert-True (
    (Get-NormalizedWindowsSigningThumbprint `
        -Thumbprint "01 23456789abcdef0123456789abcdef01234567") -eq
        "0123456789ABCDEF0123456789ABCDEF01234567"
) "La huella Authenticode no se normaliza de forma estricta"
Assert-Rejects {
    Get-NormalizedWindowsSigningThumbprint -Thumbprint "not-a-thumbprint"
} "Se acepto una huella Authenticode no fijada"

Assert-True (
    (Get-NormalizedRfc3161TimestampUrl `
        -TimestampUrl "https://timestamp.example.test/rfc3161") -eq
        "https://timestamp.example.test/rfc3161"
) "Se rechazo una URL RFC3161 HTTPS valida"
Assert-True (
    (Get-NormalizedRfc3161TimestampUrl `
        -TimestampUrl "http://timestamp.example.test/rfc3161") -eq
        "http://timestamp.example.test/rfc3161"
) "Se rechazo una URL RFC3161 HTTP valida"
foreach ($unsafeTimestampUrl in @(
    "file:///tmp/timestamp",
    "https://user:password@timestamp.example.test/",
    "https://timestamp.example.test/#fragment"
)) {
    Assert-Rejects {
        Get-NormalizedRfc3161TimestampUrl `
            -TimestampUrl $unsafeTimestampUrl
    } "Se acepto una URL RFC3161 insegura: $unsafeTimestampUrl"
}

Assert-Contains `
    -Path $helpers `
    -Expected @(
        "& `$SignToolPath sign",
        "/sha1 `$thumbprint",
        "/s My",
        "/fd SHA256",
        "/tr `$rfc3161Url",
        "/td SHA256",
        "& `$SignToolPath verify /pa /all /v",
        "TimeStamperCertificate",
        "Resolve-WindowsSignTool",
        "Get-WindowsAuthenticodeAlgorithms",
        "1.3.6.1.4.1.311.3.3.1",
        "RequireSha256Rfc3161"
    )
$helperContent = Get-Content -LiteralPath $helpers -Raw
$fileDigestOffset = $helperContent.IndexOf("/fd SHA256")
$rfc3161Offset = $helperContent.IndexOf("/tr `$rfc3161Url")
$timestampDigestOffset = $helperContent.IndexOf("/td SHA256")
Assert-True (
    $fileDigestOffset -ge 0 -and
    $rfc3161Offset -gt $fileDigestOffset -and
    $timestampDigestOffset -gt $rfc3161Offset
) "SignTool no fija SHA-256 y RFC3161 en el orden requerido"

$singleFileSigner = Join-Path $releaseScripts "sign-windows-file.ps1"
Assert-Contains `
    -Path $singleFileSigner `
    -Expected @(
        "ConfigurationPath",
        "Invoke-WindowsAuthenticodeSign",
        "Assert-WindowsAuthenticodeFile",
        "evidence_path",
        "Copy-Item"
    )

$finalizer = Join-Path $releaseScripts "finalize-windows-release.ps1"
Assert-Contains `
    -Path $finalizer `
    -Expected @(
        "Import-PfxCertificate",
        "Cert:\CurrentUser\My",
        "ConvertTo-SecureString",
        "Remove-Item",
        "-DeleteKey",
        "Remove-Item Env:WINDOWS_SIGNING_PFX_BASE64",
        "Remove-Item Env:WINDOWS_SIGNING_PFX_PASSWORD",
        "La limpieza de credenciales Authenticode temporales fallo",
        "Invoke-WindowsAuthenticodeSign",
        "Assert-WindowsAuthenticodeFile",
        "GRXFIRMA_UNINSTALL_SIGNER",
        "uninstallerEvidence",
        "NSIS no produjo evidencia del uninstaller firmado",
        "sign-windows-file.ps1",
        "windows-authenticode.ps1"
    )
$finalizerContent = Get-Content -LiteralPath $finalizer -Raw
Assert-True (
    -not $finalizerContent.Contains("Set-AuthenticodeSignature")
) "La release conserva el timestamp Authenticode legacy de PowerShell"
$signerHereStringStart = $finalizerContent.IndexOf(
    '$uninstallerSignerContent = @"'
)
$signerHereStringEnd = $finalizerContent.IndexOf(
    '"@',
    $signerHereStringStart
)
Assert-True (
    $signerHereStringStart -ge 0 -and
    $signerHereStringEnd -gt $signerHereStringStart
) "No se encontro el wrapper del uninstaller NSIS"
$signerHereString = $finalizerContent.Substring(
    $signerHereStringStart,
    $signerHereStringEnd - $signerHereStringStart
)
Assert-True (
    -not $signerHereString.Contains("PFX") -and
    -not $signerHereString.Contains("PASSWORD")
) "El wrapper NSIS expone el PFX o su password"

$verifier = Join-Path $releaseScripts "verify-windows-release.ps1"
Assert-Contains `
    -Path $verifier `
    -Expected @(
        "Assert-WindowsAuthenticodeFile",
        "Resolve-WindowsSignTool",
        "schema_version = 2",
        "file_digest_algorithm = `$algorithms.FileDigestAlgorithm",
        "timestamp_protocol = `$algorithms.TimestampProtocol",
        "timestamp_digest_algorithm = `$algorithms.TimestampDigestAlgorithm",
        "RequireSha256Rfc3161"
    )

$signingInclude = Join-Path $windowsPackaging "authenticode-signing.nsh"
Assert-Contains `
    -Path $signingInclude `
    -Expected @(
        "!ifdef GRXFIRMA_UNINSTALL_SIGNER",
        "!uninstfinalize",
        '"${GRXFIRMA_UNINSTALL_SIGNER}" "%1"'
    )
foreach ($nsi in Get-ChildItem `
    -LiteralPath $windowsPackaging `
    -File `
    -Filter "grxfirma-*.nsi") {
    Assert-Contains `
        -Path $nsi.FullName `
        -Expected @('!include "authenticode-signing.nsh"')
}

$releaseWorkflow = Join-Path $repositoryRoot ".github/workflows/release.yml"
$compatibilityWorkflow = Join-Path `
    $repositoryRoot `
    ".github/workflows/compatibility.yml"
Assert-Contains `
    -Path $releaseWorkflow `
    -Expected @(
        "runs-on: windows-2022",
        "choco install nsis --version 3.12.0 --yes --no-progress"
    )
Assert-Contains `
    -Path $compatibilityWorkflow `
    -Expected @(
        "runs-on: windows-2022",
        "choco install nsis --version 3.12.0 --yes --no-progress",
        "test-windows-authenticode-contracts.ps1"
    )
$releaseWorkflowContent = Get-Content -LiteralPath $releaseWorkflow -Raw
$compatibilityWorkflowContent = Get-Content `
    -LiteralPath $compatibilityWorkflow `
    -Raw
Assert-True (
    -not $releaseWorkflowContent.Contains("runs-on: windows-latest")
) "La release Windows conserva un runner flotante"
Assert-True (
    -not $compatibilityWorkflowContent.Contains("windows-latest")
) "La compatibilidad Windows conserva un runner flotante"
Assert-True (
    -not $releaseWorkflowContent.Contains("choco install nsis -y")
) "La release conserva una version NSIS flotante"

$artifactPolicy = Join-Path $releaseScripts "release_artifacts.py"
Assert-Contains `
    -Path $artifactPolicy `
    -Expected @(
        'windows_evidence.get("schema_version") != 2',
        'item.get("file_digest_algorithm") != "SHA256"',
        'item.get("timestamp_protocol") != "RFC3161"',
        'item.get("timestamp_digest_algorithm") != "SHA256"'
    )

$makensis = Get-Command makensis -ErrorAction SilentlyContinue
if ($null -eq $makensis) {
    if ($RequireNsis) {
        throw "makensis es obligatorio para probar !uninstfinalize."
    }
} else {
    $temporaryRoot = Join-Path `
        ([System.IO.Path]::GetTempPath()) `
        ("grxfirma NSIS signing hook " + [guid]::NewGuid().ToString("N"))
    $stage = Join-Path $temporaryRoot "stage"
    $output = Join-Path $temporaryRoot "signed-hook-setup.exe"
    $evidence = Join-Path $temporaryRoot "uninstaller-evidence.exe"
    New-Item -ItemType Directory -Path $stage -Force | Out-Null
    try {
        $productIcon = Join-Path `
            $repositoryRoot `
            "packaging/windows/grxfirma-grx.ico"
        foreach ($name in @(
            "grxfirma.exe",
            "README_CLI_WINDOWS.md",
            "VERSION.txt"
        )) {
            [System.IO.File]::WriteAllText(
                (Join-Path $stage $name),
                "fixture"
            )
        }
        Copy-Item `
            -LiteralPath $productIcon `
            -Destination (Join-Path $stage "grxfirma-grx.ico")

        $runningOnWindows =
            $PSVersionTable.PSEdition -eq "Desktop" -or
            $IsWindows -eq $true
        if ($runningOnWindows) {
            $signer = Join-Path $temporaryRoot "copy signed uninstaller.cmd"
            $signerContent = @"
@echo off
copy /Y "%~1" "$evidence" >nul
exit /b %ERRORLEVEL%
"@
            Set-Content `
                -LiteralPath $signer `
                -Value $signerContent `
                -Encoding ASCII
        } else {
            $signer = Join-Path $temporaryRoot "copy signed uninstaller.sh"
            $escapedEvidence = $evidence.Replace("'", "'\''")
            $signerContent = @"
#!/bin/sh
set -eu
cp "`$1" '$escapedEvidence'
"@
            [System.IO.File]::WriteAllText(
                $signer,
                $signerContent,
                [System.Text.UTF8Encoding]::new($false)
            )
            & chmod 700 $signer
            if ($LASTEXITCODE -ne 0) {
                throw "No se pudo preparar el hook NSIS de prueba."
            }
        }

        $definePrefix = if ($runningOnWindows) { "/D" } else { "-D" }
        $makensisArguments = @(
            "${definePrefix}VERSION=0.0.0-test",
            "${definePrefix}ARCH=amd64",
            "${definePrefix}STAGE_DIR=$stage",
            "${definePrefix}OUT_FILE=$output",
            "${definePrefix}GRXFIRMA_UNINSTALL_SIGNER=$signer",
            "${definePrefix}MUI_ICON=$productIcon",
            "${definePrefix}MUI_UNICON=$productIcon",
            (Join-Path $windowsPackaging "grxfirma-cli.nsi")
        )
        & $makensis.Source @makensisArguments |
            Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "makensis no pudo ejecutar el hook !uninstfinalize."
        }
        Assert-True (
            (Test-Path -LiteralPath $output -PathType Leaf) -and
            (Test-Path -LiteralPath $evidence -PathType Leaf)
        ) "NSIS no genero setup y evidencia mediante !uninstfinalize"
    } finally {
        if (Test-Path -LiteralPath $temporaryRoot) {
            Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
        }
    }
}

Write-Output "Windows Authenticode/NSIS contract tests passed."
