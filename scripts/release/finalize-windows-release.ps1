# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$RepositoryRoot,

    [Parameter(Mandatory = $true)]
    [string]$OutputDirectory
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if (-not $IsWindows) {
    throw "La firma Authenticode oficial requiere Windows."
}

$authenticodeHelpers = Join-Path $PSScriptRoot 'windows-authenticode.ps1'
if (-not (Test-Path -LiteralPath $authenticodeHelpers -PathType Leaf)) {
    throw "Falta windows-authenticode.ps1."
}
. $authenticodeHelpers
$suiteLayoutHelpers = Join-Path $PSScriptRoot 'windows-suite-layout.ps1'
if (-not (Test-Path -LiteralPath $suiteLayoutHelpers -PathType Leaf)) {
    throw "Falta windows-suite-layout.ps1."
}
. $suiteLayoutHelpers

foreach ($name in @(
    'WINDOWS_SIGNING_PFX_BASE64',
    'WINDOWS_SIGNING_PFX_PASSWORD',
    'WINDOWS_SIGNING_CERT_THUMBPRINT'
)) {
    if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))) {
        throw "Falta la credencial obligatoria $name"
    }
}

$RepositoryRoot = (Resolve-Path -LiteralPath $RepositoryRoot).Path
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
$expectedThumbprint = ($env:WINDOWS_SIGNING_CERT_THUMBPRINT -replace '\s', '').ToUpperInvariant()
if ($expectedThumbprint -notmatch '^[0-9A-F]{40}$') {
    throw "WINDOWS_SIGNING_CERT_THUMBPRINT debe tener 40 hexadecimales."
}

try {
    $pfxBytes = [Convert]::FromBase64String($env:WINDOWS_SIGNING_PFX_BASE64)
} catch {
    throw "WINDOWS_SIGNING_PFX_BASE64 no contiene Base64 valido."
}

