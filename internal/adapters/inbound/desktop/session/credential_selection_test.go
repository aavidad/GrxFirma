// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package session_test

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/inbound/desktop/session"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
)

type credentialAwareSelector struct {
	*mockCertSelector
	cleared bool
}

func (*credentialAwareSelector) SupportsCredentialLoading() bool { return true }
func (s *credentialAwareSelector) ClearCredentials()             { s.cleared = true }

func TestOrchestrator_SelectorConCargaRecuperaVacioYElegirOtroConUnico(t *testing.T) {
	for _, initial := range []string{"vacio", "unico", "caducado"} {
		t.Run(initial, func(t *testing.T) {
			var refs []domain.CertificateRef
			if initial == "unico" {
				refs = []domain.CertificateRef{certRef("anterior")}
			}
			if initial == "caducado" {
				refs = []domain.CertificateRef{certRefExpirado("anterior")}
			}
			selected := false
			selector := &credentialAwareSelector{mockCertSelector: &mockCertSelector{selectFn: func(context.Context, []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				selected = true
				return certpicker.ResultadoSeleccion{Certificado: certRef("nuevo-p12")}, nil
			}}}
			keyID := ""
			orch := session.New(session.Config{
				CertCatalog:  &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) { return refs, nil }},
				CertSelector: selector,
				KeyProvider: &mockKeyProvider{keyForFn: func(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
					keyID = ref.ID
					return &mockSigningKey{id: ref.ID}, nil
				}},
				TriphaseExec: &mockSimpleExecutor{},
				Progress:     &mockProgressProvider{},
			})
			if err := orch.HandleRequest(context.Background(), solicitudSimpleValida()); err != nil {
				t.Fatal(err)
			}
			if !selected || keyID != "nuevo-p12" || !selector.cleared {
				t.Fatalf("recuperación/limpieza incompleta: selected=%t key=%s cleared=%t", selected, keyID, selector.cleared)
			}
		})
	}
}
