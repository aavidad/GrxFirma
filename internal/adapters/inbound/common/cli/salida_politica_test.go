// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGuardarSalidaCLI_Politicas(t *testing.T) {
	casos := []struct {
		politica      string
		existe        bool
		wantErr       bool
		wantFinal     string
		wantRenamed   bool
		wantOverwrote bool
	}{
		{"rename", false, false, "doc_firmado.pdf", false, false},
		{"rename", true, false, "doc_firmado_001.pdf", true, false},
		{"", true, false, "doc_firmado_001.pdf", true, false},
		{"fail", false, false, "doc_firmado.pdf", false, false},
		{"fail", true, true, "", false, false},
		{"force", true, false, "doc_firmado.pdf", false, true},
		{"force", false, false, "doc_firmado.pdf", false, false},
	}
	for _, c := range casos {
		t.Run(c.politica+map[bool]string{true: "-existe", false: "-nuevo"}[c.existe], func(t *testing.T) {
			dir := t.TempDir()
			destino := filepath.Join(dir, "doc_firmado.pdf")
			if c.existe {
				if err := os.WriteFile(destino, []byte("previo"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ruta, renamed, overwrote, err := guardarSalidaCLI(destino, c.politica, []byte("nuevo"))
			if c.wantErr {
				if !errors.Is(err, os.ErrExist) {
					t.Fatalf("err = %v, want ErrExist", err)
				}
			} else if err != nil {
				t.Fatalf("err = %v", err)
			} else if ruta != filepath.Join(dir, c.wantFinal) || renamed != c.wantRenamed || overwrote != c.wantOverwrote {
				t.Fatalf("ruta=%q renamed=%v overwrote=%v", ruta, renamed, overwrote)
			}
			if c.existe && !c.wantOverwrote {
				if got, _ := os.ReadFile(destino); string(got) != "previo" {
					t.Fatalf("se perdió el fichero previo: %q", got)
				}
			}
		})
	}
}
