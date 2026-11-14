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

# Nombres de icono que versiones publicadas anteriores copiaron junto al
# ejecutable y registraron en DefaultIcon. Hasta la 0.0.118 era
# grxfirma-diputacion.ico y de la 0.0.119 a la 0.0.121, grxfirma.ico (el
# nombre cambió para que la caché de iconos de Windows no sirva la imagen
# anterior); las versiones sin icono propio usaban el propio ejecutable, que
# se acepta siempre.
$script:AfirmaLegacyIconFileNames = @(
    "grxfirma-diputacion.ico",
    "grxfirma.ico"
)

function New-AfirmaProtocolOwnerSet {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$Plan
    )

    return [pscustomobject]@{
        OwnerValues = @($Plan | ForEach-Object {
            New-AfirmaOwnedValueSnapshot -Plan $_
        })
    }
}

function Get-AfirmaProtocolLegacyOwnerSets {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = ""
    )

    # Solo cambia DefaultIcon y siempre dentro de la carpeta del ejecutable
    # actual. La orden shell\open\command sigue siendo la del ejecutable
    # instalado, así que no se acepta ningún valor de otra ruta o producto.
    $separatorIndex = $ExecutablePath.LastIndexOfAny([char[]]@('\', '/'))
    if ($separatorIndex -le 0) {
        throw "La ruta del ejecutable de afirma:// no es absoluta."
    }
    $installDir = $ExecutablePath.Substring(0, $separatorIndex)
    $candidateIcons = @("") + @(
        $script:AfirmaLegacyIconFileNames | ForEach-Object {
            "$installDir\$_"
        }
    )

    $currentSet = New-AfirmaProtocolOwnerSet -Plan @(
        Get-AfirmaProtocolRegistrationPlan `
            -ProtocolKey $ProtocolKey `
            -ExecutablePath $ExecutablePath `
            -IconPath $IconPath
    )
    $sets = @()
    foreach ($candidateIcon in $candidateIcons) {
        $candidateSet = New-AfirmaProtocolOwnerSet -Plan @(
            Get-AfirmaProtocolRegistrationPlan `
                -ProtocolKey $ProtocolKey `
                -ExecutablePath $ExecutablePath `
                -IconPath $candidateIcon
        )
        if (Test-AfirmaRegistrySnapshotsEqual `
                -Left $candidateSet.OwnerValues[2] `
                -Right $currentSet.OwnerValues[2]) {
            continue
        }
        $sets += $candidateSet
    }
    return $sets
}

function Assert-AfirmaProtocolLegacyOwnerSets {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$Plan,
        [object[]]$OwnerSets = @()
    )

    foreach ($ownerSet in @($OwnerSets)) {
        $owners = @($ownerSet.OwnerValues)
        if ($owners.Count -ne $Plan.Count) {
            throw "El plan de migracion del protocolo afirma:// esta incompleto."
        }
        for ($index = 0; $index -lt $Plan.Count; $index++) {
            $expected = New-AfirmaOwnedValueSnapshot -Plan $Plan[$index]
            if (-not [string]::Equals(
                    [string]$owners[$index].Path,
                    [string]$expected.Path,
                    [System.StringComparison]::OrdinalIgnoreCase
                ) -or
                -not [string]::Equals(
                    [string]$owners[$index].Name,
                    [string]$expected.Name,
                    [System.StringComparison]::Ordinal
                ) -or
                -not [bool]$owners[$index].ValueExisted -or
                [string]$owners[$index].Kind -ne
                    [Microsoft.Win32.RegistryValueKind]::String.ToString()) {
                throw "El plan de migracion del protocolo afirma:// no es valido."
            }
            # Solo DefaultIcon puede diferir del plan actual.
            if ($index -ne 2 -and
                -not (Test-AfirmaRegistrySnapshotsEqual `
                    -Left $owners[$index] `
                    -Right $expected)) {
                throw "El plan de migracion del protocolo afirma:// cambia valores distintos del icono."
            }
        }
    }
}

function Test-AfirmaOwnerValuesMatchSet {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$Values,
        [Parameter(Mandatory = $true)]
        [object[]]$OwnerValues
    )

    if ($Values.Count -ne $OwnerValues.Count) {
        return $false
    }
    for ($index = 0; $index -lt $Values.Count; $index++) {
        if (-not (Test-AfirmaRegistrySnapshotsEqual `
                -Left $Values[$index] `
                -Right $OwnerValues[$index])) {
            return $false
        }
    }
    return $true
}

