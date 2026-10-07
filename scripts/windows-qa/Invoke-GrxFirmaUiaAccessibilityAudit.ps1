# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

<#
.SYNOPSIS
Inventario de accesibilidad de la interfaz WinUI con UI Automation.

.DESCRIPTION
Abre GrxFirma, recorre cada sección del menú principal y anota, por sección,
los controles interactivos sin nombre, los objetivos menores de 24x24 px, las
imágenes de contenido sin nombre, los controles habilitados que no reciben el
foco del teclado, los encabezados (HeadingLevel) y los campos obligatorios.
Si la sesión está desbloqueada, también recorre el foco con Tab. Guarda una
captura de cada sección con PrintWindow.

Debe ejecutarse en la sesión gráfica (tarea programada interactiva). Con la
sesión bloqueada UI Automation sigue respondiendo, pero no se envían teclas.
Base del método de docs/ACCESIBILIDAD.md.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)] [string] $OutputFile,
    [Parameter(Mandatory)] [string] $ScreenshotDirectory,
    [string] $InstallDirectory = "$env:LOCALAPPDATA\Programs\GrxFirma",
    [string] $UiBinary = '',
    [int] $StartupSeconds = 20,
    [int] $TabSteps = 45
)
$ErrorActionPreference = 'Continue'
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes, System.Windows.Forms, System.Drawing
Add-Type @"
using System; using System.Runtime.InteropServices;
public static class GrxA11yNative {
  [DllImport("user32.dll")] public static extern void mouse_event(int f, int x, int y, int d, int e);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, int f);
}
"@
New-Item -ItemType Directory -Force $ScreenshotDirectory | Out-Null
# Despierta la pantalla: con el monitor dormido las capturas salen negras.
[GrxA11yNative]::mouse_event(1, 5, 5, 0, 0)

if (-not $UiBinary) { $UiBinary = Join-Path $InstallDirectory 'DesktopWinUI\grxfirma-winui.exe' }
Get-Process grxfirma* -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 2
Start-Process (Join-Path $InstallDirectory 'DesktopLauncher\grxfirma-gui.exe') -ArgumentList '--frontend=winui', "--ui-binary=$UiBinary"
Start-Sleep -Seconds $StartupSeconds

$lines = [System.Collections.Generic.List[string]]::new()
$AE = [System.Windows.Automation.AutomationElement]
$CT = [System.Windows.Automation.ControlType]
$ids = [System.Windows.Automation.AutomationElementIdentifiers]
$window = $AE::RootElement.FindFirst('Children', [System.Windows.Automation.PropertyCondition]::new($AE::NameProperty, 'GrxFirma'))
if (-not $window) {
    'sin ventana' | Set-Content -Encoding utf8 $OutputFile
    exit 1
}
$hwnd = [IntPtr]$window.Current.NativeWindowHandle
[GrxA11yNative]::SetForegroundWindow($hwnd) | Out-Null
$interactive = @($CT::Button, $CT::Edit, $CT::ComboBox, $CT::CheckBox, $CT::RadioButton, $CT::Slider,
    $CT::Spinner, $CT::Hyperlink, $CT::ListItem, $CT::MenuItem, $CT::TabItem, $CT::SplitButton, $CT::TreeItem, $CT::DataItem)

function Save-Screenshot([string] $name) {
    $r = $window.Current.BoundingRectangle
    $bmp = New-Object System.Drawing.Bitmap ([int]$r.Width), ([int]$r.Height)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $hdc = $g.GetHdc(); [GrxA11yNative]::PrintWindow($hwnd, $hdc, 2) | Out-Null; $g.ReleaseHdc($hdc)
    $bmp.Save((Join-Path $ScreenshotDirectory "$name.png")); $g.Dispose(); $bmp.Dispose()
}

