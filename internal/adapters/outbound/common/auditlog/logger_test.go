// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package auditlog_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/ports"
)

func TestLogger_Log_EscribeJSONL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := auditlog.New(path, 1024)

	err := logger.Log(context.Background(), ports.Evidence{
		Type:      "operacion.firma",
		Timestamp: time.Now(),
		Payload:   []byte(`{"operacion":"firma","resultado":"ok"}`),
	})
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "{\"cadena\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"operacion\":\"firma\",\"resultado\":\"ok\"}\n" {
		t.Fatalf("contenido inesperado: %q", string(data))
	}
}

func TestLogger_Log_RotaPorTamano(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := auditlog.New(path, 140)

	if err := logger.Log(context.Background(), ports.Evidence{Payload: []byte(`{"n":1}`)}); err != nil {
		t.Fatalf("primer Log() error = %v", err)
	}
	if err := logger.Log(context.Background(), ports.Evidence{Payload: []byte(`{"n":2,"payload":"12345678901234567890"}`)}); err != nil {
		t.Fatalf("segundo Log() error = %v", err)
	}

	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("ReadFile(rotated) error = %v", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(current) error = %v", err)
	}

	primero := "{\"cadena\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"n\":1}"
	if string(rotated) != primero+"\n" {
		t.Fatalf("rotado inesperado: %q", string(rotated))
	}
	// La cadena continúa a través de la rotación.
	if string(current) != "{\"cadena\":\""+auditlog.Huella([]byte(primero))+"\",\"n\":2,\"payload\":\"12345678901234567890\"}\n" {
		t.Fatalf("actual inesperado: %q", string(current))
	}
	if inf, err := auditlog.VerificarCadena(path+".1", path); err != nil || inf.Registros != 2 || inf.InicioRetirado {
		t.Fatalf("VerificarCadena() = %+v, %v", inf, err)
	}
}

func TestLogger_Log_RetiraAuditoriaFueraDeRetencion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte("old-current\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(current): %v", err)
	}
	if err := os.WriteFile(path+".1", []byte("old-rotated\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(rotated): %v", err)
	}
	old := time.Now().AddDate(-1, 0, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes(current): %v", err)
	}
	if err := os.Chtimes(path+".1", old, old); err != nil {
		t.Fatalf("Chtimes(rotated): %v", err)
	}

	logger := auditlog.New(path, 1024)
	if err := logger.Log(context.Background(), ports.Evidence{
		Payload: []byte(`{"nuevo":true}`),
	}); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(current): %v", err)
	}
	// Retirado todo el registro anterior, la cadena vuelve a empezar.
	if string(data) != "{\"cadena\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"nuevo\":true}\n" {
		t.Fatalf("contenido tras retencion = %q", data)
	}
	if _, err := os.Lstat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("la rotacion caducada no fue retirada: %v", err)
	}
}

func TestLogger_Log_RechazaEnlaceEnDestino(t *testing.T) {
	dir := t.TempDir()
	external := filepath.Join(dir, "external.jsonl")
	if err := os.WriteFile(external, []byte("external\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(external): %v", err)
	}
	link := filepath.Join(dir, "audit.jsonl")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlinks no disponibles: %v", err)
	}
	logger := auditlog.New(link, 1024)
	if err := logger.Log(context.Background(), ports.Evidence{
		Payload: []byte(`{"nuevo":true}`),
	}); err == nil {
		t.Fatal("Log aceptó un enlace simbólico como auditoría")
	}
	data, err := os.ReadFile(external)
	if err != nil {
		t.Fatalf("ReadFile(external): %v", err)
	}
	if string(data) != "external\n" {
		t.Fatalf("el destino externo fue alterado: %q", data)
	}
}

func TestLogger_Log_RechazaEvidenciaMayorQueElLimite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := auditlog.New(path, 8)
	if err := logger.Log(context.Background(), ports.Evidence{
		Payload: []byte(`{"demasiado":"grande"}`),
	}); err == nil {
		t.Fatal("Log aceptó una evidencia mayor que el límite")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("se creó auditoría tras rechazar el registro: %v", err)
	}
}
