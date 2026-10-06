# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Raiz = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
# Misma regla que scripts/comprobar-novedades.py, sin depender de Python.
$versionNovedades = (Get-Content -LiteralPath (Join-Path $Raiz "VERSION.txt") -Raw).Trim()
$ficheroNovedades = Join-Path $Raiz "docs/NOVEDADES.md"
if ((Get-Item -LiteralPath $ficheroNovedades).Length -gt 65536) {
    throw "Compilación detenida: docs/NOVEDADES.md supera el límite de 64 KiB de la ayuda."
}
$patronNovedades = "(?m)^## " + [regex]::Escape($versionNovedades) + " — \d{4}-\d{2}-\d{2}\s*$"
if ([regex]::Matches((Get-Content -LiteralPath $ficheroNovedades -Raw -Encoding UTF8), $patronNovedades).Count -ne 1) {
    throw "Compilación detenida: complete la sección '## $versionNovedades — AAAA-MM-DD' en docs/NOVEDADES.md."
}
. (Join-Path $PSScriptRoot "reproducible-build.ps1")
$nsisPathPreflight = Join-Path $PSScriptRoot "nsis-path-preflight.ps1"
if (-not (Test-Path -LiteralPath $nsisPathPreflight -PathType Leaf)) {
    throw "No se encuentra nsis-path-preflight.ps1 junto al empaquetador."
}
. $nsisPathPreflight
$SourceDateEpoch = Initialize-GrxFirmaReproducibleBuild -RootDirectory $Raiz
$versionFile = Join-Path $Raiz "VERSION.txt"
if (Test-Path $versionFile) {
    $Version = (Get-Content -Path $versionFile -Raw).Trim()
} else {
    $Version = (git -C $Raiz describe --tags --always --dirty 2>$null)
    if ([string]::IsNullOrWhiteSpace($Version)) {
        $Version = "dev"
    }
}

$Arquitectura = if ($env:GOARCH) { $env:GOARCH } else { "amd64" }
Assert-GrxFirmaWindowsFyneToolchain -Architecture $Arquitectura
$Salida = Join-Path $Raiz "release/windows-suite"
$Escenario = Join-Path $Salida "GrxFirma-$Version-windows-$Arquitectura"
$Zip = Join-Path $Salida "GrxFirma-$Version-windows-$Arquitectura.zip"
$Setup = Join-Path $Salida "GrxFirma-$Version-windows-$Arquitectura-setup.exe"
$AfirmaUriExe = Join-Path $Escenario "grxfirma-afirmauri.exe"
$DesktopQmlEscenario = Join-Path $Raiz "release/windows-desktop-qml/GrxFirma-$Version-desktop-qml-windows-$Arquitectura"
$DesktopWinUiEscenario = Join-Path $Raiz "release/windows-desktop-winui/GrxFirma-$Version-desktop-winui-windows-amd64"
$HelpSource = Join-Path $Raiz "cmd/gui-qml/help"
$DefaultExtensionKey = Join-Path $env:USERPROFILE ".local\share\grxfirma\build-keys\chromium-extension.pem"
$HelpAvailable = $false
if (Test-Path -LiteralPath $HelpSource -PathType Container) {
    $HelpAvailable = $null -ne (
        Get-ChildItem -LiteralPath $HelpSource -File -Recurse -ErrorAction SilentlyContinue |
            Select-Object -First 1
    )
}
$RequireQtStage = $args -contains "--with-qt"
$RequireWinUiStage = $args -contains "--with-winui"
foreach ($argument in $args) {
    if ($argument -notin @("--nsis", "--with-qt", "--with-winui")) {
        throw "Argumento no soportado: $argument"
    }
}
$MakeNsis = if ($args -contains "--nsis") {
    Resolve-GrxFirmaMakeNsis
} else {
    $null
}

function Get-PackBrowser {
    $candidates = @(
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
        "$env:ProgramFiles\BraveSoftware\Brave-Browser\Application\brave.exe",
        "${env:ProgramFiles(x86)}\BraveSoftware\Brave-Browser\Application\brave.exe"
    )
    foreach ($candidate in $candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path $candidate)) {
            return $candidate
        }
    }
    return $null
}

function Get-OrCreateChromiumExtensionKey {
    if (-not [string]::IsNullOrWhiteSpace($env:GRXFIRMA_CHROMIUM_EXTENSION_KEY) -and (Test-Path $env:GRXFIRMA_CHROMIUM_EXTENSION_KEY)) {
        return $env:GRXFIRMA_CHROMIUM_EXTENSION_KEY
    }
    $keyDir = Split-Path -Parent $DefaultExtensionKey
    New-Item -ItemType Directory -Force -Path $keyDir | Out-Null
    if (-not (Test-Path $DefaultExtensionKey)) {
        & openssl genrsa -out $DefaultExtensionKey 2048 | Out-Null
    }
    if (Test-Path $DefaultExtensionKey) {
        return $DefaultExtensionKey
    }
    return $null
}

