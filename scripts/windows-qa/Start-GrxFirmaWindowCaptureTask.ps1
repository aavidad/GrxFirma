# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [string]$RepositoryRoot = (Join-Path $PSScriptRoot "..\.."),
    [string]$OutputRoot,
    [ValidateSet("amd64")]
    [string]$Architecture = "amd64",
    [ValidateSet("Qt", "WinUI")]
    [string]$Frontend = "Qt",
    [ValidateSet("Auto", "PrintWindow", "WindowsGraphicsCapture")]
    [string]$CaptureMethod = "Auto",
    [ValidateRange(5, 3600)]
    [int]$DurationSeconds = 30,
    [ValidateRange(250, 60000)]
    [int]$IntervalMilliseconds = 1000,
    [ValidateRange(1, 2000)]
    [int]$MaxCaptures = 250,
    [ValidateRange(10485760, 2147483648)]
    [long]$MaxArtifactBytes = 536870912,
    [ValidateRange(1, 100)]
    [int]$MaxRetainedRuns = 20,
    [ValidateRange(1, 500)]
    [int]$MaxRetainedTaskResults = 50,
    [string]$ExecutablePath,
    [switch]$SkipBuild,
    [switch]$CloseAfterCapture,
    [Parameter(Mandatory)]
    [switch]$TestDataOnly,
    [string]$TaskName,
    [ValidateRange(30, 14400)]
    [int]$WaitTimeoutSeconds = 3600,
    [switch]$NoWait,
    [switch]$KeepTask
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows) {
    throw "La tarea interactiva requiere Windows 10 o posterior."
}
if ([System.Environment]::OSVersion.Version.Major -lt 10) {
    throw "La tarea no es compatible con versiones anteriores a Windows 10."
}
if ($PSVersionTable.PSVersion.Major -lt 7) {
    throw "El lanzador requiere PowerShell 7 o posterior."
}
if (-not $TestDataOnly) {
    throw "Use -TestDataOnly y abra unicamente datos, certificados y documentos sinteticos."
}
if ($WaitTimeoutSeconds -le ($DurationSeconds + 10)) {
    throw "WaitTimeoutSeconds debe superar DurationSeconds en al menos 10 segundos."
}

function Assert-LocalPathWithoutReparsePoint {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string]$Label
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if (
        $fullPath.StartsWith(
            "\\",
            [System.StringComparison]::Ordinal
        )
    ) {
        throw "$Label debe residir en un volumen local; no se permiten rutas UNC o de dispositivo."
    }
    $pathRoot = [System.IO.Path]::GetPathRoot($fullPath)
    if ([string]::IsNullOrWhiteSpace($pathRoot)) {
        throw "No se pudo determinar el volumen de $Label."
    }
    try {
        $drive = [System.IO.DriveInfo]::new($pathRoot)
    } catch {
        throw "No se pudo verificar el tipo de volumen de ${Label}: $($_.Exception.Message)"
    }
    if ($drive.DriveType -eq [System.IO.DriveType]::Network) {
        throw "$Label no puede residir en una unidad de red mapeada."
    }
    $current = $fullPath.TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )
    while (-not [string]::IsNullOrWhiteSpace($current)) {
        if (Test-Path -LiteralPath $current) {
            $item = Get-Item -LiteralPath $current -Force
            if (
                ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
            ) {
                throw "$Label no puede atravesar puntos de reanalisis: $current"
            }
        }
        $parent = [System.IO.Path]::GetDirectoryName($current)
        if (
            [string]::IsNullOrWhiteSpace($parent) -or
            $parent.Equals($current, [System.StringComparison]::OrdinalIgnoreCase)
        ) {
            break
        }
        $current = $parent.TrimEnd(
            [System.IO.Path]::DirectorySeparatorChar,
            [System.IO.Path]::AltDirectorySeparatorChar
        )
    }
}

