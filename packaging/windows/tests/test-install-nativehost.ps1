# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"
$runningOnWindows =
    $PSVersionTable.PSEdition -eq "Desktop" -or
    $IsWindows -eq $true

$scriptPath = Join-Path (Split-Path -Parent $PSScriptRoot) "install-nativehost.ps1"
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    $scriptPath,
    [ref]$tokens,
    [ref]$parseErrors
)
if ($parseErrors.Count -gt 0) {
    throw "No se pudo analizar install-nativehost.ps1"
}

$requiredFunctions = @(
    "Test-FirefoxXpiApproved",
    "Write-Utf8NoBom",
    "Register-ChromiumUpdateUrlExtension",
    "Get-RegisteredStoreExtensionIds",
    "Remove-LegacyNativeHostInstallation"
)
foreach ($functionName in $requiredFunctions) {
    $definition = $ast.FindAll({
        param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
            $node.Name -eq $functionName
    }, $true) | Select-Object -First 1
    if ($null -eq $definition) {
        throw "Falta la funcion $functionName"
    }
    . ([ScriptBlock]::Create($definition.Extent.Text))
}

# La extension portafirmas no forma parte de GrxFirma: el instalador no debe
# registrar su host ni autorizar sus identificadores.
$installScriptText = [IO.File]::ReadAllText($scriptPath)
foreach ($forbidden in @("portafirmas@dipgra.es", "ipkpimgjhkjibkbhfdhggjldlaetbcoa")) {
    if ($installScriptText.Contains($forbidden)) {
        throw "install-nativehost.ps1 todavia autoriza $forbidden"
    }
}
if ($installScriptText -match 'Name\s*=\s*"io\.github\.aavidad\.portafirmas"') {
    throw "install-nativehost.ps1 todavia registra el host io.github.aavidad.portafirmas"
}

$uninstallScriptPath = Join-Path (Split-Path -Parent $PSScriptRoot) "uninstall-nativehost.ps1"
$uninstallTokens = $null
$uninstallParseErrors = $null
$uninstallAst = [System.Management.Automation.Language.Parser]::ParseFile(
    $uninstallScriptPath,
    [ref]$uninstallTokens,
    [ref]$uninstallParseErrors
)
if ($uninstallParseErrors.Count -gt 0) {
    throw "No se pudo analizar uninstall-nativehost.ps1"
}
$removeFirefoxDefinition = $uninstallAst.FindAll({
    param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq "Remove-FirefoxXpiIfOwned"
}, $true) | Select-Object -First 1
if ($null -eq $removeFirefoxDefinition) {
    throw "Falta la funcion Remove-FirefoxXpiIfOwned"
}
. ([ScriptBlock]::Create($removeFirefoxDefinition.Extent.Text))

function Assert-NoGrxFirmaReparsePoint {
    param([string]$Path)
}
$restoreInstallDefinition = $ast.FindAll({
    param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq "Restore-NativeHostInstallSnapshot"
}, $true) | Select-Object -First 1
if ($null -eq $restoreInstallDefinition) {
    throw "Falta la funcion Restore-NativeHostInstallSnapshot"
}
. ([ScriptBlock]::Create($restoreInstallDefinition.Extent.Text))

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

$snapshotDefinition = $ast.FindAll({
    param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq "Get-RegistroCadenaSnapshot"
}, $true) | Select-Object -First 1
Assert-True ($null -ne $snapshotDefinition) `
    "Falta la funcion Get-RegistroCadenaSnapshot"
$snapshotNameParameter = $snapshotDefinition.Body.ParamBlock.Parameters |
    Where-Object { $_.Name.VariablePath.UserPath -eq "Nombre" } |
    Select-Object -First 1
Assert-True ($null -ne $snapshotNameParameter) `
    "Get-RegistroCadenaSnapshot no declara el parametro Nombre"