function Find-AfirmaMatchingOwnerSet {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$Values,
        [Parameter(Mandatory = $true)]
        [object[]]$OwnerSets
    )

    foreach ($ownerSet in $OwnerSets) {
        if (Test-AfirmaOwnerValuesMatchSet `
                -Values $Values `
                -OwnerValues @($ownerSet.OwnerValues)) {
            return $ownerSet
        }
    }
    return $null
}

function Read-AfirmaProtocolSnapshot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [object[]]$Plan,
        [object[]]$AcceptedLegacyOwnerSets = @()
    )

    $legacySets = @($AcceptedLegacyOwnerSets)
    Assert-AfirmaProtocolLegacyOwnerSets -Plan $Plan -OwnerSets $legacySets

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
        if ([bool]$snapshots[$index].ValueExisted) {
            try {
                ConvertFrom-AfirmaSnapshotValue -Snapshot $snapshots[$index] |
                    Out-Null
            } catch {
                throw "La instantanea del protocolo afirma:// contiene un valor no valido."
            }
        }
    }

    $acceptedSets = @(New-AfirmaProtocolOwnerSet -Plan $Plan) + $legacySets
    if ($null -eq (Find-AfirmaMatchingOwnerSet `
            -Values $owners `
            -OwnerSets $acceptedSets)) {
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
        [string]$SnapshotPath,
        # Comprueba la propiedad sin escribir registro ni instantánea.
        [switch]$ValidateOnly
    )

    $plan = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath)
    $currentSet = New-AfirmaProtocolOwnerSet -Plan $plan
    $ownerValues = @($currentSet.OwnerValues)
    $legacySets = @(Get-AfirmaProtocolLegacyOwnerSets `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath)
    $acceptedSets = @($currentSet) + $legacySets
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
            -AcceptedLegacyOwnerSets $legacySets
        for ($index = 0; $index -lt $plan.Count; $index++) {
            $knownOwner = Test-AfirmaRegistrySnapshotsEqual `
                -Left $currentValues[$index] `
                -Right $state.OwnerValues[$index]
            if (-not $knownOwner) {
                # Un valor que escribió esta misma instalación en otra
                # versión (por ejemplo, el icono anterior) sigue siendo propio.
                foreach ($acceptedSet in $acceptedSets) {
                    if (Test-AfirmaRegistrySnapshotsEqual `
                            -Left $currentValues[$index] `
                            -Right $acceptedSet.OwnerValues[$index]) {
                        $knownOwner = $true
                        break
                    }
                }
            }
            $knownPrevious = Test-AfirmaRegistrySnapshotsEqual `
                -Left $currentValues[$index] `
                -Right $state.Snapshots[$index]
            if (-not $knownOwner -and -not $knownPrevious) {
                throw "El protocolo afirma:// cambio desde la instalacion anterior; se aborta para no sobrescribir otro handler."
            }
        }
        $originalValues = @($state.Snapshots)
    } else {
        $ownedSet = Find-AfirmaMatchingOwnerSet `
            -Values $currentValues `
            -OwnerSets $acceptedSets
        if ($null -ne $ownedSet) {
            $originalValues = @($plan | ForEach-Object {
                New-AfirmaAbsentValueSnapshot -Plan $_
            })
        } else {
            $originalValues = @($currentValues)
        }
    }

    if ($ValidateOnly) {
        return
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

# Elección del programa que atiende afirma:// (Configuración de GrxFirma).
# La aplicación la guarda en HKCU\Software\GrxFirma; el instalador solo la
# lee para no volver a quitarle el protocolo a AutoFirma al actualizar. La
# misma lógica, sin PowerShell, está en el motor
# (internal/adapters/outbound/desktop/afirmahandler).
$script:AfirmaPreferenceKey = "Software\GrxFirma"
$script:AfirmaPreferenceValue = "AfirmaProtocolHandler"

function Get-AfirmaProtocolHandlerPreference {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey(
        $script:AfirmaPreferenceKey,
        $false
    )
    if ($null -eq $key) {
        return ""
    }
    try {
        if (-not (@($key.GetValueNames()) -contains $script:AfirmaPreferenceValue) -or
            $key.GetValueKind($script:AfirmaPreferenceValue) -ne
                [Microsoft.Win32.RegistryValueKind]::String) {
            return ""
        }
        $value = ([string]$key.GetValue($script:AfirmaPreferenceValue)).Trim().ToLowerInvariant()
    } finally {
        $key.Dispose()
    }
    if ($script:AfirmaHandlerChoices -contains $value) {
        return $value
    }
    return ""
}

# Valores de la preferencia y de la política manejador_afirma. «preguntar»
# deja afirma:// a GrxFirma, que pregunta en cada firma (lote 15).
$script:AfirmaHandlerChoices = @("grxfirma", "autofirma", "preguntar")

function Set-AfirmaProtocolHandlerPreference {
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet("grxfirma", "autofirma", "preguntar")]
        [string]$Handler
    )

    Set-AfirmaRegistryString `
        -Path $script:AfirmaPreferenceKey `
        -Name $script:AfirmaPreferenceValue `
        -Value $Handler
}

