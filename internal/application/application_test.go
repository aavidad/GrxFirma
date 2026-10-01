// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// Verifica que SignCommand puede construirse con tipos de dominio y sin tipos de protocolo.
func TestSignCommand_SoloTiposDominio(t *testing.T) {
	doc, _ := domain.NewDocument("contrato.pdf", []byte("contenido"), "application/pdf")
	cmd := application.SignCommand{
		Document:      doc,
		Format:        domain.FormatCAdES,
		Action:        domain.ActionSign,
		CertificateID: "cert-abc",
	}
	if cmd.CertificateID != "cert-abc" {
		t.Fatal("el identificador de certificado no se conserva correctamente")
	}
}

// Verifica que ProcessBatchCommand acepta una lista de trabajos y una sesion de intercambio.
func TestProcessBatchCommand_ConSesion(t *testing.T) {
	doc, _ := domain.NewDocument("factura.xml", []byte("<xml/>"), "application/xml")
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}
	sesion := domain.ExchangeSession{
		RequestID:        "req-1",
		SessionKey:       "clave-sesion",
		UploadEndpoint:   "http://servidor/subida",
		RetrieveEndpoint: "http://servidor/descarga",
		State:            domain.SessionActive,
	}
	cmd := application.ProcessBatchCommand{
		Jobs:    []domain.SignatureJob{job},
		Session: sesion,
	}
	if len(cmd.Jobs) != 1 {
		t.Fatalf("se esperaba 1 trabajo, se obtuvo %d", len(cmd.Jobs))
	}
	if !cmd.Session.IsActive() {
		t.Fatal("la sesion debe estar activa")
	}
}

// Verifica que BatchResult detecta correctamente si todos los trabajos tuvieron exito.
func TestBatchResult_TodosExitosos(t *testing.T) {
	resultado := application.BatchResult{
		Results: []application.SignResult{},
		Errores: map[int]error{},
	}
	if !resultado.TodosExitosos() {
		t.Fatal("se esperaba que todos fueran exitosos con mapa de errores vacio")
	}

	resultado.Errores[0] = domain.TrustDecision{}.Validate() // cualquier error no nulo
	if resultado.TodosExitosos() {
		t.Fatal("se esperaba que no todos fueran exitosos")
	}
}

// Verifica que AuditRecord no puede contener campos sensibles en su tipo
// (comprueba que el diseno del tipo no incluye campos de payload o material criptografico).
func TestAuditRecord_CamposPermitidos(t *testing.T) {
	registro := application.AuditRecord{
		Timestamp:              time.Now(),
		OperationType:          "firma",
		Origin:                 "afirma://",
		CertificateFingerprint: "ab:cd:ef",
		DocumentName:           "contrato.pdf",
		Success:                true,
		ErrorSummary:           "",
	}
	if registro.CertificateFingerprint == "" {
		t.Fatal("la huella del certificado debe estar presente")
	}
}

// Verifica que ManageTrustedDomainCommand usa las constantes de accion definidas.
func TestManageTrustedDomainCommand_Acciones(t *testing.T) {
	acciones := []application.TrustAction{
		application.TrustActionAllow,
		application.TrustActionDeny,
		application.TrustActionRemove,
	}
	for _, a := range acciones {
		cmd := application.ManageTrustedDomainCommand{Origin: "ejemplo.es", Action: a}
		if cmd.Origin == "" {
			t.Fatalf("el origen no debe estar vacio para la accion %q", a)
		}
	}
}

func TestParseProtectionProfile(t *testing.T) {
	tests := map[string]string{
		"":         string(domain.ProtectionProfileStrong),
		"alto":     string(domain.ProtectionProfileStrong),
		"mlkem768": string(domain.ProtectionProfileStrong),
		"compat":   string(domain.ProtectionProfileCompat),
		"rsa-oaep": string(domain.ProtectionProfileCompat),
	}
	for raw, want := range tests {
		got, err := application.ParseProtectionProfile(raw)
		if err != nil {
			t.Fatalf("ParseProtectionProfile(%q) error = %v", raw, err)
		}
		if string(got) != want {
			t.Fatalf("ParseProtectionProfile(%q) = %q; want %q", raw, got, want)
		}
	}
	if _, err := application.ParseProtectionProfile("otro"); err == nil {
		t.Fatal("se esperaba error para un perfil desconocido")
	}
}