function Build-ChromiumExtensionAsset {
    param(
        [string]$ZipSource,
        [string]$OutputDir
    )

    if (-not (Test-Path $ZipSource)) {
        return
    }
    $buildCrx = if ([string]::IsNullOrWhiteSpace($env:GRXFIRMA_BUILD_CHROMIUM_CRX)) {
        "0"
    } else {
        $env:GRXFIRMA_BUILD_CHROMIUM_CRX
    }
    if ($buildCrx -eq "0") {
        return
    }
    if ($buildCrx -ne "1") {
        throw "GRXFIRMA_BUILD_CHROMIUM_CRX debe valer 0 o 1."
    }

    $packBrowser = Get-PackBrowser
    if ([string]::IsNullOrWhiteSpace($packBrowser)) {
        Write-Warning "No hay navegador Chromium disponible para empaquetar la extension."
        return
    }

    $tmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-ext-" + [System.Guid]::NewGuid().ToString("N"))
    $extractDir = Join-Path $tmpRoot "ext"
    New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
    Expand-Archive -Path $ZipSource -DestinationPath $extractDir -Force

    $extensionDir = $extractDir
    if (Test-Path (Join-Path $extractDir "chromium")) {
        $extensionDir = Join-Path $extractDir "chromium"
    }

    $keyPath = Get-OrCreateChromiumExtensionKey
    $packArgs = @("--pack-extension=$extensionDir")
    if (-not [string]::IsNullOrWhiteSpace($keyPath)) {
        $packArgs += "--pack-extension-key=$keyPath"
    }
    & $packBrowser $packArgs | Out-Null

    $crxPath = @(
        (Join-Path $extractDir ((Split-Path $extensionDir -Leaf) + ".crx")),
        ($extractDir + ".crx"),
        ($extensionDir + ".crx")
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1
    $pemPath = @(
        (Join-Path $extractDir ((Split-Path $extensionDir -Leaf) + ".pem")),
        ($extractDir + ".pem"),
        ($extensionDir + ".pem")
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1
    if (-not [string]::IsNullOrWhiteSpace($keyPath) -and (Test-Path $keyPath)) {
        $pemPath = $keyPath
    }
    if ([string]::IsNullOrWhiteSpace($crxPath) -or (-not (Test-Path $crxPath)) -or
        [string]::IsNullOrWhiteSpace($pemPath) -or (-not (Test-Path $pemPath))) {
        Write-Warning "No se pudo generar el CRX Chromium empaquetado."
        return
    }

    $extensionIdScript = @'
import hashlib
import subprocess
import sys
pub_der = subprocess.check_output(["openssl", "pkey", "-in", sys.argv[1], "-pubout", "-outform", "DER"])
hex_digest = hashlib.sha256(pub_der).hexdigest()[:32]
print(hex_digest.translate(str.maketrans("0123456789abcdef", "abcdefghijklmnop")))
'@
    $extensionId = (& python -c $extensionIdScript $pemPath).Trim()
    $versionScript = @'
import json
import sys
import zipfile
with zipfile.ZipFile(sys.argv[1]) as zf:
    for name in zf.namelist():
        if name.endswith("manifest.json"):
            print(json.loads(zf.read(name)).get("version", "1.0.0"))
            break
    else:
        print("1.0.0")
'@
    $version = (& python -c $versionScript $ZipSource).Trim()

    Copy-Item $crxPath (Join-Path $OutputDir "grxfirma-extension-chromium.crx") -Force
    Set-Content -Path (Join-Path $OutputDir "grxfirma-extension-chromium.id") -Value $extensionId -Encoding ASCII
    Set-Content -Path (Join-Path $OutputDir "grxfirma-extension-chromium.version") -Value $version -Encoding ASCII
    Remove-Item $tmpRoot -Recurse -Force
}

function Write-ArtifactManifest {
    param(
        [string]$OutputDir,
        [string[]]$Candidates
    )

    $existing = @()
    foreach ($candidate in $Candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path $candidate)) {
            $existing += $candidate
        }
    }
    if ($existing.Count -eq 0) {
        throw "No hay artefactos para inventariar en $OutputDir"
    }

    $shaPath = Join-Path $OutputDir "SHA256SUMS.txt"
    $artifactReport = Join-Path $OutputDir "ARTIFACTS.md"

    $shaLines = @()
    $reportLines = @(
        "# Windows Suite Artifacts",
        "",
        "| File | Size (bytes) | SHA-256 |",
        "| --- | ---: | --- |"
    )

    foreach ($path in $existing) {
        $file = Get-Item $path
        $hash = (Get-FileHash -Algorithm SHA256 -Path $file.FullName).Hash.ToLowerInvariant()
        $shaLines += ("{0} *{1}" -f $hash, $file.Name)
        $reportLines += ("| `{0}` | {1} | `{2}` |" -f $file.Name, $file.Length, $hash)
    }

    Set-Content -Path $shaPath -Value $shaLines -Encoding ASCII
    Set-Content -Path $artifactReport -Value $reportLines -Encoding UTF8

    Push-Location $OutputDir
    try {
        foreach ($path in $existing) {
            $file = Split-Path -Leaf $path
            $entry = Select-String -Path $shaPath -Pattern (" \*" + [regex]::Escape($file) + "$")
            if (-not $entry) {
                throw "No se ha podido verificar el checksum de $file"
            }
            $expected = ($entry.Line -split '\s+')[0].ToLowerInvariant()
            $actual = (Get-FileHash -Algorithm SHA256 -Path $file).Hash.ToLowerInvariant()
            if ($expected -ne $actual) {
                throw "Checksum inesperado para $file"
            }
        }
    } finally {
        Pop-Location
    }
}

