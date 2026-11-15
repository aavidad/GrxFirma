// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package session_test

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	"grxfirma/internal/adapters/inbound/desktop/session"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/presentation/desktop/certpicker"
)

func TestManifiestoRecuperadoNoCedeTSAHTTPAlFirmadorSinAjuste(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	params := url.QueryEscape("profile=baseline\ntsaURL=http://tsa.example.invalid/tsr")
	manifest := []byte(`<sign><e k="id" v="req-tsa"/><e k="stservlet" v="https%3A%2F%2Fexample.com%2FStorageService"/><e k="format" v="CAdES"/><e k="dat" v="ZG9jdW1lbnRv"/><e k="extraParams" v="` + params + `"/></sign>`)
	firmas, subidas := 0, 0
	orch := session.New(session.Config{
		Aprobador: aprobadorPrueba{}, ComponerAprobacion: componerAprobacionPrueba,
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, refs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: refs[0]}, nil
		}},
		Signer: &mockSignerEngine{signFn: func(context.Context, domain.SignatureJob, ports.SigningKey) (domain.SignatureResult, error) {
			firmas++
			return domain.SignatureResult{Data: []byte("firma")}, nil
		}},
		Progress: &mockProgressProvider{}, Notify: &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{evaluateFn: func(context.Context, string) (domain.TrustDecision, error) {
			return domain.TrustDecision{Status: domain.TrustAllowed}, nil
		}}),
		TriphaseExec: &mockSimpleExecutor{
			retrieveRawFn:     func(context.Context, domain.ExchangeSession) ([]byte, error) { return manifest, nil },
			uploadSignatureFn: func(context.Context, domain.ExchangeSession, []byte, []byte) error { subidas++; return nil },
		},
	})
	request := afirmauri.Solicitud{Operacion: afirmauri.OperacionFirma, Origenes: []string{"https://example.com"}, Sesion: sesionValida(), RetrieveCommand: &application.RetrieveRequestCommand{Session: sesionValida()}}
	ctx, warnings := domain.ConAvisosOperacion(context.Background())
	err := orch.HandleRequest(ctx, request)
	var blocked *excepciones.Block
	if !errors.As(err, &blocked) || blocked.ID != "tsa_http" || firmas != 0 || subidas != 0 || len(warnings.Claves()) != 0 {
		t.Fatalf("TSA HTTP se cedió sin ajuste: err=%v firmas=%d subidas=%d avisos=%v", err, firmas, subidas, warnings.Claves())
	}
	excepciones.SetForTest(t, "tsa_http", true)
	ctx, warnings = domain.ConAvisosOperacion(context.Background())
	if err := orch.HandleRequest(ctx, request); err != nil || firmas != 1 || subidas != 1 {
		t.Fatalf("TSA HTTP autorizada no completó: err=%v firmas=%d subidas=%d", err, firmas, subidas)
	}
	if !slices.Contains(warnings.Claves(), excepciones.Warning("tsa_http")) {
		t.Fatalf("falta aviso TSA en resultado: %v", warnings.Claves())
	}
}
