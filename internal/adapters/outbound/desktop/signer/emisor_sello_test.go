// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
)

// La firma y las vistas previas comparten el mismo criterio para el emisor.
func TestEmisorSelloCertificadoCoincideConFirma(t *testing.T) {
	casos := []struct {
		emisor pkix.Name
		want   string
	}{
		{pkix.Name{CommonName: "CA QA", Organization: []string{"Organizacion Emisora"}}, "Organizacion Emisora"},
		{pkix.Name{CommonName: "CA QA"}, "CA QA"},
		{pkix.Name{}, ""},
	}
	for _, c := range casos {
		cert := &x509.Certificate{Issuer: c.emisor, Subject: pkix.Name{CommonName: "Firmante"}}
		if got := EmisorSelloCertificado(cert); got != c.want {
			t.Fatalf("EmisorSelloCertificado = %q; quiero %q", got, c.want)
		}
		if NombreFirmanteSello(cert) != nombreFirmantePAdES(&ClaveLocal{cert: cert}) {
			t.Fatal("el titular de la vista previa no coincide con el de la firma")
		}
	}
}
