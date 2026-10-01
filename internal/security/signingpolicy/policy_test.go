// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signingpolicy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
	"testing"
	"time"
)

func TestValidateCertificateDER(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*x509.Certificate)
		now    time.Time
		want   Reason
	}{
		{name: "grxfirmado_sin_anclas", now: now},
		{name: "limite_inicial_inclusivo", now: now.Add(-time.Hour)},
		{name: "limite_final_inclusivo", now: now.Add(time.Hour)},
		{name: "antes_de_vigencia", now: now.Add(-time.Hour - time.Nanosecond), want: NotYetValid},
		{name: "despues_de_vigencia", now: now.Add(time.Hour + time.Nanosecond), want: Expired},
		{name: "ca_con_permiso_de_firma", now: now, change: func(c *x509.Certificate) { c.IsCA = true }, want: CertificateAuthority},
		{name: "solo_cifrado", now: now, change: func(c *x509.Certificate) { c.KeyUsage = x509.KeyUsageKeyEncipherment }, want: DisallowedKeyUsage},
		{name: "compromiso_de_contenido", now: now, change: func(c *x509.Certificate) { c.KeyUsage = x509.KeyUsageContentCommitment }},
		{name: "uso_ausente", now: now, change: func(c *x509.Certificate) { c.KeyUsage = 0 }},
		{name: "uso_presente_vacio", now: now, change: func(c *x509.Certificate) {
			c.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 15}, Critical: true, Value: []byte{3, 1, 0}}}
		}, want: DisallowedKeyUsage},
		{name: "solo_servidor_web", now: now, change: func(c *x509.Certificate) {
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}, want: DisallowedExtKeyUsage},
		{name: "cliente_y_correo_como_fnmt", now: now, change: func(c *x509.Certificate) {
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageEmailProtection}
		}},
		{name: "servidor_y_cliente", now: now, change: func(c *x509.Certificate) {
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		}},
		{name: "firma_de_documentos_rfc9336", now: now, change: func(c *x509.Certificate) {
			c.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 36}}
		}},
		{name: "solo_sellado_de_tiempo", now: now, change: func(c *x509.Certificate) {
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}
		}, want: DisallowedExtKeyUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
			if tc.change != nil {
				tc.change(cert)
			}
			der, err := x509.CreateCertificate(rand.Reader, cert, cert, key.Public(), key)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateCertificateDER(der, tc.now)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			assertReason(t, err, tc.want)
		})
	}
	t.Run("sin_identidad", func(t *testing.T) { assertReason(t, ValidateCertificateDER(nil, now), MissingIdentity) })
	t.Run("der_invalido", func(t *testing.T) {
		assertReason(t, ValidateCertificateDER([]byte("DER inválido"), now), InvalidCertificate)
	})
}

func assertReason(t *testing.T, err error, want Reason) {
	t.Helper()
	var typed *Error
	if !errors.Is(err, ErrCertificateUnsuitable) || !errors.As(err, &typed) || typed.Reason != want {
		t.Fatalf("error = %v; motivo esperado %s", err, want)
	}
}
