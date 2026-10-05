# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Finalizador Windows cuando firma SignPath Foundation. Ninguna clave pasa por
# este runner: los bytes viajan a SignPath como artefactos de GitHub Actions y
# vuelven firmados. El workflow alterna estas fases con la accion oficial:
#
#   Prepare     selecciona los PE que hay que firmar (los mismos que con
#               certificado propio), captura los uninstallers NSIS y deja la
#               ronda 1 en
#               <WorkDirectory>/round1-unsigned.
#   Installers  comprueba la ronda 1 firmada, la aplica a las etapas, replica el
#               backend, regenera los ZIP y genera los instaladores con el
#               uninstaller firmado. Deja la ronda 2 en round2-unsigned.
#   Complete    comprueba los instaladores firmados, los publica junto a los
#               ZIP en release/windows-official y regenera WINDOWS-SIGNATURES.json
#               y SHA256SUMS-windows.txt.
#
# Cada fichero devuelto debe ser el original con una firma Authenticode
# anadida al final, del certificado fijado y con sello RFC3161 SHA-256.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('Prepare', 'Installers', 'Complete')]
    [string]$Phase,

    [Parameter(Mandatory = $true)]
    [string]$RepositoryRoot,

    [Parameter(Mandatory = $true)]
    [string]$WorkDirectory,

    [string]$OutputDirectory
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if (-not $IsWindows) {
    throw "La firma Authenticode oficial requiere Windows."
}

foreach ($helperName in @('windows-authenticode.ps1', 'windows-suite-layout.ps1')) {
    $helperPath = Join-Path $PSScriptRoot $helperName
    if (-not (Test-Path -LiteralPath $helperPath -PathType Leaf)) {
        throw "Falta $helperName."
    }
    . $helperPath
}

