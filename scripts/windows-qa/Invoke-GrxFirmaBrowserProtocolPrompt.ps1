# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [switch]$TestDataOnly,
    [Parameter(Mandatory)]
    [string]$BrowserExecutablePath,
    [Parameter(Mandatory)]
    [string]$UserDataDirectory,
    [ValidateSet("leftmost", "rightmost")]
    [string]$ButtonPosition = "rightmost",
    [Parameter(Mandatory)]
    [string]$ResultPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows -or -not $TestDataOnly) {
    throw "La aprobacion UIA requiere Windows y -TestDataOnly."
}

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

function Resolve-ExistingFile {
    param([Parameter(Mandatory)][string]$Path)

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if (-not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
        throw "No existe el ejecutable indicado."
    }
    return $fullPath
}

function Resolve-QAPath {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Purpose
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $qaRoot = [System.IO.Path]::GetFullPath(
        (Join-Path $env:LOCALAPPDATA "GrxFirma\QA")
    )
    if (
        -not $fullPath.StartsWith(
            $qaRoot + [System.IO.Path]::DirectorySeparatorChar,
            [System.StringComparison]::OrdinalIgnoreCase
        )
    ) {
        throw "$Purpose debe quedar dentro del directorio fijo de QA."
    }
    return $fullPath
}

function Write-JsonAtomically {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)]$Value
    )

    $parent = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
    }
    $json = $Value | ConvertTo-Json -Depth 6
    $temporary = "$Path.$([guid]::NewGuid().ToString('N')).tmp"
    [System.IO.File]::WriteAllText(
        $temporary,
        $json,
        [System.Text.UTF8Encoding]::new($false)
    )
    [System.IO.File]::Move($temporary, $Path, $true)
}

$browserPath = Resolve-ExistingFile -Path $BrowserExecutablePath
$profilePath = Resolve-QAPath `
    -Path $UserDataDirectory `
    -Purpose "UserDataDirectory"
$resultFullPath = Resolve-QAPath -Path $ResultPath -Purpose "ResultPath"
$currentSession = [System.Diagnostics.Process]::GetCurrentProcess().SessionId

$profileArguments = @(
    "--user-data-dir=$profilePath",
    "--user-data-dir=`"$profilePath`""
)
$browserProcesses = @(
    Get-CimInstance Win32_Process -Filter "Name='msedge.exe'" |
        Where-Object {
            $commandLineMatches = $false
            foreach ($profileArgument in $profileArguments) {
                if ($_.CommandLine -like "*$profileArgument*") {
                    $commandLineMatches = $true
                    break
                }
            }
            $_.SessionId -eq $currentSession -and
            -not [string]::IsNullOrWhiteSpace($_.ExecutablePath) -and
            [string]::Equals(
                [System.IO.Path]::GetFullPath($_.ExecutablePath),
                $browserPath,
                [System.StringComparison]::OrdinalIgnoreCase
            ) -and
            $commandLineMatches
        }
)
if ($browserProcesses.Count -lt 1) {
    throw "No se encontro el navegador de QA esperado."
}

$processIds = @($browserProcesses | ForEach-Object { [int]$_.ProcessId })
$processCondition = [System.Windows.Automation.Condition]::FalseCondition
foreach ($processId in $processIds) {
    $condition = [System.Windows.Automation.PropertyCondition]::new(
        [System.Windows.Automation.AutomationElement]::ProcessIdProperty,
        $processId
    )
    if (
        $processCondition -eq
        [System.Windows.Automation.Condition]::FalseCondition
    ) {
        $processCondition = $condition
    } else {
        $processCondition = [System.Windows.Automation.OrCondition]::new(
            $processCondition,
            $condition
        )
    }
}

$root = [System.Windows.Automation.AutomationElement]::RootElement
$windows = $root.FindAll(
    [System.Windows.Automation.TreeScope]::Children,
    $processCondition
)
$candidates = [System.Collections.Generic.List[object]]::new()
for ($windowIndex = 0; $windowIndex -lt $windows.Count; $windowIndex++) {
    $window = $windows.Item($windowIndex)
    $buttons = $window.FindAll(
        [System.Windows.Automation.TreeScope]::Descendants,
        [System.Windows.Automation.AndCondition]::new(
            [System.Windows.Automation.PropertyCondition]::new(
                [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
                [System.Windows.Automation.ControlType]::Button
            ),
            [System.Windows.Automation.PropertyCondition]::new(
                [System.Windows.Automation.AutomationElement]::ClassNameProperty,
                "MdTextButton"
            )
        )
    )
    for ($buttonIndex = 0; $buttonIndex -lt $buttons.Count; $buttonIndex++) {
        $button = $buttons.Item($buttonIndex)
        try {
            $rectangle = $button.Current.BoundingRectangle
            if (
                $button.Current.IsEnabled -and
                -not $button.Current.IsOffscreen -and
                $rectangle.Width -gt 0 -and
                $rectangle.Height -gt 0
            ) {
                $candidates.Add([pscustomobject]@{
                    Element = $button
                    ProcessId = $button.Current.ProcessId
                    Left = $rectangle.Left
                    Width = $rectangle.Width
                    Height = $rectangle.Height
                })
            }
        } catch {
            continue
        }
    }
}

if ($candidates.Count -ne 2) {
    throw "Se esperaban exactamente dos botones del aviso de protocolo."
}

$sortedCandidates = @($candidates | Sort-Object Left)
$approval = if ($ButtonPosition -eq "leftmost") {
    $sortedCandidates[0]
} else {
    $sortedCandidates[-1]
}
$pattern = $approval.Element.GetCurrentPattern(
    [System.Windows.Automation.InvokePattern]::Pattern
)
if ($null -eq $pattern) {
    throw "El boton de aprobacion no ofrece InvokePattern."
}
$pattern.Invoke()

$result = [ordered]@{
    schemaVersion = 1
    tool = "GrxFirma isolated browser protocol approval"
    capturedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    sessionId = $currentSession
    browserProcessCount = $browserProcesses.Count
    candidateButtonCount = $candidates.Count
    invokedProcessId = $approval.ProcessId
    invokedPosition = $ButtonPosition
    privacy = "No se registran nombres accesibles, URL ni rutas de documentos."
}
Write-JsonAtomically -Path $resultFullPath -Value $result
