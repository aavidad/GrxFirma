<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Captura QA de ventanas GrxFirma en Windows 10

Estas herramientas compilan, abren y capturan las interfaces Qt/QML o WinUI 3
de GrxFirma sin tomar una captura del escritorio.

## Salvaguardas

- Solo se enumeran ventanas cuya identidad pertenece al ejecutable GrxFirma
  lanzado por el script o a descendientes de su linaje. La identidad estable
  combina PID, instante de creación, ruta canónica directa dentro de la stage y
  SHA-256 de un nombre cerrado; se revalida antes y después de cada captura. En
  WinUI el proceso raíz es el launcher Go, que crea el canal local y entrega a
  la GUI el PID que esta debe verificar.
- Cada imagen se obtiene sobre un HWND concreto y ya validado. El modo
  automático usa `Windows.Graphics.Capture` con `CreateForWindow(HWND)` para
  WinUI y `user32.PrintWindow` para Qt. La primera API conserva el contenido
  compuesto de WinUI que `PrintWindow` puede devolver en blanco.
- En WinUI no basta con producir un PNG válido: el helper analiza únicamente
  el interior de la ventana, descarta frames prácticamente uniformes y
  reintenta durante un máximo de cinco segundos sobre el mismo HWND, PID y
  dimensiones. Si el cliente no llega a renderizarse, el manifiesto registra
  `window-content-not-ready` y la ejecución sin capturas útiles falla.
- Si se fuerza `PrintWindow`, se aplica el mismo umbral al bitmap del HWND. Un
  cliente uniforme produce `print-window-content-not-ready`; esta ruta de
  diagnóstico tampoco puede declarar éxito por una PNG blanca. El PNG se
  escribe primero con nombre temporal y solo se publica mediante un movimiento
  atómico si HWND e identidad del proceso siguen siendo los esperados.
- Antes de invocar ese helper, el flujo WinUI restaura y activa de forma
  acotada únicamente el HWND/PID ya validado. UI Automation debe observar
  durante tres muestras consecutivas un árbol estable con controles
  accionables reales; el manifiesto guarda sólo sus recuentos, nunca nombres o
  textos. Un árbol ausente, incompleto o inestable produce una omisión
  `winui-*-not-ready` y no una captura blanca aceptada.
- No hay captura de monitor, selector de pantalla, `BitBlt`,
  `CopyFromScreen` ni acceso al DC del escritorio. El helper gráfico no ofrece
  `CreateForMonitor`: exige el HWND, PID y dimensiones ya observados y los
  revalida antes y después de obtener el frame.
- Se omiten ventanas minimizadas, títulos con términos sensibles y ventanas
  donde UI Automation detecta un control de contraseña. Si UI Automation no
  puede inspeccionar una ventana, se omite.
- Se omiten los diálogos comunes nativos de Windows (`#32770`), como los
  selectores de fichero, porque pueden mostrar rutas, accesos rápidos o
  documentos ajenos a los datos sintéticos.
- Es obligatorio declarar `-TestDataOnly`. Usa documentos, certificados y
  identidades sintéticos: una captura visual nunca puede garantizar que un
  dato real mostrado accidentalmente deje de ser visible.
- El manifiesto no guarda títulos de ventana ni metadatos derivados de ellos,
  líneas de comando, variables de entorno, nombre de usuario ni nombre del
  equipo.
- El directorio de cada ejecución permite acceso únicamente al usuario actual
  y a `SYSTEM`; la herramienta verifica la DACL antes de escribir imágenes.
- Las rutas de artefactos deben estar en un volumen local y no pueden atravesar
  puntos de reanálisis. Se rechazan rutas UNC, unidades de red mapeadas,
  enlaces y junctions.
- Se conservan como máximo 20 ejecuciones completas por defecto. Solo se
  eliminan directorios con nombre de ejecución, manifiesto válido de esta
  herramienta y sin puntos de reanálisis; cada candidato se vuelve a validar
  inmediatamente antes de borrarlo y cualquier error impide su borrado.
- Las capturas idénticas consecutivas de una ventana se deduplican. Por defecto
  se detiene al alcanzar 250 imágenes únicas o 512 MiB.
- Con `-CloseAfterCapture` se cierran también los descendientes GrxFirma del
  launcher. Antes de actuar se vuelven a comprobar PID, inicio, nombre cerrado,
  ruta y hash dentro de la misma stage; no se finalizan otras instancias del
  equipo.
