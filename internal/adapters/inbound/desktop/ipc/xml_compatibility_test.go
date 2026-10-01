// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

func TestDespachar_Verify_CompatibilidadXMLNoAfirmaFirmaValida(t *testing.T) {
	t.Parallel()
	inputPath := filepath.Join(t.TempDir(), "legacy.xsig")
	if err := os.WriteFile(inputPath, []byte("<firma-sintetica/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	const warning = "La firma requiere una canonicalización de compatibilidad no declarada."
	verifier := &stubVerificar{result: application.VerifyResult{
		Verification: domain.VerificationResult{
			Valid:    false,
			Reason:   warning,
			Format:   "XAdES",
			Coverage: "unknown",
			Integrity: domain.VerificationAspect{
				Status: domain.VerificationStatusWarning,
				Reason: warning,
			},
			Certificate: domain.VerificationAspect{Status: domain.VerificationStatusValid},
			Trust:       domain.VerificationAspect{Status: domain.VerificationStatusValid},
			Warnings:    []string{warning},
			Evidence: []domain.VerificationEvidence{{
				Type:    "xml.canonicalization.compatibility",
				Summary: warning,
			}},
		},
	}}
	m := &Manejador{Verificar: verifier}
	resp := m.despachar(context.Background(), peticionJSON(t, "verify", paramsVerify{InputPath: inputPath}))

	// El sobre describe la ejecución de Verify, no la validez de la firma.
	// Se comprueba también el JSON real: valid=false no puede desaparecer como
	// valor cero ni transformarse en éxito por Certificate/Trust válidos.
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		OK      bool   `json:"ok"`
		Action  string `json:"action"`
		Outcome string `json:"outcome"`
		Data    struct {
			Valid       *bool                            `json:"valid"`
			Reason      string                           `json:"reason"`
			Format      string                           `json:"format"`
			Coverage    string                           `json:"coverage"`
			Integrity   resultadoVerificacionAspecto     `json:"integrity"`
			Certificate resultadoVerificacionAspecto     `json:"certificate"`
			Trust       resultadoVerificacionAspecto     `json:"trust"`
			Warnings    []string                         `json:"warnings"`
			Evidence    []resultadoVerificacionEvidencia `json:"evidence"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if !wire.OK || wire.Action != "verify" || wire.Outcome != ipcOutcomeSuccess {
		t.Fatalf("Verify debe completar la operación: %s", raw)
	}
	if wire.Data.Valid == nil || *wire.Data.Valid {
		t.Fatalf("el contrato debe transportar valid=false explícito: %s", raw)
	}
	if wire.Data.Format != "XAdES" || wire.Data.Coverage != "unknown" || wire.Data.Reason != warning {
		t.Fatalf("clasificación de compatibilidad alterada: %s", raw)
	}
	if wire.Data.Integrity.Status != "warning" || wire.Data.Integrity.Reason != warning {
		t.Fatalf("integridad de compatibilidad debe seguir siendo advertencia: %s", raw)
	}
	if wire.Data.Certificate.Status != "valid" || wire.Data.Trust.Status != "valid" {
		t.Fatalf("certificado y confianza deben conservar su evaluación independiente: %s", raw)
	}
	if len(wire.Data.Warnings) != 1 || wire.Data.Warnings[0] != warning ||
		len(wire.Data.Evidence) != 1 || wire.Data.Evidence[0].Type != "xml.canonicalization.compatibility" ||
		wire.Data.Evidence[0].Summary != warning {
		t.Fatalf("advertencia o evidencia de compatibilidad perdida: %s", raw)
	}
}
