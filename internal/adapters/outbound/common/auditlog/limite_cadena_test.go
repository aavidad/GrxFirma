// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package auditlog_test

import (
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/auditlog"
)

// F-05: documenta el límite de la cadena. Quien puede escribir el fichero
// reescribe un registro y recalcula las huellas siguientes, y VerificarCadena
// no se entera. Si algún día la cadena lleva clave o sello, este test debe
// cambiar a propósito.
func TestVerificarCadena_NoProtegeFrenteAQuienEscribeElFichero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	r1 := auditlog.Encadenar([]byte(`{"operacion":"firma","n":1}`), "")
	r2 := auditlog.Encadenar([]byte(`{"operacion":"firma","n":2}`), auditlog.Huella(r1))

	// El atacante cambia n:1 por n:9 y recalcula la huella que lleva el segundo.
	falso1 := auditlog.Encadenar([]byte(`{"operacion":"firma","n":9}`), "")
	falso2 := auditlog.Encadenar([]byte(`{"operacion":"firma","n":2}`), auditlog.Huella(falso1))
	if string(falso1) == string(r1) || string(falso2) == string(r2) {
		t.Fatal("la prueba no ha cambiado nada")
	}
	if err := os.WriteFile(path, []byte(string(falso1)+"\n"+string(falso2)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inf, err := auditlog.VerificarCadena(path)
	if err != nil || inf.Registros != 2 {
		t.Fatalf("VerificarCadena() = %+v, %v; la cadena sin clave acepta una reescritura coherente", inf, err)
	}
}
