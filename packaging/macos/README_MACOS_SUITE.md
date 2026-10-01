<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma Suite para macOS

Este paquete unifica los componentes principales de `GrxFirma` para macOS:

- CLI
- `nativehost`
- handler `afirma://`
- GUI desktop Qt/QML con su backend IPC

## Qué incluye

- `grxfirma`
- `grxfirma-nativehost`
- `GrxFirma AfirmaURI.app`
- `GrxFirma Desktop Qt.app`
  - `Contents/MacOS/grxfirma-gui-qml` (frontend Qt/QML)
  - `Contents/MacOS/grxfirma-gui` (bootstrap y servidor IPC)
  - `Contents/MacOS/grxfirma` (backend principal)
- `install-suite.sh`
- `install-nativehost.sh`
- `install-afirmauri.sh`
- `install-desktop-qml.sh`
- `uninstall-suite.sh`
- `README_MACOS_SUITE.md`
- `SAFARI_BROWSER_EXTENSION.md`
- `VERSION.txt`
- `extensions/` con artefactos Firefox y Chromium
- `safari/` si se genera con `GRXFIRMA_BUILD_SAFARI=1` en macOS

## Qué instala

La instalación por usuario con `install-suite.sh` instala y registra:

- la CLI en una ruta local del usuario;
- el `nativehost` y sus manifiestos de `Native Messaging` para Chrome, Chromium, Edge, Brave, Vivaldi, Opera y Firefox;
- la aplicación manejadora de `afirma://` en `~/Applications`;
- la aplicación desktop Qt/QML y su backend en `~/Applications`;
- scripts auxiliares de instalación.

El instalador `--pkg` coloca ambas aplicaciones en `/Applications`, la CLI en
`/usr/local/bin` y el payload en `/Library/Application Support/GrxFirma`.
Su `postinstall` registra el usuario de consola y un `LaunchAgent` repite el
registro de forma idempotente para cada sesión gráfica posterior.

## Instalación

```bash
chmod +x install-suite.sh
./install-suite.sh
```

La instalación por usuario es transaccional: antes de modificar la CLI, las
aplicaciones, las extensiones o los manifiestos de navegador guarda una copia
de todos los destinos afectados. Si falla incluso el último componente,
restaura el estado completo anterior y vuelve a registrar en LaunchServices la
versión previa del handler. La misma restauración se ejecuta ante `SIGINT` o
`SIGTERM`. También rechaza ancestros simbólicos en los destinos para no
escribir fuera de `HOME`.

## Desinstalación

Para retirar la instalación del usuario actual:

```bash
~/Library/Application\ Support/GrxFirma/uninstall-suite.sh
```

El script es idempotente, elimina las aplicaciones y registros que pertenecen
a GrxFirma, conserva manifiestos modificados o ajenos y no sigue enlaces
simbólicos. Desde el directorio extraído también puede ejecutarse directamente
como `./uninstall-suite.sh`.

Si la instalación procede del PKG, limpia primero los registros de cada usuario
y después elimina el payload del sistema:

```bash
sudo "/Library/Application Support/GrxFirma/uninstall-suite.sh" --system
```

El modo `--system` solo elimina el enlace de `/usr/local/bin/grxfirma` si
apunta al binario de este paquete y conserva cualquier fichero ajeno que ocupe
esa ruta.

## Construcción

Debe construirse desde un macOS real con Qt 6, `qmake` y `macdeployqt`
disponibles:

```bash
./packaging/macos/build-suite.sh
GOARCH=arm64 ./packaging/macos/build-suite.sh
```

La suite final ya no admite un cross-build parcial desde Linux: el despliegue
correcto de frameworks y plugins de la GUI requiere `macdeployqt` sobre macOS.
Los empaquetadores de componentes Go individuales conservan su ruta con
toolchain Darwin cruzada para diagnósticos, pero no sustituyen este build final.

Arquitecturas soportadas por el script:

- `GOARCH=amd64`
- `GOARCH=arm64`

Si quieres generar tambien el instalador nativo:

```bash
./packaging/macos/build-suite.sh --pkg
```

La construcción valida el contenido del `.tar.gz` y del `.pkg` antes de
publicarlos, incluidos los tres ejecutables obligatorios del bundle Desktop Qt.
El preflight de instalación y la firma rechazan también un bundle sin el
bootstrap IPC. En `release/macos-suite/` genera además `SHA256SUMS.txt` y
`ARTIFACTS.md` con tamaños y huellas SHA-256 verificadas.

Firma y notarizacion opcionales:

- `MACOS_CODESIGN_IDENTITY`: identidad para `codesign`
- `MACOS_INSTALLER_IDENTITY`: identidad para firmar el `.pkg`
- `MACOS_NOTARY_PROFILE`: perfil de `notarytool` ya configurado en el llavero

