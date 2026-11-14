// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"strings"
	"unicode"
)

// TextoVisible prepara para mostrarlo un dato que no controla la aplicación,
// como el nombre de un certificado o de su emisor. Cambia por «?» los
// controles C0/C1, los caracteres de formato (Cf: controles de dirección como
// U+202E y los de anchura cero) y el carácter de sustitución. Así un nombre no
// puede inyectar secuencias en una terminal ni cambiar el orden visible del
// texto en una pantalla, y la persona ve que el nombre traía algo raro.
//
// Solo es para mostrar: los identificadores y huellas no pasan por aquí.
func TextoVisible(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == unicode.ReplacementChar {
			return '?'
		}
		return r
	}, s)
}
