// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"testing"

	"grxfirma/internal/domain"
)

func TestPKCS1_FirmaCrudaVerificable(t *testing.T) {
	priv, cert := certForTest(t, "PKCS1")
	datos := []byte("reto de autenticación")
	doc, _ := domain.NewDocument("reto", datos, "application/octet-stream")
	res, err := NewPKCS1Signer().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: formatPKCS1, Action: domain.ActionSign,
		Options: map[string]string{"algorithm": "SHA512withRSA"},
	}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	h := sha512.Sum512(datos)
	if err := rsa.VerifyPKCS1v15(cert.PublicKey.(*rsa.PublicKey), crypto.SHA512, h[:], res.Data); err != nil {
		t.Fatalf("la firma PKCS#1 no verifica: %v", err)
	}
	if res.Algorithm != "SHA512withRSA" {
		t.Fatalf("algoritmo = %q", res.Algorithm)
	}
}
