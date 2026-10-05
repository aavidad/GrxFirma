// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"grxfirma/internal/adapters/outbound/common/eni"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidarENI(t *testing.T) {
	valid, err := eni.Generar(eni.Documento{Contenido: []byte("pdf"), NombreFormato: "PDF", Metadatos: eni.Metadatos{Organos: []string{"L01180877"}, EstadoElaboracion: "EE01", TipoDocumental: "TD99"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "eni.xml")
	for _, tc := range []struct {
		data []byte
		rc   int
	}{{valid, 0}, {[]byte(strings.ReplaceAll(string(valid), "TD99", "TD21")), 1}} {
		if err := os.WriteFile(path, tc.data, 0600); err != nil {
			t.Fatal(err)
		}
		a := New(nil, nil)
		var out, stderr strings.Builder
		a.Stdout = &out
		a.Stderr = &stderr
		if rc := a.Run(context.Background(), []string{"-operacion", "validar-eni", "-entrada", path}); rc != tc.rc {
			t.Fatalf("rc=%d: %s", rc, stderr.String())
		}
		if out.Len() == 0 {
			t.Fatal("falta informe")
		}
	}
}
