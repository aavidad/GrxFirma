# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [string[]]$Roots = @("packaging", "scripts")
)

$ErrorActionPreference = "Stop"
$parseFailures = @()
$checked = 0

foreach ($root in $Roots) {
    if (-not (Test-Path -LiteralPath $root)) {
        throw "PowerShell syntax root does not exist: $root"
    }

    Get-ChildItem -LiteralPath $root -Recurse -File -Filter "*.ps1" | ForEach-Object {
        $tokens = $null
        $errors = $null
        [void][System.Management.Automation.Language.Parser]::ParseFile(
            $_.FullName,
            [ref]$tokens,
            [ref]$errors
        )
        $checked++
        foreach ($parseError in $errors) {
            $parseFailures += ("{0}:{1}:{2}: {3}" -f `
                $_.FullName,
                $parseError.Extent.StartLineNumber,
                $parseError.Extent.StartColumnNumber,
                $parseError.Message)
        }
    }
}

if ($parseFailures.Count -gt 0) {
    $parseFailures | ForEach-Object { Write-Error $_ }
    throw "PowerShell syntax validation failed with $($parseFailures.Count) error(s)."
}

Write-Host "PowerShell syntax validation passed: $checked file(s)."
