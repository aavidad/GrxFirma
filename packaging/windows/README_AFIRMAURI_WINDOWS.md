<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma handler afirma:// para Windows

Este paquete contiene el manejador protocolario `afirma://` para Windows.

## Qué incluye

- `grxfirma-afirmauri.exe`
- `install-afirmauri.ps1`
- `uninstall-afirmauri.ps1`
- `install-path-safety.ps1`
- `invoke-uninstall-silent.ps1`
- `README_AFIRMAURI_WINDOWS.md`
- `VERSION.txt`

## Para qué sirve

Permite registrar en Windows el esquema `afirma://` para que portales y sedes electrónicas puedan invocar GrxFirma.

En la práctica:

- el navegador o el sistema abre una URI `afirma://...`;
- Windows entrega la URI al manejador registrado;
- `grxfirma-afirmauri.exe` ejecuta el flujo protocolario y dialoga con el portal remoto.

## Instalación

La entrega independiente se llama `GrxFirma-0.0.90-afirmauri-windows-amd64.zip`
o `GrxFirma-0.0.90-afirmauri-windows-amd64-setup.exe`.

Desde PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-afirmauri.ps1
```

El script:

- copia `grxfirma-afirmauri.exe` a `%LOCALAPPDATA%\Programs\GrxFirma\AfirmaURI`;
- borra el `afirmauri-handler.cmd` que dejaban versiones anteriores (reenviaba
  la URI a `cmd.exe` y no lo usaba nadie);
- registra `HKCU\Software\Classes\afirma` para que la URI `afirma://` abra
  directamente `grxfirma-afirmauri.exe` con `"%1"`.

Para desinstalar manualmente y retirar también el registro por usuario:

```powershell
.\uninstall-afirmauri.ps1
```

El instalador NSIS es completamente por usuario, no eleva privilegios y deja
su copia de mantenimiento en
`%LOCALAPPDATA%\Programs\GrxFirma\AfirmaURI`. El desinstalador ejecuta
primero la limpieza de la copia operativa y conserva el propio desinstalador si
esa operación falla.

El modo `setup.exe /S` no abre cuadros de diálogo que puedan quedar ocultos:
un fallo devuelve `1603`. La entrada registrada incluye
`QuietUninstallString` para despliegues gestionados; usa una copia temporal con
ACL exclusiva, propaga el código de salida y rechaza enlaces, junctions y otros
reparse points antes de borrar la copia de mantenimiento.

Las versiones anteriores del NSIS se instalaban por equipo y escribían en
`HKLM`. El instalador actual comprueba las vistas de registro de 32 y 64 bits
antes de copiar nada. Si encuentra aquella instalación del handler o de la
Suite, se detiene y pide retirarla primero desde **Configuración > Aplicaciones**
aceptando el aviso UAC, o ejecutando el `uninstall.exe` antiguo como
administrador. Después puede instalarse normalmente la edición por usuario.

## Convivencia con AutoFirma

GrxFirma y AutoFirma (la aplicación Java del Gobierno) pueden estar instalados
a la vez. AutoFirma registra `afirma://` para todo el equipo en
`HKLM\Software\Classes\afirma`; GrxFirma lo registra para el usuario en
`HKCU\Software\Classes\afirma`, que tiene prioridad.

AutoFirma no siempre queda en `HKLM`. Si se instaló solo para el usuario, o
se instaló o actualizó con la misma cuenta después de GrxFirma, Windows guarda
sus valores en `HKCU\Software\Classes\afirma`, porque esa clave ya existe. Por
eso GrxFirma busca AutoFirma en tres sitios: la orden que había en `HKCU`
antes de GrxFirma (guardada en la instantánea), la de `HKLM` y la que hay ahora
en `HKCU` si no es la suya. Hasta la 0.0.125 solo miraba `HKLM` y, en esos
equipos, decía que AutoFirma no estaba instalado y no dejaba elegirlo.

En **Configuración > Programa para firmar en las webs** (WinUI y Qt) se elige
cuál abre las firmas de los portales:

- **GrxFirma**: vuelve a registrar HKCU con la misma instantánea
  (`afirma-protocol-snapshot.json`).
- **AutoFirma**: el motor retira el registro HKCU de GrxFirma y restaura lo que
  hubiera antes de GrxFirma (normalmente nada, y Windows usa el de AutoFirma;
  si AutoFirma estaba en `HKCU`, vuelve su registro). Solo se ofrece si en
  alguno de los tres sitios hay una orden de `afirma://` con un
  `Autofirma.exe` que existe.
- **Preguntar cada vez**: GrxFirma sigue registrado y, en cada firma, pregunta
  «¿Con qué programa quiere firmar en esta web?»; se puede marcar «Recordar mi
  elección». Cerrar el diálogo equivale a firmar con GrxFirma sin recordar
  nada. También necesita AutoFirma instalado.

