# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [string]$RepositoryRoot = (Join-Path $PSScriptRoot "..\.."),
    [string]$OutputRoot,
    [ValidateSet("amd64")]
    [string]$Architecture = "amd64",
    [ValidateSet("Qt", "WinUI")]
    [string]$Frontend = "Qt",
    [ValidateSet("Auto", "PrintWindow", "WindowsGraphicsCapture")]
    [string]$CaptureMethod = "Auto",
    [ValidateRange(5, 3600)]
    [int]$DurationSeconds = 30,
    [ValidateRange(250, 60000)]
    [int]$IntervalMilliseconds = 1000,
    [ValidateRange(1, 2000)]
    [int]$MaxCaptures = 250,
    [ValidateRange(10485760, 2147483648)]
    [long]$MaxArtifactBytes = 536870912,
    [ValidateRange(1, 100)]
    [int]$MaxRetainedRuns = 20,
    [string]$ExecutablePath,
    [switch]$SkipBuild,
    [switch]$BuildPerformedBeforeLaunch,
    [switch]$CloseAfterCapture,
    [Parameter(Mandatory)]
    [switch]$TestDataOnly,
    [string]$ResultPointerPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not $IsWindows) {
    throw "La captura de ventanas requiere Windows 10 o posterior."
}
if ([System.Environment]::OSVersion.Version.Major -lt 10) {
    throw "La captura no es compatible con versiones anteriores a Windows 10."
}
if ($PSVersionTable.PSVersion.Major -lt 7) {
    throw "La captura requiere PowerShell 7 o posterior."
}
if (-not $TestDataOnly) {
    throw "Use -TestDataOnly y abra unicamente datos, certificados y documentos sinteticos."
}

function Get-NormalizedFullPath {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [string]$BasePath
    )

    if ([System.IO.Path]::IsPathFullyQualified($Path)) {
        return [System.IO.Path]::GetFullPath($Path)
    }
    if ([string]::IsNullOrWhiteSpace($BasePath)) {
        return [System.IO.Path]::GetFullPath($Path)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $BasePath $Path))
}

function Test-PathIsWithin {
    param(
        [Parameter(Mandatory)]
        [string]$Candidate,
        [Parameter(Mandatory)]
        [string]$Parent
    )

    $candidatePath = [System.IO.Path]::GetFullPath($Candidate)
    $parentPath = [System.IO.Path]::GetFullPath($Parent).TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )
    if (
        $candidatePath.Equals(
            $parentPath,
            [System.StringComparison]::OrdinalIgnoreCase
        )
    ) {
        return $true
    }
    $prefix = $parentPath + [System.IO.Path]::DirectorySeparatorChar
    return $candidatePath.StartsWith(
        $prefix,
        [System.StringComparison]::OrdinalIgnoreCase
    )
}

function Assert-LocalPathWithoutReparsePoint {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string]$Label
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if (
        $fullPath.StartsWith(
            "\\",
            [System.StringComparison]::Ordinal
        )
    ) {
        throw "$Label debe residir en un volumen local; no se permiten rutas UNC o de dispositivo."
    }
    $pathRoot = [System.IO.Path]::GetPathRoot($fullPath)
    if ([string]::IsNullOrWhiteSpace($pathRoot)) {
        throw "No se pudo determinar el volumen de $Label."
    }
    try {
        $drive = [System.IO.DriveInfo]::new($pathRoot)
    } catch {
        throw "No se pudo verificar el tipo de volumen de ${Label}: $($_.Exception.Message)"
    }
    if ($drive.DriveType -eq [System.IO.DriveType]::Network) {
        throw "$Label no puede residir en una unidad de red mapeada."
    }

    $current = $fullPath.TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )
    while (-not [string]::IsNullOrWhiteSpace($current)) {
        if (Test-Path -LiteralPath $current) {
            $item = Get-Item -LiteralPath $current -Force
            if (
                ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
            ) {
                throw "$Label no puede atravesar puntos de reanalisis: $current"
            }
        }
        $parent = [System.IO.Path]::GetDirectoryName($current)
        if (
            [string]::IsNullOrWhiteSpace($parent) -or
            $parent.Equals($current, [System.StringComparison]::OrdinalIgnoreCase)
        ) {
            break
        }
        $current = $parent.TrimEnd(
            [System.IO.Path]::DirectorySeparatorChar,
            [System.IO.Path]::AltDirectorySeparatorChar
        )
    }
}

function Protect-ArtifactDirectory {
    param(
        [Parameter(Mandatory)]
        [string]$Path
    )

    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    $systemUser = [System.Security.Principal.SecurityIdentifier]::new("S-1-5-18")
    $inheritance = [System.Security.AccessControl.InheritanceFlags]::ContainerInherit -bor
        [System.Security.AccessControl.InheritanceFlags]::ObjectInherit
    $propagation = [System.Security.AccessControl.PropagationFlags]::None
    $rights = [System.Security.AccessControl.FileSystemRights]::FullControl
    $allow = [System.Security.AccessControl.AccessControlType]::Allow

    $security = [System.Security.AccessControl.DirectorySecurity]::new()
    $security.SetOwner($currentUser)
    $security.SetAccessRuleProtection($true, $false)
    [void]$security.AddAccessRule(
        [System.Security.AccessControl.FileSystemAccessRule]::new(
            $currentUser,
            $rights,
            $inheritance,
            $propagation,
            $allow
        )
    )
    [void]$security.AddAccessRule(
        [System.Security.AccessControl.FileSystemAccessRule]::new(
            $systemUser,
            $rights,
            $inheritance,
            $propagation,
            $allow
        )
    )
    Set-Acl -LiteralPath $Path -AclObject $security

    $verified = Get-Acl -LiteralPath $Path
    if (-not $verified.AreAccessRulesProtected) {
        throw "La DACL de artefactos conserva herencia no controlada: $Path"
    }
    $owner = $verified.GetOwner(
        [System.Security.Principal.SecurityIdentifier]
    )
    if ($owner.Value -ne $currentUser.Value) {
        throw "El directorio de artefactos no pertenece al usuario actual: $Path"
    }
    $allowedSids = @($currentUser.Value, $systemUser.Value)
    $rules = $verified.GetAccessRules(
        $true,
        $true,
        [System.Security.Principal.SecurityIdentifier]
    )
    $verifiedSids = [System.Collections.Generic.HashSet[string]]::new()
    foreach ($rule in $rules) {
        if (
            $allowedSids -notcontains $rule.IdentityReference.Value -or
            $rule.AccessControlType -ne $allow -or
            ($rule.FileSystemRights -band $rights) -ne $rights
        ) {
            throw "La DACL de artefactos contiene una identidad o regla no permitida: $Path"
        }
        [void]$verifiedSids.Add($rule.IdentityReference.Value)
    }
    foreach ($allowedSid in $allowedSids) {
        if (-not $verifiedSids.Contains($allowedSid)) {
            throw "La DACL de artefactos no concede acceso a una identidad obligatoria: $Path"
        }
    }
}

