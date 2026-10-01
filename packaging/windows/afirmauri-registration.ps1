# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

function Get-AfirmaProtocolRegistrationPlan {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = ""
    )

    $resolvedIconPath = if ([string]::IsNullOrWhiteSpace($IconPath)) {
        $ExecutablePath
    } else {
        $IconPath
    }
    return @(
        [pscustomobject]@{
            Path = $ProtocolKey
            Name = ""
            Value = "URL:GrxFirma Protocol"
        },
        [pscustomobject]@{
            Path = $ProtocolKey
            Name = "URL Protocol"
            Value = ""
        },
        [pscustomobject]@{
            Path = "$ProtocolKey\DefaultIcon"
            Name = ""
            Value = "$resolvedIconPath,0"
        },
        [pscustomobject]@{
            Path = "$ProtocolKey\shell\open\command"
            Name = ""
            Value = "`"$ExecutablePath`" `"%1`""
        }
    )
}

function Get-AfirmaRegistryValueSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$Name
    )

    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($Path, $false)
    if ($null -eq $key) {
        return [pscustomobject]@{
            Path = $Path
            Name = $Name
            KeyExisted = $false
            ValueExisted = $false
            Kind = $null
            Value = $null
        }
    }
    try {
        $valueExisted = @($key.GetValueNames()) -contains $Name
        if (-not $valueExisted) {
            return [pscustomobject]@{
                Path = $Path
                Name = $Name
                KeyExisted = $true
                ValueExisted = $false
                Kind = $null
                Value = $null
            }
        }

        $kind = $key.GetValueKind($Name)
        $value = $key.GetValue(
            $Name,
            $null,
            [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
        )
        $encodedValue = switch ($kind) {
            ([Microsoft.Win32.RegistryValueKind]::Binary) {
                [Convert]::ToBase64String([byte[]]$value)
                break
            }
            ([Microsoft.Win32.RegistryValueKind]::None) {
                [Convert]::ToBase64String([byte[]]$value)
                break
            }
            ([Microsoft.Win32.RegistryValueKind]::MultiString) {
                @([string[]]$value)
                break
            }
            ([Microsoft.Win32.RegistryValueKind]::DWord) {
                ([uint32]$value).ToString(
                    [System.Globalization.CultureInfo]::InvariantCulture
                )
                break
            }
            ([Microsoft.Win32.RegistryValueKind]::QWord) {
                ([uint64]$value).ToString(
                    [System.Globalization.CultureInfo]::InvariantCulture
                )
                break
            }
            default {
                [string]$value
                break
            }
        }

        return [pscustomobject]@{
            Path = $Path
            Name = $Name
            KeyExisted = $true
            ValueExisted = $true
            Kind = $kind.ToString()
            Value = $encodedValue
        }
    } finally {
        $key.Dispose()
    }
}

function New-AfirmaOwnedValueSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Plan
    )

    return [pscustomobject]@{
        Path = [string]$Plan.Path
        Name = [string]$Plan.Name
        KeyExisted = $true
        ValueExisted = $true
        Kind = [Microsoft.Win32.RegistryValueKind]::String.ToString()
        Value = [string]$Plan.Value
    }
}

function New-AfirmaAbsentValueSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Plan
    )

    return [pscustomobject]@{
        Path = [string]$Plan.Path
        Name = [string]$Plan.Name
        KeyExisted = $false
        ValueExisted = $false
        Kind = $null
        Value = $null
    }
}

