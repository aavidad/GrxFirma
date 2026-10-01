// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"grxfirma/internal/domain"
)

func TestReverseCertificateRefs(t *testing.T) {
	t.Parallel()

	items := []domain.CertificateRef{
		{ID: "cofirma-2"},
		{ID: "cofirma-1"},
		{ID: "principal"},
	}
	reverseCertificateRefs(items)
	if got := []string{items[0].ID, items[1].ID, items[2].ID}; got[0] != "principal" || got[1] != "cofirma-1" || got[2] != "cofirma-2" {
		t.Fatalf("orden inesperado tras reverseCertificateRefs: %#v", got)
	}
}

func TestReverseCertificates(t *testing.T) {
	t.Parallel()

	items := []*x509.Certificate{
		{Subject: pkix.Name{CommonName: "cofirma-2"}},
		{Subject: pkix.Name{CommonName: "principal"}},
	}
	reverseCertificates(items)
	if got := items[0].Subject.CommonName; got != "principal" {
		t.Fatalf("primer certificado inesperado tras reverseCertificates: %q", got)
	}
}

func TestReverseStringSlices(t *testing.T) {
	t.Parallel()

	items := [][]string{
		{"firmante=cofirma-2", "algoritmo=1"},
		{"firmante=principal", "algoritmo=2"},
	}
	reverseStringSlices(items)
	if got := items[0][0]; got != "firmante=principal" {
		t.Fatalf("primer bloque inesperado tras reverseStringSlices: %q", got)
	}
}
