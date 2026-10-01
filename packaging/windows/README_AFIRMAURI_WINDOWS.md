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

Autor: Oficina de Software Libre de la Diputacion de Granada.
- Alberto Avidad Fernandez
- Oficina de Software Libre - Diputación de Granada

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
