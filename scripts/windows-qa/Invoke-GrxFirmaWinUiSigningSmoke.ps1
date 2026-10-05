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
    [string]$ApplicationDirectory,
    [ValidateLength(1, 256)]
    [string]$CertificateNameContains =
        "GrxFirma QA Synthetic Signing",
    [ValidateLength(1, 128)]
    [string]$CertificateEvidenceLabel =
        "synthetic-current-user-non-exportable",
    [ValidateSet("SystemStore", "TemporaryFile", "WindowsImport")]
    [string]$CertificateSource = "SystemStore",
    [string]$OfficialFnmtCredentialPath,
    [switch]$ManualSaveConfirmation,
    [switch]$VisibleSealPades,
    [ValidateSet(0, 90, 180, 270)]
    [int]$VisibleSealRotation = 90,
    [switch]$ExternalPointerInputConfirmation,
    [ValidateRange(30, 3600)]
    [int]$TimeoutSeconds = 120
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows) {
    throw "La prueba de firma WinUI requiere Windows 10 o posterior."
}
if (-not $TestDataOnly) {
    throw "Use -TestDataOnly con el certificado y el documento sinteticos de QA."
}

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

$OfficialFnmtTestCredentialSha256 =
    "6E0CAD97B78BE2918ED54A64A0DD4F3F6E4C16E01B405EF0836FB91B77A3FFB4"

if (-not ("GrxFirma.QA.SigningSmokeNative" -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;

namespace GrxFirma.QA
{
    public static class SigningSmokeNative
    {
        public const uint GW_OWNER = 4;
        private delegate bool EnumWindowsProc(
            IntPtr window,
            IntPtr parameter);

        [DllImport("user32.dll")]
        public static extern IntPtr GetWindow(IntPtr hWnd, uint uCmd);

        [DllImport("user32.dll")]
        private static extern bool SetForegroundWindow(IntPtr window);

        [DllImport("user32.dll")]
        private static extern bool IsWindow(IntPtr window);

        [DllImport("user32.dll")]
        private static extern bool IsWindowVisible(IntPtr window);

        [DllImport("user32.dll")]
        private static extern void keybd_event(
            byte virtualKey,
            byte scanCode,
            uint flags,
            UIntPtr extraInfo);

        [DllImport("user32.dll", SetLastError = true)]
        private static extern uint SendInput(
            uint inputCount,
            INPUT[] inputs,
            int inputSize);

        [DllImport("user32.dll")]
        private static extern bool SetCursorPos(int x, int y);

        [DllImport("user32.dll")]
        private static extern IntPtr SetThreadDpiAwarenessContext(
            IntPtr dpiContext);

        [DllImport("user32.dll")]
        private static extern void mouse_event(
            uint flags,
            uint x,
            uint y,
            uint data,
            UIntPtr extraInfo);

        [DllImport("user32.dll")]
        private static extern bool EnumWindows(
            EnumWindowsProc callback,
            IntPtr parameter);

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetClassName(
            IntPtr hWnd,
            StringBuilder className,
            int maximumCount);

        [StructLayout(LayoutKind.Sequential)]
        private struct INPUT
        {
            public uint type;
            public INPUTUNION value;
        }

        [StructLayout(LayoutKind.Explicit)]
        private struct INPUTUNION
        {
            [FieldOffset(0)]
            public MOUSEINPUT mouse;

            [FieldOffset(0)]
            public KEYBDINPUT keyboard;
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct MOUSEINPUT
        {
            public int dx;
            public int dy;
            public uint mouseData;
            public uint flags;
            public uint time;
            public UIntPtr extraInfo;
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct KEYBDINPUT
        {
            public ushort virtualKey;
            public ushort scanCode;
            public uint flags;
            public uint time;
            public UIntPtr extraInfo;
        }

        public static string ReadClassName(IntPtr hWnd)
        {
            StringBuilder value = new StringBuilder(256);
            int length = GetClassName(hWnd, value, value.Capacity);
            return length <= 0 ? String.Empty : value.ToString(0, length);
        }

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

        public static void SendSelectAllAndUnicodeText(
            IntPtr targetWindow,
            string value)
        {
            const byte VK_CONTROL = 0x11;
            const byte VK_A = 0x41;
            const uint KEYEVENTF_KEYUP = 0x0002;
            const uint INPUT_KEYBOARD = 1;
            const uint KEYEVENTF_UNICODE = 0x0004;

            if (targetWindow == IntPtr.Zero)
            {
                throw new ArgumentException(
                    "La ventana de destino no es valida.",
                    nameof(targetWindow));
            }
            if (String.IsNullOrEmpty(value))
            {
                throw new ArgumentException(
                    "El texto sintetico no puede estar vacio.",
                    nameof(value));
            }

            keybd_event(VK_CONTROL, 0, 0, UIntPtr.Zero);
            keybd_event(VK_A, 0, 0, UIntPtr.Zero);
            keybd_event(VK_A, 0, KEYEVENTF_KEYUP, UIntPtr.Zero);
            keybd_event(VK_CONTROL, 0, KEYEVENTF_KEYUP, UIntPtr.Zero);

            List<INPUT> inputs = new List<INPUT>(value.Length * 2);
            foreach (char character in value)
            {
                INPUT pressed = new INPUT();
                pressed.type = INPUT_KEYBOARD;
                pressed.value.keyboard.scanCode = character;
                pressed.value.keyboard.flags = KEYEVENTF_UNICODE;
                inputs.Add(pressed);

                INPUT released = pressed;
                released.value.keyboard.flags =
                    KEYEVENTF_UNICODE | KEYEVENTF_KEYUP;
                inputs.Add(released);
            }

            INPUT[] inputArray = inputs.ToArray();
            uint sent = SendInput(
                (uint)inputArray.Length,
                inputArray,
                Marshal.SizeOf(typeof(INPUT)));
            if (sent != inputArray.Length)
            {
                throw new InvalidOperationException(
                    "Windows no pudo entregar toda la ruta sintetica al selector.");
            }
        }

        public static void SendLeftClick(
            IntPtr targetWindow,
            int screenX,
            int screenY)
        {
            const uint MOUSEEVENTF_LEFTDOWN = 0x0002;
            const uint MOUSEEVENTF_LEFTUP = 0x0004;
            if (targetWindow == IntPtr.Zero)
            {
                throw new ArgumentException(
                    "La ventana de destino no es valida.",
                    nameof(targetWindow));
            }
            IntPtr previousDpiContext =
                SetThreadDpiAwarenessContext(new IntPtr(-4));
            try
            {
                SetForegroundWindow(targetWindow);
                Thread.Sleep(250);
                if (!SetCursorPos(screenX, screenY))
                {
                    throw new InvalidOperationException(
                        "No se pudo situar el cursor en el control verificado.");
                }
                mouse_event(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, UIntPtr.Zero);
                mouse_event(MOUSEEVENTF_LEFTUP, 0, 0, 0, UIntPtr.Zero);
            }
            finally
            {
                if (previousDpiContext != IntPtr.Zero)
                {
                    SetThreadDpiAwarenessContext(previousDpiContext);
                }
            }
        }

        public static void SendLeftDrag(
            IntPtr targetWindow,
            int startX,
            int startY,
            int endX,
            int endY)
        {
            const uint MOUSEEVENTF_LEFTDOWN = 0x0002;
            const uint MOUSEEVENTF_LEFTUP = 0x0004;
            const int stepCount = 12;
            if (targetWindow == IntPtr.Zero)
            {
                throw new ArgumentException(
                    "La ventana de destino no es valida.",
                    nameof(targetWindow));
            }
            IntPtr previousDpiContext =
                SetThreadDpiAwarenessContext(new IntPtr(-4));
            try
            {
                SetForegroundWindow(targetWindow);
                Thread.Sleep(250);
                if (!SetCursorPos(startX, startY))
                {
                    throw new InvalidOperationException(
                        "No se pudo situar el cursor al comenzar el arrastre.");
                }
                // El primer clic activa el HWND cuando la tarea gráfica aún
                // no tiene derecho de primer plano. El segundo gesto ya llega
                // al control WinUI dentro del mismo contexto DPI físico.
                mouse_event(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, UIntPtr.Zero);
                mouse_event(MOUSEEVENTF_LEFTUP, 0, 0, 0, UIntPtr.Zero);
                Thread.Sleep(250);
                mouse_event(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, UIntPtr.Zero);
                for (int step = 1; step <= stepCount; step++)
                {
                    int x = startX + ((endX - startX) * step / stepCount);
                    int y = startY + ((endY - startY) * step / stepCount);
                    SetCursorPos(x, y);
                    Thread.Sleep(30);
                }
                mouse_event(MOUSEEVENTF_LEFTUP, 0, 0, 0, UIntPtr.Zero);
                Thread.Sleep(150);
            }
            finally
            {
                if (previousDpiContext != IntPtr.Zero)
                {
                    SetThreadDpiAwarenessContext(previousDpiContext);
                }
            }
        }

        public static bool IsVisibleWindow(IntPtr targetWindow)
        {
            return targetWindow != IntPtr.Zero &&
                IsWindow(targetWindow) &&
                IsWindowVisible(targetWindow);
        }

        public static void SendEnter(IntPtr targetWindow)
        {
            const byte VK_RETURN = 0x0D;
            const uint KEYEVENTF_KEYUP = 0x0002;
            if (targetWindow == IntPtr.Zero)
            {
                throw new ArgumentException(
                    "La ventana de destino no es valida.",
                    nameof(targetWindow));
            }
            SetForegroundWindow(targetWindow);
            keybd_event(VK_RETURN, 0, 0, UIntPtr.Zero);
            keybd_event(
                VK_RETURN,
                0,
                KEYEVENTF_KEYUP,
                UIntPtr.Zero);
        }

        public static void SendSpace(IntPtr targetWindow)
        {
            const byte VK_SPACE = 0x20;
            const uint KEYEVENTF_KEYUP = 0x0002;
            if (targetWindow == IntPtr.Zero)
            {
                throw new ArgumentException(
                    "La ventana de destino no es valida.",
                    nameof(targetWindow));
            }
            SetForegroundWindow(targetWindow);
            Thread.Sleep(250);
            keybd_event(VK_SPACE, 0, 0, UIntPtr.Zero);
            keybd_event(
                VK_SPACE,
                0,
                KEYEVENTF_KEYUP,
                UIntPtr.Zero);
        }
    }
}
'@
}

function Assert-LocalPathWithoutReparsePoint {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string]$Label,
        [switch]$AllowMissingLeaf
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if ($fullPath.StartsWith("\\", [System.StringComparison]::Ordinal)) {
        throw "$Label no puede ser una ruta UNC."
    }
    $root = [System.IO.Path]::GetPathRoot($fullPath)
    if ([string]::IsNullOrWhiteSpace($root)) {
        throw "No se pudo determinar el volumen de $Label."
    }
    $drive = [System.IO.DriveInfo]::new($root)
    if (-not $drive.IsReady -or $drive.DriveType -ne [System.IO.DriveType]::Fixed) {
        throw "$Label debe residir en un volumen local fijo."
    }

    $current = if ($AllowMissingLeaf) {
        [System.IO.Path]::GetDirectoryName($fullPath)
    } else {
        $fullPath
    }
    while (-not [string]::IsNullOrWhiteSpace($current)) {
        if (Test-Path -LiteralPath $current) {
            $item = Get-Item -LiteralPath $current -Force
            if (
                ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
            ) {
                throw "$Label no puede atravesar puntos de reanalisis."
            }
        }
        $parent = [System.IO.Path]::GetDirectoryName($current)
        if (
            [string]::IsNullOrWhiteSpace($parent) -or
            $parent.Equals($current, [System.StringComparison]::OrdinalIgnoreCase)
        ) {
            break
        }
        $current = $parent
    }
}

function Protect-QADirectory {
    param([Parameter(Mandatory)][string]$Path)

    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    $icacls = Join-Path $env:WINDIR "System32\icacls.exe"
    $arguments = @(
        $Path,
        "/inheritance:r",
        "/grant:r",
        "*$($currentUser.Value):(OI)(CI)F",
        "*S-1-5-18:(OI)(CI)F"
    )
    & $icacls @arguments | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo proteger el directorio privado de la prueba."
    }
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
    try {
        for ($attempt = 0; $attempt -lt 40; $attempt++) {
            try {
                [System.IO.File]::Move($temporary, $Path, $true)
                return
            } catch [System.IO.IOException] {
                if ($attempt -eq 39) {
                    throw
                }
                Start-Sleep -Milliseconds 50
            }
        }
    } finally {
        if (Test-Path -LiteralPath $temporary -PathType Leaf) {
            [System.IO.File]::Delete($temporary)
        }
    }
}

function Wait-QASignal {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][datetime]$Deadline,
        [Parameter(Mandatory)][string]$Label
    )

    while ([datetime]::UtcNow -lt $Deadline) {
        if (Test-Path -LiteralPath $Path -PathType Leaf) {
            [System.IO.File]::Delete($Path)
            Start-Sleep -Milliseconds 500
            return
        }
        Start-Sleep -Milliseconds 100
    }
    throw "No se confirmó $Label dentro del plazo de QA."
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
                $processId = $window.Current.ProcessId
                if ($processId -le 0) {
                    continue
                }
                $processPath = Get-ProcessPath -ProcessId $processId
                if (
                    -not [string]::IsNullOrWhiteSpace($processPath) -and
                    [System.IO.Path]::GetFullPath($processPath).Equals(
                        $ExpectedProcessPath,
                        [System.StringComparison]::OrdinalIgnoreCase
                    ) -and
                    $window.Current.NativeWindowHandle -ne 0
                ) {
                    return $window
                }
            } catch {
                continue
            }
        }
        Start-Sleep -Milliseconds 250
    }
    throw "No aparecio la ventana WinUI del candidato dentro del plazo."
}

