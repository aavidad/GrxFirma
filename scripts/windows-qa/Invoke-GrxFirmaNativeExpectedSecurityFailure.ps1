# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [switch]$TestDataOnly,
    [Parameter(Mandatory)]
    [string]$HandlerPath,
    [Parameter(Mandatory)]
    [string]$CertificateSubjectContains,
    [Parameter(Mandatory)]
    [ValidateSet("SHA1", "TLS_LEGACY_UNSUPPORTED")]
    [string]$ExpectedFailure,
    [Parameter(Mandatory)]
    [string]$ResultPath,
    [ValidateRange(15, 180)]
    [int]$TimeoutSeconds = 90
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows -or -not $TestDataOnly) {
    throw "La prueba requiere Windows y -TestDataOnly."
}

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

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

function Invoke-UiaElement {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $pattern = $null
    if (
        -not $Element.TryGetCurrentPattern(
            [System.Windows.Automation.InvokePattern]::Pattern,
            [ref]$pattern
        )
    ) {
        throw "El control esperado no ofrece InvokePattern."
    }
    $pattern.Invoke()
}

function Select-UiaRadio {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $pattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.SelectionItemPattern]::Pattern,
            [ref]$pattern
        )
    ) {
        $pattern.Select()
        return
    }
    Invoke-UiaElement -Element $Element
}

function Get-SafeFailureText {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return ""
    }
    $clean = $Value.Replace("`0", "").Trim()
    $clean = [regex]::Replace(
        $clean,
        "(?i)https?://\S+",
        "[endpoint omitido]"
    )
    $clean = [regex]::Replace(
        $clean,
        "(?i)\b[A-Z]:\\[^\r\n]*",
        "[ruta local omitida]"
    )
    if ($clean.Length -gt 2048) {
        return $clean.Substring(0, 2048)
    }
    return $clean
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
    $temporary = "$Path.$([guid]::NewGuid().ToString('N')).tmp"
    [System.IO.File]::WriteAllText(
        $temporary,
        ($Value | ConvertTo-Json -Depth 6),
        [System.Text.UTF8Encoding]::new($false)
    )
    [System.IO.File]::Move($temporary, $Path, $true)
}

$handlerFullPath = Resolve-QAPath -Path $HandlerPath -Purpose "HandlerPath"
if (
    -not (Test-Path -LiteralPath $handlerFullPath -PathType Leaf) -or
    -not [System.IO.Path]::GetFileName($handlerFullPath).Equals(
        "grxfirma-afirmauri.exe",
        [System.StringComparison]::OrdinalIgnoreCase
    )
) {
    throw "HandlerPath no identifica el handler afirma:// de QA."
}
$resultFullPath = Resolve-QAPath -Path $ResultPath -Purpose "ResultPath"
$certificateMarker = $CertificateSubjectContains.Trim()
if ($certificateMarker.Length -lt 8 -or $certificateMarker.Length -gt 128) {
    throw "CertificateSubjectContains no tiene una longitud segura."
}

$handlers = @(
    Get-Process -Name "grxfirma-afirmauri" -ErrorAction SilentlyContinue |
        Where-Object {
            try {
                [System.IO.Path]::GetFullPath($_.Path).Equals(
                    $handlerFullPath,
                    [System.StringComparison]::OrdinalIgnoreCase
                )
            } catch {
                $false
            }
        }
)
if ($handlers.Count -ne 1) {
    throw "Se esperaba exactamente un handler afirma:// de QA en ejecución."
}
$handler = $handlers[0]

$expectedFragments = if ($ExpectedFailure -eq "SHA1") {
    @("sha-1", "obsoleto", "inseguro", "no se ha firmado")
} else {
    @(
        "tls 1.0",
        "tls 1.1",
        "obsoleto",
        "tls 1.2",
        "portal",
        "tls_legacy_unsupported"
    )
}

$approved = $false
$certificateSelected = $false
$failureDetail = ""
$deadline = (Get-Date).AddSeconds($TimeoutSeconds)
$root = [System.Windows.Automation.AutomationElement]::RootElement
$processCondition = [System.Windows.Automation.PropertyCondition]::new(
    [System.Windows.Automation.AutomationElement]::ProcessIdProperty,
    $handler.Id
)
$processWindowCondition = [System.Windows.Automation.AndCondition]::new(
    $processCondition,
    [System.Windows.Automation.PropertyCondition]::new(
        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
        [System.Windows.Automation.ControlType]::Window
    )
)

