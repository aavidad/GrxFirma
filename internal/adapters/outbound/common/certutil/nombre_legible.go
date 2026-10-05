// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certutil

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"strings"
)

// pkix.Name.String() solo da nombre a nueve atributos; el resto (nombre y
// apellidos de los certificados de la FNMT, organizationIdentifier, title…)
// sale como OID con el valor DER en hexadecimal: «2.5.4.42=#1306…». Estas
// funciones escriben el mismo nombre distinguido (mismo orden, mismo formato
// RFC 4514) con el nombre del atributo y su texto. Son solo para mostrar:
// para comparar o identificar un certificado se usa su huella o su DER.

// nombresAtributosDN son los atributos que pkix no nombra. Los nueve que sí
// nombra (CN, SERIALNUMBER, C, L, ST, STREET, O, OU, POSTALCODE) conservan
// la etiqueta de Go para no cambiar lo que ya se mostraba.
var nombresAtributosDN = map[string]string{
	"2.5.4.4":                    "SN",
	"2.5.4.12":                   "title",
	"2.5.4.13":                   "description",
	"2.5.4.15":                   "businessCategory",
	"2.5.4.41":                   "name",
	"2.5.4.42":                   "GN",
	"2.5.4.43":                   "initials",
	"2.5.4.44":                   "generationQualifier",
	"2.5.4.45":                   "uniqueIdentifier",
	"2.5.4.46":                   "dnQualifier",
	"2.5.4.65":                   "pseudonym",
	"2.5.4.97":                   "organizationIdentifier",
	"1.2.840.113549.1.9.1":       "emailAddress",
	"0.9.2342.19200300.100.1.1":  "UID",
	"0.9.2342.19200300.100.1.25": "DC",
	"1.3.6.1.4.1.311.60.2.1.3":   "jurisdictionC",
}

// NombreLegible es pkix.Name.String() con nombre y texto en los atributos
// que Go deja como OID y hexadecimal.
func NombreLegible(n pkix.Name) string {
	return DNLegible(n.String())
}

// DNLegible reescribe un nombre distinguido en formato RFC 4514 (como lo
// escribe crypto/x509): da nombre a los OID conocidos y decodifica los
// valores «#…» que son cadenas de texto ASN.1. Lo que no reconoce lo deja
// tal cual.
func DNLegible(dn string) string {
	if !strings.Contains(dn, "=") {
		return dn
	}
	var out strings.Builder
	for _, parte := range trocearDN(dn) {
		out.WriteString(atributoLegible(parte.texto))
		if parte.separador != 0 {
			out.WriteByte(parte.separador)
		}
	}
	return out.String()
}

type parteDN struct {
	texto     string
	separador byte
}

// trocearDN separa los atributos por «,» o «+» sin escapar y conserva el
// separador para recomponer el nombre igual.
func trocearDN(dn string) []parteDN {
	var partes []parteDN
	inicio := 0
	for i := 0; i < len(dn); i++ {
		switch dn[i] {
		case '\\':
			i++
		case ',', '+':
			partes = append(partes, parteDN{texto: dn[inicio:i], separador: dn[i]})
			inicio = i + 1
		}
	}
	return append(partes, parteDN{texto: dn[inicio:]})
}

func atributoLegible(atributo string) string {
	tipo, valor, ok := strings.Cut(atributo, "=")
	if !ok {
		return atributo
	}
	clave := strings.TrimSpace(tipo)
	if nombre, conocido := nombresAtributosDN[clave]; conocido {
		tipo = strings.Replace(tipo, clave, nombre, 1)
	}
	if texto, ok := textoDER(strings.TrimSpace(valor)); ok {
		valor = escaparValorDN(texto)
	}
	return tipo + "=" + valor
}

// textoDER decodifica «#<DER en hexadecimal>» cuando es una cadena ASN.1
// (PrintableString, UTF8String, IA5String, BMPString…).
func textoDER(valor string) (string, bool) {
	if !strings.HasPrefix(valor, "#") {
		return "", false
	}
	der, err := hex.DecodeString(valor[1:])
	if err != nil {
		return "", false
	}
	var texto string
	if resto, err := asn1.Unmarshal(der, &texto); err != nil || len(resto) != 0 {
		return "", false
	}
	return texto, true
}

// escaparValorDN aplica los mismos escapes RFC 4514 que crypto/x509.
func escaparValorDN(valor string) string {
	var out strings.Builder
	ultimo := len(valor) - 1
	for i, c := range valor {
		escapar := false
		switch c {
		case ',', '+', '"', '\\', '<', '>', ';':
			escapar = true
		case ' ':
			escapar = i == 0 || i == ultimo
		case '#':
			escapar = i == 0
		}
		if escapar {
			out.WriteByte('\\')
		}
		out.WriteRune(c)
	}
	return out.String()
}