function Find-DescendantByName {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory)]
        [string]$Name
    )

    $condition = [System.Windows.Automation.PropertyCondition]::new(
        [System.Windows.Automation.AutomationElement]::NameProperty,
        $Name
    )
    return $Root.FindFirst(
        [System.Windows.Automation.TreeScope]::Descendants,
        $condition
    )
}

function Wait-DescendantByName {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory)]
        [string]$Name,
        [Parameter(Mandatory)]
        [datetime]$Deadline,
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
    throw "No aparecio o no se habilito el control QA esperado: $Name"
}

function Wait-DescendantNameContains {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Root,
        [Parameter(Mandatory)]
        [string]$Text,
        [Parameter(Mandatory)]
        [datetime]$Deadline
    )

    while ([datetime]::UtcNow -lt $Deadline) {
        try {
            $elements = $Root.FindAll(
                [System.Windows.Automation.TreeScope]::Descendants,
                [System.Windows.Automation.Condition]::TrueCondition
            )
            foreach ($element in $elements) {
                $name = $element.Current.Name
                if (
                    -not [string]::IsNullOrWhiteSpace($name) -and
                    $name.Contains(
                        $Text,
                        [System.StringComparison]::OrdinalIgnoreCase
                    )
                ) {
                    return $element
                }
            }
        } catch {
        }
        Start-Sleep -Milliseconds 250
    }
    throw "No aparecio el estado QA esperado: $Text"
}

function Expand-UiaElement {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $scrollPattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
        Start-Sleep -Milliseconds 250
    }
    $expandPattern = $null
    if (
        -not $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ExpandCollapsePattern]::Pattern,
            [ref]$expandPattern
        )
    ) {
        throw "El control esperado no ofrece ExpandCollapsePattern."
    }
    if (
        $expandPattern.Current.ExpandCollapseState -ne
            [System.Windows.Automation.ExpandCollapseState]::Expanded
    ) {
        $expandPattern.Expand()
        Start-Sleep -Milliseconds 350
    }
}

function Enable-UiaToggle {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $scrollPattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
        Start-Sleep -Milliseconds 250
    }
    $togglePattern = $null
    if (
        -not $Element.TryGetCurrentPattern(
            [System.Windows.Automation.TogglePattern]::Pattern,
            [ref]$togglePattern
        )
    ) {
        throw "El control esperado no ofrece TogglePattern."
    }
    if (
        $togglePattern.Current.ToggleState -ne
            [System.Windows.Automation.ToggleState]::On
    ) {
        $togglePattern.Toggle()
        Start-Sleep -Milliseconds 350
    }
    if (
        $togglePattern.Current.ToggleState -ne
            [System.Windows.Automation.ToggleState]::On
    ) {
        throw "La opción esperada no quedó activada."
    }
}

function Select-UiaComboItem {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Combo,
        [Parameter(Mandatory)][string]$ItemName,
        [Parameter(Mandatory)][datetime]$Deadline
    )

    $scrollPattern = $null
    if (
        $Combo.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
        Start-Sleep -Milliseconds 250
    }
    $expandPattern = $null
    if (
        -not $Combo.TryGetCurrentPattern(
            [System.Windows.Automation.ExpandCollapsePattern]::Pattern,
            [ref]$expandPattern
        )
    ) {
        throw "El selector esperado no admite despliegue accesible."
    }
    $expandPattern.Expand()

    $root = [System.Windows.Automation.AutomationElement]::RootElement
    $condition = [System.Windows.Automation.AndCondition]::new(
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::NameProperty,
            $ItemName
        ),
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
            [System.Windows.Automation.ControlType]::ListItem
        )
    )
    while ([datetime]::UtcNow -lt $Deadline) {
        $item = $root.FindFirst(
            [System.Windows.Automation.TreeScope]::Descendants,
            $condition
        )
        if ($null -ne $item) {
            $selectionItemPattern = $null
            if (
                $item.TryGetCurrentPattern(
                    [System.Windows.Automation.SelectionItemPattern]::Pattern,
                    [ref]$selectionItemPattern
                )
            ) {
                $selectionItemPattern.Select()
                Start-Sleep -Milliseconds 350
                return
            }
        }
        Start-Sleep -Milliseconds 150
    }
    throw "No apareció la opción accesible esperada: $ItemName"
}

function Get-UiaNumericValue {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $candidates = @($Element)
    foreach ($descendant in $Element.FindAll(
        [System.Windows.Automation.TreeScope]::Descendants,
        [System.Windows.Automation.Condition]::TrueCondition
    )) {
        $candidates += $descendant
    }
    foreach ($candidate in $candidates) {
        try {
            $rangePattern = $null
            if (
                $candidate.TryGetCurrentPattern(
                    [System.Windows.Automation.RangeValuePattern]::Pattern,
                    [ref]$rangePattern
                )
            ) {
                return [double]$rangePattern.Current.Value
            }
            $valuePattern = $null
            if (
                $candidate.TryGetCurrentPattern(
                    [System.Windows.Automation.ValuePattern]::Pattern,
                    [ref]$valuePattern
                )
            ) {
                $rawValue = $valuePattern.Current.Value
                foreach ($culture in @(
                    [System.Globalization.CultureInfo]::InvariantCulture,
                    [System.Globalization.CultureInfo]::CurrentCulture
                )) {
                    try {
                        return [Convert]::ToDouble($rawValue, $culture)
                    } catch {
                    }
                }
            }
        } catch {
        }
    }
    throw "El campo numérico no expuso un valor accesible."
}

