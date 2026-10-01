# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
if (
    [System.Environment]::OSVersion.Platform -ne
    [System.PlatformID]::Win32NT
) {
    throw "La prueba de ciclo de vida del certificado QA solo se ejecuta en Windows."
}

$helperPath = Join-Path `
    $PSScriptRoot `
    "Manage-GrxFirmaSyntheticSigningCertificate.ps1"
if (-not (Test-Path -LiteralPath $helperPath -PathType Leaf)) {
    throw "Falta el helper de certificado sintetico: $helperPath"
}

function Invoke-CertificateHelper {
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet("Create", "Remove", "Status")]
        [string]$Action
    )

    $output = & $helperPath -Action $Action
    if ($output.Count -ne 1) {
        throw "El helper QA no devolvio un unico resultado JSON."
    }

    return $output | ConvertFrom-Json
}

function Test-ThumbprintInStore {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Thumbprint,

        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.StoreName]$StoreName,

        [Parameter(Mandatory = $true)]
        [System.Security.Cryptography.X509Certificates.StoreLocation]$StoreLocation
    )

    $store = [System.Security.Cryptography.X509Certificates.X509Store]::new(
        $StoreName,
        $StoreLocation
    )
    try {
        $store.Open(
            [System.Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly
        )
        return @(
            $store.Certificates | Where-Object {
                $_.Thumbprint.Replace(" ", "").ToUpperInvariant() -ceq
                    $Thumbprint
            }
        ).Count -gt 0
    }
    finally {
        $store.Close()
        $store.Dispose()
    }
}

$thumbprint = $null
$keyName = $null
$certificateSpkiSha256 = $null
$inventoryPath = Join-Path `
    $env:LOCALAPPDATA `
    "GrxFirma\QA\synthetic-signing-certificate\inventory.json"
$recoveryPath = Join-Path `
    $env:LOCALAPPDATA `
    "GrxFirma\QA\synthetic-signing-certificate\recovery.json"
