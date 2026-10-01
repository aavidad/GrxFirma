<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# android-keystore

Estado: implementacion base operativa para Fase 6.

Puerto implementado:
- `ports.SigningKeyProvider`

Dependencias de plataforma:
- Android Keystore
- capa nativa Java/Kotlin
- soporte `gomobile bind`

Implementacion actual:
- el adaptador depende de un `Resolver` inyectable para consultar Android Keystore desde la capa Java/Kotlin
- devuelve una referencia opaca `ports.SigningKey` sin exponer APIs nativas al nucleo Go
- mantiene compatibilidad con `gomobile bind` y evita librerias externas

Limitaciones:
- la resolucion real de alias y permisos sigue dependiendo del bridge Android
- la firma hardware-backed y las politicas biométricas siguen siendo responsabilidad del SO
- si el bridge no devuelve `KeyID`, se usa como fallback el `ID` o la huella del certificado
