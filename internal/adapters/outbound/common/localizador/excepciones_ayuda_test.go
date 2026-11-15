// SPDX-License-Identifier: EUPL-1.2
package localizador_test

import (
	"encoding/json"
	"grxfirma/internal/security/excepciones"
	"os"
	"path/filepath"
	"regexp"
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
			block := cat[e.BlockKey]
			if strings.Contains(block, "-activar-excepcion") || strings.Contains(block, "-acepto-el-aviso") || strings.Contains(block, "settings.json") || strings.Contains(block, "config.json") {
				t.Errorf("bloqueo dependiente de terminal o ficheros: %s/%s", lang, e.ID)
			}
			if !strings.Contains(block, cat[e.SettingKey]) || !strings.Contains(block, "GrxFirma") {
				t.Errorf("bloqueo sin ajuste o aplicación: %s/%s", lang, e.ID)
			}
		}
	}
}

func TestExceptionAcceptanceIsReadyForPeople(t *testing.T) {
	forbidden := []string{"settings.json", "config.json", "policy.json", "tokens.json", "trusted-domains", "excepciones_inseguras", "permitir_", "%APPDATA%", "~/.config", "cuando esté disponible", "cuando la sección esté disponible", "when available", "once available"}
	acronyms := regexp.MustCompile(`(?i)\b(?:AES(?:-(?:CBC|GCM))?|RSA|SHA-(?:1|256|384|512)|DES|CMS|PDF|TSA|HTTPS?|PKCS)\b`)
	for _, lang := range []string{"es", "en", "ca", "de", "eu", "fr", "gl", "it", "pt", "va", "zh"} {
		raw, err := os.ReadFile(filepath.Join("locales", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		mobile, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "mobile", "android", "app", "src", "main", "assets", "locales", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var cat, android map[string]string
		json.Unmarshal(raw, &cat)
		json.Unmarshal(mobile, &android)
		for _, e := range excepciones.Catalog() {
			text := cat[e.AcceptanceKey]
			if lang != "en" && (!strings.HasPrefix(text, "Puedo activarlo bajo mi responsabilidad. Sé que ") || !strings.HasSuffix(text, ". Se puede volver a desactivar en Preferencias, Seguridad.")) {
				t.Errorf("patrón incorrecto: %s/%s", lang, e.ID)
			}
			if lang == "en" && (!strings.HasPrefix(text, "I can enable this at my own risk. I understand that ") || !strings.HasSuffix(text, ". I can turn it off again in Preferences, Security.")) {
				t.Errorf("aceptación inglesa incompleta: %s", e.ID)
			}
			for _, key := range []string{e.AcceptanceKey, e.HelpKey, e.BlockKey} {
				value := cat[key]
				if android[key] != value {
					t.Errorf("Android difiere: %s/%s", lang, key)
				}
				for _, token := range forbidden {
					if strings.Contains(strings.ToLower(value), strings.ToLower(token)) {
						t.Errorf("texto interno/desfasado %q: %s/%s", token, lang, key)
					}
				}
				for _, word := range acronyms.FindAllString(value, -1) {
					if word != strings.ToUpper(word) {
						t.Errorf("sigla deformada %q: %s/%s", word, lang, key)
					}
				}
			}
		}
	}
}
