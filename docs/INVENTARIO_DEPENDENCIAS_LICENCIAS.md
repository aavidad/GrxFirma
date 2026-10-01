<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Inventario de dependencias y licencias — GrxFirma

Fecha inicial: 2026-03-20.
Última revisión: 2026-08-01.
Autor: Oficina de Software Libre de la Diputacion de Granada.
Estado: inventario operativo enlazado a SBOM formal.

---

## Objetivo

Tener una visión mínima y defendible de:

- dependencias directas del producto,
- dependencias indirectas relevantes,
- licencias base,
- y puntos donde todavía existen llamadas externas que deben justificarse.

La enumeración mecánica se publica como `SBOM` formal `SPDX JSON` en CI y en
cada release oficial. Este documento aporta la clasificación y los límites
operativos que una SBOM por sí sola no explica.

---

## Resumen ejecutivo

Dependencias directas declaradas en [go.mod](../go.mod):

| Dependencia | Rol en el producto | Tipo |
|-------------|--------------------|------|
| `fyne.io/fyne/v2` | UI desktop y diálogos Fyne | Directa |
| `github.com/miekg/pkcs11` | Base experimental PKCS#11 no conectada a producción | Directa |
| `github.com/gowebpki/jcs` | Canonicalización RFC 8785 de identidad reforzada | Directa |
| `software.sslmate.com/src/go-pkcs12` | Importación/lectura de PKCS#12 | Directa |

La mayor parte del resto de dependencias vienen arrastradas por:

- Fyne
- toolchain gráfica
- utilidades de test

---

## Licencias base comprobadas localmente

Comprobadas en caché local de módulos:

- `fyne.io/fyne/v2`: licencia `BSD 3-Clause`
- `github.com/miekg/pkcs11`: licencia tipo `BSD 3-Clause`
- `github.com/gowebpki/jcs`: licencia `Apache-2.0`
- `software.sslmate.com/src/go-pkcs12`: licencia tipo `BSD`

Nota:

- este inventario es válido como línea base técnica,
- pero antes de licitación conviene automatizar la verificación de licencias y añadir copia o referencia completa en el expediente.

---

## Dependencias directas

| Dependencia | Versión | Función | Riesgo / observación | Estado |
|-------------|---------|---------|----------------------|--------|
| `fyne.io/fyne/v2` | `v2.7.3` | Interfaz gráfica desktop | Dependencia grande y crítica para UX/accesibilidad; requiere revisión específica de accesibilidad y estabilidad | `PARCIAL` |
| `github.com/miekg/pkcs11` | `v1.1.2` | Base experimental de acceso a hardware criptográfico | No es alcanzable desde los binarios de producción; antes de habilitarla requiere PIN/login, asociación certificado-clave, mecanismos, aislamiento y pruebas con hardware | `PARCIAL` |
| `github.com/gowebpki/jcs` | `v1.0.1` | Canonicalización RFC 8785 del contrato `identidad-reforzada/v1` | Apache-2.0, versión fijada | `HECHO` |
| `software.sslmate.com/src/go-pkcs12` | `v0.7.2` | Importación de certificados P12 | Dependencia razonable y acotada | `HECHO` |

---

## Dependencias indirectas relevantes

Estas no se toman como bloqueo por sí solas, pero deben aparecer en una `SBOM` final:

| Grupo | Ejemplos | Observación |
|-------|----------|-------------|
| UI Fyne | `fyne.io/systray`, `github.com/godbus/dbus/v5`, `github.com/go-gl/*`, `golang.org/x/image` | Arrastradas por la capa gráfica |
| i18n y texto | `github.com/nicksnyder/go-i18n/v2`, `golang.org/x/text` | Relevantes para localización y renderizado |
| Test | `github.com/stretchr/testify` | No es dependencia operativa de runtime |
| Criptografía/stdlib extendida | `golang.org/x/crypto`, `golang.org/x/sys`, `golang.org/x/net` | Comunes, pero deben aparecer en inventario formal |

---

## Dependencias externas no-Go y herramientas del sistema

Aunque el criterio general del proyecto es evitar `exec.Command` en el núcleo, hoy existen puntos donde se usan herramientas del sistema en adaptadores o tests:

### Runtime / adaptadores

| Herramienta | Ubicación actual | Uso |
|-------------|------------------|-----|
| `certutil` | adaptadores NSS / TLS local | consulta e instalación de certificados en NSS |
| `openssl` | adaptador NSS Linux | manipulación auxiliar PKCS#12/PEM |
| `security` | adaptador macOS TLS local | acceso a almacén del sistema en macOS |
| `notify-send` u homólogos | notificación desktop Linux | aviso visual nativo |

### Test y conformidad

| Herramienta | Uso |
|-------------|-----|
| `pdfsig` | validación externa de `PAdES` |
| `qpdf` | validación estructural PDF |
| `GRXFIRMA_DSS_RUNNER` | validación DSS externa |

Conclusión:

- el núcleo de firma y verificación está bien orientado a Go nativo,
- pero el expediente de producto debe **documentar explícitamente** estas dependencias de borde y de test.

---

## Riesgos de licencia y suministro

Los riesgos que hay que controlar antes de una venta pública son:

1. dependencias indirectas no inventariadas formalmente
2. módulos de CGo / PKCS#11 dependientes del entorno cliente
3. herramientas externas del sistema no garantizadas en todos los despliegues
4. clasificación y aceptación de licencias todavía no cerradas por un responsable

---

## Qué falta para cerrar este bloque

- vincular cada dependencia con su licencia exacta y versión
- diferenciar:
  - dependencia operativa de runtime
  - dependencia solo de test
  - dependencia opcional por plataforma
- documentar qué parte del producto deja de estar soportada si falta cada herramienta externa
- conservar la revisión y el triage de la `SBOM` de cada release real

---

## Recomendación operativa

Antes de una venta pública seria, cerrar al menos:

1. tabla final y avisos de licencias
2. política y responsable de actualización de dependencias
3. justificación de cada dependencia externa de sistema
4. aceptación trazable de la `SBOM` y de sus escaneos por release
