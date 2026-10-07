// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package wincertstore

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// APIs de Windows que abren una clave privada o recorren proveedores y
// contenedores. Con una clave en tarjeta pueden pedir que se inserte o buscar
// el lector, así que solo deben llamarse al firmar.
var apisQueAbrenClaves = map[string]bool{
	"CryptAcquireCertificatePrivateKey": true,
	"CryptFindCertificateKeyProvInfo":   true,
	"CryptAcquireContext":               true,
	"CryptAcquireContextW":              true,
	"NCryptOpenStorageProvider":         true,
	"NCryptOpenKey":                     true,
}

// funcionUnicaQueAbreClaves es el único punto autorizado a abrir la clave.
const funcionUnicaQueAbreClaves = "adquirirClaveWindows"

type grafoPaquete struct {
	llamadas map[string]map[string]bool
	procs    map[string]string // nombre de la función -> API cargada con NewProc
}

func analizarPaquete(t *testing.T) grafoPaquete {
	t.Helper()
	ficheros, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(ficheros) == 0 {
		// Binario de pruebas copiado a otra máquina, sin las fuentes.
		t.Skip("las fuentes del paquete no están disponibles")
	}
	grafo := grafoPaquete{llamadas: map[string]map[string]bool{}, procs: map[string]string{}}
	fset := token.NewFileSet()
	for _, nombre := range ficheros {
		if strings.HasSuffix(nombre, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, nombre, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				llamadas := map[string]bool{}
				if d.Body != nil {
					ast.Inspect(d.Body, func(n ast.Node) bool {
						switch x := n.(type) {
						case *ast.CallExpr:
							switch fn := x.Fun.(type) {
							case *ast.Ident:
								llamadas[fn.Name] = true
							case *ast.SelectorExpr:
								llamadas[fn.Sel.Name] = true
								// procX.Call(...) invoca la API cargada en procX.
								if id, ok := fn.X.(*ast.Ident); ok && fn.Sel.Name == "Call" {
									llamadas[id.Name] = true
								}
							}
						case *ast.Ident:
							// Funciones pasadas como valor, p. ej. adquirir(...).
							llamadas[x.Name] = true
						}
						return true
					})
				}
				grafo.llamadas[d.Name.Name] = llamadas
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, valor := range vs.Values {
						llamada, ok := valor.(*ast.CallExpr)
						if !ok || len(llamada.Args) != 1 {
							continue
						}
						sel, ok := llamada.Fun.(*ast.SelectorExpr)
						lit, okLit := llamada.Args[0].(*ast.BasicLit)
						if !ok || !okLit || sel.Sel.Name != "NewProc" || i >= len(vs.Names) {
							continue
						}
						api, err := strconv.Unquote(lit.Value)
						if err == nil {
							grafo.procs[vs.Names[i].Name] = api
						}
					}
				}
			}
		}
	}
	return grafo
}

// alcanzables devuelve las funciones del paquete y las APIs nativas que se
// pueden alcanzar desde la raíz (por nombre; basta para este paquete).
func (g grafoPaquete) alcanzables(raiz string) map[string]bool {
	vistos := map[string]bool{}
	pendientes := []string{raiz}
	for len(pendientes) > 0 {
		actual := pendientes[len(pendientes)-1]
		pendientes = pendientes[:len(pendientes)-1]
		if vistos[actual] {
			continue
		}
		vistos[actual] = true
		if api, ok := g.procs[actual]; ok {
			vistos[api] = true
		}
		for llamada := range g.llamadas[actual] {
			if !vistos[llamada] {
				pendientes = append(pendientes, llamada)
			}
		}
	}
	return vistos
}

func TestContrato_ListarYResolverNoAbrenClavesPrivadas(t *testing.T) {
	t.Parallel()
	grafo := analizarPaquete(t)
	if _, ok := grafo.llamadas["listarAlmacen"]; !ok {
		t.Fatal("no se encuentra listarAlmacen; actualice las raíces del contrato")
	}

	// Caminos sin acción expresa del usuario: listar el catálogo, comprobar si
	// hay clave y preparar el firmante (KeyFor no firma todavía).
	for _, raiz := range []string{"List", "listarAlmacen", "tieneClavePrivada", "KeyFor", "nuevoFirmanteWindows", "obtenerCadena"} {
		for nombre := range grafo.alcanzables(raiz) {
			if apisQueAbrenClaves[nombre] || nombre == funcionUnicaQueAbreClaves {
				t.Errorf("%s alcanza %s: listar o resolver un certificado no debe abrir su clave privada", raiz, nombre)
			}
		}
	}

	// La clave solo se abre al firmar.
	if !grafo.alcanzables("Sign")[funcionUnicaQueAbreClaves] {
		t.Errorf("Sign ya no alcanza %s; revise el contrato", funcionUnicaQueAbreClaves)
	}
}

func TestContrato_SoloUnaFuncionAbreClavesPrivadas(t *testing.T) {
	t.Parallel()
	grafo := analizarPaquete(t)
	for funcion, llamadas := range grafo.llamadas {
		for llamada := range llamadas {
			api := llamada
			if cargada, ok := grafo.procs[llamada]; ok {
				api = cargada
			}
			if apisQueAbrenClaves[api] && funcion != funcionUnicaQueAbreClaves {
				t.Errorf("%s usa %s; solo %s puede abrir claves privadas", funcion, api, funcionUnicaQueAbreClaves)
			}
		}
	}
	for variable, api := range grafo.procs {
		if apisQueAbrenClaves[api] {
			t.Errorf("%s carga %s; las claves se abren con la API de x/sys en %s", variable, api, funcionUnicaQueAbreClaves)
		}
	}
}

func TestAccesoClaveNoReintentable(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		err    error
		quiere bool
	}{
		{"cancelado por el usuario", syscall.Errno(scardWCancelledByUser), true},
		{"sin tarjeta", syscall.Errno(scardENoSmartcard), true},
		{"sin lectores", syscall.Errno(scardENoReadersAvailable), true},
		{"lector no disponible", syscall.Errno(scardEReaderUnavailable), true},
		{"tarjeta retirada", syscall.Errno(scardWRemovedCard), true},
		{"operación cancelada", syscall.Errno(scardECancelled), true},
		{"ERROR_CANCELLED como HRESULT", syscall.Errno(hresultErrorCancelled), true},
		{"ERROR_CANCELLED Win32", syscall.Errno(win32ErrorCancelled), true},
		{"envuelto", fmt.Errorf("wincertstore: adquiriendo: %w", syscall.Errno(scardWCancelledByUser)), true},
		{"clave no encontrada", syscall.Errno(0x80090016), false},
		{"error sin código", errors.New("otro fallo"), false},
		{"nil", nil, false},
	}
	for _, c := range casos {
		if got := accesoClaveNoReintentable(c.err); got != c.quiere {
			t.Errorf("%s: accesoClaveNoReintentable = %v; quiere %v", c.nombre, got, c.quiere)
		}
	}
}