if ([string]::IsNullOrWhiteSpace($env:WINDOWS_SIGNING_CERT_THUMBPRINT)) {
    throw "Falta la huella obligatoria WINDOWS_SIGNING_CERT_THUMBPRINT"
}
$expectedThumbprint = Get-NormalizedWindowsSigningThumbprint `
    -Thumbprint $env:WINDOWS_SIGNING_CERT_THUMBPRINT

$RepositoryRoot = (Resolve-Path -LiteralPath $RepositoryRoot).Path
$WorkDirectory = [System.IO.Path]::GetFullPath($WorkDirectory)
$repositoryPrefix = $RepositoryRoot.TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar
if ($WorkDirectory.StartsWith($repositoryPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "WorkDirectory debe quedar fuera del repositorio para no mezclarse con la etapa."
}
$planPath = Join-Path $WorkDirectory 'plan.json'
$round1Unsigned = Join-Path $WorkDirectory 'round1-unsigned'
$round1Signed = Join-Path $WorkDirectory 'round1-signed'
$round2Unsigned = Join-Path $WorkDirectory 'round2-unsigned'
$round2Signed = Join-Path $WorkDirectory 'round2-signed'
$nsisDirectory = Join-Path $WorkDirectory 'nsis'

$signToolPath = Resolve-WindowsSignTool
$version = (Get-Content -LiteralPath (Join-Path $RepositoryRoot 'VERSION.txt') -Raw).Trim()
$architecture = 'amd64'

function Get-ReleaseSha256 {
    param([Parameter(Mandatory = $true)][string]$Path)
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Get-ReleaseBuilds {
    return @(
        [ordered]@{
            Kind = "desktop-qml"
            Stage = Join-Path $RepositoryRoot "release/windows-desktop-qml/GrxFirma-$version-desktop-qml-windows-$architecture"
            Zip = Join-Path $RepositoryRoot "release/windows-desktop-qml/GrxFirma-$version-desktop-qml-windows-$architecture.zip"
            Setup = Join-Path $RepositoryRoot "release/windows-desktop-qml/GrxFirma-$version-desktop-qml-windows-$architecture-setup.exe"
            Nsi = Join-Path $RepositoryRoot 'packaging/windows/grxfirma-desktop-qml.nsi'
        },
        [ordered]@{
            Kind = "suite"
            Stage = Join-Path $RepositoryRoot "release/windows-suite/GrxFirma-$version-windows-$architecture"
            Zip = Join-Path $RepositoryRoot "release/windows-suite/GrxFirma-$version-windows-$architecture.zip"
            Setup = Join-Path $RepositoryRoot "release/windows-suite/GrxFirma-$version-windows-$architecture-setup.exe"
            Nsi = Join-Path $RepositoryRoot 'packaging/windows/grxfirma-suite.nsi'
        }
    )
}

function Get-NsisComponentDefines {
    param([Parameter(Mandatory = $true)]$Build)
    if ($Build.Kind -ne 'suite') {
        return @()
    }
    $defines = @(
        Get-WindowsSuiteComponentDefines `
            -StageDirectory $Build.Stage `
            -DefinePrefix "/D"
    )
    if ($defines -notcontains "/DHAS_WINUI=1" -or
        $defines -notcontains "/DHAS_QT=1") {
        throw "La Suite Windows oficial debe contener WinUI y Qt completos."
    }
    return $defines
}

function Get-RelativeFileSet {
    param([Parameter(Mandatory = $true)][string]$Root)
    if (-not (Test-Path -LiteralPath $Root -PathType Container)) {
        throw "No existe el directorio devuelto por SignPath: $Root"
    }
    $reparse = Get-ChildItem -LiteralPath $Root -Recurse -Force |
        Where-Object { ($_.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0 } |
        Select-Object -First 1
    if ($null -ne $reparse) {
        throw "El artefacto devuelto contiene un punto de reanalisis: $($reparse.FullName)"
    }
    return @(
        Get-ChildItem -LiteralPath $Root -Recurse -File -Force |
            ForEach-Object {
                [System.IO.Path]::GetRelativePath($Root, $_.FullName).Replace('\', '/')
            } |
            Sort-Object
    )
}

function Assert-ExactFileSet {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string[]]$Expected
    )
    $actual = @(Get-RelativeFileSet -Root $Root)
    $expectedSorted = @($Expected | Sort-Object)
    $missing = @($expectedSorted | Where-Object { $actual -notcontains $_ })
    $extra = @($actual | Where-Object { $expectedSorted -notcontains $_ })
    if ($missing.Count -gt 0 -or $extra.Count -gt 0) {
        throw "El artefacto firmado no coincide con lo enviado. Faltan: [$($missing -join ', ')] Sobran: [$($extra -join ', ')]"
    }
}

function Assert-SignPathReturn {
    param(
        [Parameter(Mandatory = $true)][string]$UnsignedPath,
        [Parameter(Mandatory = $true)][string]$SignedPath,
        [Parameter(Mandatory = $true)][string]$ExpectedUnsignedSha256
    )
    if ((Get-ReleaseSha256 -Path $UnsignedPath) -ne $ExpectedUnsignedSha256) {
        throw "La copia sin firmar cambio despues de enviarla a SignPath: $UnsignedPath"
    }
    Assert-WindowsAuthenticodeSignedCopy `
        -UnsignedPath $UnsignedPath `
        -SignedPath $SignedPath
    Assert-WindowsAuthenticodeFile `
        -Path $SignedPath `
        -SignToolPath $signToolPath `
        -ExpectedThumbprint $expectedThumbprint `
        -RequireSha256Rfc3161 |
        Out-Null
}

function Assert-SafeHookPath {
    param([Parameter(Mandatory = $true)][string[]]$Paths)
    foreach ($path in $Paths) {
        if ($path -match '["%\r\n]') {
            throw "Ruta no representable de forma segura en el hook NSIS: $path"
        }
    }
}

function Invoke-NsisWithHook {
    param(
        [Parameter(Mandatory = $true)]$Build,
        [Parameter(Mandatory = $true)][string]$OutFile,
        [Parameter(Mandatory = $true)][System.Collections.IDictionary]$HookConfiguration,
        [Parameter(Mandatory = $true)][string]$Token
    )
    $makensis = Get-Command makensis -ErrorAction Stop
    $hookScript = Join-Path $PSScriptRoot 'signpath-uninstaller-hook.ps1'
    $currentPowerShell = (Get-Process -Id $PID).Path
    $configurationPath = Join-Path $nsisDirectory "$Token-hook.json"
    $wrapperPath = Join-Path $nsisDirectory "$Token-hook.cmd"
    Assert-SafeHookPath -Paths @($hookScript, $currentPowerShell, $configurationPath, $wrapperPath)

    [System.IO.File]::WriteAllText(
        $configurationPath,
        ($HookConfiguration | ConvertTo-Json -Depth 4) + [Environment]::NewLine,
        [System.Text.UTF8Encoding]::new($false)
    )
    $wrapperContent = @"
@echo off
"$currentPowerShell" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$hookScript" -ConfigurationPath "$configurationPath" -FilePath "%~1"
exit /b %ERRORLEVEL%
"@
    Set-Content -LiteralPath $wrapperPath -Value $wrapperContent -Encoding ASCII

    $componentDefines = @(Get-NsisComponentDefines -Build $Build)
    Remove-Item -LiteralPath $OutFile -Force -ErrorAction SilentlyContinue
    & $makensis.Source `
        "/DVERSION=$version" `
        "/DARCH=$architecture" `
        "/DSTAGE_DIR=$($Build.Stage)" `
        "/DOUT_FILE=$OutFile" `
        "/DGRXFIRMA_UNINSTALL_SIGNER=$wrapperPath" `
        @componentDefines `
        $Build.Nsi
    if ($LASTEXITCODE -ne 0) {
        throw "makensis fallo con codigo $LASTEXITCODE para $($Build.Nsi)"
    }
    if (-not (Test-Path -LiteralPath $OutFile -PathType Leaf)) {
        throw "makensis no genero $OutFile"
    }
}

