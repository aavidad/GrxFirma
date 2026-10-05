<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma NativeHost para Windows

Este paquete contiene la integración de `Native Messaging` para navegadores en Windows.

## Qué incluye

- `grxfirma-nativehost.exe`
- `install-nativehost.ps1`
- `uninstall-nativehost.ps1`
- `install-path-safety.ps1`
- `README_NATIVEHOST_WINDOWS.md`
- `VERSION.txt`

## Para qué sirve

Permite que una extensión de navegador se comunique con GrxFirma mediante el protocolo `Native Messaging`.

Sirve para:

- detectar la presencia de GrxFirma;
- listar certificados;
- solicitar firma y verificación;
- devolver respuestas a la web a través de la extensión y el host nativo.

## Navegadores previstos

- Google Chrome
- Chromium
- Microsoft Edge
- Brave
- Vivaldi
- Opera
- Mozilla Firefox

## Instalación

El ZIP independiente se llama `GrxFirma-0.0.90-nativehost-windows-amd64.zip`.

Desde PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-nativehost.ps1
```

El script:

- copia `grxfirma-nativehost.exe` a `%LOCALAPPDATA%\Programs\GrxFirma\NativeHost`;
- deja los artefactos de extension Firefox y Chromium dentro de `extensions\`;
- genera los manifiestos JSON de `Native Messaging`;
- registra en `HKCU` las rutas de manifiesto para Chrome, Chromium, Edge, Brave, Vivaldi, Opera y Firefox.
- instala la extension de Firefox en perfiles existentes solo cuando el XPI esta
  firmado por Mozilla y coincide con el SHA-256 de su metadato;
- intenta ademas desplegar ese XPI aprobado en la distribucion global de Firefox
  cuando el proceso tiene permisos;
- registra la ficha oficial de Chrome y Edge solo cuando se proporcionan sus
  IDs publicados mediante `GRXFIRMA_CHROMIUM_EXTENSION_ID` y
  `GRXFIRMA_EDGE_EXTENSION_ID`; los navegadores piden confirmación al usuario.
  No instala un CRX propio;
- conserva un inventario de esos registros y recupera los IDs válidos al
  reinstalar, aunque ya no estén definidas las variables de entorno.

La instalación es transaccional. Antes de modificar el sistema crea una
instantánea segura del payload existente y de los 21 valores `HKCU` de Native
Messaging; si falla cualquier copia o escritura, restaura directorio,
manifiestos, inventario y registro en orden inverso. Los IDs publicados y las
URL oficiales de tienda se validan antes de la primera modificación.

Para desinstalar y retirar binario, manifiestos, registros y despliegues
externos administrados por el paquete:

```powershell
.\uninstall-nativehost.ps1
```

Los scripts solo aceptan la ruta fija
`%LOCALAPPDATA%\Programs\GrxFirma\NativeHost`, verifican el marcador del producto y
rechazan reparse points antes de eliminar contenido.
La desinstalación elimina únicamente valores registrales que todavía apuntan a
esta instalación y conserva claves o valores ajenos. También conserva un XPI
de Firefox que haya sido actualizado o sustituido después de instalarlo.

## Observaciones

- Este paquete no instala la app desktop completa ni el handler `afirma://`.
- La instalación estable Chromium requiere tienda o política empresarial. El
  instalador solo registra la ficha de Chrome o Edge cuando recibe el ID
  publicado correspondiente. Usa la URL oficial de actualización de cada
  tienda; no acepta una URL de actualización personalizada.
- Los IDs configurados, o recuperados del inventario en una reinstalación, se
  validan y se incorporan a `allowed_origins`.
- Los manifiestos del host se registran tambien en rutas compatibles con Vivaldi
  y Opera; la disponibilidad final depende del canal y politicas de cada navegador.
- La compilación del `nativehost` es multiplataforma, pero el registro de navegadores es específico de Windows.
- El host nativo muestra una confirmación de Windows antes de firmar. El modo
  automático queda reservado a despliegues gestionados mediante
  `GRXFIRMA_NATIVEHOST_AUTO_APPROVE=1`.
- Los nombres de host registrados son:
  - `com.grxfirma.native`
  - `io.github.aavidad.grxfirma`
  - `io.github.aavidad.portafirmas`
- Las versiones anteriores registraban `com.dipgra.grxfirma`,
  `com.dipgra.portafirmas` y la extensión de Firefox `extension@dipgra.es`. Al
  instalar o actualizar se borran los registros y manifiestos con esos nombres
  que apuntan a esta instalación, y las copias de la extensión anterior que
  GrxFirma dejó en los perfiles de Firefox. El desinstalador limpia los nombres
  nuevos y los anteriores.

## Construcción del paquete

Desde Linux:

```bash
./packaging/windows/build-nativehost.sh
```

Desde Windows con PowerShell:

```powershell
.\packaging\windows\build-nativehost.ps1
```

La ruta shell actual soporta hoy:

- `GOARCH=amd64`

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autor: Oficina de Software Libre de la Diputacion de Granada.
- Alberto Avidad Fernandez
- Oficina de Software Libre - Diputación de Granada

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
