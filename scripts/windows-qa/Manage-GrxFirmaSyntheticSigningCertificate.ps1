# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet("Create", "Remove", "Status")]
    [string]$Action
)

$ErrorActionPreference = "Stop"
$script:ToolId = "grxfirma-windows-qa-synthetic-signing-certificate"
$script:SchemaVersion = 2
$script:RecoverySchemaVersion = 1
$script:StoreScope = "CurrentUser"
$script:StoreName = "My"
$script:StoreLocation = "Cert:\CurrentUser\My"
$script:Purpose = "synthetic-document-signing-qa-only"
$script:DocumentSigningEkuOid = "1.3.6.1.4.1.311.10.3.12"
$script:CodeSigningEkuOid = "1.3.6.1.5.5.7.3.3"
$script:SubjectPrefix = "CN=GrxFirma QA Synthetic Signing "
$script:KeyNamePrefix = "GrxFirma-QA-Synthetic-"

function Assert-WindowsPlatform {
    if (
        [System.Environment]::OSVersion.Platform -ne
        [System.PlatformID]::Win32NT
    ) {
        throw "Este helper QA solo puede gestionar certificados en Windows."
    }
}

function Test-ReparsePoint {
    param(
        [Parameter(Mandatory = $true)]
        [System.IO.FileSystemInfo]$Item
    )

    return (
        ($Item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
    )
}

function Assert-FixedLocalPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [switch]$AllowMissingLeaf
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if (
        $fullPath.StartsWith(
            "\\",
            [System.StringComparison]::Ordinal
        )
    ) {
        throw "La ruta del inventario QA no puede ser UNC."
    }

    $pathRoot = [System.IO.Path]::GetPathRoot($fullPath)
    if ([string]::IsNullOrWhiteSpace($pathRoot)) {
        throw "La ruta del inventario QA no tiene un volumen local."
    }

    $drive = [System.IO.DriveInfo]::new($pathRoot)
    if (
        -not $drive.IsReady -or
        $drive.DriveType -ne [System.IO.DriveType]::Fixed
    ) {
        throw "El inventario QA debe residir en un volumen local fijo."
    }

    $relativePath = $fullPath.Substring($pathRoot.Length)
    $segments = $relativePath.Split(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.StringSplitOptions]::RemoveEmptyEntries
    )
    $currentPath = $pathRoot
    for ($index = 0; $index -lt $segments.Count; $index++) {
        $currentPath = Join-Path $currentPath $segments[$index]
        $isLeaf = $index -eq ($segments.Count - 1)
        if (-not (Test-Path -LiteralPath $currentPath)) {
            if ($isLeaf -and $AllowMissingLeaf) {
                break
            }
            continue
        }

        $item = Get-Item -LiteralPath $currentPath -Force
        if (Test-ReparsePoint -Item $item) {
            throw "La ruta del inventario QA no puede atravesar puntos de reanalisis."
        }
    }

    return $fullPath
}

function New-RestrictedFileSystemRule {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Principal.SecurityIdentifier]$Identity,

        [Parameter(Mandatory = $true)]
        [System.Security.AccessControl.InheritanceFlags]$InheritanceFlags
    )

    return [System.Security.AccessControl.FileSystemAccessRule]::new(
        $Identity,
        [System.Security.AccessControl.FileSystemRights]::FullControl,
        $InheritanceFlags,
        [System.Security.AccessControl.PropagationFlags]::None,
        [System.Security.AccessControl.AccessControlType]::Allow
    )
}

function Get-RestrictedIdentities {
    $currentIdentity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    if ($null -eq $currentIdentity.User) {
        throw "No se pudo resolver la identidad del usuario QA."
    }

    return @(
        $currentIdentity.User,
        [System.Security.Principal.SecurityIdentifier]::new(
            [System.Security.Principal.WellKnownSidType]::LocalSystemSid,
            $null
        )
    )
}