function Read-SigningPlan {
    if (-not (Test-Path -LiteralPath $planPath -PathType Leaf)) {
        throw "Falta el plan de firma SignPath: $planPath"
    }
    $plan = Get-Content -LiteralPath $planPath -Raw | ConvertFrom-Json
    if ($plan.schema_version -ne 1 -or
        $plan.version -ne $version -or
        $plan.expected_signer_thumbprint -ne $expectedThumbprint) {
        throw "El plan de firma SignPath no corresponde a esta version o huella."
    }
    return $plan
}

function Write-SigningPlan {
    param([Parameter(Mandatory = $true)]$Plan)
    [System.IO.File]::WriteAllText(
        $planPath,
        ($Plan | ConvertTo-Json -Depth 8) + [Environment]::NewLine,
        [System.Text.UTF8Encoding]::new($false)
    )
}

function Get-PlanBuild {
    param(
        [Parameter(Mandatory = $true)]$Plan,
        [Parameter(Mandatory = $true)][string]$Kind
    )
    $found = @($Plan.builds | Where-Object { $_.kind -eq $Kind })
    if ($found.Count -ne 1) {
        throw "El plan de firma no contiene exactamente una entrada para $Kind"
    }
    return $found[0]
}

switch ($Phase) {
    'Prepare' {
        if (Test-Path -LiteralPath $WorkDirectory) {
            Remove-Item -LiteralPath $WorkDirectory -Recurse -Force
        }
        foreach ($directory in @($WorkDirectory, $round1Unsigned, $nsisDirectory)) {
            New-Item -ItemType Directory -Force -Path $directory | Out-Null
        }
        New-Item -ItemType Directory -Force -Path (Join-Path $round1Unsigned 'uninstallers') | Out-Null

        $planBuilds = @()
        foreach ($build in Get-ReleaseBuilds) {
            if (-not (Test-Path -LiteralPath $build.Stage -PathType Container)) {
                throw "Falta la etapa Windows que debe firmarse: $($build.Stage)"
            }
            $isSuite = $build.Kind -eq 'suite'
            [void](Get-NsisComponentDefines -Build $build)
            $portableExecutables = @(
                Get-ChildItem -LiteralPath $build.Stage -Recurse -File |
                    Where-Object { $_.Extension -in @('.exe', '.dll') }
            )
            if (@($portableExecutables | Where-Object { $_.Extension -eq '.dll' }).Count -eq 0) {
                throw "La etapa Qt no contiene bibliotecas DLL que verificar y firmar: $($build.Stage)"
            }
            $signingTargets = @(
                Get-WindowsReleaseSigningTargets `
                    -StageDirectory $build.Stage `
                    -Suite:$isSuite
            )
            if (@($signingTargets | Where-Object { $_.Name -like 'grxfirma*.exe' }).Count -eq 0) {
                throw "No hay ejecutables propios que firmar en $($build.Stage)"
            }

            # Mismo criterio que con certificado propio: se firman los PE propios y los
            # de terceros sin firma; se conservan las firmas validas con sello
            # de tiempo y se rechaza cualquier otra.
            $payload = @()
            foreach ($portableExecutable in $signingTargets) {
                if ($portableExecutable.Name -notlike 'grxfirma*.exe') {
                    $vendorSignature = Get-AuthenticodeSignature -FilePath $portableExecutable.FullName
                    if ($vendorSignature.Status -eq [System.Management.Automation.SignatureStatus]::Valid) {
                        if ($null -eq $vendorSignature.TimeStamperCertificate) {
                            throw "El binario de proveedor carece de timestamp Authenticode: $($portableExecutable.FullName)"
                        }
                        continue
                    }
                    if ($vendorSignature.Status -ne [System.Management.Automation.SignatureStatus]::NotSigned) {
                        throw "Firma de proveedor no valida en $($portableExecutable.FullName): $($vendorSignature.Status)"
                    }
                }
                $relative = [System.IO.Path]::GetRelativePath(
                    $build.Stage,
                    $portableExecutable.FullName
                ).Replace('\', '/')
                $destination = Join-Path $round1Unsigned "payload/$($build.Kind)/$relative"
                New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
                Copy-Item -LiteralPath $portableExecutable.FullName -Destination $destination
                $payload += [ordered]@{
                    path = "payload/$($build.Kind)/$relative"
                    stage_relative = $relative
                    unsigned_sha256 = Get-ReleaseSha256 -Path $destination
                }
            }

            # El uninstaller no incluye la carga util: se puede capturar ya y
            # firmar en la misma ronda que los PE.
            $token = [System.IO.Path]::GetFileNameWithoutExtension([string]$build.Setup)
            $capture = Join-Path $round1Unsigned "uninstallers/$token-uninstall.exe"
            $pass1Setup = Join-Path $nsisDirectory "$token-capture.exe"
            Invoke-NsisWithHook `
                -Build $build `
                -OutFile $pass1Setup `
                -Token "$token-capture" `
                -HookConfiguration ([ordered]@{
                    schema_version = 1
                    mode = 'capture'
                    capture_path = $capture
                })
            Remove-Item -LiteralPath $pass1Setup -Force
            if (-not (Test-Path -LiteralPath $capture -PathType Leaf)) {
                throw "NSIS no entrego el uninstaller de $($build.Nsi) al hook de captura."
            }

            $planBuilds += [ordered]@{
                kind = $build.Kind
                payload = @($payload)
                uninstaller = [ordered]@{
                    path = "uninstallers/$token-uninstall.exe"
                    unsigned_sha256 = Get-ReleaseSha256 -Path $capture
                }
            }
        }

        Write-SigningPlan -Plan ([ordered]@{
            schema_version = 1
            version = $version
            expected_signer_thumbprint = $expectedThumbprint
            builds = @($planBuilds)
        })
        Write-Output "Ronda 1 SignPath preparada en $round1Unsigned"
    }

    'Installers' {
        $plan = Read-SigningPlan
        $expectedFiles = @()
        foreach ($planBuild in $plan.builds) {
            $expectedFiles += @($planBuild.payload | ForEach-Object { $_.path })
            $expectedFiles += $planBuild.uninstaller.path
        }
        Assert-ExactFileSet -Root $round1Signed -Expected $expectedFiles
        if (Test-Path -LiteralPath $round2Unsigned) {
            Remove-Item -LiteralPath $round2Unsigned -Recurse -Force
        }
        New-Item -ItemType Directory -Force -Path $round2Unsigned | Out-Null

        $installers = @()
        foreach ($build in Get-ReleaseBuilds) {
            $planBuild = Get-PlanBuild -Plan $plan -Kind $build.Kind
            foreach ($entry in $planBuild.payload) {
                $unsignedPath = Join-Path $round1Unsigned $entry.path
                $signedPath = Join-Path $round1Signed $entry.path
                Assert-SignPathReturn `
                    -UnsignedPath $unsignedPath `
                    -SignedPath $signedPath `
                    -ExpectedUnsignedSha256 $entry.unsigned_sha256
                $stageFile = Join-Path $build.Stage $entry.stage_relative
                if ((Get-ReleaseSha256 -Path $stageFile) -ne $entry.unsigned_sha256) {
                    throw "La etapa cambio mientras SignPath firmaba: $stageFile"
                }
                Copy-Item -LiteralPath $signedPath -Destination $stageFile -Force
            }

            $signedUninstaller = Join-Path $round1Signed $planBuild.uninstaller.path
            Assert-SignPathReturn `
                -UnsignedPath (Join-Path $round1Unsigned $planBuild.uninstaller.path) `
                -SignedPath $signedUninstaller `
                -ExpectedUnsignedSha256 $planBuild.uninstaller.unsigned_sha256

            if ($build.Kind -eq 'suite') {
                # El launcher se firma una sola vez; las copias de los payloads
                # reciben esos bytes y el inventario WinUI se regenera.
                Sync-WindowsSuiteSharedBackend -StageDirectory $build.Stage
            }

            Remove-Item -LiteralPath $build.Zip -Force -ErrorAction SilentlyContinue
            Compress-Archive -Path $build.Stage -DestinationPath $build.Zip -CompressionLevel Optimal

            $token = [System.IO.Path]::GetFileNameWithoutExtension([string]$build.Setup)
            $evidence = Join-Path $nsisDirectory "$token-uninstall-evidence.exe"
            Remove-Item -LiteralPath $evidence -Force -ErrorAction SilentlyContinue
            Invoke-NsisWithHook `
                -Build $build `
                -OutFile $build.Setup `
                -Token "$token-inject" `
                -HookConfiguration ([ordered]@{
                    schema_version = 1
                    mode = 'inject'
                    expected_unsigned_sha256 = $planBuild.uninstaller.unsigned_sha256
                    signed_path = $signedUninstaller
                    evidence_path = $evidence
                })
            if (-not (Test-Path -LiteralPath $evidence -PathType Leaf)) {
                throw "NSIS no produjo evidencia del uninstaller firmado para $($build.Nsi)"
            }
            if ((Get-ReleaseSha256 -Path $evidence) -ne (Get-ReleaseSha256 -Path $signedUninstaller)) {
                throw "El instalador no incorpora el uninstaller firmado por SignPath: $($build.Nsi)"
            }
            Assert-WindowsAuthenticodeFile `
                -Path $evidence `
                -SignToolPath $signToolPath `
                -ExpectedThumbprint $expectedThumbprint `
                -RequireSha256Rfc3161 |
                Out-Null

            $setupName = [System.IO.Path]::GetFileName([string]$build.Setup)
            Copy-Item -LiteralPath $build.Setup -Destination (Join-Path $round2Unsigned $setupName)
            $installers += [ordered]@{
                kind = $build.Kind
                path = $setupName
                unsigned_sha256 = Get-ReleaseSha256 -Path $build.Setup
                zip_sha256 = Get-ReleaseSha256 -Path $build.Zip
            }
        }

        $planDocument = [ordered]@{
            schema_version = 1
            version = $plan.version
            expected_signer_thumbprint = $plan.expected_signer_thumbprint
            builds = @($plan.builds)
            installers = @($installers)
        }
        Write-SigningPlan -Plan $planDocument
        Write-Output "Ronda 2 SignPath preparada en $round2Unsigned"
    }

    'Complete' {
        if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
            throw "La fase Complete necesita OutputDirectory."
        }
        $OutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
        $requiredOutputDirectory = [System.IO.Path]::GetFullPath(
            (Join-Path $RepositoryRoot 'release/windows-official')
        )
        if (-not $OutputDirectory.Equals(
                $requiredOutputDirectory,
                [System.StringComparison]::OrdinalIgnoreCase
            )) {
            throw "OutputDirectory debe ser $requiredOutputDirectory"
        }
        $plan = Read-SigningPlan
        if ($null -eq $plan.PSObject.Properties['installers'] -or
            @($plan.installers).Count -ne 2) {
            throw "El plan no contiene los dos instaladores de la ronda 2."
        }
        Assert-ExactFileSet `
            -Root $round2Signed `
            -Expected @($plan.installers | ForEach-Object { $_.path })

        if (Test-Path -LiteralPath $OutputDirectory) {
            Remove-Item -LiteralPath $OutputDirectory -Recurse -Force
        }
        New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null

        foreach ($build in Get-ReleaseBuilds) {
            $installer = @($plan.installers | Where-Object { $_.kind -eq $build.Kind })
            if ($installer.Count -ne 1) {
                throw "El plan no contiene exactamente un instalador para $($build.Kind)"
            }
            $installer = $installer[0]
            $signedSetup = Join-Path $round2Signed $installer.path
            Assert-SignPathReturn `
                -UnsignedPath (Join-Path $round2Unsigned $installer.path) `
                -SignedPath $signedSetup `
                -ExpectedUnsignedSha256 $installer.unsigned_sha256
            if ((Get-ReleaseSha256 -Path $build.Zip) -ne $installer.zip_sha256) {
                throw "El ZIP cambio despues de generar el instalador: $($build.Zip)"
            }
            Copy-Item -LiteralPath $build.Zip -Destination $OutputDirectory -Force
            Copy-Item -LiteralPath $signedSetup -Destination (Join-Path $OutputDirectory $installer.path) -Force
        }

        & (Join-Path $PSScriptRoot 'verify-windows-release.ps1') `
            -ArtifactDirectory $OutputDirectory `
            -ExpectedThumbprint $expectedThumbprint `
            -SignToolPath $signToolPath `
            -WriteManifest

        $publicFiles = @(Get-ChildItem -LiteralPath $OutputDirectory -File | Sort-Object Name)
        $checksumLines = foreach ($file in $publicFiles) {
            $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
            "{0}  {1}" -f $hash, $file.Name
        }
        Set-Content -LiteralPath (Join-Path $OutputDirectory 'SHA256SUMS-windows.txt') -Value $checksumLines -Encoding ASCII

        & (Join-Path $PSScriptRoot 'verify-windows-release.ps1') `
            -ArtifactDirectory $OutputDirectory `
            -ExpectedThumbprint $expectedThumbprint `
            -SignToolPath $signToolPath `
            -RequireEvidence `
            -RequireDualGui
        Write-Output "Artefactos Windows firmados por SignPath y verificados en $OutputDirectory"
    }
}