- Las imágenes y `manifest.json` se escriben, por defecto, en:

  ```text
  %LOCALAPPDATA%\GrxFirma\QA\window-captures\<timestamp-UTC>-<id>\
  ```

  Esa ruta está físicamente fuera del repositorio. La compilación usa la stage
  existente bajo `release/windows-desktop-qml/` o
  `release/windows-desktop-winui/`, ambas excluidas por `.gitignore`.

No uses esta herramienta con contraseñas, PIN, tokens, certificados personales
ni documentos reales. Las ventanas omitidas aparecen en el manifiesto solo con
un motivo técnico y sin su título.

## Requisitos

- Windows 10 o posterior.
- PowerShell 7 (`pwsh.exe`).
- Una sesión gráfica iniciada y desbloqueada.
- Para compilar Qt: Qt 6, `qmake`, `windeployqt` y `mingw32-make` o `nmake`,
  según documenta `packaging/windows/README_DESKTOP_QML_WINDOWS.md`.
- Para compilar WinUI: .NET SDK y Windows App SDK fijados por el proyecto,
  según `packaging/windows/README_DESKTOP_WINUI_WINDOWS.md`.
- Para iniciar desde SSH, la cuenta SSH debe ser la misma que mantiene abierta
  la sesión gráfica. No se piden ni almacenan credenciales para otra cuenta.
- La captura por HWND de WinUI requiere Windows 10 1903 (build 18362) o
  posterior. La aplicación conserva 1809 como mínimo técnico; en 1809 el
  capturador puede usar `-CaptureMethod PrintWindow`, aunque esa vía puede
  dejar en blanco el contenido compuesto de WinUI.

## Ejecución local interactiva

Desde la raíz del worktree:

```powershell
pwsh -NoProfile -File .\scripts\windows-qa\Invoke-GrxFirmaWindowCapture.ps1 `
  -TestDataOnly `
  -DurationSeconds 30 `
  -IntervalMilliseconds 1000 `
  -CloseAfterCapture
```

El comando compila `desktop-qml`, abre `grxfirma-gui-qml.exe`, captura sus
ventanas durante 30 segundos y devuelve la ruta de la ejecución. Durante ese
intervalo se pueden abrir manualmente diálogos seguros de la aplicación.

Para WinUI, el mismo flujo selecciona el empaquetador nativo y arranca la GUI
a través del launcher Go:

```powershell
pwsh -NoProfile -File .\scripts\windows-qa\Invoke-GrxFirmaWindowCapture.ps1 `
  -Frontend WinUI `
  -CaptureMethod Auto `
  -TestDataOnly `
  -DurationSeconds 30 `
  -CloseAfterCapture
```

En `Auto`, WinUI selecciona `WindowsGraphicsCapture` y Qt selecciona
`PrintWindow`. Se puede forzar uno de los dos métodos únicamente para
diagnóstico:

```powershell
pwsh -NoProfile -File .\scripts\windows-qa\Invoke-GrxFirmaWindowCapture.ps1 `
  -Frontend WinUI `
  -CaptureMethod WindowsGraphicsCapture `
  -TestDataOnly `
  -DurationSeconds 20
```

El helper `GrxFirma.WindowCapture` se restaura con lockfile, se compila con
el SDK .NET fijado por `global.json` dentro del directorio protegido de la
ejecución y se retira al terminar. No se instala ni se conserva como componente
del producto.

Para reutilizar una build existente:

```powershell
pwsh -NoProfile -File .\scripts\windows-qa\Invoke-GrxFirmaWindowCapture.ps1 `
  -TestDataOnly `
  -SkipBuild `
  -DurationSeconds 20
```

Sin `-CloseAfterCapture`, la aplicación se deja abierta. Si se indica
`-OutputRoot`, el script rechaza rutas dentro del worktree, UNC o con puntos de
reanálisis, incluidas unidades de red mapeadas. Los límites se pueden reducir
con `-MaxCaptures`, `-MaxArtifactBytes` y `-MaxRetainedRuns`.

## Ejecución desde SSH en la sesión gráfica

Con el mismo usuario que tiene abierta y desbloqueada la sesión de Windows:

```powershell
ssh usuario@equipo
cd C:\src\GrxFirma
pwsh -NoProfile -File .\scripts\windows-qa\Start-GrxFirmaWindowCaptureTask.ps1 `
  -Frontend WinUI `
  -CaptureMethod Auto `
  -TestDataOnly `
  -DurationSeconds 30 `
  -IntervalMilliseconds 1000 `
  -CloseAfterCapture
```

