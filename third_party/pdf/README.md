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

Se omite la orden de ejemplo `pdfpasswd`.
