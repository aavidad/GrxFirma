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

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type verifyResultadoMock struct {
	resultado domain.VerificationResult
}

func (m verifyResultadoMock) Execute(_ context.Context, _ application.VerifyCommand) (application.VerifyResult, error) {
	return application.VerifyResult{Verification: m.resultado}, nil
}

func TestVerifyFirmaNoValidaDevuelve2(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "firma.pdf")
	if err := os.WriteFile(entrada, []byte("%PDF-1.7\nfirma"), 0o600); err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		nombre    string
		resultado domain.VerificationResult
		args      []string
		esperado  int
	}{
		{"texto no valida", domain.NewVerificationFailure("PAdES", "firma alterada", nil), nil, 2},
		{"json no valida", domain.NewVerificationFailure("PAdES", "firma alterada", nil), []string{"-salida-json"}, 2},
		{"valida con integridad invalida", domain.VerificationResult{Valid: true, Integrity: domain.VerificationAspect{Status: domain.VerificationStatusInvalid}}, nil, 2},
		{"texto valida", domain.NewVerificationSuccess("PAdES", "ok", nil), nil, 0},
		{"json valida", domain.NewVerificationSuccess("PAdES", "ok", nil), []string{"-salida-json"}, 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			adaptador := New(nil, nil).WithVerificador(verifyResultadoMock{resultado: c.resultado})
			var stdout, stderr strings.Builder
			adaptador.Stdout = &stdout
			adaptador.Stderr = &stderr
			args := append([]string{"-operacion", "verificar", "-entrada", entrada}, c.args...)
			if rc := adaptador.Run(context.Background(), args); rc != c.esperado {
				t.Fatalf("codigo = %d, quiero %d; stdout=%s stderr=%s", rc, c.esperado, stdout.String(), stderr.String())
			}
		})
	}
}