function Get-AfirmaCommandExecutable {
    param(
        [AllowEmptyString()]
        [string]$Command
    )

    $text = ([string]$Command).Trim()
    if ($text.Length -eq 0) {
        return ""
    }
    if ($text.StartsWith('"')) {
        $end = $text.IndexOf('"', 1)
        if ($end -lt 0) {
            return ""
        }
        return $text.Substring(1, $end - 1)
    }
    $from = 0
    while ($true) {
        $index = $text.IndexOf(".exe", $from, [System.StringComparison]::OrdinalIgnoreCase)
        if ($index -lt 0) {
            break
        }
        $end = $index + 4
        if ($end -eq $text.Length -or $text[$end] -eq ' ' -or $text[$end] -eq "`t") {
            return $text.Substring(0, $end)
        }
        $from = $end
    }
    return ($text -split '\s+')[0]
}

# Devuelve el ejecutable de AutoFirma registrado para el equipo o "".
function Get-AfirmaAutoFirmaExecutable {
    param(
        [string]$ProtocolKey = "Software\Classes\afirma"
    )

    $key = [Microsoft.Win32.Registry]::LocalMachine.OpenSubKey(
        "$ProtocolKey\shell\open\command",
        $false
    )
    if ($null -eq $key) {
        return ""
    }
    try {
        $command = [string]$key.GetValue("", "")
    } finally {
        $key.Dispose()
    }
    return (Get-AfirmaAutoFirmaExecutableFromCommand -Command $command)
}

# Verdadero si la ruta existe y es un fichero (se sustituye en las pruebas).
function Test-AfirmaAutoFirmaExecutablePath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    return (Test-Path -LiteralPath $Path -PathType Leaf)
}

# Devuelve el Autofirma.exe de una orden de afirma:// o "".
function Get-AfirmaAutoFirmaExecutableFromCommand {
    param(
        [AllowEmptyString()]
        [string]$Command
    )

    $executable = Get-AfirmaCommandExecutable -Command $Command
    if ($executable -notmatch '^[A-Za-z]:[\\/]' -or
        -not [string]::Equals(
            @($executable -split '[\\/]')[-1],
            "Autofirma.exe",
            [System.StringComparison]::OrdinalIgnoreCase
        ) -or
        -not (Test-AfirmaAutoFirmaExecutablePath -Path $executable)) {
        return ""
    }
    return $executable
}

# Devuelve el Autofirma.exe que atendía afirma:// en HKCU antes de GrxFirma
# (la orden guardada en la instantánea) o "". AutoFirma no siempre queda en
# HKLM: si se instaló solo para el usuario, o con la misma cuenta después de
# GrxFirma, Windows guarda sus valores en HKCU\Software\Classes.
function Get-AfirmaSavedAutoFirmaExecutable {
    param(
        [object[]]$Snapshots = @()
    )

    foreach ($snapshot in @($Snapshots)) {
        if ($null -eq $snapshot -or
            -not [bool]$snapshot.ValueExisted -or
            [string]$snapshot.Name -ne "" -or
            -not ([string]$snapshot.Path).EndsWith(
                "\shell\open\command",
                [System.StringComparison]::OrdinalIgnoreCase
            ) -or
            ([string]$snapshot.Kind -ne "String" -and [string]$snapshot.Kind -ne "ExpandString")) {
            continue
        }
        $executable = Get-AfirmaAutoFirmaExecutableFromCommand -Command ([string]$snapshot.Value)
        if (-not [string]::IsNullOrEmpty($executable)) {
            return $executable
        }
    }
    return ""
}

