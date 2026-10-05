# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

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

    $definitions = @()
    foreach ($functionName in $FunctionNames) {
        $definition = $ast.FindAll({
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                $node.Name -eq $functionName
        }, $true) | Select-Object -First 1
        if ($null -eq $definition) {
            throw "Falta la funcion $functionName en $ScriptPath"
        }
        $definitions += $definition.Extent.Text
    }
    return $definitions
}

function Add-TestFile {
    param(
        [string]$Root,
        [string]$RelativePath
    )

    $path = Join-Path $Root $RelativePath
    $parent = Split-Path -Parent $path
    if (-not (Test-Path -LiteralPath $parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    [System.IO.File]::WriteAllText($path, "fixture")
}

function New-QtArtifactFixture {
    [System.Diagnostics.CodeAnalysis.SuppressMessageAttribute(
        "PSUseShouldProcessForStateChangingFunctions",
        "",
        Justification = "La funcion solo escribe dentro del directorio temporal aislado del test."
    )]
    param(
        [string]$Root,
        [string]$StageName,
        [ValidateSet("MSVC", "MinGW")]
        [string]$Layout,
        [switch]$Suite
    )

    $stage = Join-Path $Root $StageName
    New-Item -ItemType Directory -Force -Path $stage | Out-Null
    $qtRoot = if ($Suite) {
        Join-Path $stage "desktop-qt"
    } else {
        $stage
    }
    New-Item -ItemType Directory -Force -Path $qtRoot | Out-Null

    foreach ($file in @(
        "grxfirma-gui-qml.exe",
        "grxfirma-gui.exe",
        "grxfirma.exe",
        "install-desktop-qml.ps1",
        "uninstall-desktop-qml.ps1",
        "install-path-safety.ps1",
        "invoke-uninstall-silent.ps1",
        "README_DESKTOP_QML_WINDOWS.md",
        "VERSION.txt",
        "Qt6Core.dll",
        "Qt6Gui.dll",
        "Qt6Qml.dll",
        "Qt6Quick.dll",
        "platforms/qwindows.dll",
        "qml/main.qml",
        "assets/logo.png"
    )) {
        Add-TestFile -Root $qtRoot -RelativePath $file
    }

    $pluginRoot = if ($Layout -eq "MSVC") { "qml/QtRuntime" } else { "qt-qml/QtRuntime" }
    foreach ($plugin in @(
        "qmlplugin.dll",
        "qtquick2plugin.dll",
        "qtquickcontrols2plugin.dll",
        "qquicklayoutsplugin.dll",
        "qtquickdialogsplugin.dll",
        "qmlsettingsplugin.dll"
    )) {
        Add-TestFile -Root $qtRoot -RelativePath (Join-Path $pluginRoot $plugin)
    }

    if ($Layout -eq "MSVC") {
        Add-TestFile -Root $qtRoot -RelativePath "vc_redist.x64.exe"
    } else {
        foreach ($runtime in @("libgcc_s_seh-1.dll", "libstdc++-6.dll", "libwinpthread-1.dll")) {
            Add-TestFile -Root $qtRoot -RelativePath $runtime
        }
    }

    if ($Suite) {
        foreach ($file in @(
            "grxfirma.exe",
            "grxfirma-gui.exe",
            "grxfirma-nativehost.exe",
            "grxfirma-afirmauri.exe",
            "README_WINDOWS_SUITE.md",
            "install-suite.ps1",
            "install-nativehost.ps1",
            "install-afirmauri.ps1",
            "afirmauri-registration.ps1",
            "uninstall-suite.ps1",
            "uninstall-nativehost.ps1",
            "uninstall-afirmauri.ps1",
            "install-desktop-qml.ps1",
            "uninstall-desktop-qml.ps1",
            "install-desktop-winui.ps1",
            "uninstall-desktop-winui.ps1",
            "remove-unselected-desktop.ps1",
            "install-path-safety.ps1",
            "invoke-uninstall-silent.ps1",
            "VERSION.txt",
            "policies/GrxFirma.admx",
            "policies/es-ES/GrxFirma.adml",
            "policies/en-US/GrxFirma.adml",
            "help/NOVEDADES.md",
            "extensions/grxfirma-extension-chromium.zip",
            "extensions/grxfirma-extension-firefox.xpi",
            "extensions/grxfirma-extension-firefox.metadata.json"
        )) {
            Add-TestFile -Root $stage -RelativePath $file
        }
    }

    return $stage
}

function New-TestZip {
    [System.Diagnostics.CodeAnalysis.SuppressMessageAttribute(
        "PSUseShouldProcessForStateChangingFunctions",
        "",
        Justification = "La funcion solo escribe dentro del directorio temporal aislado del test."
    )]
    param(
        [string]$StageDir,
        [string]$ZipPath
    )

    if (Test-Path -LiteralPath $ZipPath) {
        Remove-Item -LiteralPath $ZipPath -Force
    }
    Compress-Archive -Path $StageDir -DestinationPath $ZipPath -Force
}

function Assert-Throw {
    param(
        [scriptblock]$Action,
        [string]$Message
    )

    $thrown = $false
    try {
        & $Action
    } catch {
        $thrown = $true
    }
    if (-not $thrown) {
        throw $Message
    }
}

$windowsDir = Split-Path -Parent $PSScriptRoot
$desktopScript = Join-Path $windowsDir "build-desktop-qml.ps1"
$suiteScript = Join-Path $windowsDir "build-suite.ps1"
$desktopFunctions = @(
    "Get-QtRuntimeRequirement",
    "Get-QtQmlPluginRequirement",
    "Test-CompilerRuntime",
    "Assert-QtRuntimeStage",
    "Get-ZipEntryName",
    "Assert-DesktopQmlZipArtifact"
)
$suiteFunctions = @(
    "Get-QtRuntimeRequirement",
    "Get-QtQmlPluginRequirement",
    "Test-CompilerRuntime",
    "Assert-QtRuntimeStage",
    "Get-ZipEntryName",
    "Assert-SuiteZipArtifact"
)

foreach ($definition in Get-ScriptFunctionDefinition -ScriptPath $desktopScript -FunctionNames $desktopFunctions) {
    . ([scriptblock]::Create($definition))
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-qt-validation-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    foreach ($layout in @("MSVC", "MinGW")) {
        $stageName = "desktop-$layout"
        $stage = New-QtArtifactFixture -Root $tmp -StageName $stageName -Layout $layout
        $zip = Join-Path $tmp "$stageName.zip"
        Assert-QtRuntimeStage -StageDir $stage
        New-TestZip -StageDir $stage -ZipPath $zip
        Assert-DesktopQmlZipArtifact -ZipPath $zip -StageName $stageName -ExpectHelp $false
    }

    $invalidStage = New-QtArtifactFixture -Root $tmp -StageName "desktop-invalid" -Layout "MSVC"
    Remove-Item -LiteralPath (Join-Path $invalidStage "platforms/qwindows.dll") -Force
    Assert-Throw {
        Assert-QtRuntimeStage -StageDir $invalidStage
    } "Se acepto una stage sin platforms/qwindows.dll"

    $invalidZipStage = New-QtArtifactFixture -Root $tmp -StageName "desktop-invalid-zip" -Layout "MSVC"
    Remove-Item -LiteralPath (Join-Path $invalidZipStage "Qt6Quick.dll") -Force
    $invalidZip = Join-Path $tmp "desktop-invalid-zip.zip"
    New-TestZip -StageDir $invalidZipStage -ZipPath $invalidZip
    Assert-Throw {
        Assert-DesktopQmlZipArtifact -ZipPath $invalidZip -StageName "desktop-invalid-zip" -ExpectHelp $false
    } "Se acepto un ZIP sin Qt6Quick.dll"

    $missingIpcStage = New-QtArtifactFixture -Root $tmp -StageName "desktop-no-ipc" -Layout "MSVC"
    Remove-Item -LiteralPath (Join-Path $missingIpcStage "grxfirma-gui.exe") -Force
    $missingIpcZip = Join-Path $tmp "desktop-no-ipc.zip"
    New-TestZip -StageDir $missingIpcStage -ZipPath $missingIpcZip
    Assert-Throw {
        Assert-DesktopQmlZipArtifact `
            -ZipPath $missingIpcZip `
            -StageName "desktop-no-ipc" `
            -ExpectHelp $false
    } "Se acepto un ZIP Desktop Qt sin el backend IPC"

    $missingRuntimeStage = New-QtArtifactFixture -Root $tmp -StageName "desktop-no-runtime" -Layout "MSVC"
    Remove-Item -LiteralPath (Join-Path $missingRuntimeStage "vc_redist.x64.exe") -Force
    Assert-Throw {
        Assert-QtRuntimeStage -StageDir $missingRuntimeStage
    } "Se acepto una stage sin runtime de compilador"

    $nestedRuntimeStage = New-QtArtifactFixture -Root $tmp -StageName "desktop-nested-runtime" -Layout "MSVC"
    Remove-Item -LiteralPath (Join-Path $nestedRuntimeStage "vc_redist.x64.exe") -Force
    Add-TestFile -Root $nestedRuntimeStage -RelativePath "qml/QtRuntime/vc_redist.x64.exe"
    Assert-Throw {
        Assert-QtRuntimeStage -StageDir $nestedRuntimeStage
    } "Se acepto un redistribuible MSVC fuera de la raiz de la stage"
    $nestedRuntimeZip = Join-Path $tmp "desktop-nested-runtime.zip"
    New-TestZip -StageDir $nestedRuntimeStage -ZipPath $nestedRuntimeZip
    Assert-Throw {
        Assert-DesktopQmlZipArtifact `
            -ZipPath $nestedRuntimeZip `
            -StageName "desktop-nested-runtime" `
            -ExpectHelp $false
    } "Se acepto un redistribuible MSVC fuera de la raiz del ZIP"

    $missingPluginStage = New-QtArtifactFixture -Root $tmp -StageName "desktop-no-plugin" -Layout "MinGW"
    Remove-Item -LiteralPath (Join-Path $missingPluginStage "qt-qml/QtRuntime/qmlsettingsplugin.dll") -Force
    $missingPluginZip = Join-Path $tmp "desktop-no-plugin.zip"
    New-TestZip -StageDir $missingPluginStage -ZipPath $missingPluginZip
    Assert-Throw {
        Assert-DesktopQmlZipArtifact -ZipPath $missingPluginZip -StageName "desktop-no-plugin" -ExpectHelp $false
    } "Se acepto un ZIP sin el plugin de Qt.labs.settings"

    foreach ($definition in Get-ScriptFunctionDefinition -ScriptPath $suiteScript -FunctionNames $suiteFunctions) {
        . ([scriptblock]::Create($definition))
    }

    foreach ($layout in @("MSVC", "MinGW")) {
        $stageName = "suite-$layout"
        $stage = New-QtArtifactFixture -Root $tmp -StageName $stageName -Layout $layout -Suite
        $zip = Join-Path $tmp "$stageName.zip"
        New-TestZip -StageDir $stage -ZipPath $zip
        Assert-SuiteZipArtifact `
            -ZipPath $zip `
            -StageName $stageName `
            -QtIntegrated $true `
            -QtHelpIntegrated $false `
            -WinUiIntegrated $false
    }

    $suiteWithoutIpc = New-QtArtifactFixture -Root $tmp -StageName "suite-no-ipc" -Layout "MSVC" -Suite
    Remove-Item -LiteralPath (Join-Path $suiteWithoutIpc "grxfirma-gui.exe") -Force
    $suiteWithoutIpcZip = Join-Path $tmp "suite-no-ipc.zip"
    New-TestZip -StageDir $suiteWithoutIpc -ZipPath $suiteWithoutIpcZip
    Assert-Throw {
        Assert-SuiteZipArtifact `
            -ZipPath $suiteWithoutIpcZip `
            -StageName "suite-no-ipc" `
            -QtIntegrated $true `
            -QtHelpIntegrated $false `
            -WinUiIntegrated $false
    } "Se acepto un ZIP de Suite con frontend Qt pero sin backend IPC"

} finally {
    if (Test-Path -LiteralPath $tmp) {
        Remove-Item -LiteralPath $tmp -Recurse -Force
    }
}

Write-Output "Windows Qt runtime validation tests passed."
