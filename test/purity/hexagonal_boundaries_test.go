// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// TestFronterasHexagonales (T115) impone las dependencias permitidas de la
// arquitectura hexagonal: las capas internas no pueden depender de las externas.
//
//	domain      → (nada de grxfirma.v2 salvo domain)
//	ports       → domain
//	application → domain, ports        (NUNCA adapters)
//
// Analiza los imports de cada fuente (AST) y falla si alguna capa importa una
// capa prohibida. Complementa a TestSinDependenciasProhibidas, que vigila
// dependencias externas (go.mod), vigilando aquí las dependencias internas.
package purity_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFronterasHexagonales(t *testing.T) {
	raiz := raizModulo(t)

	const (
		base    = "grxfirma/internal/"
		domain  = base + "domain"
		ports   = base + "ports"
		app     = base + "application"
		adapter = base + "adapters"
	)

	// Por capa: prefijos de import internos que están PROHIBIDOS.
	capas := []struct {
		nombre     string
		dir        string
		prohibidos []string
	}{
		{"domain", filepath.Join(raiz, "internal", "domain"), []string{ports, app, adapter}},
		{"ports", filepath.Join(raiz, "internal", "ports"), []string{app, adapter}},
		{"application", filepath.Join(raiz, "internal", "application"), []string{adapter}},
	}

	fset := token.NewFileSet()
	for _, capa := range capas {
		err := filepath.WalkDir(capa.dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if perr != nil {
				t.Fatalf("no se pudo parsear %s: %v", path, perr)
			}
			for _, imp := range file.Imports {
				ruta := strings.Trim(imp.Path.Value, `"`)
				for _, prohibido := range capa.prohibidos {
					if ruta == prohibido || strings.HasPrefix(ruta, prohibido+"/") {
						t.Errorf("violación hexagonal: la capa %q (%s) importa %q, que está prohibido", capa.nombre, relParaTest(raiz, path), ruta)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("error recorriendo %s: %v", capa.dir, err)
		}
	}
}

func relParaTest(raiz, path string) string {
	if rel, err := filepath.Rel(raiz, path); err == nil {
		return rel
	}
	return path
}