function Test-CaptureRunDeletionCandidate {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string]$OutputRoot
    )

    try {
        $directory = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
        if (
            -not $directory.PSIsContainer -or
            $directory.Name -notmatch "^\d{8}T\d{9}Z-[a-f0-9]{8}$" -or
            -not (Test-PathIsWithin -Candidate $directory.FullName -Parent $OutputRoot) -or
            ($directory.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
        ) {
            return $false
        }

        $descendants = @(
            Get-ChildItem -LiteralPath $directory.FullName -Recurse -Force `
                -ErrorAction Stop
        )
        if (
            $null -ne (
                $descendants |
                    Where-Object {
                        ($_.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
                    } |
                    Select-Object -First 1
            )
        ) {
            return $false
        }

        $manifestPath = Join-Path $directory.FullName "manifest.json"
        $manifestItem = Get-Item -LiteralPath $manifestPath -Force -ErrorAction Stop
        if (
            $manifestItem.PSIsContainer -or
            ($manifestItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
        ) {
            return $false
        }
        $manifest = Get-Content -LiteralPath $manifestPath -Raw -ErrorAction Stop |
            ConvertFrom-Json -ErrorAction Stop
        if (
            $null -eq $manifest.PSObject.Properties["schemaVersion"] -or
            $manifest.schemaVersion -ne 1 -or
            $null -eq $manifest.PSObject.Properties["tool"] -or
            $manifest.tool -ne "GrxFirma window-only QA capture" -or
            $null -eq $manifest.PSObject.Properties["runId"] -or
            $manifest.runId -ne $directory.Name
        ) {
            return $false
        }

        return $true
    } catch {
        # Fail-closed: si no se puede enumerar, leer o validar el candidato,
        # no se considera apto para borrado.
        return $false
    }
}

function Remove-ExpiredCaptureRuns {
    param(
        [Parameter(Mandatory)]
        [string]$OutputRoot,
        [Parameter(Mandatory)]
        [int]$RetainedRuns
    )

    if (-not (Test-Path -LiteralPath $OutputRoot -PathType Container)) {
        return 0
    }
    $eligible = @()
    foreach (
        $directory in Get-ChildItem -LiteralPath $OutputRoot -Directory -Force `
            -ErrorAction Stop
    ) {
        if (
            Test-CaptureRunDeletionCandidate `
                -Path $directory.FullName `
                -OutputRoot $OutputRoot
        ) {
            $eligible += $directory
        }
    }

    $keepExisting = [Math]::Max(0, $RetainedRuns - 1)
    $expired = @(
        $eligible |
            Sort-Object Name -Descending |
            Select-Object -Skip $keepExisting
    )
    $removed = 0
    foreach ($directory in $expired) {
        # Se revalida inmediatamente antes de borrar para no confiar en el
        # DirectoryInfo obtenido durante la primera enumeracion.
        if (
            -not (
                Test-CaptureRunDeletionCandidate `
                    -Path $directory.FullName `
                    -OutputRoot $OutputRoot
            )
        ) {
            continue
        }
        Remove-Item -LiteralPath $directory.FullName -Recurse -Force `
            -ErrorAction Stop
        $removed++
    }
    return $removed
}

function Get-SanitizedFailureMessage {
    param(
        [Parameter(Mandatory)]
        [string]$Message
    )

    $sanitized = $Message.Replace($script:ResolvedRepositoryRoot, "<repository>")
    if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        $sanitized = $sanitized.Replace($env:LOCALAPPDATA, "<localappdata>")
    }
    if (-not [string]::IsNullOrWhiteSpace($env:USERPROFILE)) {
        $sanitized = $sanitized.Replace($env:USERPROFILE, "<userprofile>")
    }
    if (-not [string]::IsNullOrWhiteSpace($script:ResolvedOutputRoot)) {
        $sanitized = $sanitized.Replace($script:ResolvedOutputRoot, "<output>")
    }
    $sanitized = ($sanitized -replace "[\r\n]+", " ").Trim()
    if ($sanitized.Length -gt 500) {
        return $sanitized.Substring(0, 500)
    }
    return $sanitized
}

function Write-JsonAtomically {
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [object]$Value,
        [ValidateRange(2, 20)]
        [int]$Depth = 8
    )

    $parent = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
    }
    $temporary = Join-Path $parent (
        ".{0}.{1}.tmp" -f (Split-Path -Leaf $Path), [guid]::NewGuid().ToString("N")
    )
    try {
        $Value |
            ConvertTo-Json -Depth $Depth |
            Set-Content -LiteralPath $temporary -Encoding utf8
        [System.IO.File]::Move($temporary, $Path, $true)
    } finally {
        if (Test-Path -LiteralPath $temporary -PathType Leaf) {
            Remove-Item -LiteralPath $temporary -Force
        }
    }
}

function Write-ResultPointer {
    param(
        [Parameter(Mandatory)]
        [ValidateSet("running", "succeeded", "failed")]
        [string]$Status,
        [string]$Failure
    )

    if ([string]::IsNullOrWhiteSpace($ResultPointerPath)) {
        return
    }
    $pointer = [ordered]@{
        schemaVersion = 1
        tool = "GrxFirma window-only QA task pointer"
        pointerId = [System.IO.Path]::GetFileNameWithoutExtension($ResultPointerPath)
        status = $Status
        updatedAtUtc = [DateTimeOffset]::UtcNow.ToString("o")
        runDirectory = $script:RunDirectory
        manifestPath = if ($script:RunDirectory) {
            Join-Path $script:RunDirectory "manifest.json"
        } else {
            $null
        }
        failure = $Failure
    }
    Write-JsonAtomically -Path $ResultPointerPath -Value $pointer -Depth 4
}

function Initialize-NativeWindowCapture {
    if (-not ("GrxFirma.QA.WindowCaptureNative" -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

namespace GrxFirma.QA
{
    public sealed class TopLevelWindow
    {
        public long Handle { get; set; }
        public int ProcessId { get; set; }
        public string Title { get; set; }
        public string ClassName { get; set; }
        public bool Visible { get; set; }
        public bool Minimized { get; set; }
        public int Left { get; set; }
        public int Top { get; set; }
        public int Width { get; set; }
        public int Height { get; set; }
    }

    public static class WindowCaptureNative
    {
        private delegate bool EnumWindowsProc(IntPtr window, IntPtr parameter);

        [StructLayout(LayoutKind.Sequential)]
        private struct RECT
        {
            public int Left;
            public int Top;
            public int Right;
            public int Bottom;
        }

        [DllImport("user32.dll")]
        private static extern bool EnumWindows(EnumWindowsProc callback, IntPtr parameter);

        [DllImport("user32.dll")]
        private static extern bool IsWindow(IntPtr window);

        [DllImport("user32.dll")]
        private static extern bool IsWindowVisible(IntPtr window);

        [DllImport("user32.dll")]
        private static extern bool IsIconic(IntPtr window);

        [DllImport("user32.dll")]
        private static extern int GetWindowTextLength(IntPtr window);

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetWindowText(
            IntPtr window,
            StringBuilder text,
            int maximumLength
        );

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetClassName(
            IntPtr window,
            StringBuilder className,
            int maximumLength
        );

        [DllImport("user32.dll")]
        private static extern uint GetWindowThreadProcessId(
            IntPtr window,
            out uint processId
        );

        [DllImport("user32.dll")]
        private static extern bool GetWindowRect(IntPtr window, out RECT rectangle);

        [DllImport("user32.dll")]
        private static extern bool ShowWindowAsync(IntPtr window, int command);

        [DllImport("user32.dll")]
        private static extern bool BringWindowToTop(IntPtr window);

        [DllImport("user32.dll")]
        private static extern bool SetForegroundWindow(IntPtr window);

        [DllImport("user32.dll", SetLastError = true)]
        public static extern bool PrintWindow(
            IntPtr window,
            IntPtr destination,
            uint flags
        );

        public static TopLevelWindow[] Enumerate()
        {
            var result = new List<TopLevelWindow>();
            EnumWindows(delegate (IntPtr window, IntPtr parameter)
            {
                uint processId;
                GetWindowThreadProcessId(window, out processId);
                RECT rectangle;
                if (!GetWindowRect(window, out rectangle))
                {
                    return true;
                }
                int length = GetWindowTextLength(window);
                var title = new StringBuilder(Math.Max(length + 1, 1));
                GetWindowText(window, title, title.Capacity);
                var className = new StringBuilder(256);
                GetClassName(window, className, className.Capacity);
                result.Add(new TopLevelWindow
                {
                    Handle = window.ToInt64(),
                    ProcessId = unchecked((int)processId),
                    Title = title.ToString(),
                    ClassName = className.ToString(),
                    Visible = IsWindowVisible(window),
                    Minimized = IsIconic(window),
                    Left = rectangle.Left,
                    Top = rectangle.Top,
                    Width = Math.Max(0, rectangle.Right - rectangle.Left),
                    Height = Math.Max(0, rectangle.Bottom - rectangle.Top)
                });
                return true;
            }, IntPtr.Zero);
            return result.ToArray();
        }

        public static bool PrepareVerifiedWindow(
            long handle,
            int expectedProcessId,
            int expectedWidth,
            int expectedHeight)
        {
            var window = new IntPtr(handle);
            if (!IsVerifiedWindow(
                    handle,
                    expectedProcessId,
                    expectedWidth,
                    expectedHeight))
            {
                return false;
            }

            if (IsIconic(window))
            {
                ShowWindowAsync(window, 9);
            }
            if (!IsWindowVisible(window))
            {
                ShowWindowAsync(window, 5);
            }
            BringWindowToTop(window);
            SetForegroundWindow(window);

            return IsVerifiedWindow(
                handle,
                expectedProcessId,
                expectedWidth,
                expectedHeight) &&
                IsWindowVisible(window) &&
                !IsIconic(window);
        }

        public static bool IsVerifiedWindow(
            long handle,
            int expectedProcessId,
            int expectedWidth,
            int expectedHeight)
        {
            var window = new IntPtr(handle);
            uint processId;
            RECT rectangle;
            return IsWindow(window) &&
                GetWindowThreadProcessId(window, out processId) != 0 &&
                processId == unchecked((uint)expectedProcessId) &&
                GetWindowRect(window, out rectangle) &&
                rectangle.Right - rectangle.Left == expectedWidth &&
                rectangle.Bottom - rectangle.Top == expectedHeight;
        }
    }
}
'@
    }

    Add-Type -AssemblyName System.Drawing
    Add-Type -AssemblyName UIAutomationClient
    Add-Type -AssemblyName UIAutomationTypes
}

function Test-WindowIsSafeToCapture {
    param(
        [Parameter(Mandatory)]
        [long]$Handle,
        [AllowEmptyString()]
        [string]$Title,
        [AllowEmptyString()]
        [string]$ClassName
    )

    if ($ClassName -ieq "#32770") {
        return [pscustomobject]@{
            Safe = $false
            Reason = "common-dialog-class"
        }
    }

    $sensitiveTitlePattern = (
        "(?i)(contrase(?:ñ|n)a|contrasenya|password|passphrase|" +
        "mot\s+de\s+passe|senha|palavra-passe|passwort|kennwort|" +
        "pasahitz|parola\s+d['’]ordine|clave|chave|pin|token|" +
        "secreto|secret|segredo|geheim|credencial|credential|" +
        "private\s+key|clave\s+privada|chiave\s+privata|" +
        "pkcs#?12|\bp12\b|\bpfx\b|密码|密碼|口令|私钥|私鑰)"
    )
    if ($Title -match $sensitiveTitlePattern) {
        return [pscustomobject]@{
            Safe = $false
            Reason = "sensitive-title"
        }
    }

    try {
        $element = [System.Windows.Automation.AutomationElement]::FromHandle(
            [IntPtr]$Handle
        )
        if ($null -eq $element) {
            return [pscustomobject]@{
                Safe = $false
                Reason = "uia-root-unavailable"
            }
        }
        $passwordCondition = [System.Windows.Automation.PropertyCondition]::new(
            [System.Windows.Automation.AutomationElement]::IsPasswordProperty,
            $true
        )
        $passwordElement = $element.FindFirst(
            [System.Windows.Automation.TreeScope]::Descendants,
            $passwordCondition
        )
        if ($null -ne $passwordElement) {
            return [pscustomobject]@{
                Safe = $false
                Reason = "password-control"
            }
        }
    } catch {
        # La comprobacion es fail-closed: una ventana que no pueda inspeccionarse
        # no se convierte en una captura potencialmente sensible.
        return [pscustomobject]@{
            Safe = $false
            Reason = "uia-inspection-failed"
        }
    }

    return [pscustomobject]@{
        Safe = $true
        Reason = $null
    }
}

function Wait-WinUIWindowReady {
    param(
        [Parameter(Mandatory)]
        [long]$Handle,
        [Parameter(Mandatory)]
        [int]$ProcessId,
        [Parameter(Mandatory)]
        [int]$Width,
        [Parameter(Mandatory)]
        [int]$Height,
        [ValidateRange(1000, 15000)]
        [int]$TimeoutMilliseconds = 8000
    )

    $deadline = [DateTimeOffset]::UtcNow.AddMilliseconds(
        $TimeoutMilliseconds
    )
    $lastSignature = $null
    $stableSamples = 0
    $lastControlCount = 0
    $lastActionableCount = 0
    $observedRoot = $false
    $observedControls = $false
    $actionableTypes = @(
        [System.Windows.Automation.ControlType]::Button,
        [System.Windows.Automation.ControlType]::CheckBox,
        [System.Windows.Automation.ControlType]::ComboBox,
        [System.Windows.Automation.ControlType]::DataGrid,
        [System.Windows.Automation.ControlType]::Document,
        [System.Windows.Automation.ControlType]::Edit,
        [System.Windows.Automation.ControlType]::Hyperlink,
        [System.Windows.Automation.ControlType]::List,
        [System.Windows.Automation.ControlType]::ListItem,
        [System.Windows.Automation.ControlType]::RadioButton,
        [System.Windows.Automation.ControlType]::Tab,
        [System.Windows.Automation.ControlType]::Tree
    )

    do {
        if (
            -not (
                [GrxFirma.QA.WindowCaptureNative]::PrepareVerifiedWindow(
                    $Handle,
                    $ProcessId,
                    $Width,
                    $Height
                )
            )
        ) {
            return [pscustomobject]@{
                Ready = $false
                Reason = "winui-window-changed-before-ready"
                ControlCount = $lastControlCount
                ActionableControlCount = $lastActionableCount
            }
        }

        try {
            $root = [System.Windows.Automation.AutomationElement]::FromHandle(
                [IntPtr]$Handle
            )
            if ($null -ne $root) {
                $observedRoot = $true
                $elements = $root.FindAll(
                    [System.Windows.Automation.TreeScope]::Descendants,
                    [System.Windows.Automation.Condition]::TrueCondition
                )
                $controlCount = 0
                $actionableCount = 0
                $typeCounts = [System.Collections.Generic.Dictionary[int, int]]::new()
                foreach ($element in $elements) {
                    try {
                        $controlType = $element.Current.ControlType
                        if ($null -eq $controlType) {
                            continue
                        }
                        $controlCount++
                        if ($actionableTypes -contains $controlType) {
                            $actionableCount++
                        }
                        $typeId = $controlType.Id
                        if ($typeCounts.ContainsKey($typeId)) {
                            $typeCounts[$typeId]++
                        } else {
                            $typeCounts[$typeId] = 1
                        }
                    } catch {
                        # Un elemento que desaparece durante la enumeracion no
                        # convierte en estable el arbol parcial.
                        $controlCount = 0
                        $actionableCount = 0
                        break
                    }
                }
                $lastControlCount = $controlCount
                $lastActionableCount = $actionableCount
                if ($controlCount -ge 4 -and $actionableCount -ge 2) {
                    $observedControls = $true
                    $typeSignature = @(
                        $typeCounts.GetEnumerator() |
                            Sort-Object Key |
                            ForEach-Object {
                                "{0}={1}" -f $_.Key, $_.Value
                            }
                    ) -join ","
                    $signature = "{0}:{1}:{2}" -f (
                        $controlCount
                    ), $actionableCount, $typeSignature
                    if ($signature -eq $lastSignature) {
                        $stableSamples++
                    } else {
                        $lastSignature = $signature
                        $stableSamples = 1
                    }
                    if ($stableSamples -ge 3) {
                        return [pscustomobject]@{
                            Ready = $true
                            Reason = $null
                            ControlCount = $controlCount
                            ActionableControlCount = $actionableCount
                        }
                    }
                } else {
                    $lastSignature = $null
                    $stableSamples = 0
                }
            }
        } catch {
            $lastSignature = $null
            $stableSamples = 0
        }
        Start-Sleep -Milliseconds 250
    } while ([DateTimeOffset]::UtcNow -lt $deadline)

    $reason = if (-not $observedRoot) {
        "winui-uia-root-not-ready"
    } elseif (-not $observedControls) {
        "winui-control-tree-not-ready"
    } else {
        "winui-control-tree-not-stable"
    }
    return [pscustomobject]@{
        Ready = $false
        Reason = $reason
        ControlCount = $lastControlCount
        ActionableControlCount = $lastActionableCount
    }
}

function Save-PrintWindowImage {
    param(
        [Parameter(Mandatory)]
        [long]$Handle,
        [Parameter(Mandatory)]
        [int]$Width,
        [Parameter(Mandatory)]
        [int]$Height,
        [Parameter(Mandatory)]
        [object]$ProcessIdentity,
        [Parameter(Mandatory)]
        [System.Collections.Generic.Dictionary[string, object]]$ExecutableCatalog,
        [Parameter(Mandatory)]
        [string]$Path
    )

    $script:LastPrintWindowFailure = $null
    $temporaryPath = Join-Path (Split-Path -Parent $Path) (
        ".{0}.{1}.tmp" -f (Split-Path -Leaf $Path), [guid]::NewGuid().ToString("N")
    )
    if (
        -not (
            Test-VerifiedProcessIdentity `
                -ExpectedIdentity $ProcessIdentity `
                -ExecutableCatalog $ExecutableCatalog
        ) -or
        -not (
            [GrxFirma.QA.WindowCaptureNative]::IsVerifiedWindow(
                $Handle,
                $ProcessIdentity.ProcessId,
                $Width,
                $Height
            )
        )
    ) {
        $script:LastPrintWindowFailure = "print-window-target-changed"
        return $false
    }
    $bitmap = [System.Drawing.Bitmap]::new(
        $Width,
        $Height,
        [System.Drawing.Imaging.PixelFormat]::Format32bppArgb
    )
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    try {
        $graphics.Clear([System.Drawing.Color]::Black)
        $deviceContext = $graphics.GetHdc()
        try {
            # PW_RENDERFULLCONTENT (2) y el fallback 0 siguen siendo capturas
            # de un HWND concreto. No se obtiene nunca un DC del escritorio.
            $captured = [GrxFirma.QA.WindowCaptureNative]::PrintWindow(
                [IntPtr]$Handle,
                $deviceContext,
                2
            )
            if (-not $captured) {
                $captured = [GrxFirma.QA.WindowCaptureNative]::PrintWindow(
                    [IntPtr]$Handle,
                    $deviceContext,
                    0
                )
            }
        } finally {
            $graphics.ReleaseHdc($deviceContext)
        }
        if (-not $captured) {
            $script:LastPrintWindowFailure = "print-window-failed"
            return $false
        }
        if (-not (Test-BitmapHasUsefulClientContent -Bitmap $bitmap)) {
            $script:LastPrintWindowFailure = (
                "print-window-content-not-ready"
            )
            return $false
        }
        $bitmap.Save(
            $temporaryPath,
            [System.Drawing.Imaging.ImageFormat]::Png
        )
        if (
            -not (
                Test-VerifiedProcessIdentity `
                    -ExpectedIdentity $ProcessIdentity `
                    -ExecutableCatalog $ExecutableCatalog
            ) -or
            -not (
                [GrxFirma.QA.WindowCaptureNative]::IsVerifiedWindow(
                    $Handle,
                    $ProcessIdentity.ProcessId,
                    $Width,
                    $Height
                )
            )
        ) {
            $script:LastPrintWindowFailure = "print-window-target-changed"
            return $false
        }
        [System.IO.File]::Move($temporaryPath, $Path, $false)
        return $true
    } finally {
        $graphics.Dispose()
        $bitmap.Dispose()
        if (Test-Path -LiteralPath $temporaryPath -PathType Leaf) {
            Remove-Item -LiteralPath $temporaryPath -Force
        }
    }
}

function Test-BitmapHasUsefulClientContent {
    param(
        [Parameter(Mandatory)]
        [System.Drawing.Bitmap]$Bitmap
    )

    $width = $Bitmap.Width
    $height = $Bitmap.Height
    $horizontalMargin = [Math]::Max(16, [int]($width / 50))
    $topMargin = [Math]::Max(48, [int]($height / 10))
    $bottomMargin = [Math]::Max(16, [int]($height / 50))
    if (
        $width -le ($horizontalMargin * 2) -or
        $height -le ($topMargin + $bottomMargin)
    ) {
        return $false
    }

    $colorCounts = [int[]]::new(4096)
    $sampleCount = 0
    $dominantCount = 0
    $sampleStep = [Math]::Max(
        1,
        [int]([Math]::Min($width, $height) / 128)
    )
    for (
        $y = $topMargin;
        $y -lt ($height - $bottomMargin);
        $y += $sampleStep
    ) {
        for (
            $x = $horizontalMargin;
            $x -lt ($width - $horizontalMargin);
            $x += $sampleStep
        ) {
            $color = $Bitmap.GetPixel($x, $y)
            if ($color.A -lt 64) {
                continue
            }
            $key = (
                (($color.R -shr 4) -shl 8) -bor
                (($color.G -shr 4) -shl 4) -bor
                ($color.B -shr 4)
            )
            $colorCounts[$key]++
            $dominantCount = [Math]::Max(
                $dominantCount,
                $colorCounts[$key]
            )
            $sampleCount++
        }
    }
    if ($sampleCount -lt 100) {
        return $false
    }
    $minimumNonDominantSamples = [Math]::Max(
        24,
        [int](($sampleCount + 199) / 200)
    )
    return ($sampleCount - $dominantCount) -ge (
        $minimumNonDominantSamples
    )
}

function Build-GraphicsCaptureHelper {
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory)]
        [string]$WorkingDirectory
    )

    if (
        [System.Environment]::OSVersion.Version.Build -lt 18362
    ) {
        throw (
            "WindowsGraphicsCapture por HWND requiere Windows 10 1903 " +
            "(build 18362) o posterior."
        )
    }
    $dotnet = Get-Command dotnet.exe -ErrorAction Stop
    $project = Join-Path `
        $RepositoryRoot `
        "scripts\windows-qa\GrxFirma.WindowCapture\GrxFirma.WindowCapture.csproj"
    if (-not (Test-Path -LiteralPath $project -PathType Leaf)) {
        throw "No se encontro el helper de captura por HWND."
    }
    Assert-LocalPathWithoutReparsePoint `
        -Path $project `
        -Label "GraphicsCaptureHelper"

    $obj = Join-Path $WorkingDirectory "obj"
    $bin = Join-Path $WorkingDirectory "bin"
    $publish = Join-Path $WorkingDirectory "publish"
    New-Item -ItemType Directory -Path $WorkingDirectory -Force | Out-Null
    Protect-ArtifactDirectory -Path $WorkingDirectory
    $commonProperties = @(
        "-p:BaseIntermediateOutputPath=$obj\",
        "-p:BaseOutputPath=$bin\",
        "-p:UseSharedCompilation=false"
    )
    & $dotnet.Source restore `
        $project `
        --locked-mode `
        --runtime win-x64 `
        @commonProperties |
        Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo restaurar de forma bloqueada el helper WindowsGraphicsCapture."
    }
    & $dotnet.Source publish `
        $project `
        --configuration Release `
        --runtime win-x64 `
        --self-contained false `
        --no-restore `
        --output $publish `
        @commonProperties |
        Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw "No se pudo compilar el helper WindowsGraphicsCapture."
    }

    $helper = Join-Path $publish "GrxFirma.WindowCapture.exe"
    if (-not (Test-Path -LiteralPath $helper -PathType Leaf)) {
        throw "La compilacion no produjo el helper WindowsGraphicsCapture."
    }
    Assert-LocalPathWithoutReparsePoint `
        -Path $helper `
        -Label "GraphicsCaptureHelper"
    return $helper
}

function Save-GraphicsCaptureWindowImage {
    param(
        [Parameter(Mandatory)]
        [string]$HelperPath,
        [Parameter(Mandatory)]
        [long]$Handle,
        [Parameter(Mandatory)]
        [int]$ProcessId,
        [Parameter(Mandatory)]
        [int]$Width,
        [Parameter(Mandatory)]
        [int]$Height,
        [Parameter(Mandatory)]
        [object]$ProcessIdentity,
        [Parameter(Mandatory)]
        [string]$Path
    )

    $arguments = @(
        "--window-handle",
        ("0x{0}" -f $Handle.ToString("x")),
        "--expected-process-id",
        $ProcessId.ToString([System.Globalization.CultureInfo]::InvariantCulture),
        "--expected-process-start-utc-ticks",
        $ProcessIdentity.StartTimeUtcTicks.ToString(
            [System.Globalization.CultureInfo]::InvariantCulture
        ),
        "--expected-process-path",
        $ProcessIdentity.ExecutablePath,
        "--expected-process-sha256",
        $ProcessIdentity.ExecutableSha256,
        "--expected-width",
        $Width.ToString([System.Globalization.CultureInfo]::InvariantCulture),
        "--expected-height",
        $Height.ToString([System.Globalization.CultureInfo]::InvariantCulture),
        "--output",
        $Path,
        "--timeout-ms",
        "5000"
    )
    $helperOutput = @(& $HelperPath @arguments 2>&1)
    $helperExitCode = $LASTEXITCODE
    $captured = $helperExitCode -eq 0 -and
        (Test-Path -LiteralPath $Path -PathType Leaf)
    $allowedFailureReasons = @(
        "invalid-arguments",
        "invalid-window-handle",
        "invalid-process-id",
        "invalid-output-path",
        "invalid-timeout",
        "graphics-capture-unavailable",
        "graphics-capture-unsupported",
        "graphics-capture-timeout",
        "graphics-capture-frame-failed",
        "graphics-capture-item-failed",
        "graphics-capture-frame-pool-failed",
        "window-content-not-ready",
        "target-window-unavailable",
        "target-window-process-mismatch",
        "target-window-process-not-allowed",
        "target-window-process-unavailable",
        "target-window-process-start-mismatch",
        "target-window-process-path-mismatch",
        "target-window-process-hash-mismatch",
        "target-window-bounds-unavailable",
        "target-window-bounds-changed",
        "invalid-capture-dimensions",
        "invalid-frame-pixel-format",
        "invalid-frame-buffer",
        "invalid-client-content-region",
        "insufficient-client-samples",
        "invalid-output-directory",
        "invalid-png-size",
        "network-output-forbidden",
        "output-volume-unavailable",
        "reparse-output-forbidden",
        "unexpected-capture-failure"
    )
    $script:LastGraphicsCaptureFailure = $null
    if (-not $captured) {
        foreach ($line in $helperOutput) {
            $candidate = ([string]$line).Trim()
            if ($allowedFailureReasons -contains $candidate) {
                $script:LastGraphicsCaptureFailure = $candidate
            }
        }
    }
    if (-not $captured -and (Test-Path -LiteralPath $Path -PathType Leaf)) {
        Remove-Item -LiteralPath $Path -Force
    }
    return $captured
}

function New-AllowedExecutableCatalog {
    param(
        [Parameter(Mandatory)]
        [string]$ApplicationDirectory
    )

    $catalog = [System.Collections.Generic.Dictionary[string, object]]::new(
        [System.StringComparer]::OrdinalIgnoreCase
    )
    foreach ($name in @(
        "grxfirma-gui-qml.exe",
        "grxfirma-winui.exe",
        "grxfirma.exe",
        "grxfirma-gui.exe",
        "grxfirma-afirmauri.exe"
    )) {
        $candidate = Join-Path $ApplicationDirectory $name
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            continue
        }
        Assert-LocalPathWithoutReparsePoint `
            -Path $candidate `
            -Label "AllowedExecutable"
        $resolved = (Resolve-Path -LiteralPath $candidate -ErrorAction Stop).Path
        if (
            -not (
                (Split-Path -Parent $resolved).Equals(
                    $ApplicationDirectory,
                    [System.StringComparison]::OrdinalIgnoreCase
                )
            )
        ) {
            throw "Un ejecutable permitido debe residir directamente en la stage."
        }
        $catalog[$resolved] = [pscustomobject]@{
            ExecutablePath = $resolved
            ExecutableSha256 = (
                Get-FileHash -LiteralPath $resolved -Algorithm SHA256
            ).Hash.ToLowerInvariant()
            ProcessName = [System.IO.Path]::GetFileNameWithoutExtension($resolved)
        }
    }
    return ,$catalog
}

