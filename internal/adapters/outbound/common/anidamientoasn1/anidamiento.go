// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package anidamientoasn1 comprueba, sin recursión, la profundidad de
// anidamiento de una codificación ASN.1 BER o DER antes de dársela a un
// analizador recursivo (ber2der de digitorus/pkcs7, normalizeBERToDER).
// Un "30 80 30 80 ..." de millones de niveles agota la pila de la gorrutina,
// y eso es fatal en Go: ningún recover lo para (hallazgo E2-05).
package anidamientoasn1

import "errors"

// MaxProfundidad es el anidamiento que admiten los llamadores. Un CMS o un
// sello de tiempo reales no pasan de unos 20 niveles.
const MaxProfundidad = 64

// ErrDemasiadoProfundo indica un anidamiento mayor que el permitido.
var ErrDemasiadoProfundo = errors.New("anidamiento ASN.1 demasiado profundo")

// ErrMalFormado indica una codificación que no se puede recorrer.
var ErrMalFormado = errors.New("codificación ASN.1 mal formada")

// marco es un elemento construido abierto: fin es el desplazamiento de su
// final, o -1 si su longitud es indefinida.
type marco struct{ fin int }

// Comprobar recorre datos elemento a elemento y devuelve ErrDemasiadoProfundo
// si algún elemento construido queda a más de max niveles, o ErrMalFormado si
// la codificación no se puede recorrer. Admite varios elementos seguidos en el
// nivel superior. La memoria usada es proporcional a max, no a los datos.
func Comprobar(datos []byte, max int) error {
	pila := make([]marco, 0, 16)
	pos := 0
	for {
		// Cerrar los elementos de longitud definida que terminan aquí.
		for len(pila) > 0 && pila[len(pila)-1].fin >= 0 && pos >= pila[len(pila)-1].fin {
			if pos > pila[len(pila)-1].fin {
				return ErrMalFormado
			}
			pila = pila[:len(pila)-1]
		}
		if len(pila) > 0 && pila[len(pila)-1].fin < 0 && pos+1 < len(datos) && datos[pos] == 0 && datos[pos+1] == 0 {
			pos += 2
			pila = pila[:len(pila)-1]
			continue
		}
		if pos >= len(datos) {
			if len(pila) != 0 {
				return ErrMalFormado
			}
			return nil
		}
		construido := datos[pos]&0x20 != 0
		if datos[pos]&0x1f == 0x1f {
			// Etiqueta de varios bytes: los de continuación llevan el bit alto.
			pos++
			for pos < len(datos) && datos[pos]&0x80 != 0 {
				pos++
			}
		}
		pos++
		if pos >= len(datos) {
			return ErrMalFormado
		}
		l := int(datos[pos])
		pos++
		longitud := -1
		switch {
		case l == 0x80:
			if !construido {
				return ErrMalFormado
			}
		case l < 0x80:
			longitud = l
		default:
			n := l & 0x7f
			if n > 4 || pos+n > len(datos) {
				return ErrMalFormado
			}
			longitud = 0
			for i := 0; i < n; i++ {
				longitud = longitud<<8 | int(datos[pos+i])
			}
			pos += n
		}
		if longitud > len(datos)-pos {
			return ErrMalFormado
		}
		if !construido {
			pos += longitud
			continue
		}
		fin := -1
		if longitud >= 0 {
			fin = pos + longitud
			if len(pila) > 0 && pila[len(pila)-1].fin >= 0 && fin > pila[len(pila)-1].fin {
				return ErrMalFormado
			}
		}
		if len(pila) >= max {
			return ErrDemasiadoProfundo
		}
		pila = append(pila, marco{fin: fin})
	}
}
