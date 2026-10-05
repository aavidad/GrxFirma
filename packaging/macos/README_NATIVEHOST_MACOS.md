<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# GrxFirma NativeHost para macOS

Este paquete contiene la integración de `Native Messaging` para navegadores en macOS.

## Qué incluye

- `grxfirma-nativehost`
- `install-nativehost.sh`
- `README_NATIVEHOST_MACOS.md`
- `VERSION.txt`

## Para qué sirve

Permite que una extensión de navegador se comunique con `GrxFirma` mediante `Native Messaging`.

Sirve para:

- detectar la presencia de `GrxFirma`;
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

Safari no esta incluido en este instalador de `NativeMessagingHosts`.

## Instalación

```bash
chmod +x install-nativehost.sh
./install-nativehost.sh
```

El script:

- copia `grxfirma-nativehost` a `~/Library/Application Support/GrxFirma/NativeHost`;
- genera los manifiestos JSON de `Native Messaging`;
- los escribe en las rutas de navegador del usuario;
- conserva las extensiones empaquetadas junto al host;
- sólo instala el XPI en perfiles Firefox cuando consta como firmado y coincide
  con la huella SHA-256 de sus metadatos;
- sólo registra canales externos Chromium cuando la URL usa HTTPS y el ID es
  válido.

## Construcción

Debe construirse desde un macOS real o desde Linux con una toolchain Darwin
cruzada configurada en `CC` y `CXX` (por ejemplo `osxcross`):

```bash
./packaging/macos/build-nativehost.sh
```

Si lo ejecutas en Linux sin toolchain Darwin, el script aborta con:

```text
error: el build macOS requiere macOS real o una toolchain Darwin cruzada configurada en CC/CXX.
       En Linux, exporta una toolchain tipo osxcross antes de ejecutar este script.
```

## Observaciones

- Este paquete no instala la app desktop completa ni el handler `afirma://`.
- Safari Web Extension usa app contenedora y mensajeria nativa de Apple. Este
  paquete solo cubre los navegadores que leen manifiestos `NativeMessagingHosts`;
  para Safari hay que generar y firmar una app Safari especifica. El handler
  `afirma://` puede servir como integracion web alternativa, pero no sustituye
  a una extension Safari completa.
- El host nativo muestra una confirmacion de macOS antes de firmar. El modo
  automatico queda reservado a despliegues gestionados mediante
  `GRXFIRMA_NATIVEHOST_AUTO_APPROVE=1`.
- El CRX local, cuando se solicita al empaquetador con
  `GRXFIRMA_BUILD_CHROMIUM_CRX=1`, no se instala mediante `external_crx`. Para un canal gestionado
  se usan `GRXFIRMA_CHROMIUM_EXTENSION_ID`,
  `GRXFIRMA_CHROMIUM_EXTENSION_UPDATE_URL`,
  `GRXFIRMA_EDGE_EXTENSION_ID` y
  `GRXFIRMA_EDGE_EXTENSION_UPDATE_URL`; las URL deben ser HTTPS.
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

## Licencia

Software libre bajo licencia EUPL 1.2 o posterior.

Autoría: Alberto Avidad Fernández

Sin garantía:
- esta herramienta se entrega SIN GARANTÍA de ningún tipo.