# Verdadero si la persona eligió AutoFirma, AutoFirma sigue instalado y el
# registro HKCU de afirma:// es exactamente el de antes de GrxFirma (es decir,
# GrxFirma lo retiró y nadie lo ha cambiado después). Ante cualquier duda
# devuelve falso y el instalador sigue su camino normal, con sus comprobaciones
# de propiedad.
function Test-AfirmaProtocolKeptForAutoFirma {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = "",
        [Parameter(Mandatory = $true)]
        [string]$SnapshotPath
    )

    if ((Get-AfirmaProtocolHandlerPreference) -ne "autofirma") {
        return $false
    }
    return (Test-AfirmaProtocolRetiredForAutoFirma `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath `
        -SnapshotPath $SnapshotPath)
}

# Verdadero si GrxFirma ya devolvió afirma:// a AutoFirma: AutoFirma sigue
# instalado y el registro HKCU es exactamente el de antes de GrxFirma.
function Test-AfirmaProtocolRetiredForAutoFirma {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = "",
        [Parameter(Mandatory = $true)]
        [string]$SnapshotPath
    )

    if (-not (Test-Path -LiteralPath $SnapshotPath -PathType Leaf)) {
        return $false
    }
    $plan = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath)
    try {
        $state = Read-AfirmaProtocolSnapshot `
            -Path $SnapshotPath `
            -ProtocolKey $ProtocolKey `
            -Plan $plan `
            -AcceptedLegacyOwnerSets @(Get-AfirmaProtocolLegacyOwnerSets `
                -ProtocolKey $ProtocolKey `
                -ExecutablePath $ExecutablePath `
                -IconPath $IconPath)
    } catch {
        return $false
    }
    if ([string]::IsNullOrEmpty((Get-AfirmaAutoFirmaExecutable -ProtocolKey $ProtocolKey)) -and
        [string]::IsNullOrEmpty((Get-AfirmaSavedAutoFirmaExecutable -Snapshots @($state.Snapshots)))) {
        return $false
    }
    return (Test-AfirmaProtocolValuesMatch -Expected @($state.Snapshots))
}

# Política de máquina manejador_afirma (HKLM\SOFTWARE\Policies\GrxFirma, vista
# de 64 bits, como la lee GrxFirma). Value es "" si no fija nada. Si existe pero
# no se puede leer o no se entiende, Unreadable: se mantiene lo de hoy.
function Get-AfirmaMachinePolicyHandler {
    $result = [pscustomobject]@{ Value = ""; Unreadable = $false }
    try {
        $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey(
            [Microsoft.Win32.RegistryHive]::LocalMachine,
            [Microsoft.Win32.RegistryView]::Registry64
        )
        try {
            $key = $base.OpenSubKey("SOFTWARE\Policies\GrxFirma", $false)
            if ($null -eq $key) {
                return $result
            }
            try {
                if (-not (@($key.GetValueNames()) -contains "manejador_afirma")) {
                    return $result
                }
                $kind = $key.GetValueKind("manejador_afirma")
                if ($kind -ne [Microsoft.Win32.RegistryValueKind]::String -and
                    $kind -ne [Microsoft.Win32.RegistryValueKind]::ExpandString) {
                    $result.Unreadable = $true
                    return $result
                }
                $value = ([string]$key.GetValue("manejador_afirma")).Trim().ToLowerInvariant()
            } finally {
                $key.Dispose()
            }
        } finally {
            $base.Dispose()
        }
    } catch {
        $result.Unreadable = $true
        return $result
    }
    if ($script:AfirmaHandlerChoices -contains $value) {
        $result.Value = $value
    } else {
        $result.Unreadable = $true
    }
    return $result
}

# Qué programa debe atender afirma:// tras instalar: el que fija la política;
# si no, el que pide el instalador (/AFIRMA=); si no, el que eligió la persona;
# y si no eligió nada, GrxFirma, como hasta ahora.
function Resolve-AfirmaInstallHandler {
    param(
        [AllowEmptyString()]
        [string]$Requested = "",
        [AllowEmptyString()]
        [string]$Preference = "",
        [object]$Policy = $null
    )

    if ($null -ne $Policy -and -not [bool]$Policy.Unreadable -and
        $script:AfirmaHandlerChoices -contains [string]$Policy.Value) {
        return [pscustomobject]@{ Handler = [string]$Policy.Value; Source = "politica" }
    }
    $requestedValue = ([string]$Requested).Trim().ToLowerInvariant()
    if ($requestedValue.Length -gt 0) {
        if (-not ($script:AfirmaHandlerChoices -contains $requestedValue)) {
            throw "Valor no válido para el programa de firma en las webs: '$Requested'. Use grxfirma, autofirma o preguntar."
        }
        return [pscustomobject]@{ Handler = $requestedValue; Source = "parametro" }
    }
    if ($script:AfirmaHandlerChoices -contains $Preference) {
        return [pscustomobject]@{ Handler = $Preference; Source = "preferencia" }
    }
    return [pscustomobject]@{ Handler = "grxfirma"; Source = "defecto" }
}

# Verdadero si la orden actual de afirma:// en HKCU es la de AutoFirma (por
# ejemplo, AutoFirma se instaló con la misma cuenta después de GrxFirma).
function Test-AfirmaAutoFirmaIsCurrent {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey
    )

    $command = Get-AfirmaRegistryValueSnapshot `
        -Path "$ProtocolKey\shell\open\command" `
        -Name ""
    if (-not [bool]$command.ValueExisted) {
        return $false
    }
    return -not [string]::IsNullOrEmpty(
        (Get-AfirmaAutoFirmaExecutableFromCommand -Command ([string]$command.Value))
    )
}

# Verdadero si AutoFirma está instalado: en el registro del equipo, en el que
# guardó GrxFirma al instalarse o en el registro actual del usuario.
function Test-AfirmaAutoFirmaAvailable {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = "",
        [Parameter(Mandatory = $true)]
        [string]$SnapshotPath
    )

    if (-not [string]::IsNullOrEmpty((Get-AfirmaAutoFirmaExecutable -ProtocolKey $ProtocolKey))) {
        return $true
    }
    if (Test-AfirmaAutoFirmaIsCurrent -ProtocolKey $ProtocolKey) {
        return $true
    }
    if (-not (Test-Path -LiteralPath $SnapshotPath -PathType Leaf)) {
        return $false
    }
    $plan = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath)
    try {
        $state = Read-AfirmaProtocolSnapshot `
            -Path $SnapshotPath `
            -ProtocolKey $ProtocolKey `
            -Plan $plan `
            -AcceptedLegacyOwnerSets @(Get-AfirmaProtocolLegacyOwnerSets `
                -ProtocolKey $ProtocolKey `
                -ExecutablePath $ExecutablePath `
                -IconPath $IconPath)
    } catch {
        return $false
    }
    return -not [string]::IsNullOrEmpty(
        (Get-AfirmaSavedAutoFirmaExecutable -Snapshots @($state.Snapshots))
    )
}

