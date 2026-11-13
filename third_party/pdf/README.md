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

Base: github.com/digitorus/pdf@v0.1.2

Cambios posteriores a la incorporación (ver `git log -- third_party/pdf`):

- `revisions.go` y cambios en `read.go`: lectura de las revisiones
  incrementales de un PDF para el dictamen de verificación v2 (commit 4fe9229);
- ajuste en `read.go` señalado por gosec (commit 7c4c397);
- `go.mod` con la versión de Go del proyecto (commit b3d9120).

### Vigilancia de vulnerabilidades

Como `go.mod` sustituye el módulo por este directorio (`replace`), los
escáneres no comparan la copia con los avisos publicados para
`github.com/digitorus/pdf`. El flujo `security-dependencies.yml` lee la línea
`Base:` de este fichero, monta un `go.mod` sintético con esa versión y lo pasa
por OSV-Scanner. Si aparece un aviso, hay que revisar a mano si el código
afectado sigue en la copia y anotar aquí la decisión.