function Get-QtRuntimeRequirement {
    return @(
        "Qt6Core.dll",
        "Qt6Gui.dll",
        "Qt6Qml.dll",
        "Qt6Quick.dll",
        "platforms/qwindows.dll"
    )
}

function Get-QtQmlPluginRequirement {
    return @(
        "qmlplugin.dll",
        "qtquick2plugin.dll",
        "qtquickcontrols2plugin.dll",
        "qquicklayoutsplugin.dll",
        "qtquickdialogsplugin.dll",
        "qmlsettingsplugin.dll"
    )
}

function Test-CompilerRuntime {
    param(
        [string[]]$FileNames
    )

    $redistributables = @($FileNames | Where-Object { $_ -like "vc_redist.*.exe" })
    $hasMsvcRuntime =
        ($redistributables.Count -eq 1) -and
        ($redistributables[0] -ieq "vc_redist.x64.exe")
    $hasMinGwRuntime =
        ($FileNames -contains "libstdc++-6.dll") -and
        ($FileNames -contains "libwinpthread-1.dll") -and
        (@($FileNames | Where-Object { $_ -like "libgcc_s_*-1.dll" }).Count -gt 0)
    return (($redistributables.Count -eq 0) -and $hasMinGwRuntime) -or $hasMsvcRuntime
}

function Assert-QtRuntimeStage {
    param(
        [string]$StageDir
    )

    $missing = @()
    foreach ($relativePath in Get-QtRuntimeRequirement) {
        $path = Join-Path $StageDir $relativePath
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            $missing += $relativePath
        }
    }

    $deployedFileNames = @(
        Get-ChildItem -LiteralPath $StageDir -File -Recurse -ErrorAction SilentlyContinue |
            ForEach-Object { $_.Name }
    )
    $runtimeFileNames = @(
        Get-ChildItem -LiteralPath $StageDir -File -ErrorAction SilentlyContinue |
            ForEach-Object { $_.Name }
    )
    foreach ($pluginName in Get-QtQmlPluginRequirement) {
        if ($deployedFileNames -notcontains $pluginName) {
            $missing += "*/$pluginName"
        }
    }
    if (-not (Test-CompilerRuntime -FileNames $runtimeFileNames)) {
        $missing += "runtime de compilador MSVC o MinGW"
    }

    if ($missing.Count -gt 0) {
        throw ("La stage Qt/QML de Windows esta incompleta. Faltan:`n  - " + ($missing -join "`n  - "))
    }
}

