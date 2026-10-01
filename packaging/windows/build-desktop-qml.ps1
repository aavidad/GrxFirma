# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

$Raiz = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
. (Join-Path $PSScriptRoot "reproducible-build.ps1")
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
$Salida = Join-Path $Raiz "release/windows-desktop-qml"
$Escenario = Join-Path $Salida "GrxFirma-$Version-desktop-qml-windows-$Arquitectura"
$BuildDir = Join-Path $Salida "build-$Arquitectura"
$Zip = Join-Path $Salida "GrxFirma-$Version-desktop-qml-windows-$Arquitectura.zip"
$Setup = Join-Path $Salida "GrxFirma-$Version-desktop-qml-windows-$Arquitectura-setup.exe"
$ProyectoQml = Join-Path $Raiz "cmd/gui-qml/grxfirma_qt.pro"
$MakeNsis = if ($args -contains "--nsis") {
    Resolve-GrxFirmaMakeNsis
} else {
    $null
}
$HelpSource = Join-Path $Raiz "cmd/gui-qml/help"
$HelpAvailable = $false
if (Test-Path -LiteralPath $HelpSource -PathType Container) {
    $HelpAvailable = $null -ne (
        Get-ChildItem -LiteralPath $HelpSource -File -Recurse -ErrorAction SilentlyContinue |
            Select-Object -First 1
    )
}

function Get-QMakeCommand {
    if ($env:QMAKE) {
        return $env:QMAKE
    }
    foreach ($candidate in @("qmake6", "qmake")) {
        $cmd = Get-Command $candidate -ErrorAction SilentlyContinue
        if (-not $cmd) { continue }
        $version = & $cmd.Source -query QT_VERSION 2>$null
        if ($version -and $version.StartsWith("6.")) {
            return $cmd.Source
        }
    }
    throw "Se requiere Qt6 para compilar el frontend Qt/QML."
}

function Get-WinDeployQtCommand {
    param(
        [string]$QMake
    )

    if ($env:WINDEPLOYQT) {
        if (-not (Test-Path -LiteralPath $env:WINDEPLOYQT -PathType Leaf)) {
            throw "WINDEPLOYQT no apunta a un ejecutable valido: $env:WINDEPLOYQT"
        }
        return $env:WINDEPLOYQT
    }

    $qtBinDir = (& $QMake -query QT_INSTALL_BINS 2>$null)
    if (-not [string]::IsNullOrWhiteSpace($qtBinDir)) {
        foreach ($name in @("windeployqt.exe", "windeployqt")) {
            $candidate = Join-Path $qtBinDir $name
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                return $candidate
            }
        }
    }

    $command = Get-Command windeployqt -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    throw "No se ha encontrado windeployqt en el kit Qt seleccionado."
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
        "# Windows Desktop Qt/QML Artifacts",
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
        throw ("El runtime Qt/QML desplegado esta incompleto. Faltan:`n  - " + ($missing -join "`n  - "))
    }
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

