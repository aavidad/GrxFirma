<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Textos para las fichas de tienda

Textos listos para pegar en Chrome Web Store, Microsoft Edge Add-ons y Firefox Add-ons (AMO). La extensión trae dos idiomas en `_locales`: español (predeterminado) e inglés. Cada tienda toma el nombre y el resumen corto del manifiesto según el idioma; el resto se escribe en la ficha.

La guía paso a paso está en [TIENDAS-EXTENSION.md](../../../docs/distribucion/TIENDAS-EXTENSION.md). Las capturas y gráficos están en `capturas/` y `graficos/` de esta carpeta.

## Datos comunes

| Campo | Valor |
|---|---|
| Nombre | GrxFirma (viene del manifiesto: `extensionName`) |
| Categoría Chrome / Edge | Productividad › Herramientas (Edge: Productividad) |
| Categoría AMO | Otros / Privacidad y seguridad (elegir «Other» si no se quiere asociar a privacidad) |
| Sitio web | https://aavidad.github.io/GrxFirma/ |
| Soporte | avidad@dipgra.es y https://github.com/aavidad/GrxFirma/issues |
| Política de privacidad | https://aavidad.github.io/GrxFirma/privacidad.html |
| Licencia | EUPL-1.2 (en AMO: «Otra licencia», enlazar https://github.com/aavidad/GrxFirma/blob/main/LICENSE) |
| Idiomas de la ficha | Español (principal) e inglés |
| Precio | Gratuita, sin compras ni anuncios |
| Visibilidad recomendada | Pública. Para una primera ronda, «No listada» en Chrome/Edge o «On your own» en AMO |

## Español

### Resumen (132 caracteres como máximo)

Chrome y Edge muestran como resumen la descripción del manifiesto (`extensionDescription`, 98 caracteres):

> Extensión de GrxFirma para firmar documentos PDF y acreditar la identidad ante sitios autorizados.

Si la tienda deja escribir otro resumen, esta versión tiene 116 caracteres:

> Firme con GrxFirma los PDF que abre en el navegador. Necesita la aplicación de escritorio GrxFirma y su certificado.

### Resumen para AMO (250 caracteres como máximo)

> Conecta Firefox con la aplicación de escritorio GrxFirma para firmar con su certificado los PDF que abre en sitios permitidos. Necesita GrxFirma instalada. Los documentos no pasan por servidores de GrxFirma.

### Descripción

```text
GrxFirma es una aplicación de escritorio para firmar documentos con certificado electrónico. Esta extensión la conecta con el navegador.

Qué hace
• En los sitios permitidos, al abrir un PDF aparece el botón «Firmar PDF». Al pulsarlo, la extensión pasa el documento a GrxFirma, que pide elegir el certificado y aprobar la firma.
• En los portales autorizados, puede acreditar su identidad con el certificado. GrxFirma pide siempre su aprobación y la clave privada no sale del equipo.
• En «Sitios de confianza» puede añadir otros sitios HTTPS. El navegador le pedirá permiso para cada uno y puede retirarlo cuando quiera.

Qué necesita
• La aplicación GrxFirma instalada en Windows o Linux: https://github.com/aavidad/GrxFirma/releases
• Un certificado electrónico instalado en el equipo o en una tarjeta.
Sin la aplicación, la extensión avisa de que no puede conectar y no firma nada.

Privacidad
• La extensión no tiene cuentas ni envía documentos o datos de uso a servidores de GrxFirma.
• El PDF se descarga del portal donde está, con su sesión, y se entrega al firmador del equipo.
• No ejecuta código descargado de Internet.
Política completa: https://aavidad.github.io/GrxFirma/privacidad.html

GrxFirma es software libre con licencia EUPL-1.2. Código fuente: https://github.com/aavidad/GrxFirma
```

### Pies de las capturas

1. `01-firmar-pdf.png`: Abra un PDF en un sitio permitido y pulse «Firmar PDF».
2. `02-ventana-extension.png`: La ventana de la extensión indica si GrxFirma está conectada.
3. `03-sitios-de-confianza.png`: Añada o retire sitios donde quiere ver el botón de firma.

## English

### Summary (132 characters maximum)

Chrome and Edge show the manifest description (`extensionDescription`, 86 characters):

> GrxFirma extension for signing PDF documents and proving identity to authorised sites.

If the store allows a separate summary (106 characters):

> Sign the PDFs you open in your browser with GrxFirma. Requires the GrxFirma desktop app and a certificate.

### AMO summary (250 characters maximum)

> Connects Firefox to the GrxFirma desktop app so you can sign PDFs opened on allowed sites with your own certificate. Requires GrxFirma to be installed. Documents never go through GrxFirma servers.

### Description

```text
GrxFirma is a desktop application for signing documents with an electronic certificate. This extension connects it to your browser.

What it does
• On allowed sites, a "Sign PDF" button appears when you open a PDF. Clicking it hands the document to GrxFirma, which asks you to choose a certificate and approve the signature.
• On authorised portals, it can prove your identity with your certificate. GrxFirma always asks for your approval and the private key never leaves your computer.
• Under "Trusted sites" you can add other HTTPS sites. The browser asks for permission for each one and you can withdraw it at any time.

What you need
• The GrxFirma application installed on Windows or Linux: https://github.com/aavidad/GrxFirma/releases
• An electronic certificate installed on your computer or on a smart card.
Without the application, the extension reports that it cannot connect and signs nothing.

Privacy
• The extension has no accounts and sends no documents or usage data to GrxFirma servers.
• The PDF is downloaded from the portal where it is published, using your session, and handed to the signer on your computer.
• It does not run code downloaded from the Internet.
Full policy: https://aavidad.github.io/GrxFirma/privacidad.html

GrxFirma is free software under the EUPL-1.2 licence. Source code: https://github.com/aavidad/GrxFirma
```

### Screenshot captions

1. `01-firmar-pdf.png`: Open a PDF on an allowed site and click "Sign PDF".
2. `02-ventana-extension.png`: The extension window shows whether GrxFirma is connected.
3. `03-sitios-de-confianza.png`: Add or remove the sites where you want the signing button.

## Chrome Web Store: pestaña «Privacy practices»

Chrome pide un texto por permiso. Pegar estos (en inglés, que es lo que lee el equipo de revisión).

| Campo | Texto |
|---|---|
| Single purpose | Connects the browser to the GrxFirma desktop signing application so the user can sign PDF documents opened on allowed sites and, on authorised portals, prove their identity with their own certificate after explicit approval. |
| `storage` | Stores the list of HTTPS sites the user adds under "Trusted sites", reads the list fixed by an organisation through managed storage, and keeps a PDF the user chose to sign for at most five minutes in session storage until the local signer collects it with a one-time token. |
| `nativeMessaging` | Talks to the native host `io.github.aavidad.grxfirma` installed by the GrxFirma desktop application, to check that the application is present and to request identity proofs. Certificate selection, approval and signing happen in that application; the extension cannot sign without it. |
| `scripting` | Registers the PDF detector content script on HTTPS sites the user adds in "Trusted sites" (after the browser grants the optional host permission) and unregisters it when the site is removed. It never injects code fetched from the network. |
| Host permissions | `https://*.dipgra.es/*` and `https://*.savia.net/*` are the built-in portals where the PDF button and identity proof run. `https://127.0.0.1/*` is the GrxFirma local signer on the user's own computer (port 63118): the extension opens its signer page, hands it the chosen PDF and checks the local service. The optional `https://*/*` is requested one site at a time, only when the user adds that site, and is removed when the user deletes it. |
| Remote code | No, I am not using remote code. All JavaScript is in the package. |

### Uso de datos (casillas)

Marcar lo que la extensión toca durante una operación pedida por la persona usuaria:

- Personally identifiable information: sí. El certificado público (nombre y, según el certificado, NIF) se devuelve al portal autorizado tras aprobar una prueba de identidad.
- Authentication information: sí. La firma del reto de identidad sirve para iniciar sesión en ese portal.
- Website content: sí. El PDF que la persona decide firmar.
- El resto (salud, finanzas, comunicaciones, ubicación, historial, actividad): no.

Marcar las tres certificaciones: no se venden ni se transfieren datos a terceros fuera de los casos permitidos, no se usan para fines ajenos a la función única y no se usan para evaluar solvencia ni conceder préstamos.

## Microsoft Edge Add-ons

Edge pide menos campos. Usar el mismo resumen y descripción, la misma URL de privacidad y estas respuestas:

- «¿La extensión accede a datos personales?»: Sí. Pegar el párrafo de uso de datos anterior.
- Notas para la certificación: el texto de [NOTAS_REVISORES.md](NOTAS_REVISORES.md) (sección en inglés).
- Mercados: todos, o solo España si se prefiere empezar por ahí.

## Firefox Add-ons (AMO)

- Recolección de datos: la toma del manifiesto (`personallyIdentifyingInfo` y `websiteContent`). AMO lo muestra en el instalador; no hay que marcar nada más.
- Plataformas: solo Firefox de escritorio. No marcar Firefox para Android: no admite mensajería nativa.
- «¿Usa código minificado, concatenado o generado?»: No. El XPI es una copia directa de `packaging/browser-extensions/src/firefox`, sin herramientas de compilación, así que no hace falta subir código fuente aparte.
- Notas para revisores: el texto de [NOTAS_REVISORES.md](NOTAS_REVISORES.md).
