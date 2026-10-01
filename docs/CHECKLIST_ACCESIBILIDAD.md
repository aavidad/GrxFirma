<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Checklist de accesibilidad — GrxFirma

Fecha inicial: 2026-03-20.
Última revisión: 2026-07-26.
Autor: Oficina de Software Libre de la Diputacion de Granada.
Estado: checklist operativo vivo.

---

## Objetivo

Preparar `GrxFirma` para una revisión de accesibilidad alineada con:

- `Directiva (UE) 2016/2102`
- `RD 1112/2018`
- `EN 301 549`

Esta checklist no es una auditoría formal, pero sí marca:

- qué revisar,
- qué parece bien orientado,
- qué huecos visibles hay hoy,
- y qué pruebas manuales habría que ejecutar.

---

## Ámbito

Se centra en:

- UI principal Qt/QML y superficies locales web
- UI Fyne legacy
- diálogos de confianza
- selector de certificados
- ventanas protocolarias
- mensajes de error y progreso

No cubre todavía:

- web externa
- lectores PDF de terceros
- componentes nativos fuera del binario principal

---

## Estado inicial observado

Estado reconciliado:

- Qt/QML expone nombres/roles accesibles y contratos automatizados;
- `scripts/accessibility/run-atspi-audit.sh` inspecciona el árbol vivo y el job
  Linux de `Compatibility` lo ejecuta con AT-SPI/Xvfb;
- estado y selección mantienen texto/ícono además del color;
- siguen sin ejecutarse lector de pantalla, contraste, DPI y teclado sobre los
  paquetes finales de Linux, Windows y macOS.

Conclusión inicial:

- estado general: `PARCIAL`

---

## Checklist

| Área | Qué hay que cumplir | Estado | Evidencia actual | Hueco principal | Próximo paso |
|------|---------------------|--------|------------------|-----------------|--------------|
| Navegación por teclado | Todo el flujo debe poder usarse sin ratón | `PARCIAL` | Contratos Qt y árbol AT-SPI automatizado | Falta recorrido manual completo por plataforma | Ejecutar pruebas manuales de teclado |
| Foco visible | El elemento enfocado debe ser claramente visible | `PARCIAL` | Controles Qt accesibles y auditables | Falta evidencia visual/manual en paquetes finales | Revisar foco visible en todos los diálogos |
| Lectura comprensible | Textos claros, no ambiguos, consistentes | `PARCIAL` | Textos en castellano y mensajes más claros recientes | Falta revisión de consistencia y densidad de mensajes | Hacer revisión editorial de UX |
| Tamaño legible | Texto y controles legibles en pantallas normales | `PARCIAL` | Se ampliaron varias ventanas y tamaños de texto | No hay criterio ni prueba sistemática | Definir tamaño mínimo aceptable y probar |
| Contraste | Colores con contraste suficiente | `PENDIENTE` | No hay medición | Riesgo en etiquetas de estado y selección | Revisar contraste con tema claro/oscuro |
| Mensajes de error | Deben ser comprensibles y accionables | `PARCIAL` | Hay esfuerzos de sanitización y mensajes en castellano | Falta revisión UX de errores reales | Auditar errores frecuentes del flujo web |
| Confirmaciones críticas | Diálogos claros y no engañosos | `PARCIAL` | Existe diálogo TOFU y selección explícita | Falta validación de accesibilidad y claridad legal | Revisar textos y foco |
| Progreso de operación | El usuario debe saber qué está pasando | `PARCIAL` | Existen diálogos y ventana protocolaria | La semántica de progreso aún ha sido inestable | Revisar feedback de estado estable |
| Cancelación | Debe existir salida clara y segura | `PARCIAL` | Hay botones de cancelar/cierre | Falta prueba sistemática de cancelación y foco | Probar cancelación por teclado y cierre |
| Dependencia de color | No basar estado solo en colores | `PARCIAL` | Estados principales conservan texto/ícono | Falta revisión exhaustiva de todas las pantallas | Auditar temas y estados |
| Compatibilidad con lector de pantalla | Al menos no bloquear tecnologías de asistencia | `PARCIAL` | Gate AT-SPI del árbol Qt en CI Linux | Falta interacción con lector real y sistemas restantes | Probar lectores en Linux/Windows/macOS |
| Escalado y DPI | Debe soportar escalado razonable | `PENDIENTE` | No documentado | Riesgo en layouts recientes grandes | Probar en escalado 100/125/150% |

---

## Pruebas manuales mínimas recomendadas

### Flujo 1 — confianza de dominio

- abrir diálogo TOFU
- navegar solo con teclado
- cambiar foco entre botones
- activar decisión sin ratón
- comprobar visibilidad del foco

### Flujo 2 — selección de certificado

- abrir selector
- moverse por lista sin ratón
- filtrar por texto
- seleccionar certificado por teclado
- confirmar selección
- cancelar sin ratón

### Flujo 3 — operación protocolaria

- lanzar `afirma://`
- comprobar legibilidad del estado
- verificar que no desaparece información importante demasiado rápido
- confirmar que los errores son entendibles

---

## Riesgos actuales

1. mejoras visuales recientes sin auditoría real de foco y teclado
2. posibles dependencias de color en estados de certificado
3. falta de evidencia de lector de pantalla
4. falta de declaración de accesibilidad

---

## Criterio de cierre mínimo

No marcar este bloque como cerrado hasta tener:

- revisión manual de teclado y foco
- revisión de contraste
- checklist firmada de los tres flujos críticos
- límites conocidos documentados
