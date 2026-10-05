<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# SBOM y baseline de cadena de suministro — GrxFirma

Fecha inicial: 2026-03-20.
Última revisión: 2026-07-26.
Autoría: Alberto Avidad Fernández
Estado: baseline manual complementada con SBOM formal de CI y release.

---

## Objetivo

Mantener una línea base legible de la composición del producto y explicar la
`SBOM` formal que generan los gates automatizados.

Este documento no sustituye el artefacto `SPDX JSON` de una release concreta,
pero sirve para:

- saber qué entra en el producto,
- distinguir runtime de test,
- y preparar auditoría de cadena de suministro.

## Automatización y publicación existentes

Estado real hoy:

- `.github/workflows/security-sbom.yml` genera con `syft` fijado un inventario
  `SPDX JSON` completo del árbol y una segunda `SBOM` del alcance distribuido;
  `grype` bloquea sobre esta última. El alcance se construye de forma
  fail-closed con los dos módulos Go y los classpaths Android
  `productionReleaseRuntimeClasspath` y
  `verificationReleaseRuntimeClasspath`, y se valida que todos sus paquetes
  Maven —y solo ellos— aparezcan en la `SBOM`;
- existe workflow de `govulncheck` en
  `.github/workflows/security-govulncheck.yml`;
- `.github/workflows/release.yml` vuelve a generar la `SBOM` formal del árbol
  fuente para el tag, valida su estructura y la publica como asset de la
  release junto con sus evidencias de integridad.

Esto cubre la generación y publicación técnica. No equivale por sí solo a un
expediente proveedor `NIS2` / `CRA`: siguen siendo necesarios responsable,
triage, clasificación de licencias y seguimiento histórico.

---

## Componentes principales del producto

| Componente | Tipo | Observación |
|-----------|------|-------------|
| `grxfirma` | Binario principal desktop/CLI | Punto de entrada principal |
| `grxfirma-afirmauri` | Binario protocolario | Integra `afirma://` y flujos legacy |
| `grxfirma-nativehost` | Binario Native Messaging | Bridge navegador/host |
| Configuración local | Fichero y variables de entorno | Afecta defaults seguros y operación |
| UIs Fyne y Qt/QML | Presentación desktop | Selector, confianza, progreso, protocolo y configuración |
| Servicio REST y web | Integración local | API autenticada, panel navegador y firma por lotes |
| Bridges navegador | Extensión y Native Messaging | Integración Chromium/Firefox con el agente local |
| Clientes móviles | Android/iOS | Proyectos nativos y bridge hacia el núcleo |

---

## Dependencias Go directas

Tomadas de [go.mod](../go.mod):

| Módulo | Versión | Función | Uso principal |
|--------|---------|---------|---------------|
| `fyne.io/fyne/v2` | `v2.7.3` | UI desktop | runtime |
| `github.com/miekg/pkcs11` | `v1.1.2` | base experimental PKCS#11 | dependencia no alcanzable desde binarios de producción |
| `software.sslmate.com/src/go-pkcs12` | `v0.7.0` | importación PKCS#12 | runtime |

---

## Dependencias Go indirectas más visibles

No exhaustivo, pero relevante:

| Grupo | Ejemplos | Tipo |
|-------|----------|------|
| UI / gráficos | `fyne.io/systray`, `github.com/go-gl/*`, `golang.org/x/image` | runtime |
| Sistema / integración | `github.com/godbus/dbus/v5`, `golang.org/x/sys` | runtime |
| Texto / i18n | `golang.org/x/text`, `github.com/nicksnyder/go-i18n/v2` | runtime |
| Test | `github.com/stretchr/testify` | test |

---

## Herramientas externas detectadas

### Runtime / sistema

| Herramienta | Motivo |
|-------------|--------|
| `certutil` | gestión NSS / confianza local |
| `openssl` | apoyo en algunas rutas NSS/P12 |
| `security` | integración de confianza/local trust en macOS |
| notificadores del sistema | notificación desktop Linux |

### Test / validación

| Herramienta | Motivo |
|-------------|--------|
| `pdfsig` | validación externa de `PAdES` |
| `qpdf` | validación estructural PDF |
| `GRXFIRMA_DSS_RUNNER` | conformidad DSS |

---

## Alcance que debe gobernarse

El inventario formal enumera el árbol fuente y el gate separado cubre las
dependencias del runtime distribuido. El expediente de producto debe clasificar
además:

- dependencias runtime obligatorias
- dependencias runtime opcionales por plataforma
- dependencias solo de test
- herramientas externas del sistema
- licencias
- versión exacta del módulo
- hash o fuente de procedencia

---

## Riesgos abiertos

1. falta clasificación formal runtime/test/licencias en el entregable final
2. falta asignar responsable y conservar el histórico de triage enlazado al inventario
3. falta demostrar, por release real, la revisión y aceptación de los informes
4. la SBOM del árbol fuente no sustituye un inventario contractual de herramientas del sistema

---

## Próximo paso recomendado

1. adjuntar licencias y clasificación runtime/test
2. asignar propietario y SLA al triage de cada release
3. enlazar la evidencia de la release con
   [INVENTARIO_DEPENDENCIAS_LICENCIAS.md](INVENTARIO_DEPENDENCIAS_LICENCIAS.md)
