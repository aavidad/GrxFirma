# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Fuentes de CsWinRT en forma canónica para que grxfirma-winui.dll sea
# reproducible. Este fichero se carga con «.» desde build-desktop-winui.ps1.
#
# El generador de CsWinRT (WinRT.SourceGenerator, Generator.WinRTAotSourceGenerator)
# recorre un ImmutableHashSet para escribir WinRTCCWVtable.g.cs. Su orden sale
# del hash de las cadenas, que .NET siembra al azar en cada proceso del
# compilador. Por eso, en cada compilación cambian dos cosas:
#   - el orden de las clases *WinRTTypeDetails, y con él el de los tipos en la dll;
#   - el nombre de las clases que comparten varios tipos con la misma vtable
#     (todas las *_obj1_Bindings de x:Bind y MainWindow, por ejemplo), que el
#     generador toma del primer tipo que encuentra.
# Esta función toma las fuentes que escribió el generador, da a cada clase
# compartida el nombre del menor de los tipos que la usan, ordena las clases
# y escribe la lista de ficheros que compila GrxFirma.WinUI en lugar del
# generador (ver cmd/gui-winui/Directory.Build.props). El código generado
# es el mismo: solo cambian el orden y el nombre de clases internas.

function ConvertTo-GrxFirmaCanonicalCsWinRTSources {
    [System.Diagnostics.CodeAnalysis.SuppressMessageAttribute(
        "PSUseShouldProcessForStateChangingFunctions",
        "",
        Justification = "Solo escribe en el directorio temporal del empaquetador."
    )]
    param(
        # Carpeta WinRT.SourceGenerator de CompilerGeneratedFilesOutputPath.
        [Parameter(Mandatory = $true)]
        [string]$GeneratedDirectory,
        [Parameter(Mandatory = $true)]
        [string]$DestinationDirectory,
        # Lista de ficheros, en el orden en que se compilan.
        [Parameter(Mandatory = $true)]
        [string]$ListPath
    )

    $source = Get-Item -LiteralPath $GeneratedDirectory -ErrorAction Stop
    if (-not $source.PSIsContainer) {
        throw "Las fuentes generadas de CsWinRT no son un directorio: $GeneratedDirectory"
    }
    if (Test-Path -LiteralPath $DestinationDirectory) {
        throw "El destino de las fuentes canónicas de CsWinRT ya existe: $DestinationDirectory"
    }

    $sourceRoot = $source.FullName.TrimEnd('\', '/')
    $texts = @{}
    $relativePaths = New-Object System.Collections.Generic.List[string]
    foreach ($file in Get-ChildItem -LiteralPath $sourceRoot -File -Recurse -Force) {
        $relative = $file.FullName.Substring($sourceRoot.Length).TrimStart('\', '/').Replace('\', '/')
        if (-not $relative.EndsWith(".cs", [System.StringComparison]::Ordinal)) {
            throw "Fichero inesperado entre las fuentes de CsWinRT: $relative"
        }
        # ReadAllText quita la marca BOM; se vuelve a poner al escribir.
        $texts[$relative] = [System.IO.File]::ReadAllText($file.FullName, [System.Text.Encoding]::UTF8)
        $relativePaths.Add($relative)
    }
    if ($relativePaths.Count -eq 0) {
        throw "CsWinRT no ha generado ninguna fuente en $sourceRoot"
    }
    $sortedPaths = $relativePaths.ToArray()
    [Array]::Sort($sortedPaths, [StringComparer]::Ordinal)

    $aotPrefix = "Generator.WinRTAotSourceGenerator/"
    $ccwPath = $aotPrefix + "WinRTCCWVtable.g.cs"
    $typeFileSuffix = ".WinRTVtable.g.cs"
    $identifier = "[A-Za-z_][A-Za-z0-9_]*"
    $renamed = 0

    if ($texts.ContainsKey($ccwPath)) {
        $ccw = $texts[$ccwPath]
        $newline = if ($ccw.Contains("`r`n")) { "`r`n" } else { "`n" }

        $namespaceMatch = [regex]::Match($ccw, "\A\s*namespace (WinRT\.$($identifier)VtableClasses)\s*\r?\n\{\r?\n")
        if (-not $namespaceMatch.Success) {
            throw "WinRTCCWVtable.g.cs no empieza por el espacio de nombres esperado de CsWinRT."
        }
        $vtableNamespace = $namespaceMatch.Groups[1].Value
        $footerMatch = [regex]::Match($ccw, "\r?\n\}\s*\z")
        if (-not $footerMatch.Success) {
            throw "WinRTCCWVtable.g.cs no termina cerrando su espacio de nombres."
        }
        $bodyStart = $namespaceMatch.Index + $namespaceMatch.Length
        $bodyEnd = $footerMatch.Index
        if ($bodyEnd -lt $bodyStart) {
            throw "WinRTCCWVtable.g.cs no tiene cuerpo."
        }
        $body = $ccw.Substring($bodyStart, $bodyEnd - $bodyStart)
        $classPattern = "(?m)^internal sealed class ($identifier)WinRTTypeDetails : global::WinRT\.IWinRTExposedTypeDetails\r?$"
        $classMatches = [regex]::Matches($body, $classPattern)
        if ($classMatches.Count -eq 0) {
            throw "WinRTCCWVtable.g.cs no contiene clases WinRTTypeDetails."
        }
        if ($body.Substring(0, $classMatches[0].Index).Trim().Length -ne 0) {
            throw "WinRTCCWVtable.g.cs tiene contenido inesperado antes de la primera clase."
        }

        # Bloques de clase: desde su declaración hasta la siguiente o el final.
        $blocks = @{}
        for ($index = 0; $index -lt $classMatches.Count; $index++) {
            $name = $classMatches[$index].Groups[1].Value
            $start = $classMatches[$index].Index
            $end = if ($index + 1 -lt $classMatches.Count) { $classMatches[$index + 1].Index } else { $body.Length }
            if ($blocks.ContainsKey($name)) {
                throw "Clase WinRTTypeDetails duplicada en WinRTCCWVtable.g.cs: $name"
            }
            $blocks[$name] = $body.Substring($start, $end - $start).TrimEnd()
        }

        # Qué tipos usa cada clase: el fichero de cada tipo la cita en
        # [WinRTExposedType]. El nombre del tipo sale del nombre del fichero
        # (Espacio.De.Nombres.Tipo.WinRTVtable.g.cs -> Espacio_De_Nombres_Tipo),
        # igual que lo forma el generador.
        $referencePattern = "global::" + [regex]::Escape($vtableNamespace) + "\.($identifier)WinRTTypeDetails\b"
        $users = @{}
        foreach ($name in $blocks.Keys) {
            $users[$name] = New-Object System.Collections.Generic.List[string]
        }
        foreach ($relative in $sortedPaths) {
            if ($relative -eq $ccwPath) {
                continue
            }
            foreach ($reference in [regex]::Matches($texts[$relative], $referencePattern)) {
                $name = $reference.Groups[1].Value
                if (-not $blocks.ContainsKey($name)) {
                    throw "$relative cita una clase que no está en WinRTCCWVtable.g.cs: $name"
                }
                $fileName = $relative.Substring($relative.LastIndexOf('/') + 1)
                if ($relative.StartsWith($aotPrefix, [System.StringComparison]::Ordinal) -and
                    $fileName.EndsWith($typeFileSuffix, [System.StringComparison]::Ordinal)) {
                    $typeName = $fileName.Substring(0, $fileName.Length - $typeFileSuffix.Length).Replace('.', '_')
                    if (-not $users[$name].Contains($typeName)) {
                        $users[$name].Add($typeName)
                    }
                }
            }
        }

        # Nombre canónico: el menor (ordinal) de los tipos que la usan. El
        # generador siempre la llama como uno de ellos; si no, algo ha cambiado
        # en CsWinRT y es mejor parar que adivinar.
        $canonicalNames = @{}
        $takenNames = @{}
        foreach ($name in $blocks.Keys) {
            $canonical = $name
            if ($users[$name].Count -gt 0) {
                if (-not $users[$name].Contains($name)) {
                    throw "La clase $($name)WinRTTypeDetails no lleva el nombre de ninguno de los tipos que la usan."
                }
                $candidates = $users[$name].ToArray()
                [Array]::Sort($candidates, [StringComparer]::Ordinal)
                $canonical = $candidates[0]
            }
            if ($takenNames.ContainsKey($canonical)) {
                throw "Dos clases WinRTTypeDetails quedarían con el mismo nombre: $canonical"
            }
            $takenNames[$canonical] = $true
            $canonicalNames[$name] = $canonical
            if ($canonical -ne $name) {
                $renamed++
            }
        }

        $renameReference = [System.Text.RegularExpressions.MatchEvaluator] {
            param($match)
            "global::$vtableNamespace.$($canonicalNames[$match.Groups[1].Value])WinRTTypeDetails"
        }
        foreach ($relative in $sortedPaths) {
            if ($relative -ne $ccwPath) {
                $texts[$relative] = [regex]::Replace($texts[$relative], $referencePattern, $renameReference)
            }
        }

        $orderedBlocks = New-Object 'System.Collections.Generic.SortedDictionary[string,string]' ([StringComparer]::Ordinal)
        foreach ($name in $blocks.Keys) {
            $canonical = $canonicalNames[$name]
            $declaration = New-Object System.Text.RegularExpressions.Regex(
                "\Ainternal sealed class $($identifier)WinRTTypeDetails(?= : global::WinRT\.IWinRTExposedTypeDetails)"
            )
            if (-not $declaration.IsMatch($blocks[$name])) {
                throw "No se encuentra la declaración de $($name)WinRTTypeDetails."
            }
            $block = $declaration.Replace(
                $blocks[$name],
                "internal sealed class $($canonical)WinRTTypeDetails",
                1
            )
            $orderedBlocks.Add($canonical, $block)
        }
        $builder = New-Object System.Text.StringBuilder
        [void]$builder.Append("namespace $vtableNamespace$newline{$newline")
        foreach ($block in $orderedBlocks.Values) {
            [void]$builder.Append($block).Append($newline).Append($newline)
        }
        [void]$builder.Append("}$newline")
        $texts[$ccwPath] = $builder.ToString()
    }

    New-Item -ItemType Directory -Force -Path $DestinationDirectory | Out-Null
    $destinationRoot = (Get-Item -LiteralPath $DestinationDirectory).FullName.TrimEnd('\', '/')
    $encoding = New-Object System.Text.UTF8Encoding($true)
    $listed = New-Object System.Collections.Generic.List[string]
    foreach ($relative in $sortedPaths) {
        $target = Join-Path $destinationRoot ($relative.Replace('/', [System.IO.Path]::DirectorySeparatorChar))
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
        [System.IO.File]::WriteAllText($target, $texts[$relative], $encoding)
        $listed.Add($target)
    }
    [System.IO.File]::WriteAllLines($ListPath, $listed.ToArray(), (New-Object System.Text.UTF8Encoding($false)))

    return [pscustomobject]@{
        Files = $listed.Count
        Renamed = $renamed
    }
}
