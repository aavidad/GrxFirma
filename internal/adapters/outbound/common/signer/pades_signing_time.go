// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"regexp"
	"strconv"
	"time"
)

// fechaPDF reconoce una fecha PDF con zona explícita: D:AAAAMMDDHHmmSS
// seguida de Z o de ±HH'mm'. Sin zona la hora sería ambigua y no se usa.
var fechaPDF = regexp.MustCompile(`^D:(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(Z|[+-]\d{2}'?\d{2}'?)`)

// extractPDFSigningTimeM lee /M del diccionario de firma. Las firmas PAdES
// no llevan signingTime en el CMS; /M sí está dentro del rango firmado (fuera
// del hueco de /Contents), así que la protege la firma igual que aquel
// atributo: es la hora del equipo de quien firmó. Si /M cae fuera de lo
// firmado o no tiene una fecha con zona, no hay fecha.
func extractPDFSigningTimeM(block []byte, dictStart int, byteRange [4]int) time.Time {
	text := string(block)
	for i := 0; i+2 < len(text); i++ {
		if text[i] != '/' || text[i+1] != 'M' {
			continue
		}
		j := i + 2
		if j < len(text) && text[j] != '(' && text[j] != ' ' && text[j] != '\t' && text[j] != '\r' && text[j] != '\n' {
			continue // /MDP, /Matrix…
		}
		for j < len(text) && (text[j] == ' ' || text[j] == '\t' || text[j] == '\r' || text[j] == '\n') {
			j++
		}
		if j >= len(text) || text[j] != '(' {
			continue
		}
		fin := j + 1
		for fin < len(text) && text[fin] != ')' && fin-j < 64 {
			fin++
		}
		if fin >= len(text) || text[fin] != ')' {
			return time.Time{}
		}
		pos := dictStart + i
		dentro := (pos >= byteRange[0] && dictStart+fin < byteRange[0]+byteRange[1]) ||
			(pos >= byteRange[2] && dictStart+fin < byteRange[2]+byteRange[3])
		if !dentro {
			return time.Time{}
		}
		return leerFechaPDF(text[j+1 : fin])
	}
	return time.Time{}
}

func leerFechaPDF(valor string) time.Time {
	m := fechaPDF.FindStringSubmatch(valor)
	if m == nil {
		return time.Time{}
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	zona := time.UTC
	if m[7] != "Z" {
		digitos := []byte{}
		for _, c := range []byte(m[7][1:]) {
			if c >= '0' && c <= '9' {
				digitos = append(digitos, c)
			}
		}
		desfase := (n(string(digitos[:2]))*60 + n(string(digitos[2:]))) * 60
		if desfase > 14*3600 {
			return time.Time{}
		}
		if m[7][0] == '-' {
			desfase = -desfase
		}
		zona = time.FixedZone("", desfase)
	}
	fecha := time.Date(n(m[1]), time.Month(n(m[2])), n(m[3]), n(m[4]), n(m[5]), n(m[6]), 0, zona)
	// time.Date normaliza valores fuera de rango (mes 13…): se rechazan.
	if fecha.Year() != n(m[1]) || int(fecha.Month()) != n(m[2]) || fecha.Day() != n(m[3]) ||
		fecha.Hour() != n(m[4]) || fecha.Minute() != n(m[5]) || fecha.Second() != n(m[6]) {
		return time.Time{}
	}
	return fecha.UTC()
}