try {
    while ((Get-Date) -lt $deadline -and [string]::IsNullOrWhiteSpace($failureDetail)) {
        $windows = $root.FindAll(
            [System.Windows.Automation.TreeScope]::Descendants,
            $processWindowCondition
        )
        for ($windowIndex = 0; $windowIndex -lt $windows.Count; $windowIndex++) {
            $window = $windows.Item($windowIndex)
            $windowName = $window.Current.Name.Trim()

            if (
                -not $certificateSelected -and
                $windowName -eq "GrxFirma — Seleccionar certificado"
            ) {
                # La sonda puede adjuntarse cuando la aprobación inicial ya se
                # ha cerrado. La presencia de este selector exacto, propiedad
                # del único handler de QA validado, acredita esa fase.
                $approved = $true
                $radios = $window.FindAll(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
                        [System.Windows.Automation.ControlType]::RadioButton
                    )
                )
                $matches = @(
                    for ($radioIndex = 0; $radioIndex -lt $radios.Count; $radioIndex++) {
                        $radio = $radios.Item($radioIndex)
                        if (
                            $radio.Current.Name.IndexOf(
                                $certificateMarker,
                                [System.StringComparison]::OrdinalIgnoreCase
                            ) -ge 0
                        ) {
                            $radio
                        }
                    }
                )
                if ($matches.Count -ne 1) {
                    throw "El selector no contiene exactamente un certificado de QA coincidente."
                }
                $useCertificate = $window.FindFirst(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::NameProperty,
                        "Usar el certificado seleccionado"
                    )
                )
                if ($null -eq $useCertificate) {
                    throw "El selector no expone el botón seguro de confirmación."
                }
                Select-UiaRadio -Element $matches[0]
                Invoke-UiaElement -Element $useCertificate
                $certificateSelected = $true
                Start-Sleep -Milliseconds 250
                continue
            }

            $instruction = $window.FindFirst(
                [System.Windows.Automation.TreeScope]::Descendants,
                [System.Windows.Automation.PropertyCondition]::new(
                    [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                    "MainInstruction"
                )
            )
            if ($null -eq $instruction) {
                continue
            }
            $instructionText = $instruction.Current.Name.Trim()

            if (-not $approved -and $instructionText -eq "Confirma la operación de firma") {
                $acceptButton = $window.FindFirst(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                        "CommandButton_1101"
                    )
                )
                if ($null -eq $acceptButton) {
                    throw "No se encontró la aprobación segura del handler."
                }
                Invoke-UiaElement -Element $acceptButton
                $approved = $true
                Start-Sleep -Milliseconds 250
                continue
            }

            if ($instructionText -eq "Operación no completada") {
                $content = $window.FindFirst(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                        "ContentText"
                    )
                )
                if ($null -ne $content) {
                    $failureDetail = Get-SafeFailureText -Value $content.Current.Name
                }
                break
            }
        }
        Start-Sleep -Milliseconds 150
    }

    if (-not $approved) {
        throw "No se aprobó la operación web de QA."
    }
    if (-not $certificateSelected) {
        throw "No se seleccionó el certificado de QA."
    }
    if ([string]::IsNullOrWhiteSpace($failureDetail)) {
        throw "No apareció el fallo de seguridad esperado."
    }
    $lower = $failureDetail.ToLowerInvariant()
    foreach ($fragment in $expectedFragments) {
        if (-not $lower.Contains($fragment)) {
            throw "El diagnóstico no contiene el fragmento seguro esperado '$fragment'."
        }
    }

    $document = [ordered]@{
        schemaVersion = 1
        tool = "GrxFirma native expected security failure"
        capturedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
        handlerSha256 = (
            Get-FileHash -LiteralPath $handlerFullPath -Algorithm SHA256
        ).Hash
        expectedFailure = $ExpectedFailure
        approvalCompleted = $approved
        exactQACertificateSelected = $certificateSelected
        diagnosticValidated = $true
        diagnostic = $failureDetail
        privacy = "No se registran URI, identificadores de sesión, certificados ajenos ni rutas de documentos."
    }
    Write-JsonAtomically -Path $resultFullPath -Value $document
} finally {
    if (-not $handler.HasExited) {
        $window = $root.FindFirst(
            [System.Windows.Automation.TreeScope]::Descendants,
            $processWindowCondition
        )
        if ($null -ne $window) {
            $closeButton = $window.FindFirst(
                [System.Windows.Automation.TreeScope]::Descendants,
                [System.Windows.Automation.PropertyCondition]::new(
                    [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                    "CommandButton_1103"
                )
            )
            if ($null -ne $closeButton) {
                try {
                    Invoke-UiaElement -Element $closeButton
                } catch {
                    # El cierre se revalida por identidad de proceso abajo.
                }
            }
        }
        Start-Sleep -Milliseconds 300
        $handler.Refresh()
        if (-not $handler.HasExited) {
            Stop-Process -Id $handler.Id -Force -ErrorAction SilentlyContinue
        }
    }
    $handler.Dispose()
}
