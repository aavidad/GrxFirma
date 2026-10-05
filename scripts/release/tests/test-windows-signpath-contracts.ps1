# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Contratos de la firma Windows con SignPath. No usa certificados: simula la
# tabla Authenticode que anade SignPath y comprueba que cualquier otro cambio
# se rechaza, que el hook NSIS solo acepta el uninstaller enviado y que el
# workflow mantiene las dos rondas, la descarga y la verificacion.

[CmdletBinding()]
param(
    [switch]$RequireNsis
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-Rejects {
    param([scriptblock]$Action, [string]$Message)
    try {
        & $Action
    } catch {
        return
    }
    throw $Message
}

function Assert-Contains {
    param([string]$Path, [string[]]$Expected)
    $content = Get-Content -LiteralPath $Path -Raw
    foreach ($fragment in $Expected) {
        if (-not $content.Contains($fragment)) {
            throw "Falta '$fragment' en $Path"
        }
    }
}

function New-FixturePe {
    param([int]$Length = 517)
    $bytes = [byte[]]::new($Length)
    for ($index = 0; $index -lt $Length; $index++) {
        $bytes[$index] = [byte](($index * 31 + 7) % 251)
    }
    $bytes[0] = 0x4d
    $bytes[1] = 0x5a
    [BitConverter]::GetBytes([uint32]0x80).CopyTo($bytes, 0x3c)
    [BitConverter]::GetBytes([uint32]0x00004550).CopyTo($bytes, 0x80)
    $optionalHeader = 0x80 + 24
    [BitConverter]::GetBytes([uint16]0x020b).CopyTo($bytes, $optionalHeader)
    [BitConverter]::GetBytes([uint32]0x11111111).CopyTo($bytes, $optionalHeader + 64)
    [BitConverter]::GetBytes([uint32]16).CopyTo($bytes, $optionalHeader + 108)
    $certificateDirectory = $optionalHeader + 112 + 32
    [BitConverter]::GetBytes([uint64]0).CopyTo($bytes, $certificateDirectory)
    return $bytes
}

function New-SimulatedSignature {
    param([byte[]]$Unsigned, [int]$BlobLength = 64)
    $aligned = [int]([math]::Ceiling($Unsigned.Length / 8.0) * 8)
    $signed = [byte[]]::new($aligned + $BlobLength)
    [Array]::Copy($Unsigned, $signed, $Unsigned.Length)
    [BitConverter]::GetBytes([uint32]$BlobLength).CopyTo($signed, $aligned)
    [BitConverter]::GetBytes([uint16]0x0200).CopyTo($signed, $aligned + 4)
    [BitConverter]::GetBytes([uint16]0x0002).CopyTo($signed, $aligned + 6)
    for ($index = $aligned + 8; $index -lt $signed.Length; $index++) {
        $signed[$index] = 0xa5
    }
    $optionalHeader = 0x80 + 24
    [BitConverter]::GetBytes([uint32]0x22222222).CopyTo($signed, $optionalHeader + 64)
    $certificateDirectory = $optionalHeader + 112 + 32
    [BitConverter]::GetBytes([uint32]$aligned).CopyTo($signed, $certificateDirectory)
    [BitConverter]::GetBytes([uint32]$BlobLength).CopyTo($signed, $certificateDirectory + 4)
    return $signed
}

$releaseScripts = Split-Path -Parent $PSScriptRoot
$repositoryRoot = Split-Path -Parent (Split-Path -Parent $releaseScripts)
. (Join-Path $releaseScripts 'windows-authenticode.ps1')
$hook = Join-Path $releaseScripts 'signpath-uninstaller-hook.ps1'

$temporaryRoot = Join-Path `
    ([System.IO.Path]::GetTempPath()) `
    ("grxfirma signpath contracts " + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $temporaryRoot | Out-Null
try {
    # 1. Copia firmada: solo se admite la tabla de certificados anadida.
    $unsignedPath = Join-Path $temporaryRoot 'unsigned.exe'
    $signedPath = Join-Path $temporaryRoot 'signed.exe'
    $unsigned = New-FixturePe
    [System.IO.File]::WriteAllBytes($unsignedPath, $unsigned)
    $signed = New-SimulatedSignature -Unsigned $unsigned
    [System.IO.File]::WriteAllBytes($signedPath, $signed)
    Assert-WindowsAuthenticodeSignedCopy `
        -UnsignedPath $unsignedPath `
        -SignedPath $signedPath

    $alignedUnsignedPath = Join-Path $temporaryRoot 'aligned-unsigned.exe'
    $alignedSignedPath = Join-Path $temporaryRoot 'aligned-signed.exe'
    $alignedUnsigned = New-FixturePe -Length 520
    [System.IO.File]::WriteAllBytes($alignedUnsignedPath, $alignedUnsigned)
    [System.IO.File]::WriteAllBytes(
        $alignedSignedPath,
        (New-SimulatedSignature -Unsigned $alignedUnsigned)
    )
    Assert-WindowsAuthenticodeSignedCopy `
        -UnsignedPath $alignedUnsignedPath `
        -SignedPath $alignedSignedPath

    $tamperCases = @(
        @{ Name = 'cuerpo'; Mutate = { param($b) $b[300] = [byte]($b[300] -bxor 0xff) } },
        @{ Name = 'cabecera DOS'; Mutate = { param($b) $b[2] = [byte]($b[2] -bxor 0x01) } },
        @{ Name = 'relleno'; Mutate = { param($b) $b[518] = 0x01 } }
    )
    foreach ($case in $tamperCases) {
        $copy = [byte[]]$signed.Clone()
        & $case.Mutate $copy
        $tamperedPath = Join-Path $temporaryRoot 'tampered.exe'
        [System.IO.File]::WriteAllBytes($tamperedPath, $copy)
        Assert-Rejects {
            Assert-WindowsAuthenticodeSignedCopy `
                -UnsignedPath $unsignedPath `
                -SignedPath $tamperedPath
        } "Se acepto una copia firmada con cambios en: $($case.Name)"
    }

    $trailingPath = Join-Path $temporaryRoot 'trailing.exe'
    [System.IO.File]::WriteAllBytes($trailingPath, [byte[]]($signed + [byte[]](0x00)))
    Assert-Rejects {
        Assert-WindowsAuthenticodeSignedCopy `
            -UnsignedPath $unsignedPath `
            -SignedPath $trailingPath
    } "Se acepto una copia con datos tras la tabla de certificados."

    $truncatedPath = Join-Path $temporaryRoot 'truncated.exe'
    [System.IO.File]::WriteAllBytes($truncatedPath, [byte[]]$unsigned[0..400])
    Assert-Rejects {
        Assert-WindowsAuthenticodeSignedCopy `
            -UnsignedPath $unsignedPath `
            -SignedPath $truncatedPath
    } "Se acepto una copia firmada truncada."

    Assert-Rejects {
        Assert-WindowsAuthenticodeSignedCopy `
            -UnsignedPath $signedPath `
            -SignedPath $signedPath
    } "Se acepto como original un PE que ya tenia tabla de certificados."

    # 2. Hook NSIS: captura sin modificar y sustitucion solo si coincide.
    $captureDirectory = Join-Path $temporaryRoot 'capture'
    New-Item -ItemType Directory -Force -Path $captureDirectory | Out-Null
    $generated = Join-Path $temporaryRoot 'generated-uninstaller.exe'
    [System.IO.File]::WriteAllBytes($generated, $unsigned)
    $captureConfiguration = Join-Path $temporaryRoot 'capture.json'
    $capturePath = Join-Path $captureDirectory 'uninstall.exe'
    [ordered]@{ schema_version = 1; mode = 'capture'; capture_path = $capturePath } |
        ConvertTo-Json | Set-Content -LiteralPath $captureConfiguration -Encoding UTF8
    & $hook -ConfigurationPath $captureConfiguration -FilePath $generated
    Assert-True (
        (Get-FileHash -LiteralPath $capturePath).Hash -eq
            (Get-FileHash -LiteralPath $generated).Hash
    ) "El hook no capturo el uninstaller exacto."
    Assert-Rejects {
        & $hook -ConfigurationPath $captureConfiguration -FilePath $generated
    } "El hook sobrescribio una captura previa."

    $evidence = Join-Path $temporaryRoot 'evidence.exe'
    $injectConfiguration = Join-Path $temporaryRoot 'inject.json'
    [ordered]@{
        schema_version = 1
        mode = 'inject'
        expected_unsigned_sha256 = (Get-FileHash -LiteralPath $capturePath).Hash.ToLowerInvariant()
        signed_path = $signedPath
        evidence_path = $evidence
    } | ConvertTo-Json | Set-Content -LiteralPath $injectConfiguration -Encoding UTF8
    & $hook -ConfigurationPath $injectConfiguration -FilePath $generated
    $signedHash = (Get-FileHash -LiteralPath $signedPath).Hash
    Assert-True (
        (Get-FileHash -LiteralPath $generated).Hash -eq $signedHash -and
        (Get-FileHash -LiteralPath $evidence).Hash -eq $signedHash
    ) "El hook no sustituyo el uninstaller por la copia firmada."

    $different = Join-Path $temporaryRoot 'different-uninstaller.exe'
    [System.IO.File]::WriteAllBytes($different, (New-FixturePe -Length 600))
    Assert-Rejects {
        & $hook -ConfigurationPath $injectConfiguration -FilePath $different
    } "El hook acepto un uninstaller distinto del enviado a SignPath."

    # 3. NSIS regenera el mismo uninstaller aunque cambie la carga util y
    #    acepta la sustitucion por la copia firmada.
    $makensis = Get-Command makensis -ErrorAction SilentlyContinue
    if ($null -eq $makensis) {
        if ($RequireNsis) {
            throw "makensis es obligatorio para probar el uninstaller determinista."
        }
    } else {
        $runningOnWindows = $PSVersionTable.PSEdition -eq 'Desktop' -or $IsWindows -eq $true
        $definePrefix = if ($runningOnWindows) { '/D' } else { '-D' }
        $powerShell = (Get-Process -Id $PID).Path
        $productIcon = Join-Path $repositoryRoot 'packaging/windows/grxfirma-diputacion.ico'
        $windowsPackaging = Join-Path $repositoryRoot 'packaging/windows'

        function New-HookWrapper {
            param([string]$Configuration, [string]$Name)
            if ($runningOnWindows) {
                $wrapper = Join-Path $temporaryRoot "$Name.cmd"
                Set-Content -LiteralPath $wrapper -Encoding ASCII -Value @"
@echo off
"$powerShell" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$hook" -ConfigurationPath "$Configuration" -FilePath "%~1"
exit /b %ERRORLEVEL%
"@
            } else {
                $wrapper = Join-Path $temporaryRoot "$Name.sh"
                $quote = { param($value) "'" + $value.Replace("'", "'\''") + "'" }
                [System.IO.File]::WriteAllText(
                    $wrapper,
                    "#!/bin/sh`nexec $(& $quote $powerShell) -NoLogo -NoProfile -NonInteractive -File $(& $quote $hook) -ConfigurationPath $(& $quote $Configuration) -FilePath `"`$1`"`n",
                    [System.Text.UTF8Encoding]::new($false)
                )
                & chmod 700 $wrapper
            }
            return $wrapper
        }

        function Invoke-FixtureNsis {
            param([string]$Stage, [string]$Output, [string]$Wrapper)
            & $makensis.Source `
                "${definePrefix}VERSION=0.0.0-test" `
                "${definePrefix}ARCH=amd64" `
                "${definePrefix}STAGE_DIR=$Stage" `
                "${definePrefix}OUT_FILE=$Output" `
                "${definePrefix}GRXFIRMA_UNINSTALL_SIGNER=$Wrapper" `
                "${definePrefix}MUI_ICON=$productIcon" `
                "${definePrefix}MUI_UNICON=$productIcon" `
                (Join-Path $windowsPackaging 'grxfirma-cli.nsi') |
                Out-Null
            return $LASTEXITCODE
        }

        $captures = @()
        foreach ($variant in @('unsigned-payload', 'signed-payload-with-more-bytes')) {
            $stage = Join-Path $temporaryRoot "stage-$variant"
            New-Item -ItemType Directory -Force -Path $stage | Out-Null
            [System.IO.File]::WriteAllText((Join-Path $stage 'grxfirma.exe'), $variant)
            [System.IO.File]::WriteAllText((Join-Path $stage 'README_CLI_WINDOWS.md'), 'fixture')
            [System.IO.File]::WriteAllText((Join-Path $stage 'VERSION.txt'), '0.0.0-test')
            Copy-Item -LiteralPath $productIcon -Destination (Join-Path $stage 'grxfirma-diputacion.ico')
            $capture = Join-Path $captureDirectory "nsis-$variant.exe"
            $configuration = Join-Path $temporaryRoot "nsis-$variant.json"
            [ordered]@{ schema_version = 1; mode = 'capture'; capture_path = $capture } |
                ConvertTo-Json | Set-Content -LiteralPath $configuration -Encoding UTF8
            $wrapper = New-HookWrapper -Configuration $configuration -Name "capture-$variant"
            $exitCode = Invoke-FixtureNsis `
                -Stage $stage `
                -Output (Join-Path $temporaryRoot "setup-$variant.exe") `
                -Wrapper $wrapper
            Assert-True ($exitCode -eq 0) "makensis fallo al capturar el uninstaller ($variant)."
            $captures += $capture
        }
        $capturedHash = (Get-FileHash -LiteralPath $captures[0]).Hash
        Assert-True (
            $capturedHash -eq (Get-FileHash -LiteralPath $captures[1]).Hash
        ) "NSIS no regenera el mismo uninstaller cuando cambia la carga util."

        $simulatedSigned = Join-Path $temporaryRoot 'nsis-signed-uninstaller.exe'
        [System.IO.File]::WriteAllBytes(
            $simulatedSigned,
            [byte[]]([System.IO.File]::ReadAllBytes($captures[0]) + [byte[]](1..16))
        )
        $nsisEvidence = Join-Path $temporaryRoot 'nsis-evidence.exe'
        $configuration = Join-Path $temporaryRoot 'nsis-inject.json'
        [ordered]@{
            schema_version = 1
            mode = 'inject'
            expected_unsigned_sha256 = $capturedHash.ToLowerInvariant()
            signed_path = $simulatedSigned
            evidence_path = $nsisEvidence
        } | ConvertTo-Json | Set-Content -LiteralPath $configuration -Encoding UTF8
        $wrapper = New-HookWrapper -Configuration $configuration -Name 'inject'
        $exitCode = Invoke-FixtureNsis `
            -Stage (Join-Path $temporaryRoot 'stage-signed-payload-with-more-bytes') `
            -Output (Join-Path $temporaryRoot 'setup-injected.exe') `
            -Wrapper $wrapper
        Assert-True ($exitCode -eq 0) "makensis no acepto el uninstaller firmado."
        Assert-True (
            (Get-FileHash -LiteralPath $nsisEvidence).Hash -eq
                (Get-FileHash -LiteralPath $simulatedSigned).Hash
        ) "La evidencia NSIS no es el uninstaller firmado."

        [ordered]@{
            schema_version = 1
            mode = 'inject'
            expected_unsigned_sha256 = ('0' * 64)
            signed_path = $simulatedSigned
            evidence_path = (Join-Path $temporaryRoot 'nsis-evidence-mismatch.exe')
        } | ConvertTo-Json | Set-Content -LiteralPath $configuration -Encoding UTF8
        # makensis ignora el codigo de salida de !uninstfinalize; el
        # finalizador exige la evidencia, que el hook solo escribe si coincide.
        [void](Invoke-FixtureNsis `
            -Stage (Join-Path $temporaryRoot 'stage-unsigned-payload') `
            -Output (Join-Path $temporaryRoot 'setup-mismatch.exe') `
            -Wrapper $wrapper)
        Assert-True (
            -not (Test-Path -LiteralPath (Join-Path $temporaryRoot 'nsis-evidence-mismatch.exe'))
        ) "El hook dejo evidencia de un uninstaller que no coincide con el firmado."
    }
} finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}

# 4. Contratos estaticos del finalizador y del workflow.
$finalizer = Join-Path $releaseScripts 'finalize-windows-release-signpath.ps1'
Assert-Contains -Path $finalizer -Expected @(
    "ValidateSet('Prepare', 'Installers', 'Complete')",
    'Get-WindowsReleaseSigningTargets',
    'Assert-WindowsAuthenticodeSignedCopy',
    'Assert-WindowsAuthenticodeFile',
    '-RequireSha256Rfc3161',
    'Assert-ExactFileSet',
    'Sync-WindowsSuiteSharedBackend',
    'La etapa cambio mientras SignPath firmaba',
    "mode = 'capture'",
    "mode = 'inject'",
    'verify-windows-release.ps1',
    '-WriteManifest',
    'SHA256SUMS-windows.txt',
    '-RequireEvidence',
    'NSIS no produjo evidencia del uninstaller firmado',
    'Remove-Item -LiteralPath $evidence -Force',
    'La Suite Windows oficial debe contener WinUI y Qt completos.'
)
$finalizerContent = Get-Content -LiteralPath $finalizer -Raw
foreach ($forbidden in @('PFX', 'PASSWORD', 'SIGNPATH_API_TOKEN', 'Set-AuthenticodeSignature')) {
    Assert-True (
        -not $finalizerContent.Contains($forbidden)
    ) "El finalizador SignPath no debe manejar credenciales: $forbidden"
}
$applyOffset = $finalizerContent.IndexOf('Copy-Item -LiteralPath $signedPath -Destination $stageFile')
$verifyOffset = $finalizerContent.IndexOf('Assert-SignPathReturn `', $finalizerContent.IndexOf("'Installers' {"))
$syncOffset = $finalizerContent.IndexOf('Sync-WindowsSuiteSharedBackend -StageDirectory')
$zipOffset = $finalizerContent.IndexOf('Compress-Archive', $syncOffset)
Assert-True (
    $verifyOffset -ge 0 -and
    $applyOffset -gt $verifyOffset -and
    $syncOffset -gt $applyOffset -and
    $zipOffset -gt $syncOffset
) "El finalizador SignPath no verifica antes de aplicar o no replica antes del ZIP."

$workflow = Join-Path $repositoryRoot '.github/workflows/release.yml'
Assert-Contains -Path $workflow -Expected @(
    'select-windows-signing-mode.sh',
    'artifact-configuration-slug: windows-payload',
    'artifact-configuration-slug: windows-installers',
    'output-artifact-directory: ${{ runner.temp }}/signpath/round1-signed',
    'output-artifact-directory: ${{ runner.temp }}/signpath/round2-signed',
    'github-artifact-id: ${{ steps.signpath-round1-unsigned.outputs.artifact-id }}',
    'github-artifact-id: ${{ steps.signpath-round2-unsigned.outputs.artifact-id }}',
    '-Phase Prepare',
    '-Phase Installers',
    '-Phase Complete'
)

Write-Output "Windows SignPath contract tests passed."
