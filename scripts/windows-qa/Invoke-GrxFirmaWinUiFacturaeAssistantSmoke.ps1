# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [string]$RepositoryRoot = (Join-Path $PSScriptRoot "..\.."),
    [Parameter(Mandatory)]
    [switch]$TestDataOnly,
    [Parameter(Mandatory)]
    [string]$ResultPath,
    [ValidateSet("EnableAndOpen", "Disable")]
    [string]$Mode = "EnableAndOpen",
    [ValidateRange(0, 100)]
    [int]$AssistantScrollPercent = 0,
    [ValidateRange(30, 180)]
    [int]$TimeoutSeconds = 90
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows -or -not $TestDataOnly) {
    throw "Esta prueba requiere Windows y -TestDataOnly."
}

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

function Write-JsonAtomically {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)]$Value
    )

    $json = $Value | ConvertTo-Json -Depth 6
    $temporary = "$Path.$([guid]::NewGuid().ToString('N')).tmp"
    [System.IO.File]::WriteAllText(
        $temporary,
        $json,
        [System.Text.UTF8Encoding]::new($false)
    )
    [System.IO.File]::Move($temporary, $Path, $true)
}

function Get-ProcessPath {
    param([Parameter(Mandatory)][int]$ProcessId)

    try {
        return (Get-Process -Id $ProcessId -ErrorAction Stop).Path
    } catch {
        return $null
    }
}

