<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# SBOM y baseline de cadena de suministro — GrxFirma

Fecha inicial: 2026-03-20.
Última revisión: 2026-11-14.
Autoría: Alberto Avidad Fernández
Estado: baseline con expediente generado por versión y SBOM formal de CI/release.

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

Selección tomada de [go.mod](../go.mod); la enumeración completa de manifiestos
está en el [expediente generado](dependencias/0.0.127/inventario.json):

| Módulo | Versión | Función | Uso principal |
|--------|---------|---------|---------------|
| `fyne.io/fyne/v2` | `v2.7.3` | UI desktop | runtime |
| `github.com/miekg/pkcs11` | `v1.1.2` | base experimental PKCS#11 | dependencia no alcanzable desde binarios de producción |
| `software.sslmate.com/src/go-pkcs12` | `v0.7.2` | importación PKCS#12 | runtime |

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

1. quedan 38 licencias iniciales sin acreditar y versiones de kits/SDK por
   atestar; la clasificación fuente ya está generada;
2. falta asignar responsable y obtener la aceptación del triage inicial;
3. faltan bases locales Maven/NuGet y Grype actual para completar la revisión;
4. falta vincular las evidencias fuente a los artefactos publicados de cada
   plataforma y cerrar su inventario contractual de herramientas.

---

## Próximo paso recomendado

Ya existen la clasificación por versión, el procedimiento sin red y la
[plantilla de próxima versión](dependencias/proxima/README.md). Queda:

1. cerrar las 38 licencias pendientes y adjuntar avisos y SPDX de los
   artefactos publicados, que el expediente 0.0.127 no examinó;
2. conseguir las bases locales de Maven y NuGet y una de Grype reciente, y
   repetir esos escaneos;
3. que Alberto asigne propietario y decida los compromisos de soporte y triage;
4. abrir el expediente de la siguiente versión con la plantilla y conservar el
   histórico de evidencias.


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
