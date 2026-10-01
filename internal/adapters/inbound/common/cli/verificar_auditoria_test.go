// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/ports"
)

func TestRunVerificarAuditoria(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "audit.jsonl")
	registro := auditlog.New(ruta, 1<<20)
	for _, p := range []string{`{"operacion":"firma"}`, `{"operacion":"cofirma"}`, `{"operacion":"verificacion"}`} {
		if err := registro.Log(context.Background(), ports.Evidence{Payload: []byte(p)}); err != nil {
			t.Fatal(err)
		}
	}
	ejecutar := func() (int, string) {
		var stdout, stderr strings.Builder
		a := New(nil, nil)
		a.Stdout, a.Stderr = &stdout, &stderr
		rc := a.Run(context.Background(), []string{"-operacion", "verificar-auditoria", "-entrada", ruta})
		return rc, stdout.String() + stderr.String()
	}
	if rc, salida := ejecutar(); rc != 0 || !strings.Contains(salida, "3") {
		t.Fatalf("rc=%d salida=%q", rc, salida)
	}
	data, _ := os.ReadFile(ruta)
	if err := os.WriteFile(ruta, bytes.Replace(data, []byte("cofirma"), []byte("borrado"), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, salida := ejecutar(); rc == 0 || !strings.Contains(salida, "registro 3") {
		t.Fatalf("la manipulación debe detectarse: rc=%d salida=%q", rc, salida)
	}
}

// Sin -entrada se verifica el registro del usuario.
func TestRunVerificarAuditoria_SinEntradaUsaElRegistroDelUsuario(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr strings.Builder
	a := New(nil, nil)
	a.Stdout, a.Stderr = &stdout, &stderr
	if rc := a.Run(context.Background(), []string{"-modo-cli", "-operacion", "verificar-auditoria"}); rc != 0 || !strings.Contains(stdout.String(), "0") {
		t.Fatalf("rc=%d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
	}
}

// La contraseña de un PDF cifrado llega a la firma desde la entrada segura,
// nunca desde argv.
func TestRunFirmar_ContrasenaPDFPorEntradaSegura(t *testing.T) {
	entrada := filepath.Join(t.TempDir(), "cifrado.pdf")
	if err := os.WriteFile(entrada, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	firma := &signMock{}
	a := New(firma, nil)
	a.Stdin = strings.NewReader("secreto-pdf\n")
	var stdout, stderr strings.Builder
	a.Stdout, a.Stderr = &stdout, &stderr
	a.Run(context.Background(), []string{"-modo-cli", "-operacion", "firmar", "-formato", "pades", "-entrada", entrada, "-contrasena-pdf-stdin", "-no-guardar"})
	if firma.last.Options["userPassword"] != "secreto-pdf" {
		t.Fatalf("la contraseña no llegó a la firma: %v (stderr=%q)", firma.last.Options, stderr.String())
	}
	if rc := New(firma, nil).Run(context.Background(), []string{"-modo-cli", "-operacion", "firmar", "-entrada", entrada, "-opcion", "userPassword=x"}); rc == 0 {
		t.Fatal("la contraseña por argv debe rechazarse")
	}
}
