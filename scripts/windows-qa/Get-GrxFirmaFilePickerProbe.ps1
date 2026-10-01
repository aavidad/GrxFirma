# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [switch]$TestDataOnly,
    [Parameter(Mandatory)]
    [string]$ResultPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows -or -not $TestDataOnly) {
    throw "La sonda del selector requiere Windows y -TestDataOnly."
}

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

if (-not ("GrxFirma.QA.FilePickerProbeNative" -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;

namespace GrxFirma.QA
{
    public static class FilePickerProbeNative
    {
        private delegate bool EnumWindowsProc(
            IntPtr window,
            IntPtr parameter);

        [DllImport("user32.dll")]
        private static extern bool EnumWindows(
            EnumWindowsProc callback,
            IntPtr parameter);

        public static IntPtr[] EnumerateTopLevelHandles()
        {
            List<IntPtr> handles = new List<IntPtr>();
            EnumWindows(delegate(IntPtr window, IntPtr parameter)
            {
                handles.Add(window);
                return true;
            }, IntPtr.Zero);
            return handles.ToArray();
        }
    }
}
'@
}

$resultFullPath = [System.IO.Path]::GetFullPath($ResultPath)
$qaRoot = [System.IO.Path]::GetFullPath(
    (Join-Path $env:LOCALAPPDATA "GrxFirma\QA\file-picker-probe")
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

$sessionId = [System.Diagnostics.Process]::GetCurrentProcess().SessionId
$syntheticRoot = [System.IO.Path]::GetFullPath(
    (Join-Path $env:LOCALAPPDATA "GrxFirma\QA")
)
$records = [System.Collections.Generic.List[object]]::new()
$rootWindows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
    [System.Windows.Automation.TreeScope]::Children,
    [System.Windows.Automation.Condition]::TrueCondition
)
$windows = [System.Collections.Generic.List[
    System.Windows.Automation.AutomationElement
]]::new()
$knownHandles = [System.Collections.Generic.HashSet[long]]::new()
foreach ($window in $rootWindows) {
    try {
        [void]$knownHandles.Add($window.Current.NativeWindowHandle)
        $windows.Add($window)
    } catch {
    }
}
foreach (
    $nativeHandle in
    [GrxFirma.QA.FilePickerProbeNative]::EnumerateTopLevelHandles()
) {
    try {
        if (-not $knownHandles.Add($nativeHandle.ToInt64())) {
            continue
        }
        $window = [System.Windows.Automation.AutomationElement]::FromHandle(
            $nativeHandle
        )
        if ($null -ne $window) {
            $windows.Add($window)
        }
    } catch {
    }
}

foreach ($window in $windows) {
    try {
        $process = Get-Process -Id $window.Current.ProcessId -ErrorAction Stop
        if ($process.SessionId -ne $sessionId) {
            continue
        }
        $fileNameElements = $window.FindAll(
            [System.Windows.Automation.TreeScope]::Descendants,
            [System.Windows.Automation.PropertyCondition]::new(
                [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                "1148"
            )
        )
        $acceptElements = $window.FindAll(
            [System.Windows.Automation.TreeScope]::Descendants,
            [System.Windows.Automation.PropertyCondition]::new(
                [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
                "1"
            )
        )
        $isPickerHost = $process.ProcessName -eq "PickerHost"
        if (
            -not $isPickerHost -and
            (
                $fileNameElements.Count -eq 0 -or
                $acceptElements.Count -eq 0
            )
        ) {
            continue
        }

        $fileNames = [System.Collections.Generic.List[object]]::new()
        foreach ($element in $fileNameElements) {
            $valuePattern = $null
            $hasValue = $element.TryGetCurrentPattern(
                [System.Windows.Automation.ValuePattern]::Pattern,
                [ref]$valuePattern
            )
            $value = if ($hasValue) {
                $valuePattern.Current.Value
            } else {
                ""
            }
            $fileNames.Add([ordered]@{
                controlType = $element.Current.ControlType.ProgrammaticName
                className = $element.Current.ClassName
                supportsValue = $hasValue
                valueLength = $value.Length
                valueIsSyntheticQaPath = (
                    -not [string]::IsNullOrWhiteSpace($value) -and
                    $value.StartsWith(
                        $syntheticRoot,
                        [System.StringComparison]::OrdinalIgnoreCase
                    )
                )
            })
        }

        $acceptButtons = [System.Collections.Generic.List[object]]::new()
        foreach ($element in $acceptElements) {
            $invokePattern = $null
            $acceptButtons.Add([ordered]@{
                controlType = $element.Current.ControlType.ProgrammaticName
                className = $element.Current.ClassName
                accessibleName = $element.Current.Name
                isEnabled = $element.Current.IsEnabled
                supportsInvoke = $element.TryGetCurrentPattern(
                    [System.Windows.Automation.InvokePattern]::Pattern,
                    [ref]$invokePattern
                )
            })
        }

        $controls = [System.Collections.Generic.List[object]]::new()
        if ($isPickerHost) {
            $descendants = $window.FindAll(
                [System.Windows.Automation.TreeScope]::Descendants,
                [System.Windows.Automation.Condition]::TrueCondition
            )
            $limit = [math]::Min($descendants.Count, 500)
            for ($index = 0; $index -lt $limit; $index++) {
                try {
                    $element = $descendants.Item($index)
                    if (
                        $element.Current.ControlType -notin @(
                            [System.Windows.Automation.ControlType]::Edit,
                            [System.Windows.Automation.ControlType]::ComboBox,
                            [System.Windows.Automation.ControlType]::Button
                        )
                    ) {
                        continue
                    }
                    $valuePattern = $null
                    $invokePattern = $null
                    $supportsValue = $element.TryGetCurrentPattern(
                        [System.Windows.Automation.ValuePattern]::Pattern,
                        [ref]$valuePattern
                    )
                    $value = if ($supportsValue) {
                        $valuePattern.Current.Value
                    } else {
                        ""
                    }
                    $controls.Add([ordered]@{
                        automationId = $element.Current.AutomationId
                        className = $element.Current.ClassName
                        controlType =
                            $element.Current.ControlType.ProgrammaticName
                        accessibleName = $element.Current.Name
                        isEnabled = $element.Current.IsEnabled
                        supportsValue = $supportsValue
                        valueLength = $value.Length
                        valueIsSyntheticQaPath = (
                            -not [string]::IsNullOrWhiteSpace($value) -and
                            $value.StartsWith(
                                $syntheticRoot,
                                [System.StringComparison]::OrdinalIgnoreCase
                            )
                        )
                        supportsInvoke = $element.TryGetCurrentPattern(
                            [System.Windows.Automation.InvokePattern]::Pattern,
                            [ref]$invokePattern
                        )
                    })
                } catch {
                    continue
                }
            }
        }

        $records.Add([ordered]@{
            processName = $process.ProcessName
            nativeWindowHandle = $window.Current.NativeWindowHandle
            fileNames = $fileNames
            acceptButtons = $acceptButtons
            controls = $controls
        })
    } catch {
        continue
    }
}

$result = [ordered]@{
    schemaVersion = 1
    tool = "GrxFirma file picker probe"
    capturedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    privacy = "No se registran rutas ni nombres de ficheros."
    pickers = $records
}
$json = $result | ConvertTo-Json -Depth 7
[System.IO.File]::WriteAllText(
    $resultFullPath,
    $json,
    [System.Text.UTF8Encoding]::new($false)
)