# Devuelve afirma:// a lo que había antes de GrxFirma (AutoFirma). Primero
# registra GrxFirma para tener una instantánea propia y comprobada: si otro
# programa cambió el registro, Install-AfirmaProtocolRegistration aborta sin
# escribir.
function Switch-AfirmaProtocolToAutoFirma {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = "",
        [Parameter(Mandatory = $true)]
        [string]$SnapshotPath
    )

    Install-AfirmaProtocolRegistration `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath `
        -SnapshotPath $SnapshotPath
    $plan = @(Get-AfirmaProtocolRegistrationPlan `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath $ExecutablePath `
        -IconPath $IconPath)
    $state = Read-AfirmaProtocolSnapshot `
        -Path $SnapshotPath `
        -ProtocolKey $ProtocolKey `
        -Plan $plan `
        -AcceptedLegacyOwnerSets @(Get-AfirmaProtocolLegacyOwnerSets `
            -ProtocolKey $ProtocolKey `
            -ExecutablePath $ExecutablePath `
            -IconPath $IconPath)
    if (-not (Restore-AfirmaProtocolRegistration `
            -OwnerValues @($state.OwnerValues) `
            -Snapshots @($state.Snapshots))) {
        throw "El protocolo afirma:// cambio durante la instalacion; se aborta para no sobrescribir otro handler."
    }
}

# Aplica al instalar el programa para firmar en las webs y devuelve el que
# queda (grxfirma, autofirma o preguntar). Con -ValidateOnly solo comprueba la
# propiedad del registro, sin escribir nada.
function Invoke-AfirmaInstallHandlerChoice {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ProtocolKey,
        [Parameter(Mandatory = $true)]
        [string]$ExecutablePath,
        [string]$IconPath = "",
        [Parameter(Mandatory = $true)]
        [string]$SnapshotPath,
        [AllowEmptyString()]
        [string]$Requested = "",
        [switch]$ValidateOnly
    )

    $policy = Get-AfirmaMachinePolicyHandler
    if ([bool]$policy.Unreadable) {
        Write-Warning "No se pudo leer la política manejador_afirma del equipo; se mantiene la elección de cada usuario."
    }
    $preference = Get-AfirmaProtocolHandlerPreference
    $decision = Resolve-AfirmaInstallHandler -Requested $Requested -Preference $preference -Policy $policy
    $handler = $decision.Handler
    $explicit = $decision.Source -eq "politica" -or $decision.Source -eq "parametro"
    $common = @{
        ProtocolKey = $ProtocolKey
        ExecutablePath = $ExecutablePath
        IconPath = $IconPath
        SnapshotPath = $SnapshotPath
    }
    $autoFirmaCurrent = Test-AfirmaAutoFirmaIsCurrent -ProtocolKey $ProtocolKey
    $autoFirmaAvailable = Test-AfirmaAutoFirmaAvailable @common
    if ($handler -ne "grxfirma" -and -not $autoFirmaAvailable) {
        if ($explicit) {
            Write-Warning "AutoFirma no está instalado en este equipo: las firmas de las webs se abrirán con GrxFirma."
        }
        $handler = "grxfirma"
    }

    $applied = $handler
    if ($handler -eq "autofirma") {
        if ($autoFirmaCurrent -or (Test-AfirmaProtocolRetiredForAutoFirma @common)) {
            Write-Host "AutoFirma sigue atendiendo las firmas de las webs."
        } elseif ($ValidateOnly) {
            Install-AfirmaProtocolRegistration @common -ValidateOnly
        } else {
            Switch-AfirmaProtocolToAutoFirma @common
            Write-Host "Las firmas de las webs se abrirán con AutoFirma."
        }
    } elseif ($autoFirmaCurrent -and -not $explicit -and
        (Test-Path -LiteralPath $SnapshotPath -PathType Leaf) -and
        -not (Test-AfirmaProtocolRetiredForAutoFirma @common)) {
        # AutoFirma se adueñó del protocolo después de instalar GrxFirma (el
        # registro ya no es ni el de GrxFirma ni el de antes). No se
        # sobrescribe: la persona puede elegir en Configuración. Sin
        # instantánea (primera instalación) o con el registro retirado por
        # GrxFirma, se registra como hasta ahora.
        Write-Warning "AutoFirma atiende ahora las firmas de las webs. GrxFirma no lo cambia; puede elegir el programa en Configuración, «Programa para firmar en las webs»."
        $applied = "autofirma"
    } else {
        Install-AfirmaProtocolRegistration @common -ValidateOnly:$ValidateOnly
    }

    if ($ValidateOnly) {
        return $applied
    }
    # La preferencia guarda la elección de la persona; la política no la toca,
    # para que vuelva a valer si la organización la retira.
    $newPreference = ""
    if ($decision.Source -eq "parametro") {
        $newPreference = $applied
    } elseif ($decision.Source -ne "politica" -and $preference -eq "autofirma" -and $applied -eq "grxfirma") {
        # AutoFirma ya no está: GrxFirma vuelve a atender las firmas.
        $newPreference = "grxfirma"
    }
    if ($newPreference.Length -gt 0 -and $newPreference -ne $preference) {
        Set-AfirmaProtocolHandlerPreference -Handler $newPreference
    }
    return $applied
}

# Para la página del instalador NSIS: escribe «autofirma|política|preferencia»
# (por ejemplo «1|-|preguntar»), con «-» cuando no hay valor. Solo lee.
# Una política ilegible cuenta como «-»: se mantiene lo de cada usuario.
function Write-AfirmaInstallHandlerDetection {
    param(
        [string]$InstallDir = "$env:LOCALAPPDATA\Programs\GrxFirma\AfirmaURI",
        [string]$ProtocolKey = "Software\Classes\afirma"
    )

    $available = Test-AfirmaAutoFirmaAvailable `
        -ProtocolKey $ProtocolKey `
        -ExecutablePath (Join-Path $InstallDir "grxfirma-afirmauri.exe") `
        -IconPath (Join-Path $InstallDir "grxfirma-grx.ico") `
        -SnapshotPath (Join-Path $InstallDir "afirma-protocol-snapshot.json")
    $policy = Get-AfirmaMachinePolicyHandler
    $policyValue = "-"
    if (-not [bool]$policy.Unreadable -and -not [string]::IsNullOrEmpty([string]$policy.Value)) {
        $policyValue = [string]$policy.Value
    }
    $preference = Get-AfirmaProtocolHandlerPreference
    if ([string]::IsNullOrEmpty($preference)) {
        $preference = "-"
    }
    $flag = "0"
    if ($available) {
        $flag = "1"
    }
    Write-Output "$flag|$policyValue|$preference"
}
