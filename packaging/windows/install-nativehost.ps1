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
    throw "No se encuentra install-path-safety.ps1 junto al instalador."
}
. $pathSafety

$exeSource = Join-Path $baseDir "grxfirma-nativehost.exe"

function Get-FirefoxProfileDirectory {
    $profileRoot = Join-Path $env:APPDATA "Mozilla\Firefox\Profiles"
    if (-not (Test-Path $profileRoot)) {
        return @()
    }

    return @(Get-ChildItem -Path $profileRoot -Directory -ErrorAction SilentlyContinue | Where-Object {
        Test-Path (Join-Path $_.FullName "prefs.js")
    })
}

function Install-FirefoxExtension {
    param(
        [string]$XpiSource
    )

    if (-not (Test-Path $XpiSource)) {
        return
    }

    $xpiName = "grxfirma@aavidad.github.io.xpi"
    $profileTargets = @()
    foreach ($profileDir in Get-FirefoxProfileDirectory) {
        $profileExtensionDir = Join-Path $profileDir.FullName "extensions"
        New-Item -ItemType Directory -Force -Path $profileExtensionDir | Out-Null
        $profileTargets += $profileExtensionDir
    }

    foreach ($targetDir in $profileTargets) {
        try {
            Copy-Item $XpiSource (Join-Path $targetDir $xpiName) -Force
        } catch {
            Write-Warning "No se pudo instalar la extension de Firefox en '$targetDir': $($_.Exception.Message)"
        }
    }

    $distributionTargets = @()
    foreach ($firefoxDir in @($env:ProgramFiles, ${env:ProgramFiles(x86)})) {
        if ([string]::IsNullOrWhiteSpace($firefoxDir)) {
            continue
        }
        $distributionDir = Join-Path $firefoxDir "Mozilla Firefox\distribution\extensions"
        $distributionTargets += $distributionDir
    }

    foreach ($targetDir in $distributionTargets | Select-Object -Unique) {
        try {
            New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
            Copy-Item $XpiSource (Join-Path $targetDir $xpiName) -Force
        } catch {
            Write-Warning "No se pudo instalar la extension de Firefox en '$targetDir': $($_.Exception.Message)"
        }
    }

    if ($profileTargets.Count -gt 0) {
        Write-Output "Extension Firefox desplegada en perfiles de usuario: $($profileTargets.Count)"
    } else {
        Write-Output "No se encontraron perfiles Firefox del usuario. La extension se deja empaquetada para despliegue posterior."
    }
}

function Test-FirefoxXpiApproved {
    param(
        [string]$XpiPath,
        [string]$MetadataPath
    )

    if ((-not (Test-Path $XpiPath)) -or (-not (Test-Path $MetadataPath))) {
        return $false
    }

    try {
        $metadata = Get-Content -Path $MetadataPath -Raw | ConvertFrom-Json
        if ($metadata.signed -ne $true -or $metadata.extension_id -ne "grxfirma@aavidad.github.io") {
            return $false
        }
        $expectedHash = [string]$metadata.xpi_sha256
        if ($expectedHash -notmatch '^[0-9a-fA-F]{64}$') {
            return $false
        }
        $actualHash = (Get-FileHash -Path $XpiPath -Algorithm SHA256).Hash
        return $actualHash.Equals($expectedHash, [System.StringComparison]::OrdinalIgnoreCase)
    } catch {
        Write-Warning "No se pudo validar el metadato del XPI Firefox: $($_.Exception.Message)"
        return $false
    }
}

