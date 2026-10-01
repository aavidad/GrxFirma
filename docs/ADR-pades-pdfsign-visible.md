<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# ADR: `pdfsign` para PAdES visible real

## Estado

Aprobado

## Contexto

La implementación PAdES nativa del repositorio genera un PDF firmado válido para
casos básicos, pero no preserva de forma robusta el PDF original cuando se
necesita firma visible con apariencia real.

El requisito de producto es:

- firmar el PDF original, no recrearlo;
- añadir un widget de firma visible;
- mantener compatibilidad con lectores PDF comunes.

## Decisión

Se permite el uso de `github.com/digitorus/pdfsign` únicamente para la ruta de
PAdES sobre PDFs reales, especialmente cuando se solicita sello visible.

La ruta nativa previa se mantiene como fallback para:

- pruebas con PDFs sintéticos mínimos;
- entornos donde la firma incremental real falle;
- compatibilidad con la batería histórica de tests.

## Consecuencias

- Se recupera una capacidad que ya existió en workspaces anteriores del proyecto.
- La app puede volver a firmar PDFs reales con apariencia visible sin destruir
  el documento original.
- La apariencia visible soporta:
  - una página concreta;
  - rangos como `1,3-5`;
  - todas las páginas con `all`.
- Esa capacidad queda expuesta de forma coherente en:
  - la CLI;
  - la GUI Qt6;
  - la consola web local `/`;
  - el firmador web local `/signer`.
- El sello generado por backend admite además:
  - metadatos `PAdES` de motivo, ubicación y contacto;
  - un `QR` opcional incrustado cuando no se usa imagen personalizada.
- Se acepta una dependencia externa acotada y justificada solo para este caso.
