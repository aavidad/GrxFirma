<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ios-deeplink

Estado: operativo como adaptador de entrada mobile.

Responsabilidad:
- recibir URLs de deep link en iOS,
- validar el esquema soportado,
- traducir la petición a comandos internos del núcleo.

Dependencias de plataforma:
- iOS SDK
- capa nativa Swift/Objective-C
- `gomobile bind` como limite de integracion

Contrato actual:
- el deep link se normaliza a `mobile.RequestPayload`;
- la fachada mobile expone metodos JSON y tipos sencillos compatibles con `gomobile bind`;
- la decision entre app principal, extension o callback de plataforma sigue estando en la capa nativa iOS.

Notas:
- el esquema `afirma://` y los datos de entrada se validan en este borde;
- el nucleo no conoce APIs iOS ni tipos UIKit/Foundation.
