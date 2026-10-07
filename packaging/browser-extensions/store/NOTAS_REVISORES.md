<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Notas para revisores / Reviewer notes

## Español

### Propósito y dependencia

Esta es la extensión complementaria de la aplicación de firma de escritorio GrxFirma. Se distribuye junto con GrxFirma y no hace nada útil sin ella. Su único propósito es conectar el navegador con el firmador local para firmar PDF y, en portales autorizados, acreditar la identidad del usuario con su consentimiento.

En Windows, el instalador de GrxFirma registra la ficha de tienda para Chrome y Edge si están configurados sus IDs publicados. Ambos navegadores preguntan al usuario antes de activar la extensión; el instalador no coloca copias propias de ella en esos navegadores. Firefox requiere un XPI firmado por Mozilla.

### Funciones y permisos

- `storage`: conserva los sitios añadidos por el usuario y, durante unos minutos, un PDF precargado y su token de un solo uso en `storage.session`. `storage.managed` permite leer sitios fijados por la organización.
- `nativeMessaging`: conecta con el host `io.github.aavidad.grxfirma` instalado por la aplicación. La extensión no firma sin GrxFirma.
- `scripting`: registra de forma dinámica el detector de PDF en sitios nuevos después de obtener permiso; el puente de identidad no se registra allí.
- `host_permissions`: `https://*.dipgra.es/*` y `https://*.savia.net/*` son los portales de fábrica para el botón de PDF y la prueba de identidad. `https://127.0.0.1/*` da acceso al firmador local de GrxFirma.
- `optional_host_permissions`: permite pedir acceso a un sitio HTTPS al añadirlo en «Sitios de confianza». La solicitud se hace con el gesto del usuario. Al quitar el sitio, la extensión retira el permiso y el detector. Los sitios añadidos por el usuario solo reciben el botón de PDF.

Los sitios fijados por política de empresa se muestran como «Fijado por su organización» y no pueden borrarse desde la extensión. La organización también debe conceder el permiso de host: sin él, la extensión no registra scripts en ese sitio. La prueba de identidad solo está disponible en los dominios de fábrica y en los fijados por esa política. Chromium declara un esquema `managed_schema`; Firefox lee `storage.managed` de su política empresarial.

### Datos y prueba

La extensión descarga el PDF con la sesión del portal solo cuando el usuario pulsa «Firmar». Lo entrega temporalmente al firmador local; si la descarga o el guardado fallan, abre el firmador sin documento. Tras la aprobación de una prueba de identidad, devuelve al mismo portal el certificado público, su cadena y la firma del reto. No comparte claves privadas. No envía estos datos a servidores de GrxFirma. El código ejecutado está incluido en el paquete: no se descarga ni ejecuta código remoto.

### Cómo probarla

Sin la aplicación instalada se puede comprobar casi todo:

1. Abra la ventana de la extensión. Debe decir «No se pudo conectar con GrxFirma…»: el host nativo no existe y no hay más intentos.
2. Pulse «Sitios de confianza», añada `www.w3.org` y acepte el permiso que pide el navegador.
3. Abra `https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf`. Aparece el botón «Firmar PDF» abajo a la derecha.
4. Púlselo. La extensión descarga el PDF y abre una pestaña en `https://127.0.0.1:63118/signer`, que es el firmador local de GrxFirma. Sin la aplicación, el navegador indica que no puede conectar con esa dirección. El PDF guardado para la entrega caduca a los cinco minutos.
5. Quite `www.w3.org` de la lista. El navegador retira el permiso y el botón deja de aparecer al recargar el PDF.

Con la aplicación, instale GrxFirma desde https://github.com/aavidad/GrxFirma/releases (en Linux basta el paquete `.deb`) y un certificado de pruebas. Al pulsar «Firmar PDF» se abre el firmador local con el documento y pide elegir certificado y aprobar. La prueba de identidad necesita un portal que implemente el protocolo de reto y no se activa en sitios añadidos por el usuario.

### Mensajería nativa

La extensión abre `runtime.connectNative("io.github.aavidad.grxfirma")` para comprobar que GrxFirma está instalada (`ping`) y para la prueba de identidad (`proveIdentity`). El instalador de GrxFirma registra ese host con el ID de la extensión en `allowed_origins`, y el host vuelve a comprobar el origen que lo llama. Los mensajes son JSON con un `requestId` y una acción concreta. La firma de un PDF no pasa por la mensajería nativa: la extensión abre el firmador local en `https://127.0.0.1:63118/signer` y un script de contenido limitado a esa dirección le entrega el documento con un token de un solo uso. La extensión no expone la mensajería nativa a las páginas: no declara `externally_connectable` ni `web_accessible_resources`, y el service worker solo acepta de cada script de contenido las acciones previstas para su origen.

