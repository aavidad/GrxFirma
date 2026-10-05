# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\NativeHost"
)

$ErrorActionPreference = "Stop"

$baseDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$pathSafety = Join-Path $baseDir "install-path-safety.ps1"
if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
    throw "No se encuentra install-path-safety.ps1 junto al desinstalador."
}
. $pathSafety

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "NativeHost"

function Remove-RegistryValueIfOwned {
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [AllowEmptyString()]
        [string]$ValueName,
        [Parameter(Mandatory = $true)]
        [string]$ExpectedValue,
        [switch]$PathValue
    )

    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($Path, $true)
    if ($null -eq $key) {
        return
    }
    try {
        if (@($key.GetValueNames()) -notcontains $ValueName) {
            return
        }
        $actual = $key.GetValue(
            $ValueName,
            $null,
            [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
        )
        $actualText = [string]$actual
        $matches = if ($PathValue) {
            try {
                [string]::Equals(
                    [System.IO.Path]::GetFullPath($actualText),
                    [System.IO.Path]::GetFullPath($ExpectedValue),
                    [System.StringComparison]::OrdinalIgnoreCase
                )
            } catch {
                $false
            }
        } else {
            [string]::Equals(
                $actualText,
                $ExpectedValue,
                [System.StringComparison]::Ordinal
            )
        }
        if (-not $matches) {
            Write-Warning "Se conserva HKCU:\$Path porque ya no pertenece a esta instalacion."
            return
        }
        if ($PSCmdlet.ShouldProcess("HKCU:\$Path", "Eliminar valor de registro propio")) {
            $key.DeleteValue($ValueName, $false)
        }
    } finally {
        $key.Dispose()
    }
}

function Get-FirefoxProfileDirectory {
    $profileRoot = Join-Path $env:APPDATA "Mozilla\Firefox\Profiles"
    if (-not (Test-Path $profileRoot)) {
        return @()
    }

    return @(Get-ChildItem -Path $profileRoot -Directory -ErrorAction SilentlyContinue | Where-Object {
        Test-Path (Join-Path $_.FullName "prefs.js")
    })
}

$nativeHosts = @(
    "com.grxfirma.native",
    "io.github.aavidad.grxfirma",
    # Nombres de versiones anteriores y del host de la extension portafirmas, que
    # ya no forma parte de GrxFirma; se retiran solo si apuntan a esta instalacion.
    "io.github.aavidad.portafirmas",
    "com.dipgra.grxfirma",
    "com.dipgra.portafirmas"
)

foreach ($nativeHost in $nativeHosts) {
    foreach ($target in @(
        @{ BaseKey = "Software\Google\Chrome\NativeMessagingHosts"; Manifest = "$nativeHost.chrome.json" },
        @{ BaseKey = "Software\Chromium\NativeMessagingHosts"; Manifest = "$nativeHost.chrome.json" },
        @{ BaseKey = "Software\Microsoft\Edge\NativeMessagingHosts"; Manifest = "$nativeHost.chrome.json" },
        @{ BaseKey = "Software\BraveSoftware\Brave-Browser\NativeMessagingHosts"; Manifest = "$nativeHost.chrome.json" },
        @{ BaseKey = "Software\Vivaldi\NativeMessagingHosts"; Manifest = "$nativeHost.chrome.json" },
        @{ BaseKey = "Software\Opera Software\NativeMessagingHosts"; Manifest = "$nativeHost.chrome.json" },
        @{ BaseKey = "Software\Mozilla\NativeMessagingHosts"; Manifest = "$nativeHost.firefox.json" }
    )) {
        Remove-RegistryValueIfOwned `
            -Path "$($target.BaseKey)\$nativeHost" `
            -ValueName "" `
            -ExpectedValue (Join-Path $InstallDir "manifests\$($target.Manifest)") `
            -PathValue
    }
}

