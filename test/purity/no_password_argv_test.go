// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// TestNoPasswordEnArgv (T111) impide la regresión CWE-214: pasar contraseñas por
// la línea de comandos, donde quedan visibles en la tabla de procesos (`ps`).
//
// Analiza el AST de todas las fuentes de internal/ y cmd/ buscando llamadas
// exec.Command/exec.CommandContext y falla si detecta:
//   - flags de contraseña "en claro" (-K, -W, -passin/-passout con "pass:", …)
//     seguidos de un argumento que NO sea claramente una referencia a fichero
//     o variable de entorno;
//   - literales "pass:<algo>" en cualquier argumento.
//
// Las herramientas externas admiten variantes de fichero (openssl -passin
// file:, pk12util -k/-w, …) o entorno; esas son las únicas permitidas.
package purity_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/common/secretinput"
)

func TestNoPasswordEnArgv(t *testing.T) {
	raiz := raizModulo(t)
	fset := token.NewFileSet()

	for _, sub := range []string{"internal", "cmd"} {
		dir := filepath.Join(raiz, sub)
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Fatalf("no se pudo parsear %s: %v", path, perr)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !esLlamadaExec(call) {
					return true
				}
				args := argumentosLiterales(call)
				comprobarArgvSeguro(t, fset, call.Pos(), path, args)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("error recorriendo %s: %v", dir, err)
		}
	}
}

// TestProcesosPropiosNoAceptanSecretosEnArgv amplía T111 a los argumentos de
// los propios binarios. El gate anterior solo analizaba exec.Command y no
// detectaba contraseñas o tokens aceptados por flag/os.Args en el proceso
// principal.
func TestProcesosPropiosNoAceptanSecretosEnArgv(t *testing.T) {
	raiz := raizModulo(t)
	fset := token.NewFileSet()
	policyPath := filepath.Join(
		raiz,
		"internal", "adapters", "inbound", "common", "secretinput", "input.go",
	)

	for _, sub := range []string{"internal", "cmd"} {
		dir := filepath.Join(raiz, sub)
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || path == policyPath {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Fatalf("no se pudo parsear %s: %v", path, perr)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					if name, ok := registeredFlagValueName(node); ok && secretinput.IsSecretValueFlagName(name) {
						loc := fset.Position(node.Pos())
						t.Errorf(
							"%s:%d: la bandera %q registra un secreto como string en argv (CWE-214); use stdin o una referencia no secreta",
							path,
							loc.Line,
							name,
						)
					}
				case *ast.CaseClause:
					for _, expr := range node.List {
						literal, ok := stringLiteral(expr)
						if !ok || !strings.HasPrefix(literal, "-") || !secretinput.IsSecretValueFlagName(literal) {
							continue
						}
						loc := fset.Position(expr.Pos())
						t.Errorf(
							"%s:%d: el parser propio reconoce la bandera secreta %q en argv (CWE-214)",
							path,
							loc.Line,
							literal,
						)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("error recorriendo %s: %v", dir, err)
		}
	}
}

func TestEntradasPropiasAplicanPoliticaComunArgv(t *testing.T) {
	raiz := raizModulo(t)
	for _, relative := range []string{
		"cmd/grxfirma/main.go",
		"cmd/grxfirma-gui/main.go",
		"internal/adapters/inbound/common/cli/adapter.go",
	} {
		path := filepath.Join(raiz, filepath.FromSlash(relative))
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		if !strings.Contains(string(source), "secretinput.RejectArgv(") {
			t.Errorf("%s no aplica la política común contra secretos en argv", relative)
		}
		if !strings.Contains(string(source), "secretinput.UserMessage(") {
			t.Errorf("%s no traduce los errores tipados de la política de secretos", relative)
		}
		if !strings.Contains(string(source), "secretinput.ConsumeEnvironment(") {
			t.Errorf("%s no consume tempranamente el entorno sensible", relative)
		}
	}
}

func TestOpcionGenericaValidaClavesSensiblesAntesDeFlagVar(t *testing.T) {
	path := filepath.Join(
		raizModulo(t),
		"internal", "adapters", "inbound", "common", "cli", "adapter.go",
	)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "secretinput.ValidateOptionAssignment(value)") {
		t.Fatal("opcionesCLI.Set no aplica la política común a las claves sensibles")
	}
}

func TestDetectorBanderasPropiasIncluyeFlagVar(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", `package fixture
import "flag"
func f() {
	var value flag.Value
	flag.Var(value, "password", "")
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.Var(value, "token-rest", "")
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if name, registered := registeredFlagValueName(call); registered &&
				secretinput.IsSecretValueFlagName(name) {
				found++
			}
		}
		return true
	})
	if found != 2 {
		t.Fatalf("el detector encontró %d banderas flag.Var sensibles; se esperaban 2", found)
	}
}

func registeredFlagValueName(call *ast.CallExpr) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	index := -1
	switch selector.Sel.Name {
	case "String":
		if pkg.Name != "flag" {
			return "", false
		}
		index = 0
	case "StringVar":
		index = 1
	case "Var":
		index = 1
	default:
		return "", false
	}
	if index >= len(call.Args) {
		return "", false
	}
	return stringLiteral(call.Args[index])
}

func stringLiteral(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// esLlamadaExec detecta exec.Command / exec.CommandContext.
func esLlamadaExec(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "exec" {
		return false
	}
	return sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext"
}

// argumentosLiterales devuelve, por posición, el valor de string literal de
// cada argumento (o "" si el argumento no es un literal de string).
func argumentosLiterales(call *ast.CallExpr) []string {
	out := make([]string, 0, len(call.Args))
	for _, a := range call.Args {
		if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			out = append(out, strings.Trim(lit.Value, "`\""))
		} else {
			out = append(out, "")
		}
	}
	return out
}

// flagsPasswordEnClaro son flags cuyo valor asociado es una contraseña que NO
// debe ir por argv. Sus variantes de fichero (minúsculas -k/-w) sí se permiten.
var flagsPasswordEnClaro = map[string]bool{
	"-K": true, // pk12util slot password (usar -k <fichero>)
	"-W": true, // pk12util PKCS12 password (usar -w <fichero>)
}

func comprobarArgvSeguro(t *testing.T, fset *token.FileSet, pos token.Pos, path string, args []string) {
	loc := fset.Position(pos)
	for i, arg := range args {
		// 1) Literal "pass:<algo>" en cualquier argumento (openssl -passin pass:...).
		if strings.HasPrefix(arg, "pass:") && arg != "pass:" {
			t.Errorf("%s:%d: contraseña literal en argv (%q); usa file:<fichero> o env:", path, loc.Line, arg)
		}
		// 2) Flag de contraseña en claro cuyo siguiente argumento no es un
		//    literal vacío (es decir, se le pasa un valor por argv).
		if flagsPasswordEnClaro[arg] {
			t.Errorf("%s:%d: flag %q pasa la contraseña por argv (CWE-214); usa la variante de fichero (%s)", path, loc.Line, arg, strings.ToLower(arg))
		}
		_ = i
	}
}
