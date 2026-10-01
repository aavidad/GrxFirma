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
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// --- Mocks ---

type catalogoMock struct {
	certs []domain.CertificateRef
	err   error
}

func (m *catalogoMock) List(_ context.Context) ([]domain.CertificateRef, error) {
	return m.certs, m.err
}

type proveedorClavesMock struct {
	clave    ports.SigningKey
	err      error
	llamadas int
}

func (m *proveedorClavesMock) KeyFor(_ context.Context, _ domain.CertificateRef) (ports.SigningKey, error) {
	m.llamadas++
	return m.clave, m.err
}

type claveMock struct{ id string }

func (c claveMock) KeyID() string { return c.id }
func (c claveMock) CertificateChainDER() [][]byte {
	return [][]byte{[]byte("cert-der")}
}

type claveCerrableMock struct {
	claveMock
	cierres int
}

func (c *claveCerrableMock) Close() {
	c.cierres++
}

type motorMock struct {
	resultado domain.SignatureResult
	err       error
}

func (m *motorMock) Sign(_ context.Context, _ domain.SignatureJob, _ ports.SigningKey) (domain.SignatureResult, error) {
	return m.resultado, m.err
}

type aprobadorMock struct {
	aprobado bool
	err      error
	mensaje  string
}

func (m *aprobadorMock) Request(_ context.Context, mensaje string) (bool, error) {
	m.mensaje = mensaje
	return m.aprobado, m.err
}

type loggerMock struct{ registros []ports.Evidence }

func (m *loggerMock) Log(_ context.Context, e ports.Evidence) error {
	m.registros = append(m.registros, e)
	return nil
}

type relojMock struct{}

func (r relojMock) Now() time.Time { return time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC) }

type publicadorMock struct{ eventos []ports.Event }

func (p *publicadorMock) Publish(_ context.Context, e ports.Event) error {
	p.eventos = append(p.eventos, e)
	return nil
}

// --- Helpers ---

func certPrueba() domain.CertificateRef {
	return domain.CertificateRef{
		ID:          "cert-001",
		Subject:     "CN=Test, O=Dipgra",
		Issuer:      "CN=CA Dipgra",
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		Fingerprint: "ab:cd:ef",
	}
}

func docPrueba() domain.Document {
	doc, _ := domain.NewDocument("contrato.pdf", []byte("contenido del pdf"), "application/pdf")
	return doc
}

func nuevoCasoDeUso(
	certs []domain.CertificateRef,
	aprobado bool,
	resultadoFirma domain.SignatureResult,
	logger *loggerMock,
	publicador *publicadorMock,
) *application.SignDocumentUseCase {
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	return application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: certs},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		&motorMock{resultado: resultadoFirma},
		&aprobadorMock{aprobado: aprobado},
		auditor,
		publicador,
	)
}

// --- Tests ---

func TestSignDocument_Exito(t *testing.T) {
	logger := &loggerMock{}
	publicador := &publicadorMock{}
	resultado := domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma"), Algorithm: "RSA"}

	uc := nuevoCasoDeUso([]domain.CertificateRef{certPrueba()}, true, resultado, logger, publicador)

	cmd := application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}

	res, err := uc.Ejecutar(context.Background(), cmd)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if string(res.Result.Data) != "firma" {
		t.Fatal("el resultado de firma no coincide")
	}
	if len(logger.registros) != 1 {
		t.Fatalf("se esperaba 1 registro de auditoria, se obtuvo %d", len(logger.registros))
	}
	if len(publicador.eventos) < 2 {
		t.Fatalf("se esperaban al menos 2 eventos publicados, se obtuvieron %d", len(publicador.eventos))
	}
}

func TestSignDocument_ConsentimientoMuestraAplicacionYOrigenSaneados(t *testing.T) {
	aprobador := &aprobadorMock{aprobado: true}
	uc := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"clave-001"}},
		&motorMock{resultado: domain.SignatureResult{
			Format: domain.FormatCAdES,
			Data:   []byte("firma"),
		}},
		aprobador,
		nil,
		nil,
	)
	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document:             docPrueba(),
		Format:               domain.FormatCAdES,
		Action:               domain.ActionSign,
		RequesterApplication: "Extensión GrxFirma\ninyectado",
		RequesterOrigin:      "https://sede.example.test\r\nengaño",
	})
	if err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	for _, expected := range []string{
		"Aplicación solicitante: Extensión GrxFirma inyectado",
		"Origen solicitante: https://sede.example.test engaño",
	} {
		if !strings.Contains(aprobador.mensaje, expected) {
			t.Errorf("mensaje de aprobación %q no contiene %q", aprobador.mensaje, expected)
		}
	}
	if strings.ContainsAny(aprobador.mensaje, "\r") {
		t.Fatalf("mensaje contiene retorno de carro: %q", aprobador.mensaje)
	}
}

func TestSignDocument_UsuarioCancela(t *testing.T) {
	logger := &loggerMock{}
	uc := nuevoCasoDeUso([]domain.CertificateRef{certPrueba()}, false, domain.SignatureResult{}, logger, &publicadorMock{})

	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	if err == nil {
		t.Fatal("se esperaba error por cancelacion del usuario")
	}
	if len(logger.registros) != 1 {
		t.Fatal("la cancelacion debe quedar registrada en auditoria")
	}
}

