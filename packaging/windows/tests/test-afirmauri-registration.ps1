# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
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

function Copy-TestSnapshot {
    param([object]$Snapshot)

    return [pscustomobject]@{
        Path = [string]$Snapshot.Path
        Name = [string]$Snapshot.Name
        KeyExisted = [bool]$Snapshot.KeyExisted
        ValueExisted = [bool]$Snapshot.ValueExisted
        Kind = if ($null -eq $Snapshot.Kind) {
            $null
        } else {
            [string]$Snapshot.Kind
        }
        Value = if ($Snapshot.Value -is [array]) {
            @($Snapshot.Value)
        } else {
            $Snapshot.Value
        }
    }
}

function Get-TestSnapshotKey {
    param(
        [string]$Path,
        [string]$Name
    )
    return "$($Path.ToLowerInvariant())|$Name"
}

$windowsDir = Split-Path -Parent $PSScriptRoot
$helperPath = Join-Path $windowsDir "afirmauri-registration.ps1"
. $helperPath

$protocolKey = "Software\GrxFirma\Tests\AfirmaURI-Mock"
$executablePath = "C:\Users\Test\AppData\Local\Programs\GrxFirma\AfirmaURI\grxfirma-afirmauri.exe"
$plan = @(Get-AfirmaProtocolRegistrationPlan `
    -ProtocolKey $protocolKey `
    -ExecutablePath $executablePath)
