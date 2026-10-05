// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package informeverificacion

import (
	"regexp"
	"strconv"
	"strings"

	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/adapters/outbound/common/localizador"
)

const prefijoDetalle = "verificacion.detalle."

var (
	coberturaFirmaPDF = regexp.MustCompile(`^cobertura_firma_pdf_(\d{1,4})$`)
	revisionHasta     = regexp.MustCompile(`^revision_hasta_(\d{1,12})_de_(\d{1,12})$`)
	cadenaFirmante    = regexp.MustCompile(`^signer\[(\d{1,4})\]\.chain_length$`)
)

// TraducirDetalle convierte una evidencia técnica del verificador
// («formato_detectado=PAdES») en una línea del catálogo del idioma
// («Detected format: PAdES»). Las frases ya catalogadas se traducen por su
// literal; lo que el catálogo no conoce se deja tal cual.
func TraducirDetalle(loc *localizador.Localizador, linea string) string {
	clave, valor, ok := strings.Cut(linea, "=")
	if !ok || clave == "" || strings.ContainsAny(clave, " \t") {
		return loc.T(linea)
	}
	linea = detalleConDNLegible(linea)
	_, valor, _ = strings.Cut(linea, "=")
	var rotulo string
	if m := coberturaFirmaPDF.FindStringSubmatch(clave); m != nil {
		n, _ := strconv.Atoi(m[1])
		rotulo = loc.T(prefijoDetalle+"cobertura_firma_pdf", n)
	} else if m := cadenaFirmante.FindStringSubmatch(clave); m != nil {
		// «signer[0].chain_length=3»: firmantes numerados desde 1.
		n, _ := strconv.Atoi(m[1])
		rotulo = loc.T(prefijoDetalle+"cadena_firmante", n+1)
	} else if id := prefijoDetalle + clave; loc.T(id) != id {
		rotulo = loc.T(id)
	} else {
		return linea
	}
	return loc.T(prefijoDetalle+"formato", rotulo, traducirValorDetalle(loc, valor))
}

// detallesConDN son las evidencias cuyo valor es un nombre distinguido.
var detallesConDN = map[string]bool{"firmante": true, "subject": true, "issuer": true, "anchor": true}

// detalleConDNLegible escribe legible el DN de una evidencia «subject=…»;
// el resto de líneas no cambia.
func detalleConDNLegible(linea string) string {
	clave, valor, ok := strings.Cut(linea, "=")
	if !ok || !detallesConDN[clave] {
		return linea
	}
	return clave + "=" + certutil.DNLegible(valor)
}

func traducirValorDetalle(loc *localizador.Localizador, valor string) string {
	if m := revisionHasta.FindStringSubmatch(valor); m != nil {
		hasta, _ := strconv.ParseInt(m[1], 10, 64)
		total, _ := strconv.ParseInt(m[2], 10, 64)
		return loc.T(prefijoDetalle+"valor.revision_hasta", hasta, total)
	}
	if id := prefijoDetalle + "valor." + valor; valor != "" && loc.T(id) != id {
		return loc.T(id)
	}
	return valor
}

// TraducirDetalles aplica TraducirDetalle a una lista.
func TraducirDetalles(loc *localizador.Localizador, lineas []string) []string {
	if len(lineas) == 0 {
		return nil
	}
	out := make([]string, len(lineas))
	for i, linea := range lineas {
		out[i] = TraducirDetalle(loc, linea)
	}
	return out
}
