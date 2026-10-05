// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package informeverificacion

import (
	"encoding/asn1"
	"encoding/hex"
	"strings"
)

// nombreDistinguido son los atributos de un nombre distinguido (RFC 4514,
// como lo escribe crypto/x509) que una persona necesita leer: nombre,
// identificador y organización. El DN completo queda para los detalles
// técnicos del informe.
type nombreDistinguido struct {
	comun, identificador, organizacion, nombre, apellidos string
}

func leerNombreDistinguido(dn string) nombreDistinguido {
	var n nombreDistinguido
	for _, componente := range componentesDN(dn) {
		tipo, valor, ok := strings.Cut(componente, "=")
		if !ok {
			continue
		}
		valor = valorDN(strings.TrimSpace(valor))
		if valor == "" {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(tipo)) {
		case "CN", "2.5.4.3":
			n.comun = primero(n.comun, valor)
		case "SERIALNUMBER", "2.5.4.5":
			n.identificador = primero(n.identificador, valor)
		case "O", "2.5.4.10":
			n.organizacion = primero(n.organizacion, valor)
		case "GIVENNAME", "GN", "2.5.4.42":
			n.nombre = primero(n.nombre, valor)
		case "SN", "SURNAME", "2.5.4.4":
			n.apellidos = primero(n.apellidos, valor)
		}
	}
	return n
}

// legible es el nombre que se muestra: el CN o, si falta, nombre y
// apellidos; en último caso, el DN recibido.
func (n nombreDistinguido) legible(dn string) string {
	if n.comun != "" {
		return n.comun
	}
	if completo := strings.TrimSpace(n.nombre + " " + n.apellidos); completo != "" {
		return completo
	}
	return dn
}

func primero(actual, nuevo string) string {
	if actual != "" {
		return actual
	}
	return nuevo
}

// componentesDN separa por comas, punto y coma o «+» no escapados.
func componentesDN(dn string) []string {
	var out []string
	var actual strings.Builder
	for i := 0; i < len(dn); i++ {
		c := dn[i]
		if c == '\\' && i+1 < len(dn) {
			actual.WriteByte(c)
			actual.WriteByte(dn[i+1])
			i++
			continue
		}
		if c == ',' || c == ';' || c == '+' {
			out = append(out, actual.String())
			actual.Reset()
			continue
		}
		actual.WriteByte(c)
	}
	return append(out, actual.String())
}

// valorDN deshace los escapes RFC 4514. Un valor «#…» es el DER del
// atributo en hexadecimal (crypto/x509 lo escribe así para los OID que no
// conoce, como nombre y apellidos): se decodifica si es una cadena de texto.
func valorDN(valor string) string {
	if strings.HasPrefix(valor, "#") {
		der, err := hex.DecodeString(valor[1:])
		if err != nil {
			return ""
		}
		var texto string
		if resto, err := asn1.Unmarshal(der, &texto); err != nil || len(resto) != 0 {
			return ""
		}
		return texto
	}
	var out strings.Builder
	for i := 0; i < len(valor); i++ {
		if valor[i] == '\\' && i+1 < len(valor) {
			if i+2 < len(valor) && esHex(valor[i+1]) && esHex(valor[i+2]) {
				b, _ := hex.DecodeString(valor[i+1 : i+3])
				out.Write(b)
				i += 2
				continue
			}
			out.WriteByte(valor[i+1])
			i++
			continue
		}
		out.WriteByte(valor[i])
	}
	return out.String()
}

func esHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
