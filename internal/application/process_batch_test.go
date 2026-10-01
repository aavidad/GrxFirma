// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type motorBatchMock struct {
	resultados []domain.SignatureResult
	errores    map[int]error
	llamadas   int
}

type proveedorClavesCancelMock struct {
	clave  ports.SigningKey
	cancel context.CancelFunc
}

func (m *proveedorClavesCancelMock) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	m.cancel()
	return m.clave, nil
}

func (m *motorBatchMock) Sign(_ context.Context, _ domain.SignatureJob, _ ports.SigningKey) (domain.SignatureResult, error) {
	index := m.llamadas
	m.llamadas++
	if err := m.errores[index]; err != nil {
		return domain.SignatureResult{}, err
	}
	if index < len(m.resultados) {
		return m.resultados[index], nil
	}
	return domain.SignatureResult{}, nil
}

func lotePrueba() application.ProcessBatchCommand {
	doc1, _ := domain.NewDocument("a.pdf", []byte("a"), "application/pdf")
	doc2, _ := domain.NewDocument("b.pdf", []byte("b"), "application/pdf")
	return application.ProcessBatchCommand{
		Jobs: []domain.SignatureJob{
			{Document: doc1, Format: domain.FormatCAdES, Action: domain.ActionSign},
			{Document: doc2, Format: domain.FormatCAdES, Action: domain.ActionSign},
		},
		Session: domain.ExchangeSession{
			RequestID:        "req-1",
			SessionKey:       "k",
			UploadEndpoint:   "https://upload",
			RetrieveEndpoint: "https://retrieve",
			State:            domain.SessionActive,
		},
	}
}

func TestProcessBatch_Exito(t *testing.T) {
	t.Parallel()

	logger := &loggerMock{}
	publicador := &publicadorMock{}
	motor := &motorBatchMock{resultados: []domain.SignatureResult{
		{Format: domain.FormatCAdES, Data: []byte("sig-a"), Algorithm: "RSA"},
		{Format: domain.FormatCAdES, Data: []byte("sig-b"), Algorithm: "RSA"},
	}}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		motor,
		&aprobadorMock{aprobado: true},
		auditor,
		publicador,
	)

	resultado, err := uc.Ejecutar(context.Background(), lotePrueba())
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if len(resultado.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(resultado.Results))
	}
	if len(resultado.Errores) != 0 {
		t.Fatalf("errores = %d, want 0", len(resultado.Errores))
	}
	if len(publicador.eventos) < 4 {
		t.Fatalf("eventos = %d, want >= 4", len(publicador.eventos))
	}
	if len(logger.registros) < 3 {
		t.Fatalf("registros = %d, want >= 3", len(logger.registros))
	}
}

func TestProcessBatch_ErrorParcial(t *testing.T) {
	t.Parallel()

	logger := &loggerMock{}
	motor := &motorBatchMock{
		resultados: []domain.SignatureResult{
			{Format: domain.FormatCAdES, Data: []byte("sig-a"), Algorithm: "RSA"},
		},
		errores: map[int]error{1: errors.New("fallo criptografico")},
	}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		motor,
		&aprobadorMock{aprobado: true},
		auditor,
		&publicadorMock{},
	)

	resultado, err := uc.Ejecutar(context.Background(), lotePrueba())
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if len(resultado.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(resultado.Results))
	}
	if len(resultado.Errores) != 1 || resultado.Errores[1] == nil {
		t.Fatalf("errores = %+v, want error en indice 1", resultado.Errores)
	}
}

func TestProcessBatch_StopOnErrorNoProcesaTrabajosPosteriores(t *testing.T) {
	t.Parallel()

	cmd := lotePrueba()
	doc3, _ := domain.NewDocument("c.pdf", []byte("c"), "application/pdf")
	cmd.Jobs = append(cmd.Jobs, domain.SignatureJob{
		Document: doc3,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	cmd.StopOnError = true

	motor := &motorBatchMock{
		resultados: []domain.SignatureResult{
			{Format: domain.FormatCAdES, Data: []byte("sig-a"), Algorithm: "RSA"},
		},
		errores: map[int]error{1: errors.New("fallo criptografico")},
	}
	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		motor,
		&aprobadorMock{aprobado: true},
		nil,
		nil,
	)

	resultado, err := uc.Ejecutar(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if motor.llamadas != 2 {
		t.Fatalf("llamadas al motor = %d, want 2", motor.llamadas)
	}
	if len(resultado.Results) != 1 || len(resultado.Errores) != 1 || resultado.Errores[1] == nil {
		t.Fatalf("resultado stopOnError inesperado: results=%d errores=%#v", len(resultado.Results), resultado.Errores)
	}
}

func TestProcessBatch_SinCertificados(t *testing.T) {
	t.Parallel()

	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{},
		&proveedorClavesMock{},
		&motorBatchMock{},
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, &loggerMock{}),
		&publicadorMock{},
	)

	if _, err := uc.Ejecutar(context.Background(), lotePrueba()); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}

func TestProcessBatch_VariosCertificadosSinSeleccion(t *testing.T) {
	t.Parallel()

	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba(), {
			ID: "cert-2", Subject: "CN=Otro", Fingerprint: "ff", NotAfter: certPrueba().NotAfter,
		}}},
		&proveedorClavesMock{},
		&motorBatchMock{},
		&aprobadorMock{aprobado: true},
		application.NuevoAuditUseCase(relojMock{}, &loggerMock{}),
		&publicadorMock{},
	)

	if _, err := uc.Ejecutar(context.Background(), lotePrueba()); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}

func TestProcessBatch_UsuarioCancela(t *testing.T) {
	t.Parallel()

	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		&motorBatchMock{},
		&aprobadorMock{aprobado: false},
		application.NuevoAuditUseCase(relojMock{}, &loggerMock{}),
		&publicadorMock{},
	)

	if _, err := uc.Ejecutar(context.Background(), lotePrueba()); err == nil {
		t.Fatal("Ejecutar() error = nil, want error")
	}
}

func TestProcessBatch_SinAprobadorFallaAntesDeFirmar(t *testing.T) {
	t.Parallel()

	motor := &motorBatchMock{}
	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"no-debe-usarse"}},
		motor,
		nil,
		nil,
		nil,
	)

	_, err := uc.Ejecutar(context.Background(), lotePrueba())
	if err == nil || !strings.Contains(err.Error(), "aprobador del lote no configurado") {
		t.Fatalf("Ejecutar() error = %v, want error fail-closed", err)
	}
	if motor.llamadas != 0 {
		t.Fatalf("motor.llamadas = %d, want 0", motor.llamadas)
	}
}

func TestProcessBatch_CierraClaveSiElContextoSeCancelaTrasAdquirirla(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	key := &claveCerrableMock{claveMock: claveMock{id: "key-close"}}
	uc := application.NuevoProcessBatchUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesCancelMock{clave: key, cancel: cancel},
		&motorBatchMock{},
		&aprobadorMock{aprobado: true},
		nil,
		nil,
	)

	result, err := uc.Ejecutar(ctx, lotePrueba())
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if !errors.Is(result.Errores[0], context.Canceled) {
		t.Fatalf("error del primer trabajo = %v, want context.Canceled", result.Errores[0])
	}
	if key.cierres != 1 {
		t.Fatalf("cierres de clave = %d, want 1", key.cierres)
	}
}
