# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

function Get-ScriptFunctionDefinition {
    param(
        [string]$ScriptPath,
        [string[]]$FunctionNames
    )

    $tokens = $null
    $parseErrors = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseFile(
        $ScriptPath,
        [ref]$tokens,
        [ref]$parseErrors
    )
    if ($parseErrors.Count -gt 0) {
        throw "No se pudo analizar $ScriptPath"
    }
    foreach ($functionName in $FunctionNames) {
        $definition = $ast.FindAll({
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                $node.Name -eq $functionName
        }, $true) | Select-Object -First 1
        if ($null -eq $definition) {
            throw "Falta la funcion $functionName en $ScriptPath"
        }
        Write-Output $definition.Extent.Text
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

function Write-TestPe {
    param(
        [string]$Path,
        [uint16]$Machine = 0x8664
    )

    $bytes = [byte[]]::new(512)
    $bytes[0] = 0x4D
    $bytes[1] = 0x5A
    [BitConverter]::GetBytes([uint32]0x80).CopyTo($bytes, 0x3C)
    $bytes[0x80] = 0x50
    $bytes[0x81] = 0x45
    $bytes[0x82] = 0
    $bytes[0x83] = 0
    [BitConverter]::GetBytes($Machine).CopyTo($bytes, 0x84)
    [System.IO.File]::WriteAllBytes($Path, $bytes)
}

$windowsDirectory = Split-Path -Parent $PSScriptRoot
$buildScript = Join-Path $windowsDirectory "build-desktop-winui.ps1"
$scriptContent = Get-Content -LiteralPath $buildScript -Raw

foreach ($required in @(
    '10.0.302',
    'Microsoft.WindowsAppSDK',
    '2.3.1',
    '--locked-mode',
    '--runtime $Script:RuntimeIdentifier',
    '--self-contained true',
    '-p:WindowsPackageType=None',
    '-p:EnableMsixTooling=true',
    '-p:WindowsAppSDKSelfContained=true',
    '-p:PublishSingleFile=false',
    '-p:PublishTrimmed=false',
    '-p:PublishReadyToRun=false',
    'Invoke-GrxFirmaGoBuild',
    'PUBLISH-MANIFEST.sha256'
)) {
    if (-not $scriptContent.Contains($required)) {
        throw "Falta el contrato '$required' en build-desktop-winui.ps1"
    }
}
foreach ($forbidden in @("makensis", "build-suite.ps1", "ConvertTo-SecureString")) {
    if ($scriptContent.Contains($forbidden)) {
        throw "El build WinUI aislado contiene integracion no autorizada: $forbidden"
    }
}

foreach ($definition in Get-ScriptFunctionDefinition `
    -ScriptPath $buildScript `
    -FunctionNames @(
        "Assert-GrxFirmaPathWithinRoot",
        "Assert-GrxFirmaNoReparsePoints",
        "Get-GrxFirmaPeMachine",
        "Assert-GrxFirmaStageSecrets",
        "Assert-GrxFirmaWinUiPublication",
        "Write-GrxFirmaPublishManifest",
        "Assert-GrxFirmaPublishManifest"
    )) {
    . ([scriptblock]::Create($definition))
}

$temporaryRoot = Join-Path (
    [System.IO.Path]::GetTempPath()
) ("grxfirma-winui-contract-" + [Guid]::NewGuid().ToString("N"))
$stage = Join-Path $temporaryRoot "stage"
$app = Join-Path $stage "app"
New-Item -ItemType Directory -Force -Path $app | Out-Null

try {
    [System.IO.File]::WriteAllText(
        (Join-Path $stage "README_DESKTOP_WINUI_WINDOWS.md"),
        "fixture"
    )
    [System.IO.File]::WriteAllText((Join-Path $stage "VERSION.txt"), "test")
    [System.IO.File]::WriteAllText((Join-Path $app "TestApp.dll"), "managed")
    [System.IO.File]::WriteAllText((Join-Path $app "TestApp.pri"), "pri")
    $helpDirectory = Join-Path $app "help"
    New-Item -ItemType Directory -Force -Path $helpDirectory | Out-Null
    $userGuide = Join-Path $helpDirectory "guia-usuario.txt"
    [System.IO.File]::WriteAllText(
        $userGuide,
        ("Guia local de usuario. " * 16)
    )
    $releaseNotes = Join-Path $helpDirectory "NOVEDADES.md"
    [System.IO.File]::WriteAllText(
        $releaseNotes,
        "## 2.0.2 — 2026-09-26`n`n- Cambio de prueba.`n"
    )
    foreach ($runtime in @(
        "coreclr.dll",
        "hostfxr.dll",
        "hostpolicy.dll",
        "Microsoft.UI.Xaml.dll",
        "Microsoft.WindowsAppRuntime.dll"
    )) {
        [System.IO.File]::WriteAllText((Join-Path $app $runtime), "runtime")
    }
    [System.IO.File]::WriteAllText(
        (Join-Path $app "TestApp.runtimeconfig.json"),
        '{"runtimeOptions":{"tfm":"net10.0","includedFrameworks":[{"name":"Microsoft.NETCore.App","version":"10.0.10"}]}}'
    )
    [System.IO.File]::WriteAllText(
        (Join-Path $app "TestApp.deps.json"),
        '{"runtimeTarget":{"name":".NETCoreApp,Version=v10.0/win-x64"},"libraries":{"Microsoft.WindowsAppSDK/2.3.1":{"type":"package"}}}'
    )
    Write-TestPe -Path (Join-Path $app "TestApp.exe")
    Write-TestPe -Path (Join-Path $app "grxfirma-gui.exe")

    Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    Write-GrxFirmaPublishManifest -StageDirectory $stage
    $manifest = Join-Path $stage "PUBLISH-MANIFEST.sha256"
    if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) {
        throw "No se genero el manifiesto de publicacion WinUI"
    }
    $manifestContent = Get-Content -LiteralPath $manifest -Raw
    foreach ($requiredEntry in @(
        "app/TestApp.exe",
        "app/grxfirma-gui.exe",
        "app/TestApp.pri",
        "app/help/guia-usuario.txt",
        "app/help/NOVEDADES.md",
        "VERSION.txt"
    )) {
        if (-not $manifestContent.Contains($requiredEntry)) {
            throw "El manifiesto no contiene $requiredEntry"
        }
    }
    Assert-GrxFirmaPublishManifest -StageDirectory $stage

    $managedEntryPoint = Join-Path $app "TestApp.dll"
    $validManagedEntryPoint = [System.IO.File]::ReadAllBytes($managedEntryPoint)
    [System.IO.File]::WriteAllText($managedEntryPoint, "tampered")
    Assert-Throws {
        Assert-GrxFirmaPublishManifest -StageDirectory $stage
    } "Se acepto un fichero manipulado despues de generar el manifiesto"
    [System.IO.File]::WriteAllBytes($managedEntryPoint, $validManagedEntryPoint)
    Assert-GrxFirmaPublishManifest -StageDirectory $stage

    $validUserGuide = [System.IO.File]::ReadAllBytes($userGuide)
    Remove-Item -LiteralPath $userGuide -Force
    Assert-Throws {
        Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    } "Se acepto una publicacion WinUI sin guia local"
    [System.IO.File]::WriteAllBytes($userGuide, $validUserGuide)
    Assert-GrxFirmaPublishManifest -StageDirectory $stage

    $validReleaseNotes = [System.IO.File]::ReadAllBytes($releaseNotes)
    Remove-Item -LiteralPath $releaseNotes -Force
    Assert-Throws {
        Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    } "Se acepto una publicacion WinUI sin novedades"
    [System.IO.File]::WriteAllBytes($releaseNotes, $validReleaseNotes)
    Assert-GrxFirmaPublishManifest -StageDirectory $stage

    $resourcesPri = Join-Path $app "TestApp.pri"
    $validResourcesPri = [System.IO.File]::ReadAllBytes($resourcesPri)
    Remove-Item -LiteralPath $resourcesPri -Force
    Assert-Throws {
        Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    } "Se acepto una publicacion WinUI sin el indice PRI de la aplicacion"
    [System.IO.File]::WriteAllBytes($resourcesPri, $validResourcesPri)
    Assert-GrxFirmaPublishManifest -StageDirectory $stage

    $runtimeConfig = Join-Path $app "TestApp.runtimeconfig.json"
    $validRuntimeConfig = Get-Content -LiteralPath $runtimeConfig -Raw
    [System.IO.File]::WriteAllText(
        $runtimeConfig,
        '{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.10"}}}'
    )
    Assert-Throws {
        Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    } "Se acepto una publicacion framework-dependent"
    [System.IO.File]::WriteAllText($runtimeConfig, $validRuntimeConfig)

    $secretPath = Join-Path $stage ".env"
    [System.IO.File]::WriteAllText($secretPath, "GRXFIRMA_PASSWORD=test")
    Assert-Throws {
        Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    } "Se acepto un .env dentro del stage"
    Remove-Item -LiteralPath $secretPath -Force

    $tlsLog = Join-Path $stage "debug.log"
    [System.IO.File]::WriteAllText($tlsLog, "CLIENT_RANDOM deadbeef secret")
    Assert-Throws {
        Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    } "Se acepto un key log TLS dentro del stage"
    Remove-Item -LiteralPath $tlsLog -Force

    Write-TestPe -Path (Join-Path $app "TestApp.exe") -Machine 0x014C
    Assert-Throws {
        Assert-GrxFirmaWinUiPublication -StageDirectory $stage
    } "Se acepto un frontend WinUI x86"
    Write-TestPe -Path (Join-Path $app "TestApp.exe")

    Assert-Throws {
        Assert-GrxFirmaPathWithinRoot `
            -Path (Join-Path $temporaryRoot "../outside") `
            -Root $stage
    } "Se acepto una ruta fuera del stage"

    $linkPath = Join-Path $stage "runtime-link.dll"
    $linkCreated = $false
    try {
        New-Item `
            -ItemType SymbolicLink `
            -Path $linkPath `
            -Target (Join-Path $app "coreclr.dll") `
            -ErrorAction Stop | Out-Null
        $linkCreated = $true
    } catch {
        Write-Warning "El host no permite crear symlinks; se omite esa asercion dinamica."
    }
    if ($linkCreated) {
        Assert-Throws {
            Assert-GrxFirmaWinUiPublication -StageDirectory $stage
        } "Se acepto un symlink dentro del stage"
        Remove-Item -LiteralPath $linkPath -Force
    }
} finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}

Write-Output "Windows WinUI build contract tests passed."
