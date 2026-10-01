// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package auditlog_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/ports"
)

func escribirRegistros(t *testing.T, path string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := auditlog.New(path, 1<<20).Log(context.Background(), ports.Evidence{Payload: []byte(`{"operacion":"firma","n":` + string(rune('0'+i)) + `}`)}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerificarCadena_DetectaAlteracionesYBorrados(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	// Cada registro lo escribe un proceso distinto: la cadena se recupera
	// del fichero.
	escribirRegistros(t, path, 4)
	if inf, err := auditlog.VerificarCadena(path); err != nil || inf.Registros != 4 {
		t.Fatalf("cadena íntegra: %+v %v", inf, err)
	}
	original, _ := os.ReadFile(path)
	lineas := bytes.Split(bytes.TrimSpace(original), []byte("\n"))

	casos := map[string][]byte{
		"alterado": bytes.Replace(original, []byte(`"n":1`), []byte(`"n":7`), 1),
		"borrado":  append(append(append([]byte{}, lineas[0]...), '\n'), bytes.Join(lineas[2:], []byte("\n"))...),
		"insertado": append(append(append([]byte{}, lineas[0]...), []byte("\n{\"cadena\":\"00\",\"falso\":true}\n")...),
			bytes.Join(lineas[1:], []byte("\n"))...),
	}
	for nombre, contenido := range casos {
		if err := os.WriteFile(path, contenido, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := auditlog.VerificarCadena(path); !errors.Is(err, auditlog.ErrCadenaRota) {
			t.Errorf("%s: se esperaba cadena rota, obtenido %v", nombre, err)
		}
	}

	// Registros anteriores retirados por la retención: se informa, no es error.
	if err := os.WriteFile(path, append(bytes.Join(lineas[2:], []byte("\n")), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if inf, err := auditlog.VerificarCadena(path); err != nil || !inf.InicioRetirado || inf.Registros != 2 {
		t.Fatalf("inicio retirado: %+v %v", inf, err)
	}
}

func TestEncadenar_EvidenciaNoObjetoSeGuardaComoTexto(t *testing.T) {
	r := auditlog.Encadenar([]byte("texto libre"), "")
	if string(r) != `{"cadena":"`+auditlog.InicioCadena+`","evidencia":"texto libre"}` {
		t.Fatalf("registro inesperado: %s", r)
	}
	// Una evidencia que ya trae "cadena" no puede suplantar la huella.
	r = auditlog.Encadenar([]byte(`{"cadena":"falsa"}`), "ab")
	if !bytes.HasPrefix(r, []byte(`{"cadena":"ab","evidencia":`)) {
		t.Fatalf("registro inesperado: %s", r)
	}
}