El lanzador:

1. comprueba que la cuenta SSH coincide con el usuario gráfico;
2. compila en el propio proceso SSH con el toolchain correspondiente a Qt o
   WinUI;
3. registra una tarea manual, sin trigger presente ni futuro, de nivel limitado
   con logon `Interactive`;
4. inicia la tarea inmediatamente en esa sesión usando `-SkipBuild`;
5. espera el puntero de resultado y detecta si la tarea termina sin completarlo;
6. muestra las rutas de la ejecución y del manifiesto;
7. detiene y elimina la tarea también ante error o timeout.

La acción programada solo contiene rutas y parámetros de QA, nunca
credenciales. Con `-NoWait`, el comando devuelve el control tras iniciar la
tarea y conserva su definición; imprime la ruta del puntero y el comando exacto
para retirarla. `-KeepTask` conserva una tarea ya finalizada para diagnóstico.
Estas dos opciones son las únicas que evitan la limpieza automática.
Una espera normal elimina su puntero de coordinación al terminar. Los punteros
que deben conservarse para `-NoWait` o `-KeepTask` se limitan a 50 por defecto
(`-MaxRetainedTaskResults`); solo se podan ficheros con nombre, esquema,
identificador y marca de herramienta válidos, revalidados antes del borrado.

La tarea gráfica no hereda la configuración temporal de la sesión SSH. Por
ello, la compilación se hace antes de registrarla. Prepara el toolchain del
frontend elegido en la terminal SSH, o usa `-SkipBuild` con un stage ya
validado.

## Certificado sintético de firma

`Manage-GrxFirmaSyntheticSigningCertificate.ps1` prepara material efímero
para probar la selección y la firma local sin usar un certificado personal. Es
un helper no interactivo para Windows 10 y PowerShell 7; no genera ni exporta
un PFX.

```powershell
$certificate = pwsh -NoProfile -NonInteractive -File `
  .\scripts\windows-qa\Manage-GrxFirmaSyntheticSigningCertificate.ps1 `
  -Action Create | ConvertFrom-Json
$certificate.state
$certificate.thumbprint
```

La creación es idempotente. Si el certificado inventariado sigue siendo válido,
devuelve `already-present`; en otro caso solo reemplaza el certificado si antes
puede demostrar que coincide con la identidad sintética inventariada.

El certificado:

- es RSA de 2048 bits, se firma a sí mismo con SHA-256 y caduca aproximadamente en 48
  horas;
- contiene únicamente uso de clave `DigitalSignature`, `CA=false` y el EKU de
  firma de documentos;
- solicita a Windows una clave no exportable y detecta como error una clave que
  el proveedor declare exportable;
- se instala exclusivamente en `Cert:\CurrentUser\My`;
- no se añade a `Root`, `CA`, `TrustedPublisher` ni a ningún almacén de máquina,
  y no se usa para Authenticode.

La clave se crea directamente en el proveedor CNG de software de Windows para
el usuario actual y el certificado resultante se añade de forma explícita a
`My`. No se usa `New-SelfSignedCertificate`: en Windows 10 ese cmdlet puede
dejar una copia pública adicional del grxfirmado en `CurrentUser\CA`, lo que
ampliaría innecesariamente el estado modificado por la prueba.

El helper guarda la huella del certificado, el identificador aleatorio y el
SHA-256 del `SubjectPublicKeyInfo` (SPKI) en:

```text
%LOCALAPPDATA%\GrxFirma\QA\synthetic-signing-certificate\inventory.json
```

El directorio y el fichero desactivan la herencia y permiten acceso solo al
usuario actual y a `SYSTEM`. Las rutas UNC, unidades no fijas y puntos de
reanálisis se rechazan. El inventario no permite elegir almacén ni ruta desde
la línea de comandos. La salida JSON no expone la ruta absoluta del inventario;
las pruebas que necesiten inspeccionarlo usan la ubicación fija anterior.

Durante la creación también se mantiene
`synthetic-signing-certificate\recovery.json`. Se escribe de forma atómica y
con los mismos permisos antes de crear la clave, después de ligar su SHA-256
SPKI, después de guardar el certificado y después de escribir el inventario.
Una ejecución posterior completa o revierte el estado interrumpido únicamente
si puede validar esa identidad pública y el contrato criptográfico. Al
completar la creación se retira el diario.

