# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"

$trustedPowerShellModulePath = Join-Path $PSHOME "Modules"
if (-not (Test-Path -LiteralPath $trustedPowerShellModulePath -PathType Container)) {
    throw "No se encuentra el directorio de modulos del motor de PowerShell activo."
}
$env:PSModulePath = [System.IO.Path]::GetFullPath(
    $trustedPowerShellModulePath
)

function Get-GrxFirmaInstallRoot {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw "LOCALAPPDATA no esta definido; no se puede resolver una instalacion por usuario segura."
    }
    $installRoot = [System.IO.Path]::GetFullPath(
        (Join-Path $env:LOCALAPPDATA "Programs\GrxFirma")
    ).TrimEnd('\', '/')
    $dataRoot = [System.IO.Path]::GetFullPath(
        (Join-Path $env:LOCALAPPDATA "GrxFirma")
    ).TrimEnd('\', '/')
    $separator = [System.IO.Path]::DirectorySeparatorChar
    if ([string]::Equals($installRoot, $dataRoot,
            [System.StringComparison]::OrdinalIgnoreCase) -or
        $installRoot.StartsWith($dataRoot + $separator,
            [System.StringComparison]::OrdinalIgnoreCase) -or
        $dataRoot.StartsWith($installRoot + $separator,
            [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "La instalacion y los datos de GrxFirma no pueden coincidir ni anidarse."
    }
    return $installRoot
}

function Assert-NoGrxFirmaReparsePoint {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $root = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if (($root.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "La ruta de instalacion no puede ser un reparse point: $Path"
    }
    $nested = @(
        Get-ChildItem -LiteralPath $Path -Force -Recurse -ErrorAction Stop |
            Where-Object {
                ($_.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
            }
    )
    if ($nested.Count -gt 0) {
        throw "La instalacion contiene reparse points y no se puede modificar con seguridad: $($nested[0].FullName)"
    }
}

function Resolve-GrxFirmaInstallPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [ValidateSet("CLI", "NativeHost", "AfirmaURI", "DesktopLauncher", "DesktopQML", "DesktopWinUI")]
        [string]$Component
    )

    $expected = [System.IO.Path]::GetFullPath(
        (Join-Path (Get-GrxFirmaInstallRoot) $Component)
    ).TrimEnd('\')
    $actual = [System.IO.Path]::GetFullPath($Path).TrimEnd('\')
    if (-not [string]::Equals(
        $actual,
        $expected,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        throw "Ruta de instalacion no permitida para ${Component}: $actual. Debe ser exactamente $expected"
    }

    $root = Get-GrxFirmaInstallRoot
    Assert-NoGrxFirmaReparsePoint -Path $root
    Assert-NoGrxFirmaReparsePoint -Path $actual
    return $actual
}

function Get-GrxFirmaInstallMarker {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )
    return Join-Path $Path ".grxfirma-install"
}

function Get-GrxFirmaSuiteOwnershipMarker {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )
    return Join-Path $Path ".grxfirma-suite-component"
}

function Set-GrxFirmaSuiteOwnershipMarker {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [ValidateSet("CLI", "NativeHost", "AfirmaURI", "DesktopLauncher", "DesktopQML", "DesktopWinUI")]
        [string]$Component
    )

    $actual = Resolve-GrxFirmaInstallPath -Path $Path -Component $Component
    if (-not (Test-GrxFirmaInstallMarker -Path $actual -Component $Component)) {
        throw "No se puede asignar propiedad de la suite sin un marcador de instalacion valido: $actual"
    }
    Set-Content `
        -LiteralPath (Get-GrxFirmaSuiteOwnershipMarker -Path $actual) `
        -Value "GrxFirma:Suite:$Component" `
        -Encoding ASCII
}

function Test-GrxFirmaSuiteOwnershipMarker {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [ValidateSet("CLI", "NativeHost", "AfirmaURI", "DesktopLauncher", "DesktopQML", "DesktopWinUI")]
        [string]$Component
    )

    $actual = Resolve-GrxFirmaInstallPath -Path $Path -Component $Component
    $marker = Get-GrxFirmaSuiteOwnershipMarker -Path $actual
    if (-not (Test-Path -LiteralPath $marker -PathType Leaf)) {
        return $false
    }
    $value = (Get-Content -LiteralPath $marker -Raw -ErrorAction Stop).Trim()
    return [string]::Equals(
        $value,
        "GrxFirma:Suite:$Component",
        [System.StringComparison]::Ordinal
    )
}

function Test-GrxFirmaInstallMarker {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Component
    )

    $marker = Get-GrxFirmaInstallMarker -Path $Path
    if (-not (Test-Path -LiteralPath $marker -PathType Leaf)) {
        return $false
    }
    $value = (Get-Content -LiteralPath $marker -Raw -ErrorAction Stop).Trim()
    return [string]::Equals(
        $value,
        "GrxFirma:$Component",
        [System.StringComparison]::Ordinal
    )
}

function Initialize-GrxFirmaInstallDirectory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [ValidateSet("CLI", "NativeHost", "AfirmaURI", "DesktopLauncher", "DesktopQML", "DesktopWinUI")]
        [string]$Component,
        [Parameter(Mandatory = $true)]
        [string]$LegacyPayload
    )

    $actual = Resolve-GrxFirmaInstallPath -Path $Path -Component $Component
    if (Test-Path -LiteralPath $actual) {
        if (-not (Test-GrxFirmaInstallMarker -Path $actual -Component $Component)) {
            $legacyPath = Join-Path $actual $LegacyPayload
            if (-not (Test-Path -LiteralPath $legacyPath -PathType Leaf)) {
                throw "La ruta contiene datos que no pertenecen a GrxFirma y no se modificara: $actual"
            }
        }
    } else {
        New-Item -ItemType Directory -Force -Path $actual | Out-Null
    }

    Set-Content `
        -LiteralPath (Get-GrxFirmaInstallMarker -Path $actual) `
        -Value "GrxFirma:$Component" `
        -Encoding ASCII
    return $actual
}

function Clear-GrxFirmaInstallDirectory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [ValidateSet("CLI", "NativeHost", "AfirmaURI", "DesktopLauncher", "DesktopQML", "DesktopWinUI")]
        [string]$Component,
        [Parameter(Mandatory = $true)]
        [string]$LegacyPayload
    )

    $actual = Initialize-GrxFirmaInstallDirectory `
        -Path $Path `
        -Component $Component `
        -LegacyPayload $LegacyPayload
    Assert-NoGrxFirmaReparsePoint -Path $actual
    Get-ChildItem -LiteralPath $actual -Force -ErrorAction Stop |
        Where-Object { $_.Name -ne ".grxfirma-install" } |
        ForEach-Object {
            Remove-Item -LiteralPath $_.FullName -Recurse -Force -ErrorAction Stop
        }
    return $actual
}

function Remove-GrxFirmaInstallDirectory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [ValidateSet("CLI", "NativeHost", "AfirmaURI", "DesktopLauncher", "DesktopQML", "DesktopWinUI")]
        [string]$Component,
        [Parameter(Mandatory = $true)]
        [string]$LegacyPayload
    )

    $actual = Resolve-GrxFirmaInstallPath -Path $Path -Component $Component
    if (-not (Test-Path -LiteralPath $actual)) {
        return
    }
    Assert-NoGrxFirmaReparsePoint -Path $actual
    if (-not (Test-GrxFirmaInstallMarker -Path $actual -Component $Component)) {
        $legacyPath = Join-Path $actual $LegacyPayload
        if (-not (Test-Path -LiteralPath $legacyPath -PathType Leaf)) {
            throw "Falta el marcador de propiedad; no se eliminara la ruta: $actual"
        }
    }
    Remove-Item -LiteralPath $actual -Recurse -Force -ErrorAction Stop
}

function Resolve-GrxFirmaBaseInstallPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $expected = Get-GrxFirmaInstallRoot
    $actual = [System.IO.Path]::GetFullPath($Path).TrimEnd('\')
    if (-not [string]::Equals(
        $actual,
        $expected,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        throw "Ruta base de instalacion no permitida: $actual. Debe ser exactamente $expected"
    }
    Assert-NoGrxFirmaReparsePoint -Path $actual
    return $actual
}

# Ruta del ejecutable de un proceso. Desde un PowerShell de 32 bits
# Process.Path de un proceso de 64 bits llega vacío: entonces se consulta WMI,
# que la devuelve sin depender de la arquitectura.
function Get-GrxFirmaProcessPath {
    param([Parameter(Mandatory = $true)] $Process)
    $path = $null
    try { $path = $Process.Path } catch { $path = $null }
    if ([string]::IsNullOrWhiteSpace($path)) {
        try {
            $path = (Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $($Process.Id)" -ErrorAction Stop).ExecutablePath
        } catch {
            $path = $null
        }
    }
    if ([string]::IsNullOrWhiteSpace($path)) { return $null }
    return [System.IO.Path]::GetFullPath($path)
}

function Stop-GrxFirmaInstalledProcesses {
    param(
        [Parameter(Mandatory = $true)] [string]$Path,
        [Parameter(Mandatory = $true)]
        [ValidateSet("CLI", "NativeHost", "AfirmaURI", "DesktopLauncher", "DesktopQML", "DesktopWinUI")]
        [string]$Component
    )

    $verified = Resolve-GrxFirmaInstallPath -Path $Path -Component $Component
    $prefix = $verified.TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar
    foreach ($process in Get-Process -ErrorAction Stop) {
        $executable = Get-GrxFirmaProcessPath -Process $process
        if (-not $executable) {
            continue
        }
        if (-not $executable.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
            continue
        }
        try {
            $process.Kill()
            if (-not $process.WaitForExit(10000)) {
                throw "El proceso $($process.Id) no termino en 10 segundos: $executable"
            }
        } catch [System.InvalidOperationException] {
            # El proceso ya termino.
        }
    }
}
