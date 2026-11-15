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
	// Formas que solo pueden ser tuteo: pronombres, posesivos y verbos en
	// segunda persona del singular. «activas» no está porque casi siempre es
	// adjetivo («conexiones seguras activas»).
	tuteoFormas = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}{])(tu|tus|ti|contigo|te|tú|puedes|quieres|tienes|debes|necesitas|confías|estás|sabes|prefieres|eliges|deseas|has|hayas|hubieras|podrás|tendrás|deberás|aceptas|aceptaste|indicas|usas|quieras|puedas|elijas|aceptes|esperabas|querías|vuelvas|aportas|arrancas|reconoces|acabas|marcas|pides|guárdalo|inténtalo|hazlo|asegúrate|arrástralo|arrástralos|indícalo|divídela|orientarte|obligarte|autenticarte|protegerte)(?:$|[^\p{L}\p{N}}])`)
	// Imperativos de tú al empezar una frase o cláusula. Coinciden con la
	// tercera persona, así que las descripciones («Revisa una factura…») van
	// a testdata/i18n_tuteo_permitidas.json con su motivo.
	tuteoImperativo = regexp.MustCompile(`(?i)(?:^|[.:;!?¿¡(\n«]\s*|,\s+|\sy\s+|\so\s+)(selecciona|pulsa|introduce|arrastra|elige|reinicia|revisa|haz|escribe|mantén|toca|lee|usa)(?:$|[^\p{L}\p{N}}])`)
)

func tuteoEn(texto string) []string {
	var formas []string
	for _, m := range tuteoFormas.FindAllStringSubmatch(texto, -1) {
		formas = append(formas, m[1])
	}
	for _, m := range tuteoImperativo.FindAllStringSubmatch(texto, -1) {
		formas = append(formas, m[1])
	}
	return formas
}

// TestI18nCastellanoTrataDeUsted impide que un texto visible en castellano
// vuelva a tutear. Revisa el catálogo común, el de Android y los de los
// instaladores de Windows y Linux.
func TestI18nCastellanoTrataDeUsted(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "i18n_tuteo_permitidas.json"))
	if err != nil {
		t.Fatalf("lista de excepciones de tuteo: %v", err)
	}
	var permitidas map[string]string
	if err := json.Unmarshal(data, &permitidas); err != nil {
		t.Fatalf("lista de excepciones de tuteo: %v", err)
	}
	raiz := filepath.Join("..", "..", "..", "..", "..")
	catalogos := []string{
		filepath.Join("locales", "es.json"),
		filepath.Join(raiz, "mobile", "android", "app", "src", "main", "assets", "locales", "es.json"),
		filepath.Join(raiz, "packaging", "windows", "i18n", "es.json"),
		filepath.Join(raiz, "packaging", "linux", "i18n", "es.json"),
	}
	usadas := map[string]bool{}
	for _, ruta := range catalogos {
		catalogo, err := cargarCatalogoEstricto(ruta)
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		var fallos []string
		for clave, valor := range catalogo {
			formas := tuteoEn(valor)
			if len(formas) == 0 {
				continue
			}
			if motivo, ok := permitidas[clave]; ok && motivo != "" {
				usadas[clave] = true
				continue
			}
			fallos = append(fallos, clave)
		}
		sort.Strings(fallos)
		for _, clave := range fallos {
			t.Errorf("%s: %q tutea (%q). Use el trato de usted o, si es tercera persona, añada la clave con su motivo a testdata/i18n_tuteo_permitidas.json",
				ruta, clave, tuteoEn(catalogo[clave]))
		}
	}
	for clave := range permitidas {
		if !usadas[clave] {
			t.Errorf("testdata/i18n_tuteo_permitidas.json: %q ya no hace falta o no existe; quítela", clave)
		}
	}
}

func TestI18nDetectorTuteo(t *testing.T) {
	for _, texto := range []string{"¿Quieres firmar?", "Añadir tu firma", "Si confías en esta firma, puedes aceptarla.", "Selecciona un fichero.", "Guárdalo solo si sabes qué es."} {
		if len(tuteoEn(texto)) == 0 {
			t.Errorf("no detecta el tuteo de %q", texto)
		}
	}
	for _, texto := range []string{"¿Desea firmar?", "Añadir su firma", "Si confía en esta firma, puede aceptarla.", "Seleccione un fichero.", "Reglas activas", "Firma del documento", "Sello de tiempo"} {
		if f := tuteoEn(texto); len(f) != 0 {
			t.Errorf("falso positivo en %q: %q", texto, f)
		}
	}
}
