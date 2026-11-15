// SPDX-License-Identifier: EUPL-1.2
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func snapshotHomeV6(t *testing.T, home string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s %d", info.Mode(), info.ModTime().UnixNano())
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(" %x", sha256.Sum256(data))
		}
		rel, _ := filepath.Rel(home, path)
		out[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestV6OpcionesSinURINoTocanHOME(t *testing.T) {
	previous := ensureStartupLocalTLSTrust
	t.Cleanup(func() { ensureStartupLocalTLSTrust = previous })
	calls := 0
	ensureStartupLocalTLSTrust = func(context.Context, string) (bool, error) { calls++; return false, nil }
	machinepolicy.UnsetForTest(t, "permitir_ca_instalacion_automatica")
	excepciones.SetForTest(t, "ca_instalacion_automatica", true)
	for _, arg := range []string{"--help", "-h", "-help", "--version", "-version", "--diagnostico", "--diagnose", "--desconocida", "documento.pdf", "https://portal.example", "afirma:", "afirma://", "afirma:%zz"} {
		t.Run(arg, func(t *testing.T) {
			calls = 0
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
			t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
			if err := os.WriteFile(filepath.Join(home, "existente"), []byte("no modificar"), 0600); err != nil {
				t.Fatal(err)
			}
			before := snapshotHomeV6(t, home)
			var output bytes.Buffer
			handled := false
			code := runConDependencias(context.Background(), []string{arg}, &output, func(context.Context, io.Writer, string) int { handled = true; return 0 })
			want := 1
			if arg == "--help" || arg == "-h" || arg == "-help" || arg == "--version" || arg == "-version" {
				want = 0
			}
			if code != want || handled || calls != 0 {
				t.Errorf("%s: code=%d want=%d handler=%v almacén=%d", arg, code, want, handled, calls)
			}
			if !reflect.DeepEqual(before, snapshotHomeV6(t, home)) {
				t.Fatal("se modificó HOME al procesar una opción sin URI")
			}
		})
	}
}