function Remove-LegacyNativeHostRegistration {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$ExpectedValue
    )

    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($Path, $true)
    if ($null -eq $key) {
        return $false
    }
    $deleteKey = $false
    try {
        if (@($key.GetValueNames()) -notcontains "") {
            return $false
        }
        $actual = [string]$key.GetValue(
            "",
            $null,
            [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
        )
        $owned = $false
        try {
            $owned = [string]::Equals(
                [System.IO.Path]::GetFullPath($actual),
                [System.IO.Path]::GetFullPath($ExpectedValue),
                [System.StringComparison]::OrdinalIgnoreCase
            )
        } catch {
            $owned = $false
        }
        if (-not $owned) {
            Write-Warning "Se conserva HKCU:\$Path porque no apunta a esta instalacion."
            return $false
        }
        $key.DeleteValue("", $false)
        $deleteKey = ($key.ValueCount -eq 0) -and ($key.SubKeyCount -eq 0)
    } finally {
        $key.Dispose()
    }
    if ($deleteKey) {
        [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKey($Path, $false)
    }
    return $true
}

function Remove-LegacyNativeHostInstallation {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ManifestsDir,
        [Parameter(Mandatory = $true)]
        [string]$ExtensionsDir
    )

    # Las versiones anteriores usaban para los hosts y la extension el espacio de nombres
    # com.dipgra y el ID extension@dipgra.es, y algunas registraban tambien el
    # host de la extension portafirmas, que no forma parte de GrxFirma. Solo se
    # retira lo que apunta a esta instalacion para no dejar hosts huerfanos ni
    # tocar registros ajenos.
    foreach ($legacyHost in @("com.dipgra.grxfirma", "com.dipgra.portafirmas", "io.github.aavidad.portafirmas")) {
        foreach ($target in @(
            @{ BaseKey = "Software\Google\Chrome\NativeMessagingHosts"; Manifest = "$legacyHost.chrome.json" },
            @{ BaseKey = "Software\Chromium\NativeMessagingHosts"; Manifest = "$legacyHost.chrome.json" },
            @{ BaseKey = "Software\Microsoft\Edge\NativeMessagingHosts"; Manifest = "$legacyHost.chrome.json" },
            @{ BaseKey = "Software\BraveSoftware\Brave-Browser\NativeMessagingHosts"; Manifest = "$legacyHost.chrome.json" },
            @{ BaseKey = "Software\Vivaldi\NativeMessagingHosts"; Manifest = "$legacyHost.chrome.json" },
            @{ BaseKey = "Software\Opera Software\NativeMessagingHosts"; Manifest = "$legacyHost.chrome.json" },
            @{ BaseKey = "Software\Mozilla\NativeMessagingHosts"; Manifest = "$legacyHost.firefox.json" }
        )) {
            try {
                Remove-LegacyNativeHostRegistration `
                    -Path "$($target.BaseKey)\$legacyHost" `
                    -ExpectedValue (Join-Path $ManifestsDir $target.Manifest) | Out-Null
            } catch {
                Write-Warning "No se pudo retirar el host anterior HKCU:\$($target.BaseKey)\$($legacyHost): $($_.Exception.Message)"
            }
        }
        foreach ($suffix in @("chrome.json", "firefox.json")) {
            $legacyManifest = Join-Path $ManifestsDir "$legacyHost.$suffix"
            if (Test-Path -LiteralPath $legacyManifest -PathType Leaf) {
                Remove-Item -LiteralPath $legacyManifest -Force -ErrorAction SilentlyContinue
            }
        }
    }

    $legacyPackagedXpi = Join-Path $ExtensionsDir "firefox\dipgra-extension-firefox.xpi"
    if (Test-Path -LiteralPath $legacyPackagedXpi -PathType Leaf) {
        $legacyHash = (Get-FileHash -LiteralPath $legacyPackagedXpi -Algorithm SHA256).Hash
        $legacyTargets = @()
        foreach ($profileDir in Get-FirefoxProfileDirectory) {
            $legacyTargets += Join-Path $profileDir.FullName "extensions\extension@dipgra.es.xpi"
        }
        foreach ($firefoxDir in @($env:ProgramFiles, ${env:ProgramFiles(x86)})) {
            if (-not [string]::IsNullOrWhiteSpace($firefoxDir)) {
                $legacyTargets += Join-Path $firefoxDir "Mozilla Firefox\distribution\extensions\extension@dipgra.es.xpi"
            }
        }
        foreach ($legacyTarget in $legacyTargets | Select-Object -Unique) {
            if (-not (Test-Path -LiteralPath $legacyTarget -PathType Leaf)) {
                continue
            }
            try {
                $actualHash = (Get-FileHash -LiteralPath $legacyTarget -Algorithm SHA256).Hash
                if ($actualHash.Equals($legacyHash, [System.StringComparison]::OrdinalIgnoreCase)) {
                    Remove-Item -LiteralPath $legacyTarget -Force
                } else {
                    Write-Warning "Se conserva la extension Firefox anterior porque fue actualizada o reemplazada: $legacyTarget"
                }
            } catch {
                Write-Warning "No se pudo retirar la extension Firefox anterior '$legacyTarget': $($_.Exception.Message)"
            }
        }
    }

    foreach ($legacyFile in @(
        "firefox\dipgra-extension-firefox.xpi",
        "firefox\dipgra-extension-firefox.metadata.json",
        "chromium\dipgra-extension-chromium.zip"
    )) {
        $legacyPath = Join-Path $ExtensionsDir $legacyFile
        if (Test-Path -LiteralPath $legacyPath -PathType Leaf) {
            Remove-Item -LiteralPath $legacyPath -Force -ErrorAction SilentlyContinue
        }
    }
}