Esta recuperación reduce los residuos ante errores y cierres entre fases, pero
no promete recuperación absoluta ante una terminación forzada en el instante
entre la creación de la clave CNG y el registro de su SPKI. Si queda una clave
en esa ventana, el helper se niega a borrarla automáticamente porque solo
dispone del nombre; conserva el diario para investigación y requiere
intervención manual.

Comprueba el estado sin cambiar el almacén:

```powershell
pwsh -NoProfile -NonInteractive -File `
  .\scripts\windows-qa\Manage-GrxFirmaSyntheticSigningCertificate.ps1 `
  -Action Status
```

Retíralo al terminar, incluso si la prueba funcional falla:

```powershell
pwsh -NoProfile -NonInteractive -File `
  .\scripts\windows-qa\Manage-GrxFirmaSyntheticSigningCertificate.ps1 `
  -Action Remove
```

`Remove` es idempotente y llama al proveedor de certificados con `-DeleteKey`.
Solo elimina la huella de `CurrentUser\My` registrada en el inventario después
de volver a comprobar sujeto, emisor, clave privada, algoritmos, restricciones
de CA y EKU. Si el certificado ya no está, la clave CNG solo se elimina tras
comprobar que su proveedor, ámbito de usuario, algoritmo, tamaño, uso,
exportabilidad y SHA-256 SPKI coinciden con el inventario. Nunca se borra una
clave solo por su nombre. Un inventario manipulado o una identidad distinta
producen un error en vez de ampliar el borrado.

Este certificado no representa una identidad reconocida ni una cadena de
confianza real. Sirve para probar el flujo criptográfico local y los mensajes
de la aplicación, pero no permite completar portales que exijan un certificado
emitido por una autoridad admitida. Tampoco sustituye la firma Authenticode de
los binarios de distribución.

La existencia y las pruebas de política/ciclo de vida de este arnés no
constituyen una campaña de producto. El acta final debe registrar el commit,
los hashes del instalador, la versión de Windows 10 y la eliminación confirmada
del material sintético.

## Prueba funcional de firma WinUI

`Invoke-GrxFirmaWinUiSigningSmoke.ps1` recorre la interfaz WinUI con un
documento de texto sintético y el certificado anterior. La aplicación debe
estar ya abierta desde el `grxfirma-gui.exe` de la stage
`windows-desktop-winui`; el controlador y la aplicación deben ejecutarse en la
misma sesión gráfica. No se debe ejecutar en una sesión compartida con una
persona u otra automatización que esté usando GrxFirma.

```powershell
$root = (Resolve-Path .).Path
$qa = Join-Path $env:LOCALAPPDATA "GrxFirma\QA\winui-signing-smoke"
$run = [DateTimeOffset]::UtcNow.ToString("yyyyMMddTHHmmssfffZ") +
  "-" + [guid]::NewGuid().ToString("N").Substring(0, 8)
$result = Join-Path $qa "$run.json"

pwsh -NoProfile -NonInteractive -File `
  .\scripts\windows-qa\Invoke-GrxFirmaWinUiSigningSmoke.ps1 `
  -RepositoryRoot $root `
  -TestDataOnly `
  -ResultPath $result `
  -TimeoutSeconds 300
```

Para una ejecución desatendida en una sesión gráfica reservada para QA,
`Invoke-GrxFirmaWinUiSigningSmokeTask.ps1` puede abrir el candidato exacto,
ejecutar el controlador anterior y cerrar únicamente los procesos que ella
misma haya creado:

```powershell
$app = Join-Path `
  $root `
  "release\windows-desktop-winui\GrxFirma-0.0.90-desktop-winui-windows-amd64\app"
$diagnostic = Join-Path $qa "$run.diagnostic.txt"

pwsh -NoProfile -NonInteractive -File `
  .\scripts\windows-qa\Invoke-GrxFirmaWinUiSigningSmokeTask.ps1 `
  -RepositoryRoot $root `
  -ApplicationDirectory $app `
  -ResultPath $result `
  -DiagnosticPath $diagnostic `
  -LaunchApplication `
  -TimeoutSeconds 300
```