function Protect-TaskResultDirectory {
    param(
        [Parameter(Mandatory)]
        [string]$Path
    )

    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    $systemUser = [System.Security.Principal.SecurityIdentifier]::new("S-1-5-18")
    $inheritance = [System.Security.AccessControl.InheritanceFlags]::ContainerInherit -bor
        [System.Security.AccessControl.InheritanceFlags]::ObjectInherit
    $propagation = [System.Security.AccessControl.PropagationFlags]::None
    $rights = [System.Security.AccessControl.FileSystemRights]::FullControl
    $allow = [System.Security.AccessControl.AccessControlType]::Allow
    $security = [System.Security.AccessControl.DirectorySecurity]::new()
    $security.SetOwner($currentUser)
    $security.SetAccessRuleProtection($true, $false)
    [void]$security.AddAccessRule(
        [System.Security.AccessControl.FileSystemAccessRule]::new(
            $currentUser,
            $rights,
            $inheritance,
            $propagation,
            $allow
        )
    )
    [void]$security.AddAccessRule(
        [System.Security.AccessControl.FileSystemAccessRule]::new(
            $systemUser,
            $rights,
            $inheritance,
            $propagation,
            $allow
        )
    )
    Set-Acl -LiteralPath $Path -AclObject $security

    $verified = Get-Acl -LiteralPath $Path
    if (-not $verified.AreAccessRulesProtected) {
        throw "La DACL del resultado de tarea conserva herencia no controlada: $Path"
    }
    $owner = $verified.GetOwner(
        [System.Security.Principal.SecurityIdentifier]
    )
    if ($owner.Value -ne $currentUser.Value) {
        throw "El directorio de resultado no pertenece al usuario actual: $Path"
    }
    $allowedSids = @($currentUser.Value, $systemUser.Value)
    $rules = $verified.GetAccessRules(
        $true,
        $true,
        [System.Security.Principal.SecurityIdentifier]
    )
    $verifiedSids = [System.Collections.Generic.HashSet[string]]::new()
    foreach ($rule in $rules) {
        if (
            $allowedSids -notcontains $rule.IdentityReference.Value -or
            $rule.AccessControlType -ne $allow -or
            ($rule.FileSystemRights -band $rights) -ne $rights
        ) {
            throw "La DACL del resultado contiene una regla no permitida: $Path"
        }
        [void]$verifiedSids.Add($rule.IdentityReference.Value)
    }
    foreach ($allowedSid in $allowedSids) {
        if (-not $verifiedSids.Contains($allowedSid)) {
            throw "La DACL del resultado no concede acceso a una identidad obligatoria: $Path"
        }
    }
}

function Test-TaskResultPointerDeletionCandidate {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string]$TaskResultDirectory
    )

    try {
        $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
        $root = [System.IO.Path]::GetFullPath($TaskResultDirectory).TrimEnd(
            [System.IO.Path]::DirectorySeparatorChar,
            [System.IO.Path]::AltDirectorySeparatorChar
        )
        $parent = [System.IO.Path]::GetDirectoryName(
            [System.IO.Path]::GetFullPath($item.FullName)
        ).TrimEnd(
            [System.IO.Path]::DirectorySeparatorChar,
            [System.IO.Path]::AltDirectorySeparatorChar
        )
        if (
            $item.PSIsContainer -or
            -not $parent.Equals($root, [System.StringComparison]::OrdinalIgnoreCase) -or
            $item.Name -notmatch "^\d{8}T\d{9}Z-[a-f0-9]{8}\.json$" -or
            ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
        ) {
            return $false
        }

        $pointer = Get-Content -LiteralPath $item.FullName -Raw -ErrorAction Stop |
            ConvertFrom-Json -ErrorAction Stop
        $expectedId = [System.IO.Path]::GetFileNameWithoutExtension($item.Name)
        if (
            $null -eq $pointer.PSObject.Properties["schemaVersion"] -or
            $pointer.schemaVersion -ne 1 -or
            $null -eq $pointer.PSObject.Properties["tool"] -or
            $pointer.tool -ne "GrxFirma window-only QA task pointer" -or
            $null -eq $pointer.PSObject.Properties["pointerId"] -or
            $pointer.pointerId -ne $expectedId -or
            $null -eq $pointer.PSObject.Properties["status"] -or
            $pointer.status -notin @("running", "succeeded", "failed")
        ) {
            return $false
        }
        return $true
    } catch {
        # Fail-closed: un puntero que no pueda verificarse nunca se borra.
        return $false
    }
}

