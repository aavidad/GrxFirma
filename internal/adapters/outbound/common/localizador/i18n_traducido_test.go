// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

var (
	// Marcadores, URLs y rutas no cuentan como texto que traducir.
	elementosNoTraducibles = regexp.MustCompile(`%[A-Za-z_][A-Za-z0-9_.-]*%|%[0-9]|%[a-z]|https?://\S+|\{[^}]*\}`)
	palabraTraducible      = regexp.MustCompile(`[a-záéíóúñ]{4,}`)
	identificadorTecnico   = regexp.MustCompile(`^[a-z]+[A-Z][A-Za-z0-9]*$`)
)

func necesitaTraduccion(texto string) bool {
	if identificadorTecnico.MatchString(texto) {
		return false
	}
	return palabraTraducible.MatchString(elementosNoTraducibles.ReplaceAllString(texto, ""))
}

// Un texto que sigue igual que el español (o, fuera del inglés, igual que el
// inglés) suele ser una traducción olvidada. Las coincidencias legítimas
// (palabras que se escriben igual, nombres propios) se declaran en testdata.
func TestI18nNoQuedanTextosSinTraducir(t *testing.T) {
	es := cargarLocale(t, "es")
	en := cargarLocale(t, "en")
	permitidas := cargarIgualesPermitidas(t)
	for _, idioma := range idiomasI18n {
		if idioma == "es" {
			continue
		}
		catalogo := cargarLocale(t, idioma)
		var pendientes []string
		for clave, original := range es {
			valor := catalogo[clave]
			if !necesitaTraduccion(original) || permitidas[idioma][clave] {
				continue
			}
			igualEspanol := valor == original
			igualIngles := idioma != "en" && valor == en[clave] && en[clave] != original
			if igualEspanol || igualIngles {
				pendientes = append(pendientes, clave)
			}
		}
		if len(pendientes) == 0 {
			continue
		}
		sort.Strings(pendientes)
		muestra := pendientes
		if len(muestra) > 15 {
			muestra = muestra[:15]
		}
		t.Errorf("%s: %d textos sin traducir (igual que es/en). Tradúzcalos o, si coinciden de verdad, añádalos a testdata/i18n_iguales_permitidas.json. Ejemplos: %q",
			idioma, len(pendientes), muestra)
	}
}

func cargarIgualesPermitidas(t *testing.T) map[string]map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "i18n_iguales_permitidas.json"))
	if err != nil {
		t.Fatalf("lista de coincidencias permitidas: %v", err)
	}
	var lista map[string][]string
	if err := json.Unmarshal(data, &lista); err != nil {
		t.Fatalf("lista de coincidencias permitidas: %v", err)
	}
	resultado := make(map[string]map[string]bool, len(lista))
	for idioma, claves := range lista {
		resultado[idioma] = make(map[string]bool, len(claves))
		for _, clave := range claves {
			resultado[idioma][clave] = true
		}
	}
	return resultado
}
