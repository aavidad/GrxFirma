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
- genera `afirmauri-handler.cmd`;
- registra `HKCU\Software\Classes\afirma` para que la URI `afirma://` abra el handler.

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

En **Configuración > Firmas desde los portales** (WinUI y Qt) se elige cuál
abre las firmas de los portales:

- **AutoFirma**: el motor retira el registro HKCU de GrxFirma y restaura lo que
  hubiera antes de GrxFirma (normalmente nada, y Windows usa el de AutoFirma).
  Solo se ofrece si AutoFirma tiene registrado `afirma://` y existe su
  `Autofirma.exe`.
- **GrxFirma**: vuelve a registrar HKCU con la misma instantánea
  (`afirma-protocol-snapshot.json`).

El motor lo hace con la API del registro, sin PowerShell, y con las mismas
comprobaciones de propiedad que el instalador: si otro programa ha cambiado el
registro, no toca nada. La elección se guarda en
`HKCU\Software\GrxFirma\AfirmaProtocolHandler`; una actualización la
respeta mientras AutoFirma siga instalado y el registro siga retirado.

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
