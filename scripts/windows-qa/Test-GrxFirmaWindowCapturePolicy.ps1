# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$scripts = @(
    (Join-Path $PSScriptRoot "Invoke-GrxFirmaWindowCapture.ps1"),
    (Join-Path $PSScriptRoot "Start-GrxFirmaWindowCaptureTask.ps1")
)
$graphicsHelperRoot = Join-Path `
    $PSScriptRoot `
    "GrxFirma.WindowCapture"
$graphicsHelperProject = Join-Path `
    $graphicsHelperRoot `
    "GrxFirma.WindowCapture.csproj"
$graphicsHelperSource = Join-Path $graphicsHelperRoot "Program.cs"
$graphicsHelperLock = Join-Path $graphicsHelperRoot "packages.lock.json"

foreach ($script in $scripts) {
    $tokens = $null
    $errors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile(
        $script,
        [ref]$tokens,
        [ref]$errors
    )
    if ($errors.Count -gt 0) {
        $messages = $errors | ForEach-Object {
            "{0}:{1}:{2}: {3}" -f `
                $script,
                $_.Extent.StartLineNumber,
                $_.Extent.StartColumnNumber,
                $_.Message
        }
        throw "PowerShell invalido:`n$($messages -join "`n")"
    }
}

$captureSource = Get-Content -LiteralPath $scripts[0] -Raw
$requiredPatterns = @(
    "WindowCaptureNative]::PrintWindow",
    "GetWindowThreadProcessId",
    "GetClassName",
    "launched-grxfirma-lineage",
    "[ValidateSet(""Qt"", ""WinUI"")]",
    "grxfirma-winui.exe",
    "grxfirma-gui.exe",
    "windowTitlesStored = `$false",
    "titleMetadataStored = `$false",
    "commandLinesStored = `$false",
    "[switch]`$TestDataOnly",
    "LOCALAPPDATA",
    "common-dialog-class",
    "IsPasswordProperty",
    "MaxCaptures",
    "MaxArtifactBytes",
    "MaxRetainedRuns",
    "deduplicatedCaptures",
    "Assert-LocalPathWithoutReparsePoint",
    "Protect-ArtifactDirectory",
    "[System.IO.DriveInfo]::new",
    "[System.IO.DriveType]::Network",
    "Test-CaptureRunDeletionCandidate",
    "GrxFirma window-only QA task pointer"
    "[ValidateSet(""Auto"", ""PrintWindow"", ""WindowsGraphicsCapture"")]"
    "Build-GraphicsCaptureHelper"
    "Save-GraphicsCaptureWindowImage"
    "captureTarget = ""verified-grxfirma-hwnd-only"""
    "windows-graphics-capture-failed"
    "window-content-not-ready"
    "allowedFailureReasons"
    "Stop-GrxFirmaProcessLineage"
    "nativeProcess.ExecutablePath"
    "Wait-WinUIWindowReady"
    "PrepareVerifiedWindow"
    "winui-control-tree-not-ready"
    "uiaActionableControlCount"
    "Test-BitmapHasUsefulClientContent"
    "print-window-content-not-ready"
    "print-window-target-changed"
    "New-AllowedExecutableCatalog"
    "Get-VerifiedProcessIdentity"
    "StartTimeUtcTicks"
    "ExecutableSha256"
    "[System.IO.File]::Move(`$temporaryPath, `$Path, `$false)"
)
foreach ($pattern in $requiredPatterns) {
    if (-not $captureSource.Contains($pattern)) {
        throw "Falta la salvaguarda obligatoria en el capturador: $pattern"
    }
}

$captureLoop = [regex]::Match(
    $captureSource,
    '(?s)\$safety = Test-WindowIsSafeToCapture.*?' +
        '\$imageHash = \('
)
if (-not $captureLoop.Success) {
    throw "No se pudo aislar el bucle seguro de captura."
}
if (
    $captureLoop.Value.IndexOf(
        "Save-GraphicsCaptureWindowImage",
        [System.StringComparison]::Ordinal
    ) -le
    $captureLoop.Value.IndexOf(
        "Test-WindowIsSafeToCapture",
        [System.StringComparison]::Ordinal
    )
) {
    throw "WindowsGraphicsCapture solo puede ejecutarse tras validar la ventana."
}

$forbiddenPatterns = @(
    "\bCopyFromScreen\b",
    "\bGetDesktopWindow\b",
    "\bGetShellWindow\b",
    "GetDC\s*\(\s*(?:IntPtr\.)?Zero",
    "FromHwnd\s*\(\s*(?:IntPtr\.)?Zero",
    "\bVirtualScreen\b",
    "\bPrimaryScreen\b",
    "\bAllScreens\b",
    "\bBitBlt\b"
)
foreach ($pattern in $forbiddenPatterns) {
    if (
        [regex]::IsMatch(
            $captureSource,
            $pattern,
            [System.Text.RegularExpressions.RegexOptions]::IgnoreCase
        )
    ) {
        throw "API de captura de escritorio prohibida en el capturador: $pattern"
    }
}

foreach ($pattern in @(
    "windowTitleSha256",
    "windowTitleLength",
    "Get-Sha256Text"
)) {
    if ($captureSource.Contains($pattern)) {
        throw "Metadato derivado del titulo prohibido en el manifiesto: $pattern"
    }
}

$createRunIndex = $captureSource.IndexOf(
    'New-Item -ItemType Directory -Path $script:RunDirectory'
)
$protectRunIndex = $captureSource.IndexOf(
    'Protect-ArtifactDirectory -Path $script:RunDirectory'
)
$createImagesIndex = $captureSource.IndexOf(
    'New-Item -ItemType Directory -Path $imagesDirectory'
)
if (
    $createRunIndex -lt 0 -or
    $protectRunIndex -le $createRunIndex -or
    $createImagesIndex -le $protectRunIndex
) {
    throw "RunDirectory debe protegerse antes de crear el directorio de imagenes."
}

$captureRetentionMatch = [regex]::Match(
    $captureSource,
    '(?s)function Remove-ExpiredCaptureRuns.*?function Get-SanitizedFailureMessage'
)
if (-not $captureRetentionMatch.Success) {
    throw "No se pudo aislar la poda de ejecuciones para validarla."
}
if ($captureRetentionMatch.Value.Contains("-ErrorAction SilentlyContinue")) {
    throw "La poda de ejecuciones no puede ocultar errores de enumeracion."
}
if (
    (
        [regex]::Matches(
            $captureRetentionMatch.Value,
            "Test-CaptureRunDeletionCandidate"
        )
    ).Count -lt 2
) {
    throw "La poda debe revalidar cada ejecucion inmediatamente antes de borrarla."
}

$taskSource = Get-Content -LiteralPath $scripts[1] -Raw
foreach ($pattern in @(
    "-LogonType Interactive",
    "-RunLevel Limited",
    '-TaskPath "\"',
    "sesion SSH",
    "-TestDataOnly",
    "Stop-ScheduledTask",
    "Unregister-ScheduledTask",
    "build-desktop-qml.ps1",
    "build-desktop-winui.ps1",
    '"-Frontend"',
    '"-SkipBuild"',
    '"-BuildPerformedBeforeLaunch"',
    "Assert-LocalPathWithoutReparsePoint",
    "Protect-TaskResultDirectory",
    "[System.IO.DriveInfo]::new",
    "[System.IO.DriveType]::Network",
    "MaxRetainedTaskResults",
    "Remove-ExpiredTaskResultPointers",
    "Remove-TaskResultPointerSafely",
    "Test-TaskResultPointerDeletionCandidate"
    '"-CaptureMethod"'
)) {
    if (-not $taskSource.Contains($pattern)) {
        throw "Falta la salvaguarda obligatoria en la tarea interactiva: $pattern"
    }
}

foreach ($path in @(
    $graphicsHelperProject,
    $graphicsHelperSource,
    $graphicsHelperLock
)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Falta un fichero obligatorio del helper por HWND: $path"
    }
}

