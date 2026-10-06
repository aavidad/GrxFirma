# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

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

function Add-FixtureFile {
    param(
        [string]$Root,
        [string]$RelativePath,
        [string]$Content = "fixture"
    )

    $path = Join-Path $Root $RelativePath
    $parent = Split-Path -Parent $path
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
    [System.IO.File]::WriteAllText(
        $path,
        $Content,
        [System.Text.UTF8Encoding]::new($false)
    )
}

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
        $definition.Extent.Text
    }
}

$windowsDir = Split-Path -Parent $PSScriptRoot
$repositoryRoot = Split-Path -Parent (Split-Path -Parent $windowsDir)
$suiteNsi = Join-Path $windowsDir "grxfirma-suite.nsi"
$suiteNsiContent = Get-Content -LiteralPath $suiteNsi -Raw
Assert-Contains -Path $suiteNsi -Expected @(
    "RequestExecutionLevel user",
    "!insertmacro MUI_PAGE_COMPONENTS",
    'Section "Motor, navegador y línea de comandos (obligatorio)" SEC_CORE',
    "SectionIn RO",
    "!ifdef HAS_WINUI",
    'Section "Interfaz nativa de Windows (WinUI 3) - recomendada" SEC_WINUI',
    "!ifdef HAS_QT",
    'Section /o "Interfaz multiplataforma Qt/QML (opcional)" SEC_QT',
    'File /r "${STAGE_DIR}\desktop-winui\*.*"',
    'File /r "${STAGE_DIR}\desktop-qt\*.*"',
    '-PackageDir "$INSTDIR\desktop-winui"',
    '-PackageDir "$INSTDIR\desktop-qt"',
    '-LauncherPath "$LOCALAPPDATA\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe"',
    "remove-unselected-desktop.ps1",
    "SectionGetFlags",
    "-KeepQt",
    "-KeepWinUi",
    'RMDir /r "$INSTDIR\desktop-winui"',
    'RMDir /r "$INSTDIR\desktop-qt"'
)
if ($suiteNsiContent -match "RequestExecutionLevel\s+admin") {
    throw "El instalador dual solicita elevacion administrativa."
}
if ($suiteNsiContent -match
    'Section\s+/o\s+"Interfaz nativa de Windows') {
    throw "WinUI no queda seleccionada por defecto cuando forma parte del instalador."
}
$uninstallerOffset = $suiteNsiContent.IndexOf(
    'WriteUninstaller "$INSTDIR\uninstall.exe"',
    [System.StringComparison]::Ordinal
)
$firstOptionalOffset = $suiteNsiContent.IndexOf(
    'Section "Interfaz nativa de Windows',
    [System.StringComparison]::Ordinal
)
if ($uninstallerOffset -lt 0 -or
    $firstOptionalOffset -lt 0 -or
    $uninstallerOffset -gt $firstOptionalOffset) {
    throw "Un fallo de GUI opcional puede ocurrir antes de dejar un desinstalador recuperable."
}
foreach ($componentBlock in @(
    '(?ms)Section "Interfaz nativa de Windows.*?nsExec::ExecToLog.*?!insertmacro GrxFirmaExitOnExecFailure.*?RMDir /r "\$INSTDIR\\desktop-winui".*?SectionEnd',
    '(?ms)Section /o "Interfaz multiplataforma Qt/QML.*?nsExec::ExecToLog.*?!insertmacro GrxFirmaExitOnExecFailure.*?RMDir /r "\$INSTDIR\\desktop-qt".*?SectionEnd'
)) {
    if ($suiteNsiContent -notmatch $componentBlock) {
        throw "La seccion GUI no conserva payload y desinstalador cuando falla."
    }
}

