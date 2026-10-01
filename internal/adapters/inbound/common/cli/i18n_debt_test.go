// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Este gate no afirma que la CLI esté localizada. Acota únicamente la deuda
// estática que puede atribuirse sin ejecutar casos de uso: ayudas, errores
// construidos con errors.New/fmt.Errorf y literales enviados directamente a
// stdout/stderr. No cuenta JSON, datos dinámicos ni textos indirectos.
//
// Se miden palabras humanas y segmentos de texto por separado. Los comandos,
// flags, rutas, URLs, placeholders, formatos y otros identificadores técnicos
// no suman deuda. Una traducción debe reducir (o mantener) ambos límites; si se
// añade texto visible sin localizar, el cambio debe fallar.
func TestDeudaI18nCLINoAumenta(t *testing.T) {
	t.Parallel()

	// Baseline explícito actualizado el 2026-07-29. No subir estos límites:
	// envolver los literales en el localizador y reducir el valor correspondiente.
	limites := map[string]metricaDeudaCLI{
		"cmd/grxfirma/bootstrap.go/error": {
			palabras:  26,
			segmentos: 5,
		},
		"cmd/grxfirma/main.go/error": {
			palabras:  40,
			segmentos: 4,
		},
		"cmd/grxfirma/main.go/help": {
			palabras:  433,
			segmentos: 2,
		},
		"cmd/grxfirma/main.go/output": {
			palabras:  45,
			segmentos: 12,
		},
		"cmd/grxfirma/desktop_common.go/error": {
			palabras:  12,
			segmentos: 3,
		},
		"cmd/grxfirma/desktop_common.go/output": {
			palabras:  6,
			segmentos: 2,
		},
		"cmd/grxfirma/desktop_fyne.go/error": {
			palabras:  12,
			segmentos: 2,
		},
		"cmd/grxfirma/desktop_fyne.go/output": {
			palabras:  12,
			segmentos: 5,
		},
		"cmd/grxfirma/desktop_headless.go/output": {
			palabras:  9,
			segmentos: 1,
		},
		"internal/adapters/inbound/common/cli/adapter.go/error": {
			palabras:  229,
			segmentos: 39,
		},
		"internal/adapters/inbound/common/cli/adapter.go/help": {
			palabras:  556,
			segmentos: 84,
		},
		"internal/adapters/inbound/common/cli/adapter.go/output": {
			palabras:  400,
			segmentos: 99,
		},
	}

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no se pudo resolver la ruta del test")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", "..", "..", ".."))

	metricas := medirDeudaCLI(t, repoRoot)
	for clave, esperado := range limites {
		actual, existe := metricas[clave]
		if !existe {
			t.Errorf("métrica i18n CLI esperada ausente: %s", clave)
			continue
		}
		if actual != esperado {
			t.Errorf(
				"%s cambia la deuda i18n: palabras=%d (baseline %d), segmentos=%d (baseline %d); localiza los textos y reduce el baseline en el mismo cambio",
				clave,
				actual.palabras,
				esperado.palabras,
				actual.segmentos,
				esperado.segmentos,
			)
		}
	}
	for clave, actual := range metricas {
		if _, ok := limites[clave]; !ok {
			t.Errorf(
				"métrica i18n CLI sin baseline explícito: %s (palabras=%d, segmentos=%d)",
				clave,
				actual.palabras,
				actual.segmentos,
			)
		}
	}
}

func TestDetectorDeudaI18nCLIExcluyeSintaxisTecnica(t *testing.T) {
	t.Parallel()

	for nombre, texto := range map[string]string{
		"comando":     "grxfirma -modo-cli -entrada /ruta/documento.pdf",
		"endpoint":    "POST /auth/verify",
		"formato":     "CAdES|PAdES|XAdES|ASiC-XAdES",
		"id":          "cert-firma-1",
		"mime":        "application/pdf",
		"placeholder": "{{FORMATOS_FIRMA}} <%s>",
		"url":         "https://127.0.0.1:63118/openapi.json",
	} {
		if got := contarPalabrasHumanasCLI(texto); got != 0 {
			t.Errorf("%s técnico contabiliza %d palabra(s): %q", nombre, got, texto)
		}
	}
	if got := contarPalabrasHumanasCLI("La firma se ha completado correctamente."); got != 6 {
		t.Errorf("frase humana contabiliza %d palabras; se esperaban 6", got)
	}
	if got := contarPalabrasHumanasCLI("-nuevo <fichero> Descripción visible sin localizar"); got != 4 {
		t.Errorf("descripción de flag contabiliza %d palabras; se esperaban 4", got)
	}
	if got := contarPalabrasHumanasCLI("OPERACIÓN FALLIDA"); got != 2 {
		t.Errorf("texto humano en mayúsculas contabiliza %d palabras; se esperaban 2", got)
	}
}

