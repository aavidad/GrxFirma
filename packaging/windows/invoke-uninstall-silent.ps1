# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

param(
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$UninstallerPath,
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$InstallDir,
    [switch]$ValidateOnly
)

$ErrorActionPreference = "Stop"

function Assert-RegularFileNoReparse {
    param([string]$Path)

    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ($item.PSIsContainer -or
        ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "El fichero no es regular o es un punto de reanalisis: $Path"
    }
}

function Assert-DirectoryNoReparse {
    param([string]$Path)

    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if (-not $item.PSIsContainer -or
        ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "El directorio no es regular o es un punto de reanalisis: $Path"
    }
}

function Assert-NoReparseTree {
    param([string]$Path)

    $pending = New-Object "System.Collections.Generic.Queue[string]"
    $pending.Enqueue($Path)
    while ($pending.Count -gt 0) {
        $current = $pending.Dequeue()
        Assert-DirectoryNoReparse -Path $current
        foreach ($child in Get-ChildItem -LiteralPath $current -Force -ErrorAction Stop) {
            if (($child.Attributes -band
                [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "El arbol de desinstalacion contiene un punto de reanalisis: $($child.FullName)"
            }
            if ($child.PSIsContainer) {
                $pending.Enqueue($child.FullName)
            }
        }
    }
}

function Assert-ExclusiveDirectoryAcl {
    param(
        [string]$Path,
        [System.Security.Principal.SecurityIdentifier]$OwnerSid
    )

    Assert-DirectoryNoReparse -Path $Path
    $aclSections =
        [System.Security.AccessControl.AccessControlSections]::Access -bor
        [System.Security.AccessControl.AccessControlSections]::Owner
    $acl = [System.Security.AccessControl.DirectorySecurity]::new(
        $Path,
        $aclSections
    )
    if (-not $acl.AreAccessRulesProtected) {
        throw "El directorio temporal conserva permisos heredados."
    }
    $aclOwnerSid = $acl.GetOwner(
        [System.Security.Principal.SecurityIdentifier]
    )
    if ($aclOwnerSid -ne $OwnerSid) {
        throw "El directorio temporal no pertenece al usuario actual."
    }

    $ownerHasFullControl = $false
    $rules = $acl.GetAccessRules(
        $true,
        $true,
        [System.Security.Principal.SecurityIdentifier]
    )
    foreach ($rule in $rules) {
        if ($rule.AccessControlType -ne
            [System.Security.AccessControl.AccessControlType]::Allow) {
            continue
        }
        if ($rule.IdentityReference -ne $OwnerSid) {
            throw "El directorio temporal concede acceso a otra identidad."
        }
        if (($rule.FileSystemRights -band
            [System.Security.AccessControl.FileSystemRights]::FullControl) -eq
            [System.Security.AccessControl.FileSystemRights]::FullControl) {
            $ownerHasFullControl = $true
        }
    }
    if (-not $ownerHasFullControl) {
        throw "El usuario actual no controla el directorio temporal."
    }
}

function New-ExclusiveTemporaryDirectory {
    param(
        [string]$Path,
        [System.Security.Principal.SecurityIdentifier]$OwnerSid
    )

    $security = New-Object System.Security.AccessControl.DirectorySecurity
    $security.SetAccessRuleProtection($true, $false)
    $security.SetOwner($OwnerSid)
    $rights = [System.Security.AccessControl.FileSystemRights]::FullControl
    $inheritance =
        [System.Security.AccessControl.InheritanceFlags]::ContainerInherit -bor
        [System.Security.AccessControl.InheritanceFlags]::ObjectInherit
    $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
        $OwnerSid,
        $rights,
        $inheritance,
        [System.Security.AccessControl.PropagationFlags]::None,
        [System.Security.AccessControl.AccessControlType]::Allow
    )
    [void]$security.AddAccessRule($rule)
    if ($PSVersionTable.PSEdition -eq "Core") {
        $directoryInfo = [System.IO.DirectoryInfo]::new($Path)
        [System.IO.FileSystemAclExtensions]::Create(
            $directoryInfo,
            $security
        )
    } else {
        [void][System.IO.Directory]::CreateDirectory($Path, $security)
    }
}

function Copy-RegularFileExclusive {
    param(
        [string]$Source,
        [string]$Destination
    )

    Assert-RegularFileNoReparse -Path $Source
    $sourceStream = $null
    $destinationStream = $null
    try {
        $sourceStream = New-Object System.IO.FileStream(
            $Source,
            [System.IO.FileMode]::Open,
            [System.IO.FileAccess]::Read,
            [System.IO.FileShare]::Read
        )
        Assert-RegularFileNoReparse -Path $Source
        $destinationStream = New-Object System.IO.FileStream(
            $Destination,
            [System.IO.FileMode]::CreateNew,
            [System.IO.FileAccess]::Write,
            [System.IO.FileShare]::None
        )
        $sourceStream.CopyTo($destinationStream)
        $destinationStream.Flush($true)
    } finally {
        if ($null -ne $destinationStream) {
            $destinationStream.Dispose()
        }
        if ($null -ne $sourceStream) {
            $sourceStream.Dispose()
        }
    }
    Assert-RegularFileNoReparse -Path $Destination
}

function Remove-TemporaryUninstallerSafely {
    param(
        [string]$TemporaryRoot,
        [string]$TemporaryDirectory,
        [string]$TemporaryUninstaller,
        [System.Security.Principal.SecurityIdentifier]$OwnerSid
    )

    try {
        Assert-DirectoryNoReparse -Path $TemporaryRoot
        Assert-ExclusiveDirectoryAcl `
            -Path $TemporaryDirectory `
            -OwnerSid $OwnerSid
        if (Test-Path -LiteralPath $TemporaryUninstaller) {
            Assert-RegularFileNoReparse -Path $TemporaryUninstaller
            $fileRemoved = $false
            for ($attempt = 0; $attempt -lt 20; $attempt++) {
                try {
                    Remove-Item `
                        -LiteralPath $TemporaryUninstaller `
                        -Force `
                        -ErrorAction Stop
                    $fileRemoved = $true
                    break
                } catch [System.IO.IOException],
                        [System.UnauthorizedAccessException] {
                    if ($attempt -eq 19) {
                        throw
                    }
                    Start-Sleep -Milliseconds 100
                }
            }
            if (-not $fileRemoved) {
                throw "No se pudo retirar el desinstalador temporal."
            }
        }
        Assert-ExclusiveDirectoryAcl `
            -Path $TemporaryDirectory `
            -OwnerSid $OwnerSid
        $remaining = @(Get-ChildItem -LiteralPath $TemporaryDirectory -Force -ErrorAction Stop)
        if ($remaining.Count -ne 0) {
            throw "El directorio temporal contiene ficheros inesperados."
        }
        $directoryRemoved = $false
        for ($attempt = 0; $attempt -lt 20; $attempt++) {
            try {
                [System.IO.Directory]::Delete($TemporaryDirectory, $false)
                $directoryRemoved = $true
                break
            } catch [System.IO.IOException],
                    [System.UnauthorizedAccessException] {
                if ($attempt -eq 19) {
                    throw
                }
                Start-Sleep -Milliseconds 100
            }
        }
        if (-not $directoryRemoved) {
            throw "No se pudo retirar el directorio temporal."
        }
        return -not (Test-Path -LiteralPath $TemporaryDirectory)
    } catch {
        [Console]::Error.WriteLine(
            "No se pudo limpiar de forma segura el desinstalador temporal: $($_.Exception.Message)"
        )
        return $false
    }
}

$normalizedInstallDir = [System.IO.Path]::GetFullPath($InstallDir).TrimEnd('\', '/')
$normalizedUninstallerPath = [System.IO.Path]::GetFullPath($UninstallerPath)
$expectedUninstallerPath = [System.IO.Path]::GetFullPath(
    (Join-Path $normalizedInstallDir "uninstall.exe")
)
if (-not [string]::Equals(
    $normalizedUninstallerPath,
    $expectedUninstallerPath,
    [System.StringComparison]::OrdinalIgnoreCase
)) {
    throw "El desinstalador debe estar dentro del directorio de instalacion."
}

Assert-NoReparseTree -Path $normalizedInstallDir
Assert-RegularFileNoReparse -Path $normalizedUninstallerPath
if ($ValidateOnly) {
    exit 0
}

$temporaryRoot = [System.IO.Path]::GetFullPath(
    [System.IO.Path]::GetTempPath()
).TrimEnd('\', '/')
$temporaryDirectory = Join-Path `
    $temporaryRoot `
    ("GrxFirma-Uninstall-" + [guid]::NewGuid().ToString("N"))
$temporaryUninstaller = Join-Path `
    $temporaryDirectory `
    ("uninstall-" + [guid]::NewGuid().ToString("N") + ".exe")
$currentSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
$exitCode = 1603
$temporaryDirectoryCreated = $false
try {
    Assert-NoReparseTree -Path $normalizedInstallDir
    Assert-RegularFileNoReparse -Path $normalizedUninstallerPath
    Assert-DirectoryNoReparse -Path $temporaryRoot
    New-ExclusiveTemporaryDirectory `
        -Path $temporaryDirectory `
        -OwnerSid $currentSid
    $temporaryDirectoryCreated = $true
    Assert-ExclusiveDirectoryAcl `
        -Path $temporaryDirectory `
        -OwnerSid $currentSid

    Assert-DirectoryNoReparse -Path $temporaryRoot
    Assert-ExclusiveDirectoryAcl `
        -Path $temporaryDirectory `
        -OwnerSid $currentSid
    Assert-NoReparseTree -Path $normalizedInstallDir
    Assert-RegularFileNoReparse -Path $normalizedUninstallerPath
    Copy-RegularFileExclusive `
        -Source $normalizedUninstallerPath `
        -Destination $temporaryUninstaller

    Assert-DirectoryNoReparse -Path $temporaryRoot
    Assert-ExclusiveDirectoryAcl `
        -Path $temporaryDirectory `
        -OwnerSid $currentSid
    Assert-RegularFileNoReparse -Path $temporaryUninstaller
    $executionGuard = New-Object System.IO.FileStream(
        $temporaryUninstaller,
        [System.IO.FileMode]::Open,
        [System.IO.FileAccess]::Read,
        [System.IO.FileShare]::Read
    )
    try {
        Assert-DirectoryNoReparse -Path $temporaryRoot
        Assert-ExclusiveDirectoryAcl `
            -Path $temporaryDirectory `
            -OwnerSid $currentSid
        Assert-NoReparseTree -Path $normalizedInstallDir
        Assert-RegularFileNoReparse -Path $normalizedUninstallerPath
        Assert-RegularFileNoReparse -Path $temporaryUninstaller
        $process = Start-Process `
            -FilePath $temporaryUninstaller `
            -ArgumentList @("/S", "_?=$normalizedInstallDir") `
            -PassThru `
            -ErrorAction Stop
        if (-not $process.WaitForExit(300000)) {
            $process.Kill()
            if (-not $process.WaitForExit(10000)) {
                throw "El desinstalador no termino tras detener el proceso."
            }
            throw "La desinstalacion supero los 5 minutos."
        }
        $exitCode = $process.ExitCode
    } finally {
        $executionGuard.Dispose()
    }
} catch {
    [Console]::Error.WriteLine(
        "No se pudo ejecutar el desinstalador silencioso: $($_.Exception.Message)"
    )
} finally {
    if ($temporaryDirectoryCreated) {
        $cleaned = Remove-TemporaryUninstallerSafely `
            -TemporaryRoot $temporaryRoot `
            -TemporaryDirectory $temporaryDirectory `
            -TemporaryUninstaller $temporaryUninstaller `
            -OwnerSid $currentSid
        if (-not $cleaned) {
            $exitCode = 1603
        }
    }
}

exit $exitCode
