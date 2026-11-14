// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// motorQueAvisa anota un aviso al firmar los documentos cuyo nombre esté en
// conAviso, como hace el motor real con la cofirma sin documento o la CRL
// que no cubre al firmante.
type motorQueAvisa struct {
	conAviso map[string]string
}

func (m *motorQueAvisa) Sign(ctx context.Context, job domain.SignatureJob, _ ports.SigningKey) (domain.SignatureResult, error) {
	if clave, ok := m.conAviso[job.Document.Name]; ok {
		domain.AnotarAviso(ctx, clave)
	}
	return domain.SignatureResult{Format: job.Format, Data: []byte("firma-" + job.Document.Name), Algorithm: "RSA"}, nil
}

// Lote 16c: el resultado de la firma lleva los avisos que el motor ha dado en
// esa operación, para que la CLI y las interfaces los muestren.
func TestSignDocument_DevuelveLosAvisosDelMotor(t *testing.T) {
	motor := &motorQueAvisa{conAviso: map[string]string{"contrato.pdf": domain.AvisoCofirmaSinOriginal}}
	uc := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		motor,
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, &loggerMock{}),
		&publicadorMock{},
	)
	cmd := application.SignCommand{Document: docPrueba(), Format: domain.FormatCAdES, Action: domain.ActionCoSign}
	res, err := uc.Ejecutar(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != domain.AvisoCofirmaSinOriginal {
		t.Fatalf("Warnings = %q", res.Warnings)
	}
	// Otra operación empieza sin avisos.
	motor.conAviso = nil
	if res, err = uc.Ejecutar(context.Background(), cmd); err != nil || len(res.Warnings) != 0 {
		t.Fatalf("una operación sin avisos no hereda los de la anterior: %q err=%v", res.Warnings, err)
	}
}

func TestProcessBatch_CadaTrabajoLlevaSusAvisos(t *testing.T) {
	motor := &motorQueAvisa{conAviso: map[string]string{"b.pdf": domain.AvisoCRLNoAplicable}}
	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		motor,
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, &loggerMock{}),
		&publicadorMock{},
	)
	resultado, err := uc.Ejecutar(context.Background(), lotePrueba())
	if err != nil {
		t.Fatal(err)
	}
	if len(resultado.Results) != 2 {
		t.Fatalf("results = %d", len(resultado.Results))
	}
	if len(resultado.Results[0].Warnings) != 0 {
		t.Fatalf("a.pdf no lleva avisos: %q", resultado.Results[0].Warnings)
	}
	if w := resultado.Results[1].Warnings; len(w) != 1 || w[0] != domain.AvisoCRLNoAplicable {
		t.Fatalf("b.pdf lleva el aviso de CRL: %q", w)
	}
}

func TestAvisosOperacion_SinRegistroNoHaceNadaYNoRepite(t *testing.T) {
	domain.AnotarAviso(context.Background(), domain.AvisoCRLNoAplicable) // sin registro: no falla
	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	domain.AnotarAviso(ctx, domain.AvisoCRLNoAplicable)
	domain.AnotarAviso(ctx, domain.AvisoCRLNoAplicable)
	domain.AnotarAviso(ctx, " ")
	if claves := avisos.Claves(); len(claves) != 1 || claves[0] != domain.AvisoCRLNoAplicable {
		t.Fatalf("Claves = %q", claves)
	}
}