$qtInstaller = Join-Path $windowsDir "install-desktop-qml.ps1"
$winUiInstaller = Join-Path $windowsDir "install-desktop-winui.ps1"
$suiteInstaller = Join-Path $windowsDir "install-suite.ps1"
$suiteUninstaller = Join-Path $windowsDir "uninstall-suite.ps1"
Assert-Contains -Path $qtInstaller -Expected @(
    '$useSharedLauncher',
    'if (-not $useSharedLauncher)',
    '--frontend=qt --ui-binary="',
    "GrxFirma - Qt.lnk",
    "GrxFirma - Qt.lnk",
    "El backend Qt no coincide con el lanzador compartido instalado."
)
Assert-Contains -Path $winUiInstaller -Expected @(
    "PUBLISH-MANIFEST.sha256",
    'if ($item.Name -ieq "grxfirma-gui.exe")',
    '--frontend=winui --ui-binary="',
    "GrxFirma - Windows nativo.lnk",
    "GrxFirma - Windows nativo.lnk",
    "El backend WinUI no coincide con el lanzador compartido instalado."
)
Assert-Contains -Path $suiteInstaller -Expected @(
    '"DesktopLauncher"',
    '"desktop-qt"',
    '"desktop-winui"',
    "-ManagedBySuite",
    '$hasDesktopQt = (-not $CoreOnly) -and',
    '$hasDesktopWinUi = (-not $CoreOnly) -and',
    '$CoreOnly'
)
Assert-Contains -Path $suiteUninstaller -Expected @(
    '"DesktopQML"',
    '"DesktopWinUI"',
    '"DesktopLauncher"'
)
foreach ($installerPlan in @(
    [pscustomobject]@{
        Path = $qtInstaller
        CopyFragment = 'Copy-Item $exeSource'
    },
    [pscustomobject]@{
        Path = $winUiInstaller
        CopyFragment = 'foreach ($item in Get-ChildItem'
    }
)) {
    $installerContent = Get-Content -LiteralPath $installerPlan.Path -Raw
    $markerOffset = $installerContent.LastIndexOf(
        "Set-GrxFirmaSuiteOwnershipMarker",
        [System.StringComparison]::Ordinal
    )
    $copyOffset = $installerContent.IndexOf(
        $installerPlan.CopyFragment,
        [System.StringComparison]::Ordinal
    )
    if ($markerOffset -lt 0 -or $copyOffset -lt 0 -or $markerOffset -gt $copyOffset) {
        throw "Un fallo parcial no queda marcado para desinstalacion: $($installerPlan.Path)"
    }
}
$uninstallContent = Get-Content -LiteralPath $suiteUninstaller -Raw
foreach ($frontendComponent in @("DesktopQML", "DesktopWinUI")) {
    if ($uninstallContent.IndexOf(
        '"' + $frontendComponent + '"',
        [System.StringComparison]::Ordinal
    ) -gt $uninstallContent.LastIndexOf(
        '"DesktopLauncher"',
        [System.StringComparison]::Ordinal
    )) {
        throw "El lanzador compartido se elimina antes que $frontendComponent."
    }
}

foreach ($builder in @("build-suite.ps1", "build-suite.sh")) {
    Assert-Contains -Path (Join-Path $windowsDir $builder) -Expected @(
        "--with-winui",
        "--with-qt",
        "desktop-winui",
        "desktop-qt",
        "grxfirma-gui.exe",
        "HAS_WINUI",
        "HAS_QT"
    )
}
$shellBuilder = Get-Content -LiteralPath (Join-Path $windowsDir "build-suite.sh") -Raw
Assert-Contains -Path (Join-Path $windowsDir "build-suite.sh") -Expected @(
    '! -iname ''vc_redist.*.exe''',
    '! -path "${STAGE_DIR}/desktop-qt/grxfirma-gui.exe"',
    '! -path "${STAGE_DIR}/desktop-winui/app/grxfirma-gui.exe"',
    'cp "${canonical}" "${STAGE_DIR}/desktop-qt/grxfirma-gui.exe"',
    'cp "${canonical}" "${winui_stage}/app/grxfirma-gui.exe"',
    "app/grxfirma-gui.exe",
    'ensure_winui_stage_complete "${winui_stage}"'
)
$signOffset = $shellBuilder.LastIndexOf(
    "authenticode_sign_stage_binaries",
    [System.StringComparison]::Ordinal
)
$syncOffset = $shellBuilder.LastIndexOf(
    "synchronize_shared_backend_copies",
    [System.StringComparison]::Ordinal
)
$zipOffset = $shellBuilder.LastIndexOf(
    'grxfirma_reproducible_zip "${STAGE_DIR}"',
    [System.StringComparison]::Ordinal
)
if ($signOffset -lt 0 -or $syncOffset -lt $signOffset -or $zipOffset -lt $syncOffset) {
    throw "El build no sincroniza los backends despues de Authenticode y antes del ZIP."
}
Assert-Contains -Path (Join-Path $windowsDir "build-suite.ps1") -Expected @(
    "Sync-SharedDesktopBackends",
    "PUBLISH-MANIFEST.sha256",
    "El manifiesto WinUI no inventaria el backend compartido."
)

