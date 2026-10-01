# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$helperPath = Join-Path `
    $PSScriptRoot `
    "Manage-GrxFirmaSyntheticSigningCertificate.ps1"
if (-not (Test-Path -LiteralPath $helperPath -PathType Leaf)) {
    throw "Falta el helper de certificado sintetico: $helperPath"
}

$tokens = $null
$errors = $null
[void][System.Management.Automation.Language.Parser]::ParseFile(
    $helperPath,
    [ref]$tokens,
    [ref]$errors
)
if ($errors.Count -gt 0) {
    $messages = $errors | ForEach-Object {
        "{0}:{1}:{2}: {3}" -f `
            $helperPath,
            $_.Extent.StartLineNumber,
            $_.Extent.StartColumnNumber,
            $_.Message
    }
    throw "PowerShell invalido:`n$($messages -join "`n")"
}

$source = Get-Content -LiteralPath $helperPath -Raw
$requiredPatterns = @(
    '[ValidateSet("Create", "Remove", "Status")]',
    '$script:StoreScope = "CurrentUser"',
    '$script:StoreName = "My"',
    '$script:StoreLocation = "Cert:\CurrentUser\My"',
    '$script:SchemaVersion = 2',
    '$script:RecoverySchemaVersion = 1',
    '$script:Purpose = "synthetic-document-signing-qa-only"',
    '$script:DocumentSigningEkuOid = "1.3.6.1.4.1.311.10.3.12"',
    '$script:CodeSigningEkuOid = "1.3.6.1.5.5.7.3.3"',
    '$script:SubjectPrefix = "CN=GrxFirma QA Synthetic Signing "',
    '$script:KeyNamePrefix = "GrxFirma-QA-Synthetic-"',
    "Assert-WindowsPlatform",
    "Assert-FixedLocalPath",
    "[System.IO.DriveType]::Fixed",
    "[System.IO.FileAttributes]::ReparsePoint",
    "Protect-InventoryDirectory",
    "Protect-InventoryFile",
    "Assert-RestrictedAcl",
    "[System.Security.Principal.WellKnownSidType]::LocalSystemSid",
    "SetAccessRuleProtection(`$true, `$false)",
    'Join-Path $env:LOCALAPPDATA "GrxFirma\QA"',
    '"synthetic-signing-certificate"',
    '"inventory.json"',
    '"recovery.json"',
    "schemaVersion = `$script:SchemaVersion",
    "tool = `$script:ToolId",
    "certificateId = `$certificateId.ToString(""D"")",
    "thumbprint = `$thumbprint",
    "keyExportPolicyRequested = ""NonExportable""",
    "keyName = `$persistedCertificate.KeyName",
    "publicKeySpkiSha256 = `$persistedCertificate.PublicKeySpkiSha256",
    "trusted = `$false",
    "authenticode = `$false",
    "New-PersistedSyntheticCertificate",
    "CngKeyCreationParameters",
    "MicrosoftSoftwareKeyStorageProvider",
    "CngExportPolicies]::None",
    "CngKeyUsages]::Signing",
    "CngProperty]::new(",
    '"Length"',
    "[System.BitConverter]::GetBytes(2048)",
    "CngAlgorithm]::Rsa",
    "CertificateRequest]::new(",
    "HashAlgorithmName]::SHA256",
    "RSASignaturePadding]::Pkcs1",
    "X509BasicConstraintsExtension",
    "X509KeyUsageExtension",
    "X509EnhancedKeyUsageExtension",
    "X509Store]::new(",
    "OpenFlags]::ReadWrite",
    "`$store.Add(`$generatedCertificate)",
    "[datetime]::Now.AddDays(2)",
    "Assert-SyntheticCertificateIdentity",
    "Get-PublicKeySpkiSha256FromRsa",
    "Get-CertificatePublicKeySpkiSha256",
    "Get-NormalizedPublicKeySha256",
    "Assert-CngKeyMatchesPublicIdentity",
    "ExpectedPublicKeySpkiSha256",
    "IsMachineKey",
    "CngAlgorithmGroup]::Rsa",
    "CngKeyUsages]::Signing",
    "CngExportPolicies]::None",
    "GetRSAPrivateKey",
    "GetRSAPublicKey",
    "Remove-ExactSyntheticCertificate",
    "Remove-ExactCngKey",
    "-ExpectedPublicKeySpkiSha256 (",
    "-DeleteKey",
    "Get-InventoryCertificate -Inventory `$Inventory",
    "[System.IO.File]::Move(",
    "Write-RecoveryState",
    "Read-RecoveryState",
    "Resolve-InterruptedCreation",
    'phase = "prepared"',
    '$Recovery.phase = "key-created"',
    '$Recovery.phase = "certificate-stored"',
    '$recovery.phase = "inventory-written"',
    "Remove-RecoveryState",
    "no se eliminara automaticamente",
    "already-present",
    "already-absent",
    "stale-inventory"
)
foreach ($pattern in $requiredPatterns) {
    if (-not $source.Contains($pattern)) {
        throw "Falta una salvaguarda del certificado sintetico QA: $pattern"
    }
}

