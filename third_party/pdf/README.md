go get github.com/digitorus/pdf

http://godoc.org/github.com/digitorus/pdf

This project is forked from rsc/pdf

## Copia incorporada en AutoFirmaV2

Copia de `github.com/digitorus/pdf` v0.1.2 (derivado de `rsc.io/pdf`) con
estas modificaciones, marcadas en el código:

- descifrado AES-128 (V4, AESV2) y AES-256 (V5, R5 y R6, AESV3), que la
  versión original no implementa y que abortaba con un pánico;
- acceso a los parámetros de cifrado para que el firmante pueda cifrar los
  objetos nuevos de una actualización incremental con la clave del documento.
- límites ante documentos maliciosos (revisión de seguridad de 13/11/2026,
  hallazgo E2-03), porque un desbordamiento de pila o una falta de memoria
  son fatales en Go y ningún `recover` los para: profundidad máxima de
  resolución de objetos indirectos (64) y de anidamiento de arrays y
  diccionarios (256); rechazo de un objeto comprimido dentro de sí mismo y de
  los ciclos de `/Extends`; `/N` del flujo de objetos acotado y salida
  descomprimida de cada flujo de objetos limitada a 32 MiB; `/Columns` del
  predictor entre 1 y 65536; error en vez de bucle sin fin ante cadenas,
  cadenas hexadecimales y arrays sin cerrar al final de un flujo de objetos;
  `Page` y `findInherited` cortan los ciclos de `/Kids` y `/Parent`, y
  `NumPage` devuelve 0 si `/Count` es negativo o mayor que el número de
  objetos del documento.

Se omite la orden de ejemplo `pdfpasswd`.
