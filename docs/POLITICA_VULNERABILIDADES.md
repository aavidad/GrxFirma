<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Política de vulnerabilidades y mantenimiento — GrxFirma

Fecha inicial: 2026-03-20.
Última revisión: 2026-11-14.
Autoría: Alberto Avidad Fernández
Estado: documento operativo vivo.

---

## Objetivo

Definir una base mínima para que `GrxFirma` sea defendible como producto mantenible y seguro ante:

- sector público,
- exigencias tipo `NIS2`,
- y madurez esperable de proveedor bajo `Cyber Resilience Act`.

---

## Principios

1. No ocultar vulnerabilidades conocidas.
2. Corregir primero lo explotable y lo que afecte a firma, confianza o protección de datos.
3. Mantener trazabilidad de:
   - detección
   - evaluación
   - mitigación
   - corrección
4. Diferenciar claramente:
   - fallo funcional
   - fallo de seguridad
   - limitación conocida

---

## Clasificación inicial

| Nivel | Criterio orientativo | Ejemplos |
|-------|----------------------|----------|
| `Crítica` | Permite comprometer firma, claves, confianza o datos sensibles | bypass de confianza, fuga de claves, ejecución remota |
| `Alta` | Impacto fuerte en seguridad o integridad del proceso | validación incorrecta de firma, fuga de datos en logs |
| `Media` | Debilidad explotable con condiciones o impacto limitado | hardening insuficiente, dependencia vulnerable no explotada directamente |
| `Baja` | Riesgo menor o más bien de robustez | mensajes inseguros, defaults mejorables |

---

## Flujo recomendado

1. registrar incidencia
2. clasificar severidad
3. evaluar alcance:
   - plataformas afectadas
   - formatos afectados
   - datos afectados
4. decidir mitigación temporal si hace falta
5. corregir
6. añadir prueba de regresión
7. documentar cierre

---

## Compromisos mínimos que debería poder asumir el proyecto

- mantener inventario de dependencias
- revisar dependencias y CVEs conocidas periódicamente
- disponer de canal de reporte responsable
- corregir con prioridad incidentes en:
  - firma
  - validación
  - confianza
  - tratamiento de datos

---

## Evidencias que faltan hoy

Estado actual: `BASE DEFINIDA`

Entrada publica resumida del estado real:

- [../SECURITY.md](../SECURITY.md)

Capacidades presentes:

- canal de contacto por correo para informar de vulnerabilidades, documentado
  en [../SECURITY.md](../SECURITY.md), que no fija plazos de respuesta ni una
  política LTS;
- `SBOM` `SPDX JSON` de CI y release;
- gates `govulncheck`, `grype` y OSV por ecosistema.

Lineas de trabajo abiertas:

- asignación formal de propietario para triage y aceptación de release;
- plazos de respuesta (SLA) y matriz de soporte por versiones, que hoy no
  existen y que `SECURITY.md` tampoco promete;
- enlazar desde `SECURITY.md` las auditorías internas y sus pruebas de
  regresión, si se decide publicarlas;
- ampliar el histórico inicial del expediente 0.0.127 a incidencias y backports
  de cada versión mantenida;
- cierre del expediente proveedor `NIS2` / `CRA`.

---

## Medidas inmediatas recomendadas

1. definir responsable de revisión de dependencias y seguridad
2. registrar el triage y la aceptación de cada release real
3. acordar ventanas de soporte y backport por versión
4. documentar las evidencias de revisión de cada versión en su expediente de publicación


## Expediente por versión (revisión 14/11/2026)

La clasificación generada, las licencias por identidad y versión, los informes
locales revisados, las herramientas del sistema y sus límites se conservan en
[el expediente 0.0.127](dependencias/0.0.127/README.md).
[El procedimiento reproducible](dependencias/README.md) amplía los inventarios
y controles existentes; la [próxima versión](dependencias/proxima/README.md)
tiene plantilla preparada. El control detecta dependencias nuevas sin licencia
o uso revisados y conserva visibles 38 licencias pendientes iniciales de
Qt/NuGet. La aceptación de riesgos, los responsables y los compromisos de
soporte siguen pendientes de Alberto. No equivale a cierre contractual ni
a aceptación de todos los informes de la release publicada.