$commands = [System.Management.Automation.Language.Parser]::ParseFile(
    $helperPath,
    [ref]$tokens,
    [ref]$errors
).FindAll(
    {
        param($ast)
        $ast -is [System.Management.Automation.Language.CommandAst] -and
        $ast.GetCommandName() -ceq "Remove-ExactCngKey"
    },
    $true
)
foreach ($command in $commands) {
    if (
        $command.CommandElements.Extent.Text -notcontains
            "-ExpectedPublicKeySpkiSha256"
    ) {
        throw (
            "Toda eliminacion CNG debe validar la huella publica: " +
            $command.Extent.Text
        )
    }
}

$forbiddenPatterns = @(
    'Cert:\\(?:LocalMachine|CurrentUser)\\(?:Root|CA|TrustedPublisher)',
    '\bLocalMachine\b',
    '\bTrustedPublisher\b',
    '\bExport-PfxCertificate\b',
    '\bExport-Certificate\b',
    '\bImport-(?:Pfx)?Certificate\b',
    '\bcertutil(?:\.exe)?\b',
    '\bsigntool(?:\.exe)?\b',
    '\bSet-AuthenticodeSignature\b',
    '\bGet-Credential\b',
    '\bRead-Host\b',
    '\bConvertTo-SecureString\b',
    '\bNew-SelfSignedCertificate\b'
)
foreach ($pattern in $forbiddenPatterns) {
    if (
        [regex]::IsMatch(
            $source,
            $pattern,
            [System.Text.RegularExpressions.RegexOptions]::IgnoreCase
        )
    ) {
        throw "Operacion prohibida en el helper de certificado QA: $pattern"
    }
}

$codeSigningOidCount = (
    [regex]::Matches(
        $source,
        [regex]::Escape("1.3.6.1.5.5.7.3.3")
    )
).Count
if ($codeSigningOidCount -ne 1) {
    throw "El OID de Authenticode solo puede aparecer como exclusion."
}

$createBlock = [regex]::Match(
    $source,
    '(?s)function New-PersistedSyntheticCertificate.*?' +
        'function Remove-ExactSyntheticCertificate'
)
if (-not $createBlock.Success) {
    throw "No se pudo aislar la creacion del certificado QA."
}
foreach ($pattern in @(
    "CngKeyCreationParameters",
    "MicrosoftSoftwareKeyStorageProvider",
    "CngExportPolicies]::None",
    "CngKeyUsages]::Signing",
    "[System.BitConverter]::GetBytes(2048)",
    "CngAlgorithm]::Rsa",
    "CertificateRequest]::new(",
    "HashAlgorithmName]::SHA256",
    "RSASignaturePadding]::Pkcs1",
    "X509BasicConstraintsExtension",
    "X509KeyUsageFlags]::DigitalSignature",
    "X509EnhancedKeyUsageExtension",
    "`$script:DocumentSigningEkuOid"
)) {
    if (-not $createBlock.Value.Contains($pattern)) {
        throw "La creacion QA no fija su contrato criptografico: $pattern"
    }
}
if ($createBlock.Value.Contains("1.3.6.1.5.5.7.3.3")) {
    throw "La creacion QA no puede habilitar el OID de Authenticode."
}

$removalBlock = [regex]::Match(
    $source,
    '(?s)function Remove-ExactSyntheticCertificate.*?' +
        'function New-Result'
)
if (-not $removalBlock.Success) {
    throw "No se pudo aislar el borrado del certificado QA."
}
foreach ($pattern in @(
    "Assert-SyntheticCertificateIdentity",
    "Get-NormalizedThumbprint",
    "Join-Path `$script:StoreLocation `$thumbprint",
    "Remove-Item",
    "-LiteralPath `$certificatePath",
    "-DeleteKey",
    "Get-InventoryCertificate -Inventory `$Inventory"
)) {
    if (-not $removalBlock.Value.Contains($pattern)) {
        throw "El borrado QA no queda limitado al inventario: $pattern"
    }
}

if (
    [System.Environment]::OSVersion.Platform -ne
    [System.PlatformID]::Win32NT
) {
    $platformRejected = $false
    try {
        & $helperPath -Action Status
    }
    catch {
        $platformRejected = $_.Exception.Message.Contains(
            "solo puede gestionar certificados en Windows"
        )
    }
    if (-not $platformRejected) {
        throw "El helper debe rechazar su ejecucion fuera de Windows."
    }
}

$lifecyclePath = Join-Path `
    $PSScriptRoot `
    "Test-GrxFirmaSyntheticSigningCertificateLifecycle.ps1"
$lifecycleSource = Get-Content -LiteralPath $lifecyclePath -Raw
if (
    $lifecycleSource.Contains("Remove-LifecycleCngKeyIfPresent") -or
    $lifecycleSource.Contains("[System.Security.Cryptography.CngKey]::Open(")
) {
    throw "El ciclo de vida contiene un bypass de borrado CNG."
}
if (
    -not $lifecycleSource.Contains(
        "[void](Invoke-CertificateHelper -Action Remove)"
    ) -or
    -not $lifecycleSource.Contains(
        '"inventoryPath" -in $created.PSObject.Properties.Name'
    )
) {
    throw "El ciclo de vida no fija la limpieza y privacidad del helper."
}

Write-Host "Synthetic signing certificate policy validation passed."
