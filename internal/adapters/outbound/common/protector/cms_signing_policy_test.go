// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto/x509"
	"errors"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/signingpolicy"
)

type cmsSigningPolicyClock struct{ now time.Time }

func (c cmsSigningPolicyClock) Now() time.Time { return c.now }

func TestProtectAndSign_PreflightCertificado(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rootKey, root := newCMSRootCA(t, "CA sintética de aptitud")
	for _, tc := range []struct {
		name          string
		before, after time.Time
		usage         x509.KeyUsage
		want          signingpolicy.Reason
	}{
		{"caducado", now.Add(-2 * time.Hour), now.Add(-time.Hour), x509.KeyUsageDigitalSignature, signingpolicy.Expired},
		{"futuro", now.Add(time.Hour), now.Add(2 * time.Hour), x509.KeyUsageDigitalSignature, signingpolicy.NotYetValid},
		{"solo_cifrado", now.Add(-time.Hour), now.Add(time.Hour), x509.KeyUsageKeyEncipherment, signingpolicy.DisallowedKeyUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private, cert := newCMSSignedLeafWithUsage(t, rootKey, root, tc.before, tc.after, tc.usage)
			result, err := NuevoCMSSignedEnvelopedProtector().WithClock(cmsSigningPolicyClock{now}).ProtectAndSign(context.Background(),
				domain.ProtectionJob{Document: domain.Document{Name: "sintetico.txt", Content: []byte("sintético")}, Profile: domain.ProtectionProfileCompat},
				[]domain.ProtectionRecipient{{ID: "destinatario"}},
				&commonsigner.LocalSigningKey{Signer: private, Certificate: cert})
			var typed *signingpolicy.Error
			if !errors.As(err, &typed) || typed.Reason != tc.want || len(result.Document.Content) != 0 {
				t.Fatalf("preflight: error=%v contenido=%x", err, result.Document.Content)
			}
		})
	}
}