El motor lo hace con la API del registro, sin PowerShell, y con las mismas
comprobaciones de propiedad que el instalador: si otro programa ha cambiado el
registro, no toca nada. La elección se guarda en
`HKCU\Software\GrxFirma\AfirmaProtocolHandler` (`grxfirma`, `autofirma` o
`preguntar`); una actualización la respeta mientras AutoFirma siga instalado y
el registro siga retirado.

Cuando GrxFirma recibe un enlace y debe abrirlo AutoFirma (porque se eligió en
el diálogo o en Configuración, o porque lo fija la política), se lo pasa al
`Autofirma.exe` registrado:
el de la instantánea o el de `HKLM`, nunca uno que diga el enlace. Lo lanza sin
intérprete de órdenes, con el enlace como un único argumento, rechaza enlaces
con comillas, espacios o caracteres de control y no se reenvía a sí mismo. El
registro de actividad anota el reenvío sin el enlace. Si no puede, pregunta si
firmar con GrxFirma o cancelar; nunca se reenvía sin que la persona lo haya
elegido.

### Al instalar

- Instalación interactiva: si AutoFirma está instalado y la política no lo
  fija, la página «Programa para firmar en las webs» pregunta con qué programa
  firmar cuando una web lo pida. Por defecto marca lo que ya había; si no se
  cambia, no se toca nada.
- Instalación silenciosa: se mantiene lo que eligió la persona o, si no eligió
  nada, GrxFirma, como hasta ahora.
- `/AFIRMA=grxfirma`, `/AFIRMA=autofirma` o `/AFIRMA=preguntar` lo fija desde
  la línea de órdenes, también con `/S`; un valor no válido termina con el
  código 87 sin modificar el sistema. `install-afirmauri.ps1` e
  `install-suite.ps1` aceptan `-AfirmaHandler` con los mismos valores.
- La política `manejador_afirma` de `HKLM\SOFTWARE\Policies\GrxFirma`
  prevalece sobre todo lo anterior y nunca cambia la preferencia de la persona,
  así que si se retira vuelve a valer lo que cada uno eligió. Si no se puede
  leer, el instalador avisa y mantiene la elección de cada usuario.
- Si AutoFirma se reinstaló con la misma cuenta después de GrxFirma y ahora
  atiende `afirma://`, una actualización sin `/AFIRMA=` avisa y no lo
  sobrescribe. Con `/AFIRMA=grxfirma`, la regla de propiedad hace que el
  instalador se detenga sin escribir. Para volver a GrxFirma hay que retirar
  antes ese registro de AutoFirma, por ejemplo desinstalándolo.

Puertos: el portal elige al azar uno o varios puertos (de 49152 a 65535) y los
manda en el enlace `afirma://`; solo el programa que Windows lanza los abre, así
que con AutoFirma elegido GrxFirma no escucha en ellos. Los servicios opcionales
de GrxFirma (servidor REST local en 63118 y servicio WebSocket residente en
8080-8089) están desactivados por defecto y no atienden enlaces `afirma://`
cuando no es GrxFirma quien los recibe; si el portal eligiera justo un puerto
ocupado, AutoFirma prueba los siguientes de la lista.
La extensión de GrxFirma para el navegador usa Native Messaging, no
`afirma://`, y sigue funcionando con cualquiera de las dos opciones.

## Diagnóstico

El paquete se compila con la etiqueta `production`: no instala un lanzador de
depuración e ignora `GRXFIRMA_DEBUG` y `GRXFIRMA_LOG_LEVEL=DEBUG`. Ante un
fallo, use la incidencia saneada que prepara la aplicación. Una reproducción
con trazas detalladas requiere una build de desarrollo separada y datos
sintéticos; no debe activarse sobre el paquete de usuario.

## Observaciones

- Este paquete no instala el `nativehost` de navegador.
- La compilación del handler usa GUI (`fyne_gui`) y debe hacerse desde un Windows real o un runner con dependencias GUI compatibles.
- El registro del protocolo se hace en `HKCU`, sin requerir instalación global del sistema.
- La ruta operativa es fija; el instalador rechaza rutas distintas, datos sin
  marcador de producto y reparse points antes de una limpieza recursiva.

## Construcción del paquete

Desde Linux:

```bash
./packaging/windows/build-afirmauri.sh
./packaging/windows/build-afirmauri.sh --nsis
```

La ruta shell actual soporta hoy:

- `GOARCH=amd64`

Desde Windows con PowerShell:

```powershell
.\packaging\windows\build-afirmauri.ps1
.\packaging\windows\build-afirmauri.ps1 --nsis
```

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autoría: Alberto Avidad Fernández

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
