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
- `nativeMessaging`: conecta con el host `com.dipgra.grxfirma` instalado por la aplicación. La extensión no firma sin GrxFirma.
- `scripting`: registra de forma dinámica el detector de PDF en sitios nuevos después de obtener permiso; el puente de identidad no se registra allí.
- `host_permissions`: `https://*.dipgra.es/*` y `https://*.savia.net/*` son los portales de fábrica para el botón de PDF y la prueba de identidad. `https://127.0.0.1/*` da acceso al firmador local de GrxFirma.
- `optional_host_permissions`: permite pedir acceso a un sitio HTTPS al añadirlo en «Sitios de confianza». La solicitud se hace con el gesto del usuario. Al quitar el sitio, la extensión retira el permiso y el detector. Los sitios añadidos por el usuario solo reciben el botón de PDF.

Los sitios fijados por política de empresa se muestran como «Fijado por su organización» y no pueden borrarse desde la extensión. La organización también debe conceder el permiso de host: sin él, la extensión no registra scripts en ese sitio. La prueba de identidad solo está disponible en los dominios de fábrica y en los fijados por esa política. Chromium declara un esquema `managed_schema`; Firefox lee `storage.managed` de su política empresarial.

### Datos y prueba

La extensión descarga el PDF con la sesión del portal solo cuando el usuario pulsa «Firmar». Lo entrega temporalmente al firmador local; si la descarga o el guardado fallan, abre el firmador sin documento. Tras la aprobación de una prueba de identidad, devuelve al mismo portal el certificado público, su cadena y la firma del reto. No comparte claves privadas. No envía estos datos a servidores de GrxFirma. El código ejecutado está incluido en el paquete: no se descarga ni ejecuta código remoto.

Para probarla, instale GrxFirma y un certificado de pruebas público de la FNMT autorizado para estas pruebas. Abra un PDF en un sitio permitido y pulse «Firmar». Compruebe que se abre el firmador local, que pide la aprobación correspondiente y que puede continuar con el documento. La prueba de identidad requiere un portal que implemente el protocolo de reto; no se activa en sitios añadidos por el usuario.

## English

### Purpose and dependency

This is the companion extension of the GrxFirma desktop signing application; it is distributed together with GrxFirma and does nothing without it. Its sole purpose is to connect the browser to the local signer for PDF signing and, on authorized portals, identity proof with user approval.

On Windows, the GrxFirma installer registers the store listing for Chrome and Edge when their published extension IDs are configured. Both browsers ask the user before enabling the extension; the installer does not place private copies in those browsers. Firefox requires a Mozilla-signed XPI.

### Features and permissions

- `storage`: keeps user-added sites and, for a few minutes, a preloaded PDF and one-use token in `storage.session`. `storage.managed` supplies sites fixed by an organization.
- `nativeMessaging`: connects to the `com.dipgra.grxfirma` host installed by the desktop application. The extension cannot sign without GrxFirma.
- `scripting`: dynamically registers the PDF detector on new sites after permission is granted. It does not register the identity bridge there.
- `host_permissions`: `https://*.dipgra.es/*` and `https://*.savia.net/*` are the built-in portals for the PDF button and identity proof. `https://127.0.0.1/*` reaches GrxFirma's local signer.
- `optional_host_permissions`: lets the user grant HTTPS access when adding a trusted site. The request follows a user gesture. Removing a site removes its permission and PDF detector. User-added sites get only the PDF button.

Sites fixed by enterprise policy are marked as organization-managed and cannot be removed in the extension. The organization must also grant host permission; without it, the extension does not register scripts on that site. Identity proof remains limited to the built-in domains and enterprise-managed sites. Chromium declares a `managed_schema`; Firefox reads `storage.managed` from its enterprise policy.

### Data and testing

Only when the user clicks “Sign” does the extension download a PDF with the portal session. It passes the document temporarily to the local signer; if download or storage fails, the signer opens without a document. After identity proof is approved, the extension returns the public certificate, its chain, and the challenge signature to the same portal. It never exposes private keys or sends these data to GrxFirma servers. All executable code is packaged with the extension; there is no remote code.

To test, install GrxFirma and an authorized public FNMT test certificate. Open a PDF on an allowed site and click “Sign”. Check that the local signer opens, requests the required approval, and can continue with the document. Identity proof needs a portal implementing the challenge protocol; user-added sites cannot trigger it.
