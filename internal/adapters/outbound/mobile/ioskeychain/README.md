<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ios-keychain

Estado: implementacion base operativa para Fase 6.

Puerto implementado:
- `ports.SigningKeyProvider`

Dependencias de plataforma:
- iOS Keychain
- Secure Enclave cuando aplique
- capa nativa Swift/Objective-C
- `gomobile bind`

Implementacion actual:
- el adaptador depende de un `Resolver` inyectable para consultar Keychain/Secure Enclave desde Swift/Objective-C
- devuelve una referencia opaca `ports.SigningKey` y conserva solo metadatos minimos
- mantiene el nucleo Go libre de SDKs iOS y de librerias externas

Limitaciones:
- la resolucion real de referencias, ACLs y biometria sigue en la capa nativa iOS
- Secure Enclave se refleja como metadato, no como dependencia directa del nucleo
- si el bridge no devuelve `KeyID`, se usa como fallback el `ID` o la huella del certificado
