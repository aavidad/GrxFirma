// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"io"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type preflightEffects struct {
	crypto.Signer
	signCalls, tsaCalls, revocationCalls int
}

func (s *preflightEffects) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	s.signCalls++
	return nil, errors.New("unexpected private-key operation")
}

func (s *preflightEffects) RequestTimestamp(context.Context, []byte, crypto.Hash) ([]byte, error) {
	s.tsaCalls++
	return nil, errors.New("unexpected TSA request")
}

func (s *preflightEffects) Fetch(context.Context, *x509.Certificate, *x509.Certificate) (ports.RevocationEvidence, error) {
	s.revocationCalls++
	return ports.RevocationEvidence{}, errors.New("unexpected revocation request")
}

func TestCAdESLTAndLTARejectMissingIssuerBeforeSideEffects(t *testing.T) {
	private, cert := certForTest(t, "Synthetic preflight QA")
	for _, profile := range []string{"LT", "LTA"} {
		for _, missing := range []string{"empty-chain", "nil-issuer"} {
			t.Run(profile+"/"+missing, func(t *testing.T) {
				effects := &preflightEffects{Signer: private}
				key := &LocalSigningKey{Signer: effects, Certificate: cert}
				want := "cadena de certificación vacía"
				if missing == "nil-issuer" {
					key.Chain = []*x509.Certificate{nil}
					want = "emisor nulo"
				}
				lt := NewSignerCAdESLT(NewSignerCAdEST(NewCAdESBESDetached(), effects), effects)
				var engine ports.SignerEngine = lt
				if profile == "LTA" {
					engine = NewSignerCAdESLTA(lt, effects)
				}
				result, err := engine.Sign(context.Background(), preflightJob(), key)
				if err == nil || !strings.Contains(err.Error(), want) || len(result.Data) != 0 {
					t.Fatalf("missing issuer not rejected: %v", err)
				}
				if effects.signCalls != 0 || effects.tsaCalls != 0 || effects.revocationCalls != 0 {
					t.Fatalf("side effects before preflight: sign=%d TSA=%d revocation=%d", effects.signCalls, effects.tsaCalls, effects.revocationCalls)
				}
			})
		}
	}
}

func TestCAdESTRejectsMissingDependenciesBeforePrivateOperation(t *testing.T) {
	private, cert := certForTest(t, "Synthetic TSA preflight QA")
	for _, missing := range []string{"nil-engine", "nil-base", "nil-tsa"} {
		t.Run(missing, func(t *testing.T) {
			effects := &preflightEffects{Signer: private}
			var engine *SignerCAdEST
			switch missing {
			case "nil-base":
				engine = NewSignerCAdEST(nil, effects)
			case "nil-tsa":
				engine = NewSignerCAdEST(NewCAdESBESDetached(), nil)
			}
			result, err := engine.Sign(context.Background(), preflightJob(), &LocalSigningKey{Signer: effects, Certificate: cert})
			if err == nil || !strings.Contains(err.Error(), "configurados") || len(result.Data) != 0 || effects.signCalls != 0 || effects.tsaCalls != 0 {
				t.Fatalf("missing dependency not rejected before signing: %v", err)
			}
		})
	}
}

func preflightJob() domain.SignatureJob {
	return domain.SignatureJob{Document: domain.Document{Name: "qa.txt", MIMEType: "text/plain", Content: []byte("synthetic preflight only")}, Format: domain.FormatCAdES, Action: domain.ActionSign}
}