function Protect-InventoryDirectory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $identities = Get-RestrictedIdentities
    $security = [System.Security.AccessControl.DirectorySecurity]::new()
    $security.SetOwner($identities[0])
    $security.SetAccessRuleProtection($true, $false)
    foreach ($identity in $identities) {
        $rule = New-RestrictedFileSystemRule `
            -Identity $identity `
            -InheritanceFlags (
                [System.Security.AccessControl.InheritanceFlags]::ContainerInherit `
                -bor
                [System.Security.AccessControl.InheritanceFlags]::ObjectInherit
            )
        [void]$security.AddAccessRule($rule)
    }

    Set-Acl -LiteralPath $Path -AclObject $security
}

function Protect-InventoryFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $identities = Get-RestrictedIdentities
    $security = [System.Security.AccessControl.FileSecurity]::new()
    $security.SetOwner($identities[0])
    $security.SetAccessRuleProtection($true, $false)
    foreach ($identity in $identities) {
        $rule = New-RestrictedFileSystemRule `
            -Identity $identity `
            -InheritanceFlags (
                [System.Security.AccessControl.InheritanceFlags]::None
            )
        [void]$security.AddAccessRule($rule)
    }

    Set-Acl -LiteralPath $Path -AclObject $security
}

function Assert-RestrictedAcl {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $allowedSids = @(
        [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value,
        [System.Security.Principal.SecurityIdentifier]::new(
            [System.Security.Principal.WellKnownSidType]::LocalSystemSid,
            $null
        ).Value
    )
    $acl = Get-Acl -LiteralPath $Path
    if (-not $acl.AreAccessRulesProtected) {
        throw "El inventario QA conserva permisos heredados."
    }

    $seenSids = @{}
    foreach ($accessRule in $acl.Access) {
        $sid = $accessRule.IdentityReference.Translate(
            [System.Security.Principal.SecurityIdentifier]
        ).Value
        if (
            $accessRule.AccessControlType -ne
                [System.Security.AccessControl.AccessControlType]::Allow -or
            $sid -notin $allowedSids -or
            (
                $accessRule.FileSystemRights -band
                [System.Security.AccessControl.FileSystemRights]::FullControl
            ) -ne [System.Security.AccessControl.FileSystemRights]::FullControl
        ) {
            throw "El inventario QA contiene una regla de acceso no autorizada."
        }
        $seenSids[$sid] = $true
    }
    foreach ($allowedSid in $allowedSids) {
        if (-not $seenSids.ContainsKey($allowedSid)) {
            throw "El inventario QA no conserva todos sus permisos obligatorios."
        }
    }
}

function Initialize-InventoryLocation {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw "LOCALAPPDATA no esta disponible para el inventario QA."
    }

    $qaRoot = Join-Path $env:LOCALAPPDATA "GrxFirma\QA"
    $inventoryDirectory = Join-Path `
        $qaRoot `
        "synthetic-signing-certificate"
    $inventoryPath = Join-Path $inventoryDirectory "inventory.json"
    $recoveryPath = Join-Path $inventoryDirectory "recovery.json"

    [void](Assert-FixedLocalPath -Path $qaRoot -AllowMissingLeaf)
    [void](Assert-FixedLocalPath -Path $inventoryDirectory -AllowMissingLeaf)
    [void](Assert-FixedLocalPath -Path $inventoryPath -AllowMissingLeaf)
    [void](Assert-FixedLocalPath -Path $recoveryPath -AllowMissingLeaf)

    if (-not (Test-Path -LiteralPath $inventoryDirectory)) {
        [void](New-Item -ItemType Directory -Path $inventoryDirectory -Force)
    }

    $directoryItem = Get-Item -LiteralPath $inventoryDirectory -Force
    if (-not $directoryItem.PSIsContainer) {
        throw "La ubicacion del inventario QA no es un directorio."
    }
    if (Test-ReparsePoint -Item $directoryItem) {
        throw "El directorio del inventario QA no puede ser un punto de reanalisis."
    }

    Protect-InventoryDirectory -Path $inventoryDirectory
    Assert-RestrictedAcl -Path $inventoryDirectory
    [void](Assert-FixedLocalPath -Path $inventoryPath -AllowMissingLeaf)
    [void](Assert-FixedLocalPath -Path $recoveryPath -AllowMissingLeaf)

    return [pscustomobject]@{
        Directory = $inventoryDirectory
        Path = $inventoryPath
        RecoveryPath = $recoveryPath
    }
}

function Get-ExpectedSubject {
    param(
        [Parameter(Mandatory = $true)]
        [guid]$CertificateId
    )

    return $script:SubjectPrefix + $CertificateId.ToString("D")
}

function Get-NormalizedThumbprint {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Thumbprint
    )

    $normalized = $Thumbprint.Replace(" ", "").ToUpperInvariant()
    if ($normalized -notmatch "^[0-9A-F]{40}$") {
        throw "La huella del inventario QA no es valida."
    }

    return $normalized
}

function Get-NormalizedPublicKeySha256 {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Sha256
    )

    $normalized = $Sha256.Replace(" ", "").ToUpperInvariant()
    if ($normalized -notmatch "^[0-9A-F]{64}$") {
        throw "La huella SHA-256 de la clave publica QA no es valida."
    }

    return $normalized
}

function Get-PublicKeySpkiSha256FromRsa {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.RSA]$Rsa
    )

    $spki = $Rsa.ExportSubjectPublicKeyInfo()
    try {
        $digest = [System.Security.Cryptography.SHA256]::HashData($spki)
        try {
            return [System.Convert]::ToHexString($digest)
        }
        finally {
            [System.Security.Cryptography.CryptographicOperations]::ZeroMemory(
                $digest
            )
        }
    }
    finally {
        [System.Security.Cryptography.CryptographicOperations]::ZeroMemory(
            $spki
        )
    }
}

function Get-CertificatePublicKeySpkiSha256 {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate
    )

    $rsa = [System.Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPublicKey(
        $Certificate
    )
    if ($null -eq $rsa) {
        throw "El certificado QA no contiene una clave publica RSA."
    }
    try {
        return Get-PublicKeySpkiSha256FromRsa -Rsa $rsa
    }
    finally {
        $rsa.Dispose()
    }
}

function Read-Inventory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return $null
    }

    $item = Get-Item -LiteralPath $Path -Force
    if ($item.PSIsContainer -or (Test-ReparsePoint -Item $item)) {
        throw "El inventario QA debe ser un fichero normal."
    }
    if ($item.Length -gt 16384) {
        throw "El inventario QA supera el tamano permitido."
    }

    Assert-RestrictedAcl -Path $Path
    $inventory = Get-Content -LiteralPath $Path -Raw -Encoding UTF8 |
        ConvertFrom-Json
    foreach ($property in @(
        "schemaVersion",
        "tool",
        "storeScope",
        "storeName",
        "purpose",
        "certificateId",
        "subject",
        "thumbprint",
        "createdAtUtc",
        "notBeforeUtc",
        "notAfterUtc",
        "keyAlgorithm",
        "keyLength",
        "keyName",
        "publicKeySpkiSha256",
        "signatureHashAlgorithm",
        "keyExportPolicyRequested",
        "trusted",
        "authenticode"
    )) {
        if ($property -notin $inventory.PSObject.Properties.Name) {
            throw "El inventario QA no cumple su esquema."
        }
    }

    if (
        [int]$inventory.schemaVersion -ne $script:SchemaVersion -or
        [string]$inventory.tool -cne $script:ToolId -or
        [string]$inventory.storeScope -cne $script:StoreScope -or
        [string]$inventory.storeName -cne $script:StoreName -or
        [string]$inventory.purpose -cne $script:Purpose -or
        [string]$inventory.keyAlgorithm -cne "RSA" -or
        [int]$inventory.keyLength -ne 2048 -or
        [string]$inventory.signatureHashAlgorithm -cne "SHA256" -or
        [string]$inventory.keyExportPolicyRequested -cne "NonExportable" -or
        [bool]$inventory.trusted -ne $false -or
        [bool]$inventory.authenticode -ne $false
    ) {
        throw "El inventario QA contiene un alcance no permitido."
    }

    $certificateId = [guid]::Empty
    if (
        -not [guid]::TryParseExact(
            [string]$inventory.certificateId,
            "D",
            [ref]$certificateId
        )
    ) {
        throw "El identificador del certificado QA no es valido."
    }

    $expectedSubject = Get-ExpectedSubject -CertificateId $certificateId
    if ([string]$inventory.subject -cne $expectedSubject) {
        throw "El sujeto del inventario no identifica un certificado QA."
    }
    $expectedKeyName = $script:KeyNamePrefix + $certificateId.ToString("D")
    if ([string]$inventory.keyName -cne $expectedKeyName) {
        throw "El inventario no identifica la clave CNG sintetica esperada."
    }

    $inventory.thumbprint = Get-NormalizedThumbprint `
        -Thumbprint ([string]$inventory.thumbprint)
    $inventory.publicKeySpkiSha256 = Get-NormalizedPublicKeySha256 `
        -Sha256 ([string]$inventory.publicKeySpkiSha256)
    [void][datetime]::Parse(
        [string]$inventory.createdAtUtc,
        [System.Globalization.CultureInfo]::InvariantCulture,
        [System.Globalization.DateTimeStyles]::RoundtripKind
    )
    [void][datetime]::Parse(
        [string]$inventory.notBeforeUtc,
        [System.Globalization.CultureInfo]::InvariantCulture,
        [System.Globalization.DateTimeStyles]::RoundtripKind
    )
    [void][datetime]::Parse(
        [string]$inventory.notAfterUtc,
        [System.Globalization.CultureInfo]::InvariantCulture,
        [System.Globalization.DateTimeStyles]::RoundtripKind
    )

    return $inventory
}

function Read-RecoveryState {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return $null
    }

    $item = Get-Item -LiteralPath $Path -Force
    if ($item.PSIsContainer -or (Test-ReparsePoint -Item $item)) {
        throw "El diario de recuperacion QA debe ser un fichero normal."
    }
    if ($item.Length -gt 16384) {
        throw "El diario de recuperacion QA supera el tamano permitido."
    }

    Assert-RestrictedAcl -Path $Path
    $recovery = Get-Content -LiteralPath $Path -Raw -Encoding UTF8 |
        ConvertFrom-Json
    foreach ($property in @(
        "schemaVersion",
        "tool",
        "purpose",
        "phase",
        "certificateId",
        "subject",
        "keyName",
        "createdAtUtc",
        "notBeforeUtc",
        "notAfterUtc",
        "keyAlgorithm",
        "keyLength",
        "signatureHashAlgorithm",
        "keyExportPolicyRequested",
        "trusted",
        "authenticode"
    )) {
        if ($property -notin $recovery.PSObject.Properties.Name) {
            throw "El diario de recuperacion QA no cumple su esquema."
        }
    }

    $phase = [string]$recovery.phase
    if (
        [int]$recovery.schemaVersion -ne $script:RecoverySchemaVersion -or
        [string]$recovery.tool -cne $script:ToolId -or
        [string]$recovery.purpose -cne $script:Purpose -or
        $phase -notin @(
            "prepared",
            "key-created",
            "certificate-stored",
            "inventory-written"
        ) -or
        [string]$recovery.keyAlgorithm -cne "RSA" -or
        [int]$recovery.keyLength -ne 2048 -or
        [string]$recovery.signatureHashAlgorithm -cne "SHA256" -or
        [string]$recovery.keyExportPolicyRequested -cne "NonExportable" -or
        [bool]$recovery.trusted -ne $false -or
        [bool]$recovery.authenticode -ne $false
    ) {
        throw "El diario de recuperacion QA contiene un alcance no permitido."
    }

    $certificateId = [guid]::Empty
    if (
        -not [guid]::TryParseExact(
            [string]$recovery.certificateId,
            "D",
            [ref]$certificateId
        )
    ) {
        throw "El diario no contiene un identificador QA valido."
    }
    $expectedSubject = Get-ExpectedSubject -CertificateId $certificateId
    $expectedKeyName = $script:KeyNamePrefix + $certificateId.ToString("D")
    if (
        [string]$recovery.subject -cne $expectedSubject -or
        [string]$recovery.keyName -cne $expectedKeyName
    ) {
        throw "El diario no identifica el material sintetico esperado."
    }

    foreach ($dateProperty in @(
        "createdAtUtc",
        "notBeforeUtc",
        "notAfterUtc"
    )) {
        [void][datetime]::Parse(
            [string]$recovery.$dateProperty,
            [System.Globalization.CultureInfo]::InvariantCulture,
            [System.Globalization.DateTimeStyles]::RoundtripKind
        )
    }

    if ($phase -ne "prepared") {
        if ("publicKeySpkiSha256" -notin $recovery.PSObject.Properties.Name) {
            throw "El diario no liga la clave CNG a su material publico."
        }
        $recovery.publicKeySpkiSha256 = Get-NormalizedPublicKeySha256 `
            -Sha256 ([string]$recovery.publicKeySpkiSha256)
    }
    if ($phase -in @("certificate-stored", "inventory-written")) {
        if ("thumbprint" -notin $recovery.PSObject.Properties.Name) {
            throw "El diario no identifica el certificado ya persistido."
        }
        $recovery.thumbprint = Get-NormalizedThumbprint `
            -Thumbprint ([string]$recovery.thumbprint)
    }

    return $recovery
}

function Write-RestrictedStateFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Directory,

        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [pscustomobject]$Value,

        [switch]$AllowReplace
    )

    if (Test-Path -LiteralPath $Path) {
        if (-not $AllowReplace) {
            throw "El estado QA ya existe."
        }
        $existingItem = Get-Item -LiteralPath $Path -Force
        if ($existingItem.PSIsContainer -or (Test-ReparsePoint -Item $existingItem)) {
            throw "El estado QA existente no es un fichero normal."
        }
        Assert-RestrictedAcl -Path $Path
    }

    [void](Assert-FixedLocalPath -Path $Directory)
    [void](Assert-FixedLocalPath -Path $Path -AllowMissingLeaf)
    Assert-RestrictedAcl -Path $Directory
    $temporaryPath = Join-Path `
        $Directory `
        (".state-{0}.tmp" -f [guid]::NewGuid().ToString("N"))
    try {
        $json = $Value | ConvertTo-Json -Depth 3
        Set-Content `
            -LiteralPath $temporaryPath `
            -Value $json `
            -Encoding utf8NoBOM `
            -NoNewline
        Protect-InventoryFile -Path $temporaryPath
        Assert-RestrictedAcl -Path $temporaryPath
        [void](Assert-FixedLocalPath -Path $Path -AllowMissingLeaf)
        [System.IO.File]::Move(
            $temporaryPath,
            $Path,
            [bool]$AllowReplace
        )
        Assert-RestrictedAcl -Path $Path
    }
    finally {
        if (Test-Path -LiteralPath $temporaryPath) {
            $temporaryItem = Get-Item -LiteralPath $temporaryPath -Force
            if (-not (Test-ReparsePoint -Item $temporaryItem)) {
                [System.IO.File]::Delete($temporaryPath)
            }
        }
    }
}

function Write-Inventory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Directory,

        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [pscustomobject]$Inventory
    )

    Write-RestrictedStateFile `
        -Directory $Directory `
        -Path $Path `
        -Value $Inventory
}

function Write-RecoveryState {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Directory,

        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [pscustomobject]$Recovery
    )

    Write-RestrictedStateFile `
        -Directory $Directory `
        -Path $Path `
        -Value $Recovery `
        -AllowReplace
}