type metricaDeudaCLI struct {
	palabras  int
	segmentos int
}

func medirDeudaCLI(t *testing.T, repoRoot string) map[string]metricaDeudaCLI {
	t.Helper()

	funcionesHumanas := map[string]map[string]string{
		"internal/adapters/inbound/common/cli/adapter.go": {
			"escribirAyuda":    "help",
			"traducirErrorCLI": "error",
		},
		"cmd/grxfirma/main.go": {
			"escribirUsoGeneral":   "help",
			"imprimirUsoDetallado": "help",
		},
	}

	var rutas []string
	for _, directorio := range []string{
		"cmd/grxfirma",
		"internal/adapters/inbound/common/cli",
	} {
		err := filepath.WalkDir(filepath.Join(repoRoot, filepath.FromSlash(directorio)), func(ruta string, entrada fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entrada.IsDir() || filepath.Ext(entrada.Name()) != ".go" || strings.HasSuffix(entrada.Name(), "_test.go") {
				return nil
			}
			relativa, err := filepath.Rel(repoRoot, ruta)
			if err != nil {
				return err
			}
			rutas = append(rutas, filepath.ToSlash(relativa))
			return nil
		})
		if err != nil {
			t.Fatalf("recorriendo fuentes CLI en %s: %v", directorio, err)
		}
	}
	sort.Strings(rutas)

	resultado := make(map[string]metricaDeudaCLI)
	for _, ruta := range rutas {
		absoluta := filepath.Join(repoRoot, filepath.FromSlash(ruta))
		fset := token.NewFileSet()
		fichero, err := parser.ParseFile(fset, absoluta, nil, 0)
		if err != nil {
			t.Fatalf("parseando %s: %v", ruta, err)
		}

		for _, declaracion := range fichero.Decls {
			funcion, ok := declaracion.(*ast.FuncDecl)
			if !ok || funcion.Body == nil {
				continue
			}
			if categoria, ok := funcionesHumanas[ruta][funcion.Name.Name]; ok {
				acumularLiterales(resultado, ruta+"/"+categoria, funcion.Body)
				continue
			}
			ast.Inspect(funcion.Body, func(nodo ast.Node) bool {
				llamada, ok := nodo.(*ast.CallExpr)
				if !ok {
					return true
				}
				categoria, argumentos, ok := clasificarSumideroCLI(llamada)
				if !ok {
					return true
				}
				for _, argumento := range argumentos {
					acumularLiterales(resultado, ruta+"/"+categoria, argumento)
				}
				return false
			})
		}
	}
	return resultado
}

func clasificarSumideroCLI(llamada *ast.CallExpr) (string, []ast.Expr, bool) {
	selector, ok := llamada.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil, false
	}
	if selector.Sel.Name == "WriteString" {
		return "output", llamada.Args, true
	}
	paquete, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", nil, false
	}

	switch paquete.Name {
	case "errors":
		if selector.Sel.Name == "New" {
			return "error", llamada.Args, true
		}
	case "fmt":
		switch selector.Sel.Name {
		case "Errorf":
			return "error", llamada.Args, true
		case "Print", "Printf", "Println":
			return "output", llamada.Args, true
		case "Fprint", "Fprintf", "Fprintln":
			if len(llamada.Args) > 1 && esSalidaHumanaCLI(llamada.Args[0]) {
				return "output", llamada.Args[1:], true
			}
		}
	}
	return "", nil, false
}

func esSalidaHumanaCLI(expresion ast.Expr) bool {
	switch valor := expresion.(type) {
	case *ast.Ident:
		return valor.Name == "stdout" || valor.Name == "stderr"
	case *ast.SelectorExpr:
		return valor.Sel.Name == "Stdout" || valor.Sel.Name == "Stderr"
	default:
		return false
	}
}