function Write-Utf8NoBom {
    param(
        [string]$Path,
        [string]$Value
    )

    [System.IO.File]::WriteAllText(
        $Path,
        $Value + [Environment]::NewLine,
        [System.Text.UTF8Encoding]::new($false)
    )
}

function Set-RegistroPredeterminado {
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [string]$Ruta,
        [string]$Valor
    )

    if (-not $PSCmdlet.ShouldProcess("HKCU:\\$Ruta", "Establecer valor predeterminado")) {
        return
    }
    $clave = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($Ruta)
    if ($null -eq $clave) {
        throw "No se pudo crear la clave de registro HKCU:\\$Ruta"
    }
    try {
        $clave.SetValue("", $Valor, [Microsoft.Win32.RegistryValueKind]::String)
    } finally {
        $clave.Dispose()
    }
}

function Set-RegistroCadena {
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [string]$Ruta,
        [string]$Nombre,
        [string]$Valor
    )

    if (-not $PSCmdlet.ShouldProcess("HKCU:\\$Ruta", "Establecer valor $Nombre")) {
        return
    }
    $clave = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($Ruta)
    if ($null -eq $clave) {
        throw "No se pudo crear la clave de registro HKCU:\\$Ruta"
    }
    try {
        $clave.SetValue($Nombre, $Valor, [Microsoft.Win32.RegistryValueKind]::String)
    } finally {
        $clave.Dispose()
    }
}

function Get-RegistroCadenaSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Ruta,
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$Nombre
    )

    $clave = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($Ruta, $false)
    if ($null -eq $clave) {
        return [pscustomobject]@{
            Path = $Ruta
            Name = $Nombre
            KeyExisted = $false
            ValueExisted = $false
            Value = $null
            Kind = $null
        }
    }
    try {
        $valueExisted = @($clave.GetValueNames()) -contains $Nombre
        return [pscustomobject]@{
            Path = $Ruta
            Name = $Nombre
            KeyExisted = $true
            ValueExisted = $valueExisted
            Value = if ($valueExisted) {
                $clave.GetValue(
                    $Nombre,
                    $null,
                    [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
                )
            } else {
                $null
            }
            Kind = if ($valueExisted) {
                $clave.GetValueKind($Nombre)
            } else {
                $null
            }
        }
    } finally {
        $clave.Dispose()
    }
}

