<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# mobile-document-picker

Estado: implementacion base operativa para Fase 6.

Puerto implementado:
- `ports.DocumentPicker`

Dependencias de plataforma:
- Android SAF
- iOS UIDocumentPicker
- bridge nativo mediante `gomobile bind`

Implementacion actual:
- el adaptador depende de un `Resolver` inyectable para hablar con Android SAF o `UIDocumentPicker`
- normaliza nombre y MIME por defecto cuando la capa nativa no los informa
- mantiene el nucleo Go libre de SDKs Android/iOS y de librerias externas

Limitaciones:
- la seleccion real sigue dependiendo del bridge nativo expuesto via `gomobile bind`
- los permisos persistentes o temporales siguen siendo responsabilidad de la capa de plataforma
- la integracion con share-sheet y deep links se resuelve en los adaptadores inbound mobile