function Test-AfirmaRegistrySnapshotsEqual {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Left,
        [Parameter(Mandatory = $true)]
        [object]$Right
    )

    if (-not [string]::Equals(
            [string]$Left.Path,
            [string]$Right.Path,
            [System.StringComparison]::OrdinalIgnoreCase
        ) -or
        -not [string]::Equals(
            [string]$Left.Name,
            [string]$Right.Name,
            [System.StringComparison]::Ordinal
        ) -or
        [bool]$Left.ValueExisted -ne [bool]$Right.ValueExisted) {
        return $false
    }
    if (-not [bool]$Left.ValueExisted) {
        return $true
    }
    if (-not [string]::Equals(
            [string]$Left.Kind,
            [string]$Right.Kind,
            [System.StringComparison]::Ordinal
        )) {
        return $false
    }

    if ([string]$Left.Kind -eq
        [Microsoft.Win32.RegistryValueKind]::MultiString.ToString()) {
        $leftValues = @($Left.Value)
        $rightValues = @($Right.Value)
        if ($leftValues.Count -ne $rightValues.Count) {
            return $false
        }
        for ($index = 0; $index -lt $leftValues.Count; $index++) {
            if (-not [string]::Equals(
                    [string]$leftValues[$index],
                    [string]$rightValues[$index],
                    [System.StringComparison]::Ordinal
                )) {
                return $false
            }
        }
        return $true
    }

    $comparison = [System.StringComparison]::Ordinal
    if ([string]$Left.Kind -eq
        [Microsoft.Win32.RegistryValueKind]::String.ToString()) {
        $comparison = [System.StringComparison]::OrdinalIgnoreCase
    }
    return [string]::Equals(
        [string]$Left.Value,
        [string]$Right.Value,
        $comparison
    )
}

function ConvertFrom-AfirmaSnapshotValue {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Snapshot
    )

    $kind = [Microsoft.Win32.RegistryValueKind][System.Enum]::Parse(
        [Microsoft.Win32.RegistryValueKind],
        [string]$Snapshot.Kind,
        $false
    )
    $value = switch ($kind) {
        ([Microsoft.Win32.RegistryValueKind]::Binary) {
            [Convert]::FromBase64String([string]$Snapshot.Value)
            break
        }
        ([Microsoft.Win32.RegistryValueKind]::None) {
            [Convert]::FromBase64String([string]$Snapshot.Value)
            break
        }
        ([Microsoft.Win32.RegistryValueKind]::MultiString) {
            [string[]]@($Snapshot.Value)
            break
        }
        ([Microsoft.Win32.RegistryValueKind]::DWord) {
            [uint32]::Parse(
                [string]$Snapshot.Value,
                [System.Globalization.CultureInfo]::InvariantCulture
            )
            break
        }
        ([Microsoft.Win32.RegistryValueKind]::QWord) {
            [uint64]::Parse(
                [string]$Snapshot.Value,
                [System.Globalization.CultureInfo]::InvariantCulture
            )
            break
        }
        default {
            [string]$Snapshot.Value
            break
        }
    }
    return [pscustomobject]@{
        Kind = $kind
        Value = $value
    }
}

function Set-AfirmaRegistryString {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$Value
    )

    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($Path)
    if ($null -eq $key) {
        throw "No se pudo crear la clave de registro HKCU:\$Path"
    }
    try {
        $key.SetValue(
            $Name,
            $Value,
            [Microsoft.Win32.RegistryValueKind]::String
        )
    } finally {
        $key.Dispose()
    }
}

function Restore-AfirmaRegistryValueSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Snapshot,
        [Parameter(Mandatory = $true)]
        [object]$ExpectedCurrentSnapshot
    )

    $current = Get-AfirmaRegistryValueSnapshot `
        -Path ([string]$Snapshot.Path) `
        -Name ([string]$Snapshot.Name)
    if (Test-AfirmaRegistrySnapshotsEqual -Left $current -Right $Snapshot) {
        return
    }
    if (-not (Test-AfirmaRegistrySnapshotsEqual `
            -Left $current `
            -Right $ExpectedCurrentSnapshot)) {
        throw "El registro afirma:// cambio durante la operacion: HKCU:\$($Snapshot.Path)"
    }

    if ([bool]$Snapshot.ValueExisted) {
        $decoded = ConvertFrom-AfirmaSnapshotValue -Snapshot $Snapshot
        $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey(
            [string]$Snapshot.Path
        )
        if ($null -eq $key) {
            throw "No se pudo restaurar la clave de registro HKCU:\$($Snapshot.Path)"
        }
        try {
            $key.SetValue(
                [string]$Snapshot.Name,
                $decoded.Value,
                $decoded.Kind
            )
        } finally {
            $key.Dispose()
        }
        return
    }

    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey(
        [string]$Snapshot.Path,
        $true
    )
    if ($null -ne $key) {
        try {
            $key.DeleteValue([string]$Snapshot.Name, $false)
        } finally {
            $key.Dispose()
        }
    }
}