function Set-UiaNumericValue {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element,
        [Parameter(Mandatory)][double]$Value
    )

    $candidates = @($Element)
    foreach ($descendant in $Element.FindAll(
        [System.Windows.Automation.TreeScope]::Descendants,
        [System.Windows.Automation.Condition]::TrueCondition
    )) {
        $candidates += $descendant
    }
    foreach ($candidate in $candidates) {
        try {
            $rangePattern = $null
            if (
                $candidate.TryGetCurrentPattern(
                    [System.Windows.Automation.RangeValuePattern]::Pattern,
                    [ref]$rangePattern
                ) -and
                -not $rangePattern.Current.IsReadOnly
            ) {
                $rangePattern.SetValue($Value)
                Start-Sleep -Milliseconds 350
                return
            }
            $valuePattern = $null
            if (
                $candidate.TryGetCurrentPattern(
                    [System.Windows.Automation.ValuePattern]::Pattern,
                    [ref]$valuePattern
                ) -and
                -not $valuePattern.Current.IsReadOnly
            ) {
                $valuePattern.SetValue(
                    $Value.ToString(
                        [System.Globalization.CultureInfo]::InvariantCulture
                    )
                )
                Start-Sleep -Milliseconds 350
                return
            }
        } catch {
        }
    }
    throw "El campo numérico no admite cambios accesibles."
}

function Get-UiaRectangleSnapshot {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $scrollPattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
        Start-Sleep -Milliseconds 350
    }
    $rectangle = $Element.Current.BoundingRectangle
    if (
        -not [double]::IsFinite($rectangle.X) -or
        -not [double]::IsFinite($rectangle.Y) -or
        -not [double]::IsFinite($rectangle.Width) -or
        -not [double]::IsFinite($rectangle.Height) -or
        $rectangle.Width -lt 2 -or
        $rectangle.Height -lt 2
    ) {
        throw "El elemento visual no tiene una geometría accesible válida."
    }
    return [ordered]@{
        x = [math]::Round($rectangle.X, 2)
        y = [math]::Round($rectangle.Y, 2)
        width = [math]::Round($rectangle.Width, 2)
        height = [math]::Round($rectangle.Height, 2)
    }
}

function Invoke-UiaLeftDrag {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element,
        [Parameter(Mandatory)][int]$OwnerWindowHandle,
        [Parameter(Mandatory)][int]$DeltaX,
        [Parameter(Mandatory)][int]$DeltaY
    )

    $scrollPattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
        Start-Sleep -Milliseconds 350
    }
    $rectangle = $Element.Current.BoundingRectangle
    if ($rectangle.Width -lt 4 -or $rectangle.Height -lt 4) {
        throw "El elemento que se debe arrastrar no quedó visible."
    }
    $startX = [int][math]::Round($rectangle.X + ($rectangle.Width / 2))
    $startY = [int][math]::Round($rectangle.Y + ($rectangle.Height / 2))
    [GrxFirma.QA.SigningSmokeNative]::SendLeftDrag(
        [IntPtr]$OwnerWindowHandle,
        $startX,
        $startY,
        $startX + $DeltaX,
        $startY + $DeltaY
    )
    Start-Sleep -Milliseconds 500
}

function Build-VerifiedWindowCaptureHelper {
    param(
        [Parameter(Mandatory)][string]$Root,
        [Parameter(Mandatory)][string]$WorkingDirectory
    )

    $project = Join-Path `
        $Root `
        "scripts\windows-qa\GrxFirma.WindowCapture\GrxFirma.WindowCapture.csproj"
    Assert-LocalPathWithoutReparsePoint `
        -Path $project `
        -Label "proyecto del capturador verificado"
    if (-not (Test-Path -LiteralPath $project -PathType Leaf)) {
        throw "No existe el capturador por HWND verificado."
    }
    New-Item `
        -ItemType Directory `
        -Path $WorkingDirectory `
        -ErrorAction Stop | Out-Null
    Protect-QADirectory -Path $WorkingDirectory

    $dotnet = Get-Command dotnet.exe -ErrorAction Stop
    $obj = Join-Path $WorkingDirectory "obj"
    $bin = Join-Path $WorkingDirectory "bin"
    $publish = Join-Path $WorkingDirectory "publish"
    $properties = @(
        "-p:BaseIntermediateOutputPath=$obj\",
        "-p:BaseOutputPath=$bin\",
        "-p:UseSharedCompilation=false"
    )
    & $dotnet.Source restore `
        $project `
        --locked-mode `
        --runtime win-x64 `
        @properties | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo restaurar el capturador por HWND verificado."
    }
    & $dotnet.Source publish `
        $project `
        --configuration Release `
        --runtime win-x64 `
        --self-contained false `
        --no-restore `
        --output $publish `
        @properties | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo compilar el capturador por HWND verificado."
    }
    $helper = Join-Path $publish "GrxFirma.WindowCapture.exe"
    Assert-LocalPathWithoutReparsePoint `
        -Path $helper `
        -Label "capturador por HWND verificado"
    if (-not (Test-Path -LiteralPath $helper -PathType Leaf)) {
        throw "La compilación no produjo el capturador esperado."
    }
    return $helper
}

function Save-UiaWindowCapture {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Window,
        [Parameter(Mandatory)][string]$HelperPath,
        [Parameter(Mandatory)][string]$Path
    )

    $processId = $Window.Current.ProcessId
    $handle = [long]$Window.Current.NativeWindowHandle
    $rectangle = $Window.Current.BoundingRectangle
    $width = [int][math]::Round($rectangle.Width)
    $height = [int][math]::Round($rectangle.Height)
    $process = Get-Process -Id $processId -ErrorAction Stop
    $processPath = [System.IO.Path]::GetFullPath($process.Path)
    $arguments = @(
        "--window-handle",
        ("0x{0}" -f $handle.ToString("x")),
        "--expected-process-id",
        $processId.ToString(
            [System.Globalization.CultureInfo]::InvariantCulture
        ),
        "--expected-process-start-utc-ticks",
        $process.StartTime.ToUniversalTime().Ticks.ToString(
            [System.Globalization.CultureInfo]::InvariantCulture
        ),
        "--expected-process-path",
        $processPath,
        "--expected-process-sha256",
        (Get-FileHash `
            -LiteralPath $processPath `
            -Algorithm SHA256).Hash,
        "--expected-width",
        $width.ToString(
            [System.Globalization.CultureInfo]::InvariantCulture
        ),
        "--expected-height",
        $height.ToString(
            [System.Globalization.CultureInfo]::InvariantCulture
        ),
        "--output",
        $Path,
        "--timeout-ms",
        "5000"
    )
    $helperOutput = @(& $HelperPath @arguments 2>&1)
    if (
        $LASTEXITCODE -ne 0 -or
        -not (Test-Path -LiteralPath $Path -PathType Leaf)
    ) {
        $safeFailure = ($helperOutput | Select-Object -Last 1)
        throw "El capturador por HWND no obtuvo la ventana: $safeFailure"
    }
}

function Invoke-PickerTriggerWithNativeClick {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element,
        [Parameter(Mandatory)][int]$OwnerWindowHandle
    )

    $scrollPattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
        Start-Sleep -Milliseconds 350
    }
    $Element.SetFocus()
    Start-Sleep -Milliseconds 750
    $rectangle = $Element.Current.BoundingRectangle
    if (
        -not [double]::IsFinite($rectangle.X) -or
        -not [double]::IsFinite($rectangle.Y) -or
        -not [double]::IsFinite($rectangle.Width) -or
        -not [double]::IsFinite($rectangle.Height) -or
        $rectangle.Width -lt 2 -or
        $rectangle.Height -lt 2
    ) {
        throw "El boton que abre el selector no quedo visible para QA."
    }
    [GrxFirma.QA.SigningSmokeNative]::SendLeftClick(
        [IntPtr]$OwnerWindowHandle,
        [int][math]::Round($rectangle.X + ($rectangle.Width / 2)),
        [int][math]::Round($rectangle.Y + ($rectangle.Height / 2))
    )
}

function Invoke-UiaButtonPattern {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Element
    )

    $scrollPattern = $null
    if (
        $Element.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
        Start-Sleep -Milliseconds 350
    }
    $invokePattern = $null
    if (
        -not $Element.TryGetCurrentPattern(
            [System.Windows.Automation.InvokePattern]::Pattern,
            [ref]$invokePattern
        )
    ) {
        throw "El botón esperado no ofrece InvokePattern."
    }
    $invokePattern.Invoke()
    Start-Sleep -Milliseconds 1000
}

function Get-TopLevelHandleSet {
    $handles = [System.Collections.Generic.HashSet[int]]::new()
    $windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll(
        [System.Windows.Automation.TreeScope]::Children,
        [System.Windows.Automation.Condition]::TrueCondition
    )
    foreach ($window in $windows) {
        try {
            if ($window.Current.NativeWindowHandle -ne 0) {
                [void]$handles.Add($window.Current.NativeWindowHandle)
            }
        } catch {
        }
    }
    return $handles
}

function Test-OwnedByWindow {
    param(
        [Parameter(Mandatory)][int]$CandidateHandle,
        [Parameter(Mandatory)][int]$OwnerHandle
    )

    $visited = [System.Collections.Generic.HashSet[long]]::new()
    $current = [IntPtr]$CandidateHandle
    while ($current -ne [IntPtr]::Zero -and $visited.Add($current.ToInt64())) {
        $current = [GrxFirma.QA.SigningSmokeNative]::GetWindow(
            $current,
            [GrxFirma.QA.SigningSmokeNative]::GW_OWNER
        )
        if ($current.ToInt64() -eq $OwnerHandle) {
            return $true
        }
    }
    return $false
}

