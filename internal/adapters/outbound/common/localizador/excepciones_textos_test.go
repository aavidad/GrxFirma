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
)

func loadCatalogue(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cat map[string]string
	if err := json.Unmarshal(raw, &cat); err != nil {
		t.Fatal(path, err)
	}
	return cat
}

// Lote 39d: la ayuda también se abre desde Configuración, sin ningún bloqueo.
// Su primer apartado describe el ajuste y no cuenta una operación detenida.
func TestExceptionHelpStartsNeutral(t *testing.T) {
	neutral := map[string]string{
		"es": "Qué hace este ajuste: mientras está desactivado, GrxFirma ",
		"en": "What this setting does: while it is off, GrxFirma ",
		"ca": "Què fa aquest ajust: mentre està desactivat, GrxFirma ",
		"va": "Què fa este ajust: mentres està desactivat, GrxFirma ",
		"gl": "Que fai este axuste: mentres está desactivado, GrxFirma ",
		"eu": "Zer egiten du ezarpen honek: desaktibatuta dagoen bitartean, GrxFirmak ",
		"fr": "Ce que fait ce réglage : tant qu’il est désactivé, GrxFirma ",
		"it": "Cosa fa questa impostazione: finché è disattivata, GrxFirma ",
		"pt": "O que faz esta opção: enquanto estiver desativada, o GrxFirma ",
		"de": "Was diese Einstellung bewirkt: Solange sie deaktiviert ist, blockiert GrxFirma ",
		"zh": "此设置的作用：停用期间，GrxFirma 会阻止",
	}
	// Los títulos antiguos solo tenían sentido después de un bloqueo.
	stale := []string{"Qué ha pasado", "What happened", "Què ha passat", "Que pasou", "Zer gertatu da",
		"Ce qui s’est passé", "Cos’è successo", "O que aconteceu", "Was passiert ist", "发生了什么"}
	for lang, prefix := range neutral {
		cat := loadCatalogue(t, filepath.Join("locales", lang+".json"))
		for _, e := range excepciones.Catalog() {
			help := cat[e.HelpKey]
			if !strings.HasPrefix(help, prefix) {
				t.Errorf("ayuda que no empieza por el apartado neutro: %s/%s", lang, e.ID)
			}
			for _, old := range stale {
				if strings.Contains(help, old) {
					t.Errorf("ayuda que habla de una operación detenida (%q): %s/%s", old, lang, e.ID)
				}
			}
		}
	}
}

// «Activado por mí el …» sonaba forzado; basta con la fecha.
func TestExceptionActivationDateReadsNaturally(t *testing.T) {
	stale := []string{"por mí", "by me", "per mi", "por min", "Nik ", "par moi", "da me", "por mim", "Von mir", "由我"}
	for _, lang := range []string{"es", "en", "ca", "de", "eu", "fr", "gl", "it", "pt", "va", "zh"} {
		text := loadCatalogue(t, filepath.Join("locales", lang+".json"))["excepciones.pantalla.activada_fecha"]
		if !strings.Contains(text, "{date}") {
			t.Errorf("%s: activada_fecha sin {date}", lang)
		}
		for _, old := range stale {
			if strings.Contains(text, old) {
				t.Errorf("%s: activada_fecha dice quién la activó: %q", lang, text)
			}
		}
	}
	if got := loadCatalogue(t, filepath.Join("locales", "es.json"))["excepciones.pantalla.activada_fecha"]; got != "Activado el {date}" {
		t.Errorf("es: activada_fecha = %q", got)
	}
}

// En inglés las comillas son “ ” y no se calca «bajo mi responsabilidad»
// fuera de los textos de aceptación, que no se tocan.
func TestEnglishExceptionTextsUseEnglishConventions(t *testing.T) {
	core := loadCatalogue(t, filepath.Join("locales", "en.json"))
	android := loadCatalogue(t, filepath.Join("..", "..", "..", "..", "..", "mobile", "android", "app", "src", "main", "assets", "locales", "en.json"))
	for _, cat := range []map[string]string{core, android} {
		for key, value := range cat {
			if !strings.HasPrefix(key, "excepciones.") && !strings.HasPrefix(key, "android.inseguro.") {
				continue
			}
			if strings.ContainsAny(value, "«»") {
				t.Errorf("comillas «» en inglés: %s", key)
			}
			if !strings.HasSuffix(key, ".aceptacion") && strings.Contains(strings.ToLower(value), "under my responsibility") {
				t.Errorf("calco «under my responsibility»: %s", key)
			}
		}
	}
	if strings.Contains(core["excepciones.sha1_leer.bloqueo"], "Blocked:") {
		t.Error("el bloqueo de sha1_leer repite la operación con «Blocked:»")
	}
}
