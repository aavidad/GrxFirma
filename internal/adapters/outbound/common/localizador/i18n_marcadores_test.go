// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	// Marcadores con llaves que usan las superficies .NET y web: {0}, {1},
	// {name}... TestI18nCatalogosComunes ya cubre %s, %d, %1 y %nombre%.
	marcadorLlaves = regexp.MustCompile(`\{[A-Za-z0-9_]*\}`)
	// Las claves con forma de identificador (winui.sign.title) no llevan
	// el texto castellano; para ellas la referencia es el valor de es.json.
	claveIdentificador = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_-]+)+$`)
)

func marcadoresLlaves(valor string) []string {
	encontrados := marcadorLlaves.FindAllString(valor, -1)
	sort.Strings(encontrados)
	return encontrados
}

// TestI18nTraduccionesConservanMarcadores comprueba que cada traducción
// conserva exactamente los mismos marcadores que su texto castellano: el de
// la propia clave cuando es una frase y el valor de es.json cuando la clave
// es un identificador. Así una traducción no puede perder ni inventar un
// argumento que luego rompa el formateo o muestre «%!s(MISSING)».
func TestI18nTraduccionesConservanMarcadores(t *testing.T) {
	es := cargarLocale(t, "es")

	for clave, valor := range es {
		if claveIdentificador.MatchString(clave) {
			continue
		}
		if !firmasIguales(extraerFirmaPlaceholders(clave), extraerFirmaPlaceholders(valor)) ||
			strings.Join(marcadoresLlaves(clave), " ") != strings.Join(marcadoresLlaves(valor), " ") {
			t.Errorf("es.json: el valor de %q no conserva los marcadores de su clave: %q", clave, valor)
		}
	}

	for _, idioma := range idiomasI18n {
		if idioma == "es" {
			continue
		}
		catalogo := cargarLocale(t, idioma)
		for clave, referencia := range es {
			traduccion, ok := catalogo[clave]
			if !ok {
				continue // la paridad de claves la vigila TestI18nCatalogosComunes
			}
			esperados := strings.Join(marcadoresLlaves(referencia), " ")
			obtenidos := strings.Join(marcadoresLlaves(traduccion), " ")
			if esperados != obtenidos {
				t.Errorf("%s.json: marcadores con llaves distintos para %q: es=[%s] %s=[%s]",
					idioma, clave, esperados, idioma, obtenidos)
			}
			if !firmasIguales(extraerFirmaPlaceholders(referencia), extraerFirmaPlaceholders(traduccion)) {
				t.Errorf("%s.json: marcadores %% distintos para %q: es={%s} %s={%s}",
					idioma, clave, extraerFirmaPlaceholders(referencia), idioma, extraerFirmaPlaceholders(traduccion))
			}
		}
	}
}