function Wait-ApplicationWindow {
    param(
        [Parameter(Mandatory)][string]$ExpectedProcessPath,
        [Parameter(Mandatory)][datetime]$Deadline
    )

    $root = [System.Windows.Automation.AutomationElement]::RootElement
    while ([datetime]::UtcNow -lt $Deadline) {
        $windows = $root.FindAll(
            [System.Windows.Automation.TreeScope]::Children,
            [System.Windows.Automation.Condition]::TrueCondition
        )
        foreach ($window in $windows) {
            try {
                $processPath = Get-ProcessPath `
                    -ProcessId $window.Current.ProcessId
                if (
                    -not [string]::IsNullOrWhiteSpace($processPath) -and
                    [System.IO.Path]::GetFullPath($processPath).Equals(
                        $ExpectedProcessPath,
                        [System.StringComparison]::OrdinalIgnoreCase
                    )
                ) {
                    return $window
                }
            } catch {
                continue
            }
        }
        Start-Sleep -Milliseconds 200
    }
    throw "No aparecio la ventana WinUI del candidato."
}

function Find-DescendantByName {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory)][string]$Name
    )

    return $Root.FindFirst(
        [System.Windows.Automation.TreeScope]::Descendants,
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::NameProperty,
            $Name
        )
    )
}

function Find-DescendantByAutomationId {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory)][string]$AutomationId
    )

    return $Root.FindFirst(
        [System.Windows.Automation.TreeScope]::Descendants,
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
            $AutomationId
        )
    )
}

function Wait-DescendantByName {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][datetime]$Deadline,
        [switch]$RequireEnabled
    )

    while ([datetime]::UtcNow -lt $Deadline) {
        try {
            $element = Find-DescendantByName -Root $Root -Name $Name
            if (
                $null -ne $element -and
                (-not $RequireEnabled -or $element.Current.IsEnabled)
            ) {
                return $element
            }
        } catch {
        }
        Start-Sleep -Milliseconds 200
    }
    throw "No aparecio o no se habilito el control: $Name"
}

function Wait-DescendantByAutomationId {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory)][string]$AutomationId,
        [Parameter(Mandatory)][datetime]$Deadline,
        [switch]$RequireEnabled
    )

    while ([datetime]::UtcNow -lt $Deadline) {
        try {
            $element = Find-DescendantByAutomationId `
                -Root $Root `
                -AutomationId $AutomationId
            if (
                $null -ne $element -and
                (-not $RequireEnabled -or $element.Current.IsEnabled)
            ) {
                return $element
            }
        } catch {
        }
        Start-Sleep -Milliseconds 200
    }
    throw "No aparecio o no se habilito el control QA: $AutomationId"
}

function Invoke-UiaElement {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $scroll = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scroll
        )
    ) {
        $scroll.ScrollIntoView()
    }
    $invoke = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.InvokePattern]::Pattern,
            [ref]$invoke
        )
    ) {
        $invoke.Invoke()
        return
    }

    $toggle = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.TogglePattern]::Pattern,
            [ref]$toggle
        )
    ) {
        $toggle.Toggle()
        return
    }

    $expand = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ExpandCollapsePattern]::Pattern,
            [ref]$expand
        )
    ) {
        if (
            $expand.Current.ExpandCollapseState -eq
            [System.Windows.Automation.ExpandCollapseState]::Collapsed
        ) {
            $expand.Expand()
        } else {
            $expand.Collapse()
        }
        return
    }

    $selection = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.SelectionItemPattern]::Pattern,
            [ref]$selection
        )
    ) {
        $selection.Select()
        return
    }

    $legacy = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.LegacyIAccessiblePattern]::Pattern,
            [ref]$legacy
        )
    ) {
        $legacy.DoDefaultAction()
        return
    }

    throw "El control esperado no ofrece un patron accesible de activacion."
}

function Set-CheckboxState {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Checkbox,
        [Parameter(Mandatory)][bool]$Enabled
    )

    $toggle = $null
    if (
        -not $Checkbox.TryGetCurrentPattern(
            [System.Windows.Automation.TogglePattern]::Pattern,
            [ref]$toggle
        )
    ) {
        throw "La preferencia Facturae no ofrece TogglePattern."
    }
    $wasEnabled = $toggle.Current.ToggleState -eq
        [System.Windows.Automation.ToggleState]::On
    if ($wasEnabled -ne $Enabled) {
        $toggle.Toggle()
        return [ordered]@{
            wasEnabled = $wasEnabled
            changed = $true
        }
    }
    return [ordered]@{
        wasEnabled = $wasEnabled
        changed = $false
    }
}

$resolvedRoot = (
    Resolve-Path -LiteralPath $RepositoryRoot -ErrorAction Stop
).Path
$stageRoot = Join-Path `
    $resolvedRoot `
    "release\windows-desktop-winui\GrxFirma-$((Get-Content -LiteralPath (Join-Path $resolvedRoot "VERSION.txt") -Raw).Trim())-desktop-winui-windows-amd64"
$winUiExecutable = Join-Path $stageRoot "app\grxfirma-winui.exe"
if (-not (Test-Path -LiteralPath $winUiExecutable -PathType Leaf)) {
    throw "El stage WinUI no esta completo."
}

$resultFullPath = [System.IO.Path]::GetFullPath($ResultPath)
$qaRoot = [System.IO.Path]::GetFullPath(
    (Join-Path $env:LOCALAPPDATA "GrxFirma\QA\facturae-assistant")
)
if (
    -not $resultFullPath.StartsWith(
        $qaRoot + [System.IO.Path]::DirectorySeparatorChar,
        [System.StringComparison]::OrdinalIgnoreCase
    )
) {
    throw "ResultPath debe quedar dentro del directorio fijo de QA."
}
if (-not (Test-Path -LiteralPath $qaRoot -PathType Container)) {
    New-Item -ItemType Directory -Path $qaRoot -Force | Out-Null
}

$runId = [System.IO.Path]::GetFileNameWithoutExtension($resultFullPath)
if ($runId -notmatch "^\d{8}T\d{9}Z-[a-f0-9]{8}$") {
    throw "El nombre de ResultPath no corresponde a una ejecucion QA."
}

$result = [ordered]@{
    schemaVersion = 1
    tool = "GrxFirma WinUI Facturae assistant smoke"
    runId = $runId
    mode = $Mode
    status = "running"
    phase = "starting"
    startedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    completedAtUtc = $null
    preferenceWasEnabled = $null
    preferenceChanged = $false
    preferenceSaved = $false
    assistantRendered = $false
    assistantScrollPercent = $AssistantScrollPercent
    failure = $null
}
Write-JsonAtomically -Path $resultFullPath -Value $result

try {
    $deadline = [datetime]::UtcNow.AddSeconds($TimeoutSeconds)
    $window = Wait-ApplicationWindow `
        -ExpectedProcessPath ([System.IO.Path]::GetFullPath($winUiExecutable)) `
        -Deadline $deadline
    $result.phase = "window-found"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    $togglePane = Find-DescendantByAutomationId `
        -Root $window `
        -AutomationId "TogglePaneButton"
    if ($null -ne $togglePane) {
        Invoke-UiaElement -Element $togglePane
    }
    $result.phase = "navigation-opened"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    $settings = Wait-DescendantByAutomationId `
        -Root $window `
        -AutomationId "SettingsNavigationItem" `
        -Deadline $deadline `
        -RequireEnabled
    Invoke-UiaElement -Element $settings
    $result.phase = "settings-invoked"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    $checkbox = Wait-DescendantByAutomationId `
        -Root $window `
        -AutomationId "FacturaeToolsEnabledCheckBox" `
        -Deadline $deadline
    $result.phase = "settings-rendered"
    Write-JsonAtomically -Path $resultFullPath -Value $result
    $scrollItemPattern = $null
    if (
        $checkbox.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollItemPattern
        )
    ) {
        $scrollItemPattern.ScrollIntoView()
    }

    $checkbox = Wait-DescendantByAutomationId `
        -Root $window `
        -AutomationId "FacturaeToolsEnabledCheckBox" `
        -Deadline $deadline `
        -RequireEnabled
    $result.phase = "preference-ready"
    Write-JsonAtomically -Path $resultFullPath -Value $result
    $desiredEnabled = $Mode -eq "EnableAndOpen"
    $change = Set-CheckboxState `
        -Checkbox $checkbox `
        -Enabled $desiredEnabled
    $result.preferenceWasEnabled = $change.wasEnabled
    $result.preferenceChanged = $change.changed

    if ($change.changed) {
        $save = Wait-DescendantByAutomationId `
            -Root $window `
            -AutomationId "SaveButton" `
            -Deadline $deadline `
            -RequireEnabled
        Invoke-UiaElement -Element $save
        [void](Wait-DescendantByName `
            -Root $window `
            -Name "Preferencias guardadas" `
            -Deadline $deadline)
    }
    $result.preferenceSaved = $true
    $result.phase = "preference-saved"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    if ($desiredEnabled) {
        $togglePane = Find-DescendantByAutomationId `
            -Root $window `
            -AutomationId "TogglePaneButton"
        if ($null -ne $togglePane) {
            Invoke-UiaElement -Element $togglePane
        }
        $assistant = Wait-DescendantByAutomationId `
            -Root $window `
            -AutomationId "FacturaeNavigationItem" `
            -Deadline $deadline `
            -RequireEnabled
        Invoke-UiaElement -Element $assistant
        [void](Wait-DescendantByName `
            -Root $window `
            -Name "Guía de Facturae y FACe" `
            -Deadline $deadline)
        if ($AssistantScrollPercent -gt 0) {
            $assistantContent = Find-DescendantByName `
                -Root $window `
                -Name "Guía de Facturae y FACe"
            $assistantScroll = $null
            if (
                $null -eq $assistantContent -or
                -not $assistantContent.TryGetCurrentPattern(
                    [System.Windows.Automation.ScrollPattern]::Pattern,
                    [ref]$assistantScroll
                ) -or
                -not $assistantScroll.Current.VerticallyScrollable
            ) {
                throw "La guía Facturae no expuso desplazamiento accesible."
            }
            $assistantScroll.SetScrollPercent(
                [System.Windows.Automation.ScrollPattern]::NoScroll,
                $AssistantScrollPercent
            )
            Start-Sleep -Milliseconds 500

            # En WinUI 3 el porcentaje del ScrollPattern puede redondearse
            # mientras terminan de medirse los Expander. Para las capturas del
            # extremo inferior, anclamos además un control real mediante
            # ScrollItemPattern y evitamos declarar éxito sobre el encabezado.
            if ($AssistantScrollPercent -ge 90) {
                $bottomAnchor = Wait-DescendantByName `
                    -Root $window `
                    -Name "Consultar estado de factura en FACe" `
                    -Deadline $deadline `
                    -RequireEnabled
                $bottomScrollItem = $null
                if (
                    -not $bottomAnchor.TryGetCurrentPattern(
                        [System.Windows.Automation.ScrollItemPattern]::Pattern,
                        [ref]$bottomScrollItem
                    )
                ) {
                    throw "El final de la guía Facturae no expuso desplazamiento accesible."
                }
                $bottomScrollItem.ScrollIntoView()
                Start-Sleep -Milliseconds 750
            }
        }
        $result.assistantRendered = $true
    } else {
        Start-Sleep -Milliseconds 500
        if ($null -ne (
            Find-DescendantByAutomationId `
                -Root $window `
                -AutomationId "FacturaeNavigationItem"
        )) {
            throw "La seccion Facturae siguio expuesta tras desactivarla."
        }
    }

    $result.status = "succeeded"
    $result.phase = "completed"
} catch {
    $result.status = "failed"
    $result.failure = $_.Exception.Message
    throw
} finally {
    $result.completedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    Write-JsonAtomically -Path $resultFullPath -Value $result
}
