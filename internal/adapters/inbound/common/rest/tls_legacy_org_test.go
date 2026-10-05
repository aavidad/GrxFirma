// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

// La hoja autofirmada de la antigua orden CLI se sigue reconociendo con la
// organización de versiones anteriores y con la actual, pero no con otra.
func TestLegacyRESTLeafOwned_OrganizacionActualYAnterior(t *testing.T) {
	casos := []struct {
		org  string
		want bool
	}{
		{localtlstrust.ManagedLocalCAOrganization, true},
		{"Diputacion de Granada", true},
		{"Otra Organizacion", false},
	}
	for _, caso := range casos {
		leaf := hojaLocalhostAutofirmada(t, caso.org)
		if got := legacyRESTLeafOwned(leaf, leaf); !got && caso.want {
			t.Fatalf("legacyRESTLeafOwned(O=%q) = false; se esperaba true", caso.org)
		}
		// Sin raíz que la firme solo cuenta el perfil autofirmado.
		otra := hojaLocalhostAutofirmada(t, localtlstrust.ManagedLocalCAOrganization)
		if got := legacyRESTLeafOwned(leaf, otra); got != caso.want {
			t.Fatalf("legacyRESTLeafOwned(O=%q, raíz ajena) = %v; se esperaba %v", caso.org, got, caso.want)
		}
	}
}

func hojaLocalhostAutofirmada(t *testing.T, org string) *x509.Certificate {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "localhost", Organization: []string{org}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