$allowedExtensionRegistryRoots = @(
    "Software\Google\Chrome\Extensions",
    "Software\Chromium\Extensions",
    "Software\Microsoft\Edge\Extensions",
    "Software\BraveSoftware\Brave-Browser\Extensions",
    "Software\Vivaldi\Extensions",
    "Software\Opera Software\Extensions"
)
$registrationsPath = Join-Path $InstallDir "extensions\chromium\registrations.json"
if (Test-Path -LiteralPath $registrationsPath -PathType Leaf) {
    try {
        $registrationDocument = Get-Content -LiteralPath $registrationsPath -Raw | ConvertFrom-Json
        if ($registrationDocument.schema_version -ne 1) {
            throw "version de esquema inesperada"
        }
        foreach ($registration in @($registrationDocument.registrations)) {
            $extensionId = [string]$registration.extension_id
            $registryPath = [string]$registration.registry_path
            $updateUrl = [string]$registration.update_url
            $matchingRoot = @($allowedExtensionRegistryRoots | Where-Object {
                [string]::Equals(
                    $registryPath,
                    "$_\$extensionId",
                    [System.StringComparison]::OrdinalIgnoreCase
                )
            })
            $parsedUrl = $null
            if ($extensionId -notmatch '^[a-p]{32}$' -or
                $matchingRoot.Count -ne 1 -or
                (-not [Uri]::TryCreate($updateUrl, [UriKind]::Absolute, [ref]$parsedUrl)) -or
                $parsedUrl.Scheme -ne "https") {
                throw "registro Chromium no valido"
            }
            Remove-RegistryValueIfOwned `
                -Path $registryPath `
                -ValueName "update_url" `
                -ExpectedValue $parsedUrl.AbsoluteUri
        }
    } catch {
        Write-Warning "No se limpiaran registros de extension Chromium: $($_.Exception.Message)"
    }
}

$packagedFirefoxHashes = @()
foreach ($packagedFirefoxName in @("grxfirma-extension-firefox.xpi", "dipgra-extension-firefox.xpi")) {
    $packagedFirefoxXpi = Join-Path $InstallDir "extensions\firefox\$packagedFirefoxName"
    if (Test-Path -LiteralPath $packagedFirefoxXpi -PathType Leaf) {
        $packagedFirefoxHashes += (Get-FileHash -LiteralPath $packagedFirefoxXpi -Algorithm SHA256).Hash
    }
}

function Remove-FirefoxXpiIfOwned {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )
    if (@($packagedFirefoxHashes).Count -eq 0 -or
        (-not (Test-Path -LiteralPath $Path -PathType Leaf))) {
        return
    }
    $actualHash = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
    $owned = @($packagedFirefoxHashes | Where-Object {
        $actualHash.Equals([string]$_, [System.StringComparison]::OrdinalIgnoreCase)
    }).Count -gt 0
    if (-not $owned) {
        Write-Warning "Se conserva la extension Firefox porque fue actualizada o reemplazada: $Path"
        return
    }
    Remove-Item -LiteralPath $Path -Force
}

$firefoxXpiNames = @("grxfirma@aavidad.github.io.xpi", "extension@dipgra.es.xpi")
foreach ($profileDir in Get-FirefoxProfileDirectory) {
    foreach ($firefoxXpiName in $firefoxXpiNames) {
        $xpiPath = Join-Path $profileDir.FullName "extensions\$firefoxXpiName"
        Remove-FirefoxXpiIfOwned -Path $xpiPath
    }
}

foreach ($firefoxDir in @($env:ProgramFiles, ${env:ProgramFiles(x86)})) {
    if ([string]::IsNullOrWhiteSpace($firefoxDir)) {
        continue
    }
    foreach ($firefoxXpiName in $firefoxXpiNames) {
        $distributionXpi = Join-Path $firefoxDir "Mozilla Firefox\distribution\extensions\$firefoxXpiName"
        try {
            Remove-FirefoxXpiIfOwned -Path $distributionXpi
        } catch {
            Write-Warning "No se pudo eliminar la extension global de Firefox '$distributionXpi': $($_.Exception.Message)"
        }
    }
}

Remove-GrxFirmaInstallDirectory `
    -Path $InstallDir `
    -Component "NativeHost" `
    -LegacyPayload "grxfirma-nativehost.exe"

Write-Output "NativeHost desinstalado de: $InstallDir"
