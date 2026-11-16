// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Claves de catálogo escritas tal cual en el código WinUI. Las que acaban en
// punto son prefijos que el código completa y no se comprueban.
var claveWinUILiteral = regexp.MustCompile(`"((?:winui|excepciones)\.[a-z0-9_.]+)"`)

func TestI18nWinUIClavesLiteralesExistenEnTodosLosCatalogos(t *testing.T) {
	raiz := filepath.Join(raizRepositorio(t), "cmd", "gui-winui", "src")
	catalogos := make(map[string]map[string]string, len(idiomasI18n))
	for _, idioma := range idiomasI18n {
		catalogos[idioma] = cargarLocale(t, idioma)
	}
	vistas := 0
	err := filepath.Walk(raiz, func(ruta string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || (filepath.Ext(ruta) != ".cs" && filepath.Ext(ruta) != ".xaml") {
			return nil
		}
		fuente := leerSuperficie(t, ruta)
		for _, m := range claveWinUILiteral.FindAllStringSubmatch(fuente, -1) {
			clave := m[1]
			if strings.HasSuffix(clave, ".") {
				continue
			}
			vistas++
			for _, idioma := range idiomasI18n {
				if strings.TrimSpace(catalogos[idioma][clave]) == "" {
					t.Errorf("%s: la clave %q no está en %s.json", ruta, clave, idioma)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk(%s): %v", raiz, err)
	}
	if vistas == 0 {
		t.Fatalf("%s: no se ha encontrado ninguna clave; revise esta prueba", raiz)
	}
}

// El mapeo de diagnósticos de WinUI no lleva frases fijas: todo lo que ve la
// persona (también el marcador «[contenido protegido]») sale del catálogo.
func TestI18nWinUIMapeoDiagnosticoSinTextoFijo(t *testing.T) {
	ruta := filepath.Join(raizRepositorio(t), "cmd", "gui-winui", "src",
		"GrxFirma.WinUI.Core", "Diagnostics", "OperationDiagnosticMapper.cs")
	fuente := leerSuperficie(t, ruta)
	// Las expresiones regulares van en literales crudos """…""" y @"…".
	fuente = regexp.MustCompile(`(?s)""".*?"""`).ReplaceAllString(fuente, "")
	fuente = regexp.MustCompile(`@"(?:""|[^"])*"`).ReplaceAllString(fuente, "")
	literal := regexp.MustCompile(`"(?:\\.|[^"\\\n])*"`)
	// Dos palabras seguidas: así no saltan los fragmentos de las cadenas
	// interpoladas de los códigos técnicos ($"{phase ?? "unknown"}_ipc").
	frase := regexp.MustCompile(`\p{L}{3,} +\p{L}{2,}`)
	for numero, linea := range strings.Split(fuente, "\n") {
		if strings.HasPrefix(strings.TrimSpace(linea), "//") {
			continue
		}
		for _, crudo := range literal.FindAllString(linea, -1) {
			texto, err := strconv.Unquote(crudo)
			if err != nil {
				texto = crudo
			}
			if frase.MatchString(texto) {
				t.Errorf("%s:%d: texto visible fijo %q; use una clave del catálogo", ruta, numero+1, texto)
			}
		}
	}
	if !strings.Contains(fuente, `"winui.diagnostico.contenido_protegido"`) {
		t.Errorf("%s: el marcador de contenido protegido ya no sale del catálogo", ruta)
	}
}