function Restore-RegistroCadenaSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Snapshot,
        [Parameter(Mandatory = $true)]
        [string]$ExpectedCurrentValue
    )

    $clave = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey([string]$Snapshot.Path, $true)
    if ($null -eq $clave) {
        if ($Snapshot.KeyExisted -and $Snapshot.ValueExisted) {
            Set-RegistroCadena `
                ([string]$Snapshot.Path) `
                ([string]$Snapshot.Name) `
                ([string]$Snapshot.Value)
        }
        return
    }

    $deleteEmptyKey = $false
    try {
        $current = $clave.GetValue(
            [string]$Snapshot.Name,
            $null,
            [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
        )
        $currentValueExisted = @($clave.GetValueNames()) -contains [string]$Snapshot.Name
        $alreadyRestored =
            ($Snapshot.ValueExisted -and $currentValueExisted -and
                [string]::Equals(
                    [string]$current,
                    [string]$Snapshot.Value,
                    [System.StringComparison]::Ordinal
                )) -or
            ($Snapshot.KeyExisted -and
                (-not $Snapshot.ValueExisted) -and
                $null -eq $current)
        if ($alreadyRestored) {
            return
        }
        if ($null -ne $current -and
            (-not [string]::Equals(
                [string]$current,
                $ExpectedCurrentValue,
                [System.StringComparison]::Ordinal
            ))) {
            throw "el registro cambio mientras se revertia: HKCU:\$($Snapshot.Path)"
        }

        if ($Snapshot.ValueExisted) {
            $clave.SetValue(
                [string]$Snapshot.Name,
                $Snapshot.Value,
                [Microsoft.Win32.RegistryValueKind]$Snapshot.Kind
            )
        } else {
            $clave.DeleteValue([string]$Snapshot.Name, $false)
            $deleteEmptyKey =
                (-not $Snapshot.KeyExisted) -and
                $clave.GetValueNames().Count -eq 0 -and
                $clave.GetSubKeyNames().Count -eq 0
        }
    } finally {
        $clave.Dispose()
    }
    if ($deleteEmptyKey) {
        [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKey(
            [string]$Snapshot.Path,
            $false
        )
    }
}

function Register-ChromiumUpdateUrlExtension {
    param(
        [string]$ExtensionId,
        [string]$ExistingEdgeExtensionId,
        [string]$RegistrationInventoryPath
    )

    $defaultExtensionId = $env:GRXFIRMA_CHROMIUM_EXTENSION_ID
    if ([string]::IsNullOrWhiteSpace($defaultExtensionId)) {
        $defaultExtensionId = $ExtensionId
    }
    $edgeExtensionId = $env:GRXFIRMA_EDGE_EXTENSION_ID
    if ([string]::IsNullOrWhiteSpace($edgeExtensionId)) {
        $edgeExtensionId = $ExistingEdgeExtensionId
    }
    $defaultUpdateUrl = "https://clients2.google.com/service/update2/crx"
    $edgeUpdateUrl = "https://edge.microsoft.com/extensionwebstorebase/v1/crx"
    if (-not [string]::IsNullOrWhiteSpace($env:GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL) -and
        $env:GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL -ne $defaultUpdateUrl) {
        throw "La URL de Chrome debe ser la de Chrome Web Store: $defaultUpdateUrl"
    }
    if (-not [string]::IsNullOrWhiteSpace($env:GRXFIRMA_EDGE_EXTENSION_UPDATE_URL) -and
        $env:GRXFIRMA_EDGE_EXTENSION_UPDATE_URL -ne $edgeUpdateUrl) {
        throw "La URL de Edge debe ser la de Microsoft Edge Add-ons: $edgeUpdateUrl"
    }

    $registryTargets = @(
        @{ Name = "Chrome"; BasePath = "Software\Google\Chrome\Extensions"; ExtensionId = $defaultExtensionId; UpdateUrl = $defaultUpdateUrl },
        @{ Name = "Chromium"; BasePath = "Software\Chromium\Extensions"; ExtensionId = $defaultExtensionId; UpdateUrl = $defaultUpdateUrl },
        @{ Name = "Edge"; BasePath = "Software\Microsoft\Edge\Extensions"; ExtensionId = $edgeExtensionId; UpdateUrl = $edgeUpdateUrl },
        @{ Name = "Brave"; BasePath = "Software\BraveSoftware\Brave-Browser\Extensions"; ExtensionId = $defaultExtensionId; UpdateUrl = $defaultUpdateUrl },
        @{ Name = "Vivaldi"; BasePath = "Software\Vivaldi\Extensions"; ExtensionId = $defaultExtensionId; UpdateUrl = $defaultUpdateUrl },
        @{ Name = "Opera"; BasePath = "Software\Opera Software\Extensions"; ExtensionId = $defaultExtensionId; UpdateUrl = $defaultUpdateUrl }
    )

    $registered = @()
    foreach ($target in $registryTargets) {
        $updateUrl = [string]$target.UpdateUrl
        $targetExtensionId = [string]$target.ExtensionId
        if ([string]::IsNullOrWhiteSpace($targetExtensionId)) {
            continue
        }
        if ($targetExtensionId -notmatch '^[a-p]{32}$') {
            throw "El ID de extension para $($target.Name) no es valido: $targetExtensionId"
        }
        $parsedUrl = $null
        if ((-not [Uri]::TryCreate($updateUrl, [UriKind]::Absolute, [ref]$parsedUrl)) -or $parsedUrl.Scheme -ne "https") {
            throw "La URL de actualizacion para $($target.Name) debe ser HTTPS: $updateUrl"
        }
        $registryPath = "$($target.BasePath)\$targetExtensionId"
        $registered += [ordered]@{
            browser = $target.Name
            extension_id = $targetExtensionId
            registry_path = $registryPath
            update_url = $parsedUrl.AbsoluteUri
        }
    }

    $inventoryExisted = $false
    $inventoryBytes = $null
    if (-not [string]::IsNullOrWhiteSpace($RegistrationInventoryPath)) {
        $inventoryExisted = Test-Path -LiteralPath $RegistrationInventoryPath -PathType Leaf
        if ($inventoryExisted) {
            $inventoryBytes = [System.IO.File]::ReadAllBytes($RegistrationInventoryPath)
        }
    }
    $snapshots = @(
        $registered | ForEach-Object {
            Get-RegistroCadenaSnapshot `
                -Ruta ([string]$_.registry_path) `
                -Nombre "update_url"
        }
    )
    $attempted = 0
    try {
        if (-not [string]::IsNullOrWhiteSpace($RegistrationInventoryPath) -and $registered.Count -gt 0) {
            $registrations = [ordered]@{
                schema_version = 1
                registrations = $registered
            } | ConvertTo-Json -Depth 5
            Write-Utf8NoBom -Path $RegistrationInventoryPath -Value $registrations
        }
        foreach ($registration in $registered) {
            $attempted++
            Set-RegistroCadena `
                ([string]$registration.registry_path) `
                "update_url" `
                ([string]$registration.update_url)
        }
    } catch {
        $registrationError = $_.Exception
        $rollbackErrors = @()
        for ($index = $attempted - 1; $index -ge 0; $index--) {
            try {
                Restore-RegistroCadenaSnapshot `
                    -Snapshot $snapshots[$index] `
                    -ExpectedCurrentValue ([string]$registered[$index].update_url)
            } catch {
                $rollbackErrors += $_.Exception.Message
            }
        }
        try {
            if (-not [string]::IsNullOrWhiteSpace($RegistrationInventoryPath)) {
                if ($inventoryExisted) {
                    [System.IO.File]::WriteAllBytes($RegistrationInventoryPath, $inventoryBytes)
                } else {
                    Remove-Item -LiteralPath $RegistrationInventoryPath -Force -ErrorAction SilentlyContinue
                }
            }
        } catch {
            $rollbackErrors += $_.Exception.Message
        }
        if ($rollbackErrors.Count -gt 0) {
            throw "Fallo registrando Chromium: $($registrationError.Message). Rollback incompleto: $($rollbackErrors -join '; ')"
        }
        throw $registrationError
    }
    return $registered
}

