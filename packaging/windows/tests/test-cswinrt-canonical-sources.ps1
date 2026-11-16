# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Las fuentes de CsWinRT de dos compilaciones del mismo código, que solo
# difieren en el orden y en el nombre de las clases compartidas, deben quedar
# idénticas en forma canónica. Es lo que hace reproducible grxfirma-winui.dll.

$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

$windowsDirectory = Split-Path -Parent $PSScriptRoot
. (Join-Path $windowsDirectory "cswinrt-canonical-sources.ps1")

$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("grxfirma-cswinrt-" + [Guid]::NewGuid().ToString("N"))
$vtableNamespace = "WinRT.grxfirma_winuiVtableClasses"

function New-CcwClass {
    param([string]$Name, [string]$Interface)
    return @"
internal sealed class $($Name)WinRTTypeDetails : global::WinRT.IWinRTExposedTypeDetails
{
    public global::System.Runtime.InteropServices.ComWrappers.ComInterfaceEntry[] GetExposedInterfaces()
    {
        return new global::System.Runtime.InteropServices.ComWrappers.ComInterfaceEntry[]
        {
            new global::System.Runtime.InteropServices.ComWrappers.ComInterfaceEntry
            {
                IID = global::ABI.$($Interface)Methods.IID,
                Vtable = global::ABI.$($Interface)Methods.AbiToProjectionVftablePtr
            },
};
        }
    }
"@
}

function New-TypeFile {
    param([string]$Namespace, [string]$Type, [string]$Details)
    return @"
namespace $Namespace
{
[global::WinRT.WinRTExposedType(typeof(global::$vtableNamespace.$($Details)WinRTTypeDetails))]
partial class $Type
{
}
}
"@
}

# Escribe lo que produciría el generador recorriendo los tipos en $Order.
# La clase compartida por MainWindow y las dos clases de x:Bind toma el
# nombre del primero de ellos que aparece.
function Write-GeneratorOutput {
    param([string]$Directory, [string[]]$Order)
    $aot = Join-Path $Directory "Generator.WinRTAotSourceGenerator"
    New-Item -ItemType Directory -Force -Path $aot | Out-Null
    $shared = @("GrxFirma_WinUI_MainWindow", "GrxFirma_WinUI_Views_HashPage_HashPage_obj1_Bindings", "GrxFirma_WinUI_Views_SignPage_SignPage_obj1_Bindings")
    $sharedName = $null
    $classes = New-Object System.Collections.Generic.List[string]
    $details = @{}
    foreach ($type in $Order) {
        if ($shared -contains $type) {
            if ($null -eq $sharedName) {
                $sharedName = $type
                $classes.Add((New-CcwClass -Name $type -Interface "Microsoft.UI.Xaml.Markup.IComponentConnector"))
            }
            $details[$type] = $sharedName
        } else {
            $classes.Add((New-CcwClass -Name $type -Interface "Microsoft.UI.Xaml.Controls.IPageOverrides"))
            $details[$type] = $type
        }
    }
    $ccw = "namespace $vtableNamespace`n{`n" + ($classes -join "`n") + "`n}`n"
    [System.IO.File]::WriteAllText((Join-Path $aot "WinRTCCWVtable.g.cs"), $ccw, (New-Object System.Text.UTF8Encoding($true)))
    $files = @{
        "GrxFirma_WinUI_MainWindow" = @("GrxFirma.WinUI.MainWindow", "GrxFirma.WinUI", "MainWindow")
        "GrxFirma_WinUI_Views_HashPage" = @("GrxFirma.WinUI.Views.HashPage", "GrxFirma.WinUI.Views", "HashPage")
        "GrxFirma_WinUI_Views_HashPage_HashPage_obj1_Bindings" = @("GrxFirma.WinUI.Views.HashPage_HashPage_obj1_Bindings", "GrxFirma.WinUI.Views", "HashPage_obj1_Bindings")
        "GrxFirma_WinUI_Views_SignPage_SignPage_obj1_Bindings" = @("GrxFirma.WinUI.Views.SignPage_SignPage_obj1_Bindings", "GrxFirma.WinUI.Views", "SignPage_obj1_Bindings")
    }
    foreach ($type in $Order) {
        $file = $files[$type]
        [System.IO.File]::WriteAllText(
            (Join-Path $aot ($file[0] + ".WinRTVtable.g.cs")),
            (New-TypeFile -Namespace $file[1] -Type $file[2] -Details $details[$type]),
            (New-Object System.Text.UTF8Encoding($true))
        )
    }
    [System.IO.File]::WriteAllText((Join-Path $aot "WinRTGlobalVtableLookup.g.cs"), "namespace WinRT.grxfirma_winuiGenericHelpers`n{`n}`n")
}