### Código

Todo el JavaScript está en el paquete, sin minificar, concatenar ni transpilar. No hay `eval`, ni scripts remotos, ni dependencias de terceros. El ZIP y el XPI se generan con `python3 packaging/browser-extensions/build.py`, que solo copia los archivos de `src/chromium` o `src/firefox` (quitando material heredado que no se usa) con fechas fijas. Por eso no se adjunta un paquete de código fuente aparte; el repositorio público es https://github.com/aavidad/GrxFirma.

## English

### Purpose and dependency

This is the companion extension of the GrxFirma desktop signing application; it is distributed together with GrxFirma and does nothing without it. Its sole purpose is to connect the browser to the local signer for PDF signing and, on authorized portals, identity proof with user approval.

On Windows, the GrxFirma installer registers the store listing for Chrome and Edge when their published extension IDs are configured. Both browsers ask the user before enabling the extension; the installer does not place private copies in those browsers. Firefox requires a Mozilla-signed XPI.

### Features and permissions

- `storage`: keeps user-added sites and, for a few minutes, a preloaded PDF and one-use token in `storage.session`. `storage.managed` supplies sites fixed by an organization.
- `nativeMessaging`: connects to the `io.github.aavidad.grxfirma` host installed by the desktop application. The extension cannot sign without GrxFirma.
- `scripting`: dynamically registers the PDF detector on new sites after permission is granted. It does not register the identity bridge there.
- `host_permissions`: `https://*.dipgra.es/*` and `https://*.savia.net/*` are the built-in portals for the PDF button and identity proof. `https://127.0.0.1/*` reaches GrxFirma's local signer.
- `optional_host_permissions`: lets the user grant HTTPS access when adding a trusted site. The request follows a user gesture. Removing a site removes its permission and PDF detector. User-added sites get only the PDF button.

Sites fixed by enterprise policy are marked as organization-managed and cannot be removed in the extension. The organization must also grant host permission; without it, the extension does not register scripts on that site. Identity proof remains limited to the built-in domains and enterprise-managed sites. Chromium declares a `managed_schema`; Firefox reads `storage.managed` from its enterprise policy.

### Data and testing

Only when the user clicks “Sign” does the extension download a PDF with the portal session. It passes the document temporarily to the local signer; if download or storage fails, the signer opens without a document. After identity proof is approved, the extension returns the public certificate, its chain, and the challenge signature to the same portal. It never exposes private keys or sends these data to GrxFirma servers. All executable code is packaged with the extension; there is no remote code.

### How to test

Most of it can be checked without the desktop application:

1. Open the extension window. It should say that it could not connect to GrxFirma: the native host is missing and nothing else is attempted.
2. Click "Trusted sites", add `www.w3.org` and accept the browser permission prompt.
3. Open `https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf`. A "Sign PDF" button appears at the bottom right.
4. Click it. The extension downloads the PDF and opens a tab at `https://127.0.0.1:63118/signer`, the GrxFirma local signer. Without the application the browser reports that it cannot connect to that address. The PDF kept for the hand-over expires after five minutes.
5. Remove `www.w3.org` from the list. The browser withdraws the permission and the button no longer appears after reloading the PDF.

With the application, install GrxFirma from https://github.com/aavidad/GrxFirma/releases (the `.deb` package is enough on Linux) and a test certificate. Clicking "Sign PDF" opens the local signer with the document and asks you to pick a certificate and approve. Identity proof needs a portal implementing the challenge protocol and cannot be triggered on user-added sites.

### Native messaging

The extension calls `runtime.connectNative("io.github.aavidad.grxfirma")` to check that GrxFirma is installed (`ping`) and for identity proof (`proveIdentity`). The GrxFirma installer registers that host with this extension's ID in `allowed_origins`, and the host checks the calling origin again. Messages are JSON objects with a `requestId` and a specific action. Signing a PDF does not use native messaging: the extension opens the local signer at `https://127.0.0.1:63118/signer` and a content script limited to that address hands it the document with a one-time token. Pages cannot reach native messaging: there is no `externally_connectable` and no `web_accessible_resources`, and the service worker accepts from each content script only the actions allowed for its origin.

### Code

All JavaScript ships in the package, not minified, concatenated or transpiled. There is no `eval`, no remote script and no third-party dependency. The ZIP and XPI are produced by `python3 packaging/browser-extensions/build.py`, which only copies the files under `src/chromium` or `src/firefox` (leaving out unused legacy files) with fixed timestamps. No separate source package is attached for that reason; the public repository is https://github.com/aavidad/GrxFirma.