function Get-RegisteredStoreExtensionIds {
    param([string]$RegistrationInventoryPath)

    $ids = @{ Chrome = $null; Edge = $null }
    if (-not (Test-Path -LiteralPath $RegistrationInventoryPath -PathType Leaf)) {
        return $ids
    }
    try {
        $inventory = Get-Content -LiteralPath $RegistrationInventoryPath -Raw | ConvertFrom-Json
        if ($inventory.schema_version -ne 1) {
            return $ids
        }
        foreach ($registration in @($inventory.registrations)) {
            $browser = [string]$registration.browser
            $extensionId = [string]$registration.extension_id
            if ($extensionId -notmatch '^[a-p]{32}$') {
                continue
            }
            if ($browser -eq "Chrome" -and
                $registration.registry_path -eq "Software\Google\Chrome\Extensions\$extensionId" -and
                $registration.update_url -eq "https://clients2.google.com/service/update2/crx") {
                $ids.Chrome = $extensionId
            }
            if ($browser -eq "Edge" -and
                $registration.registry_path -eq "Software\Microsoft\Edge\Extensions\$extensionId" -and
                $registration.update_url -eq "https://edge.microsoft.com/extensionwebstorebase/v1/crx") {
                $ids.Edge = $extensionId
            }
        }
    } catch {
        Write-Warning "No se pudo leer el registro anterior de extensiones de tienda: $($_.Exception.Message)"
    }
    return $ids
}