$temporaryRoot = Join-Path (
    [System.IO.Path]::GetTempPath()
) ("grxfirma-dual-gui-" + [guid]::NewGuid().ToString("N"))
$originalLocalAppData = $env:LOCALAPPDATA
$originalAppData = $env:APPDATA
try {
    $env:LOCALAPPDATA = Join-Path $temporaryRoot "LocalAppData"
    $env:APPDATA = Join-Path $temporaryRoot "AppData"
    $makensis = Get-Command "makensis" -ErrorAction SilentlyContinue
    if ($null -ne $makensis) {
        $nsisStage = Join-Path $temporaryRoot "nsis-stage"
        foreach ($relativePath in @(
            "grxfirma.exe",
            "grxfirma-gui.exe",
            "grxfirma-nativehost.exe",
            "grxfirma-afirmauri.exe",
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
            "README_WINDOWS_SUITE.md",
            "VERSION.txt",
            "extensions/fixture.txt",
            "policies/fixture.txt",
            "help/NOVEDADES.md",
            "desktop-qt/fixture.txt",
            "desktop-winui/fixture.txt"
        )) {
            Add-FixtureFile -Root $nsisStage -RelativePath $relativePath
        }
        Copy-Item `
            -LiteralPath (Join-Path $repositoryRoot "packaging/windows/grxfirma-grx.ico") `
            -Destination (Join-Path $nsisStage "grxfirma-grx.ico")
        $nsisOutput = Join-Path $temporaryRoot "dual-gui-setup.exe"
        & $makensis.Source `
            "-DVERSION=0.0.90" `
            "-DARCH=amd64" `
            "-DSTAGE_DIR=$nsisStage" `
            "-DOUT_FILE=$nsisOutput" `
            "-DHAS_WINUI=1" `
            "-DHAS_QT=1" `
            $suiteNsi *> (Join-Path $temporaryRoot "makensis.log")
        if ($LASTEXITCODE -ne 0 -or
            (-not (Test-Path -LiteralPath $nsisOutput -PathType Leaf))) {
            $nsisLog = Get-Content -LiteralPath (Join-Path $temporaryRoot "makensis.log") -Raw
            throw "No se pudo compilar el instalador NSIS dual: $nsisLog"
        }
        Write-Output "NSIS dual compiled with both GUI components."
    }

    $package = Join-Path $temporaryRoot "winui-package"
    $launcherDir = Join-Path $env:LOCALAPPDATA "Programs\GrxFirma\DesktopLauncher"
    New-Item -ItemType Directory -Force -Path $launcherDir | Out-Null

    foreach ($relativePath in @(
        "README_DESKTOP_WINUI_WINDOWS.md",
        "VERSION.txt",
        "app/grxfirma-winui.exe",
        "app/grxfirma-gui.exe",
        "app/coreclr.dll",
        "app/hostfxr.dll",
        "app/Microsoft.UI.Xaml.dll",
        "app/Microsoft.WindowsAppRuntime.dll",
        "app/help/NOVEDADES.md"
    )) {
        Add-FixtureFile -Root $package -RelativePath $relativePath
    }
    $fixtureAssets = Join-Path $package "app/Assets"
    New-Item -ItemType Directory -Force -Path $fixtureAssets | Out-Null
    Copy-Item `
        -LiteralPath (Join-Path $repositoryRoot "packaging/windows/grxfirma-grx.ico") `
        -Destination (Join-Path $fixtureAssets "grxfirma-grx.ico")
    $launcher = Join-Path $launcherDir "grxfirma-gui.exe"
    Copy-Item `
        -LiteralPath (Join-Path $package "app/grxfirma-gui.exe") `
        -Destination $launcher

    $manifestLines = @()
    foreach ($file in Get-ChildItem -LiteralPath $package -File -Force -Recurse) {
        $relativePath = $file.FullName.Substring($package.Length).TrimStart('\', '/').Replace('\', '/')
        $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $manifestLines += "$hash *$relativePath"
    }
    [System.IO.File]::WriteAllLines(
        (Join-Path $package "PUBLISH-MANIFEST.sha256"),
        $manifestLines,
        [System.Text.UTF8Encoding]::new($false)
    )

    $suiteBuilder = Join-Path $windowsDir "build-suite.ps1"
    foreach ($definition in Get-ScriptFunctionDefinition `
        -ScriptPath $suiteBuilder `
        -FunctionNames @("Assert-WinUiRuntimeStage", "Sync-SharedDesktopBackends")) {
        . ([scriptblock]::Create($definition))
    }
    $syncStage = Join-Path $temporaryRoot "signed-suite-stage"
    Add-FixtureFile `
        -Root $syncStage `
        -RelativePath "grxfirma-gui.exe" `
        -Content "canonical-signed-backend"
    Add-FixtureFile `
        -Root $syncStage `
        -RelativePath "desktop-qt/grxfirma-gui.exe" `
        -Content "qt-before-signing"
    Copy-Item `
        -LiteralPath $package `
        -Destination (Join-Path $syncStage "desktop-winui") `
        -Recurse
    [System.IO.File]::WriteAllText(
        (Join-Path $syncStage "desktop-winui/app/coreclr.dll"),
        "runtime-after-authenticode",
        [System.Text.UTF8Encoding]::new($false)
    )
    Sync-SharedDesktopBackends `
        -StageDir $syncStage `
        -QtIntegrated $true `
        -WinUiIntegrated $true
    $canonicalHash = (Get-FileHash `
        -LiteralPath (Join-Path $syncStage "grxfirma-gui.exe") `
        -Algorithm SHA256).Hash
    foreach ($backendCopy in @(
        (Join-Path $syncStage "desktop-qt/grxfirma-gui.exe"),
        (Join-Path $syncStage "desktop-winui/app/grxfirma-gui.exe")
    )) {
        if ((Get-FileHash -LiteralPath $backendCopy -Algorithm SHA256).Hash -ne
            $canonicalHash) {
            throw "El backend de payload diverge del launcher despues de firmar."
        }
    }

    & $winUiInstaller `
        -InstallDir (Join-Path $env:LOCALAPPDATA "Programs/GrxFirma/DesktopWinUI") `
        -PackageDir $package `
        -LauncherPath $launcher `
        -ValidateOnly
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA "Programs\GrxFirma\DesktopWinUI")) {
        throw "ValidateOnly modifico el destino WinUI."
    }

    $baseInstallDir = Join-Path $env:LOCALAPPDATA "Programs\GrxFirma"
    $selectionQtDir = Join-Path $baseInstallDir "DesktopQML"
    $selectionWinUiDir = Join-Path $baseInstallDir "DesktopWinUI"
    foreach ($component in @(
        [pscustomobject]@{ Path = $selectionQtDir; Name = "DesktopQML"; Payload = "grxfirma-gui-qml.exe" },
        [pscustomobject]@{ Path = $selectionWinUiDir; Name = "DesktopWinUI"; Payload = "grxfirma-winui.exe" }
    )) {
        Add-FixtureFile -Root $component.Path -RelativePath $component.Payload
        Add-FixtureFile `
            -Root $component.Path `
            -RelativePath ".grxfirma-install" `
            -Content "GrxFirma:$($component.Name)"
        Add-FixtureFile `
            -Root $component.Path `
            -RelativePath ".grxfirma-suite-component" `
            -Content "GrxFirma:Suite:$($component.Name)"
    }
    & (Join-Path $windowsDir "remove-unselected-desktop.ps1") `
        -BaseInstallDir $baseInstallDir `
        -KeepQt "0" `
        -KeepWinUi "1"
    if (Test-Path -LiteralPath $selectionQtDir) {
        throw "La actualizacion con Qt desmarcado conservo el componente de la suite."
    }
    if (-not (Test-Path -LiteralPath $selectionWinUiDir -PathType Container)) {
        throw "La actualizacion retiro WinUI aunque seguia seleccionado."
    }
    & (Join-Path $windowsDir "remove-unselected-desktop.ps1") `
        -BaseInstallDir $baseInstallDir `
        -KeepQt "0" `
        -KeepWinUi "0"
    if (Test-Path -LiteralPath $selectionWinUiDir) {
        throw "La actualizacion con WinUI desmarcado conservo el componente de la suite."
    }

    Add-FixtureFile -Root $selectionQtDir -RelativePath "grxfirma-gui-qml.exe"
    Add-FixtureFile `
        -Root $selectionQtDir `
        -RelativePath ".grxfirma-install" `
        -Content "GrxFirma:DesktopQML"
    & (Join-Path $windowsDir "remove-unselected-desktop.ps1") `
        -BaseInstallDir $baseInstallDir `
        -KeepQt "0" `
        -KeepWinUi "0"
    if (-not (Test-Path -LiteralPath $selectionQtDir -PathType Container)) {
        throw "La actualizacion retiro una instalacion Qt manual ajena a la suite."
    }

    if ([System.Environment]::OSVersion.Platform -eq
        [System.PlatformID]::Win32NT) {
        $winUiInstallDir = Join-Path $env:LOCALAPPDATA "Programs\GrxFirma\DesktopWinUI"
        & $winUiInstaller `
            -InstallDir $winUiInstallDir `
            -PackageDir $package `
            -LauncherPath $launcher `
            -ManagedBySuite
        if (-not (Test-Path -LiteralPath (Join-Path $winUiInstallDir "grxfirma-winui.exe") -PathType Leaf)) {
            throw "No se instalo el frontend WinUI."
        }
        if (Test-Path -LiteralPath (Join-Path $winUiInstallDir "grxfirma-gui.exe")) {
            throw "WinUI duplico fisicamente el backend compartido."
        }
        $shortcutPath = Join-Path `
            $env:APPDATA `
            "Microsoft\Windows\Start Menu\Programs\GrxFirma\GrxFirma - Windows nativo.lnk"
        if (-not (Test-Path -LiteralPath $shortcutPath -PathType Leaf)) {
            throw "No se creo el acceso directo WinUI."
        }
        $wshell = New-Object -ComObject WScript.Shell
        $shortcut = $wshell.CreateShortcut($shortcutPath)
        if (-not [string]::Equals(
            $shortcut.TargetPath,
            $launcher,
            [System.StringComparison]::OrdinalIgnoreCase
        ) -or $shortcut.Arguments -notlike
            '*--frontend=winui*--ui-binary=*grxfirma-winui.exe*') {
            throw "El acceso directo WinUI no usa el lanzador compartido."
        }
        $expectedIcon = (Join-Path `
            $winUiInstallDir `
            "Assets\grxfirma-grx.ico") + ",0"
        if (-not [string]::Equals(
            $shortcut.IconLocation,
            $expectedIcon,
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
            throw "El acceso directo WinUI no usa el icono de GrxFirma."
        }

        & (Join-Path $windowsDir "uninstall-desktop-winui.ps1") `
            -InstallDir $winUiInstallDir
        if (Test-Path -LiteralPath $winUiInstallDir) {
            throw "La desinstalacion WinUI dejo su directorio."
        }
        if (Test-Path -LiteralPath $shortcutPath) {
            throw "La desinstalacion WinUI dejo su acceso directo."
        }
    }

    Add-Content -LiteralPath (Join-Path $package "app/grxfirma-winui.exe") -Value "alterado"
    Assert-Throws {
        & $winUiInstaller `
            -InstallDir (Join-Path $env:LOCALAPPDATA "Programs/GrxFirma/DesktopWinUI") `
            -PackageDir $package `
            -LauncherPath $launcher `
            -ValidateOnly
    } "El instalador WinUI acepto un payload alterado."
} finally {
    $env:LOCALAPPDATA = $originalLocalAppData
    $env:APPDATA = $originalAppData
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}

Write-Output "Windows dual GUI suite contract tests passed."
