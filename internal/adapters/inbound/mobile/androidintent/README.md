<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# android-intent

Estado: operativo como adaptador de entrada mobile.

Responsabilidad:
- recibir un `Intent` de Android,
- extraer accion, metadatos documentales y payload,
- traducirlo a comandos internos en la capa anticorrupcion mobile.

Dependencias de plataforma:
- Android SDK
- capa nativa Java/Kotlin
- `gomobile bind` como limite de integracion con el nucleo Go

Contrato actual:
- el borde Android traduce el `Intent` a `mobile.RequestPayload`;
- la fachada mobile publica JSON y tipos sencillos compatibles con `gomobile bind`;
- `ACTION_SEND_MULTIPLE` ya puede traducirse a lote interno mediante
  `HandleSharedMultiple(...)` preservando descriptores por entrada;
- la resolucion de `content://` y permisos SAF sigue siendo responsabilidad de la capa nativa Android.

Notas:
- este adaptador no expone puertos ni acopla Android al nucleo;
- la logica de firma, verificacion y sesion sigue quedando en `internal/application`.