function Assert-DesktopQmlZipArtifact {
    param(
        [string]$ZipPath,
        [string]$StageName,
        [bool]$ExpectHelp
    )

    $requiredFiles = @(
        "grxfirma-gui-qml.exe",
        "grxfirma-gui.exe",
        "grxfirma.exe",
        "install-desktop-qml.ps1",
        "uninstall-desktop-qml.ps1",
        "install-path-safety.ps1",
        "invoke-uninstall-silent.ps1",
        "README_DESKTOP_QML_WINDOWS.md",
        "VERSION.txt"
    ) + @(Get-QtRuntimeRequirement)
    $requiredDirectories = @("qml", "assets")
    if ($ExpectHelp) {
        $requiredDirectories += "help"
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
                $_.StartsWith("$StageName/", [System.StringComparison]::OrdinalIgnoreCase) -and
                    $_.Substring($StageName.Length + 1) -notmatch '/'
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
    foreach ($pluginName in Get-QtQmlPluginRequirement) {
        if ($entryFileNames -notcontains $pluginName) {
            $missing += "$StageName/**/$pluginName"
        }
    }
    if (-not (Test-CompilerRuntime -FileNames $runtimeFileNames)) {
        $missing += "$StageName/(vc_redist.x64.exe o runtime MinGW)"
    }

    if ($missing.Count -gt 0) {
        throw ("El ZIP Qt/QML esta incompleto. Faltan:`n  - " + ($missing -join "`n  - "))
    }
}

foreach ($path in @($Escenario, $BuildDir)) {
    if (Test-Path $path) {
        Remove-Item $path -Recurse -Force
    }
}
if (Test-Path $Setup) {
    Remove-Item $Setup -Force
}
New-Item -ItemType Directory -Force -Path $Escenario | Out-Null
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

Write-Output "Compilando backend Go para Windows ($Arquitectura)..."
$env:GOOS = "windows"
$env:GOARCH = $Arquitectura
Invoke-GrxFirmaGoBuild `
    -Version $Version `
    -WorkingDirectory $Raiz `
    -LinkerFlags @("-H=windowsgui") `
    -BuildArguments @("-tags", "production", "-o", (Join-Path $Escenario "grxfirma-gui.exe"), "./cmd/grxfirma-gui")
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion del backend IPC de la GUI fallo con codigo $LASTEXITCODE"
}
Invoke-GrxFirmaGoBuild `
    -Version $Version `
    -WorkingDirectory $Raiz `
    -BuildArguments @("-tags", "production", "-o", (Join-Path $Escenario "grxfirma.exe"), "./cmd/grxfirma")
if ($LASTEXITCODE -ne 0) {
    throw "La compilacion del backend Go fallo con codigo $LASTEXITCODE"
}

Write-Output "Compilando frontend Qt/QML..."
$QMake = Get-QMakeCommand
$WinDeployQt = Get-WinDeployQtCommand -QMake $QMake
# Toolchain del kit Qt: con MSVC hay que usar nmake aunque exista un
# mingw32-make en PATH (el Makefile generado no es compatible con GNU make).
$QtSpec = (& $QMake -query QMAKE_SPEC 2>$null)
$EsMsvc = ($QtSpec -match "msvc")
Push-Location $BuildDir
& $QMake $ProyectoQml
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    throw "La configuracion qmake fallo con codigo $LASTEXITCODE"
}
if ($env:GRXFIRMA_MAKE) {
    & $env:GRXFIRMA_MAKE
} elseif ($EsMsvc) {
    if (-not (Get-Command nmake -ErrorAction SilentlyContinue)) {
        throw "Kit Qt MSVC detectado ($QtSpec) pero nmake no esta en PATH (falta el entorno de VS)."
    }
    nmake
} elseif (Get-Command mingw32-make -ErrorAction SilentlyContinue) {
    mingw32-make
} elseif (Get-Command nmake -ErrorAction SilentlyContinue) {
    nmake
} else {
    throw "No se ha encontrado mingw32-make ni nmake."
}
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    throw "La compilacion del frontend Qt fallo con codigo $LASTEXITCODE"
}
Pop-Location

# MinGW deja el exe en la raiz del build; MSVC/nmake en el subdirectorio release.
$qmlExe = Join-Path $BuildDir "grxfirma-gui-qml.exe"
if (-not (Test-Path $qmlExe)) {
    $qmlExe = Join-Path $BuildDir "release/grxfirma-gui-qml.exe"
}
if (-not (Test-Path $qmlExe)) {
    throw "No se ha generado grxfirma-gui-qml.exe"
}
Copy-Item $qmlExe (Join-Path $Escenario "grxfirma-gui-qml.exe") -Force

Copy-Item (Join-Path $Raiz "cmd/gui-qml/qml") (Join-Path $Escenario "qml") -Recurse -Force
Copy-Item (Join-Path $Raiz "cmd/gui-qml/assets") (Join-Path $Escenario "assets") -Recurse -Force
Copy-Item `
    (Join-Path $Raiz "packaging/windows/grxfirma-diputacion.ico") `
    (Join-Path $Escenario "assets/grxfirma-diputacion.ico") `
    -Force
if ($HelpAvailable) {
    Copy-Item (Join-Path $Raiz "cmd/gui-qml/help") (Join-Path $Escenario "help") -Recurse -Force
}
Copy-Item (Join-Path $Raiz "packaging/windows/install-desktop-qml.ps1") (Join-Path $Escenario "install-desktop-qml.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/uninstall-desktop-qml.ps1") (Join-Path $Escenario "uninstall-desktop-qml.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/install-path-safety.ps1") (Join-Path $Escenario "install-path-safety.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/invoke-uninstall-silent.ps1") (Join-Path $Escenario "invoke-uninstall-silent.ps1") -Force
Copy-Item (Join-Path $Raiz "packaging/windows/README_DESKTOP_QML_WINDOWS.md") (Join-Path $Escenario "README_DESKTOP_QML_WINDOWS.md") -Force
Set-Content -Path (Join-Path $Escenario "VERSION.txt") -Value $Version -Encoding UTF8

Write-Output "Desplegando runtime Qt..."
# El mismo kit decide el layout QML y las dependencias de MSVC o MinGW.
& $WinDeployQt --compiler-runtime --qmldir (Join-Path $Raiz "cmd/gui-qml/qml") (Join-Path $Escenario "grxfirma-gui-qml.exe")
if ($LASTEXITCODE -ne 0) {
    throw "windeployqt fallo con codigo $LASTEXITCODE"
}
Assert-QtRuntimeStage -StageDir $Escenario

if (Test-Path $Zip) {
    Remove-Item $Zip -Force
}
New-GrxFirmaReproducibleZip `
    -SourceDirectory $Escenario `
    -DestinationPath $Zip `
    -SourceDateEpoch $SourceDateEpoch
Write-Output "ZIP generado en: $Zip"

Assert-DesktopQmlZipArtifact `
    -ZipPath $Zip `
    -StageName (Split-Path $Escenario -Leaf) `
    -ExpectHelp $HelpAvailable

if ($args -contains "--nsis") {
    & $MakeNsis `
        /DVERSION=$Version `
        /DARCH=$Arquitectura `
        /DSTAGE_DIR=$Escenario `
        /DOUT_FILE=$Setup `
        (Join-Path $Raiz "packaging/windows/grxfirma-desktop-qml.nsi")
    if ($LASTEXITCODE -ne 0) {
        throw "La construccion NSIS Qt/QML fallo con codigo $LASTEXITCODE"
    }
    Set-GrxFirmaFileTimestamp -Path $Setup -SourceDateEpoch $SourceDateEpoch
    Write-Output "Instalador NSIS generado en: $Setup"
}

Write-ArtifactManifest -OutputDir $Salida -Candidates @($Zip, $Setup)
