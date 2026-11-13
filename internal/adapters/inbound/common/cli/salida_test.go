// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// H-09: las salidas de la CLI no siguen enlaces simbólicos y no dejan un
// fichero a medias.
func TestSalidaNoSigueEnlace(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "firma.pdf")
	if err := os.WriteFile(entrada, []byte("%PDF-1.7\nfirma"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, politica := range []string{"fail", "rename", "force"} {
		for _, destino := range []string{"existente", "colgante"} {
			t.Run(politica+"/"+destino, func(t *testing.T) {
				dir := t.TempDir()
				victima := filepath.Join(dir, "victima.txt")
				if destino == "existente" {
					if err := os.WriteFile(victima, []byte("intacto"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				informe := filepath.Join(dir, "informe.html")
				if err := os.Symlink(victima, informe); err != nil {
					t.Skipf("enlaces simbólicos no disponibles: %v", err)
				}
				a := New(nil, nil).WithVerificador(verifyResultadoMock{resultado: domain.NewVerificationSuccess("PAdES", "ok", nil)})
				var stdout, stderr strings.Builder
				a.Stdout, a.Stderr = &stdout, &stderr
				a.Run(context.Background(), []string{
					"-operacion", "verificar", "-entrada", entrada,
					"-informe", informe, "-sobrescribir", politica,
				})
				datos, err := os.ReadFile(victima)
				switch destino {
				case "existente":
					if err != nil || string(datos) != "intacto" {
						t.Fatalf("la víctima cambió: %q, %v", datos, err)
					}
				case "colgante":
					if err == nil {
						t.Fatalf("se creó el destino del enlace: %q", datos)
					}
				}
				// Con rename el informe va a un nombre nuevo y el enlace no se toca.
				if info, err := os.Lstat(informe); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("el enlace ya no es el original: %v", err)
				}
			})
		}
	}
}

func TestEscribirFicheroSalida(t *testing.T) {
	dir := t.TempDir()
	nuevo := filepath.Join(dir, "nuevo.bin")
	if err := escribirFicheroSalida(nuevo, []byte("uno"), 0o600); err != nil {
		t.Fatalf("crear: %v", err)
	}
	if datos, _ := os.ReadFile(nuevo); string(datos) != "uno" {
		t.Fatalf("contenido = %q", datos)
	}
	// Un fichero regular existente se sustituye de forma atómica.
	if err := escribirFicheroSalida(nuevo, []byte("dos"), 0o600); err != nil {
		t.Fatalf("sustituir: %v", err)
	}
	if datos, _ := os.ReadFile(nuevo); string(datos) != "dos" {
		t.Fatalf("contenido = %q", datos)
	}
	// No quedan temporales.
	entradas, _ := os.ReadDir(dir)
	if len(entradas) != 1 {
		t.Fatalf("quedan ficheros temporales: %v", entradas)
	}
}
