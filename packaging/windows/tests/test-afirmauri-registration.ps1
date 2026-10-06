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
$iconPath = "C:\Users\Test\AppData\Local\Programs\GrxFirma\AfirmaURI\grxfirma-grx.ico"
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

    # Instantanea escrita por la 0.0.118: DefaultIcon apuntaba a
    # grxfirma-diputacion.ico en la misma carpeta del ejecutable.
    $installDir = "C:\Users\Test\AppData\Local\Programs\GrxFirma\AfirmaURI"
    $oldIconPath = "$installDir\grxfirma-diputacion.ico"
    $plan118 = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $oldIconPath)
    $owners118 = @($plan118 | ForEach-Object {
        New-AfirmaOwnedValueSnapshot -Plan $_
    })
    $absent118 = @($plan118 | ForEach-Object {
        New-AfirmaAbsentValueSnapshot -Plan $_
    })
    $snapshot118Path = Join-Path $tmp "afirma-protocol-snapshot-0.0.118.json"
    function Reset-Test118State {
        param([object[]]$OwnerValues = $owners118)
        $script:mockRegistry = @{}
        foreach ($ownerValue in $OwnerValues) {
            $script:mockRegistry[
                (Get-TestSnapshotKey `
                    -Path ([string]$ownerValue.Path) `
                    -Name ([string]$ownerValue.Name))
            ] = Copy-TestSnapshot -Snapshot $ownerValue
        }
        Write-AfirmaProtocolSnapshot `
            -Path $snapshot118Path `
            -ProtocolKey $protocolKey `
            -OwnerValues $OwnerValues `
            -Snapshots $absent118
        $script:mockSetAttempts = 0
        $script:mockFailSetAt = 0
        $script:mockRestoreAttempts = 0
    }

    $legacySets = @(Get-AfirmaProtocolLegacyOwnerSets `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath)
    Assert-True ($legacySets.Count -eq 3) `
        "Faltan variantes de propietario anteriores (icono del ejecutable, grxfirma-diputacion.ico y grxfirma.ico)"
    foreach ($legacySet in $legacySets) {
        Assert-True (
            [string]$legacySet.OwnerValues[3].Value -eq "`"$executablePath`" `"%1`""
        ) "Una variante anterior no apunta al ejecutable de GrxFirma"
        Assert-True (
            ([string]$legacySet.OwnerValues[2].Value).StartsWith(
                "$installDir\",
                [System.StringComparison]::OrdinalIgnoreCase
            )
        ) "Una variante anterior usa un icono fuera de la carpeta de instalacion"
    }
    $commandChangingSet = [pscustomobject]@{
        OwnerValues = @($owners118 | ForEach-Object { Copy-TestSnapshot -Snapshot $_ })
    }
    $commandChangingSet.OwnerValues[3].Value = '"C:\Otro\otro.exe" "%1"'
    Assert-Throws {
        Assert-AfirmaProtocolLegacyOwnerSets `
            -Plan $brandedPlan `
            -OwnerSets @($commandChangingSet)
    } "Se acepto una variante anterior que cambia la orden del protocolo"

    # Comprobacion previa: acepta la 0.0.118 sin escribir nada.
    Reset-Test118State
    $before118 = [System.IO.File]::ReadAllBytes($snapshot118Path)
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath `
        -SnapshotPath $snapshot118Path `
        -ValidateOnly
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La comprobacion previa escribio en el registro"
    Assert-True (
        [Convert]::ToBase64String($before118) -eq
            [Convert]::ToBase64String([System.IO.File]::ReadAllBytes($snapshot118Path))
    ) "La comprobacion previa modifico la instantanea"

    # Actualizacion 0.0.118 -> actual: acepta el icono antiguo y deja
    # registro e instantanea con grxfirma-grx.ico.
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath `
        -SnapshotPath $snapshot118Path
    $upgraded118 = Read-AfirmaProtocolSnapshot `
        -Path $snapshot118Path `
        -ProtocolKey $protocolKey `
        -Plan $brandedPlan
    Assert-True (
        [string]$upgraded118.OwnerValues[2].Value -eq "$iconPath,0"
    ) "La instantanea actualizada no usa grxfirma-grx.ico"
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected @(
            $brandedPlan | ForEach-Object { New-AfirmaOwnedValueSnapshot -Plan $_ }
        )
    ) "El registro actualizado no apunta a grxfirma-grx.ico y al ejecutable"
    for ($index = 0; $index -lt $absent118.Count; $index++) {
        Assert-True (
            Test-AfirmaRegistrySnapshotsEqual `
                -Left $upgraded118.Snapshots[$index] `
                -Right $absent118[$index]
        ) "La actualizacion perdio el estado anterior a GrxFirma"
    }

    # Registro aun con el icono antiguo e instantanea ya nueva (actualizacion
    # interrumpida): sigue siendo propio.
    $script:mockRegistry[
        (Get-TestSnapshotKey -Path ([string]$owners118[2].Path) -Name "")
    ] = Copy-TestSnapshot -Snapshot $owners118[2]
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath `
        -SnapshotPath $snapshot118Path

    # Desinstalacion con la instantanea de la 0.0.118.
    Reset-Test118State
    Assert-Throws {
        Read-AfirmaProtocolSnapshot `
            -Path $snapshot118Path `
            -ProtocolKey $protocolKey `
            -Plan $brandedPlan
    } "Sin variantes anteriores se acepto la instantanea de la 0.0.118"
    $state118 = Read-AfirmaProtocolSnapshot `
        -Path $snapshot118Path `
        -ProtocolKey $protocolKey `
        -Plan $brandedPlan `
        -AcceptedLegacyOwnerSets $legacySets
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues @($state118.OwnerValues) `
        -Snapshots @($state118.Snapshots)
    Assert-True $restored "La desinstalacion no retiro el protocolo de la 0.0.118"
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected $absent118
    ) "La desinstalacion de la 0.0.118 dejo valores del protocolo"

    # Casos negativos: icono de otra carpeta, orden de otro ejecutable u
    # otro producto. Ni se aceptan ni se escribe nada.
    foreach ($foreign in @(
        @{ Index = 2; Value = "C:\Users\Test\AppData\Local\Programs\Otro\grxfirma-diputacion.ico,0"; Message = "icono de otra carpeta" },
        @{ Index = 2; Value = "C:\Users\Test\AppData\Local\Programs\GrxFirma\CLI\grxfirma-diputacion.ico,0"; Message = "icono de otro componente" },
        @{ Index = 3; Value = '"C:\Users\Test\AppData\Local\Programs\Otro\grxfirma-afirmauri.exe" "%1"'; Message = "orden de otra ruta" },
        @{ Index = 3; Value = '"C:\Program Files\AutoFirma\AutoFirma.exe" "%1"'; Message = "orden de otro producto" }
    )) {
        $foreignOwners = @($owners118 | ForEach-Object { Copy-TestSnapshot -Snapshot $_ })
        $foreignOwners[$foreign.Index].Value = $foreign.Value
        Reset-Test118State -OwnerValues $foreignOwners
        Assert-Throws {
            Install-AfirmaProtocolRegistration `
                -ProtocolKey $protocolKey `
                -ExecutablePath $executablePath `
                -IconPath $iconPath `
                -SnapshotPath $snapshot118Path `
                -ValidateOnly
        } "La comprobacion previa acepto $($foreign.Message)"
        Assert-Throws {
            Install-AfirmaProtocolRegistration `
                -ProtocolKey $protocolKey `
                -ExecutablePath $executablePath `
                -IconPath $iconPath `
                -SnapshotPath $snapshot118Path
        } "La actualizacion acepto $($foreign.Message)"
        Assert-True ($script:mockSetAttempts -eq 0) `
            "La actualizacion escribio registro con $($foreign.Message)"
        Assert-Throws {
            Read-AfirmaProtocolSnapshot `
                -Path $snapshot118Path `
                -ProtocolKey $protocolKey `
                -Plan $brandedPlan `
                -AcceptedLegacyOwnerSets $legacySets
        } "La desinstalacion acepto $($foreign.Message)"
    }

    # Instantanea legitima pero registro cambiado por otro programa.
    Reset-Test118State
    $hijacked = Copy-TestSnapshot -Snapshot $owners118[3]
    $hijacked.Value = '"C:\Otro\otro.exe" "%1"'
    $script:mockRegistry[
        (Get-TestSnapshotKey -Path ([string]$hijacked.Path) -Name "")
    ] = $hijacked
    Assert-Throws {
        Install-AfirmaProtocolRegistration `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -IconPath $iconPath `
            -SnapshotPath $snapshot118Path
    } "La actualizacion sobrescribio un protocolo cambiado por otro programa"
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La actualizacion escribio registro sobre otro programa"
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues @($owners118) `
        -Snapshots @($absent118)
    Assert-True (-not $restored) `
        "La desinstalacion retiro un protocolo cambiado por otro programa"

    # Sin instantanea: los valores de la 0.0.118 se reconocen como propios.
    Reset-Test118State
    Remove-Item -LiteralPath $snapshot118Path -Force
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath `
        -SnapshotPath $snapshot118Path
    $noSnapshotState = Read-AfirmaProtocolSnapshot `
        -Path $snapshot118Path `
        -ProtocolKey $protocolKey `
        -Plan $brandedPlan
    for ($index = 0; $index -lt $absent118.Count; $index++) {
        Assert-True (
            Test-AfirmaRegistrySnapshotsEqual `
                -Left $noSnapshotState.Snapshots[$index] `
                -Right $absent118[$index]
        ) "Sin instantanea, los valores de la 0.0.118 se guardaron como ajenos"
    }
    Remove-Item -LiteralPath $snapshot118Path -Force

    # Instantanea escrita por la 0.0.119 a la 0.0.121: DefaultIcon apuntaba a
    # grxfirma.ico en la misma carpeta. El icono cambio de imagen sin cambiar
    # de nombre y la cache de Windows seguia sirviendo la anterior; ahora se
    # llama grxfirma-grx.ico y la actualizacion debe aceptar el valor previo.
    $icon121Path = "$installDir\grxfirma.ico"
    $plan121 = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $icon121Path)
    $owners121 = @($plan121 | ForEach-Object {
        New-AfirmaOwnedValueSnapshot -Plan $_
    })
    $absent121 = @($plan121 | ForEach-Object {
        New-AfirmaAbsentValueSnapshot -Plan $_
    })
    $snapshot121Path = Join-Path $tmp "afirma-protocol-snapshot-0.0.121.json"
    function Reset-Test121State {
        $script:mockRegistry = @{}
        foreach ($ownerValue in $owners121) {
            $script:mockRegistry[
                (Get-TestSnapshotKey `
                    -Path ([string]$ownerValue.Path) `
                    -Name ([string]$ownerValue.Name))
            ] = Copy-TestSnapshot -Snapshot $ownerValue
        }
        Write-AfirmaProtocolSnapshot `
            -Path $snapshot121Path `
            -ProtocolKey $protocolKey `
            -OwnerValues $owners121 `
            -Snapshots $absent121
        $script:mockSetAttempts = 0
        $script:mockFailSetAt = 0
        $script:mockRestoreAttempts = 0
    }
    Assert-True (
        @($legacySets | Where-Object {
            [string]$_.OwnerValues[2].Value -eq "$icon121Path,0"
        }).Count -eq 1
    ) "grxfirma.ico no figura entre los iconos anteriores propios"

    # Comprobacion previa: acepta la 0.0.121 sin escribir nada.
    Reset-Test121State
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath `
        -SnapshotPath $snapshot121Path `
        -ValidateOnly
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La comprobacion previa de la 0.0.121 escribio en el registro"

    # Actualizacion 0.0.121 -> actual.
    Install-AfirmaProtocolRegistration `
        -ProtocolKey $protocolKey `
        -ExecutablePath $executablePath `
        -IconPath $iconPath `
        -SnapshotPath $snapshot121Path
    $upgraded121 = Read-AfirmaProtocolSnapshot `
        -Path $snapshot121Path `
        -ProtocolKey $protocolKey `
        -Plan $brandedPlan
    Assert-True (
        [string]$upgraded121.OwnerValues[2].Value -eq "$iconPath,0"
    ) "La instantanea actualizada desde la 0.0.121 no usa grxfirma-grx.ico"
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected @(
            $brandedPlan | ForEach-Object { New-AfirmaOwnedValueSnapshot -Plan $_ }
        )
    ) "El registro actualizado desde la 0.0.121 no apunta a grxfirma-grx.ico"
    for ($index = 0; $index -lt $absent121.Count; $index++) {
        Assert-True (
            Test-AfirmaRegistrySnapshotsEqual `
                -Left $upgraded121.Snapshots[$index] `
                -Right $absent121[$index]
        ) "La actualizacion desde la 0.0.121 perdio el estado anterior a GrxFirma"
    }

    # Desinstalacion con la instantanea de la 0.0.121.
    Reset-Test121State
    $state121 = Read-AfirmaProtocolSnapshot `
        -Path $snapshot121Path `
        -ProtocolKey $protocolKey `
        -Plan $brandedPlan `
        -AcceptedLegacyOwnerSets $legacySets
    $restored = Restore-AfirmaProtocolRegistration `
        -OwnerValues @($state121.OwnerValues) `
        -Snapshots @($state121.Snapshots)
    Assert-True $restored "La desinstalacion no retiro el protocolo de la 0.0.121"
    Assert-True (
        Test-AfirmaProtocolValuesMatch -Expected $absent121
    ) "La desinstalacion de la 0.0.121 dejo valores del protocolo"

    # Un grxfirma.ico de otra carpeta sigue siendo ajeno.
    Reset-Test121State
    $foreign121 = Copy-TestSnapshot -Snapshot $owners121[2]
    $foreign121.Value = "C:\Users\Test\AppData\Local\Programs\Otro\grxfirma.ico,0"
    $script:mockRegistry[
        (Get-TestSnapshotKey -Path ([string]$foreign121.Path) -Name "")
    ] = $foreign121
    Assert-Throws {
        Install-AfirmaProtocolRegistration `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -IconPath $iconPath `
            -SnapshotPath $snapshot121Path
    } "Se acepto un grxfirma.ico de otra carpeta como propio"
    Assert-True ($script:mockSetAttempts -eq 0) `
        "La actualizacion escribio registro con un grxfirma.ico ajeno"
    Remove-Item -LiteralPath $snapshot121Path -Force

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
