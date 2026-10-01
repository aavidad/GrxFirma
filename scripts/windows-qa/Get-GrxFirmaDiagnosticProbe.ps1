# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [switch]$TestDataOnly,
    [Parameter(Mandatory)]
    [string]$ResultPath,
    [Parameter(Mandatory)]
    [string]$TargetProcessPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows -or -not $TestDataOnly) {
    throw "La sonda de diagnóstico requiere Windows y -TestDataOnly."
}

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

function Protect-QADirectory {
    param([Parameter(Mandatory)][string]$Path)

    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    $icacls = Join-Path $env:WINDIR "System32\icacls.exe"
    & $icacls `
        $Path `
        "/inheritance:r" `
        "/grant:r" `
        "*$($currentUser.Value):(OI)(CI)F" `
        "*S-1-5-18:(OI)(CI)F" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo proteger el directorio de la sonda."
    }
}

function Clean-ProbeText {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return ""
    }
    $clean = $Value.Replace("`0", "").Trim()
    $clean = [regex]::Replace(
        $clean,
        "(?i)\b[A-Z]:\\[^\r\n]*",
        "[ruta local omitida]")
    $clean = [regex]::Replace(
        $clean,
        "\\\\[^\s\r\n]+",
        "[ruta UNC omitida]")
    if ($clean.Length -gt 1024) {
        return $clean.Substring(0, 1024)
    }
    return $clean
}

$target = [System.IO.Path]::GetFullPath($TargetProcessPath)
$expectedRoot = [System.IO.Path]::GetFullPath(
    (Join-Path $env:LOCALAPPDATA "GrxFirma")
)
if (
    -not $target.StartsWith(
        $expectedRoot + [System.IO.Path]::DirectorySeparatorChar,
        [System.StringComparison]::OrdinalIgnoreCase
    ) -or
    -not (Test-Path -LiteralPath $target -PathType Leaf)
) {
    throw "La sonda solo admite un proceso instalado de GrxFirma."
}

$result = [System.IO.Path]::GetFullPath($ResultPath)
$qaRoot = [System.IO.Path]::GetFullPath(
    (Join-Path $env:LOCALAPPDATA "GrxFirma\QA\uia-diagnostic-probe")
)
if (
    -not $result.StartsWith(
        $qaRoot + [System.IO.Path]::DirectorySeparatorChar,
        [System.StringComparison]::OrdinalIgnoreCase
    )
) {
    throw "ResultPath debe quedar dentro del directorio fijo de QA."
}
New-Item -ItemType Directory -Path $qaRoot -Force | Out-Null
Protect-QADirectory -Path $qaRoot

$process = Get-Process |
    Where-Object {
        try {
            [System.IO.Path]::GetFullPath($_.Path).Equals(
                $target,
                [System.StringComparison]::OrdinalIgnoreCase)
        } catch {
            $false
        }
    } |
    Select-Object -First 1
if ($null -eq $process) {
    throw "No se encontró el proceso GrxFirma esperado."
}

$root = [System.Windows.Automation.AutomationElement]::RootElement
$processCondition =
    [System.Windows.Automation.PropertyCondition]::new(
        [System.Windows.Automation.AutomationElement]::ProcessIdProperty,
        $process.Id
    )
$window = $root.FindFirst(
    [System.Windows.Automation.TreeScope]::Children,
    $processCondition
)
if ($null -eq $window) {
    throw "No se encontró la ventana GrxFirma esperada."
}

$expanderCondition =
    [System.Windows.Automation.PropertyCondition]::new(
        [System.Windows.Automation.AutomationElement]::NameProperty,
        "Detalles técnicos del diagnóstico"
    )
$expander = $window.FindFirst(
    [System.Windows.Automation.TreeScope]::Descendants,
    $expanderCondition
)
if ($null -ne $expander) {
    $expandPattern = $null
    if (
        $expander.TryGetCurrentPattern(
            [System.Windows.Automation.ExpandCollapsePattern]::Pattern,
            [ref]$expandPattern
        ) -and
        $expandPattern.Current.ExpandCollapseState -ne
            [System.Windows.Automation.ExpandCollapseState]::Expanded
    ) {
        $expandPattern.Expand()
        Start-Sleep -Milliseconds 500
    }
}

$records = [System.Collections.Generic.List[object]]::new()
$elements = $window.FindAll(
    [System.Windows.Automation.TreeScope]::Descendants,
    [System.Windows.Automation.Condition]::TrueCondition
)
$limit = [math]::Min($elements.Count, 300)
for ($index = 0; $index -lt $limit; $index++) {
    try {
        $element = $elements.Item($index)
        $name = Clean-ProbeText -Value $element.Current.Name
        $automationId = $element.Current.AutomationId
        $controlType = $element.Current.ControlType.ProgrammaticName
        $value = ""
        $valuePattern = $null
        if (
            $element.TryGetCurrentPattern(
                [System.Windows.Automation.ValuePattern]::Pattern,
                [ref]$valuePattern
            )
        ) {
            $value = Clean-ProbeText -Value $valuePattern.Current.Value
        }
        if (
            -not [string]::IsNullOrWhiteSpace($name) -or
            -not [string]::IsNullOrWhiteSpace($value)
        ) {
            $records.Add([ordered]@{
                automationId = $automationId
                controlType = $controlType
                name = $name
                value = $value
            })
        }
    } catch {
        continue
    }
}

$document = [ordered]@{
    schemaVersion = 1
    tool = "GrxFirma diagnostic UIA probe"
    capturedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    processSha256 = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash
    privacy = "Las rutas locales y UNC se omiten; solo se inspecciona GrxFirma."
    elements = $records
}
$temporary = "$result.$([guid]::NewGuid().ToString('N')).tmp"
[System.IO.File]::WriteAllText(
    $temporary,
    ($document | ConvertTo-Json -Depth 5),
    [System.Text.UTF8Encoding]::new($false)
)
[System.IO.File]::Move($temporary, $result, $true)