try {
    $created = Invoke-CertificateHelper -Action Create
    if (
        $created.state -notin @("created", "already-present") -or
        -not $created.installed -or
        -not $created.usable -or
        $created.storeScope -cne "CurrentUser" -or
        $created.storeName -cne "My" -or
        $created.trusted -ne $false -or
        $created.authenticode -ne $false -or
        $created.keyExportable -ne $false
    ) {
        throw "La creacion no cumple el contrato del certificado QA."
    }
    if ("inventoryPath" -in $created.PSObject.Properties.Name) {
        throw "El helper no debe exponer rutas locales absolutas en su JSON."
    }
    if (Test-Path -LiteralPath $recoveryPath) {
        throw "La creacion completada dejo abierto su diario de recuperacion."
    }
    $thumbprint = [string]$created.thumbprint
    if ($thumbprint -notmatch "^[0-9A-F]{40}$") {
        throw "La creacion no devolvio una huella valida."
    }

    $createdAgain = Invoke-CertificateHelper -Action Create
    if (
        $createdAgain.state -cne "already-present" -or
        [string]$createdAgain.thumbprint -cne $thumbprint
    ) {
        throw "La creacion QA no es idempotente."
    }

    $status = Invoke-CertificateHelper -Action Status
    if (
        $status.state -cne "present" -or
        -not $status.installed -or
        -not $status.usable -or
        [string]$status.thumbprint -cne $thumbprint
    ) {
        throw "El estado no confirma el certificado QA creado."
    }

    $notAfterUtc = ([datetime]$status.notAfterUtc).ToUniversalTime()
    $remainingHours = ($notAfterUtc - [datetime]::UtcNow).TotalHours
    if ($remainingHours -lt 47 -or $remainingHours -gt 49) {
        throw (
            "El certificado QA no tiene la vigencia efimera esperada ({0:N2} h)." `
                -f $remainingHours
        )
    }

    if (
        -not (
            Test-ThumbprintInStore `
                -Thumbprint $thumbprint `
                -StoreName My `
                -StoreLocation CurrentUser
        )
    ) {
        throw "El certificado QA no aparece en CurrentUser\\My."
    }

    $certificatePath = "Cert:\CurrentUser\My\$thumbprint"
    $certificate = Get-Item -LiteralPath $certificatePath
    $privateKey = $null
    $publicKey = $null
    $probe = [System.Text.Encoding]::UTF8.GetBytes(
        "GrxFirma synthetic QA signing probe"
    )
    $signature = $null
    try {
        $privateKey = [System.Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPrivateKey(
            $certificate
        )
        $publicKey = [System.Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPublicKey(
            $certificate
        )
        if ($null -eq $privateKey -or $null -eq $publicKey) {
            throw "El certificado QA no ofrece sus claves RSA."
        }
        $spki = $publicKey.ExportSubjectPublicKeyInfo()
        try {
            $spkiDigest = [System.Security.Cryptography.SHA256]::HashData($spki)
            try {
                $certificateSpkiSha256 = [System.Convert]::ToHexString(
                    $spkiDigest
                )
            }
            finally {
                [System.Security.Cryptography.CryptographicOperations]::ZeroMemory(
                    $spkiDigest
                )
            }
        }
        finally {
            [System.Security.Cryptography.CryptographicOperations]::ZeroMemory(
                $spki
            )
        }
        $signature = $privateKey.SignData(
            $probe,
            [System.Security.Cryptography.HashAlgorithmName]::SHA256,
            [System.Security.Cryptography.RSASignaturePadding]::Pkcs1
        )
        if (
            -not $publicKey.VerifyData(
                $probe,
                $signature,
                [System.Security.Cryptography.HashAlgorithmName]::SHA256,
                [System.Security.Cryptography.RSASignaturePadding]::Pkcs1
            )
        ) {
            throw "La clave privada QA no completa una firma RSA verificable."
        }
    }
    finally {
        if ($null -ne $signature) {
            [System.Security.Cryptography.CryptographicOperations]::ZeroMemory(
                $signature
            )
        }
        [System.Security.Cryptography.CryptographicOperations]::ZeroMemory(
            $probe
        )
        if ($null -ne $publicKey) {
            $publicKey.Dispose()
        }
        if ($null -ne $privateKey) {
            $privateKey.Dispose()
        }
        $certificate.Dispose()
    }

    foreach ($forbiddenStore in @(
        @{
            Name = "Root"
            Location = "CurrentUser"
        },
        @{
            Name = "CertificateAuthority"
            Location = "CurrentUser"
        },
        @{
            Name = "TrustedPublisher"
            Location = "CurrentUser"
        },
        @{
            Name = "My"
            Location = "LocalMachine"
        },
        @{
            Name = "Root"
            Location = "LocalMachine"
        }
    )) {
        if (
            Test-ThumbprintInStore `
                -Thumbprint $thumbprint `
                -StoreName $forbiddenStore.Name `
                -StoreLocation $forbiddenStore.Location
        ) {
            throw (
                "El certificado QA aparecio en un almacen prohibido: {0}\\{1}." `
                    -f $forbiddenStore.Location, $forbiddenStore.Name
            )
        }
    }

    $inventoryItem = Get-Item -LiteralPath $inventoryPath -Force
    if (
        $inventoryItem.PSIsContainer -or
        (
            $inventoryItem.Attributes -band
            [System.IO.FileAttributes]::ReparsePoint
        ) -ne 0
    ) {
        throw "El inventario QA no es un fichero local normal."
    }
    $inventory = Get-Content `
        -LiteralPath $inventoryPath `
        -Raw `
        -Encoding UTF8 |
        ConvertFrom-Json
    $keyName = [string]$inventory.keyName
    if (
        [string]$inventory.publicKeySpkiSha256 -notmatch
            "^[0-9A-F]{64}$"
    ) {
        throw "El inventario QA no liga la clave a su SPKI."
    }
    if (
        [string]$inventory.publicKeySpkiSha256 -cne
            $certificateSpkiSha256
    ) {
        throw "El inventario no coincide con el SPKI del certificado."
    }
    if (
        $keyName -notmatch (
            "^GrxFirma-QA-Synthetic-" +
            "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-" +
            "[0-9a-f]{4}-[0-9a-f]{12}$"
        )
    ) {
        throw "El inventario QA no identifica una clave CNG valida."
    }

    $allowedSids = @(
        [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value,
        [System.Security.Principal.SecurityIdentifier]::new(
            [System.Security.Principal.WellKnownSidType]::LocalSystemSid,
            $null
        ).Value
    )
    $inventoryAcl = Get-Acl -LiteralPath $inventoryPath
    if (-not $inventoryAcl.AreAccessRulesProtected) {
        throw "El inventario QA conserva permisos heredados."
    }
    foreach ($accessRule in $inventoryAcl.Access) {
        $sid = $accessRule.IdentityReference.Translate(
            [System.Security.Principal.SecurityIdentifier]
        ).Value
        if (
            $accessRule.AccessControlType -eq
            [System.Security.AccessControl.AccessControlType]::Allow -and
            $sid -notin $allowedSids
        ) {
            throw "El inventario QA tiene una identidad permitida inesperada."
        }
    }

    $removed = Invoke-CertificateHelper -Action Remove
    if (
        $removed.state -cne "removed" -or
        $removed.installed -or
        (Test-Path -LiteralPath $inventoryPath) -or
        (Test-Path -LiteralPath $recoveryPath)
    ) {
        throw "El borrado no retiro el certificado y su inventario."
    }
    if (
        Test-ThumbprintInStore `
            -Thumbprint $thumbprint `
            -StoreName My `
            -StoreLocation CurrentUser
    ) {
        throw "La huella QA sigue presente despues del borrado."
    }
    $provider = [System.Security.Cryptography.CngProvider]::MicrosoftSoftwareKeyStorageProvider
    if (
        [System.Security.Cryptography.CngKey]::Exists(
            $keyName,
            $provider,
            [System.Security.Cryptography.CngKeyOpenOptions]::UserKey
        )
    ) {
        throw "La clave privada QA sigue presente despues del borrado."
    }

    $removedAgain = Invoke-CertificateHelper -Action Remove
    if ($removedAgain.state -cne "already-absent") {
        throw "El borrado QA no es idempotente."
    }
}
finally {
    [void](Invoke-CertificateHelper -Action Remove)
}

Write-Host "Synthetic signing certificate lifecycle validation passed."