function Remove-RestrictedStateFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }

    [void](Assert-FixedLocalPath -Path $Path)
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.PSIsContainer -or (Test-ReparsePoint -Item $item)) {
        throw "No se puede eliminar un estado QA que no sea un fichero normal."
    }
    Assert-RestrictedAcl -Path $Path
    [System.IO.File]::Delete($Path)
}

function Remove-Inventory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    Remove-RestrictedStateFile -Path $Path
}

function Remove-RecoveryState {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    Remove-RestrictedStateFile -Path $Path
}

function Get-InventoryCertificate {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Inventory
    )

    $thumbprint = Get-NormalizedThumbprint `
        -Thumbprint ([string]$Inventory.thumbprint)
    $store = [System.Security.Cryptography.X509Certificates.X509Store]::new(
        $script:StoreName,
        [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
    )
    try {
        $store.Open(
            [System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly
        )
        $matches = @(
            $store.Certificates | Where-Object {
                $_.Thumbprint.Replace(" ", "").ToUpperInvariant() -ceq
                    $thumbprint
            }
        )
        if ($matches.Count -eq 0) {
            return $null
        }
        if ($matches.Count -ne 1) {
            throw "La huella QA no identifica un certificado unico."
        }

        return [System.Security.Cryptography.X509Certificates.X509Certificate2]::new(
            $matches[0]
        )
    }
    finally {
        $store.Close()
        $store.Dispose()
    }
}

function Get-CertificateEkuOids {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate
    )

    $ekuExtension = $Certificate.Extensions |
        Where-Object {
            $_ -is [System.Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]
        } |
        Select-Object -First 1
    if ($null -eq $ekuExtension) {
        return @()
    }

    return @($ekuExtension.EnhancedKeyUsages | ForEach-Object { $_.Value })
}

function Test-PrivateKeyExportability {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate
    )

    $privateKey = [System.Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPrivateKey(
        $Certificate
    )
    if ($null -eq $privateKey) {
        return $null
    }

    try {
        if ($privateKey -is [System.Security.Cryptography.RSACng]) {
            $exportPolicy = $privateKey.Key.ExportPolicy
            $exportableFlags = (
                [System.Security.Cryptography.CngExportPolicies]::AllowExport `
                -bor
                [System.Security.Cryptography.CngExportPolicies]::AllowPlaintextExport
            )
            return ($exportPolicy -band $exportableFlags) -ne 0
        }
        if (
            $privateKey -is
            [System.Security.Cryptography.RSACryptoServiceProvider]
        ) {
            return $privateKey.CspKeyContainerInfo.Exportable
        }

        return $null
    }
    finally {
        $privateKey.Dispose()
    }
}

