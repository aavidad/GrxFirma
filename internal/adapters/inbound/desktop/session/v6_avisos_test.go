// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package session_test

import (
	"context"
	"grxfirma/internal/adapters/inbound/desktop/session"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/presentation/desktop/certpicker"
	"strings"
	"testing"
)

func TestV6AfirmaPublicaAvisosDeOperacion(t *testing.T) {
	for _, id := range []string{"tsa_http", "trifasica_http"} {
		e, _ := excepciones.Find(id)
		machinepolicy.UnsetForTest(t, e.PolicyKey)
		excepciones.SetForTest(t, id, true)
	}
	events := &mockEventPublisher{}
	var message string
	notify := &mockDesktopNotification{notifyFn: func(_ context.Context, _ string, body string) error { message = body; return nil }}
	orch := session.New(session.Config{
		Aprobador: aprobadorPrueba{}, ComponerAprobacion: componerAprobacionPrueba, ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{}, CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: &mockProgressProvider{}, Notify: notify, Eventos: events, TrustValidator: originvalidator.New(&mockTrustPolicy{}),
		TriphaseExec: &mockSimpleExecutor{executeFn: func(ctx context.Context, _ domain.ExchangeSession, job domain.SignatureJob) (domain.SignatureResult, error) {
			for _, id := range []string{"tsa_http", "trifasica_http"} {
				if err := excepciones.Require(ctx, id); err != nil {
					return domain.SignatureResult{}, err
				}
			}
			return domain.SignatureResult{Format: job.Format, Data: []byte("firma")}, nil
		}},
	})
	if err := orch.HandleRequest(context.Background(), solicitudSimpleValida()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tsa_http", "trifasica_http"} {
		key := excepciones.Warning(id)
		found := false
		for _, event := range events.eventos {
			if event.Type == "session.warning" && string(event.Payload) == key {
				found = true
			}
		}
		if !found || !strings.Contains(message, localizador.Detectar().T(key)) {
			t.Fatalf("afirma pierde %s: eventos=%v notificación=%q", id, found, message)
		}
	}
}