$certificateCollection = [System.Security.Cryptography.X509Certificates.X509Certificate2Collection]::new()
$signingTempDirectory = $null
$securePfxPassword = $null
$storeBaselineThumbprints = @()
$pfxThumbprints = @()
$cleanupErrors = @()
try {
    $storageFlags = [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::EphemeralKeySet
    $certificateCollection.Import($pfxBytes, $env:WINDOWS_SIGNING_PFX_PASSWORD, $storageFlags)
    $codeSigningOid = '1.3.6.1.5.5.7.3.3'
    $candidates = @(
        $certificateCollection | Where-Object {
            $_.HasPrivateKey -and @(
                $_.Extensions |
                    Where-Object { $_ -is [System.Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension] } |
                    ForEach-Object { $_.EnhancedKeyUsages } |
                    Where-Object { $_.Value -eq $codeSigningOid }
            ).Count -gt 0
        }
    )
    if ($candidates.Count -ne 1) {
        throw "El PFX debe contener exactamente una identidad privada de firma de codigo; encontradas: $($candidates.Count)"
    }
    $certificate = $candidates[0]

    if ($certificate.Thumbprint.ToUpperInvariant() -ne $expectedThumbprint) {
        throw "El certificado PFX no coincide con WINDOWS_SIGNING_CERT_THUMBPRINT."
    }
    $now = [DateTime]::UtcNow
    if ($certificate.NotBefore.ToUniversalTime() -gt $now -or $certificate.NotAfter.ToUniversalTime() -le $now) {
        throw "El certificado Authenticode no esta vigente."
    }

    $timestampServer = if ([string]::IsNullOrWhiteSpace($env:WINDOWS_TIMESTAMP_URL)) {
        'http://timestamp.digicert.com'
    } else {
        $env:WINDOWS_TIMESTAMP_URL
    }
    $timestampServer = Get-NormalizedRfc3161TimestampUrl `
        -TimestampUrl $timestampServer
    $signToolPath = Resolve-WindowsSignTool

    $versionPath = Join-Path $RepositoryRoot 'VERSION.txt'
    if (-not (Test-Path -LiteralPath $versionPath -PathType Leaf)) {
        throw "Falta VERSION.txt"
    }
    $version = (Get-Content -LiteralPath $versionPath -Raw).Trim()
    $architecture = 'amd64'
    $makensis = Get-Command makensis -ErrorAction Stop

    $signingTempDirectory = Join-Path `
        ([System.IO.Path]::GetTempPath()) `
        ("grxfirma-authenticode-" + [guid]::NewGuid().ToString('N'))
    New-Item `
        -ItemType Directory `
        -Path $signingTempDirectory `
        -ErrorAction Stop |
        Out-Null
    $temporaryAcl = Get-Acl -LiteralPath $signingTempDirectory
    $temporaryAcl.SetAccessRuleProtection($true, $false)
    foreach ($accessRule in @($temporaryAcl.Access)) {
        [void]$temporaryAcl.RemoveAccessRule($accessRule)
    }
    $currentIdentity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    $temporaryRule = [System.Security.AccessControl.FileSystemAccessRule]::new(
        $currentIdentity.User,
        [System.Security.AccessControl.FileSystemRights]::FullControl,
        [System.Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit',
        [System.Security.AccessControl.PropagationFlags]::None,
        [System.Security.AccessControl.AccessControlType]::Allow
    )
    $temporaryAcl.AddAccessRule($temporaryRule)
    Set-Acl -LiteralPath $signingTempDirectory -AclObject $temporaryAcl

    $storeBaselineThumbprints = @(
        Get-ChildItem -Path Cert:\CurrentUser\My |
            ForEach-Object { $_.Thumbprint.ToUpperInvariant() }
    )
    if ($storeBaselineThumbprints -contains $expectedThumbprint) {
        throw "El almacén CurrentUser\\My ya contiene la identidad Authenticode fijada; se aborta para no reutilizar estado ajeno."
    }
    $pfxThumbprints = @(
        $certificateCollection |
            ForEach-Object { $_.Thumbprint.ToUpperInvariant() } |
            Select-Object -Unique
    )
    $temporaryPfx = Join-Path $signingTempDirectory 'identity.pfx'
    [System.IO.File]::WriteAllBytes($temporaryPfx, $pfxBytes)
    $securePfxPassword = ConvertTo-SecureString `
        -String $env:WINDOWS_SIGNING_PFX_PASSWORD `
        -AsPlainText `
        -Force
    try {
        $importedCertificates = @(
            Import-PfxCertificate `
                -FilePath $temporaryPfx `
                -CertStoreLocation Cert:\CurrentUser\My `
                -Password $securePfxPassword `
                -ErrorAction Stop
        )
    } finally {
        Remove-Item `
            -LiteralPath $temporaryPfx `
            -Force `
            -ErrorAction SilentlyContinue
    }
    $storedIdentity = Get-Item `
        -LiteralPath "Cert:\CurrentUser\My\$expectedThumbprint" `
        -ErrorAction Stop
    if (-not $storedIdentity.HasPrivateKey -or
        @($importedCertificates | Where-Object {
            $_.Thumbprint.ToUpperInvariant() -eq $expectedThumbprint
        }).Count -ne 1) {
        throw "La identidad Authenticode temporal no se importo de forma inequivoca."
    }
    Remove-Item Env:WINDOWS_SIGNING_PFX_BASE64 -ErrorAction SilentlyContinue
    Remove-Item Env:WINDOWS_SIGNING_PFX_PASSWORD -ErrorAction SilentlyContinue
    [Array]::Clear($pfxBytes, 0, $pfxBytes.Length)
    $securePfxPassword.Dispose()
    $securePfxPassword = $null

    $signingCommandScript = Join-Path $PSScriptRoot 'sign-windows-file.ps1'
    $currentPowerShell = (Get-Process -Id $PID).Path
    foreach ($commandPath in @(
        $signingCommandScript,
        $currentPowerShell,
        $signingTempDirectory
    )) {
        if ($commandPath -match '["%\r\n]') {
            throw "Ruta no representable de forma segura en el hook NSIS: $commandPath"
        }
    }

    $version = (Get-Content -LiteralPath (Join-Path $RepositoryRoot "VERSION.txt") -Raw).Trim()
    $builds = @(
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

    if (Test-Path -LiteralPath $OutputDirectory) {
        Remove-Item -LiteralPath $OutputDirectory -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null

    foreach ($build in $builds) {
        if (-not (Test-Path -LiteralPath $build.Stage -PathType Container)) {
            throw "Falta la etapa Windows que debe firmarse: $($build.Stage)"
        }
        $portableExecutables = @(
            Get-ChildItem -LiteralPath $build.Stage -Recurse -File |
                Where-Object { $_.Extension -in @('.exe', '.dll') }
        )
        $isSuite = $build.Kind -eq "suite"
        $nsisComponentDefines = if ($isSuite) {
            @(
                Get-WindowsSuiteComponentDefines `
                    -StageDirectory $build.Stage `
                    -DefinePrefix "/D"
            )
        } else {
            @()
        }
        if ($isSuite -and (
            $nsisComponentDefines -notcontains "/DHAS_WINUI=1" -or
            $nsisComponentDefines -notcontains "/DHAS_QT=1"
        )) {
            throw "La Suite Windows oficial debe contener WinUI y Qt completos."
        }
        $signingTargets = @(
            Get-WindowsReleaseSigningTargets `
                -StageDirectory $build.Stage `
                -Suite:$isSuite
        )
        $projectExecutables = @(
            $signingTargets |
                Where-Object { $_.Name -like 'grxfirma*.exe' }
        )
        if ($projectExecutables.Count -eq 0) {
            throw "No hay ejecutables propios que firmar en $($build.Stage)"
        }
        if (@($portableExecutables | Where-Object { $_.Extension -eq '.dll' }).Count -eq 0) {
            throw "La etapa Qt no contiene bibliotecas DLL que verificar y firmar: $($build.Stage)"
        }
        foreach ($portableExecutable in $signingTargets) {
            if ($portableExecutable.Name -like 'grxfirma*.exe') {
                Invoke-WindowsAuthenticodeSign `
                    -Path $portableExecutable.FullName `
                    -SignToolPath $signToolPath `
                    -ExpectedThumbprint $expectedThumbprint `
                    -TimestampUrl $timestampServer |
                    Out-Null
                continue
            }

            $vendorSignature = Get-AuthenticodeSignature -FilePath $portableExecutable.FullName
            if ($vendorSignature.Status -eq [System.Management.Automation.SignatureStatus]::NotSigned) {
                Invoke-WindowsAuthenticodeSign `
                    -Path $portableExecutable.FullName `
                    -SignToolPath $signToolPath `
                    -ExpectedThumbprint $expectedThumbprint `
                    -TimestampUrl $timestampServer |
                    Out-Null
            } elseif ($vendorSignature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
                throw "Firma de proveedor no valida en $($portableExecutable.FullName): $($vendorSignature.Status)"
            } elseif ($null -eq $vendorSignature.TimeStamperCertificate) {
                throw "El binario de proveedor carece de timestamp Authenticode: $($portableExecutable.FullName)"
            }
        }
        if ($isSuite) {
            # El launcher se firma una sola vez. Las copias que necesitan los
            # payloads para validar la instalación reciben después esos bytes
            # exactos y el inventario WinUI se regenera tras toda firma.
            Sync-WindowsSuiteSharedBackend `
                -StageDirectory $build.Stage
        }

        Remove-Item -LiteralPath $build.Zip -Force -ErrorAction SilentlyContinue
        Compress-Archive -Path $build.Stage -DestinationPath $build.Zip -CompressionLevel Optimal

        Remove-Item -LiteralPath $build.Setup -Force -ErrorAction SilentlyContinue
        $buildToken = [System.IO.Path]::GetFileNameWithoutExtension(
            [string]$build.Setup
        )
        $uninstallerEvidence = Join-Path `
            $signingTempDirectory `
            "$buildToken-uninstall.exe"
        $signingConfiguration = Join-Path `
            $signingTempDirectory `
            "$buildToken-signing.json"
        $uninstallerSigner = Join-Path `
            $signingTempDirectory `
            "$buildToken-sign-uninstaller.cmd"
        $signingDocument = [ordered]@{
            schema_version = 1
            sign_tool_path = $signToolPath
            expected_thumbprint = $expectedThumbprint
            timestamp_url = $timestampServer
            evidence_path = $uninstallerEvidence
        } | ConvertTo-Json -Depth 4
        [System.IO.File]::WriteAllText(
            $signingConfiguration,
            $signingDocument + [Environment]::NewLine,
            [System.Text.UTF8Encoding]::new($false)
        )
        $uninstallerSignerContent = @"
@echo off
"$currentPowerShell" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$signingCommandScript" -ConfigurationPath "$signingConfiguration" -FilePath "%~1"
exit /b %ERRORLEVEL%
"@
        Set-Content `
            -LiteralPath $uninstallerSigner `
            -Value $uninstallerSignerContent `
            -Encoding ASCII

        & $makensis.Source `
            "/DVERSION=$version" `
            "/DARCH=$architecture" `
            "/DSTAGE_DIR=$($build.Stage)" `
            "/DOUT_FILE=$($build.Setup)" `
            "/DGRXFIRMA_UNINSTALL_SIGNER=$uninstallerSigner" `
            @nsisComponentDefines `
            $build.Nsi
        if ($LASTEXITCODE -ne 0) {
            throw "makensis fallo con codigo $LASTEXITCODE para $($build.Nsi)"
        }
        if (-not (Test-Path -LiteralPath $uninstallerEvidence -PathType Leaf)) {
            throw "NSIS no produjo evidencia del uninstaller firmado para $($build.Nsi)"
        }
        Assert-WindowsAuthenticodeFile `
            -Path $uninstallerEvidence `
            -SignToolPath $signToolPath `
            -ExpectedThumbprint $expectedThumbprint `
            -RequireSha256Rfc3161 |
            Out-Null
        Invoke-WindowsAuthenticodeSign `
            -Path $build.Setup `
            -SignToolPath $signToolPath `
            -ExpectedThumbprint $expectedThumbprint `
            -TimestampUrl $timestampServer |
            Out-Null

        Copy-Item -LiteralPath $build.Zip -Destination $OutputDirectory -Force
        Copy-Item -LiteralPath $build.Setup -Destination $OutputDirectory -Force
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
} finally {
    if ($pfxThumbprints.Count -gt 0) {
        foreach ($thumbprint in $pfxThumbprints) {
            if ($storeBaselineThumbprints -contains $thumbprint) {
                continue
            }
            $certificatePath = "Cert:\CurrentUser\My\$thumbprint"
            if (-not (Test-Path -LiteralPath $certificatePath)) {
                continue
            }
            $storedCertificate = Get-Item `
                -LiteralPath $certificatePath `
                -ErrorAction SilentlyContinue
            if ($null -ne $storedCertificate -and
                $storedCertificate.HasPrivateKey) {
                try {
                    Remove-Item `
                        -LiteralPath $certificatePath `
                        -DeleteKey `
                        -Force `
                        -ErrorAction Stop
                } catch {
                    $cleanupErrors += $_.Exception.Message
                }
            } else {
                try {
                    Remove-Item `
                        -LiteralPath $certificatePath `
                        -Force `
                        -ErrorAction Stop
                } catch {
                    $cleanupErrors += $_.Exception.Message
                }
            }
            if (Test-Path -LiteralPath $certificatePath) {
                $cleanupErrors +=
                    "No se elimino el certificado temporal $thumbprint"
            }
        }
    }
    if ($null -ne $securePfxPassword) {
        $securePfxPassword.Dispose()
    }
    if (-not [string]::IsNullOrWhiteSpace($signingTempDirectory) -and
        (Test-Path -LiteralPath $signingTempDirectory)) {
        try {
            Remove-Item `
                -LiteralPath $signingTempDirectory `
                -Recurse `
                -Force `
                -ErrorAction Stop
        } catch {
            $cleanupErrors += $_.Exception.Message
        }
    }
    foreach ($item in $certificateCollection) {
        $item.Dispose()
    }
    [Array]::Clear($pfxBytes, 0, $pfxBytes.Length)
    if ($cleanupErrors.Count -gt 0) {
        throw "La limpieza de credenciales Authenticode temporales fallo: $($cleanupErrors -join '; ')"
    }
}

Write-Output "Artefactos Windows reconstruidos, firmados y verificados en $OutputDirectory"