function Assert-QtSuiteStage {
    param(
        [string]$StageDir
    )

    $missing = @()
    foreach ($required in @(
        "grxfirma-gui-qml.exe",
        "grxfirma-gui.exe",
        "grxfirma.exe",
        "install-desktop-qml.ps1",
        "README_DESKTOP_QML_WINDOWS.md"
    )) {
        if (-not (Test-Path -LiteralPath (Join-Path $StageDir $required) -PathType Leaf)) {
            $missing += (Join-Path $StageDir $required)
        }
    }
    foreach ($requiredDir in @("qml", "assets")) {
        $requiredPath = Join-Path $StageDir $requiredDir
        $containsFiles = $null
        if (Test-Path -LiteralPath $requiredPath -PathType Container) {
            $containsFiles = Get-ChildItem -LiteralPath $requiredPath -File -Recurse -ErrorAction SilentlyContinue |
                Select-Object -First 1
        }
        if ($null -eq $containsFiles) {
            $missing += $requiredPath
        }
    }
    if ($HelpAvailable) {
        $helpPath = Join-Path $StageDir "help"
        $helpFiles = $null
        if (Test-Path -LiteralPath $helpPath -PathType Container) {
            $helpFiles = Get-ChildItem -LiteralPath $helpPath -File -Recurse -ErrorAction SilentlyContinue |
                Select-Object -First 1
        }
        if ($null -eq $helpFiles) {
            $missing += $helpPath
        }
    }
    if ($missing.Count -gt 0) {
        throw ("La stage Qt/QML de Windows esta incompleta. Faltan:`n  - " + ($missing -join "`n  - "))
    }
    Assert-QtRuntimeStage -StageDir $StageDir
}