La tarea se niega a arrancar si detecta cualquier backend, frontend WinUI o
frontend Qt/QML de GrxFirma en la sesión. Además mantiene un mutex local
durante toda la prueba para impedir dos ejecuciones simultáneas del mismo
arnés. Al limpiar identifica sus propios procesos por ruta completa y hora de
inicio, de modo que nunca reutiliza ni cierra una instancia preexistente. Usa
argumentos PowerShell legibles y no emplea `-EncodedCommand` ni
`WScript.Shell`. Aunque incluya estas salvaguardas, solo debe lanzarse cuando
la sesión Windows esté libre.

El mismo controlador admite tres orígenes de certificado:

- `SystemStore`: certificado ya presente en el contenedor del usuario;
- `TemporaryFile`: P12/PFX oficial de QA usado solo durante la sesión;
- `WindowsImport`: importación explícita del P12/PFX oficial de QA al
  contenedor de Windows.

Las dos variantes con fichero exigen `-OfficialFnmtCredentialPath` y solo
aceptan `fnmt-test.p12` dentro del directorio privado fijo
`%LOCALAPPDATA%\GrxFirma\QA\fnmt-official-test`. Antes de abrirlo verifican
su SHA-256 esperado; no enumeran `Descargas` ni buscan credenciales personales.
El diálogo de contraseña puede ser el Win32 propio o el respaldo CredUI del
sistema, y el controlador solo escribe la contraseña pública del certificado
oficial de pruebas en un campo que Windows marque como contraseña.

El flujo selecciona el documento, elige exclusivamente el certificado de QA
declarado para la ejecución, crea una firma CAdES detached, comprueba la
validación posterior de la página de firma, abre la sección de verificación,
selecciona la firma y el original y verifica de nuevo. En ventanas estrechas
abre primero el panel compacto de navegación.

Con `-VisibleSealPades` usa el PDF sintético versionado y bloqueado por
SHA-256, activa el sello PAdES, carga su previsualización real, arrastra la
zona, usa el tirador para cambiar el tamaño y selecciona una rotación de
`0`, `90`, `180` o `270` grados mediante `-VisibleSealRotation`. La ejecución
solo puede terminar correctamente si los campos accesibles confirman los
cambios, se obtiene una captura del HWND WinUI con
`Windows.Graphics.Capture`, se crea el PDF firmado y la verificación embebida
declara su integridad válida:

```powershell
pwsh -NoProfile -NonInteractive -File `
  .\scripts\windows-qa\Invoke-GrxFirmaWinUiSigningSmokeTask.ps1 `
  -RepositoryRoot $root `
  -ApplicationDirectory $app `
  -ResultPath $result `
  -DiagnosticPath $diagnostic `
  -LaunchApplication `
  -CertificateSource TemporaryFile `
  -OfficialFnmtCredentialPath `
    "$env:LOCALAPPDATA\GrxFirma\QA\fnmt-official-test\fnmt-test.p12" `
  -CertificateNameContains "EIDAS CERTIFICADO PRUEBAS - 99999999R" `
  -CertificateEvidenceLabel "fnmt-official-test-99999999R" `
  -VisibleSealPades `
  -VisibleSealRotation 90 `
  -TimeoutSeconds 600
```

La imagen se limita al HWND ya ligado al PID, ruta, instante de creación y
SHA-256 del ejecutable. No usa una captura del monitor ni puede incluir
selectores nativos o ventanas superpuestas. El helper se compila en el
directorio privado de la ejecución y se retira inmediatamente después.

Una tarea interactiva puede no recibir permiso de Windows para inyectar un
arrastre global aunque UI Automation sí pueda operar los controles. En el modo
normal el arnés intenta el puntero y, si la geometría no cambia, usa los campos
numéricos accesibles como respaldo y lo registra en
`visibleSealNumericFallbackUsed`. Para una comprobación dedicada del gesto
real, `-ExternalPointerInputConfirmation` detiene la ejecución en
`awaiting-external-pointer-move` y `awaiting-external-pointer-resize`; un
controlador externo de la consola gráfica debe realizar cada gesto y crear las
señales privadas que solicita el resultado. Este modo no es necesario para la
prueba instalada normal y no debe activarse sin control exclusivo de la
sesión.

Los selectores nativos reciben la ruta mediante entrada Unicode y se confirman
con un clic nativo sobre el botón accesible real. No se usa
`ValuePattern.SetValue` para escribir ni `InvokePattern` para cerrar
`PickerHost`: esa combinación puede producir
`RPC_E_CANTCALLOUT_ININPUTSYNCCALL` cuando WinUI intenta recuperar el
`StorageFile`. `-ManualSaveConfirmation` queda como modo diagnóstico de
respaldo, no como requisito de la prueba normal.