function New-FilePickerFileNameCondition {
    $conditions = @(
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
            "1148"
        ),
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
            "FileNameControlHost"
        ),
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
            "1001"
        )
    )
    return [System.Windows.Automation.OrCondition]::new(
        [System.Windows.Automation.Condition[]]$conditions
    )
}

function Wait-NewOwnedDialog {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$ApplicationWindow,
        [Parameter(Mandatory)]
        [System.Collections.Generic.HashSet[int]]$ExistingHandles,
        [Parameter(Mandatory)][int]$OwnerHandle,
        [Parameter(Mandatory)][int]$OwnerProcessId,
        [Parameter(Mandatory)][datetime]$Deadline
    )

    $root = [System.Windows.Automation.AutomationElement]::RootElement
    $ownerSessionId = (Get-Process -Id $OwnerProcessId -ErrorAction Stop).SessionId
    $fileNameCondition = New-FilePickerFileNameCondition
    $acceptCondition = [System.Windows.Automation.PropertyCondition]::new(
        [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
        "1"
    )
    while ([datetime]::UtcNow -lt $Deadline) {
        # En Windows 10 el IFileDialog puede quedar expuesto por UIA como
        # descendiente del HWND WinUI que lo posee, aunque visualmente sea
        # modal. En Windows 11 suele aparecer como ventana superior separada.
        try {
            if (
                $null -ne $ApplicationWindow.FindFirst(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    $fileNameCondition
                ) -and
                $null -ne $ApplicationWindow.FindFirst(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    $acceptCondition
                )
            ) {
                return $ApplicationWindow
            }
        } catch {
        }

        $windows = $root.FindAll(
            [System.Windows.Automation.TreeScope]::Children,
            [System.Windows.Automation.Condition]::TrueCondition
        )
        foreach ($window in $windows) {
            try {
                $handle = $window.Current.NativeWindowHandle
                $processSessionId = (
                    Get-Process `
                        -Id $window.Current.ProcessId `
                        -ErrorAction Stop
                ).SessionId
                $containsFilePicker = (
                    $null -ne $window.FindFirst(
                        [System.Windows.Automation.TreeScope]::Descendants,
                        $fileNameCondition
                    ) -and
                    $null -ne $window.FindFirst(
                        [System.Windows.Automation.TreeScope]::Descendants,
                        $acceptCondition
                    )
                )
                if (
                    $handle -ne 0 -and
                    $processSessionId -eq $ownerSessionId -and
                    $containsFilePicker -and
                    (
                        $handle -eq $OwnerHandle -or
                        (Test-OwnedByWindow `
                            -CandidateHandle $handle `
                            -OwnerHandle $OwnerHandle) -or
                        -not $ExistingHandles.Contains($handle)
                    )
                ) {
                    return $window
                }
            } catch {
                continue
            }
        }

        # PickerHost de Windows 10 puede crear un HWND superior visible que
        # AutomationElement.RootElement no enumera como hijo. Se convierte
        # entonces el HWND nativo verificado directamente a un elemento UIA.
        foreach (
            $nativeHandle in
            [GrxFirma.QA.SigningSmokeNative]::EnumerateTopLevelHandles()
        ) {
            try {
                $handle = $nativeHandle.ToInt32()
                if ($handle -eq 0) {
                    continue
                }
                $window = [System.Windows.Automation.AutomationElement]::FromHandle(
                    $nativeHandle
                )
                if ($null -eq $window) {
                    continue
                }
                $processSessionId = (
                    Get-Process `
                        -Id $window.Current.ProcessId `
                        -ErrorAction Stop
                ).SessionId
                if (
                    $processSessionId -eq $ownerSessionId -and
                    (
                        $handle -eq $OwnerHandle -or
                        (Test-OwnedByWindow `
                            -CandidateHandle $handle `
                            -OwnerHandle $OwnerHandle) -or
                        -not $ExistingHandles.Contains($handle)
                    ) -and
                    $null -ne $window.FindFirst(
                        [System.Windows.Automation.TreeScope]::Descendants,
                        $fileNameCondition
                    ) -and
                    $null -ne $window.FindFirst(
                        [System.Windows.Automation.TreeScope]::Descendants,
                        $acceptCondition
                    )
                ) {
                    return $window
                }
            } catch {
                continue
            }
        }
        Start-Sleep -Milliseconds 150
    }
    throw "No aparecio el selector de fichero propiedad de WinUI."
}