Cuando hay identidad Developer ID, la construcción firma primero cada Mach-O
y bundle Qt anidado (`framework`, `plugin`, `appex`, `xpc`, etc.) en recorrido
de máxima profundidad, y firma la aplicación contenedora al final. Así la firma
del contenedor no queda invalidada por una modificación posterior de sus
componentes internos. El gate oficial vuelve a comprobar cada aplicación con
`codesign`, Gatekeeper y la evidencia de notarización grapada.

La app Desktop Qt se ensambla por completo antes de ejecutar `macdeployqt`:
bootstrap, backend, QML, recursos, versión y `Info.plist` ya están presentes
cuando Qt despliega y firma de forma ad hoc el bundle. QML y recursos se alojan
en `Contents/Resources`, nunca junto a los ejecutables de `Contents/MacOS`, para
que `codesign` selle correctamente el contenedor. Tras `macdeployqt`, el firmador
profundo vuelve a sellar de dentro hacia fuera los frameworks Qt y los tres
ejecutables propios con identidad ad hoc, y por último el contenedor. El build
comprueba esa firma antes de crear los artefactos. Si se configura Developer ID,
la firma oficial sustituye después la ad hoc, siempre como última operación
sobre la app.

Safari opcional:

```bash
GRXFIRMA_BUILD_SAFARI=1 ./packaging/macos/build-suite.sh --pkg
```

Ese modo genera el proyecto Xcode en `safari/` cuando el build se ejecuta en
macOS con Xcode. El proyecto debe revisarse, firmarse, notarizarse y validarse
en Safari real antes de declararlo soportado.

## Reproducibilidad

Los binarios Go se construyen con dependencias de solo lectura, PGO desactivado,
`-trimpath`, metadatos VCS desactivados y `buildid` vacío; `main.version` se
mantiene. El `tar.gz` ordena las entradas y normaliza propietario, grupo, modos
y tiempos. Los árboles entregados a `pkgbuild` reciben el mismo
`SOURCE_DATE_EPOCH`, que por defecto es la fecha de `HEAD` y puede fijarse desde
el entorno.

La igualdad byte a byte solo se espera para artefactos sin firmar y con las
mismas versiones de Go, SDK/Xcode, compilador, Python/zlib, Qt/macdeployqt y
`pkgbuild`. `codesign --timestamp`, la firma del instalador, notarización y
grapado incorporan evidencia temporal externa y deben producir bytes distintos.
Apple tampoco garantiza un PKG idéntico entre versiones de `pkgbuild`.

## Observaciones

- La CLI y el `nativehost` dependen del Keychain real de macOS.
- El handler `afirma://` se registra mediante una `.app` con `Info.plist`.
- Safari no queda cubierto por los manifiestos `NativeMessagingHosts`. El
  handler `afirma://` es una integracion alternativa, no una Safari Web
  Extension completa. Para Safari hace falta app contenedora de Apple, puente
  nativo, firma/notarizacion y validacion en Safari real; ver
  `SAFARI_BROWSER_EXTENSION.md`.
- Si se genera `--pkg`, el `postinstall` registra al usuario de consola y el
  `LaunchAgent` cubre los demás usuarios al iniciar una sesión gráfica. En un
  despliegue MDM sigue siendo obligatorio validar este comportamiento con las
  políticas y versiones macOS reales de la organización.
- Si el usuario arranca la REST local en `127.0.0.1:63118`, tiene también:
  - `/` como consola técnica web local para firma, verificación, certificados, diagnóstico y firma múltiple por `/sign-batch`;
  - `/signer` como firmador web local con sello visible y firma múltiple.
- El sello visible expuesto por esas superficies web soporta:
  - una página concreta;
  - rangos como `1,3-5`;
  - todas las páginas con `all`.
- La desktop Qt/QML forma parte de la suite final y del PKG; el paquete desktop
  separado se conserva solo para pruebas o distribución manual del componente.
- La desinstalación retira el bundle Desktop Qt completo, incluido
  `Contents/MacOS/grxfirma-gui`; no deja un bootstrap IPC antiguo.
- Si el usuario tiene perfiles Firefox reales, el instalador sólo despliega el
  XPI cuando los metadatos lo marcan como firmado y su SHA-256 coincide.
- El CRX local solo se genera con `GRXFIRMA_BUILD_CHROMIUM_CRX=1`; su firma
  CRX3 aleatoria queda fuera de la reproducibilidad byte a byte y no se fuerza
  con `external_crx`. Los despliegues gestionados pueden registrar canales HTTPS
  con `GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL` y
  `GRXFIRMA_EDGE_EXTENSION_UPDATE_URL`, acompañados de sus IDs de extensión.

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autor: Oficina de Software Libre de la Diputacion de Granada.

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
