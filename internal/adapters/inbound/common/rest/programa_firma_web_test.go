// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/afirmahandler"
)

// Lote 15: ninguna web puede cambiar el programa que abre las firmas
// (afirma://). El canal REST no recibe el selector, no tiene rutas para él y
// /settings no guarda nada que lo cambie.
func TestREST_NoCambiaElProgramaParaFirmarEnLasWebs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dir := t.TempDir()
	adaptador := nuevoAdaptadorPruebas(nil, nil, nil).WithConfigDir(dir)

	intento := map[string]any{
		"manejador_afirma": "autofirma", "afirma_handler": "autofirma", "afirmaHandler": "autofirma",
		"handler": "autofirma", "programa_firma_web": "autofirma",
	}
	doJSON(t, adaptador.Routes(), http.MethodPost, "/settings", intento)
	if _, err := os.Stat(filepath.Join(home, ".config", "grxfirma", afirmahandler.XDGPreferenceName)); !os.IsNotExist(err) {
		t.Fatalf("POST /settings creó la preferencia del programa de firma: %v", err)
	}
	if datos, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil && bytes.Contains(datos, []byte("manejador_afirma")) {
		t.Fatalf("POST /settings escribió manejador_afirma en config.json: %s", datos)
	}

	for _, ruta := range []string{
		"/afirma-handler", "/afirma_handler", "/afirma_handler_select", "/afirma_handler_status",
		"/settings/afirma-handler", "/programa-firma-web", "/web-signing-program", "/v2/afirma-handler",
	} {
		for _, metodo := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
			rr := doJSON(t, adaptador.Routes(), metodo, ruta, map[string]any{"handler": "autofirma"})
			if rr.Code == http.StatusOK {
				t.Fatalf("%s %s respondió %d: %s", metodo, ruta, rr.Code, rr.Body.String())
			}
		}
	}
}

// El paquete REST no importa el selector ni nombra el puerto: si alguien lo
// conectara, esta prueba obliga a revisar la decisión.
func TestREST_NoDependeDelSelectorDeAfirma(t *testing.T) {
	ficheros, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ficheros {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		arbol, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range arbol.Imports {
			if ruta, _ := strconv.Unquote(imp.Path.Value); strings.HasSuffix(ruta, "/afirmahandler") {
				t.Fatalf("%s importa %s", f, ruta)
			}
		}
		datos, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(datos, []byte("ProtocoloAfirma")) || bytes.Contains(datos, []byte("afirma_handler")) {
			t.Fatalf("%s nombra el selector de afirma://", f)
		}
	}
}
