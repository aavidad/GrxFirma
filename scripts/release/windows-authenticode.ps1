# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

function Get-NormalizedWindowsSigningThumbprint {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Thumbprint
    )

    $normalized = ($Thumbprint -replace '\s', '').ToUpperInvariant()
    if ($normalized -notmatch '^[0-9A-F]{40}$') {
        throw "La huella Authenticode debe contener 40 hexadecimales."
    }
    return $normalized
}

function Get-NormalizedRfc3161TimestampUrl {
    param(
        [Parameter(Mandatory = $true)]
        [string]$TimestampUrl
    )

    try {
        $uri = [Uri]$TimestampUrl
    } catch {
        throw "La URL RFC3161 no es valida."
    }
    if (-not $uri.IsAbsoluteUri -or
        $uri.Scheme -notin @('http', 'https') -or
        [string]::IsNullOrWhiteSpace($uri.Host) -or
        -not [string]::IsNullOrEmpty($uri.UserInfo) -or
        -not [string]::IsNullOrEmpty($uri.Fragment)) {
        throw "La URL RFC3161 debe ser HTTP(S), absoluta, sin credenciales ni fragmento."
    }
    return $uri.AbsoluteUri
}

function Resolve-WindowsSignTool {
    if (-not ($PSVersionTable.PSEdition -eq 'Desktop' -or $IsWindows -eq $true)) {
        throw "SignTool solo esta disponible en Windows."
    }

    $command = Get-Command signtool.exe -CommandType Application -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -ne $command) {
        return $command.Source
    }

    $kitsRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\bin'
    $candidates = @()
    if (Test-Path -LiteralPath $kitsRoot -PathType Container) {
        $candidates = @(
            Get-ChildItem `
                -LiteralPath $kitsRoot `
                -Directory `
                -ErrorAction SilentlyContinue |
                ForEach-Object {
                    Join-Path $_.FullName 'x64\signtool.exe'
                } |
                Where-Object {
                    Test-Path -LiteralPath $_ -PathType Leaf
                } |
                Sort-Object -Descending
        )
        $unversioned = Join-Path $kitsRoot 'x64\signtool.exe'
        if (Test-Path -LiteralPath $unversioned -PathType Leaf) {
            $candidates += $unversioned
        }
    }
    if ($candidates.Count -eq 0) {
        throw "No se encuentra signtool.exe en PATH ni en Windows Kits 10."
    }
    return [System.IO.Path]::GetFullPath($candidates[0])
}

function ConvertFrom-WindowsDigestOid {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Oid
    )

    switch ($Oid) {
        '1.3.14.3.2.26' { 'SHA1'; break }
        '2.16.840.1.101.3.4.2.1' { 'SHA256'; break }
        '2.16.840.1.101.3.4.2.2' { 'SHA384'; break }
        '2.16.840.1.101.3.4.2.3' { 'SHA512'; break }
        default { $Oid; break }
    }
}

function Get-DerObjectLength {
    param(
        [Parameter(Mandatory = $true)]
        [byte[]]$Bytes
    )

    if ($Bytes.Length -lt 2 -or $Bytes[0] -ne 0x30) {
        throw "La firma PKCS#7 no comienza con una secuencia DER."
    }
    $lengthByte = [int]$Bytes[1]
    if (($lengthByte -band 0x80) -eq 0) {
        $contentLength = $lengthByte
        $headerLength = 2
    } else {
        $lengthOctets = $lengthByte -band 0x7f
        if ($lengthOctets -lt 1 -or $lengthOctets -gt 4 -or
            $Bytes.Length -lt (2 + $lengthOctets)) {
            throw "Longitud DER no valida en la firma PKCS#7."
        }
        $contentLength = 0
        for ($index = 0; $index -lt $lengthOctets; $index++) {
            $contentLength =
                ($contentLength -shl 8) -bor [int]$Bytes[2 + $index]
        }
        $headerLength = 2 + $lengthOctets
    }
    $totalLength = $headerLength + $contentLength
    if ($totalLength -gt $Bytes.Length) {
        throw "La firma PKCS#7 esta truncada."
    }
    return $totalLength
}

function Get-WindowsAuthenticodeSignedCms {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    Add-Type -AssemblyName System.Security.Cryptography.Pkcs
    $stream = [System.IO.File]::Open(
        $Path,
        [System.IO.FileMode]::Open,
        [System.IO.FileAccess]::Read,
        [System.IO.FileShare]::Read
    )
    $reader = [System.IO.BinaryReader]::new($stream)
    try {
        if ($stream.Length -lt 64) {
            throw "El fichero PE es demasiado corto: $Path"
        }
        $stream.Position = 0x3c
        $peOffset = [int64]$reader.ReadUInt32()
        if ($peOffset -lt 64 -or $peOffset + 24 -gt $stream.Length) {
            throw "Cabecera PE no valida: $Path"
        }
        $stream.Position = $peOffset
        if ($reader.ReadUInt32() -ne 0x00004550) {
            throw "Firma PE no valida: $Path"
        }
        $optionalHeader = $peOffset + 24
        $stream.Position = $optionalHeader
        $magic = $reader.ReadUInt16()
        $dataDirectories = switch ($magic) {
            0x010b { $optionalHeader + 96; break }
            0x020b { $optionalHeader + 112; break }
            default { throw "Formato de cabecera opcional PE no soportado: $magic" }
        }
        $certificateDirectory = $dataDirectories + (8 * 4)
        if ($certificateDirectory + 8 -gt $stream.Length) {
            throw "El PE no contiene una tabla de certificados valida."
        }
        $stream.Position = $certificateDirectory
        $certificateOffset = [int64]$reader.ReadUInt32()
        $certificateTableSize = [int64]$reader.ReadUInt32()
        if ($certificateOffset -le 0 -or
            $certificateTableSize -lt 8 -or
            $certificateOffset + $certificateTableSize -gt $stream.Length) {
            throw "El PE no contiene una firma Authenticode embebida."
        }
        $stream.Position = $certificateOffset
        $certificateLength = [int64]$reader.ReadUInt32()
        [void]$reader.ReadUInt16()
        $certificateType = $reader.ReadUInt16()
        if ($certificateType -ne 2 -or
            $certificateLength -lt 8 -or
            $certificateLength -gt $certificateTableSize) {
            throw "La tabla PE no contiene WIN_CERT_TYPE_PKCS_SIGNED_DATA valido."
        }
        $encoded = $reader.ReadBytes([int]($certificateLength - 8))
    } finally {
        $reader.Dispose()
        $stream.Dispose()
    }

    $derLength = Get-DerObjectLength -Bytes $encoded
    if ($derLength -ne $encoded.Length) {
        $encoded = [byte[]]$encoded[0..($derLength - 1)]
    }
    $cms = [System.Security.Cryptography.Pkcs.SignedCms]::new()
    $cms.Decode($encoded)
    return $cms
}

function Get-WindowsAuthenticodeAlgorithms {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    Add-Type -AssemblyName System.Formats.Asn1
    $cms = Get-WindowsAuthenticodeSignedCms -Path $Path
    if ($cms.SignerInfos.Count -ne 1) {
        throw "Se esperaba exactamente una firma Authenticode primaria en $Path"
    }
    $primarySigner = $cms.SignerInfos[0]
    $fileDigestAlgorithm = ConvertFrom-WindowsDigestOid `
        -Oid $primarySigner.DigestAlgorithm.Value
    $rfc3161Oid = '1.3.6.1.4.1.311.3.3.1'
    $timestampAttributes = @(
        $primarySigner.UnsignedAttributes |
            Where-Object { $_.Oid.Value -eq $rfc3161Oid }
    )
    if ($timestampAttributes.Count -ne 1 -or
        $timestampAttributes[0].Values.Count -ne 1) {
        return [pscustomobject]@{
            FileDigestAlgorithm = $fileDigestAlgorithm
            TimestampProtocol = "NONE_OR_LEGACY"
            TimestampDigestAlgorithm = $null
        }
    }

    $timestampCms = [System.Security.Cryptography.Pkcs.SignedCms]::new()
    $timestampCms.Decode(
        [byte[]]$timestampAttributes[0].Values[0].RawData
    )
    if ($timestampCms.ContentInfo.ContentType.Value -ne
        '1.2.840.113549.1.9.16.1.4') {
        throw "El atributo RFC3161 no contiene TSTInfo."
    }
    $tstInfo = [System.Formats.Asn1.AsnReader]::new(
        $timestampCms.ContentInfo.Content,
        [System.Formats.Asn1.AsnEncodingRules]::DER
    )
    $sequence = $tstInfo.ReadSequence()
    [void]$sequence.ReadInteger()
    [void]$sequence.ReadObjectIdentifier()
    $messageImprint = $sequence.ReadSequence()
    $algorithmIdentifier = $messageImprint.ReadSequence()
    $timestampDigestOid = $algorithmIdentifier.ReadObjectIdentifier()

    return [pscustomobject]@{
        FileDigestAlgorithm = $fileDigestAlgorithm
        TimestampProtocol = "RFC3161"
        TimestampDigestAlgorithm = ConvertFrom-WindowsDigestOid `
            -Oid $timestampDigestOid
    }
}

function Assert-WindowsAuthenticodeFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$SignToolPath,
        [string]$ExpectedThumbprint,
        [switch]$RequireSha256Rfc3161
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "No existe el fichero Authenticode: $Path"
    }
    if (-not (Test-Path -LiteralPath $SignToolPath -PathType Leaf)) {
        throw "No existe signtool.exe: $SignToolPath"
    }

    $verifyOutput = @(
        & $SignToolPath verify /pa /all /v $Path 2>&1
    )
    $verifyExitCode = $LASTEXITCODE
    if ($verifyExitCode -ne 0) {
        throw "SignTool no pudo verificar ${Path} (codigo $verifyExitCode): $($verifyOutput -join [Environment]::NewLine)"
    }

    $signature = Get-AuthenticodeSignature -FilePath $Path
    if ($signature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
        throw "Firma Authenticode no valida en ${Path}: $($signature.Status) $($signature.StatusMessage)"
    }
    if ($null -eq $signature.SignerCertificate) {
        throw "Falta certificado firmante en $Path"
    }
    if ($null -eq $signature.TimeStamperCertificate) {
        throw "Falta sello de tiempo RFC3161 en $Path"
    }
    if (-not [string]::IsNullOrWhiteSpace($ExpectedThumbprint)) {
        $expected = Get-NormalizedWindowsSigningThumbprint `
            -Thumbprint $ExpectedThumbprint
        $actual = $signature.SignerCertificate.Thumbprint.ToUpperInvariant()
        if ($actual -ne $expected) {
            throw "Firmante Authenticode inesperado en ${Path}: $actual"
        }
    }
    if ($RequireSha256Rfc3161) {
        $algorithms = Get-WindowsAuthenticodeAlgorithms -Path $Path
        if ($algorithms.FileDigestAlgorithm -ne 'SHA256' -or
            $algorithms.TimestampProtocol -ne 'RFC3161' -or
            $algorithms.TimestampDigestAlgorithm -ne 'SHA256') {
            throw "La firma de $Path no usa SHA-256 con timestamp RFC3161/SHA-256."
        }
    }
    return $signature
}

function Invoke-WindowsAuthenticodeSign {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$SignToolPath,
        [Parameter(Mandatory = $true)]
        [string]$ExpectedThumbprint,
        [Parameter(Mandatory = $true)]
        [string]$TimestampUrl
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "No existe el fichero que se debe firmar: $Path"
    }
    if (-not (Test-Path -LiteralPath $SignToolPath -PathType Leaf)) {
        throw "No existe signtool.exe: $SignToolPath"
    }
    $thumbprint = Get-NormalizedWindowsSigningThumbprint `
        -Thumbprint $ExpectedThumbprint
    $rfc3161Url = Get-NormalizedRfc3161TimestampUrl `
        -TimestampUrl $TimestampUrl

    $signOutput = @(
        & $SignToolPath sign `
            /sha1 $thumbprint `
            /s My `
            /fd SHA256 `
            /tr $rfc3161Url `
            /td SHA256 `
            /v `
            $Path 2>&1
    )
    $signExitCode = $LASTEXITCODE
    if ($signExitCode -ne 0) {
        throw "SignTool no pudo firmar ${Path} (codigo $signExitCode): $($signOutput -join [Environment]::NewLine)"
    }

    return Assert-WindowsAuthenticodeFile `
        -Path $Path `
        -SignToolPath $SignToolPath `
        -ExpectedThumbprint $thumbprint `
        -RequireSha256Rfc3161
}
