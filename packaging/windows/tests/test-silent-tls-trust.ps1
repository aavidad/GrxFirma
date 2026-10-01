# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

$ErrorActionPreference = "Stop"
$installer = Join-Path (Split-Path -Parent $PSScriptRoot) "install-afirmauri.ps1"
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    $installer, [ref]$tokens, [ref]$parseErrors
)
if ($parseErrors.Count -gt 0) {
    throw "El instalador AfirmaURI no se puede analizar"
}
$definition = $ast.FindAll({
    param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq "Invoke-GrxFirmaLocalTlsTrust"
}, $true) | Select-Object -First 1
if ($null -eq $definition) {
    throw "Falta la función de confianza TLS local"
}
. ([scriptblock]::Create($definition.Extent.Text))

$script:ProcessStarts = 0
$script:WaitMilliseconds = 0
$script:KillCalls = 0
$script:DisposeCalls = 0
$script:WaitResult = $true
$script:ExitCode = 0
function Start-Process {
    param($FilePath, $ArgumentList, $WindowStyle, [switch]$PassThru)
    $script:ProcessStarts++
    if ($ArgumentList -ne "--install-local-tls-trust" -or
        $WindowStyle -ne "Hidden" -or -not $PassThru) {
        throw "Invocación de confianza TLS inesperada"
    }
    $process = [pscustomobject]@{ ExitCode = $script:ExitCode }
    $process | Add-Member ScriptMethod WaitForExit {
        param($milliseconds)
        $script:WaitMilliseconds = $milliseconds
        return $script:WaitResult
    }
    $process | Add-Member ScriptMethod Kill { $script:KillCalls++ }
    $process | Add-Member ScriptMethod Dispose { $script:DisposeCalls++ }
    return $process
}

Invoke-GrxFirmaLocalTlsTrust -InstallDir "/tmp/GrxFirma" -SilentInstall
if ($script:ProcessStarts -ne 0) {
    throw "El modo /S ejecutó la instalación de la CA"
}

Invoke-GrxFirmaLocalTlsTrust -InstallDir "/tmp/GrxFirma"
if ($script:ProcessStarts -ne 1 -or $script:WaitMilliseconds -ne 300000 -or
    $script:KillCalls -ne 0 -or $script:DisposeCalls -ne 1) {
    throw "La instalación interactiva no esperó como máximo 5 minutos"
}

$script:WaitResult = $false
$warnings = @(Invoke-GrxFirmaLocalTlsTrust -InstallDir "/tmp/GrxFirma" 3>&1)
if ($script:ProcessStarts -ne 2 -or $script:KillCalls -ne 1 -or
    $script:DisposeCalls -ne 2 -or
    -not (($warnings | Out-String) -match "5 minutos")) {
    throw "El timeout no libera el proceso y avisa al usuario"
}

$script:WaitResult = $true
$script:ExitCode = 1
$warnings = @(Invoke-GrxFirmaLocalTlsTrust -InstallDir "/tmp/GrxFirma" 3>&1)
if ($script:ProcessStarts -ne 3 -or $script:DisposeCalls -ne 3 -or
    -not (($warnings | Out-String) -match "primer[a]? vez")) {
    throw "Un rechazo de la CA no informa del reintento en el primer uso"
}

Write-Host "PASS: instalación silenciosa y espera TLS acotada"
