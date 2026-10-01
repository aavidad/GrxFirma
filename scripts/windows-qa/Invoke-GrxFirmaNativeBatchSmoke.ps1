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
    [string]$PreSignEndpoint,
    [Parameter(Mandatory)]
    [string]$PostSignEndpoint,
    [Parameter(Mandatory)]
    [string]$StorageEndpoint,
    [Parameter(Mandatory)]
    [string]$CertificateSubjectContains,
    [Parameter(Mandatory)]
    [string]$ResultPath,
    [ValidateRange(15, 300)]
    [int]$TimeoutSeconds = 120
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows -or -not $TestDataOnly) {
    throw "La prueba batch nativa requiere Windows y -TestDataOnly."
}
if ([System.Environment]::OSVersion.Version.Major -lt 10) {
    throw "La prueba requiere Windows 10 o posterior."
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

function Assert-LoopbackEndpoint {
    param(
        [Parameter(Mandatory)][string]$Value,
        [Parameter(Mandatory)][string]$Purpose
    )

    $uri = [uri]$Value
    if (
        $uri.Scheme -notin @("http", "https") -or
        $uri.Host -notin @("127.0.0.1", "::1", "localhost") -or
        [string]::IsNullOrWhiteSpace($uri.AbsolutePath)
    ) {
        throw "$Purpose debe ser un endpoint HTTP(S) de loopback."
    }
    return $uri.AbsoluteUri
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

function Get-SafeFailureText {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return ""
    }
    $clean = $Value.Replace("`0", "").Trim()
    $clean = [regex]::Replace($clean, "(?i)https?://\S+", "[endpoint omitido]")
    $clean = [regex]::Replace($clean, "(?i)\b[A-Z]:\\[^\r\n]*", "[ruta local omitida]")
    if ($clean.Length -gt 1024) {
        return $clean.Substring(0, 1024)
    }
    return $clean
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

    $selectionPattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.SelectionItemPattern]::Pattern,
            [ref]$selectionPattern
        )
    ) {
        $selectionPattern.Select()
        return
    }
    Invoke-UiaElement -Element $Element
}

$handlerFullPath = Resolve-QAPath -Path $HandlerPath -Purpose "HandlerPath"
if (-not (Test-Path -LiteralPath $handlerFullPath -PathType Leaf)) {
    throw "No existe el handler de QA."
}
if (
    -not [System.IO.Path]::GetFileName($handlerFullPath).Equals(
        "grxfirma-afirmauri.exe",
        [System.StringComparison]::OrdinalIgnoreCase
    )
) {
    throw "HandlerPath no identifica el handler afirma://."
}
$resultFullPath = Resolve-QAPath -Path $ResultPath -Purpose "ResultPath"
$preSign = Assert-LoopbackEndpoint `
    -Value $PreSignEndpoint `
    -Purpose "PreSignEndpoint"
$postSign = Assert-LoopbackEndpoint `
    -Value $PostSignEndpoint `
    -Purpose "PostSignEndpoint"
$storage = Assert-LoopbackEndpoint `
    -Value $StorageEndpoint `
    -Purpose "StorageEndpoint"
$certificateMarker = $CertificateSubjectContains.Trim()
if ($certificateMarker.Length -lt 8 -or $certificateMarker.Length -gt 128) {
    throw "CertificateSubjectContains no tiene una longitud segura."
}

$requestId = "qa-batch-$([guid]::NewGuid().ToString('N'))"
$batch = [ordered]@{
    format = "CAdES"
    algorithm = "SHA256withRSA"
    singlesigns = @(
        [ordered]@{
            id = "documento-qa"
            datareference = "referencia-sintetica"
        }
    )
    stoponerror = $true
}
$batchBytes = [System.Text.Encoding]::UTF8.GetBytes(
    ($batch | ConvertTo-Json -Depth 5 -Compress)
)
$batchBase64 = [Convert]::ToBase64String($batchBytes)
[Array]::Clear($batchBytes, 0, $batchBytes.Length)

$rawUri = "afirma://batch?" + (@(
    "jvc=2"
    "ver=3"
    "op=batch"
    "id=$([uri]::EscapeDataString($requestId))"
    "stservlet=$([uri]::EscapeDataString($storage))"
    "batchpresignerurl=$([uri]::EscapeDataString($preSign))"
    "batchpostsignerurl=$([uri]::EscapeDataString($postSign))"
    "needcert=true"
    "jsonbatch=true"
    "dat=$([uri]::EscapeDataString($batchBase64))"
) -join "&")
$batchBase64 = $null

$startedAt = Get-Date
$handler = Start-Process `
    -FilePath $handlerFullPath `
    -ArgumentList @($rawUri) `
    -WorkingDirectory (Split-Path -Parent $handlerFullPath) `
    -PassThru
$rawUri = $null

