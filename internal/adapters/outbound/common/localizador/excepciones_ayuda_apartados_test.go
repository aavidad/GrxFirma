// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"encoding/json"
	"grxfirma/internal/security/excepciones"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// tituloApartado reproduce el corte de Qt (UnsafeText.js) y WinUI
// (UnsafeExceptionText.cs): el título va antes de los primeros dos puntos,
// normales o de ancho completo, dentro de los 80 primeros caracteres.
func tituloApartado(apartado string) string {
	limpio := strings.TrimSpace(apartado)
	for i, r := range []rune(limpio) {
		if i > 80 {
			break
		}
		if r == ':' || r == '：' {
			return strings.TrimSpace(string([]rune(limpio)[:i]))
		}
	}
	return ""
}

// Lote 39h: el lector de pantalla lee la ayuda apartado por apartado, con el
// título como encabezado, y con el caso fijado por la organización se quitan
// los tres últimos («Qué cambia», «Cómo deshacerlo» y el del bloqueo). Por eso
// cada apartado tiene título y los siete títulos son los mismos en los 26
// casos de cada idioma.
func TestExceptionHelpSectionsHaveTitles(t *testing.T) {
	for _, lang := range []string{"es", "en", "ca", "de", "eu", "fr", "gl", "it", "pt", "va", "zh"} {
		raw, err := os.ReadFile(filepath.Join("locales", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var cat map[string]string
		if json.Unmarshal(raw, &cat) != nil {
			t.Fatal("catálogo inválido", lang)
		}
		var modelo []string
		for _, e := range excepciones.Catalog() {
			apartados := strings.Split(strings.TrimSpace(cat[e.HelpKey]), "\n\n")
			titulos := make([]string, 0, len(apartados))
			for i, apartado := range apartados {
				titulo := tituloApartado(apartado)
				if titulo == "" || utf8.RuneCountInString(strings.TrimSpace(apartado)) <= utf8.RuneCountInString(titulo)+2 {
					t.Errorf("%s/%s: apartado %d sin título o sin texto", lang, e.ID, i+1)
				}
				titulos = append(titulos, titulo)
			}
			if modelo == nil {
				modelo = titulos
				continue
			}
			if strings.Join(titulos, "|") != strings.Join(modelo, "|") {
				t.Errorf("%s/%s: títulos %q distintos de %q", lang, e.ID, titulos, modelo)
			}
		}
		org := cat["excepciones.pantalla.ayuda_organizacion"]
		if tituloApartado(org) == "" || strings.Contains(org, "\n") {
			t.Errorf("%s: excepciones.pantalla.ayuda_organizacion sin título o en varios párrafos", lang)
		}
		if org == cat["excepciones.pantalla.solo_lectura"] {
			t.Errorf("%s: la ayuda de la organización repite la nota de solo lectura", lang)
		}
		if len(modelo) == 7 && tituloApartado(org) == modelo[6] {
			t.Errorf("%s: el apartado de la organización no se distingue del bloqueo", lang)
		}
	}
}
