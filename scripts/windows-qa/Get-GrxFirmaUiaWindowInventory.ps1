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
    [ValidateRange(1, 2000)]
    [int]$MaximumDescendantsPerWindow = 500
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows -or -not $TestDataOnly) {
    throw "El inventario UIA requiere Windows y -TestDataOnly."
}

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

if (-not ("GrxFirma.QA.UiaInventoryNative" -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

namespace GrxFirma.QA
{
    public sealed class NativeWindowRecord
    {
        public long Handle { get; set; }
        public long OwnerHandle { get; set; }
        public int ProcessId { get; set; }
        public string ClassName { get; set; } = String.Empty;
        public bool IsVisible { get; set; }
        public int Width { get; set; }
        public int Height { get; set; }
    }

    public static class UiaInventoryNative
    {
        private const uint GW_OWNER = 4;
        private delegate bool EnumWindowsProc(IntPtr window, IntPtr parameter);

        [StructLayout(LayoutKind.Sequential)]
        private struct Rect
        {
            public int Left;
            public int Top;
            public int Right;
            public int Bottom;
        }

        [DllImport("user32.dll")]
        private static extern bool EnumWindows(
            EnumWindowsProc callback,
            IntPtr parameter);

        [DllImport("user32.dll")]
        private static extern uint GetWindowThreadProcessId(
            IntPtr window,
            out uint processId);

        [DllImport("user32.dll")]
        private static extern IntPtr GetWindow(IntPtr window, uint command);

        [DllImport("user32.dll")]
        private static extern bool IsWindowVisible(IntPtr window);

        [DllImport("user32.dll")]
        private static extern bool GetWindowRect(IntPtr window, out Rect rectangle);

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetClassName(
            IntPtr window,
            StringBuilder className,
            int maximumCount);

        public static NativeWindowRecord[] Enumerate()
        {
            List<NativeWindowRecord> records = new List<NativeWindowRecord>();
            EnumWindows(delegate(IntPtr window, IntPtr parameter)
            {
                uint processId;
                GetWindowThreadProcessId(window, out processId);
                StringBuilder className = new StringBuilder(256);
                GetClassName(window, className, className.Capacity);
                Rect rectangle;
                GetWindowRect(window, out rectangle);
                records.Add(new NativeWindowRecord
                {
                    Handle = window.ToInt64(),
                    OwnerHandle = GetWindow(window, GW_OWNER).ToInt64(),
                    ProcessId = unchecked((int)processId),
                    ClassName = className.ToString(),
                    IsVisible = IsWindowVisible(window),
                    Width = Math.Max(0, rectangle.Right - rectangle.Left),
                    Height = Math.Max(0, rectangle.Bottom - rectangle.Top)
                });
                return true;
            }, IntPtr.Zero);
            return records.ToArray();
        }
    }
}
'@
}

function Write-JsonAtomically {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)]$Value
    )

    $json = $Value | ConvertTo-Json -Depth 8
    $temporary = "$Path.$([guid]::NewGuid().ToString('N')).tmp"
    [System.IO.File]::WriteAllText(
        $temporary,
        $json,
        [System.Text.UTF8Encoding]::new($false)
    )
    [System.IO.File]::Move($temporary, $Path, $true)
}

function Get-SafeElementRecord {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    try {
        $rectangle = $Element.Current.BoundingRectangle
        return [ordered]@{
            automationId = $Element.Current.AutomationId
            className = $Element.Current.ClassName
            controlType = $Element.Current.ControlType.ProgrammaticName
            frameworkId = $Element.Current.FrameworkId
            isEnabled = $Element.Current.IsEnabled
            isOffscreen = $Element.Current.IsOffscreen
            width = [math]::Round($rectangle.Width)
            height = [math]::Round($rectangle.Height)
        }
    } catch {
        return $null
    }
}

$resultFullPath = [System.IO.Path]::GetFullPath($ResultPath)
$qaRoot = [System.IO.Path]::GetFullPath(
    (Join-Path $env:LOCALAPPDATA "GrxFirma\QA\uia-window-inventory")
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

$currentSession = [System.Diagnostics.Process]::GetCurrentProcess().SessionId
$windowRecords = [System.Collections.Generic.List[object]]::new()
$nativeWindowRecords = [System.Collections.Generic.List[object]]::new()
$root = [System.Windows.Automation.AutomationElement]::RootElement
$windows = $root.FindAll(
    [System.Windows.Automation.TreeScope]::Children,
    [System.Windows.Automation.Condition]::TrueCondition
)

foreach ($window in $windows) {
    try {
        $processId = $window.Current.ProcessId
        if ($processId -le 0) {
            continue
        }
        $process = Get-Process -Id $processId -ErrorAction Stop
        if ($process.SessionId -ne $currentSession) {
            continue
        }

        $descendantRecords = [System.Collections.Generic.List[object]]::new()
        $descendants = $window.FindAll(
            [System.Windows.Automation.TreeScope]::Descendants,
            [System.Windows.Automation.Condition]::TrueCondition
        )
        $limit = [math]::Min(
            $descendants.Count,
            $MaximumDescendantsPerWindow
        )
        for ($index = 0; $index -lt $limit; $index++) {
            $record = Get-SafeElementRecord -Element $descendants.Item($index)
            if (
                $null -ne $record -and
                (
                    -not [string]::IsNullOrWhiteSpace($record.automationId) -or
                    $record.controlType -in @(
                        "ControlType.Edit",
                        "ControlType.Button",
                        "ControlType.ComboBox"
                    )
                )
            ) {
                $descendantRecords.Add($record)
            }
        }

        $windowRecords.Add([ordered]@{
            processName = $process.ProcessName
            processId = $processId
            nativeWindowHandle = $window.Current.NativeWindowHandle
            window = Get-SafeElementRecord -Element $window
            descendantsScanned = $limit
            descendants = $descendantRecords
        })
    } catch {
        continue
    }
}

foreach ($nativeWindow in [GrxFirma.QA.UiaInventoryNative]::Enumerate()) {
    try {
        if ($nativeWindow.ProcessId -le 0) {
            continue
        }
        $process = Get-Process `
            -Id $nativeWindow.ProcessId `
            -ErrorAction Stop
        if ($process.SessionId -ne $currentSession) {
            continue
        }
        $nativeWindowRecords.Add([ordered]@{
            processName = $process.ProcessName
            processId = $nativeWindow.ProcessId
            nativeWindowHandle = $nativeWindow.Handle
            ownerHandle = $nativeWindow.OwnerHandle
            className = $nativeWindow.ClassName
            isVisible = $nativeWindow.IsVisible
            width = $nativeWindow.Width
            height = $nativeWindow.Height
        })
    } catch {
        continue
    }
}

$result = [ordered]@{
    schemaVersion = 1
    tool = "GrxFirma UIA window inventory"
    capturedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    sessionId = $currentSession
    privacy = "No se registran nombres accesibles ni rutas de documentos."
    windows = $windowRecords
    nativeWindows = $nativeWindowRecords
}
Write-JsonAtomically -Path $resultFullPath -Value $result