function Submit-FilePickerPath {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$ApplicationWindow,
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Trigger,
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [datetime]$Deadline,
        [switch]$PickerAlreadyOpen,
        [switch]$ManualConfirmation
    )

    $existingHandles = Get-TopLevelHandleSet
    $ownerHandle = $ApplicationWindow.Current.NativeWindowHandle
    $ownerProcessId = $ApplicationWindow.Current.ProcessId
    # InvokePattern puede quedar bloqueado hasta que un FileSavePicker alojado
    # en PickerHost se cierre. La activación por teclado entrega el evento al
    # botón enfocado y permite que este mismo controlador atienda el diálogo.
    $scrollPattern = $null
    if (
        $Trigger.TryGetCurrentPattern(
            [System.Windows.Automation.ScrollItemPattern]::Pattern,
            [ref]$scrollPattern
        )
    ) {
        $scrollPattern.ScrollIntoView()
    }
    if (-not $PickerAlreadyOpen) {
        Invoke-PickerTriggerWithNativeClick `
            -Element $Trigger `
            -OwnerWindowHandle $ownerHandle
    }
    $dialog = Wait-NewOwnedDialog `
        -ApplicationWindow $ApplicationWindow `
        -ExistingHandles $existingHandles `
        -OwnerHandle $ownerHandle `
        -OwnerProcessId $ownerProcessId `
        -Deadline $Deadline

    $fileNameCondition = New-FilePickerFileNameCondition
    $fileNameElements = $dialog.FindAll(
        [System.Windows.Automation.TreeScope]::Descendants,
        $fileNameCondition
    )
    if ($fileNameElements.Count -eq 0) {
        throw "El selector no expuso el campo de nombre de fichero esperado."
    }

    $valueElement = $null
    foreach ($fileName in $fileNameElements) {
        try {
            if (
                -not $fileName.Current.IsEnabled -or
                $fileName.Current.ControlType -ne
                    [System.Windows.Automation.ControlType]::Edit
            ) {
                continue
            }
            $candidatePattern = $null
            if (
                -not $fileName.TryGetCurrentPattern(
                    [System.Windows.Automation.ValuePattern]::Pattern,
                    [ref]$candidatePattern
                ) -or
                $candidatePattern.Current.IsReadOnly
            ) {
                continue
            }
            $fileName.SetFocus()
            $valueElement = $fileName
            break
        } catch {
            # Algunos contenedores DirectUI publican ValuePattern pero no
            # admiten foco. Se continúa hasta el Edit real.
            continue
        }
    }
    if ($null -eq $valueElement) {
        throw "El campo del selector no admite foco para introducir la ruta."
    }

    # ValuePattern.SetValue entra sincrónicamente en el proveedor UIA de
    # PickerHost. Si se confirma el diálogo desde ese contexto, WinUI no puede
    # devolver el StorageFile y COM produce RPC_E_CANTCALLOUT_ININPUTSYNCCALL.
    # Después de localizar y enfocar el Edit, se abandona UIA y se entrega la
    # ruta como entrada Unicode normal.
    Start-Sleep -Milliseconds 250
    [GrxFirma.QA.SigningSmokeNative]::SendSelectAllAndUnicodeText(
        [IntPtr]$dialog.Current.NativeWindowHandle,
        $Path
    )
    Start-Sleep -Milliseconds 500

    $acceptCondition = [System.Windows.Automation.AndCondition]::new(
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::AutomationIdProperty,
            "1"
        ),
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::ControlTypeProperty,
            [System.Windows.Automation.ControlType]::Button
        )
    )
    $accept = $dialog.FindFirst(
        [System.Windows.Automation.TreeScope]::Descendants,
        $acceptCondition
    )
    if ($null -eq $accept) {
        throw "El selector no expuso su boton de confirmacion esperado."
    }

    if ($ManualConfirmation) {
        $result.phase = "awaiting-manual-save"
        Write-JsonAtomically -Path $resultFullPath -Value $result
        while ([datetime]::UtcNow -lt $Deadline) {
            # No consulte UI Automation mientras la persona pulsa Guardar.
            # PickerHost debe poder devolver StorageFile a WinUI sin que este
            # proceso mantenga una llamada COM de entrada sincrona.
            Start-Sleep -Milliseconds 250
            if ([System.IO.File]::Exists($manualConfirmationPath)) {
                [System.IO.File]::Delete($manualConfirmationPath)
                $result.phase = "manual-save-confirmed"
                Write-JsonAtomically -Path $resultFullPath -Value $result
                Start-Sleep -Milliseconds 1000
                return
            }
        }
        throw "No se confirmo manualmente el selector de guardado dentro del plazo."
    }

    # InvokePattern y WM_COMMAND pueden confirmar un HWND contenedor distinto
    # del botón real cuando PickerHost aloja el diálogo dentro de WinUI. El
    # foco accesible ya verificado y la tecla Intro entregan entrada normal y
    # evitan tanto ese falso positivo como RPC_E_CANTCALLOUT.
    $dialogHandle = [IntPtr]$dialog.Current.NativeWindowHandle
    $applicationWindowHandle =
        [IntPtr]$ApplicationWindow.Current.NativeWindowHandle
    Invoke-PickerTriggerWithNativeClick `
        -Element $accept `
        -OwnerWindowHandle $dialogHandle

    # No consulte de nuevo el proveedor UIA mientras PickerHost devuelve el
    # StorageFile a WinUI. Una llamada COM entrante en ese intervalo puede
    # provocar RPC_E_CANTCALLOUT_ININPUTSYNCCALL en la aplicación. El estado
    # nativo del HWND basta para observar el cierre sin entrar en su proveedor.
    # En Windows 10 PickerHost puede quedar incrustado en el HWND principal:
    # ese HWND no debe desaparecer; el siguiente paso funcional confirmará que
    # el selector se cerró y que WinUI recibió el StorageFile.
    if ($dialogHandle -eq $applicationWindowHandle) {
        Start-Sleep -Milliseconds 1500
        return
    }

    $pickerClosedDeadline = [datetime]::UtcNow.AddSeconds(5)
    while ([datetime]::UtcNow -lt $pickerClosedDeadline) {
        Start-Sleep -Milliseconds 200
        if (
            -not [GrxFirma.QA.SigningSmokeNative]::
                IsVisibleWindow($dialogHandle)
        ) {
            return
        }
    }

    # En máquinas de QA lentas el clic físico puede quedar consumido por el
    # cambio de foco. Se permite un único reintento con Intro sobre el botón
    # predeterminado, también como entrada nativa y sin InvokePattern.
    [GrxFirma.QA.SigningSmokeNative]::SendEnter($dialogHandle)
    $retryDeadline = [datetime]::UtcNow.AddSeconds(5)
    while ([datetime]::UtcNow -lt $retryDeadline) {
        Start-Sleep -Milliseconds 200
        if (
            -not [GrxFirma.QA.SigningSmokeNative]::
                IsVisibleWindow($dialogHandle)
        ) {
            return
        }
    }
    throw "El selector no confirmo la ruta sintetica de QA."
}

function Wait-SecureCredentialPasswordDialog {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$ApplicationWindow,
        [Parameter(Mandatory)][datetime]$Deadline
    )

    $applicationProcessId = $ApplicationWindow.Current.ProcessId
    $applicationWindowHandle =
        $ApplicationWindow.Current.NativeWindowHandle
    $root = [System.Windows.Automation.AutomationElement]::RootElement
    $nameCondition =
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::NameProperty,
            "Contraseña de la credencial"
        )
    while ([datetime]::UtcNow -lt $Deadline) {
        $dialogs = @()
        # Windows 10 puede exponer el diálogo Win32 modal como descendiente
        # UIA del HWND WinUI propietario, igual que ocurre con IFileDialog.
        # Windows 11 suele publicarlo como ventana superior independiente.
        try {
            $nestedDialog = $ApplicationWindow.FindFirst(
                [System.Windows.Automation.TreeScope]::Descendants,
                $nameCondition
            )
            if ($null -ne $nestedDialog) {
                $dialogs += $nestedDialog
            }
        } catch {
        }
        $dialogs += @($root.FindAll(
            [System.Windows.Automation.TreeScope]::Children,
            $nameCondition
        ))
        foreach ($dialog in $dialogs) {
            try {
                $handle = $dialog.Current.NativeWindowHandle
                if (
                    $dialog.Current.ProcessId -eq $applicationProcessId -and
                    $handle -ne 0 -and
                    (
                        $handle -eq $applicationWindowHandle -or
                        (Test-OwnedByWindow `
                            -CandidateHandle $handle `
                            -OwnerHandle $applicationWindowHandle)
                    )
                ) {
                    return $dialog
                }
            } catch {
                continue
            }
        }
        Start-Sleep -Milliseconds 150
    }
    throw "No apareció el diálogo seguro para la credencial FNMT de pruebas."
}

function Submit-OfficialFnmtTestPassword {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$ApplicationWindow,
        [Parameter(Mandatory)][datetime]$Deadline
    )

    $dialog = Wait-SecureCredentialPasswordDialog `
        -ApplicationWindow $ApplicationWindow `
        -Deadline $Deadline
    $dialogHandle = [IntPtr]$dialog.Current.NativeWindowHandle
    $editCondition =
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::
                AutomationIdProperty,
            "101"
        )
    $edit = $dialog.FindFirst(
        [System.Windows.Automation.TreeScope]::Descendants,
        $editCondition
    )
    if (
        $null -eq $edit -or
        $edit.Current.ControlType -ne
            [System.Windows.Automation.ControlType]::Edit
    ) {
        $passwordCondition =
            [System.Windows.Automation.AndCondition]::new(
                [System.Windows.Automation.Condition[]]@(
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::
                            ControlTypeProperty,
                        [System.Windows.Automation.ControlType]::Edit
                    ),
                    [System.Windows.Automation.PropertyCondition]::new(
                        [System.Windows.Automation.AutomationElement]::
                            IsPasswordProperty,
                        $true
                    )
                )
            )
        $edit = $dialog.FindFirst(
            [System.Windows.Automation.TreeScope]::Descendants,
            $passwordCondition
        )
    }
    if ($null -eq $edit) {
        throw (
            "El diálogo seguro no expuso un campo de contraseña " +
            "marcado por Windows."
        )
    }

    $edit.SetFocus()
    Start-Sleep -Milliseconds 250
    # 1234 es la contraseña pública del archivo oficial de certificados de
    # prueba de la FNMT. Nunca se usa ni se busca una credencial personal.
    [GrxFirma.QA.SigningSmokeNative]::SendSelectAllAndUnicodeText(
        $dialogHandle,
        "1234"
    )
    Start-Sleep -Milliseconds 250

    $accept = Find-DescendantByName `
        -Root $dialog `
        -Name "Aceptar"
    if ($null -eq $accept -or -not $accept.Current.IsEnabled) {
        throw "El diálogo seguro no expuso el botón Aceptar."
    }
    Invoke-PickerTriggerWithNativeClick `
        -Element $accept `
        -OwnerWindowHandle $dialog.Current.NativeWindowHandle

    $applicationWindowHandle =
        [IntPtr]$ApplicationWindow.Current.NativeWindowHandle
    if ($dialogHandle -eq $applicationWindowHandle) {
        # Windows 10 puede alojar CredUI dentro del mismo HWND WinUI. Ese HWND
        # principal no desaparece al aceptar; el siguiente paso funcional
        # confirma que la credencial quedó cargada.
        Start-Sleep -Milliseconds 1500
        return
    }

    $dialogNameCondition =
        [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::NameProperty,
            "Contraseña de la credencial"
        )
    $automationRoot =
        [System.Windows.Automation.AutomationElement]::RootElement
    while ([datetime]::UtcNow -lt $Deadline) {
        Start-Sleep -Milliseconds 200
        if (
            -not [GrxFirma.QA.SigningSmokeNative]::
                IsVisibleWindow($dialogHandle)
        ) {
            Start-Sleep -Milliseconds 1000
            return
        }

        # Algunos hosts de CredUI de Windows 10 conservan un HWND visible
        # después de retirar el contenido modal. UIA es la señal funcional:
        # si ya no expone el diálogo, no debemos esperar hasta agotar el plazo.
        $dialogStillExposed = $false
        foreach ($scope in @($ApplicationWindow, $automationRoot)) {
            try {
                $candidate = $scope.FindFirst(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    $dialogNameCondition
                )
                if (
                    $null -ne $candidate -and
                    -not $candidate.Current.IsOffscreen
                ) {
                    $dialogStillExposed = $true
                    break
                }
            } catch {
            }
        }
        if (-not $dialogStillExposed) {
            Start-Sleep -Milliseconds 1000
            return
        }
    }
    throw "El diálogo seguro de la credencial FNMT no se cerró."
}

function Use-CertificateSourceForQa {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$ApplicationWindow,
        [Parameter(Mandatory)]
        [ValidateSet("SystemStore", "TemporaryFile", "WindowsImport")]
        [string]$Source,
        [string]$CredentialPath,
        [Parameter(Mandatory)][datetime]$Deadline
    )

    $applicationHandle = $ApplicationWindow.Current.NativeWindowHandle
    if ($Source -eq "SystemStore") {
        $refresh = Wait-DescendantByName `
            -Root $ApplicationWindow `
            -Name "Buscar certificados en Windows" `
            -Deadline $Deadline `
            -RequireEnabled
        Invoke-PickerTriggerWithNativeClick `
            -Element $refresh `
            -OwnerWindowHandle $applicationHandle
        Start-Sleep -Milliseconds 1500
        return
    }

    $sourceButtonName = if ($Source -eq "TemporaryFile") {
        "Usar archivo de certificado solo durante esta sesión"
    } else {
        "Importar archivo de certificado en Windows"
    }
    $sourceButton = Wait-DescendantByName `
        -Root $ApplicationWindow `
        -Name $sourceButtonName `
        -Deadline $Deadline `
        -RequireEnabled

    if ($Source -eq "WindowsImport") {
        Invoke-PickerTriggerWithNativeClick `
            -Element $sourceButton `
            -OwnerWindowHandle $applicationHandle
        $confirmation = Wait-DescendantByName `
            -Root $ApplicationWindow `
            -Name "Importar en Windows" `
            -Deadline $Deadline `
            -RequireEnabled
        Invoke-PickerTriggerWithNativeClick `
            -Element $confirmation `
            -OwnerWindowHandle $applicationHandle
        Start-Sleep -Milliseconds 500
        Submit-FilePickerPath `
            -ApplicationWindow $ApplicationWindow `
            -Trigger $sourceButton `
            -Path $CredentialPath `
            -Deadline $Deadline `
            -PickerAlreadyOpen
    } else {
        Submit-FilePickerPath `
            -ApplicationWindow $ApplicationWindow `
            -Trigger $sourceButton `
            -Path $CredentialPath `
            -Deadline $Deadline
    }

    Submit-OfficialFnmtTestPassword `
        -ApplicationWindow $ApplicationWindow `
        -Deadline $Deadline
}

