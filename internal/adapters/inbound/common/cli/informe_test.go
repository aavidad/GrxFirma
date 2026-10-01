// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVerificar_GuardaInformeSinAlterarJSON(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "firma.pdf")
	if err := os.WriteFile(entrada, []byte("%PDF-1.7\nfirma"), 0o600); err != nil {
		t.Fatal(err)
	}
	informe := filepath.Join(tmp, "informe.html")
	a := New(nil, nil).WithVerificador(verifyMock{})
	var stdout, stderr strings.Builder
	a.Stdout, a.Stderr = &stdout, &stderr
	rc := a.Run(context.Background(), []string{"-operacion", "verificar", "-entrada", entrada, "-informe", informe, "-salida-json"})
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("la salida JSON debe seguir siendo JSON: %v\n%s", err, stdout.String())
	}
	html, err := os.ReadFile(informe)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "FIRMA VÁLIDA") || !strings.Contains(string(html), "firma.pdf") {
		t.Fatalf("informe inesperado:\n%s", html)
	}
}