function Assert-WinUiRuntimeStage {
    param(
        [string]$StageDir
    )

    $required = @(
        "README_DESKTOP_WINUI_WINDOWS.md",
        "VERSION.txt",
        "app/help/NOVEDADES.md",
        "PUBLISH-MANIFEST.sha256",
        "app/grxfirma-winui.exe",
        "app/grxfirma-gui.exe",
        "app/coreclr.dll",
        "app/hostfxr.dll",
        "app/Microsoft.UI.Xaml.dll",
        "app/Microsoft.WindowsAppRuntime.dll"
    )
    $missing = @()
    foreach ($relativePath in $required) {
        if (-not (Test-Path -LiteralPath (Join-Path $StageDir $relativePath) -PathType Leaf)) {
            $missing += $relativePath
        }
    }
    if ($missing.Count -gt 0) {
        throw ("La stage WinUI de Windows esta incompleta. Faltan:`n  - " + ($missing -join "`n  - "))
    }

    $manifestPath = Join-Path $StageDir "PUBLISH-MANIFEST.sha256"
    $manifestEntries = @{}
    foreach ($line in Get-Content -LiteralPath $manifestPath -ErrorAction Stop) {
        if ($line -notmatch '^([0-9a-fA-F]{64}) \*(.+)$') {
            throw "El manifiesto WinUI contiene una linea no valida."
        }
        $relativePath = $Matches[2].Replace('\', '/')
        if ([System.IO.Path]::IsPathRooted($relativePath) -or
            @($relativePath -split '[/\\]') -contains "..") {
            throw "El manifiesto WinUI contiene una ruta no permitida: $relativePath"
        }
        $manifestEntries[$relativePath] = $Matches[1].ToLowerInvariant()
    }
    $actualFiles = @(
        Get-ChildItem -LiteralPath $StageDir -File -Force -Recurse |
            Where-Object { $_.FullName -ne $manifestPath }
    )
    foreach ($file in $actualFiles) {
        $relativePath = $file.FullName.Substring($StageDir.Length).TrimStart('\', '/').Replace('\', '/')
        if (-not $manifestEntries.ContainsKey($relativePath)) {
            throw "Fichero WinUI no inventariado: $relativePath"
        }
        $actualHash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualHash -ne $manifestEntries[$relativePath]) {
            throw "Checksum inesperado en la stage WinUI: $relativePath"
        }
    }
    if ($actualFiles.Count -ne $manifestEntries.Count) {
        throw "El manifiesto WinUI referencia ficheros ausentes."
    }
}

function Sync-SharedDesktopBackends {
    param(
        [string]$StageDir,
        [bool]$QtIntegrated,
        [bool]$WinUiIntegrated
    )

    $canonicalBackend = Join-Path $StageDir "grxfirma-gui.exe"
    if ($QtIntegrated) {
        Copy-Item `
            -LiteralPath $canonicalBackend `
            -Destination (Join-Path $StageDir "desktop-qt/grxfirma-gui.exe") `
            -Force
    }
    if (-not $WinUiIntegrated) {
        return
    }

    $winUiStage = Join-Path $StageDir "desktop-winui"
    $winUiBackend = Join-Path $winUiStage "app/grxfirma-gui.exe"
    Copy-Item -LiteralPath $canonicalBackend -Destination $winUiBackend -Force
    $manifestPath = Join-Path $winUiStage "PUBLISH-MANIFEST.sha256"
    $declaredPaths = @{}
    foreach ($line in Get-Content -LiteralPath $manifestPath -ErrorAction Stop) {
        if ($line -notmatch '^([0-9a-fA-F]{64}) \*(.+)$') {
            throw "El manifiesto WinUI contiene una linea no valida."
        }
        $relativePath = $Matches[2].Replace('\', '/')
        if ([System.IO.Path]::IsPathRooted($relativePath) -or
            @($relativePath -split '[/\\]') -contains "..") {
            throw "El manifiesto WinUI contiene una ruta no permitida: $relativePath"
        }
        $declaredPaths[$relativePath] = $true
    }
    if (-not $declaredPaths.ContainsKey("app/grxfirma-gui.exe")) {
        throw "El manifiesto WinUI no inventaria el backend compartido."
    }
    $actualFiles = @(
        Get-ChildItem -LiteralPath $winUiStage -File -Force -Recurse |
            Where-Object { $_.FullName -ne $manifestPath }
    )
    $actualPaths = @{}
    foreach ($file in $actualFiles) {
        $relativePath = $file.FullName.Substring($winUiStage.Length).TrimStart('\', '/').Replace('\', '/')
        $actualPaths[$relativePath] = $file
    }
    foreach ($relativePath in $declaredPaths.Keys) {
        if (-not $actualPaths.ContainsKey($relativePath)) {
            throw "El payload WinUI perdio un fichero durante la firma: $relativePath"
        }
    }
    foreach ($relativePath in $actualPaths.Keys) {
        if (-not $declaredPaths.ContainsKey($relativePath)) {
            throw "El payload WinUI incorporo un fichero no inventariado: $relativePath"
        }
    }
    $updatedLines = foreach ($relativePath in @($actualPaths.Keys | Sort-Object)) {
        $hash = (Get-FileHash `
            -LiteralPath $actualPaths[$relativePath].FullName `
            -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash *$relativePath"
    }
    [System.IO.File]::WriteAllLines(
        $manifestPath,
        $updatedLines,
        [System.Text.UTF8Encoding]::new($false)
    )
    Assert-WinUiRuntimeStage -StageDir $winUiStage
}

function Get-ZipEntryName {
    param(
        [string]$ZipPath
    )

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $resolvedPath = (Resolve-Path -LiteralPath $ZipPath).Path
    $archive = [System.IO.Compression.ZipFile]::OpenRead($resolvedPath)
    try {
        return @(
            $archive.Entries |
                ForEach-Object { $_.FullName.Replace('\', '/') }
        )
    } finally {
        $archive.Dispose()
    }
}

function Assert-SuiteZipArtifact {
    param(
        [string]$ZipPath,
        [string]$StageName,
        [bool]$QtIntegrated,
        [bool]$QtHelpIntegrated,
        [bool]$WinUiIntegrated
    )

    $requiredFiles = @(
        "grxfirma.exe",
        "grxfirma-gui.exe",
        "grxfirma-nativehost.exe",
        "grxfirma-afirmauri.exe",
        "README_WINDOWS_SUITE.md",
        "policies/GrxFirma.admx",
        "policies/es-ES/GrxFirma.adml",
        "policies/en-US/GrxFirma.adml",
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
        "help/NOVEDADES.md",
        "extensions/grxfirma-extension-chromium.zip",
        "extensions/grxfirma-extension-firefox.xpi",
        "extensions/grxfirma-extension-firefox.metadata.json"
    )
    $requiredDirectories = @()
    $requiredPlugins = @()
    if ($QtIntegrated) {
        $requiredFiles += @(
            "desktop-qt/grxfirma-gui-qml.exe",
            "desktop-qt/grxfirma-gui.exe",
            "desktop-qt/install-desktop-qml.ps1",
            "desktop-qt/README_DESKTOP_QML_WINDOWS.md"
        )
        $requiredFiles += @(Get-QtRuntimeRequirement | ForEach-Object { "desktop-qt/$_" })
        $requiredDirectories += @("desktop-qt/qml", "desktop-qt/assets")
        $requiredPlugins += @(Get-QtQmlPluginRequirement)
    }
    if ($QtHelpIntegrated) {
        $requiredDirectories += "desktop-qt/help"
    }
    if ($WinUiIntegrated) {
        $requiredFiles += @(
            "desktop-winui/PUBLISH-MANIFEST.sha256",
            "desktop-winui/README_DESKTOP_WINUI_WINDOWS.md",
            "desktop-winui/VERSION.txt",
            "desktop-winui/app/grxfirma-winui.exe",
            "desktop-winui/app/help/NOVEDADES.md",
            "desktop-winui/app/grxfirma-gui.exe",
            "desktop-winui/app/coreclr.dll",
            "desktop-winui/app/hostfxr.dll",
            "desktop-winui/app/Microsoft.UI.Xaml.dll",
            "desktop-winui/app/Microsoft.WindowsAppRuntime.dll"
        )
    }

    $entryNames = @(Get-ZipEntryName -ZipPath $ZipPath)
    $entryFileNames = @(
        $entryNames |
            Where-Object { -not $_.EndsWith("/") } |
            ForEach-Object { [System.IO.Path]::GetFileName($_) }
    )
    $runtimeFileNames = @(
        $entryNames |
            Where-Object {
                $_.StartsWith("$StageName/desktop-qt/", [System.StringComparison]::OrdinalIgnoreCase) -and
                    $_.Substring($StageName.Length + 12) -notmatch '/'
            } |
            ForEach-Object { [System.IO.Path]::GetFileName($_) }
    )
    $missing = @()
    foreach ($relativePath in $requiredFiles) {
        $entry = "$StageName/$relativePath"
        if ($entryNames -notcontains $entry) {
            $missing += $entry
        }
    }
    foreach ($directory in $requiredDirectories) {
        $prefix = "$StageName/$directory/"
        $hasFiles = @(
            $entryNames | Where-Object {
                $_.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase) -and
                    (-not $_.EndsWith("/"))
            }
        ).Count -gt 0
        if (-not $hasFiles) {
            $missing += $prefix
        }
    }
    foreach ($pluginName in $requiredPlugins) {
        if ($entryFileNames -notcontains $pluginName) {
            $missing += "$StageName/**/$pluginName"
        }
    }
    if ($QtIntegrated -and (-not (Test-CompilerRuntime -FileNames $runtimeFileNames))) {
        $missing += "$StageName/desktop-qt/(vc_redist.x64.exe o runtime MinGW)"
    }

    if ($missing.Count -gt 0) {
        throw ("El ZIP Windows esta incompleto. Faltan:`n  - " + ($missing -join "`n  - "))
    }
}

if ($RequireQtStage -and
    (-not (Test-Path -LiteralPath $DesktopQmlEscenario -PathType Container))) {
    throw "No existe la stage Qt/QML requerida: $DesktopQmlEscenario"
}
if ($RequireQtStage) {
    Assert-QtSuiteStage -StageDir $DesktopQmlEscenario
}
if ($RequireWinUiStage) {
    if (-not (Test-Path -LiteralPath $DesktopWinUiEscenario -PathType Container)) {
        throw "No existe la stage WinUI requerida: $DesktopWinUiEscenario"
    }
    Assert-WinUiRuntimeStage -StageDir $DesktopWinUiEscenario
}

if (Test-Path $Escenario) {
    Remove-Item $Escenario -Recurse -Force
}
if (Test-Path $Zip) {
    Remove-Item $Zip -Force
}
if (Test-Path $Setup) {
    Remove-Item $Setup -Force
}
New-Item -ItemType Directory -Force -Path $Escenario | Out-Null

Write-Output "Compilando suite de Windows ($Arquitectura)..."
$env:GOOS = "windows"
$env:GOARCH = $Arquitectura

Invoke-GrxFirmaGoBuild `
    -Version $Version `
    -WorkingDirectory $Raiz `
    -BuildArguments @("-tags", "production", "-o", (Join-Path $Escenario "grxfirma.exe"), "./cmd/grxfirma")
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion de la CLI fallo con codigo $LASTEXITCODE"
}
Invoke-GrxFirmaGoBuild `
    -Version $Version `
    -WorkingDirectory $Raiz `
    -LinkerFlags @("-H=windowsgui") `
    -BuildArguments @("-tags", "production", "-o", (Join-Path $Escenario "grxfirma-gui.exe"), "./cmd/grxfirma-gui")
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion del lanzador desktop compartido fallo con codigo $LASTEXITCODE"
}
Invoke-GrxFirmaGoBuild `
    -Version $Version `
    -WorkingDirectory $Raiz `
    -BuildArguments @("-tags", "production", "-o", (Join-Path $Escenario "grxfirma-nativehost.exe"), "./cmd/nativehost")
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion del NativeHost fallo con codigo $LASTEXITCODE"
}
$cgoAnteriorDefinido = Test-Path Env:CGO_ENABLED
$cgoAnterior = $env:CGO_ENABLED
try {
    # Nunca distribuir el handler web con la implementacion headless: la
    # seleccion de certificado y la aprobacion de firma deben ser interactivas.
    $env:CGO_ENABLED = "1"
    Invoke-GrxFirmaGoBuild `
        -Version $Version `
        -WorkingDirectory $Raiz `
        -LinkerFlags @("-H=windowsgui") `
        -BuildArguments @(
            "-tags",
            "production,fyne_gui",
            "-o",
            $AfirmaUriExe,
            "./cmd/grxfirmauri"
        )
} finally {
    if ($cgoAnteriorDefinido) {
        $env:CGO_ENABLED = $cgoAnterior
    } else {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion de AfirmaURI fallo con codigo $LASTEXITCODE"
}
Assert-GrxFirmaWindowsFyneArtifact `
    -Path $AfirmaUriExe `
    -Architecture $Arquitectura

Copy-Item (Join-Path $Raiz "packaging/windows/README_WINDOWS_SUITE.md") (Join-Path $Escenario "README_WINDOWS_SUITE.md") -Force
New-Item -ItemType Directory -Force -Path (Join-Path $Escenario "help") | Out-Null
Copy-Item (Join-Path $Raiz "docs/NOVEDADES.md") (Join-Path $Escenario "help/NOVEDADES.md") -Force
# Plantillas de directiva de grupo para los administradores.
foreach ($idiomaAdmx in @("es-ES", "en-US")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $Escenario "policies/$idiomaAdmx") | Out-Null
    Copy-Item (Join-Path $Raiz "packaging/windows/admx/$idiomaAdmx/GrxFirma.adml") (Join-Path $Escenario "policies/$idiomaAdmx/GrxFirma.adml") -Force
}
Copy-Item (Join-Path $Raiz "packaging/windows/admx/GrxFirma.admx") (Join-Path $Escenario "policies/GrxFirma.admx") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/grxfirma-grx.ico") (Join-Path $Escenario "grxfirma-grx.ico") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-suite.ps1") (Join-Path $Escenario "install-suite.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-nativehost.ps1") (Join-Path $Escenario "install-nativehost.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-afirmauri.ps1") (Join-Path $Escenario "install-afirmauri.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/afirmauri-registration.ps1") (Join-Path $Escenario "afirmauri-registration.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-suite.ps1") (Join-Path $Escenario "uninstall-suite.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-nativehost.ps1") (Join-Path $Escenario "uninstall-nativehost.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-afirmauri.ps1") (Join-Path $Escenario "uninstall-afirmauri.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-desktop-qml.ps1") (Join-Path $Escenario "install-desktop-qml.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-desktop-qml.ps1") (Join-Path $Escenario "uninstall-desktop-qml.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-desktop-winui.ps1") (Join-Path $Escenario "install-desktop-winui.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-desktop-winui.ps1") (Join-Path $Escenario "uninstall-desktop-winui.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/remove-unselected-desktop.ps1") (Join-Path $Escenario "remove-unselected-desktop.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-path-safety.ps1") (Join-Path $Escenario "install-path-safety.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/invoke-uninstall-silent.ps1") (Join-Path $Escenario "invoke-uninstall-silent.ps1") -Force
New-Item -ItemType Directory -Force -Path (Join-Path $Escenario "extensions") | Out-Null
$extensionOutput = Join-Path $Escenario "extensions"
& python (Join-Path $Raiz "packaging/browser-extensions/build.py") --output-dir $extensionOutput
if ($LASTEXITCODE -ne 0) { throw "La construcción de extensiones falló" }
& python (Join-Path $Raiz "packaging/browser-extensions/verify_package.py") $extensionOutput
if ($LASTEXITCODE -ne 0) { throw "La identidad de las extensiones no coincide con GrxFirma" }
Set-Content -Path (Join-Path $Escenario "VERSION.txt") -Value $Version -Encoding UTF8

$qtIntegrated = $false
$qtStageReady = $false
if (Test-Path -LiteralPath $DesktopQmlEscenario -PathType Container) {
    try {
        Assert-QtSuiteStage -StageDir $DesktopQmlEscenario
        $qtStageReady = $true
    } catch {
        if ($RequireQtStage) {
            throw
        }
        Write-Warning (
            "Se ignora la stage Qt/QML incompleta porque no se ha solicitado " +
            "--with-qt. Detalle: $($_.Exception.Message)"
        )
    }
}

if ($qtStageReady) {
    Write-Output "Integrando frontend desktop Qt/QML en un payload aislado..."
    $qtPayload = Join-Path $Escenario "desktop-qt"
    Copy-Item -LiteralPath $DesktopQmlEscenario -Destination $qtPayload -Recurse
    Assert-QtRuntimeStage -StageDir $qtPayload
    $qtIntegrated = $true
} elseif ($RequireQtStage) {
    throw "No existe la stage Qt/QML completa requerida: $DesktopQmlEscenario"
}

$winUiIntegrated = $false
if (Test-Path -LiteralPath $DesktopWinUiEscenario -PathType Container) {
    Assert-WinUiRuntimeStage -StageDir $DesktopWinUiEscenario
    foreach ($stageVersionFile in @("VERSION.txt", "app/VERSION.txt")) {
        $stageVersion = (Get-Content -LiteralPath (Join-Path $DesktopWinUiEscenario $stageVersionFile) -Raw).Trim()
        if ($stageVersion -ne $Version) {
            throw "La stage WinUI usa $stageVersion en $stageVersionFile; reconstruya WinUI con VERSION.txt=$Version antes de empaquetar."
        }
    }
    $sourceNotesHash = (Get-FileHash -LiteralPath (Join-Path $Raiz "docs/NOVEDADES.md") -Algorithm SHA256).Hash
    $stageNotesHash = (Get-FileHash -LiteralPath (Join-Path $DesktopWinUiEscenario "app/help/NOVEDADES.md") -Algorithm SHA256).Hash
    if ($stageNotesHash -ne $sourceNotesHash) {
        throw "La stage WinUI contiene novedades distintas de docs/NOVEDADES.md; reconstruya WinUI antes de empaquetar."
    }
    Write-Output "Integrando frontend desktop WinUI autocontenido..."
    $winUiPayload = Join-Path $Escenario "desktop-winui"
    Copy-Item -LiteralPath $DesktopWinUiEscenario -Destination $winUiPayload -Recurse
    Assert-WinUiRuntimeStage -StageDir $winUiPayload
    $winUiIntegrated = $true
} elseif ($RequireWinUiStage) {
    throw "No existe la stage WinUI completa requerida: $DesktopWinUiEscenario"
}

Sync-SharedDesktopBackends `
    -StageDir $Escenario `
    -QtIntegrated $qtIntegrated `
    -WinUiIntegrated $winUiIntegrated

if (Test-Path $Zip) {
    Remove-Item $Zip -Force
}
New-GrxFirmaReproducibleZip `
    -SourceDirectory $Escenario `
    -DestinationPath $Zip `
    -SourceDateEpoch $SourceDateEpoch
Write-Output "ZIP generado en: $Zip"

$qtHelpIntegrated = $false
if ($qtIntegrated) {
    $qtHelpPath = Join-Path $DesktopQmlEscenario "help"
    if (Test-Path -LiteralPath $qtHelpPath -PathType Container) {
        $qtHelpIntegrated = $null -ne (
            Get-ChildItem -LiteralPath $qtHelpPath -File -Recurse -ErrorAction SilentlyContinue |
                Select-Object -First 1
        )
    }
}
Assert-SuiteZipArtifact `
    -ZipPath $Zip `
    -StageName (Split-Path $Escenario -Leaf) `
    -QtIntegrated $qtIntegrated `
    -QtHelpIntegrated $qtHelpIntegrated `
    -WinUiIntegrated $winUiIntegrated

if ($args -contains "--nsis") {
    $nsisScript = Join-Path $Raiz "packaging/windows/grxfirma-suite.nsi"
    $nsisPathContext = New-GrxFirmaNsisPathContext `
        -RepositoryRoot $Raiz `
        -StageDirectory $Escenario `
        -OutputPath $Setup `
        -ScriptPath $nsisScript
    $nsisArguments = @(
        "/DVERSION=$Version",
        "/DARCH=$Arquitectura",
        "/DSTAGE_DIR=$($nsisPathContext.StageDirectory)",
        "/DOUT_FILE=$($nsisPathContext.OutputPath)"
    )
    if ($winUiIntegrated) {
        $nsisArguments += "/DHAS_WINUI=1"
    }
    if ($qtIntegrated) {
        $nsisArguments += "/DHAS_QT=1"
    }
    $nsisArguments += $nsisPathContext.ScriptPath
    try {
        & $MakeNsis @nsisArguments
        if ($LASTEXITCODE -ne 0) {
            throw "La construccion NSIS de la suite fallo con codigo $LASTEXITCODE"
        }
    } finally {
        Remove-GrxFirmaNsisPathContext -Context $nsisPathContext
    }
    Set-GrxFirmaFileTimestamp -Path $Setup -SourceDateEpoch $SourceDateEpoch
    Write-Output "Instalador NSIS generado en: $Setup"
}

Write-ArtifactManifest -OutputDir $Salida -Candidates @($Zip, $Setup)
