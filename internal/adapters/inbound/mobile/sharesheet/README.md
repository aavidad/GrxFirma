<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# share-sheet

Estado: operativo como adaptador de entrada mobile.

Responsabilidad:
- recibir documentos desde la hoja de compartición de Android/iOS,
- normalizar metadatos y contenido,
- derivar la acción de firma o verificación correspondiente.

Dependencias de plataforma:
- Android Sharesheet / iOS Share Extension
- integracion nativa con `gomobile bind`

Contrato actual:
- el documento compartido se traduce a `mobile.SharedDocumentPayload`;
- la compartición múltiple puede traducirse a firma en lote mediante
  `HandleMultiple(...)`;
- la fachada mobile acepta y devuelve JSON compatible con `gomobile bind`;
- los permisos temporales y copias seguras siguen resueltos por la capa nativa de cada plataforma.

Notas:
- este adaptador no persiste por su cuenta fuera del sandbox mobile;
- la decision de firmar o verificar se delega a `application`.
