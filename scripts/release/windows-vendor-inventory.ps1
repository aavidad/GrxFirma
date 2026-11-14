# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# JD-01: inventario versionado de los PE de terceros sin firma que la
# publicación firma con la identidad oficial (SignPath o PFX propio).
#
# Antes de enviar o firmar nada, cada DLL o EXE ajeno sin firma se coteja por
# SHA-256 y nombre con packaging/windows/vendor-unsigned-pe.sha256. Lo que no
# está en el inventario se lista en el resumen del job (GITHUB_STEP_SUMMARY)
# con su huella, listo para revisarlo y copiarlo al inventario.
#
# Primera fase: aviso. Con GRXFIRMA_VENDOR_INVENTORY_ENFORCE=1 (fijado en
# release.yml) un PE fuera del inventario detiene la publicación.

Set-StrictMode -Version Latest

$script:WindowsVendorInventoryRelativePath = 'packaging/windows/vendor-unsigned-pe.sha256'

function Get-WindowsVendorInventoryPath {
    param([Parameter(Mandatory = $true)][string]$RepositoryRoot)
    return Join-Path $RepositoryRoot $script:WindowsVendorInventoryRelativePath
}

function Read-WindowsVendorInventory {
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Falta el inventario de binarios ajenos sin firma: $Path"
    }
    $byHash = @{}
    $lineNumber = 0
    foreach ($line in Get-Content -LiteralPath $Path -Encoding UTF8 -ErrorAction Stop) {
        $lineNumber++
        $trimmed = $line.Trim()
        if ($trimmed -eq '' -or $trimmed.StartsWith('#')) {
            continue
        }
        if ($trimmed -notmatch '^([0-9a-fA-F]{64}) \*([^/\\:*?"<>|]+)$') {
            throw "Línea $lineNumber no válida en el inventario ${Path}: se espera '<sha256> *<nombre>'"
        }
        $hash = $Matches[1].ToLowerInvariant()
        $name = $Matches[2].ToLowerInvariant()
        if (-not $byHash.ContainsKey($hash)) {
            $byHash[$hash] = [System.Collections.Generic.HashSet[string]]::new()
        }
        [void]$byHash[$hash].Add($name)
    }
    return $byHash
}

# Devuelve los PE ajenos de la lista de firma que no traen firma Authenticode.
# Solo funciona en Windows (Get-AuthenticodeSignature).
function Get-WindowsUnsignedVendorTargets {
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$SigningTargets)

    $unsigned = @()
    foreach ($target in $SigningTargets) {
        if ($target.Name -like 'grxfirma*.exe') {
            continue
        }
        $signature = Get-AuthenticodeSignature -FilePath $target.FullName
        if ($signature.Status -eq [System.Management.Automation.SignatureStatus]::NotSigned) {
            $unsigned += $target
        }
    }
    return $unsigned
}

function Get-WindowsVendorInventoryMisses {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Files,
        [Parameter(Mandatory = $true)][hashtable]$Inventory
    )

    $misses = @()
    foreach ($file in $Files) {
        $path = if ($file -is [System.IO.FileInfo]) { $file.FullName } else { [string]$file }
        $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256 -ErrorAction Stop).Hash.ToLowerInvariant()
        $name = [System.IO.Path]::GetFileName($path)
        if ($Inventory.ContainsKey($hash) -and $Inventory[$hash].Contains($name.ToLowerInvariant())) {
            continue
        }
        $misses += [pscustomobject]@{
            Path = $path
            Name = $name
            Sha256 = $hash
        }
    }
    return $misses
}

function Test-WindowsVendorInventoryEnforced {
    return [Environment]::GetEnvironmentVariable('GRXFIRMA_VENDOR_INVENTORY_ENFORCE') -eq '1'
}

# Coteja los PE y, si alguno no está en el inventario, lo deja en el resumen
# del job. Con -Enforce (o GRXFIRMA_VENDOR_INVENTORY_ENFORCE=1) además falla.
function Invoke-WindowsVendorInventoryCheck {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Files,
        [Parameter(Mandatory = $true)][string]$InventoryPath,
        [string]$SummaryPath = $env:GITHUB_STEP_SUMMARY,
        [switch]$Enforce
    )

    $enforced = $Enforce.IsPresent -or (Test-WindowsVendorInventoryEnforced)
    $inventory = Read-WindowsVendorInventory -Path $InventoryPath
    $misses = @(Get-WindowsVendorInventoryMisses -Files $Files -Inventory $inventory)
    if ($misses.Count -eq 0) {
        Write-Host "Inventario de binarios ajenos sin firma: los $(@($Files).Count) PE están en $InventoryPath."
        return $misses
    }

    $mode = if ($enforced) { 'error' } else { 'aviso' }
    $lines = [System.Collections.Generic.List[string]]::new()
    $lines.Add("### Binarios ajenos sin firma fuera del inventario ($mode)")
    $lines.Add('')
    $lines.Add("Estos PE no traen firma de su proveedor y se iban a firmar con el certificado oficial, pero su SHA-256 no está en ``$($script:WindowsVendorInventoryRelativePath)``. Antes de añadirlos, comprueba que salen de la instalación oficial de Qt o de la publicación de WinUI de esta misma versión.")
    $lines.Add('')
    $lines.Add('```text')
    foreach ($miss in $misses) {
        $lines.Add("$($miss.Sha256) *$($miss.Name)")
    }
    $lines.Add('```')
    $report = ($lines -join [Environment]::NewLine) + [Environment]::NewLine
    if (-not [string]::IsNullOrWhiteSpace($SummaryPath)) {
        [System.IO.File]::AppendAllText($SummaryPath, $report, [System.Text.UTF8Encoding]::new($false))
    }
    foreach ($miss in $misses) {
        Write-Warning "PE ajeno sin firma fuera del inventario: $($miss.Path) ($($miss.Sha256))"
    }
    if ($enforced) {
        throw "Hay $($misses.Count) binarios ajenos sin firma fuera del inventario; no se firma nada."
    }
    return $misses
}
