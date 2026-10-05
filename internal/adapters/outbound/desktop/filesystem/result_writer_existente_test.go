// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package filesystem_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/filesystem"
)

func TestEscribirEnDirectorioExistente_Politicas(t *testing.T) {
	casos := []struct {
		nombre     string
		politica   filesystem.PoliticaSobreescritura
		existe     bool
		wantErr    error
		wantFinal  string
		wantPrevio bool
	}{
		{"renombrar sin previo", filesystem.PoliticaRenombrar, false, nil, "doc_firmado.pdf", false},
		{"renombrar con previo", filesystem.PoliticaRenombrar, true, nil, "doc_firmado_001.pdf", true},
		{"fallar sin previo", filesystem.PoliticaFallar, false, nil, "doc_firmado.pdf", false},
		{"fallar con previo", filesystem.PoliticaFallar, true, filesystem.ErrSalidaExiste, "", true},
		{"forzar con previo", filesystem.PoliticaForzar, true, nil, "doc_firmado.pdf", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			dir := t.TempDir()
			ruta := filepath.Join(dir, "doc_firmado.pdf")
			if c.existe {
				if err := os.WriteFile(ruta, []byte("previo"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			final, err := filesystem.NuevoEscritorResultado(c.politica).EscribirEnDirectorioExistente(ruta, []byte("nuevo"))
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) || !errors.Is(err, os.ErrExist) {
					t.Fatalf("error = %v, want %v", err, c.wantErr)
				}
			} else if err != nil {
				t.Fatalf("error inesperado: %v", err)
			} else if final != filepath.Join(dir, c.wantFinal) {
				t.Fatalf("ruta final = %q, want %q", final, c.wantFinal)
			}
			if c.wantPrevio {
				if got, _ := os.ReadFile(ruta); string(got) != "previo" {
					t.Fatalf("se perdió el fichero previo: %q", got)
				}
			}
			if final != "" {
				if got, _ := os.ReadFile(final); string(got) != "nuevo" {
					t.Fatalf("contenido final = %q", got)
				}
			}
		})
	}
}

func TestEscribirEnDirectorioExistente_NoCreaCarpetas(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "no-existe", "doc.pdf")
	if _, err := filesystem.NuevoEscritorResultado(filesystem.PoliticaRenombrar).EscribirEnDirectorioExistente(ruta, []byte("x")); err == nil {
		t.Fatal("creó o aceptó una carpeta inexistente")
	}
	if _, err := os.Stat(filepath.Dir(ruta)); !os.IsNotExist(err) {
		t.Fatalf("se creó la carpeta: %v", err)
	}
}

// La persona puede elegir una carpeta que sea un enlace (p. ej. Documentos en
// otro disco); el destino final, en cambio, nunca se sigue.
func TestEscribirEnDirectorioExistente_AdmiteCarpetaEnlazadaPeroNoDestinoEnlazado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("crear enlaces simbólicos requiere privilegios en Windows")
	}
	base := t.TempDir()
	carpetaReal := filepath.Join(base, "carpetaReal")
	if err := os.Mkdir(carpetaReal, 0o700); err != nil {
		t.Fatal(err)
	}
	enlace := filepath.Join(base, "enlace")
	if err := os.Symlink(carpetaReal, enlace); err != nil {
		t.Fatal(err)
	}
	final, err := filesystem.NuevoEscritorResultado(filesystem.PoliticaRenombrar).EscribirEnDirectorioExistente(filepath.Join(enlace, "doc.pdf"), []byte("nuevo"))
	if err != nil {
		t.Fatalf("carpeta enlazada rechazada: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(carpetaReal, "doc.pdf")); string(got) != "nuevo" || final != filepath.Join(enlace, "doc.pdf") {
		t.Fatalf("final=%q contenido=%q", final, got)
	}

	victima := filepath.Join(base, "victima.txt")
	if err := os.WriteFile(victima, []byte("intacto"), 0o600); err != nil {
		t.Fatal(err)
	}
	destinoEnlazado := filepath.Join(carpetaReal, "salida.pdf")
	if err := os.Symlink(victima, destinoEnlazado); err != nil {
		t.Fatal(err)
	}
	for _, politica := range []filesystem.PoliticaSobreescritura{filesystem.PoliticaRenombrar, filesystem.PoliticaFallar, filesystem.PoliticaForzar} {
		_, _ = filesystem.NuevoEscritorResultado(politica).EscribirEnDirectorioExistente(destinoEnlazado, []byte("ataque"))
		if got, _ := os.ReadFile(victima); string(got) != "intacto" {
			t.Fatalf("política %v escribió a través del enlace: %q", politica, got)
		}
	}
}
