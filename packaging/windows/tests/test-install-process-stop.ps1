# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"
$helper = Join-Path (Split-Path -Parent $PSScriptRoot) "install-path-safety.ps1"
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    $helper, [ref]$tokens, [ref]$parseErrors
)
if ($parseErrors.Count -gt 0) {
    throw "No se puede analizar el helper de procesos."
}
foreach ($name in "Get-GrxFirmaProcessPath", "Stop-GrxFirmaInstalledProcesses") {
    $definition = $ast.FindAll({
        param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
            $node.Name -eq $name
    }, $true) | Select-Object -First 1
    if ($null -eq $definition) {
        throw "Falta $name."
    }
    . ([scriptblock]::Create($definition.Extent.Text))
}

$root = [System.IO.Path]::GetFullPath((Join-Path ([System.IO.Path]::GetTempPath()) "GrxFirma/AfirmaURI"))
function Resolve-GrxFirmaInstallPath {
    param($Path, $Component)
    if ($Component -ne "AfirmaURI" -or $Path -ne $root) {
        throw "Ruta no verificada."
    }
    return $root
}
function New-TestProcess {
    param([string]$Path, [int]$Id = 0)
    $process = [pscustomobject]@{ Path = $Path; Id = $Id; KillCalls = 0; WaitMs = 0 }
    $process | Add-Member ScriptMethod Kill { $this.KillCalls++ }
    $process | Add-Member ScriptMethod WaitForExit {
        param($milliseconds)
        $this.WaitMs = $milliseconds
        return $true
    }
    return $process
}
$inside = New-TestProcess -Path (Join-Path $root "grxfirma-afirmauri.exe")
$sibling = New-TestProcess -Path (Join-Path ($root + "-other") "grxfirma-afirmauri.exe")
$foreign = New-TestProcess -Path (Join-Path ([System.IO.Path]::GetTempPath()) "grxfirma-afirmauri.exe")
# Desde un PowerShell de 32 bits, Process.Path de un proceso de 64 bits llega
# vacío: la ruta se obtiene por WMI y el proceso propio también se detiene.
$wow64 = New-TestProcess -Path "" -Id 4242
function Get-CimInstance {
    param($ClassName, $Filter, $ErrorAction)
    if ($ClassName -eq "Win32_Process" -and $Filter -eq "ProcessId = 4242") {
        return [pscustomobject]@{ ExecutablePath = (Join-Path $root "grxfirma-afirmauri.exe") }
    }
    return $null
}
function Get-Process {
    param($ErrorAction)
    return @($inside, $sibling, $foreign, $wow64)
}
Stop-GrxFirmaInstalledProcesses -Path $root -Component "AfirmaURI"
if ($inside.KillCalls -ne 1 -or $inside.WaitMs -ne 10000 -or
    $sibling.KillCalls -ne 0 -or $foreign.KillCalls -ne 0 -or
    $wow64.KillCalls -ne 1) {
    throw "Se detuvo un proceso ajeno o no se esperó al proceso propio."
}
Write-Host "PASS: cierre acotado por ruta de ejecutable verificada"