function Restore-NativeHostInstallSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$InstallDir,
        [Parameter(Mandatory = $true)]
        [string]$BackupPayload,
        [Parameter(Mandatory = $true)]
        [bool]$PreviousInstallExisted
    )

    if (Test-Path -LiteralPath $InstallDir) {
        Assert-NoGrxFirmaReparsePoint -Path $InstallDir
        Remove-Item -LiteralPath $InstallDir -Recurse -Force -ErrorAction Stop
    }
    if ($PreviousInstallExisted) {
        if (-not (Test-Path -LiteralPath $BackupPayload -PathType Container)) {
            throw "La instantanea NativeHost no esta disponible: $BackupPayload"
        }
        Move-Item -LiteralPath $BackupPayload -Destination $InstallDir -ErrorAction Stop
    }
}

if (-not (Test-Path $exeSource)) {
    throw "No se encuentra grxfirma-nativehost.exe junto al instalador."
}

$InstallDir = Resolve-GrxFirmaInstallPath -Path $InstallDir -Component "NativeHost"
$previousInstallExisted = Test-Path -LiteralPath $InstallDir -PathType Container
if ($previousInstallExisted -and
    (-not (Test-GrxFirmaInstallMarker -Path $InstallDir -Component "NativeHost")) -and
    (-not (Test-Path -LiteralPath (Join-Path $InstallDir "grxfirma-nativehost.exe") -PathType Leaf))) {
    throw "La ruta contiene datos que no pertenecen a GrxFirma y no se modificara: $InstallDir"
}

Stop-GrxFirmaInstalledProcesses -Path $InstallDir -Component "NativeHost"