$helperProjectSource = Get-Content -LiteralPath $graphicsHelperProject -Raw
foreach ($pattern in @(
    "<TargetFramework>net10.0-windows10.0.17763.0</TargetFramework>",
    "<RuntimeIdentifier>win-x64</RuntimeIdentifier>",
    "<TreatWarningsAsErrors>true</TreatWarningsAsErrors>",
    "<RestorePackagesWithLockFile>true</RestorePackagesWithLockFile>"
)) {
    if (-not $helperProjectSource.Contains($pattern)) {
        throw "El proyecto del helper no fija su contrato: $pattern"
    }
}
if ($helperProjectSource.Contains("<PackageReference")) {
    throw "El helper QA no puede añadir dependencias NuGet directas."
}

$helperSource = Get-Content -LiteralPath $graphicsHelperSource -Raw
foreach ($pattern in @(
    "MinimumGraphicsCaptureBuild = 18362",
    "GraphicsCaptureSession.IsSupported()",
    "CreateForWindowDelegate",
    "target-window-process-mismatch",
    "target-window-process-not-allowed",
    "target-window-process-start-mismatch",
    "target-window-process-path-mismatch",
    "target-window-process-hash-mismatch",
    "GetWindowThreadProcessId",
    "ValidateTargetWindow(options)",
    "ValidateLocalOutputDirectory",
    "reparse-output-forbidden",
    "SoftwareBitmap.CreateCopyFromSurfaceAsync",
    "HasUsefulClientContent(bitmap)",
    "minimumNonDominantSamples",
    "window-content-not-ready",
    "Channel.CreateBounded<Direct3D11CaptureFrame>",
    "CryptographicOperations.ZeroMemory(pixels)",
    "CryptographicOperations.ZeroMemory(bytes)",
    "File.Move(temporaryPath, outputPath, overwrite: false)"
    "CryptographicOperations.FixedTimeEquals"
    "process.StartTime.ToUniversalTime().Ticks"
    "process.MainModule?.FileName"
)) {
    if (-not $helperSource.Contains($pattern)) {
        throw "Falta una salvaguarda del helper por HWND: $pattern"
    }
}
if (
    (
        [regex]::Matches(
            $helperSource,
            [regex]::Escape("ValidateTargetWindow(options)")
        )
    ).Count -lt 4
) {
    throw "El helper debe revalidar HWND, PID y dimensiones durante los reintentos."
}
foreach ($pattern in @(
    "\bCreateForMonitor\b",
    "\bGraphicsCapturePicker\b",
    "\bCopyFromScreen\b",
    "\bGetDesktopWindow\b",
    "\bGetShellWindow\b",
    "\bBitBlt\b",
    "\bPrimaryScreen\b",
    "\bAllScreens\b"
)) {
    if (
        [regex]::IsMatch(
            $helperSource,
            $pattern,
            [System.Text.RegularExpressions.RegexOptions]::IgnoreCase
        )
    ) {
        throw "API de escritorio prohibida en el helper por HWND: $pattern"
    }
}

