<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Límites conocidos y exclusiones — GrxFirma

Fecha: 2026-03-20.  
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

Aunque existe bastante evidencia técnica, aún faltan cierres de producto en:

- algunas validaciones externas de `PAdES`
- consolidación completa del paquete DSS
- corpus externo más amplio para ciertas variantes

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
  binarios de producción ni dispone de flujo PIN o firma con hardware validado
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

Aunque se han mejorado varias ventanas y flujos, la accesibilidad sigue pendiente de revisión formal:

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
