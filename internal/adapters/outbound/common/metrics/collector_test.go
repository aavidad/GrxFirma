// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package metrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectorExportaContadores(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")
	c := New(path, 1<<20)

	c.RecordCertificateSource(context.Background(), "p12")
	c.RecordProtocolRequest(context.Background(), "sign", "ok", time.Millisecond)
	c.RecordSign(context.Background(), "cades", "ok", 25*time.Millisecond)
	c.RecordSign(context.Background(), "cades", "error", 50*time.Millisecond)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{
		`"certificate_source"`,
		`"type": "p12"`,
		`"protocol_requests_total"`,
		`"op": "sign"`,
		`"sign_total"`,
		`"format": "cades"`,
		`"status": "ok"`,
		`"status": "error"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("no se encontro %q en %s", want, text)
		}
	}
}

func TestCollectorRotaCuandoSuperaLimite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")
	c := New(path, 100)

	_ = os.WriteFile(path, []byte(strings.Repeat("x", 95)), 0o600)
	c.RecordProtocolRequest(context.Background(), "verify", "ok", time.Millisecond)

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("se esperaba rotacion a .1: %v", err)
	}
}