$allowsEmptyDefaultValueName = @(
    $snapshotNameParameter.Attributes | Where-Object {
        $_.TypeName.FullName -match '(^|\.)AllowEmptyString(Attribute)?$'
    }
).Count -gt 0
Assert-True $allowsEmptyDefaultValueName `
    "Get-RegistroCadenaSnapshot debe aceptar Nombre vacio para el valor predeterminado"

$script:registryWrites = @()
$script:registryWriteAttempts = 0
$script:failRegistryWriteAt = 0
$script:restoredSnapshots = @()
function Set-RegistroCadena {
    param(
        [string]$Ruta,
        [string]$Nombre,
        [string]$Valor
    )
    $script:registryWriteAttempts++
    if ($script:failRegistryWriteAt -gt 0 -and
        $script:registryWriteAttempts -eq $script:failRegistryWriteAt) {
        throw "fallo de registro simulado"
    }
    $script:registryWrites += [pscustomobject]@{
        Path = $Ruta
        Name = $Nombre
        Value = $Valor
    }
}

function Get-RegistroCadenaSnapshot {
    param(
        [string]$Ruta,
        [string]$Nombre
    )
    return [pscustomobject]@{
        Path = $Ruta
        Name = $Nombre
        KeyExisted = $false
        ValueExisted = $false
        Value = $null
    }
}

function Restore-RegistroCadenaSnapshot {
    param(
        [object]$Snapshot,
        [string]$ExpectedCurrentValue
    )
    $script:restoredSnapshots += [pscustomobject]@{
        Snapshot = $Snapshot
        ExpectedCurrentValue = $ExpectedCurrentValue
    }
}

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("grxfirma-win-installer-test-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
$previousChromiumExtensionId = $env:GRXFIRMA_CHROMIUM_EXTENSION_ID
$previousEdgeExtensionId = $env:GRXFIRMA_EDGE_EXTENSION_ID
$previousChromiumUpdateUrl = $env:GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL
$previousEdgeUpdateUrl = $env:GRXFIRMA_EDGE_EXTENSION_UPDATE_URL
try {
    $xpi = Join-Path $tmp "extension.xpi"
    $metadataPath = Join-Path $tmp "extension.metadata.json"
    [IO.File]::WriteAllBytes($xpi, [Text.Encoding]::UTF8.GetBytes("test-xpi"))
    $hash = (Get-FileHash -Path $xpi -Algorithm SHA256).Hash.ToLowerInvariant()
    $metadata = @{
        extension_id = "grxfirma@aavidad.github.io"
        signed = $true
        version = "1.0.0"
        xpi_sha256 = $hash
    } | ConvertTo-Json
    Write-Utf8NoBom -Path $metadataPath -Value $metadata

    Assert-True (Test-FirefoxXpiApproved -XpiPath $xpi -MetadataPath $metadataPath) "XPI valido rechazado"
    [IO.File]::AppendAllText($xpi, "tampered")
    Assert-True (-not (Test-FirefoxXpiApproved -XpiPath $xpi -MetadataPath $metadataPath)) "XPI manipulado aceptado"

    $packagedFirefoxXpi = Join-Path $tmp "packaged-firefox.xpi"
    [IO.File]::WriteAllText($packagedFirefoxXpi, "signed-firefox-package")
    $packagedFirefoxHash = (Get-FileHash -LiteralPath $packagedFirefoxXpi -Algorithm SHA256).Hash
    $packagedFirefoxHashes = @($packagedFirefoxHash)
    $ownedFirefoxXpi = Join-Path $tmp "owned-firefox.xpi"
    Copy-Item -LiteralPath $packagedFirefoxXpi -Destination $ownedFirefoxXpi
    Remove-FirefoxXpiIfOwned -Path $ownedFirefoxXpi
    Assert-True (-not (Test-Path -LiteralPath $ownedFirefoxXpi)) "No se retiro el XPI Firefox propiedad de la instalacion"
    $updatedFirefoxXpi = Join-Path $tmp "updated-firefox.xpi"
    [IO.File]::WriteAllText($updatedFirefoxXpi, "newer-firefox-package")
    Remove-FirefoxXpiIfOwned -Path $updatedFirefoxXpi
    Assert-True (Test-Path -LiteralPath $updatedFirefoxXpi) "Se retiro un XPI Firefox actualizado por otro origen"

    $restoreRoot = Join-Path $tmp "filesystem-rollback"
    $restoreInstallDir = Join-Path $restoreRoot "NativeHost"
    $restoreBackupPayload = Join-Path $restoreRoot "backup/payload"
    New-Item -ItemType Directory -Force -Path $restoreInstallDir | Out-Null
    New-Item -ItemType Directory -Force -Path $restoreBackupPayload | Out-Null
    [IO.File]::WriteAllText((Join-Path $restoreInstallDir "new.exe"), "new")
    [IO.File]::WriteAllText((Join-Path $restoreBackupPayload "old.exe"), "old")
    Restore-NativeHostInstallSnapshot `
        -InstallDir $restoreInstallDir `
        -BackupPayload $restoreBackupPayload `
        -PreviousInstallExisted $true
    Assert-True (Test-Path -LiteralPath (Join-Path $restoreInstallDir "old.exe")) "El rollback no restauro el payload anterior"
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $restoreInstallDir "new.exe"))) "El rollback conservo el payload parcial"
    Assert-True (-not (Test-Path -LiteralPath $restoreBackupPayload)) "El rollback no consumio la instantanea restaurada"

    Remove-Item -LiteralPath $restoreInstallDir -Recurse -Force
    New-Item -ItemType Directory -Path $restoreInstallDir | Out-Null
    [IO.File]::WriteAllText((Join-Path $restoreInstallDir "partial.exe"), "partial")
    Restore-NativeHostInstallSnapshot `
        -InstallDir $restoreInstallDir `
        -BackupPayload (Join-Path $restoreRoot "unused") `
        -PreviousInstallExisted $false
    Assert-True (-not (Test-Path -LiteralPath $restoreInstallDir)) "El rollback de instalacion nueva dejo ficheros parciales"

    $bytes = [IO.File]::ReadAllBytes($metadataPath)
    $hasBom = $bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
    Assert-True (-not $hasBom) "El JSON se escribio con BOM UTF-8"

    $env:GRXFIRMA_CHROMIUM_EXTENSION_ID = $null
    $env:GRXFIRMA_EDGE_EXTENSION_ID = $null
    $env:GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL = $null
    $env:GRXFIRMA_EDGE_EXTENSION_UPDATE_URL = $null
    $registered = @(Register-ChromiumUpdateUrlExtension)
    Assert-True ($registered.Count -eq 0) "Se registro una extension sin ID publicado"
    Assert-True ($script:registryWrites.Count -eq 0) "Se escribio el registro sin ID publicado"
    $env:GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL = "https://updates.example.test/extension.xml"
    $customUrlRejected = $false
    try {
        Register-ChromiumUpdateUrlExtension -ExtensionId "abcdefghijklmnopabcdefghijklmnop" | Out-Null
    } catch {
        $customUrlRejected = $true
    }
    Assert-True $customUrlRejected "Se acepto una URL ajena a Chrome Web Store"
    $env:GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL = $null

    $sourceText = Get-Content -LiteralPath $scriptPath -Raw
    Assert-True (-not ($sourceText -match '\$chromiumCrxSource|Copy-Item\s+\$chromiumCrx')) `
        "El instalador todavia depende de un CRX local"

    $chromeId = "abcdefghijklmnopabcdefghijklmnop"
    $edgeId = "ponmlkjihgfedcbaponmlkjihgfedcba"
    $registrationInventory = Join-Path $tmp "registrations.json"
    $registered = @(
        Register-ChromiumUpdateUrlExtension `
            -ExtensionId $chromeId `
            -RegistrationInventoryPath $registrationInventory
    )
    Assert-True ($registered.Count -eq 5) "El ID Chrome no registro los cinco navegadores Chromium"
    Assert-True ($script:registryWrites.Count -eq 5) "No se escribieron todos los registros Chromium"
    Assert-True (@($registered | Where-Object { $_.browser -eq "Edge" }).Count -eq 0) `
        "Se registro Edge sin ID publicado propio"
    $storedRegistrations = Get-Content -LiteralPath $registrationInventory -Raw | ConvertFrom-Json
    Assert-True ($storedRegistrations.schema_version -eq 1) "Inventario Chromium con esquema inesperado"
    Assert-True (@($storedRegistrations.registrations).Count -eq 5) "Inventario Chromium incompleto"
    foreach ($record in $registered) {
        Assert-True ($record.extension_id -eq $chromeId) "Inventario Chromium con ID inesperado"
        Assert-True ($record.registry_path -match "\\Extensions\\$chromeId`$") "Inventario Chromium con ruta inesperada"
        Assert-True ($record.update_url -eq "https://clients2.google.com/service/update2/crx") `
            "El registro Chromium no apunta a Chrome Web Store"
    }

    $savedIds = Get-RegisteredStoreExtensionIds -RegistrationInventoryPath $registrationInventory
    Assert-True ($savedIds.Chrome -eq $chromeId) "No se recupero el ID publicado de Chrome"
    Assert-True ([string]::IsNullOrWhiteSpace($savedIds.Edge)) "Se invento un ID publicado de Edge"
    $script:registryWrites = @()
    $registered = @(
        Register-ChromiumUpdateUrlExtension `
            -ExistingEdgeExtensionId $edgeId `
            -RegistrationInventoryPath $registrationInventory
    )
    Assert-True ($registered.Count -eq 1) "El ID publicado solo en Edge registro otros navegadores"
    Assert-True ($registered[0].browser -eq "Edge") "No se registro Edge"
    Assert-True ($registered[0].extension_id -eq $edgeId) "El registro Edge uso otro ID"
    Assert-True ($registered[0].update_url -eq "https://edge.microsoft.com/extensionwebstorebase/v1/crx") `
        "El registro Edge no apunta a Microsoft Edge Add-ons"
    $savedIds = Get-RegisteredStoreExtensionIds -RegistrationInventoryPath $registrationInventory
    Assert-True ($savedIds.Edge -eq $edgeId) "No se recupero el ID publicado de Edge"
    $untrustedInventory = Join-Path $tmp "untrusted-registrations.json"
    Write-Utf8NoBom -Path $untrustedInventory -Value (@{
        schema_version = 1
        registrations = @(@{
            browser = "Chrome"
            extension_id = $chromeId
            registry_path = "Software\Google\Chrome\Extensions\$chromeId"
            update_url = "https://updates.example.test/extension.xml"
        })
    } | ConvertTo-Json -Depth 5)
    $untrustedIds = Get-RegisteredStoreExtensionIds -RegistrationInventoryPath $untrustedInventory
    Assert-True ([string]::IsNullOrWhiteSpace($untrustedIds.Chrome)) `
        "Se recupero como tienda un registro con URL ajena"

    $env:GRXFIRMA_EDGE_EXTENSION_ID = "invalid-edge-id"
    $script:registryWrites = @()
    $edgeIdRejected = $false
    try {
        Register-ChromiumUpdateUrlExtension -ExtensionId $chromeId | Out-Null
    } catch {
        $edgeIdRejected = $true
    }
    Assert-True $edgeIdRejected "Se acepto un ID Edge no valido"
    Assert-True ($script:registryWrites.Count -eq 0) "Se escribieron registros parciales antes de validar Edge"
    $env:GRXFIRMA_EDGE_EXTENSION_ID = $null

    [IO.File]::WriteAllText($registrationInventory, "inventario-anterior")
    $script:registryWrites = @()
    $script:registryWriteAttempts = 0
    $script:failRegistryWriteAt = 3
    $script:restoredSnapshots = @()
    $transactionFailed = $false
    try {
        Register-ChromiumUpdateUrlExtension `
            -ExtensionId $chromeId `
            -RegistrationInventoryPath $registrationInventory | Out-Null
    } catch {
        $transactionFailed = $true
    }
    Assert-True $transactionFailed "No se propago el fallo parcial de registro Chromium"
    Assert-True ($script:restoredSnapshots.Count -eq 3) "El rollback Chromium no cubrio todos los intentos"
    Assert-True (
        (Get-Content -LiteralPath $registrationInventory -Raw) -eq "inventario-anterior"
    ) "El rollback Chromium no restauro el inventario anterior"
    $script:failRegistryWriteAt = 0

    $env:GRXFIRMA_CHROMIUM_EXTENSION_ID = "invalid-extension-id"
    $invalidIdRejected = $false
    try {
        Register-ChromiumUpdateUrlExtension -ExtensionId $chromeId | Out-Null
    } catch {
        $invalidIdRejected = $true
    }
    Assert-True $invalidIdRejected "Se acepto un ID Chromium no valido"
    $env:GRXFIRMA_CHROMIUM_EXTENSION_ID = $null

    $legacyRoot = Join-Path $tmp "legacy-migration"
    $legacyManifests = Join-Path $legacyRoot "manifests"
    $legacyExtensions = Join-Path $legacyRoot "extensions"
    $legacyProfile = Join-Path $legacyRoot "profile"
    foreach ($dir in @(
        $legacyManifests,
        (Join-Path $legacyExtensions "firefox"),
        (Join-Path $legacyExtensions "chromium"),
        (Join-Path $legacyProfile "extensions")
    )) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    foreach ($legacyHostName in @("com.dipgra.grxfirma", "com.dipgra.portafirmas", "io.github.aavidad.portafirmas")) {
        foreach ($suffix in @("chrome.json", "firefox.json")) {
            [IO.File]::WriteAllText((Join-Path $legacyManifests "$legacyHostName.$suffix"), "{}")
        }
    }
    $currentManifest = Join-Path $legacyManifests "io.github.aavidad.grxfirma.chrome.json"
    [IO.File]::WriteAllText($currentManifest, "{}")
    $legacyPackagedXpi = Join-Path $legacyExtensions "firefox\dipgra-extension-firefox.xpi"
    [IO.File]::WriteAllText($legacyPackagedXpi, "legacy-signed-xpi")
    [IO.File]::WriteAllText((Join-Path $legacyExtensions "firefox\dipgra-extension-firefox.metadata.json"), "{}")
    [IO.File]::WriteAllText((Join-Path $legacyExtensions "chromium\dipgra-extension-chromium.zip"), "zip")
    $legacyProfileXpi = Join-Path $legacyProfile "extensions\extension@dipgra.es.xpi"
    Copy-Item -LiteralPath $legacyPackagedXpi -Destination $legacyProfileXpi

    $script:legacyRegistryCalls = @()
    function Remove-LegacyNativeHostRegistration {
        param([string]$Path, [string]$ExpectedValue)
        $script:legacyRegistryCalls += [pscustomobject]@{ Path = $Path; Expected = $ExpectedValue }
        return $true
    }
    function Get-FirefoxProfileDirectory {
        return @(Get-Item -LiteralPath $legacyProfile)
    }
    $previousProgramFiles = $env:ProgramFiles
    $env:ProgramFiles = Join-Path $legacyRoot "no-program-files"
    try {
        Remove-LegacyNativeHostInstallation -ManifestsDir $legacyManifests -ExtensionsDir $legacyExtensions 3>$null
    } finally {
        $env:ProgramFiles = $previousProgramFiles
    }
    Assert-True ($script:legacyRegistryCalls.Count -eq 21) `
        "La migracion no reviso los siete navegadores para los tres hosts anteriores"
    Assert-True (@($script:legacyRegistryCalls | Where-Object {
        $_.Path -notmatch '\\(com\.dipgra\.(grxfirma|portafirmas)|io\.github\.aavidad\.portafirmas)$'
    }).Count -eq 0) "La migracion intento retirar un host que no es de versiones anteriores"
    Assert-True (@($script:legacyRegistryCalls | Where-Object {
        $_.Path -eq "Software\Mozilla\NativeMessagingHosts\com.dipgra.grxfirma" -and
            $_.Expected -eq (Join-Path $legacyManifests "com.dipgra.grxfirma.firefox.json")
    }).Count -eq 1) "La migracion no comprueba que el host anterior de Firefox apunte a esta instalacion"
    Assert-True (@(Get-ChildItem -LiteralPath $legacyManifests -Filter "com.dipgra.*").Count -eq 0) `
        "La migracion dejo manifiestos de hosts anteriores"
    Assert-True (@(Get-ChildItem -LiteralPath $legacyManifests -Filter "io.github.aavidad.portafirmas.*").Count -eq 0) `
        "La migracion dejo manifiestos del host de portafirmas"
    Assert-True (@($script:legacyRegistryCalls | Where-Object {
        $_.Path -eq "Software\Google\Chrome\NativeMessagingHosts\io.github.aavidad.portafirmas" -and
            $_.Expected -eq (Join-Path $legacyManifests "io.github.aavidad.portafirmas.chrome.json")
    }).Count -eq 1) "La migracion no comprueba que el host de portafirmas apunte a esta instalacion"
    Assert-True (Test-Path -LiteralPath $currentManifest) "La migracion borro un manifiesto vigente"
    Assert-True (-not (Test-Path -LiteralPath $legacyProfileXpi)) `
        "La migracion no retiro la extension Firefox anterior instalada por GrxFirma"
    Assert-True (@(Get-ChildItem -LiteralPath $legacyExtensions -Recurse -Filter "dipgra-extension-*").Count -eq 0) `
        "La migracion dejo paquetes de extension con el nombre anterior"

    [IO.File]::WriteAllText($legacyPackagedXpi, "legacy-signed-xpi")
    [IO.File]::WriteAllText($legacyProfileXpi, "xpi-replaced-by-user")
    Remove-LegacyNativeHostInstallation -ManifestsDir $legacyManifests -ExtensionsDir $legacyExtensions 3>$null
    Assert-True (Test-Path -LiteralPath $legacyProfileXpi) `
        "La migracion retiro una extension Firefox que no coincide con la instalada por GrxFirma"

    if ($runningOnWindows) {
        foreach ($functionName in @(
            "Set-RegistroCadena",
            "Get-RegistroCadenaSnapshot",
            "Restore-RegistroCadenaSnapshot"
        )) {
            $definition = $ast.FindAll({
                param($node)
                $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                    $node.Name -eq $functionName
            }, $true) | Select-Object -First 1
            if ($null -eq $definition) {
                throw "Falta la funcion de registro real $functionName"
            }
            . ([ScriptBlock]::Create($definition.Extent.Text))
        }
        $removeRegistryDefinition = $uninstallAst.FindAll({
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                $node.Name -eq "Remove-RegistryValueIfOwned"
        }, $true) | Select-Object -First 1
        if ($null -eq $removeRegistryDefinition) {
            throw "Falta la funcion Remove-RegistryValueIfOwned"
        }
        . ([ScriptBlock]::Create($removeRegistryDefinition.Extent.Text))

        $legacyRegistryDefinition = $ast.FindAll({
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                $node.Name -eq "Remove-LegacyNativeHostRegistration"
        }, $true) | Select-Object -First 1
        if ($null -eq $legacyRegistryDefinition) {
            throw "Falta la funcion Remove-LegacyNativeHostRegistration"
        }
        . ([ScriptBlock]::Create($legacyRegistryDefinition.Extent.Text))

        $testRegistryRoot = "Software\GrxFirma\Tests\NativeHost-$([guid]::NewGuid().ToString('N'))"
        try {
            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($testRegistryRoot)
            try {
                $registryKey.SetValue(
                    "",
                    "%LOCALAPPDATA%\GrxFirma\old-native-host.json",
                    [Microsoft.Win32.RegistryValueKind]::ExpandString
                )
                $registryKey.SetValue(
                    "update_url",
                    "https://old.example.test/update.xml",
                    [Microsoft.Win32.RegistryValueKind]::ExpandString
                )
            } finally {
                $registryKey.Dispose()
            }

            $defaultSnapshot = Get-RegistroCadenaSnapshot `
                -Ruta $testRegistryRoot `
                -Nombre ""
            Assert-True ($defaultSnapshot.Name -eq "") `
                "El snapshot no identifico el valor predeterminado del registro"
            Assert-True $defaultSnapshot.ValueExisted `
                "El snapshot no detecto el valor predeterminado existente"
            Assert-True (
                $defaultSnapshot.Value -eq "%LOCALAPPDATA%\GrxFirma\old-native-host.json"
            ) "El snapshot expandio o altero el valor predeterminado"
            Assert-True (
                $defaultSnapshot.Kind -eq [Microsoft.Win32.RegistryValueKind]::ExpandString
            ) "El snapshot no conservo el tipo del valor predeterminado"

            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($testRegistryRoot, $true)
            try {
                $registryKey.SetValue(
                    "",
                    "C:\changed-native-host.json",
                    [Microsoft.Win32.RegistryValueKind]::String
                )
            } finally {
                $registryKey.Dispose()
            }
            Restore-RegistroCadenaSnapshot `
                -Snapshot $defaultSnapshot `
                -ExpectedCurrentValue "C:\changed-native-host.json"
            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($testRegistryRoot, $false)
            try {
                Assert-True (
                    $registryKey.GetValue(
                        "",
                        $null,
                        [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
                    ) -eq "%LOCALAPPDATA%\GrxFirma\old-native-host.json"
                ) "El rollback real no restauro el valor predeterminado"
                Assert-True (
                    $registryKey.GetValueKind("") -eq
                        [Microsoft.Win32.RegistryValueKind]::ExpandString
                ) "El rollback real no restauro el tipo del valor predeterminado"
            } finally {
                $registryKey.Dispose()
            }

            $snapshot = Get-RegistroCadenaSnapshot -Ruta $testRegistryRoot -Nombre "update_url"
            Set-RegistroCadena $testRegistryRoot "update_url" "https://new.example.test/update.xml"
            Restore-RegistroCadenaSnapshot `
                -Snapshot $snapshot `
                -ExpectedCurrentValue "https://new.example.test/update.xml"
            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($testRegistryRoot, $false)
            try {
                Assert-True (
                    $registryKey.GetValue(
                        "update_url",
                        $null,
                        [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
                    ) -eq "https://old.example.test/update.xml"
                ) "El rollback real no restauro el valor de registro anterior"
                Assert-True (
                    $registryKey.GetValueKind("update_url") -eq
                        [Microsoft.Win32.RegistryValueKind]::ExpandString
                ) "El rollback real no restauro el tipo del valor anterior"
            } finally {
                $registryKey.Dispose()
            }

            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($testRegistryRoot, $true)
            try {
                $registryKey.SetValue("foreign", "preserve", [Microsoft.Win32.RegistryValueKind]::String)
                $registryKey.SetValue("update_url", "https://owned.example.test/update.xml", [Microsoft.Win32.RegistryValueKind]::String)
            } finally {
                $registryKey.Dispose()
            }
            Remove-RegistryValueIfOwned `
                -Path $testRegistryRoot `
                -ValueName "update_url" `
                -ExpectedValue "https://owned.example.test/update.xml"
            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($testRegistryRoot, $false)
            try {
                Assert-True (
                    @($registryKey.GetValueNames()) -notcontains "update_url"
                ) "El desinstalador no retiro su valor de registro"
                Assert-True (
                    $registryKey.GetValue("foreign") -eq "preserve"
                ) "El desinstalador retiro un valor de registro ajeno"
            } finally {
                $registryKey.Dispose()
            }

            Set-RegistroCadena $testRegistryRoot "update_url" "https://replaced.example.test/update.xml"
            Remove-RegistryValueIfOwned `
                -Path $testRegistryRoot `
                -ValueName "update_url" `
                -ExpectedValue "https://owned.example.test/update.xml"
            $registryKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($testRegistryRoot, $false)
            try {
                Assert-True (
                    $registryKey.GetValue("update_url") -eq "https://replaced.example.test/update.xml"
                ) "El desinstalador retiro un registro Chromium reemplazado"
            } finally {
                $registryKey.Dispose()
            }

            $legacyHostKey = "$testRegistryRoot\com.dipgra.grxfirma"
            $ownedLegacyManifest = "C:\GrxFirma\NativeHost\manifests\com.dipgra.grxfirma.chrome.json"
            Set-RegistroCadena $legacyHostKey "" $ownedLegacyManifest
            Assert-True (Remove-LegacyNativeHostRegistration -Path $legacyHostKey -ExpectedValue $ownedLegacyManifest) `
                "La migracion no retiro el host anterior propio"
            $legacyKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($legacyHostKey, $false)
            Assert-True ($null -eq $legacyKey) "La migracion dejo vacia la clave del host anterior"

            Set-RegistroCadena $legacyHostKey "" "C:\Otra\manifiesto.json"
            Assert-True (-not (Remove-LegacyNativeHostRegistration -Path $legacyHostKey -ExpectedValue $ownedLegacyManifest 3>$null)) `
                "La migracion retiro un host anterior que apunta a otra instalacion"
            $legacyKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($legacyHostKey, $false)
            try {
                Assert-True ($legacyKey.GetValue("") -eq "C:\Otra\manifiesto.json") `
                    "La migracion altero un host ajeno"
            } finally {
                $legacyKey.Dispose()
            }
        } finally {
            [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($testRegistryRoot, $false)
        }
    }
} finally {
    $env:GRXFIRMA_CHROMIUM_EXTENSION_ID = $previousChromiumExtensionId
    $env:GRXFIRMA_EDGE_EXTENSION_ID = $previousEdgeExtensionId
    $env:GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL = $previousChromiumUpdateUrl
    $env:GRXFIRMA_EDGE_EXTENSION_UPDATE_URL = $previousEdgeUpdateUrl
    if (Test-Path $tmp) {
        Remove-Item $tmp -Recurse -Force
    }
}

Write-Output "Windows native-host installer tests passed."