Assert-True ($plan.Count -eq 4) "El plan afirma:// no contiene los cuatro valores esperados"
Assert-True (
    [string]$plan[2].Path -eq "$protocolKey\DefaultIcon"
) "El plan afirma:// no incluye DefaultIcon"
Assert-True (
    [string]$plan[2].Value -eq "$executablePath,0"
) "DefaultIcon no apunta al ejecutable instalado"
$iconPath = "C:\Users\Test\AppData\Local\Programs\GrxFirma\AfirmaURI\grxfirma-diputacion.ico"
$brandedPlan = @(Get-AfirmaProtocolRegistrationPlan `
    -ProtocolKey $protocolKey `
    -ExecutablePath $executablePath `
    -IconPath $iconPath)
Assert-True (
    [string]$brandedPlan[2].Value -eq "$iconPath,0"
) "DefaultIcon no admite el icono canónico de GrxFirma Diputación"

$tmp = Join-Path `
    ([System.IO.Path]::GetTempPath()) `
    ("grxfirma-afirmauri-test-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $script:mockRegistry = @{}
    $script:mockSetAttempts = 0
    $script:mockFailSetAt = 0
    $script:mockRestoreAttempts = 0

    function Get-AfirmaRegistryValueSnapshot {
        param(
            [string]$Path,
            [string]$Name
        )
        $key = Get-TestSnapshotKey -Path $Path -Name $Name
        if ($script:mockRegistry.ContainsKey($key)) {
            return Copy-TestSnapshot -Snapshot $script:mockRegistry[$key]
        }
        return [pscustomobject]@{
            Path = $Path
            Name = $Name
            KeyExisted = $false
            ValueExisted = $false
            Kind = $null
            Value = $null
        }
    }

    function Set-AfirmaRegistryString {
        param(
            [string]$Path,
            [string]$Name,
            [string]$Value
        )
        $script:mockSetAttempts++
        if ($script:mockFailSetAt -gt 0 -and
            $script:mockSetAttempts -eq $script:mockFailSetAt) {
            throw "fallo de registro simulado"
        }
        $script:mockRegistry[
            (Get-TestSnapshotKey -Path $Path -Name $Name)
        ] = [pscustomobject]@{
            Path = $Path
            Name = $Name
            KeyExisted = $true
            ValueExisted = $true
            Kind = [Microsoft.Win32.RegistryValueKind]::String.ToString()
            Value = $Value
        }
    }

    function Restore-AfirmaRegistryValueSnapshot {
        param(
            [object]$Snapshot,
            [object]$ExpectedCurrentSnapshot
        )
        $script:mockRestoreAttempts++
        $key = Get-TestSnapshotKey `
            -Path ([string]$Snapshot.Path) `
            -Name ([string]$Snapshot.Name)
        $current = Get-AfirmaRegistryValueSnapshot `
            -Path ([string]$Snapshot.Path) `
            -Name ([string]$Snapshot.Name)
        if (Test-AfirmaRegistrySnapshotsEqual -Left $current -Right $Snapshot) {
            return
        }
        if (-not (Test-AfirmaRegistrySnapshotsEqual `
                -Left $current `
                -Right $ExpectedCurrentSnapshot)) {
            throw "cambio de registro simulado"
        }
        $script:mockRegistry[$key] = Copy-TestSnapshot -Snapshot $Snapshot
    }

    function Remove-AfirmaEmptySnapshotKeys {
        param([object[]]$Snapshots)
    }

    $oldValues = @(
        [pscustomobject]@{
            Path = $plan[0].Path
            Name = $plan[0].Name
            KeyExisted = $true
            ValueExisted = $true
            Kind = [Microsoft.Win32.RegistryValueKind]::ExpandString.ToString()
            Value = "URL:Handler anterior"
        },
        [pscustomobject]@{
            Path = $plan[1].Path
            Name = $plan[1].Name
            KeyExisted = $true
            ValueExisted = $false
            Kind = $null
            Value = $null
        },
        [pscustomobject]@{
            Path = $plan[2].Path
            Name = $plan[2].Name
            KeyExisted = $true
            ValueExisted = $true
            Kind = [Microsoft.Win32.RegistryValueKind]::String.ToString()
            Value = "C:\OldSigner\old.ico,0"
        },
        [pscustomobject]@{
            Path = $plan[3].Path
            Name = $plan[3].Name
            KeyExisted = $true
            ValueExisted = $true
            Kind = [Microsoft.Win32.RegistryValueKind]::ExpandString.ToString()
            Value = '"C:\OldSigner\old.exe" "%1"'
        }
    )
    foreach ($oldValue in $oldValues) {
        $script:mockRegistry[
            (Get-TestSnapshotKey `
                -Path ([string]$oldValue.Path) `
                -Name ([string]$oldValue.Name))
        ] = Copy-TestSnapshot -Snapshot $oldValue
    }

    $snapshotPath = Join-Path $tmp "handler-snapshot.json"
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -SnapshotPath $snapshotPath
    $storedState = Read-AfirmaProtocolSnapshot `
        -Path $snapshotPath `
        -ProtocolKey $protocolKey `
        -Plan $plan
    for ($index = 0; $index -lt $oldValues.Count; $index++) {
        Assert-True (
            Test-AfirmaRegistrySnapshotsEqual `
                -Left $storedState.Snapshots[$index] `
                -Right $oldValues[$index]
        ) "La instalacion no conservo el handler anterior"
    }
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected @($storedState.OwnerValues)
    ) "La instalacion no escribio todos los valores de propiedad"

    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -SnapshotPath $snapshotPath
    $upgradeState = Read-AfirmaProtocolSnapshot `
        -Path $snapshotPath `
        -ProtocolKey $protocolKey `
        -Plan $plan
    Assert-True (
        Test-AfirmaRegistrySnapshotsEqual `
            -Left $upgradeState.Snapshots[3] `
            -Right $oldValues[3]
    ) "El upgrade reemplazo la instantanea original por su propio comando"

    $snapshotBytes = [System.IO.File]::ReadAllBytes($snapshotPath)
    $tamperedState = Get-Content -LiteralPath $snapshotPath -Raw |
        ConvertFrom-Json
    $tamperedState.owner_values[2].Value = "C:\ForeignSigner\foreign.ico,0"
    [System.IO.File]::WriteAllText(
        $snapshotPath,
        ($tamperedState | ConvertTo-Json -Depth 8),
        [System.Text.UTF8Encoding]::new($false)
    )
    $script:mockSetAttempts = 0
    Assert-Throws {
        Install-AfirmaProtocolRegistration `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -SnapshotPath $snapshotPath
    } "La instalacion acepto un marcador de propiedad manipulado"
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La instalacion escribio registro tras detectar una instantanea manipulada"
    [System.IO.File]::WriteAllBytes($snapshotPath, $snapshotBytes)

    $foreignIcon = Copy-TestSnapshot -Snapshot $upgradeState.OwnerValues[2]
    $foreignIcon.Value = "C:\ForeignSigner\foreign.ico,0"
    $script:mockRegistry[
        (Get-TestSnapshotKey `
            -Path ([string]$foreignIcon.Path) `
            -Name ([string]$foreignIcon.Name))
    ] = $foreignIcon
    $script:mockSetAttempts = 0
    Assert-Throws {
        Install-AfirmaProtocolRegistration `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -SnapshotPath $snapshotPath
    } "La instalacion sobrescribio un DefaultIcon cambiado por otro handler"
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La instalacion escribio registro antes de abortar por perdida de propiedad"

    $script:mockRegistry[
        (Get-TestSnapshotKey `
            -Path ([string]$upgradeState.OwnerValues[2].Path) `
            -Name ([string]$upgradeState.OwnerValues[2].Name))
    ] = Copy-TestSnapshot -Snapshot $upgradeState.OwnerValues[2]
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues @($upgradeState.OwnerValues) `
        -Snapshots @($upgradeState.Snapshots)
    Assert-True $restored "La desinstalacion no restauro un handler del que seguia siendo propietaria"
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected $oldValues
    ) "La desinstalacion no restauro todos los valores anteriores"

    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -SnapshotPath $snapshotPath
    $ownedState = Read-AfirmaProtocolSnapshot `
        -Path $snapshotPath `
        -ProtocolKey $protocolKey `
        -Plan $plan
    $foreignIcon = Copy-TestSnapshot -Snapshot $ownedState.OwnerValues[2]
    $foreignIcon.Value = "C:\ForeignSigner\foreign.ico,0"
    $script:mockRegistry[
        (Get-TestSnapshotKey `
            -Path ([string]$foreignIcon.Path) `
            -Name ([string]$foreignIcon.Name))
    ] = $foreignIcon
    $script:mockRestoreAttempts = 0
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues @($ownedState.OwnerValues) `
        -Snapshots @($ownedState.Snapshots)
    Assert-True (-not $restored) `
        "La desinstalacion restauro valores tras perder DefaultIcon"
    Assert-True ($script:mockRestoreAttempts -eq 0) `
        "La desinstalacion empezo a restaurar sin conservar la propiedad completa"

    Remove-Item -LiteralPath $snapshotPath -Force
    $script:mockRegistry = @{}
    foreach ($oldValue in $oldValues) {
        $script:mockRegistry[
            (Get-TestSnapshotKey `
                -Path ([string]$oldValue.Path) `
                -Name ([string]$oldValue.Name))
        ] = Copy-TestSnapshot -Snapshot $oldValue
    }
    $script:mockSetAttempts = 0
    $script:mockFailSetAt = 3
    Assert-Throws {
        Install-AfirmaProtocolRegistration `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -SnapshotPath $snapshotPath
    } "No se propago el fallo parcial de registro"
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected $oldValues
    ) "El rollback no restauro el handler previo tras un fallo parcial"
    Assert-True (-not (Test-Path -LiteralPath $snapshotPath)) `
        "El rollback dejo una instantanea de una instalacion fallida"

    $migrationSnapshotPath = Join-Path $tmp "icon-migration-snapshot.json"
    $script:mockRegistry = @{}
    $script:mockSetAttempts = 0
    $script:mockFailSetAt = 0
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -SnapshotPath $migrationSnapshotPath
    $legacyState = Read-AfirmaProtocolSnapshot `
        -Path $migrationSnapshotPath `
        -ProtocolKey $protocolKey `
        -Plan $plan
    $legacySnapshots = @($legacyState.Snapshots | ForEach-Object {
        Copy-TestSnapshot -Snapshot $_
    })

    $legacySnapshotBytes = [System.IO.File]::ReadAllBytes(
        $migrationSnapshotPath
    )
    $foreignLegacyState = Get-Content `
        -LiteralPath $migrationSnapshotPath `
        -Raw |
        ConvertFrom-Json
    $foreignLegacyState.owner_values[2].Value =
        "C:\ForeignSigner\foreign.ico,0"
    [System.IO.File]::WriteAllText(
        $migrationSnapshotPath,
        ($foreignLegacyState | ConvertTo-Json -Depth 8),
        [System.Text.UTF8Encoding]::new($false)
    )
    $script:mockSetAttempts = 0
    Assert-Throws {
        Install-AfirmaProtocolRegistration `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -IconPath $iconPath `
            -SnapshotPath $migrationSnapshotPath
    } "La migracion acepto un propietario heredado con icono ajeno"
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La migracion escribio registro con un propietario heredado ajeno"
    [System.IO.File]::WriteAllBytes(
        $migrationSnapshotPath,
        $legacySnapshotBytes
    )

    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath `
        -SnapshotPath $migrationSnapshotPath
    $migratedState = Read-AfirmaProtocolSnapshot `
        -Path $migrationSnapshotPath `
        -ProtocolKey $protocolKey `
        -Plan $brandedPlan
    Assert-True (
        Test-AfirmaRegistrySnapshotsEqual `
            -Left $migratedState.OwnerValues[2] `
            -Right (New-AfirmaOwnedValueSnapshot -Plan $brandedPlan[2])
    ) "La migracion no actualizo la propiedad de DefaultIcon al icono de marca"
    for ($index = 0; $index -lt $legacySnapshots.Count; $index++) {
        Assert-True (
            Test-AfirmaRegistrySnapshotsEqual `
                -Left $migratedState.Snapshots[$index] `
                -Right $legacySnapshots[$index]
        ) "La migracion del icono reemplazo la instantanea original"
    }
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected @($migratedState.OwnerValues)
    ) "La migracion del icono no escribio el nuevo plan completo"

    $foreignMigratedIcon = Copy-TestSnapshot `
        -Snapshot $migratedState.OwnerValues[2]
    $foreignMigratedIcon.Value = "C:\ForeignSigner\foreign.ico,0"
    $script:mockRegistry[
        (Get-TestSnapshotKey `
            -Path ([string]$foreignMigratedIcon.Path) `
            -Name ([string]$foreignMigratedIcon.Name))
    ] = $foreignMigratedIcon
    $script:mockSetAttempts = 0
    Assert-Throws {
        Install-AfirmaProtocolRegistration `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -IconPath $iconPath `
            -SnapshotPath $migrationSnapshotPath
    } "La migracion acepto un DefaultIcon ajeno"
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La migracion escribio registro tras detectar un DefaultIcon ajeno"
    Remove-Item -LiteralPath $migrationSnapshotPath -Force

    . $helperPath
    $runningOnWindows =
        $PSVersionTable.PSEdition -eq "Desktop" -or
        $IsWindows -eq $true
    if ($runningOnWindows) {
        $realProtocolKey =
            "Software\GrxFirma\Tests\AfirmaURI-$([guid]::NewGuid().ToString('N'))"
        $realExecutable = "C:\GrxFirma-Test\grxfirma-afirmauri.exe"
        $realPlan = @(Get-AfirmaProtocolRegistrationPlan `
            -ProtocolKey $realProtocolKey `
            -ExecutablePath $realExecutable)
        $realSnapshotPath = Join-Path $tmp "real-handler-snapshot.json"
        try {
            $rootKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey(
                $realProtocolKey
            )
            try {
                $rootKey.SetValue(
                    "",
                    "URL:Handler de prueba",
                    [Microsoft.Win32.RegistryValueKind]::ExpandString
                )
                $rootKey.SetValue(
                    "foreign",
                    "preserve",
                    [Microsoft.Win32.RegistryValueKind]::String
                )
            } finally {
                $rootKey.Dispose()
            }
            $iconKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey(
                "$realProtocolKey\DefaultIcon"
            )
            try {
                $iconKey.SetValue(
                    "",
                    "C:\OldSigner\old.ico,0",
                    [Microsoft.Win32.RegistryValueKind]::String
                )
            } finally {
                $iconKey.Dispose()
            }
            $commandKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey(
                "$realProtocolKey\shell\open\command"
            )
            try {
                $commandKey.SetValue(
                    "",
                    '"%LOCALAPPDATA%\OldSigner\old.exe" "%1"',
                    [Microsoft.Win32.RegistryValueKind]::ExpandString
                )
            } finally {
                $commandKey.Dispose()
            }

            $realOriginal = @($realPlan | ForEach-Object {
                Get-AfirmaRegistryValueSnapshot `
                    -Path ([string]$_.Path) `
                    -Name ([string]$_.Name)
            })
            Install-AfirmaProtocolRegistration `
                -ProtocolKey $realProtocolKey `
                -ExecutablePath $realExecutable `
                -SnapshotPath $realSnapshotPath
            $realState = Read-AfirmaProtocolSnapshot `
                -Path $realSnapshotPath `
                -ProtocolKey $realProtocolKey `
                -Plan $realPlan
            Assert-True (
                Test-AfirmaProtocolValuesMatch -Expected @($realState.OwnerValues)
            ) "La escritura HKCU real no registro comando e icono"

            Install-AfirmaProtocolRegistration `
                -ProtocolKey $realProtocolKey `
                -ExecutablePath $realExecutable `
                -SnapshotPath $realSnapshotPath
            $realUpgradeState = Read-AfirmaProtocolSnapshot `
                -Path $realSnapshotPath `
                -ProtocolKey $realProtocolKey `
                -Plan $realPlan
            Assert-True (
                Test-AfirmaRegistrySnapshotsEqual `
                    -Left $realUpgradeState.Snapshots[3] `
                    -Right $realOriginal[3]
            ) "El upgrade HKCU real perdio el comando anterior"

            $restored = Restore-AfirmaProtocolRegistration `
                -OwnerValues @($realUpgradeState.OwnerValues) `
                -Snapshots @($realUpgradeState.Snapshots)
            Assert-True $restored "La restauracion HKCU real rechazo sus propios valores"
            Assert-True (
                Test-AfirmaProtocolValuesMatch -Expected $realOriginal
            ) "La restauracion HKCU real no conservo valores y tipos"
            $rootKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey(
                $realProtocolKey,
                $false
            )
            try {
                Assert-True ($rootKey.GetValue("foreign") -eq "preserve") `
                    "La restauracion elimino un valor ajeno no sustituido"
            } finally {
                $rootKey.Dispose()
            }

            [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree(
                $realProtocolKey,
                $false
            )
            Remove-Item -LiteralPath $realSnapshotPath -Force
            Install-AfirmaProtocolRegistration `
                -ProtocolKey $realProtocolKey `
                -ExecutablePath $realExecutable `
                -SnapshotPath $realSnapshotPath
            $emptyOriginalState = Read-AfirmaProtocolSnapshot `
                -Path $realSnapshotPath `
                -ProtocolKey $realProtocolKey `
                -Plan $realPlan
            $restored = Restore-AfirmaProtocolRegistration `
                -OwnerValues @($emptyOriginalState.OwnerValues) `
                -Snapshots @($emptyOriginalState.Snapshots)
            Assert-True $restored `
                "La restauracion HKCU real rechazo una instalacion nueva"
            $remainingKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey(
                $realProtocolKey,
                $false
            )
            if ($null -ne $remainingKey) {
                $remainingKey.Dispose()
                throw "La restauracion HKCU real dejo una clave afirma:// que antes no existia"
            }
        } finally {
            [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree(
                $realProtocolKey,
                $false
            )
        }
    }
} finally {
    if (Test-Path -LiteralPath $tmp) {
        Remove-Item -LiteralPath $tmp -Recurse -Force
    }
}

Write-Output "Windows afirma:// registration tests passed."
