# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

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

function Assert-Rejects {
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

function Add-FixtureFile {
    param(
        [string]$Root,
        [string]$RelativePath,
        [string]$Content = "fixture"
    )
    $path = Join-Path $Root $RelativePath
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $path) |
        Out-Null
    [System.IO.File]::WriteAllText(
        $path,
        $Content,
        [System.Text.UTF8Encoding]::new($false)
    )
}

function Write-FixtureManifest {
    param([string]$WinUiStage)

    $manifestPath = Join-Path $WinUiStage "PUBLISH-MANIFEST.sha256"
    $lines = foreach ($file in Get-ChildItem `
        -LiteralPath $WinUiStage `
        -File `
        -Force `
        -Recurse |
        Where-Object { $_.FullName -ne $manifestPath } |
        Sort-Object FullName) {
        $relativePath = [System.IO.Path]::GetRelativePath(
            $WinUiStage,
            $file.FullName
        ).Replace('\', '/')
        $hash = (Get-FileHash `
            -LiteralPath $file.FullName `
            -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash *$relativePath"
    }
    [System.IO.File]::WriteAllLines(
        $manifestPath,
        $lines,
        [System.Text.UTF8Encoding]::new($false)
    )
}

$releaseRoot = Split-Path -Parent $PSScriptRoot
$layoutHelpers = Join-Path $releaseRoot "windows-suite-layout.ps1"
. $layoutHelpers

$temporaryRoot = Join-Path (
    [System.IO.Path]::GetTempPath()
) ("grxfirma-release-dual-gui-" + [guid]::NewGuid().ToString("N"))
try {
    $stage = Join-Path $temporaryRoot "suite"
    foreach ($fixture in @(
        [pscustomobject]@{ Path = "grxfirma-gui.exe"; Content = "canonical-before-signing" },
        [pscustomobject]@{ Path = "grxfirma.exe"; Content = "cli-before-signing" },
        [pscustomobject]@{ Path = "desktop-qt/grxfirma-gui.exe"; Content = "qt-backend-before-signing" },
        [pscustomobject]@{ Path = "desktop-qt/grxfirma-gui-qml.exe"; Content = "qt-frontend-before-signing" },
        [pscustomobject]@{ Path = "desktop-qt/Qt6Core.dll"; Content = "qt-runtime-before-signing" },
        [pscustomobject]@{ Path = "desktop-qt/vc_redist.x64.exe"; Content = "microsoft-redist" },
        [pscustomobject]@{ Path = "desktop-winui/README_DESKTOP_WINUI_WINDOWS.md"; Content = "readme" },
        [pscustomobject]@{ Path = "desktop-winui/VERSION.txt"; Content = "test" },
        [pscustomobject]@{ Path = "desktop-winui/app/grxfirma-gui.exe"; Content = "winui-backend-before-signing" },
        [pscustomobject]@{ Path = "desktop-winui/app/grxfirma-winui.exe"; Content = "winui-frontend-before-signing" },
        [pscustomobject]@{ Path = "desktop-winui/app/coreclr.dll"; Content = "coreclr-before-signing" },
        [pscustomobject]@{ Path = "desktop-winui/app/Microsoft.UI.Xaml.dll"; Content = "winui-runtime-before-signing" }
    )) {
        Add-FixtureFile `
            -Root $stage `
            -RelativePath $fixture.Path `
            -Content $fixture.Content
    }
    Write-FixtureManifest `
        -WinUiStage (Join-Path $stage "desktop-winui")

    $defines = @(
        Get-WindowsSuiteComponentDefines `
            -StageDirectory $stage `
            -DefinePrefix "/D"
    )
    Assert-True (
        $defines.Count -eq 2 -and
        $defines[0] -eq "/DHAS_WINUI=1" -and
        $defines[1] -eq "/DHAS_QT=1"
    ) "No se detectaron de forma determinista ambos componentes NSIS."

    $incompleteStage = Join-Path $temporaryRoot "incomplete"
    Add-FixtureFile `
        -Root $incompleteStage `
        -RelativePath "desktop-winui/app/grxfirma-winui.exe"
    Assert-Rejects {
        Get-WindowsSuiteComponentDefines `
            -StageDirectory $incompleteStage |
            Out-Null
    } "Se acepto una etapa que anunciaba WinUI pero estaba incompleta."

    $targets = @(
        Get-WindowsReleaseSigningTargets `
            -StageDirectory $stage `
            -Suite
    )
    $targetRelativePaths = @(
        $targets | ForEach-Object {
            [System.IO.Path]::GetRelativePath(
                $stage,
                $_.FullName
            ).Replace('\', '/')
        }
    )
    foreach ($requiredTarget in @(
        "grxfirma-gui.exe",
        "grxfirma.exe",
        "desktop-qt/grxfirma-gui-qml.exe",
        "desktop-qt/Qt6Core.dll",
        "desktop-winui/app/grxfirma-winui.exe",
        "desktop-winui/app/coreclr.dll",
        "desktop-winui/app/Microsoft.UI.Xaml.dll"
    )) {
        Assert-True (
            $targetRelativePaths -contains $requiredTarget
        ) "No se firmaria el objetivo requerido: $requiredTarget"
    }
    foreach ($forbiddenTarget in @(
        "desktop-qt/grxfirma-gui.exe",
        "desktop-winui/app/grxfirma-gui.exe",
        "desktop-qt/vc_redist.x64.exe"
    )) {
        Assert-True (
            $targetRelativePaths -notcontains $forbiddenTarget
        ) "El plan intentaria firmar el objetivo excluido: $forbiddenTarget"
    }
    Assert-True (
        @($targetRelativePaths | Where-Object {
            [System.IO.Path]::GetFileName($_) -eq "grxfirma-gui.exe"
        }).Count -eq 1
    ) "El launcher canonico no se firma exactamente una vez."

    # Simula el cambio de bytes que introduce Authenticode sin usar certificado.
    foreach ($target in $targets) {
        [System.IO.File]::AppendAllText(
            $target.FullName,
            "|authenticode",
            [System.Text.UTF8Encoding]::new($false)
        )
    }
    Sync-WindowsSuiteSharedBackend -StageDirectory $stage

    $canonicalHash = (Get-FileHash `
        -LiteralPath (Join-Path $stage "grxfirma-gui.exe") `
        -Algorithm SHA256).Hash
    foreach ($backendCopy in @(
        "desktop-qt/grxfirma-gui.exe",
        "desktop-winui/app/grxfirma-gui.exe"
    )) {
        Assert-True (
            (Get-FileHash `
                -LiteralPath (Join-Path $stage $backendCopy) `
                -Algorithm SHA256).Hash -eq $canonicalHash
        ) "El backend replicado no coincide byte a byte: $backendCopy"
    }
    Assert-WindowsWinUiPublishManifest `
        -WinUiStageDirectory (Join-Path $stage "desktop-winui")

    [System.IO.File]::AppendAllText(
        (Join-Path $stage "desktop-winui/app/coreclr.dll"),
        "|tampered",
        [System.Text.UTF8Encoding]::new($false)
    )
    Assert-Rejects {
        Assert-WindowsSuiteSharedBackend -StageDirectory $stage
    } "El verificador acepto un runtime WinUI posterior al manifiesto."

    $finalizer = Join-Path $releaseRoot "finalize-windows-release.ps1"
    $finalizerContent = Get-Content -LiteralPath $finalizer -Raw
    foreach ($fragment in @(
        "Get-WindowsSuiteComponentDefines",
        "Get-WindowsReleaseSigningTargets",
        "Sync-WindowsSuiteSharedBackend",
        "@nsisComponentDefines",
        '"/DHAS_WINUI=1"',
        '"/DHAS_QT=1"',
        "La Suite Windows oficial debe contener WinUI y Qt completos."
    )) {
        Assert-True (
            $finalizerContent.Contains($fragment)
        ) "El finalizador no integra el contrato dual: $fragment"
    }
    $signingOffset = $finalizerContent.IndexOf(
        'foreach ($portableExecutable in $signingTargets)'
    )
    $syncOffset = $finalizerContent.IndexOf(
        "Sync-WindowsSuiteSharedBackend"
    )
    $zipOffset = $finalizerContent.IndexOf(
        "Compress-Archive"
    )
    Assert-True (
        $signingOffset -ge 0 -and
        $syncOffset -gt $signingOffset -and
        $zipOffset -gt $syncOffset
    ) "La sincronizacion no ocurre despues de toda firma y antes del ZIP."

    $verifier = Join-Path $releaseRoot "verify-windows-release.ps1"
    $verifierContent = Get-Content -LiteralPath $verifier -Raw
    Assert-True (
        $verifierContent.Contains("Assert-WindowsSuiteSharedBackend") -and
        $verifierContent.Contains("Get-WindowsSuiteComponentDefines") -and
        $verifierContent.Contains("RequireDualGui") -and
        $verifierContent.Contains('"/DHAS_WINUI=1"') -and
        $verifierContent.Contains('"/DHAS_QT=1"')
    ) "El verificador oficial no comprueba layout, backend y manifiesto dual."

    $layoutContent = Get-Content -LiteralPath $layoutHelpers -Raw
    Assert-True (
        $layoutContent.Contains("Assert-WindowsReleaseStageSafe") -and
        $layoutContent.Contains("La etapa Windows contiene un punto de reanálisis")
    ) "La publicación oficial no rechaza puntos de reanálisis anidados."
} finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}

Write-Output "Windows dual GUI official finalizer contract tests passed."