function Add-Inventory([string] $label) {
    $lines.Add("===== $label")
    $all = $window.FindAll('Descendants', [System.Windows.Automation.Condition]::TrueCondition)
    $lines.Add("elementos=" + $all.Count)
    foreach ($e in $all) {
        try {
            $c = $e.Current
            if ($c.IsOffscreen) { continue }
            $ct = $c.ControlType
            $r = $c.BoundingRectangle
            $isInteractive = $interactive -contains $ct
            $heading = "$($e.GetCurrentPropertyValue($ids::HeadingLevelProperty))"
            $required = $e.GetCurrentPropertyValue($ids::IsRequiredForFormProperty)
            $flags = @()
            if ($isInteractive -and [string]::IsNullOrWhiteSpace($c.Name)) { $flags += 'SIN_NOMBRE' }
            if ($isInteractive -and $c.IsEnabled -and $r.Width -gt 0 -and ($r.Width -lt 24 -or $r.Height -lt 24)) { $flags += 'PEQUENO' }
            if ($ct -eq $CT::Image -and $c.IsContentElement -and [string]::IsNullOrWhiteSpace($c.Name)) { $flags += 'IMAGEN_SIN_NOMBRE' }
            if ($isInteractive -and $c.IsEnabled -and -not $c.IsKeyboardFocusable -and $ct -ne $CT::ListItem) { $flags += 'SIN_TECLADO' }
            $headingText = if ($heading -match 'Level\d') { "H=$heading" } else { '' }
            if ($isInteractive -or $flags.Count -gt 0 -or $headingText -or $ct -eq $CT::Image -or $ct -eq $CT::ProgressBar) {
                $lines.Add(("{0} | {1} | {2} | {3}x{4} | {5} {6} {7} | ayuda={8}" -f $ct.ProgrammaticName.Replace('ControlType.', ''),
                    $c.Name, $c.AutomationId, [int]$r.Width, [int]$r.Height, ($flags -join ','), $headingText,
                    $(if ($required) { 'OBLIGATORIO' } else { '' }), $c.HelpText))
            }
        } catch { }
    }
}

function Add-TabWalk([string] $label) {
    if (Get-Process LogonUI -ErrorAction SilentlyContinue) { $lines.Add("----- TAB ${label}: sesión bloqueada, sin recorrido"); return }
    $lines.Add("----- TAB $label")
    for ($i = 1; $i -le $TabSteps; $i++) {
        [System.Windows.Forms.SendKeys]::SendWait('{TAB}'); Start-Sleep -Milliseconds 300
        try {
            $f = $AE::FocusedElement.Current
            $lines.Add(("{0:00} {1} | {2} | {3}" -f $i, $f.ControlType.ProgrammaticName.Replace('ControlType.', ''), $f.Name, $f.AutomationId))
        } catch { $lines.Add("$i ?") }
    }
}

$navClass = [System.Windows.Automation.PropertyCondition]::new($AE::ClassNameProperty, 'Microsoft.UI.Xaml.Controls.NavigationViewItem')
Save-Screenshot 'inicio'
Add-Inventory 'inicio'
$names = @($window.FindAll('Descendants', $navClass) | ForEach-Object { $_.Current.Name })
$lines.Add("secciones: " + ($names -join ' / '))
foreach ($name in $names) {
    $item = $window.FindFirst('Descendants', [System.Windows.Automation.AndCondition]::new($navClass,
        [System.Windows.Automation.PropertyCondition]::new($AE::NameProperty, $name)))
    if (-not $item) { continue }
    try { $item.GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select() }
    catch { try { $item.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke() } catch { $lines.Add("no se pudo abrir $name") } }
    Start-Sleep -Seconds 3
    Save-Screenshot ($name -replace '[^\w]', '_')
    Add-Inventory $name
    try { $item.SetFocus() } catch { }
    Add-TabWalk $name
}
$lines | Set-Content -Encoding utf8 $OutputFile
Get-Process grxfirma* -ErrorAction SilentlyContinue | Stop-Process -Force
