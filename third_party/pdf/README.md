go get github.com/digitorus/pdf

http://godoc.org/github.com/digitorus/pdf

This project is forked from rsc/pdf

## Copia incorporada en AutoFirmaV2

Copia de `github.com/digitorus/pdf` v0.1.2 (derivado de `rsc.io/pdf`) con
estas modificaciones, marcadas en el código:

- descifrado AES-128 (V4, AESV2) y AES-256 (V5, R5 y R6, AESV3), que la
  versión original no implementa y que abortaba con un pánico;
- acceso a los parámetros de cifrado para que el firmante pueda cifrar los
  objetos nuevos de una actualización incremental con la clave del documento;
- límites ante PDF malintencionados, porque un desbordamiento de pila o una
  falta de memoria son fatales en Go y ningún `recover` los para (incluye el
  hallazgo E2-03 de la revisión de seguridad de 13/11/2026): el árbol de
  páginas (`Page`, `BuscarPagina`, `ValidarArbolPaginas` y la herencia por
  `/Parent`) rechaza ciclos, nodos repetidos y más de 256 niveles; el índice
  (`Outline`) visita cada entrada una vez; el analizador corta a 512 niveles
  de arrays y diccionarios anidados; la resolución de objetos indirectos se
  corta a 64 niveles y la de flujos de objetos a 32, con rechazo de un objeto
  comprimido dentro de sí mismo y de las cadenas `/Extends` en ciclo o
  demasiado largas; `/N` del flujo de objetos acotado y salida descomprimida
  de cada flujo de objetos limitada a 32 MiB; `/Columns` del predictor entre
  1 y 65536; error en vez de bucle sin fin ante cadenas, cadenas
  hexadecimales y arrays sin cerrar al final de un flujo de objetos, y
  `NumPage` devuelve 0 si `/Count` es negativo o mayor que el número de
  objetos del documento;
- `Page` numera las páginas hoja a hoja en orden de documento, sin fiarse de
  los `/Count` (`RecorrerHojas`, `PaginaHoja`), como los visores y como
  `pdfsign` al colocar el sello. Si el árbol no se puede recorrer así (una
  página repetida, por ejemplo), vuelve a bajar por los `/Count`.

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
