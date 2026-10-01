// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"testing"

	"grxfirma/internal/domain"
)

func TestValidarAlgoritmosSignerInfoExigeOIDCoherente(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generar RSA: %v", err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar ECDSA: %v", err)
	}
	casos := []struct {
		nombre string
		cert   *x509.Certificate
		firma  asn1.ObjectIdentifier
		valido bool
	}{
		{"rsa correcto", &x509.Certificate{PublicKey: &rsaKey.PublicKey}, oidSignatureRSAWithSHA256, true},
		{"rsa con huella separada", &x509.Certificate{PublicKey: &rsaKey.PublicKey}, oidSignatureRSAEncryption, true},
		{"rsa declarado ecdsa", &x509.Certificate{PublicKey: &rsaKey.PublicKey}, oidSignatureECDSAWith256, false},
		{"ecdsa correcto", &x509.Certificate{PublicKey: &ecdsaKey.PublicKey}, oidSignatureECDSAWith256, true},
		{"ecdsa declarado rsa", &x509.Certificate{PublicKey: &ecdsaKey.PublicKey}, oidSignatureRSAWithSHA256, false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			info := signerInfoRaw{DigestAlgorithm: algorithmIdentifier{Algorithm: oidDigestSHA256}, SignatureAlgorithm: algorithmIdentifier{Algorithm: caso.firma}}
			_, err := validarAlgoritmosSignerInfo(caso.cert, info)
			if (err == nil) != caso.valido {
				t.Fatalf("resultado inesperado: %v", err)
			}
		})
	}
	info := signerInfoRaw{DigestAlgorithm: algorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}}, SignatureAlgorithm: algorithmIdentifier{Algorithm: oidSignatureRSAWithSHA256}}
	if _, err := validarAlgoritmosSignerInfo(&x509.Certificate{PublicKey: &rsaKey.PublicKey}, info); err == nil {
		t.Fatal("se admitió SHA-1 declarado")
	}
}

// El portal elige el resumen (Canarias, Aragón y JCyL piden SHA-512): la
// firma debe usarlo y verificarse; SHA-1 se eleva a SHA-256.
func TestCAdESBES_RespetaAlgoritmoSolicitado(t *testing.T) {
	priv, cert := certForTest(t, "CAdES-algoritmos")
	contenido := []byte("contenido firmado")
	casos := []struct {
		pedido string
		oid    asn1.ObjectIdentifier
		label  string
	}{
		{"SHA256withRSA", oidDigestSHA256, "SHA256withRSA"},
		{"SHA384withRSA", oidDigestSHA384, "SHA384withRSA"},
		{"SHA512withRSA", oidDigestSHA512, "SHA512withRSA"},
		{"SHA1withRSA", oidDigestSHA256, "SHA256withRSA"},
	}
	for _, caso := range casos {
		doc, err := domain.NewDocument("datos.bin", contenido, "application/octet-stream")
		if err != nil {
			t.Fatal(err)
		}
		res, err := NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
			Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign,
			Options: map[string]string{"algorithm": caso.pedido},
		}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
		if err != nil {
			t.Fatalf("%s: %v", caso.pedido, err)
		}
		if res.Algorithm != caso.label {
			t.Fatalf("%s: algoritmo=%q, want %q", caso.pedido, res.Algorithm, caso.label)
		}
		oidDER, _ := asn1.Marshal(caso.oid)
		if !bytes.Contains(res.Data, oidDER) {
			t.Fatalf("%s: la firma no declara el resumen %v", caso.pedido, caso.oid)
		}
		vr, _, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), res.Data, contenido)
		if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
			t.Fatalf("%s: no verifica: %v %+v", caso.pedido, err, vr.Integrity)
		}
	}
}
