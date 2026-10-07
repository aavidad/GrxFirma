# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# Elección AutoFirma/GrxFirma para afirma://: el instalador solo deja el
# protocolo a AutoFirma si la persona lo eligió, AutoFirma sigue instalado y
# el registro es exactamente el de antes de GrxFirma.

$ErrorActionPreference = "Stop"

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

$windowsDir = Split-Path -Parent $PSScriptRoot
. (Join-Path $windowsDir "afirmauri-registration.ps1")

# Ejecutable registrado por AutoFirma (sin comillas, con espacios).
$cases = @(
    @('C:\Program Files\Autofirma\Autofirma\Autofirma.exe "%1"', 'C:\Program Files\Autofirma\Autofirma\Autofirma.exe'),
    @('"C:\Users\Ana\AppData\Local\Programs\GrxFirma\AfirmaURI\grxfirma-afirmauri.exe" "%1"', 'C:\Users\Ana\AppData\Local\Programs\GrxFirma\AfirmaURI\grxfirma-afirmauri.exe'),
    @('C:\dir.exe.d\prog.exe %1', 'C:\dir.exe.d\prog.exe'),
    @('', ''),
    @('"sin cierre', '')
)
foreach ($case in $cases) {
    $got = Get-AfirmaCommandExecutable -Command $case[0]
    Assert-True ($got -eq $case[1]) "Get-AfirmaCommandExecutable('$($case[0])') devolvio '$got'"
}

$protocolKey = "Software\GrxFirma\Tests\AfirmaURI-Choice"
$executablePath = "C:\Users\Test\AppData\Local\Programs\GrxFirma\AfirmaURI\grxfirma-afirmauri.exe"
$iconPath = "C:\Users\Test\AppData\Local\Programs\GrxFirma\AfirmaURI\grxfirma-grx.ico"
$plan = @(Get-AfirmaProtocolRegistrationPlan `
    -ProtocolKey $protocolKey `
    -ExecutablePath $executablePath `
    -IconPath $iconPath)

$tmp = Join-Path `
    ([System.IO.Path]::GetTempPath()) `
    ("grxfirma-afirma-choice-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $script:mockRegistry = @{}
    $script:mockPreference = "autofirma"
    $script:mockAutoFirma = "C:\Program Files\Autofirma\Autofirma\Autofirma.exe"

    function Get-AfirmaProtocolHandlerPreference { return $script:mockPreference }
    function Get-AfirmaAutoFirmaExecutable { param([string]$ProtocolKey) return $script:mockAutoFirma }
    function Get-AfirmaRegistryValueSnapshot {
        param([string]$Path, [string]$Name)
        $key = "$($Path.ToLowerInvariant())|$Name"
        if ($script:mockRegistry.ContainsKey($key)) {
            return $script:mockRegistry[$key]
        }
        return [pscustomobject]@{
            Path = $Path; Name = $Name; KeyExisted = $false
            ValueExisted = $false; Kind = $null; Value = $null
        }
    }

    # Instantánea de una instalación limpia: no había nada antes.
    $snapshotPath = Join-Path $tmp "afirma-protocol-snapshot.json"
    Write-AfirmaProtocolSnapshot `
        -Path $snapshotPath `
        -ProtocolKey $protocolKey `
        -OwnerValues @($plan | ForEach-Object { New-AfirmaOwnedValueSnapshot -Plan $_ }) `
        -Snapshots @($plan | ForEach-Object { New-AfirmaAbsentValueSnapshot -Plan $_ })

    function Test-Kept {
        return Test-AfirmaProtocolKeptForAutoFirma `
            -ProtocolKey $protocolKey `
            -ExecutablePath $executablePath `
            -IconPath $iconPath `
            -SnapshotPath $snapshotPath
    }

    # Registro retirado (vacío), AutoFirma elegido e instalado: se respeta.
    Assert-True (Test-Kept) "No se respeto la eleccion de AutoFirma con el registro retirado"

    # Sin preferencia o con GrxFirma elegido: el instalador registra.
    $script:mockPreference = ""
    Assert-True (-not (Test-Kept)) "Sin preferencia no debe dejarse a AutoFirma"
    $script:mockPreference = "grxfirma"
    Assert-True (-not (Test-Kept)) "Con GrxFirma elegido no debe dejarse a AutoFirma"
    $script:mockPreference = "autofirma"

    # AutoFirma desinstalado: GrxFirma vuelve a registrarse.
    $script:mockAutoFirma = ""
    Assert-True (-not (Test-Kept)) "Sin AutoFirma instalado no debe quedarse sin programa"
    $script:mockAutoFirma = "C:\Program Files\Autofirma\Autofirma\Autofirma.exe"

    # Registro de GrxFirma presente: no se considera retirado.
    foreach ($entry in $plan) {
        $script:mockRegistry["$(([string]$entry.Path).ToLowerInvariant())|$($entry.Name)"] =
            New-AfirmaOwnedValueSnapshot -Plan $entry
    }
    Assert-True (-not (Test-Kept)) "Con el registro de GrxFirma presente no hay nada que respetar"

    # Otro programa ocupó el registro después: no se respeta (el instalador
    # aplicará su comprobación de propiedad y se detendrá).
    $script:mockRegistry = @{}
    $script:mockRegistry["$(([string]$plan[3].Path).ToLowerInvariant())|"] = [pscustomobject]@{
        Path = $plan[3].Path; Name = ""; KeyExisted = $true; ValueExisted = $true
        Kind = "String"; Value = '"C:\Otro\x.exe" "%1"'
    }
    Assert-True (-not (Test-Kept)) "Un registro cambiado por otro programa no es una eleccion respetable"

    # Instantánea ausente o manipulada: no se respeta.
    $script:mockRegistry = @{}
    Remove-Item -LiteralPath $snapshotPath -Force
    Assert-True (-not (Test-Kept)) "Sin instantanea no debe dejarse a AutoFirma"
    [System.IO.File]::WriteAllText($snapshotPath, "{no json")
    Assert-True (-not (Test-Kept)) "Una instantanea invalida no debe aceptarse"
} finally {
    if (Test-Path -LiteralPath $tmp) {
        Remove-Item -LiteralPath $tmp -Recurse -Force
    }
}

# Contratos del instalador: comprueba la elección antes de registrar y la
# desinstalación no avisa cuando el registro ya estaba retirado.
$installer = Get-Content -LiteralPath (Join-Path $windowsDir "install-afirmauri.ps1") -Raw
Assert-True ($installer -match 'Test-AfirmaProtocolKeptForAutoFirma') "install-afirmauri.ps1 no consulta la eleccion de AutoFirma"
Assert-True ($installer -match '(?s)if \(\$keepAutoFirma\) \{.*?\} else \{\s*Install-AfirmaProtocolRegistration') "install-afirmauri.ps1 registra aunque se eligiera AutoFirma"
$uninstaller = Get-Content -LiteralPath (Join-Path $windowsDir "uninstall-afirmauri.ps1") -Raw
Assert-True ($uninstaller -match 'Test-AfirmaProtocolValuesMatch -Expected @\(\$state.Snapshots\)') "uninstall-afirmauri.ps1 no reconoce el registro ya retirado"

Write-Output "Windows afirma:// AutoFirma choice tests passed."
