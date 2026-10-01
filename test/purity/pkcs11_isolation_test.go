// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package purity_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	pkcs11NativeDirectory = "internal/adapters/outbound/desktop/pkcs11store"
	pkcs11WorkerDirectory = "cmd/grxfirma-pkcs11-worker"
)

// All source variants are inspected, not just the current platform's build.
// This prevents a future bootstrap from bypassing the isolated client by
// importing the native adapter, including transitively via another adapter.
func TestPKCS11NativeCodeOnlyInAuxiliary(t *testing.T) {
	root := raizModulo(t)
	fset := token.NewFileSet()
	for _, directory := range []string{"cmd", "internal", "mobilebind", "presentation"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			relative := filepath.ToSlash(relParaTest(root, path))
			for _, imported := range file.Imports {
				name, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if !pkcs11ImportAllowed(relative, name) {
					t.Errorf("%s carga %q fuera del auxiliar PKCS#11", relative, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func pkcs11ImportAllowed(source, imported string) bool {
	directory := filepath.ToSlash(filepath.Dir(source))
	if imported == "grxfirma/"+pkcs11NativeDirectory {
		return directory == pkcs11WorkerDirectory || directory == pkcs11NativeDirectory
	}
	if imported == "github.com/miekg/pkcs11" || strings.HasPrefix(imported, "github.com/miekg/pkcs11/") {
		return directory == pkcs11NativeDirectory
	}
	if imported == "C" && (directory == "internal/adapters/outbound/desktop/pkcs11worker" ||
		directory == "internal/adapters/outbound/desktop/isolatedtokenstore" ||
		directory == "internal/adapters/outbound/desktop/tokenruntime" || directory == "presentation/desktop/tokenpin") {
		return false
	}
	return true
}

func TestPKCS11IsolationPolicy(t *testing.T) {
	for _, tc := range []struct {
		source, imported string
		allowed          bool
	}{
		{"cmd/grxfirma/bootstrap.go", "grxfirma/" + pkcs11NativeDirectory, false},
		{"internal/adapters/outbound/desktop/other/store.go", "grxfirma/" + pkcs11NativeDirectory, false},
		{pkcs11WorkerDirectory + "/main_linux.go", "grxfirma/" + pkcs11NativeDirectory, true},
		{pkcs11NativeDirectory + "/module_linux.go", "github.com/miekg/pkcs11", true},
		{"cmd/grxfirmauri/bootstrap.go", "github.com/miekg/pkcs11", false},
		{"internal/adapters/outbound/desktop/pkcs11worker/client_linux.go", "C", false},
		{"internal/adapters/outbound/desktop/isolatedtokenstore/store_linux.go", "C", false},
		{"internal/adapters/outbound/desktop/tokenruntime/runtime.go", "C", false},
		{"presentation/desktop/tokenpin/process_linux.go", "C", false},
		{"presentation/desktop/tokenpin/process_linux.go", "grxfirma/" + pkcs11NativeDirectory, false},
		{"cmd/grxfirma/bootstrap.go", "grxfirma/internal/adapters/outbound/desktop/isolatedtokenstore", true},
	} {
		if got := pkcs11ImportAllowed(tc.source, tc.imported); got != tc.allowed {
			t.Errorf("source=%s import=%s allowed=%v", tc.source, tc.imported, got)
		}
	}
}
