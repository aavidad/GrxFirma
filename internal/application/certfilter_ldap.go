// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"strings"
)

type pkixAttribute = pkix.AttributeTypeAndValue

// Filtro LDAP (RFC 2254) sobre los atributos de un nombre X.500, como los
// filtros issuer.rfc2254/subject.rfc2254 de AutoFirma Java, por ejemplo:
// (&(!(CN=CiberCentro*))(!(O=Gobierno de Canarias))). Admite &, |, ! y
// comparaciones de igualdad con comodines "*", sin distinguir mayúsculas.

var atributosDN = map[string]asn1.ObjectIdentifier{
	"CN":                     {2, 5, 4, 3},
	"SN":                     {2, 5, 4, 4},
	"SURNAME":                {2, 5, 4, 4},
	"SERIALNUMBER":           {2, 5, 4, 5},
	"C":                      {2, 5, 4, 6},
	"L":                      {2, 5, 4, 7},
	"ST":                     {2, 5, 4, 8},
	"STREET":                 {2, 5, 4, 9},
	"O":                      {2, 5, 4, 10},
	"OU":                     {2, 5, 4, 11},
	"T":                      {2, 5, 4, 12},
	"TITLE":                  {2, 5, 4, 12},
	"GIVENNAME":              {2, 5, 4, 42},
	"G":                      {2, 5, 4, 42},
	"PSEUDONYM":              {2, 5, 4, 65},
	"ORGANIZATIONIDENTIFIER": {2, 5, 4, 97},
	"E":                      {1, 2, 840, 113549, 1, 9, 1},
	"EMAILADDRESS":           {1, 2, 840, 113549, 1, 9, 1},
	"DC":                     {0, 9, 2342, 19200300, 100, 1, 25},
	"UID":                    {0, 9, 2342, 19200300, 100, 1, 1},
}

const maxProfundidadLDAP = 32

func cumpleLDAP(filtro string, names []pkix.AttributeTypeAndValue) bool {
	p := &parserLDAP{s: strings.TrimSpace(filtro)}
	resultado, err := p.filtro(names, 0)
	if err != nil || p.pos != len(p.s) {
		// Un filtro mal formado no debe dejar pasar certificados.
		return false
	}
	return resultado
}

type parserLDAP struct {
	s   string
	pos int
}

func (p *parserLDAP) filtro(names []pkix.AttributeTypeAndValue, profundidad int) (bool, error) {
	if profundidad > maxProfundidadLDAP {
		return false, fmt.Errorf("filtro LDAP demasiado anidado")
	}
	if p.pos >= len(p.s) || p.s[p.pos] != '(' {
		return false, fmt.Errorf("se esperaba '('")
	}
	p.pos++
	if p.pos >= len(p.s) {
		return false, fmt.Errorf("filtro LDAP incompleto")
	}
	var (
		r   bool
		err error
	)
	switch p.s[p.pos] {
	case '&', '|':
		op := p.s[p.pos]
		p.pos++
		r = op == '&'
		n := 0
		for p.pos < len(p.s) && p.s[p.pos] == '(' {
			v, err := p.filtro(names, profundidad+1)
			if err != nil {
				return false, err
			}
			if op == '&' {
				r = r && v
			} else {
				r = r || v
			}
			n++
		}
		if n == 0 {
			return false, fmt.Errorf("operador LDAP sin operandos")
		}
	case '!':
		p.pos++
		v, err := p.filtro(names, profundidad+1)
		if err != nil {
			return false, err
		}
		r = !v
	default:
		fin := strings.IndexByte(p.s[p.pos:], ')')
		if fin < 0 {
			return false, fmt.Errorf("filtro LDAP sin cerrar")
		}
		r, err = comparacionLDAP(p.s[p.pos:p.pos+fin], names)
		if err != nil {
			return false, err
		}
		p.pos += fin
	}
	if p.pos >= len(p.s) || p.s[p.pos] != ')' {
		return false, fmt.Errorf("se esperaba ')'")
	}
	p.pos++
	return r, nil
}

func comparacionLDAP(item string, names []pkix.AttributeTypeAndValue) (bool, error) {
	attr, valor, ok := strings.Cut(item, "=")
	if !ok || strings.HasSuffix(attr, "~") || strings.HasSuffix(attr, ">") || strings.HasSuffix(attr, "<") {
		return false, fmt.Errorf("solo se admiten comparaciones de igualdad")
	}
	attr = strings.ToUpper(strings.TrimSpace(attr))
	oid, conocido := atributosDN[attr]
	if !conocido {
		var err error
		if oid, err = parsearOID(attr); err != nil {
			return false, fmt.Errorf("atributo LDAP desconocido: %s", attr)
		}
	}
	patron := strings.ToLower(desescaparLDAP(strings.TrimSpace(valor)))
	for _, n := range names {
		if !n.Type.Equal(oid) {
			continue
		}
		if patron == "*" || coincideComodin(strings.ToLower(fmt.Sprint(n.Value)), patron) {
			return true, nil
		}
	}
	return false, nil
}

func parsearOID(s string) (asn1.ObjectIdentifier, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "OID."), "oid.")
	var oid asn1.ObjectIdentifier
	for _, parte := range strings.Split(s, ".") {
		var n int
		if _, err := fmt.Sscanf(parte, "%d", &n); err != nil || fmt.Sprint(n) != parte {
			return nil, fmt.Errorf("OID inválido")
		}
		oid = append(oid, n)
	}
	if len(oid) < 2 {
		return nil, fmt.Errorf("OID inválido")
	}
	return oid, nil
}

// desescaparLDAP interpreta los escapes \XX de RFC 2254.
func desescaparLDAP(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+2 < len(v) {
			var c byte
			if _, err := fmt.Sscanf(v[i+1:i+3], "%02x", &c); err == nil {
				if c == '*' {
					b.WriteString("\x00")
				} else {
					b.WriteByte(c)
				}
				i += 2
				continue
			}
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// coincideComodin compara con "*" como comodín; un "*" escapado se codifica
// como \x00 y se compara literalmente.
func coincideComodin(texto, patron string) bool {
	partes := strings.Split(patron, "*")
	for i := range partes {
		partes[i] = strings.ReplaceAll(partes[i], "\x00", "*")
	}
	if len(partes) == 1 {
		return texto == partes[0]
	}
	if !strings.HasPrefix(texto, partes[0]) {
		return false
	}
	resto := texto[len(partes[0]):]
	for _, parte := range partes[1 : len(partes)-1] {
		i := strings.Index(resto, parte)
		if i < 0 {
			return false
		}
		resto = resto[i+len(parte):]
	}
	return strings.HasSuffix(resto, partes[len(partes)-1])
}
