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
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

func TestPara_Espanol(t *testing.T) {
	t.Parallel()
	loc := localizador.Para("es")
	got := loc.T("error.formato_invalido")
	if got == "" || got == "error.formato_invalido" {
		t.Errorf("esperaba mensaje espanol, got %q", got)
	}
}

func TestPara_Ingles(t *testing.T) {
	t.Parallel()
	loc := localizador.Para("en")
	got := loc.T("error.formato_invalido")
	if got == "" || got == "error.formato_invalido" {
		t.Errorf("esperaba mensaje ingles, got %q", got)
	}
}

func TestPara_LocaleDesconocidoCaeAEspanol(t *testing.T) {
	t.Parallel()
	loc := localizador.Para("zh")
	// Debe funcionar sin panic y devolver algo (cae a es)
	got := loc.T("error.formato_invalido")
	if got == "" {
		t.Error("esperaba mensaje no vacio para locale desconocido")
	}
}

func TestT_ClaveInexistente(t *testing.T) {
	t.Parallel()
	loc := localizador.Para("es")
	clave := "clave.que.no.existe"
	got := loc.T(clave)
	if got != clave {
		t.Errorf("clave inexistente: got %q, queria la propia clave %q", got, clave)
	}
}

func TestT_ConArgs(t *testing.T) {
	t.Parallel()
	loc := localizador.Para("es")
	// "error.accion_no_soportada": "Acción no soportada: %s"
	got := loc.T("error.accion_no_soportada", "prueba")
	if got == "error.accion_no_soportada" {
		t.Error("T con args no sustituyo el mensaje")
	}
	// El resultado debe contener el argumento
	if got == "" {
		t.Error("T con args devolvio cadena vacia")
	}
}

func TestT_SinArgs(t *testing.T) {
	t.Parallel()
	loc := localizador.Para("es")
	got := loc.T("error.catalogo_no_configurado")
	if got == "" || got == "error.catalogo_no_configurado" {
		t.Errorf("T sin args: got %q", got)
	}
}

func TestLocale(t *testing.T) {
	t.Parallel()
	loc := localizador.Para("en_GB")
	if loc.Locale() != "en" {
		t.Errorf("Locale() = %q, queria %q", loc.Locale(), "en")
	}
}

func TestDetectar_NoEntra_EnPanic(t *testing.T) {
	t.Parallel()
	// Solo verificamos que no entra en panic
	loc := localizador.Detectar()
	_ = loc.T("error.formato_invalido")
}

func TestPara_LocalesAdicionales(t *testing.T) {
	t.Parallel()
	for _, locale := range []string{"fr_FR", "de_DE", "it_IT", "pt_PT", "zh_CN", "gl_ES", "eu_ES", "ca_ES", "ca-valencia"} {
		loc := localizador.Para(locale)
		if got := loc.T("GrxFirma"); got == "" {
			t.Fatalf("locale %s no devolvió mensaje", locale)
		}
	}
}

func TestLocalesJSON_ContienenParidadCanonica(t *testing.T) {
	t.Parallel()

	base := filepath.Join("locales")
	read := func(name string) map[string]string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(base, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		var data map[string]string
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatalf("Unmarshal(%s): %v", name, err)
		}
		return data
	}

	es := read("es.json")
	en := read("en.json")
	canon := map[string]struct{}{}
	for k := range es {
		canon[k] = struct{}{}
	}
	for k := range en {
		canon[k] = struct{}{}
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("ReadDir(locales): %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data := read(entry.Name())
		for key := range canon {
			if _, ok := data[key]; !ok {
				t.Fatalf("locale %s no contiene la clave %q", entry.Name(), key)
			}
		}
	}
}

func TestLocalesJSON_CubrenClavesReferenciadasPorQMLyREST(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", "..", "..", ".."))
	localesDir := filepath.Join(wd, "locales")
	qmlDir := filepath.Join(repoRoot, "cmd", "gui-qml", "qml")
	restDir := filepath.Join(repoRoot, "internal", "adapters", "inbound", "common", "rest")

	readJSON := func(name string) map[string]string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(localesDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		var data map[string]string
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatalf("Unmarshal(%s): %v", name, err)
		}
		return data
	}

	extractKeys := func(root string, allowedExt string, patterns ...*regexp.Regexp) map[string]struct{} {
		t.Helper()
		keys := map[string]struct{}{}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if filepath.Ext(path) != allowedExt {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(raw)
			for _, pattern := range patterns {
				for _, match := range pattern.FindAllStringSubmatch(text, -1) {
					for _, group := range match[1:] {
						if group != "" {
							keys[group] = struct{}{}
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Walk(%s): %v", root, err)
		}
		return keys
	}

	qmlKeysRaw := extractKeys(
		qmlDir,
		".qml",
		regexp.MustCompile(`tr\("([^"]+)"\)`),
		regexp.MustCompile(`tr\('([^']+)'\)`),
	)
	restKeysRaw := extractKeys(
		restDir,
		".go",
		regexp.MustCompile(`\bt\("([^"]+)"`),
		regexp.MustCompile(`\bt\('([^']+)'`),
	)
	required := map[string]struct{}{}
	for _, src := range []map[string]struct{}{qmlKeysRaw, restKeysRaw} {
		for k := range src {
			if k == "" {
				continue
			}
			required[k] = struct{}{}
		}
	}

	entries, err := os.ReadDir(localesDir)
	if err != nil {
		t.Fatalf("ReadDir(locales): %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data := readJSON(entry.Name())
		for key := range required {
			if _, ok := data[key]; !ok {
				t.Fatalf("locale %s no contiene la clave referenciada %q", entry.Name(), key)
			}
		}
	}
}
