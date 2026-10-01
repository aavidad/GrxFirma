# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$RepositoryRoot,
    [Parameter(Mandatory)]
    [string]$ResultPath,
    [Parameter(Mandatory)]
    [string]$DiagnosticPath,
    [string]$ApplicationDirectory,
    [string]$LauncherPath =
        "$env:LOCALAPPDATA\Programs\GrxFirma\DesktopLauncher\grxfirma-gui.exe",
    [switch]$LaunchApplication,
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

$launcherProcess = $null
$frontendPath = $null
$launchedAfter = [datetime]::Now
$qaMutex = $null
$ownsQaMutex = $false
$exitCode = 0

try {
    if ($LaunchApplication) {
        $qaMutex = [System.Threading.Mutex]::new(
            $false,
            "Local\GrxFirma-QA-InteractiveSigningSmoke"
        )
        try {
            $ownsQaMutex = $qaMutex.WaitOne(0)
        } catch [System.Threading.AbandonedMutexException] {
            $ownsQaMutex = $true
        }
        if (-not $ownsQaMutex) {
            throw (
                "Ya hay otra prueba gráfica de firma GrxFirma " +
                "activa en esta sesión."
            )
        }

        if ([string]::IsNullOrWhiteSpace($ApplicationDirectory)) {
            throw (
                "ApplicationDirectory es obligatorio cuando la tarea " +
                "debe abrir la aplicación."
            )
        }
        $applicationRoot =
            [System.IO.Path]::GetFullPath($ApplicationDirectory)
        $frontendPath = [System.IO.Path]::GetFullPath(
            (Join-Path $applicationRoot "grxfirma-winui.exe")
        )
        $resolvedLauncher =
            [System.IO.Path]::GetFullPath($LauncherPath)
        foreach ($required in @(
            $applicationRoot,
            $frontendPath,
            $resolvedLauncher
        )) {
            if (-not (Test-Path -LiteralPath $required)) {
                throw "No existe el componente QA que se debe abrir: $required"
            }
        }

        $alreadyOpen = @(
            Get-Process `
                -Name `
                    "grxfirma-gui", `
                    "grxfirma-gui-qml", `
                    "grxfirma-winui" `
                -ErrorAction SilentlyContinue
        )
        if ($alreadyOpen.Count -ne 0) {
            throw (
                "GrxFirma ya está en uso en esta sesión; " +
                "la tarea no abrirá, reutilizará ni cerrará ninguna instancia."
            )
        }

        $launchedAfter = [datetime]::Now.AddSeconds(-2)
        $launcherProcess = Start-Process `
            -FilePath $resolvedLauncher `
            -ArgumentList @(
                "--frontend=winui",
                "--ui-binary=`"$frontendPath`""
            ) `
            -WorkingDirectory $applicationRoot `
            -PassThru
    }

    & (Join-Path `
        $RepositoryRoot `
        "scripts\windows-qa\Invoke-GrxFirmaWinUiSigningSmoke.ps1") `
        -RepositoryRoot $RepositoryRoot `
        -TestDataOnly `
        -ResultPath $ResultPath `
        -ApplicationDirectory $ApplicationDirectory `
        -CertificateNameContains $CertificateNameContains `
        -CertificateEvidenceLabel $CertificateEvidenceLabel `
        -CertificateSource $CertificateSource `
        -OfficialFnmtCredentialPath $OfficialFnmtCredentialPath `
        -ManualSaveConfirmation:$ManualSaveConfirmation `
        -VisibleSealPades:$VisibleSealPades `
        -VisibleSealRotation $VisibleSealRotation `
        -ExternalPointerInputConfirmation:$ExternalPointerInputConfirmation `
        -TimeoutSeconds $TimeoutSeconds
    [System.IO.File]::WriteAllText(
        $DiagnosticPath,
        "completed",
        [System.Text.UTF8Encoding]::new($false)
    )
} catch {
    $safeMessage = $_.Exception.Message
    if ([string]::IsNullOrWhiteSpace($safeMessage)) {
        $safeMessage = "Fallo no especificado del controlador de QA."
    }
    [System.IO.File]::WriteAllText(
        $DiagnosticPath,
        $safeMessage,
        [System.Text.UTF8Encoding]::new($false)
    )
    $exitCode = 1
} finally {
    if ($LaunchApplication -and $null -ne $frontendPath) {
        foreach ($process in @(
            Get-Process `
                -Name "grxfirma-winui" `
                -ErrorAction SilentlyContinue
        )) {
            try {
                if (
                    $process.StartTime -ge $launchedAfter -and
                    [System.IO.Path]::GetFullPath($process.Path).Equals(
                        $frontendPath,
                        [System.StringComparison]::OrdinalIgnoreCase
                    )
                ) {
                    [void]$process.CloseMainWindow()
                    if (-not $process.WaitForExit(3000)) {
                        Stop-Process `
                            -Id $process.Id `
                            -ErrorAction SilentlyContinue
                    }
                }
            } catch {
                # Solo se intenta cerrar el proceso cuyo path e instante de
                # creación corresponden a esta tarea.
            }
        }
    }

    if ($null -ne $launcherProcess) {
        try {
            $current = Get-Process `
                -Id $launcherProcess.Id `
                -ErrorAction Stop
            if (
                $current.StartTime -eq $launcherProcess.StartTime -and
                [System.IO.Path]::GetFullPath($current.Path).Equals(
                    [System.IO.Path]::GetFullPath($LauncherPath),
                    [System.StringComparison]::OrdinalIgnoreCase
                )
            ) {
                [void]$current.CloseMainWindow()
                if (-not $current.WaitForExit(3000)) {
                    Stop-Process `
                        -Id $current.Id `
                        -ErrorAction SilentlyContinue
                }
            }
        } catch {
            # El backend suele terminar al cerrarse la interfaz.
        }
    }

    if ($ownsQaMutex -and $null -ne $qaMutex) {
        try {
            $qaMutex.ReleaseMutex()
        } catch {
            # El mutex solo protege la concurrencia del arnés de QA.
        }
    }
    if ($null -ne $qaMutex) {
        $qaMutex.Dispose()
    }
}

exit $exitCode
