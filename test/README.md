<!-- Derechos de autor (C) 2026 Alberto Avidad Fernández. -->
<!-- Autoría: Alberto Avidad Fernández -->
<!-- Licencia: EUPL 1.2 o posterior -->
<!-- SPDX-License-Identifier: EUPL-1.2 -->

# Directorio `test/`

Este directorio contiene los paquetes de prueba transversales y, en su raíz, un
conjunto de documentos de muestra heredados.

## Paquetes

| Paquete | Qué comprueba |
|---|---|
| `abuse/` | Entradas hostiles contra los adaptadores de entrada. |
| `conformance/` | Conformidad de firmas contra motores externos (DSS). |
| `e2e/` | Recorridos completos de firma y verificación. |
| `purity/` | Fronteras de la arquitectura hexagonal y escucha solo en loopback. |
| `regression/` | Corpus de regresión con documentos sintéticos y firmas de prueba. |

Los paquetes que dependen de herramientas externas (`qpdf`, `pdfsig`, `openssl`)
**fallan** en vez de saltarse la comprobación cuando falta la herramienta,
siempre que `GRXFIRMA_REQUIRE_EXTERNAL_TOOLS=1`. CI lo fija así a propósito:
prohíbe el verde falso.

## Documentos de muestra en la raíz

Conviven aquí entradas y salidas de firma, lo que invita a confundirlas. La
regla es: **nada de este directorio debe regenerarse con la salida de una
ejecución**. Si un artefacto firmado se actualiza con una salida nueva, deja de
detectar regresiones — pasa a comprobar que el código coincide consigo mismo.

### En uso

Los referencia `internal/adapters/outbound/desktop/signer/pades_corpus_test.go`
y `pades_dss_test.go`:

| Fichero | Papel |
|---|---|
| `prueba1.pdf` | Entrada: PDF simple. También lo usa el corpus DSS. |
| `prueba2.pdf` | Entrada: PDF simple. |
| `prueba3.pdf` | Entrada: PDF simple. |
| `prueba1_signed.pdf` | Entrada: PDF **ya firmado** con el certificado oficial de pruebas, para probar la cofirma sobre documento con firma previa. |

### Sin referenciar

Ningún fichero `.go` del repositorio los menciona. Son residuo de ejecuciones
manuales, no fixtures:

| Fichero | Tamaño |
|---|---|
| `prueba1.odt` | 9,5 KB |
| `prueba1.pdf.hash` | 64 B |
| `prueba1.pdf.hexhash` | 65 B |
Los PDF firmados manualmente con certificados personales se excluyen del
control de versiones. Las muestras firmadas que permanecen usan únicamente el
certificado oficial de pruebas.

El PDF sintético multipágina y el procedimiento para añadir sus firmas PAdES
están descritos en [regression/README.md](regression/README.md). Hasta que se
incorpore la nueva firma de pruebas, los casos PAdES que la requieren se omiten
con un mensaje explícito.

Para comprobar si la situación ha cambiado:

```bash
for f in test/*.pdf test/*.odt test/*.hash test/*.hexhash; do
  n=$(grep -rl "$(basename "$f")" --include='*.go' . | wc -l)
  printf '%-28s %s referencias\n' "$(basename "$f")" "$n"
done
```