func acumularLiterales(resultado map[string]metricaDeudaCLI, clave string, raiz ast.Node) {
	ast.Inspect(raiz, func(nodo ast.Node) bool {
		if llamada, ok := nodo.(*ast.CallExpr); ok && esLlamadaLocalizadaCLI(llamada) {
			return false
		}
		literal, ok := nodo.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		texto, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		palabras := contarPalabrasHumanasCLI(texto)
		if palabras == 0 {
			return true
		}
		actual := resultado[clave]
		actual.palabras += palabras
		actual.segmentos++
		resultado[clave] = actual
		return true
	})
}

func esLlamadaLocalizadaCLI(llamada *ast.CallExpr) bool {
	selector, ok := llamada.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return selector.Sel.Name == "t" || selector.Sel.Name == "T"
}

var (
	urlCLI         = regexp.MustCompile(`(?i)\b(?:https?|afirma)://\S+`)
	rutaCLI        = regexp.MustCompile(`(?:^|\s)(?:[A-Za-z]:[\\/]|[/.]{1,2}/)\S*`)
	flagCLI        = regexp.MustCompile(`(?:^|\s)--?[\pL\d][^\s,;]*`)
	placeholderCLI = regexp.MustCompile(`<[^>\n]+>|\{\{[^}\n]+\}\}|%[-+#0-9.*\[\]a-zA-Z]*[a-zA-Z%]`)
	mimeCLI        = regexp.MustCompile(`(?i)\b[a-z][a-z0-9.+-]*/[a-z0-9.+-]+\b`)
	binarioCLI     = regexp.MustCompile(`(?i)\bgrxfirma(?:-[a-z0-9]+)+\b`)
	entornoCLI     = regexp.MustCompile(`\bGRXFIRMA_[A-Z0-9_]+\b`)
	idCLI          = regexp.MustCompile(`(?i)\b(?:[\pL][\pL\d_.-]*\d[\pL\d_.-]*|\d[\pL\d_.-]*[\pL][\pL\d_.-]*)\b`)
	palabraCLI     = regexp.MustCompile(`[\pL][\pL'-]*`)
)

func contarPalabrasHumanasCLI(texto string) int {
	total := 0
	for _, linea := range strings.Split(texto, "\n") {
		limpia := strings.TrimSpace(linea)
		if esSintaxisCLI(limpia) {
			continue
		}
		limpia = urlCLI.ReplaceAllString(limpia, " ")
		limpia = rutaCLI.ReplaceAllString(limpia, " ")
		limpia = flagCLI.ReplaceAllString(limpia, " ")
		limpia = placeholderCLI.ReplaceAllString(limpia, " ")
		limpia = mimeCLI.ReplaceAllString(limpia, " ")
		limpia = binarioCLI.ReplaceAllString(limpia, " ")
		limpia = entornoCLI.ReplaceAllString(limpia, " ")
		limpia = idCLI.ReplaceAllString(limpia, " ")
		for _, palabra := range palabraCLI.FindAllString(limpia, -1) {
			if !esIdentificadorTecnicoCLI(palabra) {
				total++
			}
		}
	}
	return total
}

func esSintaxisCLI(linea string) bool {
	if linea == "" {
		return true
	}
	linea = strings.TrimLeft(linea, "#* \t")
	if strings.HasPrefix(linea, "grxfirma ") ||
		strings.HasPrefix(linea, "curl ") ||
		strings.HasPrefix(linea, "man ") {
		return true
	}
	for _, metodo := range []string{"GET ", "POST ", "PUT ", "PATCH ", "DELETE "} {
		if strings.HasPrefix(linea, metodo) {
			return true
		}
	}
	return false
}

func esIdentificadorTecnicoCLI(palabra string) bool {
	switch strings.ToLower(palabra) {
	case "grxfirma", "base", "cades", "pades", "xades",
		"xmldsig", "odf", "ooxml", "facturae", "asic-xades", "json", "xml",
		"cms", "sha", "pkcs", "pfx", "pem",
		"openapi", "bearer", "fyne", "qt":
		return true
	default:
		return false
	}
}