func TestSignDocument_SinCertificados(t *testing.T) {
	logger := &loggerMock{}
	uc := nuevoCasoDeUso([]domain.CertificateRef{}, true, domain.SignatureResult{}, logger, &publicadorMock{})

	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	if err == nil {
		t.Fatal("se esperaba error por falta de certificados")
	}
}

func TestSignDocument_FormatoVacioNoValido(t *testing.T) {
	logger := &loggerMock{}
	uc := nuevoCasoDeUso([]domain.CertificateRef{certPrueba()}, true, domain.SignatureResult{}, logger, &publicadorMock{})

	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(),
		Format:   domain.SignatureFormat(""),
		Action:   domain.ActionSign,
	})
	if err == nil {
		t.Fatal("se esperaba error por formato no valido")
	}
}

func TestSignDocument_ErrorDelMotor(t *testing.T) {
	logger := &loggerMock{}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		&proveedorClavesMock{clave: claveMock{"k"}},
		&motorMock{err: errors.New("fallo criptografico interno")},
		&aprobadorMock{aprobado: true},
		auditor,
		&publicadorMock{},
	)

	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	if err == nil {
		t.Fatal("se esperaba error del motor de firma")
	}
	if len(logger.registros) != 1 {
		t.Fatal("el fallo del motor debe quedar registrado en auditoria")
	}
}

func TestSignDocument_SinAprobadorFallaAntesDeUsarLaClave(t *testing.T) {
	t.Parallel()

	proveedor := &proveedorClavesMock{clave: claveMock{"no-debe-usarse"}}
	motor := &motorMock{resultado: domain.SignatureResult{
		Format: domain.FormatCAdES,
		Data:   []byte("no-debe-firmarse"),
	}}
	uc := application.NuevoSignDocumentUseCase(
		&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
		proveedor,
		motor,
		nil,
		nil,
		nil,
	)

	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	if err == nil || !strings.Contains(err.Error(), "aprobador de firma no configurado") {
		t.Fatalf("Ejecutar() error = %v, want error fail-closed", err)
	}
	if proveedor.llamadas != 0 {
		t.Fatalf("proveedor.llamadas = %d, want 0", proveedor.llamadas)
	}
}

func TestSignDocument_CierraClaveEnExitoYErrorDelMotor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		motorErr error
	}{
		{name: "exito"},
		{name: "error", motorErr: errors.New("fallo criptografico")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			key := &claveCerrableMock{claveMock: claveMock{id: "key-close"}}
			uc := application.NuevoSignDocumentUseCase(
				&catalogoMock{certs: []domain.CertificateRef{certPrueba()}},
				&proveedorClavesMock{clave: key},
				&motorMock{
					resultado: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("firma")},
					err:       tc.motorErr,
				},
				&aprobadorMock{aprobado: true},
				nil,
				nil,
			)

			_, err := uc.Ejecutar(context.Background(), application.SignCommand{
				Document: docPrueba(),
				Format:   domain.FormatCAdES,
				Action:   domain.ActionSign,
			})
			if (err != nil) != (tc.motorErr != nil) {
				t.Fatalf("Ejecutar() error = %v", err)
			}
			if key.cierres != 1 {
				t.Fatalf("cierres de clave = %d, want 1", key.cierres)
			}
		})
	}
}

func TestSignDocument_CertificadoPorID(t *testing.T) {
	cert1 := certPrueba()
	cert2 := domain.CertificateRef{ID: "cert-002", Subject: "CN=Otro", Fingerprint: "11:22:33", NotAfter: time.Now().Add(time.Hour)}
	logger := &loggerMock{}
	uc := nuevoCasoDeUso([]domain.CertificateRef{cert1, cert2}, true, domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("ok")}, logger, &publicadorMock{})

	res, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document:      docPrueba(),
		Format:        domain.FormatCAdES,
		Action:        domain.ActionSign,
		CertificateID: "cert-002",
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if res.CertificateUsed.ID != "cert-002" {
		t.Fatalf("se esperaba cert-002, se uso %s", res.CertificateUsed.ID)
	}
}

func TestSignDocument_VariosCertSinID(t *testing.T) {
	cert1 := certPrueba()
	cert2 := domain.CertificateRef{ID: "cert-002", Subject: "CN=Otro", Fingerprint: "11:22:33", NotAfter: time.Now().Add(time.Hour)}
	logger := &loggerMock{}
	uc := nuevoCasoDeUso([]domain.CertificateRef{cert1, cert2}, true, domain.SignatureResult{}, logger, &publicadorMock{})

	_, err := uc.Ejecutar(context.Background(), application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	if err == nil {
		t.Fatal("se esperaba error por ambiguedad de certificado")
	}
}

func TestSignDocument_ContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar antes de ejecutar

	logger := &loggerMock{}
	catalogo := &catalogoMock{err: context.Canceled}
	auditor := application.NuevoAuditUseCase(relojMock{}, logger)
	uc := application.NuevoSignDocumentUseCase(
		catalogo,
		&proveedorClavesMock{},
		&motorMock{},
		&aprobadorMock{},
		auditor,
		&publicadorMock{},
	)

	_, err := uc.Ejecutar(ctx, application.SignCommand{
		Document: docPrueba(),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	})
	if err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
}