function Get-VerifiedProcessIdentity {
    param(
        [Parameter(Mandatory)]
        [int]$ProcessId,
        [Parameter(Mandatory)]
        [System.Collections.Generic.Dictionary[string, object]]$ExecutableCatalog,
        [object]$ExpectedIdentity
    )

    $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if ($null -eq $process) {
        return $null
    }
    try {
        $nativeProcess = Get-CimInstance `
            -ClassName Win32_Process `
            -Filter "ProcessId = $ProcessId" `
            -ErrorAction SilentlyContinue
        if (
            $null -eq $nativeProcess -or
            [string]::IsNullOrWhiteSpace([string]$nativeProcess.ExecutablePath)
        ) {
            return $null
        }
        $path = [System.IO.Path]::GetFullPath(
            [string]$nativeProcess.ExecutablePath
        )
        $allowed = $null
        if (-not $ExecutableCatalog.TryGetValue($path, [ref]$allowed)) {
            return $null
        }
        $startTicks = $process.StartTime.ToUniversalTime().Ticks
        $hash = (
            Get-FileHash -LiteralPath $path -Algorithm SHA256
        ).Hash.ToLowerInvariant()
        if (
            $hash -ne $allowed.ExecutableSha256 -or
            $process.ProcessName -ine $allowed.ProcessName
        ) {
            return $null
        }
        if (
            $null -ne $ExpectedIdentity -and (
                $ExpectedIdentity.ProcessId -ne $ProcessId -or
                $ExpectedIdentity.StartTimeUtcTicks -ne $startTicks -or
                $ExpectedIdentity.ExecutablePath -ine $path -or
                $ExpectedIdentity.ExecutableSha256 -ine $hash
            )
        ) {
            return $null
        }
        $process.Refresh()
        if ($process.HasExited) {
            return $null
        }
        return [pscustomobject]@{
            ProcessId = $ProcessId
            ParentProcessId = [int]$nativeProcess.ParentProcessId
            StartTimeUtcTicks = [long]$startTicks
            ExecutablePath = $path
            ExecutableSha256 = $hash
            ProcessName = $process.ProcessName
        }
    } catch {
        return $null
    } finally {
        $process.Dispose()
    }
}

function Test-VerifiedProcessIdentity {
    param(
        [Parameter(Mandatory)]
        [object]$ExpectedIdentity,
        [Parameter(Mandatory)]
        [System.Collections.Generic.Dictionary[string, object]]$ExecutableCatalog
    )

    return $null -ne (
        Get-VerifiedProcessIdentity `
            -ProcessId $ExpectedIdentity.ProcessId `
            -ExecutableCatalog $ExecutableCatalog `
            -ExpectedIdentity $ExpectedIdentity
    )
}

