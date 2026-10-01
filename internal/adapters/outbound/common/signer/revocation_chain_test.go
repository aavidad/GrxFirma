// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/x509"
	"testing"

	"grxfirma/internal/domain"
)

// La cadena se construye desde el firmante aunque el CMS (o el P12 de origen)
// guarde los certificados en orden inverso. Con el orden inverso se
// emparejaban certificados que no eran emisor/sujeto y la revocación de un
// certificado revocado de la FNMT quedaba "desconocida".
func TestBuildIssuerChain_OrdenInverso(t *testing.T) {
	leaf, ca, _, _ := generarCadenaLTPrueba(t)
	chain := buildIssuerChain(leaf, []*x509.Certificate{ca, leaf})
	if len(chain) != 2 || !chain[0].Equal(leaf) || !chain[1].Equal(ca) {
		t.Fatalf("cadena mal ordenada: %d elementos", len(chain))
	}
	if got := buildIssuerChain(leaf, []*x509.Certificate{leaf}); len(got) != 1 {
		t.Fatalf("sin emisor disponible la cadena debe quedarse en la hoja, got %d", len(got))
	}
}

// Una revocación no concluyente no puede acabar presentada como
// "certificado vigente": antes se sobrescribía la advertencia al final.
func TestVerifyRevocation_NoConcluyenteNoSeDeclaraVigente(t *testing.T) {
	leaf, ca, _, _ := generarCadenaLTPrueba(t)
	v := NewCAdESVerifierWithChecker(&RevocationChecker{})
	result, err := v.VerifyRevocation(context.Background(), []*x509.Certificate{leaf, ca})
	if err != nil {
		t.Fatal(err)
	}
	if result.Certificate.Status != domain.VerificationStatusWarning {
		t.Fatalf("certificado=%q, want warning (revocación no concluyente)", result.Certificate.Status)
	}
}