function Select-CertificateByName {
    param(
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$ApplicationWindow,
        [Parameter(Mandatory)]
        [System.Windows.Automation.AutomationElement]$Combo,
        [Parameter(Mandatory)]
        [string]$NameContains,
        [Parameter(Mandatory)]
        [datetime]$Deadline
    )

    $selectionPattern = $null
    if (
        $Combo.TryGetCurrentPattern(
            [System.Windows.Automation.SelectionPattern]::Pattern,
            [ref]$selectionPattern
        )
    ) {
        foreach ($selected in $selectionPattern.Current.GetSelection()) {
            if (
                $selected.Current.Name.Contains(
                    $NameContains,
                    [System.StringComparison]::OrdinalIgnoreCase
                )
            ) {
                return
            }
        }
    }

    $expandPattern = $null
    if (
        -not $Combo.TryGetCurrentPattern(
            [System.Windows.Automation.ExpandCollapsePattern]::Pattern,
            [ref]$expandPattern
        )
    ) {
        throw "El selector de certificado no admite despliegue accesible."
    }
    $expandPattern.Expand()

    $root = [System.Windows.Automation.AutomationElement]::RootElement
    while ([datetime]::UtcNow -lt $Deadline) {
        $elements = $root.FindAll(
            [System.Windows.Automation.TreeScope]::Descendants,
            [System.Windows.Automation.Condition]::TrueCondition
        )
        foreach ($element in $elements) {
            try {
                if (
                    $element.Current.ControlType -ne
                        [System.Windows.Automation.ControlType]::ListItem -or
                    -not $element.Current.Name.Contains(
                        $NameContains,
                        [System.StringComparison]::OrdinalIgnoreCase
                    )
                ) {
                    continue
                }
                $selectPattern = $null
                if (
                    $element.TryGetCurrentPattern(
                        [System.Windows.Automation.SelectionItemPattern]::Pattern,
                        [ref]$selectPattern
                    )
                ) {
                    $selectPattern.Select()
                    return
                }
            } catch {
                continue
            }
        }
        Start-Sleep -Milliseconds 200
    }
    throw "El certificado de QA esperado no aparecio en el selector WinUI."
}

$startedAtUtc = [DateTimeOffset]::UtcNow
$resolvedRoot = (Resolve-Path -LiteralPath $RepositoryRoot -ErrorAction Stop).Path
$applicationRoot = if ([string]::IsNullOrWhiteSpace($ApplicationDirectory)) {
    Join-Path `
        $resolvedRoot `
        "release\windows-desktop-winui\GrxFirma-$((Get-Content -LiteralPath (Join-Path $resolvedRoot "VERSION.txt") -Raw).Trim())-desktop-winui-windows-amd64\app"
} else {
    [System.IO.Path]::GetFullPath($ApplicationDirectory)
}
$winUiExecutable = Join-Path $applicationRoot "grxfirma-winui.exe"
foreach ($path in @($applicationRoot, $winUiExecutable)) {
    Assert-LocalPathWithoutReparsePoint -Path $path -Label "stage WinUI"
}
if (
    -not (Test-Path -LiteralPath $winUiExecutable -PathType Leaf)
) {
    throw "El stage WinUI no esta completo."
}

$credentialFullPath = $null
if ($CertificateSource -ne "SystemStore") {
    if ([string]::IsNullOrWhiteSpace($OfficialFnmtCredentialPath)) {
        throw "La ruta de la credencial FNMT oficial de pruebas es obligatoria."
    }
    $credentialFullPath =
        [System.IO.Path]::GetFullPath($OfficialFnmtCredentialPath)
    $officialFnmtQaRoot = [System.IO.Path]::GetFullPath(
        (Join-Path `
            $env:LOCALAPPDATA `
            "GrxFirma\QA\fnmt-official-test")
    )
    if (
        -not $credentialFullPath.StartsWith(
            $officialFnmtQaRoot +
                [System.IO.Path]::DirectorySeparatorChar,
            [System.StringComparison]::OrdinalIgnoreCase
        ) -or
        -not [System.IO.Path]::GetFileName($credentialFullPath).Equals(
            "fnmt-test.p12",
            [System.StringComparison]::OrdinalIgnoreCase
        )
    ) {
        throw "Solo se admite el P12 oficial aislado de QA; no se buscan credenciales personales."
    }
    Assert-LocalPathWithoutReparsePoint `
        -Path $credentialFullPath `
        -Label "credencial FNMT oficial de pruebas"
    if (
        -not (Test-Path -LiteralPath $credentialFullPath -PathType Leaf) -or
        (Get-FileHash `
            -LiteralPath $credentialFullPath `
            -Algorithm SHA256).Hash -ne
                $OfficialFnmtTestCredentialSha256
    ) {
        throw "La credencial FNMT de QA no coincide con el archivo oficial esperado."
    }
} elseif (-not [string]::IsNullOrWhiteSpace($OfficialFnmtCredentialPath)) {
    throw "SystemStore no debe recibir una ruta de credencial."
}

$resultFullPath = [System.IO.Path]::GetFullPath($ResultPath)
Assert-LocalPathWithoutReparsePoint `
    -Path $resultFullPath `
    -Label "ResultPath" `
    -AllowMissingLeaf
$qaRoot = Join-Path $env:LOCALAPPDATA "GrxFirma\QA\winui-signing-smoke"
$qaRoot = [System.IO.Path]::GetFullPath($qaRoot)
if (
    -not $resultFullPath.StartsWith(
        $qaRoot + [System.IO.Path]::DirectorySeparatorChar,
        [System.StringComparison]::OrdinalIgnoreCase
    )
) {
    throw "ResultPath debe estar dentro del directorio fijo de QA."
}
if (-not (Test-Path -LiteralPath $qaRoot -PathType Container)) {
    New-Item -ItemType Directory -Path $qaRoot -Force | Out-Null
}
Protect-QADirectory -Path $qaRoot

$runId = [System.IO.Path]::GetFileNameWithoutExtension($resultFullPath)
if ($runId -notmatch "^\d{8}T\d{9}Z-[a-f0-9]{8}$") {
    throw "El nombre de ResultPath no corresponde a una ejecucion QA."
}
$runDirectory = Join-Path $qaRoot $runId
New-Item -ItemType Directory -Path $runDirectory -ErrorAction Stop | Out-Null
Protect-QADirectory -Path $runDirectory
$manualConfirmationPath = Join-Path `
    $runDirectory `
    "manual-save-confirmed.signal"
$externalPointerMovePath = Join-Path `
    $runDirectory `
    "external-pointer-move.signal"
$externalPointerResizePath = Join-Path `
    $runDirectory `
    "external-pointer-resize.signal"

$documentsDirectory = [Environment]::GetFolderPath(
    [Environment+SpecialFolder]::MyDocuments
)
if ([string]::IsNullOrWhiteSpace($documentsDirectory)) {
    throw "Windows no devolvio una carpeta Documentos local para PickerHost."
}
Assert-LocalPathWithoutReparsePoint `
    -Path $documentsDirectory `
    -Label "Documentos"
$pickerOutputRoot = Join-Path $documentsDirectory "GrxFirma-QA"
if (-not (Test-Path -LiteralPath $pickerOutputRoot -PathType Container)) {
    New-Item `
        -ItemType Directory `
        -Path $pickerOutputRoot `
        -ErrorAction Stop | Out-Null
}
Assert-LocalPathWithoutReparsePoint `
    -Path $pickerOutputRoot `
    -Label "salida temporal de PickerHost"
$pickerRunDirectory = Join-Path $pickerOutputRoot $runId
New-Item `
    -ItemType Directory `
    -Path $pickerRunDirectory `
    -ErrorAction Stop | Out-Null
Assert-LocalPathWithoutReparsePoint `
    -Path $pickerRunDirectory `
    -Label "ejecucion temporal de PickerHost"

$inputPath = if ($VisibleSealPades) {
    Join-Path $runDirectory "documento-sintetico.pdf"
} else {
    Join-Path $runDirectory "documento-sintetico.txt"
}
$signedFileName = if ($VisibleSealPades) {
    "documento-sintetico-firmado-$runId.pdf"
} else {
    "documento-sintetico-firmado-$runId.p7s"
}
$signedPath = Join-Path $pickerRunDirectory $signedFileName
$manualPickerSignedPath = Join-Path $documentsDirectory $signedFileName
$signatureEvidenceName = if ($VisibleSealPades) {
    "documento-sintetico-firmado.pdf"
} else {
    "documento-sintetico-firmado.p7s"
}
$signatureEvidencePath = Join-Path `
    $runDirectory `
    $signatureEvidenceName
$visibleSealCapturePath = Join-Path `
    $runDirectory `
    "sello-visible-configurado.png"
if ($VisibleSealPades) {
    $pdfFixturePath = Join-Path $resolvedRoot "test\prueba1.pdf"
    Assert-LocalPathWithoutReparsePoint `
        -Path $pdfFixturePath `
        -Label "PDF sintético de QA"
    if (
        -not (Test-Path -LiteralPath $pdfFixturePath -PathType Leaf) -or
        (Get-FileHash `
            -LiteralPath $pdfFixturePath `
            -Algorithm SHA256).Hash -ne
                "16CF7D1F8296E28DD3EC6070237CFA2354AB54B835C7DC7B77B42B3112FEFA71"
    ) {
        throw "El PDF sintético de QA no coincide con el fixture esperado."
    }
    Copy-Item `
        -LiteralPath $pdfFixturePath `
        -Destination $inputPath `
        -ErrorAction Stop
} else {
    $inputContents = (
        "GrxFirma QA signing smoke test`r`n" +
        "Run: $runId`r`n" +
        "No contiene datos personales ni produce efectos administrativos.`r`n"
    )
    [System.IO.File]::WriteAllText(
        $inputPath,
        $inputContents,
        [System.Text.UTF8Encoding]::new($false)
    )
}