$approved = $false
$certificateSelected = $false
$failureInstruction = ""
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
    while ((Get-Date) -lt $deadline -and -not $handler.HasExited) {
        $windows = $root.FindAll(
            [System.Windows.Automation.TreeScope]::Descendants,
            $processWindowCondition
        )
        for ($windowIndex = 0; $windowIndex -lt $windows.Count; $windowIndex++) {
            $window = $windows.Item($windowIndex)
            $windowName = $window.Current.Name.Trim()

            if (
                $approved -and
                -not $certificateSelected -and
                $windowName -eq "GrxFirma — Seleccionar certificado"
            ) {
                $radios = $window.FindAll(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
                        [System.Windows.Automation.ControlType]::RadioButton
                    )
                )
                $matches = [System.Collections.Generic.List[object]]::new()
                for ($radioIndex = 0; $radioIndex -lt $radios.Count; $radioIndex++) {
                    $radio = $radios.Item($radioIndex)
                    if (
                        $radio.Current.Name.IndexOf(
                            $certificateMarker,
                            [System.StringComparison]::OrdinalIgnoreCase
                        ) -ge 0
                    ) {
                        $matches.Add($radio)
                    }
                }
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
            $acceptButton = $window.FindFirst(
                [System.Windows.Automation.TreeScope]::Descendants,
                [System.Windows.Automation.PropertyCondition]::new(
                    [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                    "CommandButton_1101"
                )
            )

            if (
                -not $approved -and
                $instructionText -eq "Confirma la operación de firma" -and
                $null -ne $acceptButton
            ) {
                Invoke-UiaElement -Element $acceptButton
                $approved = $true
                Start-Sleep -Milliseconds 250
                continue
            }

            if (
                $approved -and
                -not $certificateSelected -and
                $instructionText -eq "Selecciona un certificado" -and
                $null -ne $acceptButton
            ) {
                $radios = $window.FindAll(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
                        [System.Windows.Automation.ControlType]::RadioButton
                    )
                )
                $matches = [System.Collections.Generic.List[object]]::new()
                for ($radioIndex = 0; $radioIndex -lt $radios.Count; $radioIndex++) {
                    $radio = $radios.Item($radioIndex)
                    if (
                        $radio.Current.Name.IndexOf(
                            $certificateMarker,
                            [System.StringComparison]::OrdinalIgnoreCase
                        ) -ge 0
                    ) {
                        $matches.Add($radio)
                    }
                }
                if ($matches.Count -ne 1) {
                    throw "El selector no contiene exactamente un certificado de QA coincidente."
                }
                Select-UiaRadio -Element $matches[0]
                Invoke-UiaElement -Element $acceptButton
                $certificateSelected = $true
                Start-Sleep -Milliseconds 250
                continue
            }

            if ($instructionText -eq "Operación no completada") {
                $failureInstruction = Get-SafeFailureText -Value $instructionText
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
                $cancelButton = $window.FindFirst(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                        "CommandButton_1103"
                    )
                )
                if ($null -ne $cancelButton) {
                    Invoke-UiaElement -Element $cancelButton
                }
            }
        }
        Start-Sleep -Milliseconds 150
        $handler.Refresh()
    }

    if (-not $handler.HasExited) {
        throw "La prueba batch nativa agotó el tiempo de espera."
    }
    if (-not $approved) {
        throw "No se aprobó la operación batch de QA."
    }
    if (-not $certificateSelected) {
        throw "No se seleccionó el certificado de QA."
    }
    if ($handler.ExitCode -ne 0) {
        throw "El handler finalizó con código $($handler.ExitCode): $failureDetail"
    }
    if (-not [string]::IsNullOrWhiteSpace($failureInstruction)) {
        throw "El handler mostró un fallo inesperado: $failureDetail"
    }

    $document = [ordered]@{
        schemaVersion = 1
        tool = "GrxFirma native Windows batch smoke"
        capturedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
        handlerSha256 = (
            Get-FileHash -LiteralPath $handlerFullPath -Algorithm SHA256
        ).Hash
        algorithm = "SHA256withRSA"
        format = "CAdES"
        operationCount = 1
        approvalCompleted = $approved
        exactQACertificateSelected = $certificateSelected
        exitCode = $handler.ExitCode
        elapsedMilliseconds = [math]::Round(
            ((Get-Date) - $startedAt).TotalMilliseconds
        )
        privacy = "No se registran URI, identificadores de sesión, certificados ajenos ni rutas de documentos."
    }
    Write-JsonAtomically -Path $resultFullPath -Value $document
} finally {
    $rawUri = $null
    $batchBase64 = $null
    if ($null -ne $handler) {
        $handler.Refresh()
        if (-not $handler.HasExited) {
            Stop-Process -Id $handler.Id -Force -ErrorAction SilentlyContinue
        }
        $handler.Dispose()
    }
}