function Remove-ExpiredTaskResultPointers {
    param(
        [Parameter(Mandatory)]
        [string]$TaskResultDirectory,
        [Parameter(Mandatory)]
        [int]$RetainedPointers
    )

    $eligible = @()
    foreach (
        $item in Get-ChildItem -LiteralPath $TaskResultDirectory -File -Force `
            -ErrorAction Stop
    ) {
        if (
            Test-TaskResultPointerDeletionCandidate `
                -Path $item.FullName `
                -TaskResultDirectory $TaskResultDirectory
        ) {
            $eligible += $item
        }
    }
    $keepExisting = [Math]::Max(0, $RetainedPointers - 1)
    $expired = @(
        $eligible |
            Sort-Object Name -Descending |
            Select-Object -Skip $keepExisting
    )
    $removed = 0
    foreach ($item in $expired) {
        # Revalida nombre, ubicación, contenido y reparse point justo antes del
        # borrado; no confía en el FileInfo de la primera enumeración.
        if (
            -not (
                Test-TaskResultPointerDeletionCandidate `
                    -Path $item.FullName `
                    -TaskResultDirectory $TaskResultDirectory
            )
        ) {
            continue
        }
        Remove-Item -LiteralPath $item.FullName -Force -ErrorAction Stop
        $removed++
    }
    return $removed
}

function Remove-TaskResultPointerSafely {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string]$TaskResultDirectory
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }
    if (
        -not (
            Test-TaskResultPointerDeletionCandidate `
                -Path $Path `
                -TaskResultDirectory $TaskResultDirectory
        )
    ) {
        return $false
    }
    Remove-Item -LiteralPath $Path -Force -ErrorAction Stop
    return $true
}

function Quote-TaskArgument {
    param(
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$Value
    )

    if ($Value.Contains('"')) {
        throw "Los argumentos de tarea no pueden contener comillas dobles."
    }
    # CommandLineToArgvW interpreta las barras finales antes de la comilla de
    # cierre. Duplicarlas conserva rutas como C:\qa\ sin perder el delimitador.
    $escaped = [regex]::Replace(
        $Value,
        "(\\+)$",
        {
            param($match)
            return $match.Groups[1].Value + $match.Groups[1].Value
        }
    )
    return '"' + $escaped + '"'
}

$resolvedRepositoryRoot = (
    Resolve-Path -LiteralPath $RepositoryRoot -ErrorAction Stop
).Path
$captureScript = Join-Path `
    $resolvedRepositoryRoot `
    "scripts\windows-qa\Invoke-GrxFirmaWindowCapture.ps1"
if (-not (Test-Path -LiteralPath $captureScript -PathType Leaf)) {
    throw "No se encontro el capturador interactivo: $captureScript"
}

$interactiveUser = (
    Get-CimInstance -ClassName Win32_ComputerSystem -Property UserName
).UserName
if ([string]::IsNullOrWhiteSpace($interactiveUser)) {
    throw "No hay un usuario con sesion grafica iniciada."
}
$currentIdentity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
$interactiveSid = (
    [System.Security.Principal.NTAccount]::new($interactiveUser)
).Translate([System.Security.Principal.SecurityIdentifier])
if ($interactiveSid.Value -ne $currentIdentity.User.Value) {
    throw (
        "La sesion SSH pertenece a $($currentIdentity.Name), pero la sesion grafica " +
        "pertenece a $interactiveUser. Ejecute SSH con el mismo usuario; no se solicitan " +
        "ni almacenan contrasenas para saltar entre cuentas."
    )
}

# La compilacion no necesita la sesion grafica. Se ejecuta en el proceso SSH,
# que conserva QMAKE/WINDEPLOYQT y el entorno vcvars/nmake preparado por quien
# invoca el lanzador. La tarea interactiva se limita a abrir y capturar la app.
$buildPerformedByLauncher = -not $SkipBuild
if (-not $SkipBuild) {
    $buildScriptRelative = if ($Frontend -eq "WinUI") {
        "packaging\windows\build-desktop-winui.ps1"
    } else {
        "packaging\windows\build-desktop-qml.ps1"
    }
    $buildScript = Join-Path $resolvedRepositoryRoot $buildScriptRelative
    if (-not (Test-Path -LiteralPath $buildScript -PathType Leaf)) {
        throw "No se encontro el script de build para el frontend $Frontend."
    }
    $previousArchitecture = $env:GOARCH
    try {
        $env:GOARCH = $Architecture
        & $buildScript
    } finally {
        $env:GOARCH = $previousArchitecture
    }
}

if ([string]::IsNullOrWhiteSpace($ExecutablePath)) {
    $relativeExecutable = if ($Frontend -eq "WinUI") {
        "release\windows-desktop-winui\" +
            "GrxFirma-$((Get-Content -LiteralPath (Join-Path $resolvedRepositoryRoot "VERSION.txt") -Raw).Trim())-desktop-winui-windows-$Architecture\" +
            "app\grxfirma-gui.exe"
    } else {
        "release\windows-desktop-qml\" +
            "GrxFirma-$((Get-Content -LiteralPath (Join-Path $RepositoryRoot "VERSION.txt") -Raw).Trim())-desktop-qml-windows-$Architecture\" +
            "grxfirma-gui-qml.exe"
    }
    $ExecutablePath = Join-Path $resolvedRepositoryRoot $relativeExecutable
} elseif (-not [System.IO.Path]::IsPathFullyQualified($ExecutablePath)) {
    $ExecutablePath = Join-Path $resolvedRepositoryRoot $ExecutablePath
}
$ExecutablePath = (
    Resolve-Path -LiteralPath $ExecutablePath -ErrorAction Stop
).Path
$expectedExecutableName = if ($Frontend -eq "WinUI") {
    "grxfirma-gui.exe"
} else {
    "grxfirma-gui-qml.exe"
}
if ((Split-Path -Leaf $ExecutablePath) -ine $expectedExecutableName) {
    throw "El ejecutable no corresponde al frontend $Frontend."
}
Assert-LocalPathWithoutReparsePoint `
    -Path $ExecutablePath `
    -Label "ExecutablePath"

if (-not [string]::IsNullOrWhiteSpace($OutputRoot)) {
    if (-not [System.IO.Path]::IsPathFullyQualified($OutputRoot)) {
        $OutputRoot = Join-Path $resolvedRepositoryRoot $OutputRoot
    }
    $OutputRoot = [System.IO.Path]::GetFullPath($OutputRoot)
    Assert-LocalPathWithoutReparsePoint -Path $OutputRoot -Label "OutputRoot"
}

$pwsh = Get-Command pwsh.exe -ErrorAction Stop
if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
    throw "LOCALAPPDATA no esta definido."
}
$taskResultDirectory = Join-Path `
    $env:LOCALAPPDATA `
    "GrxFirma\QA\window-captures\task-results"
Assert-LocalPathWithoutReparsePoint `
    -Path $taskResultDirectory `
    -Label "taskResultDirectory"
if (-not (Test-Path -LiteralPath $taskResultDirectory -PathType Container)) {
    New-Item -ItemType Directory -Path $taskResultDirectory -Force | Out-Null
}
Protect-TaskResultDirectory -Path $taskResultDirectory
[void](Remove-ExpiredTaskResultPointers `
    -TaskResultDirectory $taskResultDirectory `
    -RetainedPointers $MaxRetainedTaskResults)

$taskId = "{0}-{1}" -f (
    [DateTimeOffset]::UtcNow.ToString("yyyyMMddTHHmmssfffZ")
), ([guid]::NewGuid().ToString("N").Substring(0, 8))
if ([string]::IsNullOrWhiteSpace($TaskName)) {
    $TaskName = "GrxFirma-QA-$taskId"
}
if ($TaskName -notmatch "^[A-Za-z0-9_.-]+$") {
    throw "TaskName solo puede contener letras ASCII, numeros, punto, guion y guion bajo."
}
$resultPointer = Join-Path $taskResultDirectory "$taskId.json"

$captureArguments = @(
    "-NoLogo",
    "-NoProfile",
    "-NonInteractive",
    "-ExecutionPolicy",
    "Bypass",
    "-File",
    $captureScript,
    "-RepositoryRoot",
    $resolvedRepositoryRoot,
    "-Architecture",
    $Architecture,
    "-Frontend",
    $Frontend,
    "-CaptureMethod",
    $CaptureMethod,
    "-DurationSeconds",
    $DurationSeconds.ToString([System.Globalization.CultureInfo]::InvariantCulture),
    "-IntervalMilliseconds",
    $IntervalMilliseconds.ToString([System.Globalization.CultureInfo]::InvariantCulture),
    "-MaxCaptures",
    $MaxCaptures.ToString([System.Globalization.CultureInfo]::InvariantCulture),
    "-MaxArtifactBytes",
    $MaxArtifactBytes.ToString([System.Globalization.CultureInfo]::InvariantCulture),
    "-MaxRetainedRuns",
    $MaxRetainedRuns.ToString([System.Globalization.CultureInfo]::InvariantCulture),
    "-ExecutablePath",
    $ExecutablePath,
    "-ResultPointerPath",
    $resultPointer,
    "-TestDataOnly",
    "-SkipBuild"
)
if ($buildPerformedByLauncher) {
    $captureArguments += "-BuildPerformedBeforeLaunch"
}
if (-not [string]::IsNullOrWhiteSpace($OutputRoot)) {
    $captureArguments += @("-OutputRoot", $OutputRoot)
}
if ($CloseAfterCapture) {
    $captureArguments += "-CloseAfterCapture"
}
$taskArgumentString = (
    $captureArguments |
        ForEach-Object { Quote-TaskArgument -Value ([string]$_) }
) -join " "

Import-Module ScheduledTasks -ErrorAction Stop
$action = New-ScheduledTaskAction `
    -Execute $pwsh.Source `
    -Argument $taskArgumentString `
    -WorkingDirectory $resolvedRepositoryRoot
$principal = New-ScheduledTaskPrincipal `
    -UserId $interactiveUser `
    -LogonType Interactive `
    -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -ExecutionTimeLimit (New-TimeSpan -Seconds $WaitTimeoutSeconds) `
    -MultipleInstances IgnoreNew

$registered = $false
$completed = $false
try {
    Register-ScheduledTask `
        -TaskName $TaskName `
        -TaskPath "\" `
        -Action $action `
        -Principal $principal `
        -Settings $settings | Out-Null
    $registered = $true
    Start-ScheduledTask -TaskName $TaskName -TaskPath "\"

    Write-Output "Tarea iniciada en la sesion grafica: $TaskName"
    Write-Output "Puntero de resultado: $resultPointer"
    if ($NoWait) {
        Write-Output (
            "La tarea queda registrada. Al terminar: " +
            "Unregister-ScheduledTask -TaskName '$TaskName' " +
            "-TaskPath '\' -Confirm:`$false"
        )
        return
    }

    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($WaitTimeoutSeconds)
    $pointer = $null
    while ([DateTimeOffset]::UtcNow -lt $deadline) {
        if (Test-Path -LiteralPath $resultPointer -PathType Leaf) {
            try {
                $pointer = Get-Content -LiteralPath $resultPointer -Raw |
                    ConvertFrom-Json
            } catch {
                $pointer = $null
            }
            if (
                $null -ne $pointer -and
                $pointer.status -in @("succeeded", "failed")
            ) {
                $completed = $true
                break
            }
        }

        $task = Get-ScheduledTask `
            -TaskName $TaskName `
            -TaskPath "\" `
            -ErrorAction Stop
        $taskInfo = Get-ScheduledTaskInfo `
            -TaskName $TaskName `
            -TaskPath "\" `
            -ErrorAction Stop
        if (
            $task.State -ne "Running" -and
            $taskInfo.LastRunTime -gt [datetime]::MinValue
        ) {
            # Da tiempo a que el movimiento atomico del puntero final sea
            # visible despues de terminar pwsh.
            Start-Sleep -Milliseconds 500
            if (Test-Path -LiteralPath $resultPointer -PathType Leaf) {
                try {
                    $pointer = Get-Content -LiteralPath $resultPointer -Raw |
                        ConvertFrom-Json
                } catch {
                    $pointer = $null
                }
            }
            if (
                $null -ne $pointer -and
                $pointer.status -in @("succeeded", "failed")
            ) {
                $completed = $true
                break
            }
            throw (
                "La tarea termino sin producir un puntero final. " +
                "LastTaskResult=$($taskInfo.LastTaskResult)"
            )
        }
        Start-Sleep -Seconds 1
    }
    if (-not $completed) {
        throw (
            "Tiempo agotado esperando la tarea $TaskName; se detendra y retirara " +
            "su definicion salvo que se haya solicitado KeepTask."
        )
    }
    if ($pointer.status -ne "succeeded") {
        throw "La captura interactiva fallo: $($pointer.failure)"
    }

    Write-Output "Captura completada: $($pointer.runDirectory)"
    Write-Output "Manifiesto: $($pointer.manifestPath)"
} finally {
    try {
        if (
            $registered -and
            -not $NoWait -and
            -not $KeepTask
        ) {
            try {
                $cleanupTask = Get-ScheduledTask `
                    -TaskName $TaskName `
                    -TaskPath "\" `
                    -ErrorAction SilentlyContinue
                if ($null -ne $cleanupTask -and $cleanupTask.State -eq "Running") {
                    Stop-ScheduledTask `
                        -TaskName $TaskName `
                        -TaskPath "\" `
                        -ErrorAction Stop
                }
            } finally {
                Unregister-ScheduledTask `
                    -TaskName $TaskName `
                    -TaskPath "\" `
                    -Confirm:$false `
                    -ErrorAction Stop
            }
        }
    } finally {
        if (-not $NoWait -and -not $KeepTask) {
            [void](Remove-TaskResultPointerSafely `
                -Path $resultPointer `
                -TaskResultDirectory $taskResultDirectory)
        }
    }
}
