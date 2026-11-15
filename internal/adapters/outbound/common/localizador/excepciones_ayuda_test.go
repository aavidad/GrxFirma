// SPDX-License-Identifier: EUPL-1.2
package localizador_test

import (
	"encoding/json"
	"grxfirma/internal/security/excepciones"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// La ayuda que lee la persona debe coincidir dentro y fuera de la app.
func TestExceptionConsentAndHelpCatalogues(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "docs", "seguridad", "EXCEPCIONES_INSEGURAS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"es", "en", "ca", "de", "eu", "fr", "gl", "it", "pt", "va", "zh"} {
		raw, err := os.ReadFile(filepath.Join("locales", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var cat map[string]string
		if json.Unmarshal(raw, &cat) != nil {
			t.Fatal("catálogo inválido", lang)
		}
		for _, e := range excepciones.Catalog() {
			for _, key := range []string{e.TitleKey, e.RiskKey, e.AcceptanceKey, e.HelpKey, e.OrganizationKey} {
				if strings.TrimSpace(cat[key]) == "" {
					t.Errorf("%s sin %s", lang, key)
				}
			}
			if lang == "es" && !strings.Contains(strings.Join(strings.Fields(string(doc)), " "), strings.Join(strings.Fields(cat[e.HelpKey]), " ")) {
				t.Errorf("ayuda distinta en app/documento: %s", e.ID)
			}
			if len(strings.Split(strings.TrimSpace(cat[e.HelpKey]), "\n")) != 7 {
				t.Errorf("ayuda sin siete apartados: %s/%s", lang, e.ID)
			}
			if !strings.Contains(cat[e.BlockKey], "grxfirma -activar-excepcion "+e.ID) {
				t.Errorf("bloqueo sin orden exacta: %s/%s", lang, e.ID)
			}
		}
	}
}
