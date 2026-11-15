<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Límites conocidos y exclusiones — GrxFirma

Fecha: 2026-03-20. Última revisión: 2026-11-15.  
Autoría: Alberto Avidad Fernández
Estado: documento operativo de referencia.

---

## Objetivo

Dejar por escrito lo que **no** debe prometerse todavía sobre `GrxFirma`.

Este documento reduce riesgo comercial y jurídico:

- evita sobreventa,
- ayuda a acotar pilotos,
- y mejora la honestidad del expediente técnico.

---

## Límites actuales principales

### 1. Cumplimiento formal no cerrado

A fecha actual no debe afirmarse como cerrado:

- `ENS`
- `RGPD/LOPDGDD`
- accesibilidad formal `EN 301 549`
- expediente completo de proveedor tipo `NIS2/CRA`

---

### 2. Evidencia externa de firma aún en consolidación

Hay evidencia técnica local con material sintético (véase el último apartado),
pero faltan cierres de producto en:

- el validador DSS, que no está montado en el equipo de desarrollo (en otros, sin confirmar)
- `xmlsec1`, que a 14/11/2026 no se había ejecutado contra XAdES ni FacturaE
- un corpus externo más amplio para ciertas variantes
- cualquier validación por un tercero independiente: sin confirmar

---

### 3. Integraciones web reales aún en remate fino

La compatibilidad con portales reales está muy avanzada, pero todavía no debe venderse como universalmente cerrada sin una batería final de validación sobre:

- identificación / `selectcert`
- flujos específicos de ciertos portales
- sincronización final con algunas webs heredadas

---

### 4. Dependencias de entorno

Hay capacidades cuyo comportamiento depende del entorno del cliente:

- PKCS#11/DNIe: existe un adaptador experimental, pero no está conectado a los
  binarios de producción ni dispone de flujo PIN o firma con hardware validado.
  El paquete de Linux incluye el auxiliar de tarjetas, desactivado en la suite
  normal; el README no debe leerse como soporte de tarjetas en producción
- protección `compat` RSA: un certificado opaco del almacén del sistema puede
  firmar, pero no se ofrece como destinatario si su adaptador no permite
  desproteger. Para ese cifrado se necesita un P12/PFX RSA autorizado con
  `KeyEncipherment` o debe elegirse el perfil `alto`; importar solo el
  certificado público en Windows no añade capacidad de descifrado
- herramientas del sistema como `certutil`
- comportamiento de almacenes NSS
- validadores externos como `pdfsig`, `qpdf` o runner DSS

Esto obliga a definir requisitos por plataforma y escenario.

---

### 5. Accesibilidad no auditada formalmente

Se han corregido defectos concretos: tamaño del texto del 100 al 200 %, foco del
editor de sello y símbolos que el lector pronuncia en Qt (lote 28), y tiradores
del sello, aviso del lote y cambio de tema en WinUI (lote 27). Lo hecho en
WinUI está sin comprobar en Windows y lo de Qt, sin lector real. Android sigue
con dos pendientes en [ACCESIBILIDAD.md](ACCESIBILIDAD.md#6-qué-queda-pendiente).
Ninguna plataforma ha tenido revisión formal:

- teclado
- foco
- contraste
- lector de pantalla
- escalado

---

## Exclusiones recomendadas para ofertas o pilotos

Hasta nuevo cierre, conviene excluir o limitar expresamente:

- certificación formal ENS
- certificación formal de accesibilidad
- garantía de interoperabilidad universal con cualquier portal heredado
- uso de DNIe, tarjetas o tokens PKCS#11 en producción; no se ofrece hasta
  completar catálogo, asociación de clave, PIN, mecanismos criptográficos y
  validación con hardware real
- despliegues con requisitos regulatorios cerrados sin evaluación previa

---

## Uso correcto de este documento

Debe acompañar:

- pilotos
- propuestas técnicas
- revisiones internas
- y conversaciones de compra pública

para que nadie interprete el estado actual como un “cumplimiento total ya auditado”.


## Evidencia local añadida el 14/11/2026

La [batería local de interoperabilidad](../test/conformance/README.md) añade
PAdES básico y B-T con TSA sintética, OpenSSL para CMS/RFC 3161, pdfsig y qpdf;
XAdES enveloped y FacturaE conservan verificación interna y una comprobación
preparada para xmlsec1. Esta última no se ha ejecutado por faltar la herramienta.
El transporte TSA es HTTP en memoria; no acredita conectividad con una TSA real.
La instalación local de DSS y las pruebas que deberá ejecutar están descritas
en esa guía, pendientes de disponer del paquete y de configurar la confianza
de laboratorio. No se han enviado documentos a validadores públicos.

El [expediente de dependencias 0.0.127](dependencias/0.0.127/README.md) distingue
revisión técnica, licencias sin acreditar, escaneos incompletos y decisiones
pendientes de Alberto. No cierra las exclusiones anteriores.