function Test-AfirmaRegistryTreeHasValues {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($Path, $false)
    if ($null -eq $key) {
        return $false
    }
    try {
        if ($key.GetValueNames().Count -gt 0) {
            return $true
        }
        $subKeyNames = @($key.GetSubKeyNames())
    } finally {
        $key.Dispose()
    }
    foreach ($subKeyName in $subKeyNames) {
        if (Test-AfirmaRegistryTreeHasValues -Path "$Path\$subKeyName") {
            return $true
        }
    }
    return $false
}

function Remove-AfirmaEmptySnapshotKeys {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$Snapshots
    )

    $paths = @(
        $Snapshots |
            Where-Object { -not [bool]$_.KeyExisted } |
            ForEach-Object { [string]$_.Path } |
            Sort-Object -Property Length -Descending -Unique
    )
    foreach ($path in $paths) {
        $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($path, $false)
        if ($null -eq $key) {
            continue
        }
        try {
            $empty =
                $key.GetValueNames().Count -eq 0 -and
                $key.GetSubKeyNames().Count -eq 0
        } finally {
            $key.Dispose()
        }
        if ($empty) {
            [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKey($path, $false)
        }
    }

    $rootSnapshot = $Snapshots |
        Sort-Object { ([string]$_.Path).Length } |
        Select-Object -First 1
    if ($null -ne $rootSnapshot -and
        (-not [bool]$rootSnapshot.KeyExisted) -and
        (-not (Test-AfirmaRegistryTreeHasValues `
            -Path ([string]$rootSnapshot.Path)))) {
        [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree(
            [string]$rootSnapshot.Path,
            $false
        )
    }
}

function Write-AfirmaProtocolSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [object[]]$OwnerValues,
        [Parameter(Mandatory = $true)]
        [object[]]$Snapshots
    )

    $document = [ordered]@{
        schema_version = 1
        protocol_key = $ProtocolKey
        owner_values = @($OwnerValues)
        snapshots = @($Snapshots)
    } | ConvertTo-Json -Depth 8
    $temporaryPath = "$Path.tmp-$([guid]::NewGuid().ToString('N'))"
    try {
        [System.IO.File]::WriteAllText(
            $temporaryPath,
            $document + [Environment]::NewLine,
            [System.Text.UTF8Encoding]::new($false)
        )
        Move-Item `
            -LiteralPath $temporaryPath `
            -Destination $Path `
            -Force `
            -ErrorAction Stop
    } finally {
        if (Test-Path -LiteralPath $temporaryPath) {
            Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue
        }
    }
}

function Read-AfirmaProtocolSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [object[]]$Plan,
        [object[]]$AcceptedLegacyOwnerValues = @()
    )

    $legacyOwners = @($AcceptedLegacyOwnerValues)
    if ($legacyOwners.Count -ne 0 -and
        $legacyOwners.Count -ne $Plan.Count) {
        throw "El plan de migracion del protocolo afirma:// esta incompleto."
    }

    $file = Get-Item -LiteralPath $Path -ErrorAction Stop
    if ($file.Length -gt 1048576) {
        throw "La instantanea del protocolo afirma:// supera el limite permitido."
    }
    try {
        $state = Get-Content -LiteralPath $Path -Raw -ErrorAction Stop |
            ConvertFrom-Json -ErrorAction Stop
    } catch {
        throw "La instantanea del protocolo afirma:// no es JSON valido: $($_.Exception.Message)"
    }
    if ($state.schema_version -ne 1 -or
        -not [string]::Equals(
            [string]$state.protocol_key,
            $ProtocolKey,
            [System.StringComparison]::OrdinalIgnoreCase
        )) {
        throw "La instantanea del protocolo afirma:// tiene un esquema o destino no valido."
    }

    $owners = @($state.owner_values)
    $snapshots = @($state.snapshots)
    if ($owners.Count -ne $Plan.Count -or $snapshots.Count -ne $Plan.Count) {
        throw "La instantanea del protocolo afirma:// esta incompleta."
    }
    $matchesCurrentOwner = $true
    $matchesLegacyOwner = $legacyOwners.Count -eq $Plan.Count
    for ($index = 0; $index -lt $Plan.Count; $index++) {
        foreach ($record in @($owners[$index], $snapshots[$index])) {
            if (-not [string]::Equals(
                    [string]$record.Path,
                    [string]$Plan[$index].Path,
                    [System.StringComparison]::OrdinalIgnoreCase
                ) -or
                -not [string]::Equals(
                    [string]$record.Name,
                    [string]$Plan[$index].Name,
                    [System.StringComparison]::Ordinal
                )) {
                throw "La instantanea del protocolo afirma:// contiene una ruta no permitida."
            }
        }
        if (-not [bool]$owners[$index].ValueExisted -or
            [string]$owners[$index].Kind -ne
                [Microsoft.Win32.RegistryValueKind]::String.ToString()) {
            throw "La instantanea del protocolo afirma:// contiene un marcador de propiedad no valido."
        }
        $expectedOwner = New-AfirmaOwnedValueSnapshot -Plan $Plan[$index]
        if (-not (Test-AfirmaRegistrySnapshotsEqual `
                -Left $owners[$index] `
                -Right $expectedOwner)) {
            $matchesCurrentOwner = $false
        }
        if ($matchesLegacyOwner -and
            -not (Test-AfirmaRegistrySnapshotsEqual `
                -Left $owners[$index] `
                -Right $legacyOwners[$index])) {
            $matchesLegacyOwner = $false
        }
        if ([bool]$snapshots[$index].ValueExisted) {
            try {
                ConvertFrom-AfirmaSnapshotValue -Snapshot $snapshots[$index] |
                    Out-Null
            } catch {
                throw "La instantanea del protocolo afirma:// contiene un valor no valido."
            }
        }
    }
    if (-not $matchesCurrentOwner -and -not $matchesLegacyOwner) {
        throw "La instantanea del protocolo afirma:// no pertenece a este ejecutable."
    }
    return [pscustomobject]@{
        OwnerValues = $owners
        Snapshots = $snapshots
    }
}

function Test-AfirmaProtocolValuesMatch {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$Expected
    )

    foreach ($expectedValue in $Expected) {
        $current = Get-AfirmaRegistryValueSnapshot `
            -Path ([string]$expectedValue.Path) `
            -Name ([string]$expectedValue.Name)
        if (-not (Test-AfirmaRegistrySnapshotsEqual `
                -Left $current `
                -Right $expectedValue)) {
            return $false
        }
    }
    return $true
}

function Install-AfirmaProtocolRegistration {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = "",
        [Parameter(Mandatory = $true)]
        [string]$SnapshotPath
    )

    $plan = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath)
    $ownerValues = @($plan | ForEach-Object {
        New-AfirmaOwnedValueSnapshot -Plan $_
    })
    $legacyPlan = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath)
    $legacyOwnerValues = @($legacyPlan | ForEach-Object {
        New-AfirmaOwnedValueSnapshot -Plan $_
    })
    $acceptedLegacyOwnerValues = @()
    if (-not (Test-AfirmaRegistrySnapshotsEqual `
            -Left $ownerValues[2] `
            -Right $legacyOwnerValues[2])) {
        $acceptedLegacyOwnerValues = $legacyOwnerValues
    }
    $currentValues = @($plan | ForEach-Object {
        Get-AfirmaRegistryValueSnapshot `
            -Path ([string]$_.Path) `
            -Name ([string]$_.Name)
    })

    $snapshotExisted = Test-Path -LiteralPath $SnapshotPath -PathType Leaf
    $snapshotBytes = $null
    if ($snapshotExisted) {
        $snapshotBytes = [System.IO.File]::ReadAllBytes($SnapshotPath)
        $state = Read-AfirmaProtocolSnapshot `
            -Path $SnapshotPath `
            -ProtocolKey $ProtocolKey `
            -Plan $plan `
            -AcceptedLegacyOwnerValues $acceptedLegacyOwnerValues
        for ($index = 0; $index -lt $plan.Count; $index++) {
            $knownOwner = Test-AfirmaRegistrySnapshotsEqual `
                -Left $currentValues[$index] `
                -Right $state.OwnerValues[$index]
            $knownPrevious = Test-AfirmaRegistrySnapshotsEqual `
                -Left $currentValues[$index] `
                -Right $state.Snapshots[$index]
            if (-not $knownOwner -and -not $knownPrevious) {
                throw "El protocolo afirma:// cambio desde la instalacion anterior; se aborta para no sobrescribir otro handler."
            }
        }
        $originalValues = @($state.Snapshots)
    } else {
        $legacyOwned = $true
        for ($index = 0; $index -lt $plan.Count; $index++) {
            if (-not (Test-AfirmaRegistrySnapshotsEqual `
                    -Left $currentValues[$index] `
                    -Right $ownerValues[$index])) {
                $legacyOwned = $false
                break
            }
        }
        if ($legacyOwned) {
            $originalValues = @($plan | ForEach-Object {
                New-AfirmaAbsentValueSnapshot -Plan $_
            })
        } else {
            $originalValues = @($currentValues)
        }
    }

    Write-AfirmaProtocolSnapshot `
        -Path $SnapshotPath `
        -ProtocolKey $ProtocolKey `
        -OwnerValues $ownerValues `
        -Snapshots $originalValues

    $attempted = 0
    try {
        foreach ($entry in $plan) {
            $attempted++
            Set-AfirmaRegistryString `
                -Path ([string]$entry.Path) `
                -Name ([string]$entry.Name) `
                -Value ([string]$entry.Value)
        }
    } catch {
        $registrationError = $_.Exception
        $rollbackErrors = @()
        for ($index = $attempted - 1; $index -ge 0; $index--) {
            try {
                Restore-AfirmaRegistryValueSnapshot `
                    -Snapshot $currentValues[$index] `
                    -ExpectedCurrentSnapshot $ownerValues[$index]
            } catch {
                $rollbackErrors += $_.Exception.Message
            }
        }
        try {
            Remove-AfirmaEmptySnapshotKeys -Snapshots $currentValues
        } catch {
            $rollbackErrors += $_.Exception.Message
        }
        try {
            if ($snapshotExisted) {
                [System.IO.File]::WriteAllBytes($SnapshotPath, $snapshotBytes)
            } else {
                Remove-Item `
                    -LiteralPath $SnapshotPath `
                    -Force `
                    -ErrorAction SilentlyContinue
            }
        } catch {
            $rollbackErrors += $_.Exception.Message
        }
        if ($rollbackErrors.Count -gt 0) {
            throw "Fallo registrando afirma://: $($registrationError.Message). Rollback incompleto: $($rollbackErrors -join '; ')"
        }
        throw $registrationError
    }
}

function Restore-AfirmaProtocolRegistration {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$OwnerValues,
        [Parameter(Mandatory = $true)]
        [object[]]$Snapshots
    )

    if (-not (Test-AfirmaProtocolValuesMatch -Expected $OwnerValues)) {
        return $false
    }

    $attempted = 0
    try {
        for ($index = 0; $index -lt $Snapshots.Count; $index++) {
            $attempted++
            Restore-AfirmaRegistryValueSnapshot `
                -Snapshot $Snapshots[$index] `
                -ExpectedCurrentSnapshot $OwnerValues[$index]
        }
        Remove-AfirmaEmptySnapshotKeys -Snapshots $Snapshots
    } catch {
        $restoreError = $_.Exception
        $rollbackErrors = @()
        for ($index = $attempted - 1; $index -ge 0; $index--) {
            try {
                Restore-AfirmaRegistryValueSnapshot `
                    -Snapshot $OwnerValues[$index] `
                    -ExpectedCurrentSnapshot $Snapshots[$index]
            } catch {
                $rollbackErrors += $_.Exception.Message
            }
        }
        if ($rollbackErrors.Count -gt 0) {
            throw "Fallo restaurando el handler anterior: $($restoreError.Message). Rollback incompleto: $($rollbackErrors -join '; ')"
        }
        throw $restoreError
    }
    return $true
}