$result = [ordered]@{
    schemaVersion = 1
    tool = "GrxFirma WinUI synthetic signing smoke"
    runId = $runId
    status = "running"
    phase = "starting"
    startedAtUtc = $startedAtUtc.ToString("O")
    completedAtUtc = $null
    certificate = $CertificateEvidenceLabel
    certificateNameContains = $CertificateNameContains
    certificateSource = $CertificateSource
    document = if ($VisibleSealPades) {
        "synthetic-pdf"
    } else {
        "synthetic-text"
    }
    signatureFormat = if ($VisibleSealPades) {
        "pades"
    } else {
        "cades"
    }
    visibleSealRequested = [bool]$VisibleSealPades
    visibleSealRotation = if ($VisibleSealPades) {
        $VisibleSealRotation
    } else {
        $null
    }
    visibleSealInitialGeometry = $null
    visibleSealFinalGeometry = $null
    visibleSealMoveObserved = $false
    visibleSealResizeObserved = $false
    visibleSealDirectPointerMoveObserved = $false
    visibleSealDirectPointerResizeObserved = $false
    visibleSealNumericFallbackUsed = $false
    externalPointerInputRequested =
        [bool]$ExternalPointerInputConfirmation
    visibleSealMovedGeometry = $null
    visibleSealResizeHandleRectangle = $null
    visibleSealCapturePath = $null
    visibleSealCaptureSha256 = $null
    visibleSealPreviewButtonEnabledAfterRequest = $null
    signatureCreated = $false
    signatureBytes = 0
    signatureSha256 = $null
    postValidationObserved = $false
    independentVerificationObserved = $false
    protectedEvidenceCopied = $false
    temporaryOutputRemoved = $false
    manualSaveConfirmationRequested = [bool]$ManualSaveConfirmation
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

    Use-CertificateSourceForQa `
        -ApplicationWindow $window `
        -Source $CertificateSource `
        -CredentialPath $credentialFullPath `
        -Deadline $deadline
    $result.phase = "certificate-source-ready"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    $selectDocument = Wait-DescendantByName `
        -Root $window `
        -Name "Seleccionar documento para firmar" `
        -Deadline $deadline `
        -RequireEnabled
    $result.phase = "input-picker-opened"
    Write-JsonAtomically -Path $resultFullPath -Value $result
    Submit-FilePickerPath `
        -ApplicationWindow $window `
        -Trigger $selectDocument `
        -Path $inputPath `
        -Deadline $deadline
    $result.phase = "input-selected"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    $certificateCombo = Wait-DescendantByName `
        -Root $window `
        -Name "Certificado de firma" `
        -Deadline $deadline `
        -RequireEnabled
    Select-CertificateByName `
        -ApplicationWindow $window `
        -Combo $certificateCombo `
        -NameContains $CertificateNameContains `
        -Deadline $deadline
    $result.phase = "certificate-selected"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    if ($VisibleSealPades) {
        $advancedOptions = Wait-DescendantByName `
            -Root $window `
            -Name "Opciones avanzadas de firma" `
            -Deadline $deadline `
            -RequireEnabled
        Expand-UiaElement -Element $advancedOptions
        $result.phase = "visible-seal-options-expanded"
        Write-JsonAtomically -Path $resultFullPath -Value $result

        $visibleSealToggle = Wait-DescendantByName `
            -Root $window `
            -Name "Añadir sello visible en PDF" `
            -Deadline $deadline `
            -RequireEnabled
        Enable-UiaToggle -Element $visibleSealToggle
        $result.phase = "visible-seal-enabled"
        Write-JsonAtomically -Path $resultFullPath -Value $result

        $loadPreview = Wait-DescendantByName `
            -Root $window `
            -Name "Cargar previsualización PDF para el sello" `
            -Deadline $deadline `
            -RequireEnabled
        $result.phase = "visible-seal-preview-trigger-ready"
        Write-JsonAtomically -Path $resultFullPath -Value $result
        Invoke-UiaButtonPattern -Element $loadPreview
        $result.visibleSealPreviewButtonEnabledAfterRequest =
            $loadPreview.Current.IsEnabled
        $result.phase = "visible-seal-preview-requested"
        Write-JsonAtomically -Path $resultFullPath -Value $result
        [void](Wait-DescendantNameContains `
            -Root $window `
            -Text "PDF real:" `
            -Deadline $deadline)

        $geometryControls = [ordered]@{
            x = Wait-DescendantByName `
                -Root $window `
                -Name "Posición horizontal del sello en porcentaje" `
                -Deadline $deadline
            y = Wait-DescendantByName `
                -Root $window `
                -Name "Posición vertical del sello en porcentaje" `
                -Deadline $deadline
            width = Wait-DescendantByName `
                -Root $window `
                -Name "Ancho del sello en porcentaje" `
                -Deadline $deadline
            height = Wait-DescendantByName `
                -Root $window `
                -Name "Alto del sello en porcentaje" `
                -Deadline $deadline
        }
        $visibleSealRegion = Wait-DescendantByName `
            -Root $window `
            -Name "Zona del sello visible" `
            -Deadline $deadline
        $initialGeometry = [ordered]@{
            xPercent = Get-UiaNumericValue -Element $geometryControls.x
            yPercent = Get-UiaNumericValue -Element $geometryControls.y
            widthPercent =
                Get-UiaNumericValue -Element $geometryControls.width
            heightPercent =
                Get-UiaNumericValue -Element $geometryControls.height
            screenRectangle =
                Get-UiaRectangleSnapshot -Element $visibleSealRegion
        }
        $result.visibleSealInitialGeometry = $initialGeometry
        $result.phase = "visible-seal-preview-ready"
        Write-JsonAtomically -Path $resultFullPath -Value $result

        if ($ExternalPointerInputConfirmation) {
            $result.phase = "awaiting-external-pointer-move"
            Write-JsonAtomically -Path $resultFullPath -Value $result
            Wait-QASignal `
                -Path $externalPointerMovePath `
                -Deadline $deadline `
                -Label "el arrastre externo de posición"
        } else {
            Invoke-UiaLeftDrag `
                -Element $visibleSealRegion `
                -OwnerWindowHandle $window.Current.NativeWindowHandle `
                -DeltaX -60 `
                -DeltaY -40
        }
        $movedGeometry = [ordered]@{
            xPercent = Get-UiaNumericValue -Element $geometryControls.x
            yPercent = Get-UiaNumericValue -Element $geometryControls.y
        }
        if (
            [math]::Abs(
                $movedGeometry.xPercent - $initialGeometry.xPercent
            ) -lt 0.25 -or
            [math]::Abs(
                $movedGeometry.yPercent - $initialGeometry.yPercent
            ) -lt 0.25
        ) {
            Set-UiaNumericValue `
                -Element $geometryControls.x `
                -Value ($initialGeometry.xPercent - 10)
            Set-UiaNumericValue `
                -Element $geometryControls.y `
                -Value ($initialGeometry.yPercent + 8)
            $movedGeometry.xPercent =
                Get-UiaNumericValue -Element $geometryControls.x
            $movedGeometry.yPercent =
                Get-UiaNumericValue -Element $geometryControls.y
            $result.visibleSealNumericFallbackUsed = $true
        } else {
            $result.visibleSealDirectPointerMoveObserved = $true
        }
        if (
            [math]::Abs(
                $movedGeometry.xPercent - $initialGeometry.xPercent
            ) -lt 0.25 -or
            [math]::Abs(
                $movedGeometry.yPercent - $initialGeometry.yPercent
            ) -lt 0.25
        ) {
            throw "No cambió la posición X/Y del sello mediante puntero ni campos accesibles."
        }
        $result.visibleSealMoveObserved = $true
        $visibleSealRegion = Wait-DescendantByName `
            -Root $window `
            -Name "Zona del sello visible" `
            -Deadline $deadline
        $result.visibleSealMovedGeometry = [ordered]@{
            xPercent = $movedGeometry.xPercent
            yPercent = $movedGeometry.yPercent
            screenRectangle =
                Get-UiaRectangleSnapshot -Element $visibleSealRegion
        }

        $resizeHandle = Wait-DescendantByName `
            -Root $window `
            -Name "Redimensionar zona del sello" `
            -Deadline $deadline
        $result.visibleSealResizeHandleRectangle =
            Get-UiaRectangleSnapshot -Element $resizeHandle
        if ($ExternalPointerInputConfirmation) {
            $result.phase = "awaiting-external-pointer-resize"
            Write-JsonAtomically -Path $resultFullPath -Value $result
            Wait-QASignal `
                -Path $externalPointerResizePath `
                -Deadline $deadline `
                -Label "el arrastre externo de tamaño"
        } else {
            Invoke-UiaLeftDrag `
                -Element $resizeHandle `
                -OwnerWindowHandle $window.Current.NativeWindowHandle `
                -DeltaX 35 `
                -DeltaY 25
        }
        $resizedWidth =
            Get-UiaNumericValue -Element $geometryControls.width
        $resizedHeight =
            Get-UiaNumericValue -Element $geometryControls.height
        if (
            [math]::Abs(
                $resizedWidth - $initialGeometry.widthPercent
            ) -lt 0.25 -or
            [math]::Abs(
                $resizedHeight - $initialGeometry.heightPercent
            ) -lt 0.25
        ) {
            Set-UiaNumericValue `
                -Element $geometryControls.width `
                -Value ($initialGeometry.widthPercent + 5)
            Set-UiaNumericValue `
                -Element $geometryControls.height `
                -Value ($initialGeometry.heightPercent + 5)
            $resizedWidth =
                Get-UiaNumericValue -Element $geometryControls.width
            $resizedHeight =
                Get-UiaNumericValue -Element $geometryControls.height
            $result.visibleSealNumericFallbackUsed = $true
        } else {
            $result.visibleSealDirectPointerResizeObserved = $true
        }
        if (
            [math]::Abs(
                $resizedWidth - $initialGeometry.widthPercent
            ) -lt 0.25 -or
            [math]::Abs(
                $resizedHeight - $initialGeometry.heightPercent
            ) -lt 0.25
        ) {
            throw "No cambió el ancho/alto del sello mediante tirador ni campos accesibles."
        }
        $result.visibleSealResizeObserved = $true

        $rotationCombo = Wait-DescendantByName `
            -Root $window `
            -Name "Rotación del sello visible" `
            -Deadline $deadline `
            -RequireEnabled
        $rotationLabel = if ($VisibleSealRotation -eq 0) {
            "Sin rotación"
        } else {
            "$VisibleSealRotation°"
        }
        Select-UiaComboItem `
            -Combo $rotationCombo `
            -ItemName $rotationLabel `
            -Deadline $deadline

        $visibleSealRegion = Wait-DescendantByName `
            -Root $window `
            -Name "Zona del sello visible" `
            -Deadline $deadline
        $result.visibleSealFinalGeometry = [ordered]@{
            xPercent = Get-UiaNumericValue -Element $geometryControls.x
            yPercent = Get-UiaNumericValue -Element $geometryControls.y
            widthPercent =
                Get-UiaNumericValue -Element $geometryControls.width
            heightPercent =
                Get-UiaNumericValue -Element $geometryControls.height
            screenRectangle =
                Get-UiaRectangleSnapshot -Element $visibleSealRegion
        }
        $previewSurface = Wait-DescendantByName `
            -Root $window `
            -Name "Editor visual de posición del sello sobre la página PDF" `
            -Deadline $deadline
        $previewScrollPattern = $null
        if (
            $previewSurface.TryGetCurrentPattern(
                [System.Windows.Automation.ScrollItemPattern]::Pattern,
                [ref]$previewScrollPattern
            )
        ) {
            $previewScrollPattern.ScrollIntoView()
            Start-Sleep -Milliseconds 500
        }
        $captureHelperDirectory = Join-Path `
            $runDirectory `
            "capture-helper"
        try {
            $captureHelper = Build-VerifiedWindowCaptureHelper `
                -Root $resolvedRoot `
                -WorkingDirectory $captureHelperDirectory
            Save-UiaWindowCapture `
                -Window $window `
                -HelperPath $captureHelper `
                -Path $visibleSealCapturePath
        } finally {
            if (
                Test-Path `
                    -LiteralPath $captureHelperDirectory `
                    -PathType Container
            ) {
                [System.IO.Directory]::Delete(
                    $captureHelperDirectory,
                    $true
                )
            }
        }
        if (
            -not (Test-Path `
                -LiteralPath $visibleSealCapturePath `
                -PathType Leaf) -or
            (Get-Item -LiteralPath $visibleSealCapturePath).Length -le 0
        ) {
            throw "No se conservó la captura del sello visible configurado."
        }
        $result.visibleSealCapturePath =
            [System.IO.Path]::GetFileName($visibleSealCapturePath)
        $result.visibleSealCaptureSha256 = (
            Get-FileHash `
                -LiteralPath $visibleSealCapturePath `
                -Algorithm SHA256
        ).Hash
        $result.phase = "visible-seal-configured"
        Write-JsonAtomically -Path $resultFullPath -Value $result
    }

    $signButton = Wait-DescendantByName `
        -Root $window `
        -Name "Firmar documento" `
        -Deadline $deadline `
        -RequireEnabled
    $result.phase = "output-picker-opened"
    Write-JsonAtomically -Path $resultFullPath -Value $result
    Submit-FilePickerPath `
        -ApplicationWindow $window `
        -Trigger $signButton `
        -Path $signedPath `
        -Deadline $deadline `
        -ManualConfirmation:$ManualSaveConfirmation
    $result.phase = "signature-requested"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    if ($ManualSaveConfirmation) {
        $manualOutputDeadline = [datetime]::UtcNow.AddSeconds(10)
        while ([datetime]::UtcNow -lt $manualOutputDeadline) {
            if (
                Test-Path `
                    -LiteralPath $manualPickerSignedPath `
                    -PathType Leaf
            ) {
                $signedPath = $manualPickerSignedPath
                break
            }
            if (Test-Path -LiteralPath $signedPath -PathType Leaf) {
                break
            }
            Start-Sleep -Milliseconds 200
        }
    }

    [void](Wait-DescendantNameContains `
        -Root $window `
        -Text "Firma completada" `
        -Deadline $deadline)
    if (
        -not (Test-Path -LiteralPath $signedPath -PathType Leaf) -or
        (Get-Item -LiteralPath $signedPath).Length -le 0
    ) {
        throw "La interfaz anuncio exito, pero la firma no existe o esta vacia."
    }
    $result.signatureCreated = $true
    $result.signatureBytes = (Get-Item -LiteralPath $signedPath).Length
    $result.signatureSha256 = (
        Get-FileHash -LiteralPath $signedPath -Algorithm SHA256
    ).Hash
    $result.postValidationObserved = $true
    $result.phase = "signature-created-and-validated"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    $verifyNavigation = Find-DescendantByName `
        -Root $window `
        -Name "Verificar"
    if (
        $null -eq $verifyNavigation -or
        -not $verifyNavigation.Current.IsEnabled
    ) {
        $togglePaneCondition =
            [System.Windows.Automation.PropertyCondition]::new(
                [System.Windows.Automation.AutomationElement]::
                    AutomationIdProperty,
                "TogglePaneButton"
            )
        $togglePane = $window.FindFirst(
            [System.Windows.Automation.TreeScope]::Descendants,
            $togglePaneCondition
        )
        if ($null -eq $togglePane -or -not $togglePane.Current.IsEnabled)
        {
            throw "El menú compacto no expuso su botón de apertura."
        }
        Invoke-PickerTriggerWithNativeClick `
            -Element $togglePane `
            -OwnerWindowHandle $window.Current.NativeWindowHandle
        $verifyNavigation = Wait-DescendantByName `
            -Root $window `
            -Name "Verificar" `
            -Deadline $deadline `
            -RequireEnabled
    }
    $result.phase = "verification-navigation-ready"
    Write-JsonAtomically -Path $resultFullPath -Value $result
    Invoke-PickerTriggerWithNativeClick `
        -Element $verifyNavigation `
        -OwnerWindowHandle $window.Current.NativeWindowHandle

    $selectSigned = Wait-DescendantByName `
        -Root $window `
        -Name "Seleccionar fichero firmado" `
        -Deadline $deadline `
        -RequireEnabled
    Submit-FilePickerPath `
        -ApplicationWindow $window `
        -Trigger $selectSigned `
        -Path $signedPath `
        -Deadline $deadline
    $result.phase = "signed-file-selected-for-verification"
    Write-JsonAtomically -Path $resultFullPath -Value $result

    if (-not $VisibleSealPades) {
        $selectOriginal = Wait-DescendantByName `
            -Root $window `
            -Name "Seleccionar fichero original" `
            -Deadline $deadline `
            -RequireEnabled
        Submit-FilePickerPath `
            -ApplicationWindow $window `
            -Trigger $selectOriginal `
            -Path $inputPath `
            -Deadline $deadline
        $result.phase = "original-file-selected-for-verification"
        Write-JsonAtomically -Path $resultFullPath -Value $result
    }

    $verifyButton = Wait-DescendantByName `
        -Root $window `
        -Name "Verificar firma" `
        -Deadline $deadline `
        -RequireEnabled
    $result.phase = "independent-verification-ready"
    Write-JsonAtomically -Path $resultFullPath -Value $result
    Invoke-PickerTriggerWithNativeClick `
        -Element $verifyButton `
        -OwnerWindowHandle $window.Current.NativeWindowHandle
    [void](Wait-DescendantNameContains `
        -Root $window `
        -Text "Integridad: válida" `
        -Deadline $deadline)
    $result.independentVerificationObserved = $true

    Copy-Item `
        -LiteralPath $signedPath `
        -Destination $signatureEvidencePath `
        -ErrorAction Stop
    if (
        -not (Test-Path -LiteralPath $signatureEvidencePath -PathType Leaf) -or
        (Get-FileHash `
            -LiteralPath $signatureEvidencePath `
            -Algorithm SHA256).Hash -ne $result.signatureSha256
    ) {
        throw "No se pudo conservar una copia integra de la evidencia firmada."
    }
    $result.protectedEvidenceCopied = $true
    $result.status = "succeeded"
    $result.phase = "completed"
} catch {
    $result.status = "failed"
    $result.failure = $_.Exception.Message
    throw
} finally {
    $result.completedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    try {
        if (Test-Path -LiteralPath $pickerRunDirectory -PathType Container) {
            [System.IO.Directory]::Delete($pickerRunDirectory, $true)
        }
        if (
            $ManualSaveConfirmation -and
            $result.protectedEvidenceCopied -and
            (Test-Path -LiteralPath $manualPickerSignedPath -PathType Leaf)
        ) {
            [System.IO.File]::Delete($manualPickerSignedPath)
        }
        $result.temporaryOutputRemoved =
            -not (Test-Path -LiteralPath $pickerRunDirectory) -and
            (
                -not $ManualSaveConfirmation -or
                -not (Test-Path -LiteralPath $manualPickerSignedPath)
            )
    } catch {
        if ($result.status -eq "succeeded") {
            $result.status = "failed"
            $result.phase = "temporary-output-cleanup"
            $result.failure =
                "La firma se valido, pero no se retiro la salida temporal de QA."
        }
    }
    Write-JsonAtomically -Path $resultFullPath -Value $result
}
