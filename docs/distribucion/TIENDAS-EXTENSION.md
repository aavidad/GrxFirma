<!-- Derechos de autor (C) 2026 Diputación de Granada. -->
<!-- Autoría: Oficina de Software Libre de la Diputación de Granada. -->
<!-- Licencia: EUPL-1.2 -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Extensión en Chrome, Edge y Firefox

La extensión necesita GrxFirma de escritorio y su host de mensajería nativa. Generar los paquetes desde el commit que se va a publicar:

```bash
python3 packaging/browser-extensions/build.py
```

El archivo `packaging/browser-extensions/grxfirma-extension-chromium.zip` sirve para Chrome Web Store y Edge Add-ons. Para AMO, usar `grxfirma-extension-firefox-unsigned.xpi` **solo como código fuente para que Mozilla lo firme al publicar**. `grxfirma-extension-firefox.xpi` generado sin credenciales de Mozilla es una copia de desarrollo sin firma y no se debe ofrecer como descarga. El flujo oficial puede obtener un XPI firmado con `web-ext` y las credenciales de AMO.

Antes de subir una versión, comprobar la versión de ambos manifiestos, ejecutar los tests descritos en el [README de packaging](../../packaging/browser-extensions/README.md), preparar iconos y capturas, y revisar las [notas para revisores](../../packaging/browser-extensions/store/NOTAS_REVISORES.md). Publicar como URL de privacidad la [política estática](../sitio/privacidad.html) cuando Pages esté activo. La [política original de la extensión](../../packaging/browser-extensions/src/chromium/PRIVACY_POLICY.md) contiene el detalle técnico; los textos deben ser coherentes.

## Chrome Web Store

1. Crear una cuenta de desarrollador en Chrome Web Store Developer Dashboard y pagar la cuota única de 5 USD. Activar la verificación que solicite Google.
2. Crear un elemento nuevo y subir `grxfirma-extension-chromium.zip` sin descomprimirlo. Rellenar nombre, descripción, categoría, iconos y capturas; indicar que necesita GrxFirma instalado.
3. En «Privacy», declarar el uso de `storage`, `nativeMessaging`, `scripting`, los dominios de fábrica y el permiso opcional para sitios HTTPS añadidos por la persona usuaria. Indicar que el PDF se pasa al firmador local, que la identidad se devuelve solo al portal autorizado con aprobación y que no hay venta de datos ni código remoto. Añadir la URL pública de privacidad.
4. Pegar las notas de revisión en el campo de instrucciones para el equipo revisor. Enviar a revisión, publicar tras la aprobación y registrar el ID de la extensión para `GRXFIRMA_CHROMIUM_EXTENSION_ID` en el empaquetado Windows.

## Microsoft Edge Add-ons

1. Iniciar sesión o crear una cuenta del Partner Center de Microsoft; registrar el programa Edge Add-ons, sin cuota de publicación.
2. Crear una extensión, subir el mismo ZIP Chromium y completar ficha, capturas, mercados e información de soporte con `avidad@dipgra.es`.
3. Completar la declaración de permisos y privacidad con la misma explicación anterior; enlazar la política pública y aportar las notas para revisores.
4. Enviar a certificación, publicar al aprobarse y registrar el ID resultante como `GRXFIRMA_EDGE_EXTENSION_ID` para el instalador Windows.

## Firefox Add-ons (AMO)

1. Crear una cuenta Mozilla y entrar en Developer Hub de addons.mozilla.org; publicar un nuevo complemento «On this site» para que AMO lo distribuya y firme, sin cuota.
2. Subir `grxfirma-extension-firefox-unsigned.xpi` generado por `build.py` como paquete sin firmar. AMO valida y firma el XPI publicado. Mantener el ID Gecko `grxfirma@aavidad.github.io` y la versión del manifiesto.
3. Completar ficha, capturas, licencia EUPL-1.2, URL de privacidad y declaración de recolección de datos. El manifiesto Firefox declara `personallyIdentifyingInfo` y `websiteContent` por el contenido del PDF y el certificado público que pueden atravesar la extensión durante una operación; explicar que el tránsito es local o al portal autorizado, sin servidores de GrxFirma.
4. Añadir las notas para revisores y, si AMO pide el código fuente o instrucciones de construcción, facilitar el repositorio y `packaging/browser-extensions/build.py`. Enviar a revisión y descargar el XPI firmado por Mozilla para cualquier distribución fuera de AMO.

Después de cada aprobación, probar la ficha publicada con la aplicación instalada y actualizar los IDs de tienda en la configuración de construcción. Los permisos reales están en los [manifiestos Chromium](../../packaging/browser-extensions/src/chromium/manifest.json) y [Firefox](../../packaging/browser-extensions/src/firefox/manifest.json).
