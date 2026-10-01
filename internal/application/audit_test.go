// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"grxfirma/internal/application"
)

func TestAuditUseCase_Registrar_IncluyeHashYCamposSeguros(t *testing.T) {
	t.Parallel()

	logger := &loggerMock{}
	uc := application.NuevoAuditUseCase(relojMock{}, logger)
	documento := []byte("contenido muy sensible del documento")

	err := uc.Registrar(context.Background(), application.AuditCommand{
		OperationType:          "firma",
		Origin:                 "https://sede.ejemplo.es",
		CertificateFingerprint: "ABCDEF1234",
		DocumentName:           "contrato.pdf",
		DocumentData:           documento,
		Format:                 "CAdES",
		Success:                true,
	})
	if err != nil {
		t.Fatalf("Registrar() error = %v", err)
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d, want 1", len(logger.registros))
	}

	var payload map[string]any
	if err := json.Unmarshal(logger.registros[0].Payload, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	sum := sha256.Sum256(documento)
	wantHash := hex.EncodeToString(sum[:])
	if payload["documento_sha256"] != wantHash {
		t.Fatalf("documento_sha256 = %v, want %s", payload["documento_sha256"], wantHash)
	}
	if payload["resultado"] != "ok" {
		t.Fatalf("resultado = %v, want ok", payload["resultado"])
	}
	if payload["cert_fingerprint"] != "abcdef1234" {
		t.Fatalf("cert_fingerprint = %v", payload["cert_fingerprint"])
	}
	if strings.Contains(string(logger.registros[0].Payload), string(documento)) {
		t.Fatal("el payload de auditoria contiene el documento en claro")
	}
}

func TestAuditUseCase_Registrar_SanitizaResumenYNoPersisteCamposProhibidos(t *testing.T) {
	t.Parallel()

	logger := &loggerMock{}
	uc := application.NuevoAuditUseCase(relojMock{}, logger)
	documento := []byte("PRIVATE KEY SUPER SECRETA")

	err := uc.Registrar(context.Background(), application.AuditCommand{
		OperationType: "firma",
		Origin:        "afirma://\nmalicioso",
		DocumentName:  "doc.pdf",
		DocumentData:  documento,
		Success:       false,
		ErrorSummary:  "fallo\ninterno\tcon detalle",
	})
	if err != nil {
		t.Fatalf("Registrar() error = %v", err)
	}

	payload := string(logger.registros[0].Payload)
	if strings.Contains(payload, "PRIVATE KEY SUPER SECRETA") {
		t.Fatal("la auditoria persiste contenido del documento")
	}
	if strings.Contains(payload, "\n") {
		t.Fatal("la auditoria no debe contener saltos de linea en el JSONL")
	}
	if !strings.Contains(payload, "\"resultado\":\"error\"") {
		t.Fatalf("resultado inesperado: %s", payload)
	}
}
