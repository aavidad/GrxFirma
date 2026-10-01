<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma Desktop Qt/QML para Windows

Este paquete contiene la app desktop manual de GrxFirma basada en Qt/QML.

## Qué incluye

- `grxfirma-gui-qml.exe`
- `grxfirma-gui.exe`
- `grxfirma.exe`
- `install-desktop-qml.ps1`
- `uninstall-desktop-qml.ps1`
- `install-path-safety.ps1`
- `invoke-uninstall-silent.ps1`
- `README_DESKTOP_QML_WINDOWS.md`
- `VERSION.txt`
- directorios `qml/` y `assets/`
- directorio `help/` cuando existe en `cmd/gui-qml/help`

Si se construye con despliegue Qt:

- bibliotecas Qt redistribuibles
- plugins necesarios

Y, si se compila el instalador NSIS:

- `GrxFirma-0.0.90-desktop-qml-windows-amd64-setup.exe`

El ZIP portable es `GrxFirma-0.0.90-desktop-qml-windows-amd64.zip`.

## Para qué sirve

Es la app manual de escritorio para Windows:

- abrir documentos;
- seleccionar certificado;
- firmar;
- verificar;
- operar principalmente por `IPC` local con el backend de GrxFirma;
- usar opcionalmente la REST local como superficie técnica y de soporte.

Si el backend REST local está activo, expone también:

- `https://127.0.0.1:63118/` como consola técnica local;
- `https://127.0.0.1:63118/signer` como firmador web local.

Capacidades visibles actuales de esas superficies web:

- firma de uno o varios ficheros con `/sign-batch`;
- sello visible `PAdES`;
- sello en una página concreta, en rangos como `1,3-5` o en todas las páginas con `all`.

## Requisitos de construcción

Debe construirse desde un Windows con:

- Qt instalado;
- `qmake` en `PATH`;
- `windeployqt` en `PATH`;
- compilador compatible con el kit Qt usado.

## Construcción

Desde PowerShell:

```powershell
.\packaging\windows\build-desktop-qml.ps1
.\packaging\windows\build-desktop-qml.ps1 --nsis
```

Desde Linux, si ya tienes instalada una toolchain Qt para Windows
(`qmake`/`windeployqt` del kit MinGW):

```bash
./packaging/windows/build-desktop-qml.sh
./packaging/windows/build-desktop-qml.sh --nsis
```

Si el script detecta la Qt nativa de Linux en lugar de la Qt de Windows,
fallará pronto con diagnóstico y te pedirá apuntar `QMAKE` y `WINDEPLOYQT`
al kit correcto.

## Instalación manual

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-desktop-qml.ps1
```

El script:

- copia la app a `%LOCALAPPDATA%\Programs\GrxFirma\DesktopQML`;
- crea acceso directo en el menú Inicio;
- deja junto al frontend el backend IPC `grxfirma-gui.exe` y el backend
  REST/CLI `grxfirma.exe`;
- copia también `help/` si el paquete lo trae.
- acepta los dos runtimes soportados por `windeployqt`: DLL de MinGW o
  un único `vc_redist.x64.exe` de MSVC; en este último caso exige firma
  Authenticode válida de Microsoft, lo ejecuta y comprueba su código de salida.

Para desinstalar manualmente:

```powershell
.\uninstall-desktop-qml.ps1
```

El desinstalador NSIS ejecuta esa misma limpieza para no dejar la copia de
`%LOCALAPPDATA%\Programs\GrxFirma\DesktopQML` ni el acceso directo del menú Inicio.

El NSIS y los archivos de la aplicación son por usuario y no muestran un
selector de destino. La copia de mantenimiento vive en
`%LOCALAPPDATA%\Programs\GrxFirma\DesktopQt`; la app operativa se instala en
`%LOCALAPPDATA%\Programs\GrxFirma\DesktopQML`. Los scripts solo aceptan esa ruta,
exigen marcador de producto y rechazan reparse points antes de vaciarla o
eliminarla.

`setup.exe /S` termina sin diálogos bloqueantes y devuelve `1603` si falla un
proceso auxiliar. Para el despliegue gestionado, la entrada de desinstalación
incluye `QuietUninstallString`: ejecuta una copia temporal del desinstalador con
ACL exclusiva, conserva el código de salida y rechaza enlaces, junctions y
otros reparse points en el árbol de mantenimiento.

Con el runtime MinGW app-local, la instalación no necesita elevar privilegios.
Si el paquete MSVC incluye el redistribuible oficial `vc_redist.x64.exe`,
Windows puede pedir UAC al ejecutarlo aunque GrxFirma siga instalándose solo
para el usuario actual.

Las versiones anteriores del NSIS instalaban el desktop o la Suite por equipo
y escribían en `HKLM`. El instalador actual revisa las vistas de registro de 32
y 64 bits antes de copiar nada. Si encuentra una de esas versiones, se detiene
y pide desinstalarla primero desde **Configuración > Aplicaciones** aceptando
UAC, o ejecutar su `uninstall.exe` como administrador. Después puede instalarse
la edición actual por usuario.

## Ayuda localizada

Si el paquete incluye PDFs en `help/`, el frontend intenta abrir primero el
idioma seleccionado y solo cae al HTML local si no existe PDF compatible.

Nombres y rutas reconocidos:

- `help/ayuda-<locale>.pdf`
- `help/ayuda-<lang>.pdf`
- `help/ayuda.pdf`
- `help/<locale>/ayuda.pdf`
- `help/<lang>/ayuda.pdf`

Ejemplos:

- `help/ayuda-es.pdf`
- `help/ayuda-en.pdf`
- `help/es/ayuda.pdf`
- `help/en/ayuda.pdf`

## Avisos de nuevas versiones

Qt/QML puede consultar una vez por arranque la última Release estable en
GitHub. El ajuste **Configuración > Avisar de nuevas versiones** queda
habilitado por defecto y se puede desactivar; **Acerca de > Comprobar
actualizaciones** conserva la acción manual. Se requiere HTTPS a
`api.github.com:443` y, para abrir la página, a `github.com:443`.

El frontend usa la versión inyectada en el backend IPC y nunca descarga,
instala ni ejecuta artefactos. La Release debe ser pública y estable; no se
incluye un token para consultar un repositorio privado. Un error de red o
publicación explica cómo reintentar y no bloquea la firma local.

## Observaciones

- Este paquete no registra por sí solo `afirma://`.
- Está orientado a escritorio manual, no a portales web legacy.
- El frontend busca `grxfirma-gui.exe` en el mismo directorio para arrancar
  el backend IPC. `grxfirma.exe` se conserva para el modo REST experto y la
  consola web local.
- En escritorio manual el modo preferente es `IPC`; la REST local queda como vía experta, consola web local y soporte.
- Si se arranca la REST local, `/validator` (alias `/validador`) ofrece la
  utilidad avanzada para validar certificados, firmas y documentos, consultar
  OCSP/CRL, calcular hashes y exportar informes JSON.

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autor: Oficina de Software Libre de la Diputacion de Granada.
- Alberto Avidad Fernandez
- Oficina de Software Libre - Diputación de Granada

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
