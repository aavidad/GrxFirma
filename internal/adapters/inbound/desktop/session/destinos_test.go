// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package session_test

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/inbound/desktop/session"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
)

// A-03: un manifiesto <sign> recuperado no puede llevar la firma a un
// servlet que la política no permite.
func TestOrchestrator_ManifiestoConStservletNoPermitidoNoSube(t *testing.T) {
	manifiesto := []byte(`<sign><e k="id" v="r"/><e k="stservlet" v="https%3A%2F%2Fevil.example%2FStorageService"/><e k="format" v="CAdES"/><e k="dat" v="ZG9j"/></sign>`)
	var subido, firmado bool
	orch := session.New(session.Config{
		Aprobador:          aprobadorPrueba{},
		ComponerAprobacion: componerAprobacionPrueba,
		ParserLegacy:       afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Signer: &mockSignerEngine{signFn: func(context.Context, domain.SignatureJob, ports.SigningKey) (domain.SignatureResult, error) {
			firmado = true
			return domain.SignatureResult{Data: []byte("f")}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{evaluateFn: func(_ context.Context, origen string) (domain.TrustDecision, error) {
			if origen == "https://evil.example" {
				return domain.TrustDecision{Status: domain.TrustDenied}, nil
			}
			return domain.TrustDecision{Status: domain.TrustAllowed}, nil
		}}),
		TriphaseExec: &mockSimpleExecutor{
			retrieveRawFn: func(context.Context, domain.ExchangeSession) ([]byte, error) { return manifiesto, nil },
			uploadSignatureFn: func(context.Context, domain.ExchangeSession, []byte, []byte) error {
				subido = true
				return nil
			},
		},
	})
	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion:       afirmauri.OperacionFirma,
		Origenes:        []string{"https://example.com"},
		Sesion:          sesionValida(),
		RetrieveCommand: &application.RetrieveRequestCommand{Session: sesionValida()},
	})
	if err == nil || subido || firmado {
		t.Fatalf("err = %v, firmado = %v, subido = %v; se esperaba rechazo antes de firmar y subir", err, firmado, subido)
	}
}