$backupLeaf = "grxfirma-nativehost-backup-$([guid]::NewGuid().ToString('N'))"
$backupRoot = Join-Path ([System.IO.Path]::GetTempPath()) $backupLeaf
$backupParent = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\', '/')
if (-not [string]::Equals(
        [System.IO.Path]::GetFullPath((Split-Path -Parent $backupRoot)).TrimEnd('\', '/'),
        $backupParent,
        [System.StringComparison]::OrdinalIgnoreCase
    ) -or
    (Split-Path -Leaf $backupRoot) -notmatch '^grxfirma-nativehost-backup-[0-9a-f]{32}$') {
    throw "Ruta de instantanea NativeHost inesperada: $backupRoot"
}
New-Item -ItemType Directory -Path $backupRoot -ErrorAction Stop | Out-Null
$backupPayload = Join-Path $backupRoot "payload"
try {
    if ($previousInstallExisted) {
        Copy-Item `
            -LiteralPath $InstallDir `
            -Destination $backupPayload `
            -Recurse `
            -Force `
            -ErrorAction Stop
        Assert-NoGrxFirmaReparsePoint -Path $backupPayload
    }
} catch {
    Remove-Item -LiteralPath $backupRoot -Recurse -Force -ErrorAction SilentlyContinue
    throw
}

$filesystemMutationStarted = $false
$preserveBackupForRecovery = $false
try {
    $filesystemMutationStarted = $true
    $InstallDir = Initialize-GrxFirmaInstallDirectory `
        -Path $InstallDir `
        -Component "NativeHost" `
        -LegacyPayload "grxfirma-nativehost.exe"
$exeTarget = Join-Path $InstallDir "grxfirma-nativehost.exe"
Copy-Item $exeSource $exeTarget -Force

$manifestsDir = Join-Path $InstallDir "manifests"
New-Item -ItemType Directory -Force -Path $manifestsDir | Out-Null
$extensionsDir = Join-Path $InstallDir "extensions"
New-Item -ItemType Directory -Force -Path (Join-Path $extensionsDir "firefox") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $extensionsDir "chromium") | Out-Null

$firefoxXpiSource = Join-Path $baseDir "extensions\\grxfirma-extension-firefox.xpi"
$firefoxMetadataSource = Join-Path $baseDir "extensions\\grxfirma-extension-firefox.metadata.json"
$chromiumZipSource = Join-Path $baseDir "extensions\\grxfirma-extension-chromium.zip"
$registrationsPath = Join-Path $extensionsDir "chromium\registrations.json"
$registeredStoreIds = Get-RegisteredStoreExtensionIds -RegistrationInventoryPath $registrationsPath
$chromiumExtensionId = $env:GRXFIRMA_CHROMIUM_EXTENSION_ID
if ([string]::IsNullOrWhiteSpace($chromiumExtensionId)) {
    $chromiumExtensionId = $registeredStoreIds.Chrome
}
$edgeExtensionId = $env:GRXFIRMA_EDGE_EXTENSION_ID
if ([string]::IsNullOrWhiteSpace($edgeExtensionId)) {
    $edgeExtensionId = $registeredStoreIds.Edge
}
$extraChromiumOrigins = @()
foreach ($candidateId in @(
    $chromiumExtensionId,
    $edgeExtensionId
)) {
    if (-not [string]::IsNullOrWhiteSpace($candidateId)) {
        if ($candidateId -notmatch '^[a-p]{32}$') {
            throw "ID de extension Chromium no valido: $candidateId"
        }
        $origin = "chrome-extension://$candidateId/"
        if ($origin -ne "chrome-extension://pkefjandjcgdmhoonmhnllikibobijgg/" -and
            $extraChromiumOrigins -notcontains $origin) {
            $extraChromiumOrigins += $origin
        }
    }
}

$hosts = @(
    @{
        Name = "com.grxfirma.native"
        ChromeOrigins = @("chrome-extension://pkefjandjcgdmhoonmhnllikibobijgg/") + $extraChromiumOrigins
        FirefoxExtensions = @("grxfirma@aavidad.github.io")
    },
    @{
        Name = "io.github.aavidad.grxfirma"
        ChromeOrigins = @("chrome-extension://pkefjandjcgdmhoonmhnllikibobijgg/") + $extraChromiumOrigins
        FirefoxExtensions = @("grxfirma@aavidad.github.io")
    }
)

$nativeRegistryPlans = @()
foreach ($nativeHost in $hosts) {
    $chromeManifest = @{
        name = $nativeHost.Name
        description = "GrxFirma Native Messaging Host"
        path = $exeTarget
        type = "stdio"
        allowed_origins = $nativeHost.ChromeOrigins
    } | ConvertTo-Json -Depth 5

    $firefoxManifest = @{
        name = $nativeHost.Name
        description = "GrxFirma Native Messaging Host"
        path = $exeTarget
        type = "stdio"
        allowed_extensions = $nativeHost.FirefoxExtensions
    } | ConvertTo-Json -Depth 5

    $chromeManifestPath = Join-Path $manifestsDir "$($nativeHost.Name).chrome.json"
    $firefoxManifestPath = Join-Path $manifestsDir "$($nativeHost.Name).firefox.json"

    Write-Utf8NoBom -Path $chromeManifestPath -Value $chromeManifest
    Write-Utf8NoBom -Path $firefoxManifestPath -Value $firefoxManifest

    foreach ($baseKey in @(
        "Software\Google\Chrome\NativeMessagingHosts",
        "Software\Chromium\NativeMessagingHosts",
        "Software\Microsoft\Edge\NativeMessagingHosts",
        "Software\BraveSoftware\Brave-Browser\NativeMessagingHosts",
        "Software\Vivaldi\NativeMessagingHosts",
        "Software\Opera Software\NativeMessagingHosts"
    )) {
        $nativeRegistryPlans += [pscustomobject]@{
            Path = "$baseKey\$($nativeHost.Name)"
            Value = $chromeManifestPath
        }
    }
    $nativeRegistryPlans += [pscustomobject]@{
        Path = "Software\Mozilla\NativeMessagingHosts\$($nativeHost.Name)"
        Value = $firefoxManifestPath
    }
}

$installFirefoxXpi = $false
if (Test-Path $firefoxXpiSource) {
    Copy-Item $firefoxXpiSource (Join-Path $extensionsDir "firefox\\grxfirma-extension-firefox.xpi") -Force
    $firefoxMetadataTarget = Join-Path $extensionsDir "firefox\\grxfirma-extension-firefox.metadata.json"
    if (Test-Path $firefoxMetadataSource) {
        Copy-Item $firefoxMetadataSource $firefoxMetadataTarget -Force
    } elseif (Test-Path $firefoxMetadataTarget) {
        Remove-Item $firefoxMetadataTarget -Force
    }
    if (Test-FirefoxXpiApproved -XpiPath $firefoxXpiSource -MetadataPath $firefoxMetadataSource) {
        $installFirefoxXpi = $true
    } else {
        Write-Warning "El XPI Firefox no esta firmado/aprobado o no coincide con su SHA-256; no se instala en perfiles estables."
    }
}
if (Test-Path $chromiumZipSource) {
    Copy-Item $chromiumZipSource (Join-Path $extensionsDir "chromium\\grxfirma-extension-chromium.zip") -Force
}
foreach ($oldExtensionFile in @(
    "grxfirma-extension-chromium.crx",
    "grxfirma-extension-chromium.id",
    "grxfirma-extension-chromium.version",
    "dipgra-extension-chromium.crx",
    "dipgra-extension-chromium.id",
    "dipgra-extension-chromium.version"
)) {
    $oldExtensionPath = Join-Path (Join-Path $extensionsDir "chromium") $oldExtensionFile
    if (Test-Path -LiteralPath $oldExtensionPath -PathType Leaf) {
        Remove-Item -LiteralPath $oldExtensionPath -Force
    }
}
$chromiumRegistrationRequested =
    (-not [string]::IsNullOrWhiteSpace($chromiumExtensionId)) -or
    (-not [string]::IsNullOrWhiteSpace($edgeExtensionId))

$nativeSnapshots = @(
    $nativeRegistryPlans | ForEach-Object {
        Get-RegistroCadenaSnapshot -Ruta ([string]$_.Path) -Nombre ""
    }
)
$nativeAttempted = 0
$registeredBrowsers = @()
try {
    foreach ($plan in $nativeRegistryPlans) {
        $nativeAttempted++
        Set-RegistroPredeterminado ([string]$plan.Path) ([string]$plan.Value)
    }
    if ($chromiumRegistrationRequested) {
        $registeredBrowsers = @(
            Register-ChromiumUpdateUrlExtension `
                -ExtensionId $chromiumExtensionId `
                -ExistingEdgeExtensionId $edgeExtensionId `
                -RegistrationInventoryPath $registrationsPath
        )
    }
} catch {
    $nativeRegistrationError = $_.Exception
    $nativeRollbackErrors = @()
    for ($index = $nativeAttempted - 1; $index -ge 0; $index--) {
        try {
            Restore-RegistroCadenaSnapshot `
                -Snapshot $nativeSnapshots[$index] `
                -ExpectedCurrentValue ([string]$nativeRegistryPlans[$index].Value)
        } catch {
            $nativeRollbackErrors += $_.Exception.Message
        }
    }
    if ($nativeRollbackErrors.Count -gt 0) {
        throw "Fallo instalando Native Messaging: $($nativeRegistrationError.Message). Rollback incompleto: $($nativeRollbackErrors -join '; ')"
    }
    throw $nativeRegistrationError
}

if ($chromiumRegistrationRequested) {
    if ($registeredBrowsers.Count -gt 0) {
        $registeredBrowserNames = @($registeredBrowsers | ForEach-Object { $_.browser })
        Write-Output "Ficha de la tienda registrada para: $($registeredBrowserNames -join ', '). Abra el navegador y active la extension si se lo solicita."
    }
} else {
    Write-Output "Sin ID publicado de Chrome o Edge: no se registra ninguna extension. Configure GRXFIRMA_CHROMIUM_EXTENSION_ID o GRXFIRMA_EDGE_EXTENSION_ID tras publicarla."
}
if ($installFirefoxXpi) {
    try {
        Install-FirefoxExtension -XpiSource $firefoxXpiSource
    } catch {
        Write-Warning "No se pudo completar el despliegue opcional de Firefox: $($_.Exception.Message)"
    }
}
try {
    Remove-LegacyNativeHostInstallation -ManifestsDir $manifestsDir -ExtensionsDir $extensionsDir
} catch {
    Write-Warning "No se pudieron retirar los registros anteriores de Native Messaging: $($_.Exception.Message)"
}

Write-Output "NativeHost instalado en: $InstallDir"
Write-Output "Manifiestos generados en: $manifestsDir"
} catch {
    $installError = $_.Exception
    if ($filesystemMutationStarted) {
        try {
            Restore-NativeHostInstallSnapshot `
                -InstallDir $InstallDir `
                -BackupPayload $backupPayload `
                -PreviousInstallExisted $previousInstallExisted
        } catch {
            $preserveBackupForRecovery = $true
            throw "Fallo instalando NativeHost: $($installError.Message). Rollback de ficheros incompleto: $($_.Exception.Message). Instantanea conservada en $backupPayload"
        }
    }
    throw $installError
} finally {
    if (-not $preserveBackupForRecovery -and (Test-Path -LiteralPath $backupRoot)) {
        Remove-Item -LiteralPath $backupRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