function Assert-SyntheticCertificateIdentity {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate,

        [Parameter(Mandatory = $true)]
        [pscustomobject]$Inventory
    )

    $expectedSubject = [string]$Inventory.subject
    if (
        $Certificate.Subject -cne $expectedSubject -or
        $Certificate.Issuer -cne $expectedSubject -or
        (Get-NormalizedThumbprint -Thumbprint $Certificate.Thumbprint) -cne
            [string]$Inventory.thumbprint -or
        -not $Certificate.HasPrivateKey -or
        $Certificate.PublicKey.Oid.Value -cne "1.2.840.113549.1.1.1" -or
        $Certificate.SignatureAlgorithm.Value -cne "1.2.840.113549.1.1.11"
    ) {
        throw "El certificado inventariado no coincide con la identidad QA esperada."
    }

    $rsa = [System.Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPublicKey(
        $Certificate
    )
    if ($null -eq $rsa) {
        throw "El certificado QA no contiene una clave publica RSA."
    }
    try {
        if (
            $rsa.KeySize -ne 2048 -or
            (Get-PublicKeySpkiSha256FromRsa -Rsa $rsa) -cne
                (Get-NormalizedPublicKeySha256 `
                    -Sha256 ([string]$Inventory.publicKeySpkiSha256))
        ) {
            throw "El certificado QA no usa una clave RSA de 2048 bits."
        }
    }
    finally {
        $rsa.Dispose()
    }

    $privateKey = [System.Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPrivateKey(
        $Certificate
    )
    if ($null -eq $privateKey) {
        throw "El certificado QA no contiene una clave privada RSA."
    }
    try {
        if (
            $privateKey -isnot [System.Security.Cryptography.RSACng] -or
            $privateKey.Key.KeyName -cne [string]$Inventory.keyName -or
            $privateKey.Key.Provider.Provider -cne
                [System.Security.Cryptography.CngProvider]::MicrosoftSoftwareKeyStorageProvider.Provider -or
            $privateKey.Key.IsMachineKey -or
            $privateKey.Key.AlgorithmGroup.AlgorithmGroup -cne
                [System.Security.Cryptography.CngAlgorithmGroup]::Rsa.AlgorithmGroup -or
            $privateKey.Key.KeyUsage -ne
                [System.Security.Cryptography.CngKeyUsages]::Signing -or
            $privateKey.Key.ExportPolicy -ne
                [System.Security.Cryptography.CngExportPolicies]::None -or
            $privateKey.KeySize -ne 2048
        ) {
            throw "El certificado QA no esta ligado a la clave CNG esperada."
        }
    }
    finally {
        $privateKey.Dispose()
    }

    $keyUsage = $Certificate.Extensions |
        Where-Object {
            $_ -is [System.Security.Cryptography.X509Certificates.X509KeyUsageExtension]
        } |
        Select-Object -First 1
    if (
        $null -eq $keyUsage -or
        $keyUsage.KeyUsages -ne
            [System.Security.Cryptography.X509Certificates.X509KeyUsageFlags]::DigitalSignature
    ) {
        throw "El certificado QA no esta limitado al uso DigitalSignature."
    }

    $basicConstraints = $Certificate.Extensions |
        Where-Object {
            $_ -is [System.Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]
        } |
        Select-Object -First 1
    if (
        $null -eq $basicConstraints -or
        $basicConstraints.CertificateAuthority
    ) {
        throw "El certificado QA no declara de forma explicita CA=false."
    }

    $ekuOids = @(Get-CertificateEkuOids -Certificate $Certificate)
    if (
        $ekuOids.Count -ne 1 -or
        $ekuOids[0] -cne $script:DocumentSigningEkuOid -or
        $script:CodeSigningEkuOid -in $ekuOids
    ) {
        throw "El certificado QA no esta limitado a firma de documentos."
    }

    $exportable = Test-PrivateKeyExportability -Certificate $Certificate
    if ($exportable -eq $true) {
        throw "La clave privada del certificado QA es exportable."
    }
}

function Test-CertificateUsable {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate
    )

    $now = [datetime]::UtcNow
    return (
        $Certificate.NotBefore.ToUniversalTime() -le $now.AddMinutes(1) -and
        $Certificate.NotAfter.ToUniversalTime() -gt $now.AddHours(1)
    )
}

function Assert-CngKeyMatchesPublicIdentity {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.CngKey]$Key,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedKeyName,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedPublicKeySpkiSha256
    )

    if (
        -not $ExpectedKeyName.StartsWith(
            $script:KeyNamePrefix,
            [System.StringComparison]::Ordinal
        )
    ) {
        throw "El nombre de clave no pertenece al helper QA."
    }

    $expectedSha256 = Get-NormalizedPublicKeySha256 `
        -Sha256 $ExpectedPublicKeySpkiSha256
    $provider = [System.Security.Cryptography.CngProvider]::MicrosoftSoftwareKeyStorageProvider
    if (
        $Key.KeyName -cne $ExpectedKeyName -or
        $Key.Provider.Provider -cne $provider.Provider -or
        $Key.IsMachineKey -or
        $Key.AlgorithmGroup.AlgorithmGroup -cne
            [System.Security.Cryptography.CngAlgorithmGroup]::Rsa.AlgorithmGroup -or
        $Key.KeyUsage -ne [System.Security.Cryptography.CngKeyUsages]::Signing -or
        $Key.ExportPolicy -ne [System.Security.Cryptography.CngExportPolicies]::None
    ) {
        throw "La clave CNG no cumple el contrato sintetico inventariado."
    }

    $rsa = [System.Security.Cryptography.RSACng]::new($Key)
    try {
        if (
            $rsa.KeySize -ne 2048 -or
            (Get-PublicKeySpkiSha256FromRsa -Rsa $rsa) -cne $expectedSha256
        ) {
            throw "La clave CNG no coincide con el material publico inventariado."
        }
    }
    finally {
        $rsa.Dispose()
    }
}

function Test-ExactUserCngKeyExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$KeyName
    )

    $provider = [System.Security.Cryptography.CngProvider]::MicrosoftSoftwareKeyStorageProvider
    return [System.Security.Cryptography.CngKey]::Exists(
        $KeyName,
        $provider,
        [System.Security.Cryptography.CngKeyOpenOptions]::UserKey
    )
}

function Remove-ExactCngKey {
    param(
        [Parameter(Mandatory = $true)]
        [string]$KeyName,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedPublicKeySpkiSha256
    )

    if (-not (Test-ExactUserCngKeyExists -KeyName $KeyName)) {
        return
    }

    $provider = [System.Security.Cryptography.CngProvider]::MicrosoftSoftwareKeyStorageProvider
    $key = [System.Security.Cryptography.CngKey]::Open(
        $KeyName,
        $provider,
        [System.Security.Cryptography.CngKeyOpenOptions]::UserKey
    )
    try {
        Assert-CngKeyMatchesPublicIdentity `
            -Key $key `
            -ExpectedKeyName $KeyName `
            -ExpectedPublicKeySpkiSha256 $ExpectedPublicKeySpkiSha256
        $key.Delete()
    }
    finally {
        $key.Dispose()
    }
}

function New-PersistedSyntheticCertificate {
    param(
        [Parameter(Mandatory = $true)]
        [guid]$CertificateId,

        [Parameter(Mandatory = $true)]
        [string]$Subject,

        [Parameter(Mandatory = $true)]
        [datetime]$NotBefore,

        [Parameter(Mandatory = $true)]
        [datetime]$NotAfter,

        [Parameter(Mandatory = $true)]
        [pscustomobject]$Location,

        [Parameter(Mandatory = $true)]
        [pscustomobject]$Recovery
    )

    $keyName = $script:KeyNamePrefix + $CertificateId.ToString("D")
    $provider = [System.Security.Cryptography.CngProvider]::MicrosoftSoftwareKeyStorageProvider
    $key = $null
    $rsa = $null
    $generatedCertificate = $null
    $store = $null
    $thumbprint = $null
    $publicKeySpkiSha256 = $null
    $creationError = $null
    try {
        $creationParameters = [System.Security.Cryptography.CngKeyCreationParameters]::new()
        $creationParameters.Provider = $provider
        $creationParameters.ExportPolicy = [System.Security.Cryptography.CngExportPolicies]::None
        $creationParameters.KeyCreationOptions = [System.Security.Cryptography.CngKeyCreationOptions]::None
        $creationParameters.KeyUsage = [System.Security.Cryptography.CngKeyUsages]::Signing
        [void]$creationParameters.Parameters.Add(
            [System.Security.Cryptography.CngProperty]::new(
                "Length",
                [System.BitConverter]::GetBytes(2048),
                [System.Security.Cryptography.CngPropertyOptions]::None
            )
        )

        $key = [System.Security.Cryptography.CngKey]::Create(
            [System.Security.Cryptography.CngAlgorithm]::Rsa,
            $keyName,
            $creationParameters
        )
        $rsa = [System.Security.Cryptography.RSACng]::new($key)
        $publicKeySpkiSha256 = Get-PublicKeySpkiSha256FromRsa -Rsa $rsa
        $Recovery.phase = "key-created"
        $Recovery | Add-Member `
            -NotePropertyName "publicKeySpkiSha256" `
            -NotePropertyValue $publicKeySpkiSha256 `
            -Force
        Write-RecoveryState `
            -Directory $Location.Directory `
            -Path $Location.RecoveryPath `
            -Recovery $Recovery
        $request = [System.Security.Cryptography.X509Certificates.CertificateRequest]::new(
            [System.Security.Cryptography.X509Certificates.X500DistinguishedName]::new(
                $Subject
            ),
            $rsa,
            [System.Security.Cryptography.HashAlgorithmName]::SHA256,
            [System.Security.Cryptography.RSASignaturePadding]::Pkcs1
        )
        [void]$request.CertificateExtensions.Add(
            [System.Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new(
                $false,
                $false,
                0,
                $true
            )
        )
        [void]$request.CertificateExtensions.Add(
            [System.Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new(
                [System.Security.Cryptography.X509Certificates.X509KeyUsageFlags]::DigitalSignature,
                $true
            )
        )
        $ekuOids = [System.Security.Cryptography.OidCollection]::new()
        [void]$ekuOids.Add(
            [System.Security.Cryptography.Oid]::new(
                $script:DocumentSigningEkuOid,
                "Document Signing"
            )
        )
        [void]$request.CertificateExtensions.Add(
            [System.Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]::new(
                $ekuOids,
                $false
            )
        )
        $generatedCertificate = $request.CreateSelfSigned(
            [System.DateTimeOffset]::new($NotBefore),
            [System.DateTimeOffset]::new($NotAfter)
        )
        $generatedCertificate.FriendlyName = (
            "GrxFirma QA synthetic signing certificate"
        )
        $thumbprint = Get-NormalizedThumbprint `
            -Thumbprint $generatedCertificate.Thumbprint

        $store = [System.Security.Cryptography.X509Certificates.X509Store]::new(
            $script:StoreName,
            [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
        )
        $store.Open(
            [System.Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite
        )
        $store.Add($generatedCertificate)
        $Recovery.phase = "certificate-stored"
        $Recovery | Add-Member `
            -NotePropertyName "thumbprint" `
            -NotePropertyValue $thumbprint `
            -Force
        Write-RecoveryState `
            -Directory $Location.Directory `
            -Path $Location.RecoveryPath `
            -Recovery $Recovery
    }
    catch {
        $creationError = $_
    }
    finally {
        if ($null -ne $store) {
            $store.Close()
            $store.Dispose()
        }
        if ($null -ne $generatedCertificate) {
            $generatedCertificate.Dispose()
        }
        if ($null -ne $rsa) {
            $rsa.Dispose()
        }
        if ($null -ne $key) {
            $key.Dispose()
        }
    }

    if ($null -ne $creationError) {
        $cleanupSucceeded = $true
        try {
            if (
                $null -ne $thumbprint -and
                $null -ne $publicKeySpkiSha256
            ) {
                $storedCertificate = Get-InventoryCertificate `
                    -Inventory ([pscustomobject]@{
                        thumbprint = $thumbprint
                    })
                if ($null -ne $storedCertificate) {
                    try {
                        Remove-NewCertificateAfterFailedCreate `
                            -Certificate $storedCertificate `
                            -ExpectedSubject $Subject `
                            -ExpectedKeyName $keyName `
                            -ExpectedPublicKeySpkiSha256 $publicKeySpkiSha256
                    }
                    finally {
                        $storedCertificate.Dispose()
                    }
                }
                else {
                    Remove-ExactCngKey `
                        -KeyName $keyName `
                        -ExpectedPublicKeySpkiSha256 $publicKeySpkiSha256
                }
            }
            elseif ($null -ne $publicKeySpkiSha256) {
                Remove-ExactCngKey `
                    -KeyName $keyName `
                    -ExpectedPublicKeySpkiSha256 $publicKeySpkiSha256
            }
            elseif (Test-ExactUserCngKeyExists -KeyName $keyName) {
                throw (
                    "La clave existe, pero no se pudo registrar su identidad " +
                    "publica; no se eliminara automaticamente."
                )
            }
        }
        catch {
            $cleanupSucceeded = $false
        }
        if (-not $cleanupSucceeded) {
            throw "No se pudo limpiar la clave CNG tras fallar su creacion QA."
        }
        throw $creationError
    }

    $certificate = Get-InventoryCertificate -Inventory ([pscustomobject]@{
        thumbprint = $thumbprint
    })
    if ($null -eq $certificate) {
        Remove-ExactCngKey `
            -KeyName $keyName `
            -ExpectedPublicKeySpkiSha256 $publicKeySpkiSha256
        throw "El certificado QA no quedo persistido en CurrentUser\\My."
    }

    return [pscustomobject]@{
        Certificate = $certificate
        KeyName = $keyName
        PublicKeySpkiSha256 = $publicKeySpkiSha256
    }
}

function Remove-ExactSyntheticCertificate {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Inventory,

        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate
    )

    Assert-SyntheticCertificateIdentity `
        -Certificate $Certificate `
        -Inventory $Inventory
    $thumbprint = Get-NormalizedThumbprint `
        -Thumbprint ([string]$Inventory.thumbprint)
    $certificatePath = Join-Path $script:StoreLocation $thumbprint
    Remove-Item `
        -LiteralPath $certificatePath `
        -DeleteKey `
        -Force `
        -ErrorAction Stop

    $remaining = Get-InventoryCertificate -Inventory $Inventory
    if ($null -ne $remaining) {
        $remaining.Dispose()
        throw "No se pudo retirar el certificado QA inventariado."
    }
}

function Remove-NewCertificateAfterFailedCreate {
    param(
        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedSubject,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedKeyName,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedPublicKeySpkiSha256
    )

    if (
        $Certificate.Subject -cne $ExpectedSubject -or
        $Certificate.Issuer -cne $ExpectedSubject -or
        -not $ExpectedSubject.StartsWith(
            $script:SubjectPrefix,
            [System.StringComparison]::Ordinal
        )
    ) {
        throw "No se pudo acreditar la identidad del certificado QA recien creado."
    }

    $thumbprint = Get-NormalizedThumbprint `
        -Thumbprint $Certificate.Thumbprint
    Assert-SyntheticCertificateIdentity `
        -Certificate $Certificate `
        -Inventory ([pscustomobject]@{
            subject = $ExpectedSubject
            thumbprint = $thumbprint
            keyName = $ExpectedKeyName
            publicKeySpkiSha256 = $ExpectedPublicKeySpkiSha256
        })
    $certificatePath = Join-Path $script:StoreLocation $thumbprint
    Remove-Item `
        -LiteralPath $certificatePath `
        -DeleteKey `
        -Force `
        -ErrorAction Stop

    $remaining = Get-InventoryCertificate -Inventory ([pscustomobject]@{
        thumbprint = $thumbprint
    })
    if ($null -ne $remaining) {
        $remaining.Dispose()
        throw "No se pudo limpiar el certificado QA recien creado."
    }
}

function Test-RecoveryMatchesInventory {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Recovery,

        [Parameter(Mandatory = $true)]
        [pscustomobject]$Inventory
    )

    if (
        [string]$Recovery.certificateId -cne
            [string]$Inventory.certificateId -or
        [string]$Recovery.subject -cne [string]$Inventory.subject -or
        [string]$Recovery.keyName -cne [string]$Inventory.keyName -or
        (Get-NormalizedPublicKeySha256 `
            -Sha256 ([string]$Recovery.publicKeySpkiSha256)) -cne
            [string]$Inventory.publicKeySpkiSha256
    ) {
        return $false
    }

    if (
        "thumbprint" -in $Recovery.PSObject.Properties.Name -and
        (Get-NormalizedThumbprint `
            -Thumbprint ([string]$Recovery.thumbprint)) -cne
            [string]$Inventory.thumbprint
    ) {
        return $false
    }

    return $true
}

function Get-RecoveryCertificate {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Recovery
    )

    if ("thumbprint" -in $Recovery.PSObject.Properties.Name) {
        return Get-InventoryCertificate -Inventory $Recovery
    }

    $expectedPublicKeySha256 = Get-NormalizedPublicKeySha256 `
        -Sha256 ([string]$Recovery.publicKeySpkiSha256)
    $store = [System.Security.Cryptography.X509Certificates.X509Store]::new(
        $script:StoreName,
        [System.Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser
    )
    $matches = @()
    try {
        $store.Open(
            [System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly
        )
        foreach ($candidate in $store.Certificates) {
            if (
                $candidate.Subject -cne [string]$Recovery.subject -or
                $candidate.Issuer -cne [string]$Recovery.subject
            ) {
                continue
            }
            try {
                if (
                    (Get-CertificatePublicKeySpkiSha256 `
                        -Certificate $candidate) -cne $expectedPublicKeySha256
                ) {
                    continue
                }
                $matches += (
                    [System.Security.Cryptography.X509Certificates.X509Certificate2]::new(
                        $candidate
                    )
                )
            }
            catch {
                continue
            }
        }
    }
    finally {
        $store.Close()
        $store.Dispose()
    }

    if ($matches.Count -eq 0) {
        return $null
    }
    if ($matches.Count -ne 1) {
        foreach ($match in $matches) {
            $match.Dispose()
        }
        throw "El diario QA coincide con mas de un certificado."
    }

    return $matches[0]
}

function Resolve-InterruptedCreation {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Location
    )

    $recovery = Read-RecoveryState -Path $Location.RecoveryPath
    if ($null -eq $recovery) {
        return
    }

    $inventory = Read-Inventory -Path $Location.Path
    if ($recovery.phase -eq "prepared") {
        if ($null -ne $inventory) {
            throw "El diario preparado entra en conflicto con el inventario QA."
        }
        if (
            Test-ExactUserCngKeyExists `
                -KeyName ([string]$recovery.keyName)
        ) {
            throw (
                "La creacion se interrumpio antes de registrar la identidad " +
                "publica de la clave. No se eliminara automaticamente."
            )
        }
        Remove-RecoveryState -Path $Location.RecoveryPath
        return
    }

    if (
        $null -ne $inventory -and
        -not (Test-RecoveryMatchesInventory `
            -Recovery $recovery `
            -Inventory $inventory)
    ) {
        throw "El diario de recuperacion no coincide con el inventario QA."
    }

    if ($null -ne $inventory) {
        $completedCertificate = Get-InventoryCertificate -Inventory $inventory
        if ($null -ne $completedCertificate) {
            try {
                Assert-SyntheticCertificateIdentity `
                    -Certificate $completedCertificate `
                    -Inventory $inventory
                Remove-RecoveryState -Path $Location.RecoveryPath
                return
            }
            finally {
                $completedCertificate.Dispose()
            }
        }
    }

    $recoveryCertificate = Get-RecoveryCertificate -Recovery $recovery
    try {
        if ($null -ne $recoveryCertificate) {
            $recoveryIdentity = [pscustomobject]@{
                subject = [string]$recovery.subject
                thumbprint = Get-NormalizedThumbprint `
                    -Thumbprint $recoveryCertificate.Thumbprint
                keyName = [string]$recovery.keyName
                publicKeySpkiSha256 = [string]$recovery.publicKeySpkiSha256
            }
            Remove-ExactSyntheticCertificate `
                -Inventory $recoveryIdentity `
                -Certificate $recoveryCertificate
        }
        Remove-ExactCngKey `
            -KeyName ([string]$recovery.keyName) `
            -ExpectedPublicKeySpkiSha256 (
                [string]$recovery.publicKeySpkiSha256
            )
        if ($null -ne $inventory) {
            Remove-Inventory -Path $Location.Path
        }
        Remove-RecoveryState -Path $Location.RecoveryPath
    }
    finally {
        if ($null -ne $recoveryCertificate) {
            $recoveryCertificate.Dispose()
        }
    }
}

function New-Result {
    param(
        [Parameter(Mandatory = $true)]
        [string]$State,

        [pscustomobject]$Inventory,

        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Certificate,

        [bool]$Usable = $false
    )

    $exportable = $null
    if ($null -ne $Certificate -and $Certificate.HasPrivateKey) {
        $exportable = Test-PrivateKeyExportability -Certificate $Certificate
    }

    return [ordered]@{
        action = $Action.ToLowerInvariant()
        state = $State
        storeScope = $script:StoreScope
        storeName = $script:StoreName
        installed = $null -ne $Certificate
        usable = $Usable
        certificateId = if ($null -ne $Inventory) {
            [string]$Inventory.certificateId
        } else {
            $null
        }
        subject = if ($null -ne $Certificate) {
            $Certificate.Subject
        } elseif ($null -ne $Inventory) {
            [string]$Inventory.subject
        } else {
            $null
        }
        thumbprint = if ($null -ne $Inventory) {
            [string]$Inventory.thumbprint
        } else {
            $null
        }
        notAfterUtc = if ($null -ne $Certificate) {
            $Certificate.NotAfter.ToUniversalTime().ToString("O")
        } else {
            $null
        }
        keyExportable = $exportable
        trusted = $false
        authenticode = $false
    }
}

function Invoke-Create {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Location
    )

    $existingInventory = Read-Inventory -Path $Location.Path
    if ($null -ne $existingInventory) {
        $existingCertificate = Get-InventoryCertificate `
            -Inventory $existingInventory
        if ($null -eq $existingCertificate) {
            Remove-ExactCngKey `
                -KeyName ([string]$existingInventory.keyName) `
                -ExpectedPublicKeySpkiSha256 (
                    [string]$existingInventory.publicKeySpkiSha256
                )
            Remove-Inventory -Path $Location.Path
        }
        else {
            try {
                Assert-SyntheticCertificateIdentity `
                    -Certificate $existingCertificate `
                    -Inventory $existingInventory
                if (Test-CertificateUsable -Certificate $existingCertificate) {
                    return New-Result `
                        -State "already-present" `
                        -Inventory $existingInventory `
                        -Certificate $existingCertificate `
                        -Usable $true
                }

                Remove-ExactSyntheticCertificate `
                    -Inventory $existingInventory `
                    -Certificate $existingCertificate
                Remove-Inventory -Path $Location.Path
            }
            finally {
                $existingCertificate.Dispose()
            }
        }
    }

    $certificateId = [guid]::NewGuid()
    $subject = Get-ExpectedSubject -CertificateId $certificateId
    $notBefore = [datetime]::Now.AddMinutes(-5)
    $notAfter = [datetime]::Now.AddDays(2)
    $recovery = [pscustomobject][ordered]@{
        schemaVersion = $script:RecoverySchemaVersion
        tool = $script:ToolId
        purpose = $script:Purpose
        phase = "prepared"
        certificateId = $certificateId.ToString("D")
        subject = $subject
        keyName = $script:KeyNamePrefix + $certificateId.ToString("D")
        createdAtUtc = [datetime]::UtcNow.ToString("O")
        notBeforeUtc = $notBefore.ToUniversalTime().ToString("O")
        notAfterUtc = $notAfter.ToUniversalTime().ToString("O")
        keyAlgorithm = "RSA"
        keyLength = 2048
        signatureHashAlgorithm = "SHA256"
        keyExportPolicyRequested = "NonExportable"
        trusted = $false
        authenticode = $false
    }
    Write-RecoveryState `
        -Directory $Location.Directory `
        -Path $Location.RecoveryPath `
        -Recovery $recovery
    $certificate = $null
    $inventory = $null
    try {
        $persistedCertificate = New-PersistedSyntheticCertificate `
            -CertificateId $certificateId `
            -Subject $subject `
            -NotBefore $notBefore `
            -NotAfter $notAfter `
            -Location $Location `
            -Recovery $recovery
        $certificate = $persistedCertificate.Certificate

        $thumbprint = Get-NormalizedThumbprint `
            -Thumbprint $certificate.Thumbprint
        $inventory = [pscustomobject][ordered]@{
            schemaVersion = $script:SchemaVersion
            tool = $script:ToolId
            storeScope = $script:StoreScope
            storeName = $script:StoreName
            purpose = $script:Purpose
            certificateId = $certificateId.ToString("D")
            subject = $subject
            thumbprint = $thumbprint
            createdAtUtc = [datetime]::UtcNow.ToString("O")
            notBeforeUtc = $certificate.NotBefore.ToUniversalTime().ToString("O")
            notAfterUtc = $certificate.NotAfter.ToUniversalTime().ToString("O")
            keyAlgorithm = "RSA"
            keyLength = 2048
            keyName = $persistedCertificate.KeyName
            publicKeySpkiSha256 = $persistedCertificate.PublicKeySpkiSha256
            signatureHashAlgorithm = "SHA256"
            keyExportPolicyRequested = "NonExportable"
            trusted = $false
            authenticode = $false
        }

        Assert-SyntheticCertificateIdentity `
            -Certificate $certificate `
            -Inventory $inventory
        Write-Inventory `
            -Directory $Location.Directory `
            -Path $Location.Path `
            -Inventory $inventory
        $recovery.phase = "inventory-written"
        Write-RecoveryState `
            -Directory $Location.Directory `
            -Path $Location.RecoveryPath `
            -Recovery $recovery
        Remove-RecoveryState -Path $Location.RecoveryPath

        return New-Result `
            -State "created" `
            -Inventory $inventory `
            -Certificate $certificate `
            -Usable $true
    }
    catch {
        $creationError = $_
        try {
            Resolve-InterruptedCreation -Location $Location
        }
        catch {
            throw (
                "Fallo la creacion y la recuperacion segura requiere " +
                "intervencion: $($_.Exception.Message)"
            )
        }
        throw $creationError
    }
    finally {
        if ($null -ne $certificate) {
            $certificate.Dispose()
        }
    }
}

function Invoke-Remove {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Location
    )

    $inventory = Read-Inventory -Path $Location.Path
    if ($null -eq $inventory) {
        return New-Result `
            -State "already-absent"
    }

    $certificate = Get-InventoryCertificate -Inventory $inventory
    try {
        if ($null -ne $certificate) {
            Remove-ExactSyntheticCertificate `
                -Inventory $inventory `
                -Certificate $certificate
        }
        # Si el certificado fue retirado externamente, -DeleteKey no pudo
        # ejecutarse. Solo se borra la clave si su SPKI y todo el contrato CNG
        # coinciden con el inventario validado.
        Remove-ExactCngKey `
            -KeyName ([string]$inventory.keyName) `
            -ExpectedPublicKeySpkiSha256 (
                [string]$inventory.publicKeySpkiSha256
            )
        Remove-Inventory -Path $Location.Path
        return New-Result `
            -State "removed"
    }
    finally {
        if ($null -ne $certificate) {
            $certificate.Dispose()
        }
    }
}

function Invoke-Status {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Location
    )

    $inventory = Read-Inventory -Path $Location.Path
    if ($null -eq $inventory) {
        return New-Result `
            -State "absent"
    }

    $certificate = Get-InventoryCertificate -Inventory $inventory
    if ($null -eq $certificate) {
        return New-Result `
            -State "stale-inventory" `
            -Inventory $inventory
    }

    try {
        Assert-SyntheticCertificateIdentity `
            -Certificate $certificate `
            -Inventory $inventory
        $usable = Test-CertificateUsable -Certificate $certificate
        return New-Result `
            -State $(if ($usable) { "present" } else { "expired" }) `
            -Inventory $inventory `
            -Certificate $certificate `
            -Usable $usable
    }
    finally {
        $certificate.Dispose()
    }
}

Assert-WindowsPlatform
$location = Initialize-InventoryLocation
Resolve-InterruptedCreation -Location $location
$result = switch ($Action) {
    "Create" { Invoke-Create -Location $location }
    "Remove" { Invoke-Remove -Location $location }
    "Status" { Invoke-Status -Location $location }
}

$result | ConvertTo-Json -Depth 4 -Compress
