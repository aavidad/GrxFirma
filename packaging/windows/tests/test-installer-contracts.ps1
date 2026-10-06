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
    if (-not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    [System.IO.File]::WriteAllText($path, "fixture")
}

function Assert-Throws {
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

function Assert-NsisExecFailureContract {
    param([string]$Path)

    $content = Get-Content -LiteralPath $Path -Raw
    $execCount = [regex]::Matches(
        $content,
        '(?m)^\s*nsExec::ExecToLog\b'
    ).Count
    $guardCount = [regex]::Matches(
        $content,
        '(?m)^\s*!insertmacro GrxFirmaExitOnExecFailure\b'
    ).Count
    if ($execCount -eq 0) {
        throw "$Path no contiene ejecuciones externas que validar"
    }
    if ($guardCount -ne $execCount) {
        throw "$Path protege $guardCount de $execCount ejecuciones externas"
    }
    if ($content -match '(?m)^\s*MessageBox\s') {
        throw "$Path conserva un MessageBox directo que puede bloquear el modo /S"
    }
}

$script:MockAuthenticodeValid = $true
function Get-AuthenticodeSignature {
    param(
        [string]$LiteralPath
    )
    if ($script:MockAuthenticodeValid) {
        return [pscustomobject]@{
            Status = "Valid"
            SignerCertificate = [pscustomobject]@{
                Subject = "CN=Microsoft Windows, O=Microsoft Corporation, L=Redmond"
            }
        }
    }
    return [pscustomobject]@{
        Status = "HashMismatch"
        SignerCertificate = $null
    }
}

$script:MsvcProcessExitCode = 0
$script:MsvcProcessInvocations = @()
function Start-Process {
    [CmdletBinding()]
    param(
        [string]$FilePath,
        [string[]]$ArgumentList,
        [switch]$Wait,
        [switch]$PassThru
    )

    $script:MsvcProcessInvocations += [pscustomobject]@{
        FilePath = $FilePath
        ArgumentList = @($ArgumentList)
        Wait = $Wait.IsPresent
        PassThru = $PassThru.IsPresent
    }
    return [pscustomobject]@{
        ExitCode = $script:MsvcProcessExitCode
    }
}

$windowsDir = Split-Path -Parent $PSScriptRoot
$pathSafety = Join-Path $windowsDir "install-path-safety.ps1"
foreach ($definition in Get-ScriptFunctionDefinition `
    -ScriptPath (Join-Path $windowsDir "reproducible-build.ps1") `
    -FunctionNames @("Resolve-GrxFirmaMakeNsis")) {
    . ([scriptblock]::Create($definition))
}
$originalEnginePSModulePath = $env:PSModulePath
$env:PSModulePath = Join-Path `
    ([System.IO.Path]::GetTempPath()) `
    "grxfirma-invalid-module-path"
. $pathSafety
$expectedTrustedModulePath = [System.IO.Path]::GetFullPath(
    (Join-Path $PSHOME "Modules")
)
if (-not [string]::Equals(
    $env:PSModulePath,
    $expectedTrustedModulePath,
    [System.StringComparison]::OrdinalIgnoreCase
)) {
    throw "La instalacion no confina PSModulePath al motor activo"
}
$desktopInstaller = Join-Path $windowsDir "install-desktop-qml.ps1"
foreach ($definition in Get-ScriptFunctionDefinition `
    -ScriptPath $desktopInstaller `
    -FunctionNames @(
        "Get-DesktopPackageFileNames",
        "Assert-DesktopPackageRuntime",
        "Assert-MsvcRedistributableSignature",
        "Install-MsvcRedistributable"
    )) {
    . ([scriptblock]::Create($definition))
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-win-contract-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
$originalLocalAppData = $env:LOCALAPPDATA
try {
    $env:LOCALAPPDATA = Join-Path $tmp "LocalAppData"
    New-Item -ItemType Directory -Path $env:LOCALAPPDATA | Out-Null

    $originalMakeNsis = $env:MAKENSIS
    $originalPath = $env:PATH
    $originalProgramFiles = $env:ProgramFiles
    $originalProgramFilesX86 = ${env:ProgramFiles(x86)}
    try {
        $programFilesX86 = Join-Path $tmp "ProgramFilesX86"
        $expectedMakeNsis = Join-Path $programFilesX86 "NSIS/makensis.exe"
        Add-TestFile -Root $programFilesX86 -RelativePath "NSIS/makensis.exe"
        Remove-Item Env:MAKENSIS -ErrorAction SilentlyContinue
        $env:PATH = Join-Path $tmp "empty-path"
        $env:ProgramFiles = Join-Path $tmp "ProgramFiles"
        ${env:ProgramFiles(x86)} = $programFilesX86
        $resolvedMakeNsis = Resolve-GrxFirmaMakeNsis
        if (-not [string]::Equals(
            $resolvedMakeNsis,
            [System.IO.Path]::GetFullPath($expectedMakeNsis),
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
            throw "El resolver NSIS no encontró la instalación estándar fuera de PATH"
        }

        $explicitMakeNsis = Join-Path $tmp "custom-tools/makensis.exe"
        Add-TestFile -Root $tmp -RelativePath "custom-tools/makensis.exe"
        $env:MAKENSIS = $explicitMakeNsis
        $resolvedMakeNsis = Resolve-GrxFirmaMakeNsis
        if (-not [string]::Equals(
            $resolvedMakeNsis,
            [System.IO.Path]::GetFullPath($explicitMakeNsis),
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
            throw "El resolver NSIS no respetó MAKENSIS antes que la ruta estándar"
        }

        $env:MAKENSIS = Join-Path $tmp "missing-makensis.exe"
        Assert-Throws `
            -Action { Resolve-GrxFirmaMakeNsis } `
            -Message "El resolver NSIS aceptó un MAKENSIS explícito inexistente"
    } finally {
        $env:MAKENSIS = $originalMakeNsis
        $env:PATH = $originalPath
        $env:ProgramFiles = $originalProgramFiles
        ${env:ProgramFiles(x86)} = $originalProgramFilesX86
    }

    $requiredQtRuntime = @(
        "Qt6Core.dll",
        "Qt6Gui.dll",
        "Qt6Qml.dll",
        "Qt6Quick.dll",
        "platforms/qwindows.dll"
    )
    $requiredQmlPlugins = @(
        "qmlplugin.dll",
        "qtquick2plugin.dll",
        "qtquickcontrols2plugin.dll",
        "qquicklayoutsplugin.dll",
        "qtquickdialogsplugin.dll",
        "qmlsettingsplugin.dll"
    )
    $requiredMinGwRuntime = @(
        "libgcc_s_seh-1.dll",
        "libstdc++-6.dll",
        "libwinpthread-1.dll"
    )

    foreach ($layout in @("MSVC", "MinGW")) {
        $baseDir = Join-Path $tmp $layout
        New-Item -ItemType Directory -Path $baseDir | Out-Null
        foreach ($file in $requiredQtRuntime) {
            Add-TestFile -Root $baseDir -RelativePath $file
        }
        foreach ($plugin in $requiredQmlPlugins) {
            Add-TestFile -Root $baseDir -RelativePath (Join-Path "qml/QtRuntime" $plugin)
        }
        if ($layout -eq "MSVC") {
            Add-TestFile -Root $baseDir -RelativePath "vc_redist.x64.exe"
            $redistributables = @(Assert-DesktopPackageRuntime)
            if ($redistributables.Count -ne 1 -or $redistributables[0].Name -ne "vc_redist.x64.exe") {
                throw "El runtime MSVC valido no se detecto correctamente"
            }
        } else {
            foreach ($runtime in $requiredMinGwRuntime) {
                Add-TestFile -Root $baseDir -RelativePath $runtime
            }
            if (@(Assert-DesktopPackageRuntime).Count -ne 0) {
                throw "El runtime MinGW se confundio con un redistribuible MSVC"
            }
        }
    }

    $baseDir = Join-Path $tmp "MSVC"
    $msvcInstaller = Get-Item -LiteralPath (Join-Path $baseDir "vc_redist.x64.exe")
    $originalPSModulePath = $env:PSModulePath
    try {
        $env:PSModulePath = Join-Path $tmp "invalid-module-path"
        foreach ($acceptedExitCode in @(0, 1638, 3010)) {
            $script:MsvcProcessExitCode = $acceptedExitCode
            Install-MsvcRedistributable -Installers @($msvcInstaller)
        }
    } finally {
        $env:PSModulePath = $originalPSModulePath
    }
    if ($script:MsvcProcessInvocations.Count -ne 3) {
        throw "No se ejecutaron todas las pruebas del redistribuible MSVC"
    }
    foreach ($invocation in $script:MsvcProcessInvocations) {
        if (-not $invocation.Wait -or -not $invocation.PassThru) {
            throw "El instalador MSVC debe esperar y devolver su codigo de salida"
        }
        if (($invocation.ArgumentList -join " ") -ne
            "/install /quiet /norestart") {
            throw "El instalador MSVC no usa los argumentos silenciosos esperados"
        }
    }
    $script:MsvcProcessExitCode = 5
    Assert-Throws {
        Install-MsvcRedistributable -Installers @($msvcInstaller)
    } "Se acepto un fallo del redistribuible MSVC"

    $script:MockAuthenticodeValid = $false
    Assert-Throws {
        Assert-DesktopPackageRuntime | Out-Null
    } "Se acepto un redistribuible MSVC sin firma Authenticode valida"
    $script:MockAuthenticodeValid = $true

    $baseDir = Join-Path $tmp "invalid-redist"
    New-Item -ItemType Directory -Path $baseDir | Out-Null
    foreach ($file in $requiredQtRuntime) {
        Add-TestFile -Root $baseDir -RelativePath $file
    }
    foreach ($plugin in $requiredQmlPlugins) {
        Add-TestFile -Root $baseDir -RelativePath (Join-Path "qml/QtRuntime" $plugin)
    }
    Add-TestFile -Root $baseDir -RelativePath "vc_redist.x86.exe"
    Assert-Throws {
        Assert-DesktopPackageRuntime | Out-Null
    } "Se acepto un redistribuible MSVC de otra arquitectura"
    Add-TestFile -Root $baseDir -RelativePath "vc_redist.x64.exe"
    Assert-Throws {
        Assert-DesktopPackageRuntime | Out-Null
    } "Se aceptaron varios redistribuibles MSVC"

    $baseDir = Join-Path $tmp "nested-redist"
    New-Item -ItemType Directory -Path $baseDir | Out-Null
    foreach ($file in $requiredQtRuntime) {
        Add-TestFile -Root $baseDir -RelativePath $file
    }
    foreach ($plugin in $requiredQmlPlugins) {
        Add-TestFile -Root $baseDir -RelativePath (Join-Path "qml/QtRuntime" $plugin)
    }
    Add-TestFile -Root $baseDir -RelativePath "qml/QtRuntime/vc_redist.x64.exe"
    Assert-Throws {
        Assert-DesktopPackageRuntime | Out-Null
    } "Se acepto un redistribuible MSVC fuera de la raiz del paquete"

    $baseDir = Join-Path $tmp "invalid"
    New-Item -ItemType Directory -Path $baseDir | Out-Null
    foreach ($file in $requiredQtRuntime + $requiredQmlPlugins) {
        Add-TestFile -Root $baseDir -RelativePath $file
    }
    Assert-Throws {
        Assert-DesktopPackageRuntime | Out-Null
    } "Se acepto un paquete sin runtime de compilador"

    $safeDesktop = Join-Path $env:LOCALAPPDATA "Programs/GrxFirma/DesktopQML"
    $dataCache = Join-Path $env:LOCALAPPDATA "GrxFirma/Cache"
    New-Item -ItemType Directory -Force -Path $dataCache | Out-Null
    $controlSocket = Join-Path $dataCache "afirmauri-control.sock"
    Set-Content -LiteralPath $controlSocket -Value "datos de usuario" -Encoding UTF8
    $expectedInstallRoot = Join-Path $env:LOCALAPPDATA "Programs/GrxFirma"
    if (-not [string]::Equals(
        (Get-GrxFirmaInstallRoot),
        [System.IO.Path]::GetFullPath($expectedInstallRoot),
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        throw "La base de instalacion coincide con la carpeta de datos."
    }
    $resolvedDesktop = Initialize-GrxFirmaInstallDirectory `
        -Path $safeDesktop `
        -Component "DesktopQML" `
        -LegacyPayload "grxfirma-gui-qml.exe"
    Add-TestFile -Root $resolvedDesktop -RelativePath "grxfirma-gui-qml.exe"
    if (-not (Test-GrxFirmaInstallMarker -Path $resolvedDesktop -Component "DesktopQML")) {
        throw "No se creo el marcador de propiedad de DesktopQML"
    }
    Set-GrxFirmaSuiteOwnershipMarker -Path $resolvedDesktop -Component "DesktopQML"
    if (-not (Test-GrxFirmaSuiteOwnershipMarker -Path $resolvedDesktop -Component "DesktopQML")) {
        throw "No se creo el marcador de propiedad de la suite"
    }
    Clear-GrxFirmaInstallDirectory `
        -Path $resolvedDesktop `
        -Component "DesktopQML" `
        -LegacyPayload "grxfirma-gui-qml.exe" | Out-Null
    if (Test-GrxFirmaSuiteOwnershipMarker -Path $resolvedDesktop -Component "DesktopQML") {
        throw "Una reinstalacion standalone conservaria incorrectamente la propiedad de la suite"
    }
    Add-TestFile -Root $resolvedDesktop -RelativePath "grxfirma-gui-qml.exe"
    Assert-Throws {
        Clear-GrxFirmaInstallDirectory `
            -Path $env:LOCALAPPDATA `
            -Component "DesktopQML" `
            -LegacyPayload "grxfirma-gui-qml.exe" | Out-Null
    } "Se acepto LOCALAPPDATA como destino destructivo"
    Assert-Throws {
        Remove-GrxFirmaInstallDirectory `
            -Path (Join-Path $env:LOCALAPPDATA "otra-ruta") `
            -Component "DesktopQML" `
            -LegacyPayload "grxfirma-gui-qml.exe"
    } "Se acepto una ruta fuera del componente permitido"
    Remove-GrxFirmaInstallDirectory `
        -Path $safeDesktop `
        -Component "DesktopQML" `
        -LegacyPayload "grxfirma-gui-qml.exe"
    if (-not (Test-Path -LiteralPath $controlSocket -PathType Leaf)) {
        throw "La limpieza de programas elimino el socket de datos del usuario."
    }

    Assert-Contains `
        -Path (Join-Path $windowsDir "build-afirmauri.ps1") `
        -Expected @(
            "uninstall-afirmauri.ps1",
            "afirmauri-registration.ps1",
            "invoke-uninstall-silent.ps1",
            '$env:CGO_ENABLED = "1"',
            '"production,fyne_gui"',
            "Assert-GrxFirmaWindowsFyneToolchain",
            "Assert-GrxFirmaWindowsFyneArtifact"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "build-afirmauri.sh") `
        -Expected @(
            "uninstall-afirmauri.ps1",
            "afirmauri-registration.ps1",
            "invoke-uninstall-silent.ps1",
            "CGO_ENABLED=1",
            "-tags production,fyne_gui",
            "grxfirma_assert_windows_fyne_toolchain",
            "grxfirma_assert_windows_fyne_artifact"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "build-desktop-qml.ps1") `
        -Expected @(
            "uninstall-desktop-qml.ps1",
            "invoke-uninstall-silent.ps1",
            "grxfirma-gui.exe",
            "./cmd/grxfirma-gui",
            '-LinkerFlags @("-H=windowsgui")'
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "build-desktop-qml.sh") `
        -Expected @(
            "uninstall-desktop-qml.ps1",
            "invoke-uninstall-silent.ps1",
            "grxfirma-gui.exe",
            "./cmd/grxfirma-gui",
            'grxfirma_go_build "${VERSION}" "-H=windowsgui"'
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "build-suite.sh") `
        -Expected @(
            "vc_redist.x64.exe",
            "install-path-safety.ps1",
            'rm -rf -- "${STAGE_DIR}"',
            "uninstall-suite.ps1",
            "uninstall-nativehost.ps1",
            "uninstall-afirmauri.ps1",
            "uninstall-desktop-qml.ps1",
            "invoke-uninstall-silent.ps1",
            "grxfirma-gui.exe",
            "CGO_ENABLED=1",
            "-tags production,fyne_gui",
            "grxfirma_assert_windows_fyne_toolchain",
            "grxfirma_assert_windows_fyne_artifact"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "build-suite.ps1") `
        -Expected @(
            "grxfirma-gui.exe",
            "invoke-uninstall-silent.ps1",
            "Assert-SuiteZipArtifact",
            '$env:CGO_ENABLED = "1"',
            '"production,fyne_gui"',
            "Assert-GrxFirmaWindowsFyneToolchain",
            "Assert-GrxFirmaWindowsFyneArtifact",
            '$ProgressPreference = "SilentlyContinue"',
            'if ($args -contains "--nsis")',
            "nsis-path-preflight.ps1",
            "New-GrxFirmaNsisPathContext",
            "Remove-GrxFirmaNsisPathContext",
            "Resolve-GrxFirmaMakeNsis",
            '& $MakeNsis',
            "La construccion NSIS de la suite fallo"
        )
    $suiteBuilderContent = Get-Content `
        -LiteralPath (Join-Path $windowsDir "build-suite.ps1") `
        -Raw
    $progressPreferenceOffset = $suiteBuilderContent.IndexOf(
        '$ProgressPreference = "SilentlyContinue"',
        [System.StringComparison]::Ordinal
    )
    $firstArchiveOffset = $suiteBuilderContent.IndexOf(
        "Expand-Archive",
        [System.StringComparison]::Ordinal
    )
    if ($progressPreferenceOffset -lt 0 -or
        $firstArchiveOffset -lt 0 -or
        $progressPreferenceOffset -gt $firstArchiveOffset) {
        throw "build-suite.ps1 no desactiva el progreso antes de copiar o expandir payloads"
    }
    foreach ($builder in @(
        "build-cli.ps1",
        "build-afirmauri.ps1",
        "build-desktop-qml.ps1"
    )) {
        Assert-Contains `
            -Path (Join-Path $windowsDir $builder) `
            -Expected @(
                "Resolve-GrxFirmaMakeNsis",
                '& $MakeNsis'
            )
    }
    foreach ($builder in @("build-nativehost.ps1", "build-nativehost.sh")) {
        Assert-Contains `
            -Path (Join-Path $windowsDir $builder) `
            -Expected @("uninstall-nativehost.ps1", "install-path-safety.ps1")
    }
    Assert-Contains `
        -Path (Join-Path $windowsDir "install-nativehost.ps1") `
        -Expected @(
            "Get-RegistroCadenaSnapshot",
            "Restore-RegistroCadenaSnapshot",
            "Restore-NativeHostInstallSnapshot",
            "nativeRegistryPlans",
            "nativeRollbackErrors",
            "Rollback incompleto",
            "Rollback de ficheros incompleto"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "grxfirma-afirmauri.nsi") `
        -Expected @(
            "uninstall-afirmauri.ps1",
            "afirmauri-registration.ps1",
            "uninstall-afirmauri.ps1`" -InstallDir",
            "QuietUninstallString",
            "invoke-uninstall-silent.ps1"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "grxfirma-desktop-qml.nsi") `
        -Expected @(
            "uninstall-desktop-qml.ps1`" -InstallDir",
            "QuietUninstallString",
            "invoke-uninstall-silent.ps1",
            "grxfirma-gui.exe"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "grxfirma-suite.nsi") `
        -Expected @(
            "afirmauri-registration.ps1",
            "install-path-safety.ps1",
            "invoke-uninstall-silent.ps1",
            "QuietUninstallString",
            "grxfirma-gui.exe",
            "desktop-winui",
            "desktop-qt",
            "MUI_PAGE_COMPONENTS"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "invoke-uninstall-silent.ps1") `
        -Expected @(
            "OrdinalIgnoreCase",
            "ReparsePoint",
            '("GrxFirma-Uninstall-" + [guid]::NewGuid().ToString("N"))',
            "Assert-ExclusiveDirectoryAcl",
            "SetAccessRuleProtection",
            ".GetOwner(",
            "FileSystemAclExtensions",
            "[System.IO.Directory]::CreateDirectory",
            "[System.IO.FileMode]::CreateNew",
            "[System.IO.FileShare]::None",
            "Copy-RegularFileExclusive",
            "Assert-NoReparseTree",
            "ValidateOnly",
            "executionGuard",
            "Start-Process",
            '"_?=$normalizedInstallDir"',
            "WaitForExit(300000)",
            "Start-Sleep -Milliseconds 100",
            '[System.IO.Directory]::Delete($TemporaryDirectory, $false)',
            "exit `$exitCode"
        )
    $silentLauncherContent = Get-Content `
        -LiteralPath (Join-Path $windowsDir "invoke-uninstall-silent.ps1") `
        -Raw
    foreach ($unsafeFragment in @("Copy-Item", "-Recurse")) {
        if ($silentLauncherContent.Contains($unsafeFragment)) {
            throw "El lanzador silencioso conserva la operacion insegura '$unsafeFragment'"
        }
    }
    foreach ($scriptedNsi in @(
        "grxfirma-afirmauri.nsi",
        "grxfirma-desktop-qml.nsi",
        "grxfirma-suite.nsi"
    )) {
        Assert-Contains `
            -Path (Join-Path $windowsDir $scriptedNsi) `
            -Expected @(
                "invoke-uninstall-silent.ps1",
                "-ValidateOnly",
                "puntos de reanálisis",
                "SetErrorLevel 0"
            )
    }
    Assert-Throws {
        & (Join-Path $windowsDir "invoke-uninstall-silent.ps1") `
            -InstallDir (Join-Path $tmp "launcher-install") `
            -UninstallerPath (Join-Path $tmp "outside-uninstall.exe")
    } "El lanzador silencioso acepto un desinstalador fuera de InstallDir"
    if ([System.Environment]::OSVersion.Platform -eq
        [System.PlatformID]::Win32NT) {
        $launcherInstallDir = Join-Path $tmp "launcher install"
        New-Item -ItemType Directory -Path $launcherInstallDir | Out-Null
        $fixtureUninstaller = Join-Path $launcherInstallDir "uninstall.exe"
        Copy-Item `
            -LiteralPath (Join-Path $env:SystemRoot "System32\where.exe") `
            -Destination $fixtureUninstaller
        $temporaryRoot = [System.IO.Path]::GetTempPath()
        $temporaryDirectoriesBefore = @(
            Get-ChildItem `
                -LiteralPath $temporaryRoot `
                -Directory `
                -Force `
                -Filter "GrxFirma-Uninstall-*" |
                ForEach-Object { $_.FullName } |
                Sort-Object
        )
        $currentPowerShell = (Get-Process -Id $PID).Path
        & $currentPowerShell `
            -NoProfile `
            -NonInteractive `
            -ExecutionPolicy Bypass `
            -File (Join-Path $windowsDir "invoke-uninstall-silent.ps1") `
            -InstallDir $launcherInstallDir `
            -UninstallerPath $fixtureUninstaller *> $null
        $launcherExitCode = $LASTEXITCODE
        if ($launcherExitCode -eq 1603) {
            throw "El lanzador silencioso fallo antes de propagar el resultado del proceso hijo"
        }
        $temporaryDirectoriesAfter = @(
            Get-ChildItem `
                -LiteralPath $temporaryRoot `
                -Directory `
                -Force `
                -Filter "GrxFirma-Uninstall-*" |
                ForEach-Object { $_.FullName } |
                Sort-Object
        )
        if (@(Compare-Object `
            -ReferenceObject $temporaryDirectoriesBefore `
            -DifferenceObject $temporaryDirectoriesAfter).Count -ne 0) {
            throw "El lanzador silencioso dejo un directorio temporal residual"
        }
    }
    Assert-Contains `
        -Path (Join-Path $windowsDir "install-desktop-qml.ps1") `
        -Expected @(
            '$ipcBackendSource = Join-Path $baseDir "grxfirma-gui.exe"',
            'Copy-Item $ipcBackendSource',
            'Stop-GrxFirmaInstalledProcesses -Path $InstallDir -Component "DesktopQML"'
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "install-suite.ps1") `
        -Expected @(
            "grxfirma-gui.exe",
            "DesktopLauncher",
            '$hasDesktopQt',
            '$hasDesktopWinUi',
            '$hasDesktopQt = (-not $CoreOnly) -and',
            '$hasDesktopWinUi = (-not $CoreOnly) -and',
            '$CoreOnly'
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "uninstall-desktop-qml.ps1") `
        -Expected @(
            "Stop-InstalledDesktopProcesses",
            '"grxfirma-gui-qml", "grxfirma-gui", "grxfirma"'
        )
    $msixBuilder = Join-Path $windowsDir "build-msix.ps1"
    foreach ($definition in Get-ScriptFunctionDefinition `
        -ScriptPath $msixBuilder `
        -FunctionNames @("Assert-MsixStage")) {
        . ([scriptblock]::Create($definition))
    }
    $msixStage = Join-Path $tmp "msix-stage"
    New-Item -ItemType Directory -Path $msixStage | Out-Null
    Add-TestFile -Root $msixStage -RelativePath "grxfirma-gui-qml.exe"
    Assert-Throws {
        Assert-MsixStage -StageDir $msixStage
    } "Se acepto una stage MSIX sin grxfirma-gui.exe"
    Add-TestFile -Root $msixStage -RelativePath "grxfirma-gui.exe"
    Assert-MsixStage -StageDir $msixStage
    Remove-Item -LiteralPath (Join-Path $msixStage "grxfirma-gui-qml.exe") -Force
    Assert-Throws {
        Assert-MsixStage -StageDir $msixStage
    } "Se acepto una stage MSIX sin frontend Qt"
    Assert-Contains `
        -Path $msixBuilder `
        -Expected @(
            "Assert-MsixStage",
            "grxfirma-gui.exe",
            "backend IPC"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "install-suite.ps1") `
        -Expected @("-ManagedBySuite")
    Assert-Contains `
        -Path (Join-Path $windowsDir "uninstall-suite.ps1") `
        -Expected @("Test-GrxFirmaSuiteOwnershipMarker")
    Assert-Contains `
        -Path (Join-Path $windowsDir "uninstall-nativehost.ps1") `
        -Expected @(
            "Remove-RegistryValueIfOwned",
            "registrations.json",
            "Remove-FirefoxXpiIfOwned",
            "Se conserva la extension Firefox porque fue actualizada o reemplazada"
        )
    Assert-Contains `
        -Path (Join-Path $windowsDir "uninstall-afirmauri.ps1") `
        -Expected @(
            "Restore-AfirmaProtocolRegistration",
            "afirma-protocol-snapshot.json",
            '$iconTarget = Join-Path $InstallDir "grxfirma-grx.ico"',
            "-IconPath `$iconTarget",
            "--remove-local-tls-trust",
            "Start-Process",
            "-WindowStyle Hidden",
            "WaitForExit(120000)",
            "--no-prompt",
            "`$cleanupProcess.Kill()",
            "-PassThru",
            "`$cleanupProcess.ExitCode",
            "Se conserva la instalacion para reintentar",
            "Se conserva el protocolo afirma:// porque ya no pertenece a esta instalacion"
        )
    $afirmaUriUninstallerContent = Get-Content `
        -LiteralPath (Join-Path $windowsDir "uninstall-afirmauri.ps1") `
        -Raw
    if ($afirmaUriUninstallerContent.Contains('$LASTEXITCODE')) {
        throw "El desinstalador AfirmaURI no debe leer LASTEXITCODE de un ejecutable GUI"
    }

    $legacyGuard = Join-Path $windowsDir "legacy-machine-install.nsh"
    $platformPreflight = Join-Path $windowsDir "windows-platform-preflight.nsh"
    Assert-Contains `
        -Path $platformPreflight `
        -Expected @(
            '!include "x64.nsh"',
            "!define GRXFIRMA_MIN_WINDOWS_BUILD 17763",
            '${IfNot} ${IsNativeAMD64}',
            '"CurrentBuildNumber"',
            '"CurrentBuild"',
            "SetRegView 64",
            "SetRegView 32",
            "SetErrorLevel 1633",
            "IfSilent +2",
            "Quit"
        )
    $platformPreflightContent = Get-Content -LiteralPath $platformPreflight -Raw
    if ($platformPreflightContent -notmatch
        '(?ms)!macro GrxFirmaRequireSupportedWindows\b.*?\$\{IfNot\} \$\{IsNativeAMD64\}.*?SetRegView 64.*?"CurrentBuildNumber".*?"CurrentBuild".*?SetRegView 32.*?\$\{If\} \$3 < \$\{GRXFIRMA_MIN_WINDOWS_BUILD\}') {
        throw "El preflight NSIS no exige AMD64 y build 17763 antes de continuar"
    }
    if ($platformPreflightContent -notmatch
        '(?ms)!macro GrxFirmaAbortUnsupportedWindows\b.*?IfSilent \+2\s+MessageBox .*?SetErrorLevel 1633\s+Quit') {
        throw "El rechazo de plataforma NSIS puede bloquear /S o no devuelve 1633"
    }
    Assert-Contains `
        -Path $legacyGuard `
        -Expected @(
            '!include "windows-platform-preflight.nsh"',
            "Push `$3",
            "!insertmacro GrxFirmaRequireSupportedWindows",
            "!macro GrxFirmaExitOnExecFailure",
            "!macro GrxFirmaWriteInstallFailure",
            "GrxFirma-install-error.txt",
            'Fase: ${ERROR_MESSAGE}',
            "IfSilent +2",
            "MessageBox MB_ICONSTOP",
            "GrxFirmaReadLegacyMachineInstall 64",
            "GrxFirmaReadLegacyMachineInstall 32",
            '"GrxFirma"',
            "ReadRegStr `$1 HKLM",
            "SetErrorLevel 1603",
            "Quit"
        )
    $legacyGuardContent = Get-Content -LiteralPath $legacyGuard -Raw
    if ($legacyGuardContent -notmatch
        '(?ms)!macro GrxFirmaExitOnExecFailure\b.*?IfSilent \+2\s+MessageBox MB_ICONSTOP.*?SetErrorLevel 1603\s+Quit.*?!macroend') {
        throw "El guard de errores NSIS no evita dialogos en /S antes de devolver 1603"
    }
    if ($legacyGuardContent -notmatch
        '(?ms)grxfirma_legacy_machine_install_found:\s+IfSilent \+2\s+MessageBox .*?SetErrorLevel 1603\s+Quit') {
        throw "El bloqueo de instalaciones legacy puede mostrar un dialogo invisible en /S"
    }
    if ($legacyGuardContent.IndexOf("GrxFirmaRequireSupportedWindows") -gt
        $legacyGuardContent.IndexOf("GrxFirmaReadLegacyMachineInstall 64")) {
        throw "El preflight de plataforma NSIS se ejecuta despues de consultar instalaciones legacy"
    }
    foreach ($scriptedNsi in @(
        "grxfirma-afirmauri.nsi",
        "grxfirma-desktop-qml.nsi",
        "grxfirma-suite.nsi"
    )) {
        Assert-NsisExecFailureContract -Path (Join-Path $windowsDir $scriptedNsi)
    }
    $ownLegacyKeys = @{
        "grxfirma-afirmauri.nsi" = "GrxFirmaAfirmaURI"
        "grxfirma-cli.nsi" = "GrxFirmaCLI"
        "grxfirma-desktop-qml.nsi" = "GrxFirmaDesktopQt"
        "grxfirma-suite.nsi" = "GrxFirma"
    }
    foreach ($nsi in Get-ChildItem -LiteralPath $windowsDir -File -Filter "grxfirma-*.nsi") {
        $content = Get-Content -LiteralPath $nsi.FullName -Raw
        foreach ($forbidden in @(
            "RequestExecutionLevel admin",
            "InstallDirRegKey HKLM",
            "WriteRegStr HKLM",
            "DeleteRegKey HKLM",
            "MUI_PAGE_DIRECTORY",
            'RMDir /r "$INSTDIR"'
        )) {
            if ($content.Contains($forbidden)) {
                throw "$($nsi.Name) conserva el patron inseguro '$forbidden'"
            }
        }
        if (-not $content.Contains("RequestExecutionLevel user")) {
            throw "$($nsi.Name) no declara instalacion por usuario"
        }
        foreach ($guardFragment in @(
            '!include "authenticode-signing.nsh"',
            '!include "legacy-machine-install.nsh"',
            "!insertmacro GrxFirmaBlockLegacyMachineInstall"
        )) {
            if (-not $content.Contains($guardFragment)) {
                throw "$($nsi.Name) no bloquea la convivencia con una instalacion HKLM legacy"
            }
        }
        if (-not $content.Contains('"' + $ownLegacyKeys[$nsi.Name] + '"')) {
            throw "$($nsi.Name) no comprueba su propia clave HKLM legacy"
        }
        if ($content.IndexOf("GrxFirmaBlockLegacyMachineInstall") -gt
            $content.IndexOf('Section "')) {
            throw "$($nsi.Name) ejecuta el guard legacy despues de empezar a copiar"
        }
    }

    foreach ($suiteScript in @("install-suite.ps1", "uninstall-suite.ps1")) {
        $content = Get-Content -LiteralPath (Join-Path $windowsDir $suiteScript) -Raw
        if ($content -match '(?im)^\s*&\s+(?:powershell|pwsh)(?:\.exe)?\b') {
            throw "$suiteScript crea un PowerShell hijo y puede ocultar su codigo de salida"
        }
    }

    $repositoryRoot = Split-Path -Parent (Split-Path -Parent $windowsDir)
    Assert-Contains `
        -Path (Join-Path $repositoryRoot "scripts/release/finalize-windows-release.ps1") `
        -Expected @(
            "Where-Object { `$_.Extension -in @('.exe', '.dll') }",
            "La etapa Qt no contiene bibliotecas DLL",
            "Invoke-WindowsAuthenticodeSign",
            "GRXFIRMA_UNINSTALL_SIGNER",
            "uninstallerEvidence"
        )
    Assert-Contains `
        -Path (Join-Path $repositoryRoot "scripts/release/verify-windows-release.ps1") `
        -Expected @(
            "Where-Object { `$_.Extension -in @('.exe', '.dll') }",
            "El ZIP Qt no contiene bibliotecas DLL",
            "Assert-WindowsAuthenticodeFile",
            "timestamp_protocol = `$algorithms.TimestampProtocol",
            "RequireSha256Rfc3161",
            "RequireDualGui",
            "La Suite Windows oficial debe contener WinUI y Qt completos."
        )
} finally {
    $env:LOCALAPPDATA = $originalLocalAppData
    $env:PSModulePath = $originalEnginePSModulePath
    if (Test-Path -LiteralPath $tmp) {
        Remove-Item -LiteralPath $tmp -Recurse -Force
    }
}

& (Join-Path $PSScriptRoot "test-nsis-path-preflight.ps1")
Write-Output "Windows installer contract tests passed."