function Get-GrxFirmaProcessLineage {
    param(
        [Parameter(Mandatory)]
        [object]$RootIdentity,
        [Parameter(Mandatory)]
        [System.Collections.Generic.Dictionary[string, object]]$ExecutableCatalog
    )

    $allowedNames = @(
        "grxfirma-gui-qml.exe",
        "grxfirma-winui.exe",
        "grxfirma.exe",
        "grxfirma-gui.exe",
        "grxfirma-afirmauri.exe"
    )
    $identities = [System.Collections.Generic.Dictionary[int, object]]::new()
    $verifiedRoot = Get-VerifiedProcessIdentity `
        -ProcessId $RootIdentity.ProcessId `
        -ExecutableCatalog $ExecutableCatalog `
        -ExpectedIdentity $RootIdentity
    if ($null -eq $verifiedRoot) {
        return ,$identities
    }
    $identities[$verifiedRoot.ProcessId] = $verifiedRoot
    $processes = @(
        Get-CimInstance `
            -ClassName Win32_Process `
            -Property ProcessId, ParentProcessId, Name
    )
    $changed = $true
    while ($changed) {
        $changed = $false
        foreach ($process in $processes) {
            $candidateId = [int]$process.ProcessId
            if ($identities.ContainsKey($candidateId)) {
                continue
            }
            $parentId = [int]$process.ParentProcessId
            if (
                $identities.ContainsKey($parentId) -and
                ($allowedNames -contains [string]$process.Name)
            ) {
                $identity = Get-VerifiedProcessIdentity `
                    -ProcessId $candidateId `
                    -ExecutableCatalog $ExecutableCatalog
                if (
                    $null -eq $identity -or
                    $identity.ParentProcessId -ne $parentId -or
                    $identity.StartTimeUtcTicks -lt (
                        $identities[$parentId].StartTimeUtcTicks
                    )
                ) {
                    continue
                }
                $identities[$candidateId] = $identity
                $changed = $true
            }
        }
    }
    return ,$identities
}

function Stop-GrxFirmaProcessLineage {
    param(
        [Parameter(Mandatory)]
        [object]$RootIdentity,
        [Parameter(Mandatory)]
        [System.Collections.Generic.Dictionary[string, object]]$ExecutableCatalog
    )

    $lineage = Get-GrxFirmaProcessLineage `
        -RootIdentity $RootIdentity `
        -ExecutableCatalog $ExecutableCatalog
    $orderedProcessIds = @(
        @($lineage.Keys | Where-Object { $_ -ne $RootIdentity.ProcessId }) +
        @($RootIdentity.ProcessId)
    )
    foreach ($processId in $orderedProcessIds) {
        if (-not $lineage.ContainsKey($processId)) {
            continue
        }
        $expectedIdentity = $lineage[$processId]
        $process = Get-Process `
            -Id $processId `
            -ErrorAction SilentlyContinue
        if ($null -eq $process) {
            continue
        }
        try {
            $processPath = [System.IO.Path]::GetFullPath($process.Path)
            if (
                $process.StartTime.ToUniversalTime().Ticks -ne
                    $expectedIdentity.StartTimeUtcTicks -or
                $processPath -ine $expectedIdentity.ExecutablePath -or
                (
                    Get-FileHash -LiteralPath $processPath -Algorithm SHA256
                ).Hash -ine $expectedIdentity.ExecutableSha256
            ) {
                continue
            }
            if ($process.MainWindowHandle -ne [IntPtr]::Zero) {
                [void]$process.CloseMainWindow()
                if ($process.WaitForExit(1500)) {
                    continue
                }
            }
            $process.Refresh()
            if (-not $process.HasExited) {
                $process.Kill()
                [void]$process.WaitForExit(5000)
            }
        } finally {
            $process.Dispose()
        }
    }

    foreach ($processId in $lineage.Keys) {
        if (
            $null -ne (
                Get-Process `
                    -Id $processId `
                    -ErrorAction SilentlyContinue
            )
        ) {
            return $false
        }
    }
    return $true
}

$script:ResolvedRepositoryRoot = (
    Resolve-Path -LiteralPath $RepositoryRoot -ErrorAction Stop
).Path
$gitTopLevel = (& git -C $script:ResolvedRepositoryRoot rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($gitTopLevel)) {
    throw "RepositoryRoot no es un worktree Git valido."
}
$gitTopLevel = [System.IO.Path]::GetFullPath($gitTopLevel.Trim())
if (
    -not $gitTopLevel.Equals(
        [System.IO.Path]::GetFullPath($script:ResolvedRepositoryRoot),
        [System.StringComparison]::OrdinalIgnoreCase
    )
) {
    throw "RepositoryRoot debe apuntar a la raiz del worktree."
}

if ([string]::IsNullOrWhiteSpace($OutputRoot)) {
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        throw "LOCALAPPDATA no esta definido; indique -OutputRoot fuera del repositorio."
    }
    $OutputRoot = Join-Path $env:LOCALAPPDATA "GrxFirma\QA\window-captures"
}
$script:ResolvedOutputRoot = Get-NormalizedFullPath `
    -Path $OutputRoot `
    -BasePath (Get-Location).Path
if (Test-PathIsWithin -Candidate $script:ResolvedOutputRoot -Parent $script:ResolvedRepositoryRoot) {
    throw "OutputRoot debe estar fuera del worktree para evitar artefactos de QA en Git."
}
Assert-LocalPathWithoutReparsePoint `
    -Path $script:ResolvedOutputRoot `
    -Label "OutputRoot"
if (-not [string]::IsNullOrWhiteSpace($ResultPointerPath)) {
    $ResultPointerPath = Get-NormalizedFullPath `
        -Path $ResultPointerPath `
        -BasePath (Get-Location).Path
    if (Test-PathIsWithin -Candidate $ResultPointerPath -Parent $script:ResolvedRepositoryRoot) {
        throw "ResultPointerPath debe estar fuera del worktree."
    }
    Assert-LocalPathWithoutReparsePoint `
        -Path (Split-Path -Parent $ResultPointerPath) `
        -Label "ResultPointerPath"
}

if (-not (Test-Path -LiteralPath $script:ResolvedOutputRoot -PathType Container)) {
    New-Item -ItemType Directory -Path $script:ResolvedOutputRoot -Force | Out-Null
}
$retentionRemoved = Remove-ExpiredCaptureRuns `
    -OutputRoot $script:ResolvedOutputRoot `
    -RetainedRuns $MaxRetainedRuns
$runId = "{0}-{1}" -f (
    [DateTimeOffset]::UtcNow.ToString("yyyyMMddTHHmmssfffZ")
), ([guid]::NewGuid().ToString("N").Substring(0, 8))
$script:RunDirectory = Join-Path $script:ResolvedOutputRoot $runId
$imagesDirectory = Join-Path $script:RunDirectory "images"
New-Item -ItemType Directory -Path $script:RunDirectory | Out-Null
Protect-ArtifactDirectory -Path $script:RunDirectory
New-Item -ItemType Directory -Path $imagesDirectory | Out-Null
Protect-ArtifactDirectory -Path $imagesDirectory

$captureRecords = [System.Collections.Generic.List[object]]::new()
$lastWindowHashes = [System.Collections.Generic.Dictionary[string, string]]::new()
$startedAt = [DateTimeOffset]::UtcNow
$endedAt = $null
$runStatus = "failed"
$failureMessage = $null
$failureRecord = $null
$launchedProcess = $null
$buildPerformed = (-not $SkipBuild) -or $BuildPerformedBeforeLaunch
$successfulCaptures = 0
$deduplicatedCaptures = 0
$artifactBytes = [long]0
$captureLimitReached = $false
$artifactQuotaReached = $false
$manifestRecordLimitReached = $false
$maxManifestRecords = 5000
$applicationClosed = $false
$executableHash = $null
$frontendExecutableHash = $null
$commit = $null
$repositoryDirty = $null
$graphicsCaptureWorkingDirectory = $null
$graphicsCaptureHelper = $null
$applicationDirectory = $null
$executableCatalog = $null
$rootProcessIdentity = $null
$effectiveCaptureMethod = if ($CaptureMethod -eq "Auto") {
    if ($Frontend -eq "WinUI") {
        "WindowsGraphicsCapture"
    } else {
        "PrintWindow"
    }
} else {
    $CaptureMethod
}

Write-ResultPointer -Status "running"

try {
    Initialize-NativeWindowCapture
    if ($effectiveCaptureMethod -eq "WindowsGraphicsCapture") {
        $graphicsCaptureWorkingDirectory = Join-Path `
            $script:RunDirectory `
            ".graphics-capture-helper"
        $graphicsCaptureHelper = Build-GraphicsCaptureHelper `
            -RepositoryRoot $script:ResolvedRepositoryRoot `
            -WorkingDirectory $graphicsCaptureWorkingDirectory
    }

    $commitOutput = (& git -C $script:ResolvedRepositoryRoot rev-parse HEAD 2>$null)
    if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($commitOutput)) {
        $commit = $commitOutput.Trim()
    }
    $statusOutput = @(& git -C $script:ResolvedRepositoryRoot status --porcelain 2>$null)
    if ($LASTEXITCODE -eq 0) {
        $repositoryDirty = $statusOutput.Count -gt 0
    }

    if (-not $SkipBuild) {
        $buildScriptRelative = if ($Frontend -eq "WinUI") {
            "packaging\windows\build-desktop-winui.ps1"
        } else {
            "packaging\windows\build-desktop-qml.ps1"
        }
        $buildScript = Join-Path $script:ResolvedRepositoryRoot $buildScriptRelative
        if (-not (Test-Path -LiteralPath $buildScript -PathType Leaf)) {
            throw "No se encontro el script de build para el frontend $Frontend."
        }
        $previousArchitecture = $env:GOARCH
        try {
            $env:GOARCH = $Architecture
            & $buildScript
        } finally {
            $env:GOARCH = $previousArchitecture
        }
    }

    if ([string]::IsNullOrWhiteSpace($ExecutablePath)) {
        $relativeExecutable = if ($Frontend -eq "WinUI") {
            "release\windows-desktop-winui\" +
                "GrxFirma-$((Get-Content -LiteralPath (Join-Path $script:ResolvedRepositoryRoot "VERSION.txt") -Raw).Trim())-desktop-winui-windows-$Architecture\" +
                "app\grxfirma-gui.exe"
        } else {
            "release\windows-desktop-qml\" +
                "GrxFirma-$((Get-Content -LiteralPath (Join-Path $RepositoryRoot "VERSION.txt") -Raw).Trim())-desktop-qml-windows-$Architecture\" +
                "grxfirma-gui-qml.exe"
        }
        $ExecutablePath = Join-Path $script:ResolvedRepositoryRoot $relativeExecutable
    } else {
        $ExecutablePath = Get-NormalizedFullPath `
            -Path $ExecutablePath `
            -BasePath $script:ResolvedRepositoryRoot
    }
    $resolvedExecutable = (
        Resolve-Path -LiteralPath $ExecutablePath -ErrorAction Stop
    ).Path
    Assert-LocalPathWithoutReparsePoint `
        -Path $resolvedExecutable `
        -Label "ExecutablePath"
    $expectedExecutableName = if ($Frontend -eq "WinUI") {
        "grxfirma-gui.exe"
    } else {
        "grxfirma-gui-qml.exe"
    }
    if ((Split-Path -Leaf $resolvedExecutable) -ine $expectedExecutableName) {
        throw "El ejecutable no corresponde al frontend $Frontend."
    }
    $executableHash = (
        Get-FileHash -LiteralPath $resolvedExecutable -Algorithm SHA256
    ).Hash.ToLowerInvariant()
    $frontendExecutable = if ($Frontend -eq "WinUI") {
        Join-Path (Split-Path -Parent $resolvedExecutable) "grxfirma-winui.exe"
    } else {
        $resolvedExecutable
    }
    $frontendExecutable = (
        Resolve-Path -LiteralPath $frontendExecutable -ErrorAction Stop
    ).Path
    Assert-LocalPathWithoutReparsePoint `
        -Path $frontendExecutable `
        -Label "FrontendExecutable"
    $frontendExecutableHash = (
        Get-FileHash -LiteralPath $frontendExecutable -Algorithm SHA256
    ).Hash.ToLowerInvariant()
    $applicationDirectory = Split-Path -Parent $resolvedExecutable
    $executableCatalog = New-AllowedExecutableCatalog `
        -ApplicationDirectory $applicationDirectory
    foreach ($requiredExecutable in @(
        $resolvedExecutable,
        $frontendExecutable
    )) {
        if (-not $executableCatalog.ContainsKey($requiredExecutable)) {
            throw "La stage no contiene un ejecutable permitido y verificable."
        }
    }

    $previousQsgBackend = $env:QSG_RHI_BACKEND
    $previousQtQuickBackend = $env:QT_QUICK_BACKEND
    try {
        $launchParameters = @{
            FilePath = $resolvedExecutable
            WorkingDirectory = (Split-Path -Parent $resolvedExecutable)
            PassThru = $true
        }
        if ($Frontend -eq "Qt") {
            # El renderer software hace PrintWindow mas estable y evita
            # recurrir a una captura del escritorio como fallback.
            $env:QSG_RHI_BACKEND = "software"
            $env:QT_QUICK_BACKEND = "software"
        } else {
            # WinUI se inicia siempre a traves del launcher Go para que este
            # cree el pipe y entregue el PID del backend al cliente nativo.
            $launchParameters.ArgumentList = @("--frontend", "winui")
        }
        $launchedProcess = Start-Process @launchParameters
        $rootProcessIdentity = Get-VerifiedProcessIdentity `
            -ProcessId $launchedProcess.Id `
            -ExecutableCatalog $executableCatalog
        if ($null -eq $rootProcessIdentity) {
            throw "No se pudo fijar la identidad del proceso GrxFirma lanzado."
        }
    } finally {
        $env:QSG_RHI_BACKEND = $previousQsgBackend
        $env:QT_QUICK_BACKEND = $previousQtQuickBackend
    }

    $captureDeadline = [DateTimeOffset]::UtcNow.AddSeconds($DurationSeconds)
    $sequence = 0
    Start-Sleep -Milliseconds ([Math]::Min($IntervalMilliseconds, 1500))
    while (
        [DateTimeOffset]::UtcNow -lt $captureDeadline -and
        -not $captureLimitReached -and
        -not $artifactQuotaReached -and
        -not $manifestRecordLimitReached
    ) {
        if ($launchedProcess.HasExited) {
            break
        }
        $allowedProcessIdentities = Get-GrxFirmaProcessLineage `
            -RootIdentity $rootProcessIdentity `
            -ExecutableCatalog $executableCatalog
        $windows = @([GrxFirma.QA.WindowCaptureNative]::Enumerate())
        foreach ($window in $windows) {
            $canRestoreWinUI = $Frontend -eq "WinUI"
            if (
                -not $window.Visible -or
                ($window.Minimized -and -not $canRestoreWinUI) -or
                -not $allowedProcessIdentities.ContainsKey(
                    [int]$window.ProcessId
                )
            ) {
                continue
            }

            $processIdentity = Get-VerifiedProcessIdentity `
                -ProcessId ([int]$window.ProcessId) `
                -ExecutableCatalog $executableCatalog `
                -ExpectedIdentity $allowedProcessIdentities[
                    [int]$window.ProcessId
                ]
            if ($null -eq $processIdentity) {
                continue
            }
            if ($captureRecords.Count -ge $maxManifestRecords) {
                $manifestRecordLimitReached = $true
                break
            }

            $capturedAt = [DateTimeOffset]::UtcNow
            $baseRecord = [ordered]@{
                capturedAtUtc = $capturedAt.ToString("o")
                processId = [int]$window.ProcessId
                processName = $processIdentity.ProcessName
                windowHandle = ("0x{0}" -f $window.Handle.ToString("x"))
                width = [int]$window.Width
                height = [int]$window.Height
                captureMethod = $effectiveCaptureMethod
                status = $null
                reason = $null
                file = $null
                sha256 = $null
                uiaControlCount = $null
                uiaActionableControlCount = $null
            }

            if (
                $window.Width -lt 1 -or
                $window.Height -lt 1 -or
                $window.Width -gt 8192 -or
                $window.Height -gt 8192 -or
                ([long]$window.Width * [long]$window.Height) -gt 33177600
            ) {
                $baseRecord.status = "skipped"
                $baseRecord.reason = "invalid-window-dimensions"
                $captureRecords.Add([pscustomobject]$baseRecord)
                continue
            }

            $safety = Test-WindowIsSafeToCapture `
                -Handle $window.Handle `
                -Title ([string]$window.Title) `
                -ClassName ([string]$window.ClassName)
            if (-not $safety.Safe) {
                $baseRecord.status = "skipped"
                $baseRecord.reason = $safety.Reason
                $captureRecords.Add([pscustomobject]$baseRecord)
                continue
            }
            if ($Frontend -eq "WinUI") {
                $readiness = Wait-WinUIWindowReady `
                    -Handle $window.Handle `
                    -ProcessId $window.ProcessId `
                    -Width $window.Width `
                    -Height $window.Height
                $baseRecord.uiaControlCount = $readiness.ControlCount
                $baseRecord.uiaActionableControlCount = (
                    $readiness.ActionableControlCount
                )
                if (-not $readiness.Ready) {
                    $baseRecord.status = "skipped"
                    $baseRecord.reason = $readiness.Reason
                    $captureRecords.Add([pscustomobject]$baseRecord)
                    continue
                }
                $safety = Test-WindowIsSafeToCapture `
                    -Handle $window.Handle `
                    -Title ([string]$window.Title) `
                    -ClassName ([string]$window.ClassName)
                if (-not $safety.Safe) {
                    $baseRecord.status = "skipped"
                    $baseRecord.reason = $safety.Reason
                    $captureRecords.Add([pscustomobject]$baseRecord)
                    continue
                }
            }
            if ($successfulCaptures -ge $MaxCaptures) {
                $captureLimitReached = $true
                break
            }

            $sequence++
            $fileName = "{0}-p{1}-h{2}-s{3:D5}.png" -f (
                $capturedAt.ToString("yyyyMMddTHHmmssfffZ")
            ), $window.ProcessId, $window.Handle.ToString("x"), $sequence
            $imagePath = Join-Path $imagesDirectory $fileName
            $captured = if (
                $effectiveCaptureMethod -eq "WindowsGraphicsCapture"
            ) {
                Save-GraphicsCaptureWindowImage `
                    -HelperPath $graphicsCaptureHelper `
                    -Handle $window.Handle `
                    -ProcessId $window.ProcessId `
                    -Width $window.Width `
                    -Height $window.Height `
                    -ProcessIdentity $processIdentity `
                    -Path $imagePath
            } else {
                Save-PrintWindowImage `
                    -Handle $window.Handle `
                    -Width $window.Width `
                    -Height $window.Height `
                    -ProcessIdentity $processIdentity `
                    -ExecutableCatalog $executableCatalog `
                    -Path $imagePath
            }
            if (-not $captured) {
                $baseRecord.status = "failed"
                $baseRecord.reason = if (
                    $effectiveCaptureMethod -eq "WindowsGraphicsCapture"
                ) {
                    if (
                        [string]::IsNullOrWhiteSpace(
                            $script:LastGraphicsCaptureFailure
                        )
                    ) {
                        "windows-graphics-capture-failed"
                    } else {
                        $script:LastGraphicsCaptureFailure
                    }
                } else {
                    if (
                        [string]::IsNullOrWhiteSpace(
                            $script:LastPrintWindowFailure
                        )
                    ) {
                        "print-window-failed"
                    } else {
                        $script:LastPrintWindowFailure
                    }
                }
                $captureRecords.Add([pscustomobject]$baseRecord)
                continue
            }

            $imageHash = (
                Get-FileHash -LiteralPath $imagePath -Algorithm SHA256
            ).Hash.ToLowerInvariant()
            $windowKey = "{0}:{1}" -f $window.ProcessId, $window.Handle
            $previousHash = $null
            if (
                $lastWindowHashes.TryGetValue($windowKey, [ref]$previousHash) -and
                $previousHash -eq $imageHash
            ) {
                Remove-Item -LiteralPath $imagePath -Force
                $deduplicatedCaptures++
                continue
            }

            $imageLength = (Get-Item -LiteralPath $imagePath).Length
            if (($artifactBytes + $imageLength) -gt $MaxArtifactBytes) {
                Remove-Item -LiteralPath $imagePath -Force
                $baseRecord.status = "skipped"
                $baseRecord.reason = "artifact-quota-exceeded"
                $captureRecords.Add([pscustomobject]$baseRecord)
                $artifactQuotaReached = $true
                break
            }

            $lastWindowHashes[$windowKey] = $imageHash
            $baseRecord.status = "captured"
            $baseRecord.file = "images/$fileName"
            $baseRecord.sha256 = $imageHash
            $captureRecords.Add([pscustomobject]$baseRecord)
            $successfulCaptures++
            $artifactBytes += $imageLength
            if ($successfulCaptures -ge $MaxCaptures) {
                $captureLimitReached = $true
                break
            }
        }
        if (
            -not $captureLimitReached -and
            -not $artifactQuotaReached -and
            -not $manifestRecordLimitReached
        ) {
            Start-Sleep -Milliseconds $IntervalMilliseconds
        }
    }

    if ($successfulCaptures -eq 0) {
        throw (
            "No se capturo ninguna ventana segura de GrxFirma. " +
            "Compruebe que la sesion esta desbloqueada y revise los motivos del manifiesto."
        )
    }
    $runStatus = "succeeded"
} catch {
    $failureRecord = $_
    $failureMessage = Get-SanitizedFailureMessage -Message $_.Exception.Message
} finally {
    if (
        -not [string]::IsNullOrWhiteSpace(
            $graphicsCaptureWorkingDirectory
        ) -and
        (Test-Path -LiteralPath $graphicsCaptureWorkingDirectory)
    ) {
        try {
            $helperItem = Get-Item `
                -LiteralPath $graphicsCaptureWorkingDirectory `
                -Force `
                -ErrorAction Stop
            if (
                $helperItem.PSIsContainer -and
                ($helperItem.Attributes -band
                    [System.IO.FileAttributes]::ReparsePoint) -eq 0 -and
                (Test-PathIsWithin `
                    -Candidate $helperItem.FullName `
                    -Parent $script:RunDirectory)
            ) {
                Remove-Item `
                    -LiteralPath $helperItem.FullName `
                    -Recurse `
                    -Force `
                    -ErrorAction Stop
            }
        } catch {
            if ($null -eq $failureRecord) {
                $failureRecord = $_
                $failureMessage = Get-SanitizedFailureMessage `
                    -Message "No se pudo retirar el helper temporal de captura."
                $runStatus = "failed"
            }
        }
    }
    if (
        $CloseAfterCapture -and
        $null -ne $launchedProcess -and
        -not [string]::IsNullOrWhiteSpace($applicationDirectory)
    ) {
        try {
            $applicationClosed = Stop-GrxFirmaProcessLineage `
                -RootIdentity $rootProcessIdentity `
                -ExecutableCatalog $executableCatalog
        } catch {
            $applicationClosed = $false
        }
    }

    $endedAt = [DateTimeOffset]::UtcNow
    $capturedCount = @(
        $captureRecords | Where-Object { $_.status -eq "captured" }
    ).Count
    $skippedCount = @(
        $captureRecords | Where-Object { $_.status -eq "skipped" }
    ).Count
    $failedCount = @(
        $captureRecords | Where-Object { $_.status -eq "failed" }
    ).Count
    $terminationReason = if ($captureLimitReached) {
        "capture-count-limit"
    } elseif ($artifactQuotaReached) {
        "artifact-byte-quota"
    } elseif ($manifestRecordLimitReached) {
        "manifest-record-limit"
    } elseif (
        $null -ne $launchedProcess -and
        $launchedProcess.HasExited
    ) {
        "application-exited"
    } else {
        "duration-complete"
    }
    $manifest = [ordered]@{
        schemaVersion = 1
        tool = "GrxFirma window-only QA capture"
        runId = $runId
        status = $runStatus
        startedAtUtc = $startedAt.ToString("o")
        endedAtUtc = $endedAt.ToString("o")
        safety = [ordered]@{
            testDataOnlyDeclared = $true
            captureMethod = $effectiveCaptureMethod
            desktopCapture = $false
            captureTarget = "verified-grxfirma-hwnd-only"
            processScope = "launched-grxfirma-lineage"
            windowTitlesStored = $false
            titleMetadataStored = $false
            commandLinesStored = $false
            environmentStored = $false
            sensitiveWindows = "skipped-by-title-password-control-or-common-dialog"
        }
        source = [ordered]@{
            gitCommit = $commit
            repositoryDirty = $repositoryDirty
            architecture = $Architecture
            frontend = $Frontend
            buildPerformed = $buildPerformed
            executableSha256 = $executableHash
            frontendExecutableSha256 = $frontendExecutableHash
        }
        runtime = [ordered]@{
            os = [System.Environment]::OSVersion.VersionString
            powerShell = $PSVersionTable.PSVersion.ToString()
            sessionId = (Get-Process -Id $PID).SessionId
        }
        parameters = [ordered]@{
            durationSeconds = $DurationSeconds
            intervalMilliseconds = $IntervalMilliseconds
            frontend = $Frontend
            requestedCaptureMethod = $CaptureMethod
            closeAfterCapture = [bool]$CloseAfterCapture
            maxCaptures = $MaxCaptures
            maxArtifactBytes = $MaxArtifactBytes
            maxRetainedRuns = $MaxRetainedRuns
        }
        counts = [ordered]@{
            captured = $capturedCount
            deduplicated = $deduplicatedCaptures
            skipped = $skippedCount
            failed = $failedCount
            artifactBytes = $artifactBytes
            retentionRemoved = $retentionRemoved
        }
        terminationReason = $terminationReason
        applicationClosed = $applicationClosed
        failure = $failureMessage
        captures = @($captureRecords)
    }
    Write-JsonAtomically `
        -Path (Join-Path $script:RunDirectory "manifest.json") `
        -Value $manifest `
        -Depth 10

    if ($runStatus -eq "succeeded") {
        Write-ResultPointer -Status "succeeded"
    } else {
        Write-ResultPointer -Status "failed" -Failure $failureMessage
    }
}

if ($null -ne $failureRecord) {
    throw $failureRecord
}

Write-Output $script:RunDirectory