$helperLockValue = Get-Content -LiteralPath $graphicsHelperLock -Raw |
    ConvertFrom-Json -ErrorAction Stop
if (
    $helperLockValue.version -ne 1 -or
    $null -eq (
        $helperLockValue.dependencies.PSObject.Properties[
            "net10.0-windows10.0.17763"
        ]
    ) -or
    $null -eq (
        $helperLockValue.dependencies.PSObject.Properties[
            "net10.0-windows10.0.17763/win-x64"
        ]
    )
) {
    throw "El lockfile del helper no cubre TFM y RID."
}

foreach ($pattern in @(
    "New-ScheduledTaskTrigger",
    "-Trigger `$trigger",
    "AddYears(1)"
)) {
    if ($taskSource.Contains($pattern)) {
        throw "La tarea QA debe ser exclusivamente manual y no tener trigger: $pattern"
    }
}

$taskRetentionMatch = [regex]::Match(
    $taskSource,
    '(?s)function Remove-ExpiredTaskResultPointers.*?function Remove-TaskResultPointerSafely'
)
if (-not $taskRetentionMatch.Success) {
    throw "No se pudo aislar la poda de punteros para validarla."
}
if ($taskRetentionMatch.Value.Contains("-ErrorAction SilentlyContinue")) {
    throw "La poda de punteros no puede ocultar errores de enumeracion."
}
if (
    (
        [regex]::Matches(
            $taskRetentionMatch.Value,
            "Test-TaskResultPointerDeletionCandidate"
        )
    ).Count -lt 2
) {
    throw "La poda debe revalidar cada puntero inmediatamente antes de borrarlo."
}
if (
    -not [regex]::IsMatch(
        $taskSource,
        '(?s)if \(-not \$NoWait -and -not \$KeepTask\).*?' +
            'Remove-TaskResultPointerSafely'
    )
) {
    throw "La espera normal debe retirar su puntero de resultado."
}

$buildIndex = $taskSource.IndexOf("& `$buildScript")
$registerIndex = $taskSource.IndexOf("Register-ScheduledTask")
if ($buildIndex -lt 0 -or $registerIndex -le $buildIndex) {
    throw "La compilacion debe completarse en SSH antes de registrar la tarea grafica."
}
if (
    -not [regex]::IsMatch(
        $taskSource,
        '(?s)\$registered\s+-and\s+-not\s+\$NoWait\s+-and\s+-not\s+\$KeepTask'
    )
) {
    throw "La limpieza de tarea debe cubrir error y timeout salvo NoWait/KeepTask."
}
if (
    -not [regex]::IsMatch(
        $taskSource,
        '(?s)\$task\.State\s+-ne\s+"Running".*LastRunTime.*puntero final'
    )
) {
    throw "Falta detectar una tarea terminada con puntero todavia no final."
}

$taskTokens = $null
$taskErrors = $null
$taskAst = [System.Management.Automation.Language.Parser]::ParseFile(
    $scripts[1],
    [ref]$taskTokens,
    [ref]$taskErrors
)
$quoteFunction = $taskAst.Find(
    {
        param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
            $node.Name -eq "Quote-TaskArgument"
    },
    $true
)
if ($null -eq $quoteFunction) {
    throw "No se encontro Quote-TaskArgument para validar su contrato."
}
$quoteContract = [scriptblock]::Create(
    $quoteFunction.Extent.Text +
@'

if ((Quote-TaskArgument -Value "plain") -ne '"plain"') {
    throw "Quote-TaskArgument no delimita un argumento simple."
}
if ((Quote-TaskArgument -Value 'C:\qa path\') -ne '"C:\qa path\\"') {
    throw "Quote-TaskArgument no preserva espacios o barras finales."
}
if ((Quote-TaskArgument -Value "") -ne '""') {
    throw "Quote-TaskArgument no preserva un argumento vacio."
}
$quoteRejected = $false
try {
    [void](Quote-TaskArgument -Value 'bad"value')
} catch {
    $quoteRejected = $true
}
if (-not $quoteRejected) {
    throw "Quote-TaskArgument debe rechazar comillas incrustadas."
}
'@
)
& $quoteContract

Write-Output "Politica de captura de ventanas GrxFirma validada."
