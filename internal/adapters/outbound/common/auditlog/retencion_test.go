// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package auditlog_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/ports"
)

// F-04: la retención de 90 días se aplica a cada registro por su fecha, no al
// fichero entero por su mtime (que cada firma nueva renueva).
func TestLogger_Log_RetencionPorRegistro(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	ahora := time.Now().UTC()
	registro := func(cuando time.Time, n string, anterior string) []byte {
		return auditlog.Encadenar([]byte(`{"timestamp":"`+cuando.Format(time.RFC3339)+`","operacion":"firma","n":"`+n+`"}`), anterior)
	}
	a := registro(ahora.Add(-100*24*time.Hour), "A", "")
	b := registro(ahora.Add(-80*24*time.Hour), "B", auditlog.Huella(a))
	contenido := string(a) + "\n" + string(b) + "\n"
	if err := os.WriteFile(path, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	// El mtime es reciente, como ocurre con un fichero que se sigue usando.

	logger := auditlog.New(path, 1<<20)
	if err := logger.Log(context.Background(), ports.Evidence{
		Payload: []byte(`{"timestamp":"` + ahora.Format(time.RFC3339) + `","operacion":"firma","n":"C"}`),
	}); err != nil {
		t.Fatalf("Log(): %v", err)
	}
	datos, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(datos), `"n":"A"`) {
		t.Fatalf("el registro de hace 100 días sigue en el fichero:\n%s", datos)
	}
	if !strings.Contains(string(datos), `"n":"B"`) || !strings.Contains(string(datos), `"n":"C"`) {
		t.Fatalf("faltan los registros vigentes:\n%s", datos)
	}
	inf, err := auditlog.VerificarCadena(path+".1", path)
	if err != nil {
		t.Fatalf("VerificarCadena(): %v", err)
	}
	if !inf.InicioRetirado || inf.Registros != 2 {
		t.Fatalf("VerificarCadena() = %+v, quiero InicioRetirado y 2 registros", inf)
	}
}