En Windows 10, CredUI puede retirar el contenido modal y conservar su HWND
marcado como visible. Tras aceptar la contraseña QA, el arnés considera cerrado
el diálogo cuando deja de estar expuesto por UI Automation; así no agota el
tiempo esperando un handle residual, pero tampoco continúa mientras siga
visible el campo de contraseña.

Al terminar con éxito, el JSON debe indicar:

```text
status = succeeded
signatureCreated = true
postValidationObserved = true
independentVerificationObserved = true
protectedEvidenceCopied = true
temporaryOutputRemoved = true
```

La firma que eligió el selector se borra de `Documentos`. Se conserva una copia
íntegra y privada en:

```text
%LOCALAPPDATA%\GrxFirma\QA\winui-signing-smoke\<run>\
  documento-sintetico-firmado.p7s
```

En el modo PAdES se conservan
`documento-sintetico-firmado.pdf` y
`sello-visible-configurado.png` en el mismo directorio privado.

Por ello, no se debe volver a pulsar «Verificar firma» en la ventana que queda
abierta después de finalizar: sus campos todavía muestran la ruta temporal ya
retirada. Para repetir la operación se inicia una ejecución nueva.

## Inventario de accesibilidad con UI Automation

`Invoke-GrxFirmaUiaAccessibilityAudit.ps1` abre GrxFirma, pasa por cada
sección del menú y escribe en `-OutputFile` los controles sin nombre, los
objetivos de menos de 24×24 px, los que no reciben el foco del teclado, los
encabezados y los campos obligatorios, con una captura por sección en
`-ScreenshotDirectory`. Con `-UiBinary` se prueba una compilación WinUI sin
instalarla. Se lanza como tarea programada interactiva, igual que la captura.
Si la sesión está bloqueada no recorre el foco con Tab. El método y los
resultados están en `docs/ACCESIBILIDAD.md`.

## Navegadores

Este capturador no toma imágenes de Chrome ni Firefox: su lista de procesos
está cerrada a GrxFirma. Las pruebas de navegador deben usar automatización
separada, perfiles exclusivos de QA y únicamente certificados sintéticos. No
se debe ampliar la lista de procesos de este script para capturar navegadores.

## Artefactos

Cada ejecución contiene:

- `images/*.png`: contenido de ventanas GrxFirma aceptadas por las
  salvaguardas;
- `manifest.json`: timestamps UTC, commit, indicador de worktree sucio, hash
  del ejecutable, dimensiones, PID/nombre del proceso, método por HWND, hash de
  cada PNG, deduplicación, cuotas, omisiones y fallos.

Verificación rápida:

```powershell
$run = Get-ChildItem "$env:LOCALAPPDATA\GrxFirma\QA\window-captures" `
  -Directory | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
$manifest = Get-Content (Join-Path $run.FullName "manifest.json") -Raw |
  ConvertFrom-Json
$manifest.counts
$manifest.captures | Format-Table capturedAtUtc, processName, status, reason, file
```

## Validación estática

Puede ejecutarse en Windows, Linux o macOS con PowerShell 7:

```powershell
pwsh -NoProfile -File .\scripts\windows-qa\Test-GrxFirmaWindowCapturePolicy.ps1
pwsh -NoProfile -File .\scripts\windows-qa\Test-GrxFirmaSyntheticSigningCertificatePolicy.ps1
pwsh -NoProfile -File .\scripts\ci\test-powershell-syntax.ps1
```

La primera validación impide introducir APIs de captura de escritorio en el
capturador y exige las salvaguardas de proceso, privacidad y tarea interactiva.
La segunda fija el almacén, identidad, criptografía, permisos, inventario,
diario de recuperación y borrado ligado al SPKI del material sintético.

La prueba funcional del ciclo de vida solo se ejecuta en Windows. Crea o
reutiliza el certificado inventariado, verifica la idempotencia, confirma que
la misma huella no está en almacenes de confianza o de máquina y comprueba que
`Remove` borra también la clave CNG. Su bloque `finally` vuelve a invocar el
helper normal y no contiene una vía alternativa de borrado por nombre; por ello
no debe ejecutarse mientras otra prueba esté usando el mismo material:

```powershell
pwsh -NoProfile -NonInteractive -File `
  .\scripts\windows-qa\Test-GrxFirmaSyntheticSigningCertificateLifecycle.ps1
```