function Get-TreeText {
    param([string]$Directory)
    $root = (Get-Item -LiteralPath $Directory).FullName
    $parts = foreach ($file in Get-ChildItem -LiteralPath $root -File -Recurse | Sort-Object FullName) {
        $file.FullName.Substring($root.Length) + "`n" + [System.IO.File]::ReadAllText($file.FullName)
    }
    return ($parts -join "`n----`n")
}

function Assert-CanonicalThrows {
    param([string]$Generated, [string]$Message)
    $destination = Join-Path $temporaryRoot ("fail-" + [Guid]::NewGuid().ToString("N"))
    try {
        ConvertTo-GrxFirmaCanonicalCsWinRTSources `
            -GeneratedDirectory $Generated `
            -DestinationDirectory $destination `
            -ListPath "$destination.txt" | Out-Null
    } catch {
        return
    }
    throw $Message
}

try {
    $orderOne = @(
        "GrxFirma_WinUI_Views_SignPage_SignPage_obj1_Bindings",
        "GrxFirma_WinUI_Views_HashPage",
        "GrxFirma_WinUI_MainWindow",
        "GrxFirma_WinUI_Views_HashPage_HashPage_obj1_Bindings"
    )
    $orderTwo = @(
        "GrxFirma_WinUI_Views_HashPage_HashPage_obj1_Bindings",
        "GrxFirma_WinUI_MainWindow",
        "GrxFirma_WinUI_Views_SignPage_SignPage_obj1_Bindings",
        "GrxFirma_WinUI_Views_HashPage"
    )
    $generatedOne = Join-Path $temporaryRoot "one/WinRT.SourceGenerator"
    $generatedTwo = Join-Path $temporaryRoot "two-otra-ruta/WinRT.SourceGenerator"
    Write-GeneratorOutput -Directory $generatedOne -Order $orderOne
    Write-GeneratorOutput -Directory $generatedTwo -Order $orderTwo
    if ((Get-TreeText $generatedOne) -eq (Get-TreeText $generatedTwo)) {
        throw "El caso de prueba no reproduce la diferencia del generador"
    }

    $canonicalOne = Join-Path $temporaryRoot "canonical-one"
    $canonicalTwo = Join-Path $temporaryRoot "canonical-two"
    $resultOne = ConvertTo-GrxFirmaCanonicalCsWinRTSources `
        -GeneratedDirectory $generatedOne `
        -DestinationDirectory $canonicalOne `
        -ListPath (Join-Path $temporaryRoot "one.txt")
    $resultTwo = ConvertTo-GrxFirmaCanonicalCsWinRTSources `
        -GeneratedDirectory $generatedTwo `
        -DestinationDirectory $canonicalTwo `
        -ListPath (Join-Path $temporaryRoot "two.txt")
    if ((Get-TreeText $canonicalOne) -cne (Get-TreeText $canonicalTwo)) {
        throw "Las fuentes canónicas de CsWinRT dependen del orden del generador"
    }
    if ($resultOne.Files -ne 6 -or $resultTwo.Files -ne 6) {
        throw "Faltan fuentes canónicas de CsWinRT"
    }
    if ($resultOne.Renamed -ne 1 -or $resultTwo.Renamed -ne 1) {
        throw "La clase compartida no se ha renombrado una sola vez"
    }

    $aotOne = Join-Path $canonicalOne "Generator.WinRTAotSourceGenerator"
    $ccw = [System.IO.File]::ReadAllText((Join-Path $aotOne "WinRTCCWVtable.g.cs"))
    $declared = @([regex]::Matches($ccw, "(?m)^internal sealed class (\w+)WinRTTypeDetails") | ForEach-Object { $_.Groups[1].Value })
    $expected = @("GrxFirma_WinUI_MainWindow", "GrxFirma_WinUI_Views_HashPage")
    if (($declared -join ",") -cne ($expected -join ",")) {
        throw "Clases canónicas inesperadas: $($declared -join ', ')"
    }
    $bindings = [System.IO.File]::ReadAllText((Join-Path $aotOne "GrxFirma.WinUI.Views.SignPage_SignPage_obj1_Bindings.WinRTVtable.g.cs"))
    if (-not $bindings.Contains("global::$vtableNamespace.GrxFirma_WinUI_MainWindowWinRTTypeDetails")) {
        throw "Las clases de x:Bind no citan la clase compartida canónica"
    }
    $bytes = [System.IO.File]::ReadAllBytes((Join-Path $aotOne "WinRTCCWVtable.g.cs"))
    if ($bytes[0] -ne 0xEF -or $bytes[1] -ne 0xBB -or $bytes[2] -ne 0xBF) {
        throw "Las fuentes canónicas deben conservar la marca BOM"
    }

    $listed = @(Get-Content -LiteralPath (Join-Path $temporaryRoot "one.txt"))
    $relativeListed = @($listed | ForEach-Object { $_.Substring($canonicalOne.Length).TrimStart('\', '/').Replace('\', '/') })
    $sortedListed = [string[]]$relativeListed.Clone()
    [Array]::Sort($sortedListed, [StringComparer]::Ordinal)
    if (($relativeListed -join "`n") -cne ($sortedListed -join "`n")) {
        throw "La lista de fuentes canónicas no sigue un orden fijo"
    }
    foreach ($path in $listed) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "La lista cita una fuente que no existe: $path"
        }
    }

    Assert-CanonicalThrows `
        -Generated (Join-Path $temporaryRoot "no-existe") `
        -Message "Se aceptó un directorio de fuentes inexistente"

    $empty = Join-Path $temporaryRoot "vacio"
    New-Item -ItemType Directory -Force -Path $empty | Out-Null
    Assert-CanonicalThrows -Generated $empty -Message "Se aceptó un generador sin fuentes"

    $unknownReference = Join-Path $temporaryRoot "cita-rota/WinRT.SourceGenerator"
    Write-GeneratorOutput -Directory $unknownReference -Order $orderOne
    [System.IO.File]::WriteAllText(
        (Join-Path $unknownReference "Generator.WinRTAotSourceGenerator/GrxFirma.WinUI.Extra.WinRTVtable.g.cs"),
        (New-TypeFile -Namespace "GrxFirma.WinUI" -Type "Extra" -Details "GrxFirma_WinUI_NoExiste")
    )
    Assert-CanonicalThrows -Generated $unknownReference -Message "Se aceptó una cita a una clase que no existe"

    $foreignName = Join-Path $temporaryRoot "nombre-ajeno/WinRT.SourceGenerator"
    Write-GeneratorOutput -Directory $foreignName -Order $orderOne
    $foreignCcw = Join-Path $foreignName "Generator.WinRTAotSourceGenerator/WinRTCCWVtable.g.cs"
    $foreignText = [System.IO.File]::ReadAllText($foreignCcw).Replace(
        "GrxFirma_WinUI_Views_SignPage_SignPage_obj1_BindingsWinRTTypeDetails",
        "OtroNombreWinRTTypeDetails"
    )
    [System.IO.File]::WriteAllText($foreignCcw, $foreignText)
    foreach ($typeFile in Get-ChildItem -LiteralPath (Split-Path -Parent $foreignCcw) -Filter "*.WinRTVtable.g.cs") {
        $text = [System.IO.File]::ReadAllText($typeFile.FullName).Replace(
            "GrxFirma_WinUI_Views_SignPage_SignPage_obj1_BindingsWinRTTypeDetails",
            "OtroNombreWinRTTypeDetails"
        )
        [System.IO.File]::WriteAllText($typeFile.FullName, $text)
    }
    Assert-CanonicalThrows -Generated $foreignName -Message "Se aceptó una clase compartida sin el nombre de sus tipos"

    $badNamespace = Join-Path $temporaryRoot "espacio-raro/WinRT.SourceGenerator"
    Write-GeneratorOutput -Directory $badNamespace -Order $orderOne
    $badCcw = Join-Path $badNamespace "Generator.WinRTAotSourceGenerator/WinRTCCWVtable.g.cs"
    [System.IO.File]::WriteAllText($badCcw, "// cambio de formato`n" + [System.IO.File]::ReadAllText($badCcw))
    Assert-CanonicalThrows -Generated $badNamespace -Message "Se aceptó un WinRTCCWVtable.g.cs con otro formato"

    $targets = Get-Content -LiteralPath (Join-Path $windowsDirectory "../../cmd/gui-winui/Directory.Build.props") -Raw
    foreach ($marker in @(
        'BeforeTargets="CoreCompile"',
        "GrxFirmaCsWinRTCanonicalSources",
        "'%(Filename)' == 'WinRT.SourceGenerator'",
        '<Analyzer Remove="@(_GrxFirmaCsWinRTGenerator)" />',
        '<Compile Include="@(_GrxFirmaCanonicalCsWinRTSource)" />'
    )) {
        if (-not $targets.Contains($marker)) {
            throw "Falta en Directory.Build.props: $marker"
        }
    }
    $build = Get-Content -LiteralPath (Join-Path $windowsDirectory "build-desktop-winui.ps1") -Raw
    foreach ($marker in @(
        "cswinrt-canonical-sources.ps1",
        "-p:EmitCompilerGeneratedFiles=true",
        "ConvertTo-GrxFirmaCanonicalCsWinRTSources",
        "-p:GrxFirmaCsWinRTCanonicalSources=`$csWinRtSourceList"
    )) {
        if (-not $build.Contains($marker)) {
            throw "Falta en build-desktop-winui.ps1: $marker"
        }
    }
} finally {
    if (Test-Path -LiteralPath $temporaryRoot) {
        Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
    }
}

Write-Output "CsWinRT canonical sources tests passed."
